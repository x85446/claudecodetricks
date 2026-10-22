# Run record

The run record is the durable history of each plan execution. `iterate-run` owns it for both execution owners: the CLI (`/iterate` sessions observed through hooks, plus `iterate-run record append`) and the Conductor supervisor. Every report, skill and web page reads the same record through one Go API (`src/internal/runrecord`, wrapped by `src/internal/iterrun/record.go`).

The record observes. The Conductor's leases, checkpoints and run events stay authoritative for admission and results, and the plan file stays authoritative for plan content. A record write never gates admission, alters a result or fails an assignment.

## Storage

```
$ITERATE_RUN_RECORD_DIR                     (default ~/.claude/iterate-run/records)
  <project-key>/project.json                canonical project path
  <project-key>/<plan>/.lock                writer lock for the plan
  <project-key>/<plan>/<run-id>.jsonl       one append-only log per run
```

- **Project key** is the first 16 hex digits of the SHA-256 of the canonical project path (absolute, symlinks resolved). Distinct worktrees and same-named plans in different projects never share a directory.
- **Run id** (`run-<UTC timestamp>-<random>`) names one execution of a codename. A plan that runs, archives and runs again has two runs.
- Archiving a plan file moves nothing in the store. The run records the archive filename in its `archived` lifecycle event.
- `ITERATE_RUN_RECORD_DIR` selects an isolated store, the same way `ITERATE_CONDUCTOR_DIR` isolates the Conductor. The `iterrun` and `cmd/iterate-run` test binaries set it to a temporary directory.

## Event envelope

Each line is one JSON event. Schema version 1:

| Field | Meaning |
|---|---|
| `schema_version` | Version of this event's schema. A reader skips a line with a newer version and counts it as unreadable. |
| `id` | Globally unique event id. Appending an id the run already holds is a no-op. |
| `type` | One of the types below. |
| `at` | Event time: when the observed thing happened. |
| `recorded_at` | When the line was written. |
| `source` | Writer: `conductor`, `cli:hook`, `cli:record` or `import`. |
| `provenance` | `observed` for a runtime observation, `legacy:<source>` for reconstructed history. |
| `project_path`, `project_key`, `project_id` | Canonical project path, its key, and the Conductor project id when one exists. |
| `plan`, `run_id` | Codename and execution. |
| `session_id`, `agent_id`, `team` | Harness session, subagent id and team lane. |
| `requirement` | Plan step number; `0` is plan-level work such as delivery. |
| `attempt`, `attempt_key` | Attempt number, or a key (the Conductor assignment id) that identifies one attempt. |
| `assignment_id`, `conductor_run_id` | Conductor assignment and its per-assignment run directory. |
| `marker` | On `run.opened`: the execution marker, the plan's `Executing:` value as RFC 3339 UTC. |

Exactly one payload object accompanies each type:

| Type | Payload | Carries |
|---|---|---|
| `run.opened` | `versions` | Engine (`claude-code`/`codex`), executor and planner skill versions, `iterate-run` version |
| `run.lifecycle` | `lifecycle` | `state` (queued, admitted, executing, paused, resumed, interrupted, blocked, unblocked, completed, archived), `reason`, `archive_file`, `observed_via` |
| `session.observed` | `versions` | A session linked to the run, with engine, provider and model when known |
| `agent.observed` | — | A subagent and its team |
| `ownership.acquired` / `.heartbeat` / `.released` | `ownership` | `owner_kind` (`cli`/`conductor`), `owner_id`, `pid`, `reason` |
| `blocker.detected` / `.cleared` | `blocker` | `key`, `kind`, `reason` |
| `admission.considered` | `admission` | `decision` (admitted/gated) and the `gate` that decided: automation_disabled, schedule, budget, ownership, queue, other |
| `requirement.observed` | `requirement_state` | Task and validation text and checkbox state. A checked box is not evidence. |
| `attempt.started` / `.finished` | `attempt_info` | `kind`, `outcome` (passed, failed, retry, interrupted, blocked, partial), `duration_ms`, `reason`, engine, provider, model |
| `evidence.observed` | `evidence` | `result` (passed/failed/unknown), `command`, `exit_status`, `artifact_path`/`artifact_url`, `revision` (git SHA), `environment`, `covered_files`, `summary`, `imported` |
| `usage.observed` | `usage` | `request_id`, `scope`, provider, model, lane, input/output/reasoning/cache-read/cache-write tokens, `cost_usd`, `price_source`, `cumulative`, `note` |
| `interval.observed` | `interval` | `cause` (execution, model, tool, build, queue, blocked-operator, schedule-pause, unobserved), `start`, `end`, `lane` |
| `delivery.observed` | `delivery` | `stage` (source_commit, pr, review, ci, merge, install, deploy, runtime_verification), `result`, `ref`, `revision`, `target`, artifact, `url` |
| `coverage.observed` | `coverage` | `telemetry` source, `kind` (session/agent/request/plan), `state` (captured, missing, partial, unavailable), `from`/`to` |

Usage `scope` keeps figures that must never add together apart: `exact` (a provider total for one request), `estimated` (an allocation of a larger total), `session_cost` (whole-session cost, not attributable to the plan) and `allowance` (a subscription allowance observation).

## Folding

A read folds a run's lines into one view (`runrecord.Run`, also `schema_version` 1):

1. Lines whose id an earlier line carried are dropped and counted in `duplicates`.
2. Events sort by `at`, then `id`. Arrival order never changes the result.
3. Attempts group by requirement and attempt key or number. A second `attempt.started` for the same attempt adds no attempt, so duplicates never count as retries.
4. Usage keeps the latest observation per `(scope, request_id)`. Retries and streaming rewrites replace; they never add.
5. `archived` is terminal for a run; the next execution of the codename opens a new run.
6. `unknown` lists every field no event established: engine, model, ownership, usage, evidence, coverage and the rest. Totals are pointers; an absent figure stays absent and a total that some requests lacked lists the field under `partial`. Unknown is never reported as zero.

`torn` reports a trailing partial line from an interrupted write, which the read ignores. `unreadable` counts other skipped lines.

## Durability

- Every append is one `O_APPEND` write followed by `fsync`, under an exclusive `flock` on the plan's `.lock`, so the Conductor and any number of CLI hooks can write at once.
- Before appending, the writer repairs the file's tail: a torn final line is truncated (it was never accepted), and a complete final line that lacks its newline gets one.
- Reads open files read-only, take no lock and create nothing.

## Writers

| Owner | Path | Records |
|---|---|---|
| CLI hooks | `iterate-run hook pre/post` (`observeRecordHook`) | The session and subagent working on an executing plan, at most once per five minutes each, and the plan file's state when it differs from the run's latest lifecycle state. Only a plan whose file lives in the hook's own project is observed; a plan still in drafting opens no run. |
| CLI explicit | `iterate-run record append` | Any event a skill states directly: lifecycle, ownership, blockers, attempts, evidence, delivery, intervals. |
| Conductor | `RecordSource` phases in `supervisor.go` and `events.go` | `admitted` (lifecycle admitted, ownership acquired), `started` (attempt started, lifecycle executing), `event` (provider session and cumulative usage), `finished` (attempt outcome, result evidence with the checked-out git revision, requirement checkpoint, blocker, completion, archive, ownership released). |

Run resolution: a writer appends to the plan's newest run that is not archived and whose marker matches the plan file's `Executing:` value; with none, it opens a new run. A Conductor observation of an assignment always lands in the run holding that assignment's attempt, even after delivery archived the plan.

Conductor evidence comes from the result's prose and is marked `imported`; its result is `passed` only when the Conductor validated a completed result with `validation_met`. Conductor input tokens include cache tokens, which the usage `note` states.

## Readers

All reads are side-effect free: they never acquire or release ownership, launch work, change enrollment or write records.

```
iterate-run record list   [--project <dir>] [--plan <name>] [--json]
iterate-run record show   [<run-id>] [--project <dir>] [--plan <name>] [--archive <file>] [--json]
iterate-run record append --type <type> [--file <path|->] [--project <dir>] [--plan <name>] [--run <id>] [--json]
iterate-run record import [--project <dir>] [--plan <name>] [--archive <file>] [--dry-run] [--json]
iterate-run record types  [--json]
```

Flags parse in every position, `--` ends parsing, and `--project` and `--plan` default to the project holding the working directory and its current plan. `append` reads one event object from stdin or `--file`; identity comes from the flags, and unknown fields are rejected. Exit codes: 0 success (a duplicate append is success), 1 failure, 2 usage error or invalid event, 3 not found.

The dashboard serves the same reads:

| Route | Returns |
|---|---|
| `GET /api/record?project=<path>&plan=<name>` | The live execution's runs, or every run of the codename when no live plan file exists |
| `GET /api/record?project=<path>&archive=<file>` | The runs of one archived execution |
| `GET /api/record?run=<run-id>` | One run |
| `GET /api/record/runs[?project=<path>][&plan=<name>]` | Run summaries |

Any other method answers 405.

## Legacy plans

A plan execution with no record gets a read-only view built in memory from its plan or archive file, the hook event log and the Conductor's run directories. Its events carry `provenance: legacy:plan-file`, `legacy:hook-events` or `legacy:conductor-runs`:

- The plan file supplies steps and checkbox state, `Executing:`/`Finished:` times, harness, versions and prose evidence lines, each marked `imported` with result `unknown`.
- A state whose transition time the file does not state is placed at the file's modification time, and its reason says so.
- Hook events count only when their plan name, project directory and execution window all match; activity in other directories is not attributed, and the coverage note says so.
- Conductor assignments become attempts with their recorded outcomes and stream usage totals.
- Sources that are absent produce `coverage.observed` events with state `missing` or `unavailable`, never zeros.

`iterate-run record import` persists exactly that view with its provenance and stable event ids, so importing twice appends nothing. It refuses an execution that already has an observed run.
