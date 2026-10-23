# Conductor service contract

This contract defines the binary, client and persistence boundaries for Conductor. The acceptance plan tracks implementation and verification.

## Authority and packages

One installed `iterate-run` binary owns admission, queue selection, leases, assignment supervision, configuration and results. `serve` binds localhost:8420 for all pages, assets and API routes. A tick is a client of the same Go service; it does not start a webserver. A port collision produces an actionable error without trying another port.

| Package | Responsibility | Owner |
|---|---|---|
| `src/internal/conductor/model` | Versioned JSON domain types, identity and protocol | Coordinator |
| `src/internal/conductor/provider` | Literal argument compiler, readiness, real usage collectors, incremental decoders | providers |
| `src/internal/conductor/budget` | Pure pace, recurrence and overdrive evaluator | budgets |
| `src/internal/conductor` | Atomic configuration/runtime store, queue, evaluator, supervisor, reconciliation | engine |
| `src/internal/iterrun` | Existing plan reader, timeline renderer, single HTTP router and client integration | controls |
| `src/internal/runrecord` | Durable, versioned run record shared by the CLI and Conductor ([run record](run-record.md)) | controls |
| `src/cmd/iterate-run` | CLI parsing and output | controls |

The conductor packages do not import `iterrun`. A `PlanSource` interface supplies canonical plan summaries (each with its dependency state: `waiting`, `waiting_on`, `wait_reason`, `held_by`, `depth`, `milestone`, resolved by `plangraph`), pair content and validated checkpoint writes; an `iterrun` adapter reuses its existing parsers. Provider and budget packages depend only on model and the standard library. `runrecord` depends only on the standard library; the supervisor reaches it through `RecordSource`, an optional extension of the plan source that the `iterrun` adapter implements. HTTP and CLI use service functions rather than duplicate policy.

The initial shared package entrypoints are:

```go
// budget
func Universal() model.PowerBand
func ValidatePolicy(model.PowerBand) error
func ValidateRecurrence(model.Recurrence) error
func WindowAt(model.Recurrence, time.Time) (model.Occurrence, bool, error)
func Evaluate(model.PowerBand, model.Subscription, model.Usage, time.Time,
    model.Band, bool, bool, []model.Occurrence) model.BudgetDecision

// provider
func Templates() []model.Subscription
func Probe(context.Context, model.Subscription) model.Readiness
func Compile(model.Subscription, model.Sequence, model.Assignment) ([]string, error)
func Decode(context.Context, string, io.Reader, func(model.Event) error) error
func CollectUsage(context.Context, model.Subscription) (model.Usage, error)
```

`Compile` returns executable followed by literal arguments and uses the assignment's prompt and result path. An empty optional assignment supports a command preview without execution. `Evaluate` accepts maximum band, honor-overdrive and only-overdrive flags, followed by recorded occurrences; its returned occurrences carry completion transitions for the caller to persist. Package owners can extend these entrypoints through a coordinated contract revision without creating a second implementation.

## Configuration and ownership

`ITERATE_CONDUCTOR_DIR` selects an isolated store; the default is `~/.claude/iterate/conductor`. Configuration uses schema version 2, monotonic revision and explicit percent-used targets. A transaction checks the caller's expected revision, validates the entire draft and references, writes a mode-0600 temporary file, syncs it and atomically renames it while holding a process-safe store lock. Failed writes leave the previous configuration intact. Runtime state and credentials never enter configuration responses as arbitrary environment values.

Projects use absolute, symlink-resolved repository/worktree paths. Distinct worktrees stay distinct. Registration validates an existing folder, assigns a stable ID, starts disabled and does not create repository files. Existing project registry entries remain discoverable. A plan identity includes canonical project path and the exact live plan name or archive filename. URL values never become unchecked filesystem suffixes.

Subscriptions own executable/adapter definitions, sanitized allowance-pool identity, tier/reset metadata, one named power-band reference, presets, launch sequences and overdrive windows. Sequence IDs remain stable; visible names derive from subscription name and suffix. Universal is an editable named policy and the initial default. Named time windows are shared references. Referenced deletions require reassignment in the same transaction. Duplicate creates a fresh identity; upsert with a known ID is idempotent.

Freshness and hard exhaustion apply to each actual allowance pool, independently of display labels and execution-token totals. Unknown data remains unknown. Typical reset metadata is an estimate; only trusted telemetry opens reset-ended overdrive. Multiple subscriptions naming the same pool share concurrency and usage gates.

Provider observations carry a sanitized account identity for binding and mismatch checks. An absent short-window observation blocks when it represents missing evidence. A provider's authenticated response can explicitly declare that no short-window pool exists (`short_unsupported`); admission then evaluates the reported pools and the UI labels short-window capacity unsupported, without displaying a fabricated percentage.

## Decisions and recurrence

An evaluation takes a complete config snapshot, explicit clock, current scoped usage, runtime occurrences, queue and owner state. It returns ordered candidates, one selected candidate or none, machine reason codes, human explanations, bar geometry, queue identities and occurrence transitions. Pure evaluation performs no write, lease acquisition or process launch. A simulator replaces only explicit sample fields and uses this same evaluator. Saving and ticking revalidate against current revisions.

Normal rows specify a cumulative maximum band. Green permits green; yellow permits green and yellow; red permits all soft bands. The first available eligible row wins. Automations sort by priority, then trusted reset and stable identity. Overdrive participants have no row priority: nearest trusted reset wins, with persisted fair rotation for ties. Project fairness prevents starvation across ticks.

Recurrence uses Monday=1 through Sunday=7, IANA timezone, inclusive start, exclusive end, repeat interval and anchor date. Overnight occurrences belong to the start day. Equal start/end is a full day; 23:59 remains a real closing boundary. Named windows can instead start a specified number of hours before the selected subscription's trusted reset. Occurrence IDs include window identity, revision and resolved start instant. A window revision changes only when its timing changes; unrelated configuration edits never release a one-plan claim. DST is evaluated in local civil time and persisted as resolved instants.

Overdrive selects the highest enabled active unmet used target. At used >= target the occurrence completes durably, even if a later reading falls. A trusted reset invalidates the captured allowance epoch. Disable, delete or end removes admission. Overdrive bypasses soft bands only for honoring rules; it never bypasses freshness, hard quota, locks, staged approval or project windows. A 0% target grants no override. A 100% target stops below hard exhaustion. Normal green beyond the target remains dark green; bright green ends exactly at the active target.

## Bounded assignments and results

Admission atomically claims one project/plan and the shared account pool before spawning. Locks identify owner PID, start identity, run and assignment; recovery observes ownership rather than trusting only heartbeat age. Existing iterate heartbeats and nightly/conductor ownership participate in admission. A plan lease spans all agents and scheduler processes.

An assignment contains exactly one unchecked task/validation pair, canonical project, plan, run, assignment ID, step, team, kind, source digest, immutable launch/config snapshot and versioned result destination. The supervisor owns working directory, prompt, result identity and structured transport. User arguments are literal argv entries; reserved transport conflicts fail before launch. AI performs implementation or bounded diagnosis/delivery; Go selects work and advances state.

Custom CLI transport is explicit. `argv_file` uses standalone `{prompt}` and `{result}` argument placeholders for a wrapper that accepts a result destination. `prompt_file` uses `{prompt}` and appends the result-file destination, identity and schema to the coding agent's instructions. Candidate coding-agent templates use prompt-file transport without inventing an unsupported output-path flag. Readiness distinguishes a configured contract from a verified successful assignment.

The result contains matching version/identity/digest, outcome, Boolean task and validation status, an array of plain-string evidence, and an optional blocker. Codex returns that JSON object as its final message; its CLI saves the result destination. Claude and custom file transports write the destination directly. Exit zero and provider terminal events are insufficient. Only a matching, schema-valid, successful result with validation evidence permits the atomic checkpoint writer to check its pair. Unexpected changes to the source pair require reconciliation. Recovery never repeats an ambiguous delivery action automatically.

Kinds are `pair`, `diagnosis`, `delivery` and `intake`. Outcomes are `completed`, `retry`, `blocked`, `partial` and `interrupted`. Retry/no-progress accounting is per plan, pair and state signature, with a bounded attempt limit; repeated unchanged failure parks the plan with an actionable reason. Intake and notifications use durable idempotency keys. Delivery is separately validated before archival or merge claims.

`pause` and graceful `stop` prevent subsequent admission while the current pair reaches its checkpoint. `resume` rechecks every gate. `kill` cancels only recorded owned processes, drains output, reconciles the checkpoint and releases ownership after termination. No lifecycle operation kills a process by executable name alone.

Scheduled `drain_queue` claims successive plans while eligible. `one_plan` claims once per occurrence before spawn; retries retain the claim and parking consumes that slot. At closing, `finish_checkpoint` completes the active pair; `finish_plan` continues only the claimed plan while every other gate still passes; `interrupt` signals the owned process group, applies a bounded kill timeout and records resumable state. Closing actions survive restart. Overdrive end/reset/target remains authoritative even during finish-plan continuation.

## Events and polling

The supervisor concurrently drains stdout and stderr into protected per-run logs. Stdout passes through a bounded incremental provider decoder. Every normalized event contains schema version, canonical project, plan, run, assignment, monotonic run sequence, timestamp, kind and sanitized payload. Events and terminal results persist before checkpoint visibility. A malformed or oversized record produces an explicit error event; storage failure stops admission and fails the assignment rather than claiming success.

Durable JSONL events expose a cursor `(run_id, sequence)`. Reconnection reads strictly after the acknowledged cursor; duplicate provider event IDs and hook observations do not count twice. Token totals use authoritative cumulative/delta semantics, never addition of both. Allowance percentages remain separate observations with account, pool, provenance, reset epoch and observed timestamp.

Visible pages poll every three seconds. Updates replace read-only state and append unseen events while preserving draft values, focus, scroll and filters. Hidden pages suspend polling. Stale/error state is explicit. Polling never reloads the whole document to refresh a form.

The managed web service revisits enabled queued work every thirty seconds and on startup. It starts detached clients of the same binary; the clients acquire authoritative leases and own their child processes. Empty queues perform no AI work. Paused or disabled configuration suppresses this wake path. The optional nightly timer invokes the same admission service, with its own compatibility switch; global Conductor Stop disables admission from both sources. Browser connectivity does not control execution lifetime.

Claude stream tool-use identities match owned hook observations and deduplicate start/end activity. Assistant usage remains a separate event so a hook cannot hide token accounting. Codex stream item IDs lack a reliable hook-ID mapping; its stream supplies Conductor tool activity while the existing hook trace remains separate. Neither hook path can advance checkpoints or authorize quota.

## CLI and HTTP

The command root is `iterate-run conductor`. Nouns are `project`, `subscription`, `sequence`, `preset`, `power-band`, `time-window`, `overdrive`, `automation`, `usage`, `run`, `event` and `config`; collection verbs are `list`, `show`, `create`, `edit`, `duplicate` and `delete` where meaningful. Project show includes its exact queue identities and readiness breakdown. Root verbs are `status`, `explain`, `simulate`, `overdrive-conversion`, `tick`, `start`, `stop`, `pause`, `resume`, `kill` and `migrate`. Existing wrapper `iterate-run run` retains its meaning.

`overdrive-conversion --subscription ACCOUNT --id WINDOW --json` and `POST /api/conductor/overdrive-conversion` call the same read-only conversion service. The HTTP body contains `subscription_id` and `window_id`. The response preserves the window identity and supplies its fractional hour offset, exact occurrence dates, allowance source and configuration revision. Only an ordinary revisioned configuration save commits the returned window. Missing or stale trusted allowance data and recurrence shapes without an equivalent reset-relative occurrence produce explicit errors.

`GET /api/conductor/visibility?project=PATH` supplies shared project and plan-detail presentation: evaluator state, recorded effective launch sequence, admission reason, next opportunity and subscription usage observation age/source. Saved automation participants retain explicit unavailable telemetry even when their queue is empty. The schedule preview intersects project and automation windows within a fourteen-day horizon. Reset-relative windows require trusted current allowance evidence; an absent intersection or unresolved reset remains explicit. A displayed opening does not promise budget, queue or ownership eligibility. Presentation never estimates missing allowance data or dispatches work.

Global `--json`, `--config`, `--help`, `--dry-run` and `--color` parse anywhere, with `--` ending parsing. Structured writes accept `--file <path|->` and `--revision <n>`; `config apply` atomically applies a full document for conversational batch setup. No secret is accepted as a command-line argument. Machine output is one stable JSON value on stdout; diagnostics use stderr. Usage errors exit 2, revision conflicts 3, unavailable/gated operations 4, execution failures 1, success 0. Read-only waiting decisions are successful results containing reasons.

The single HTTP router exposes `/api/conductor/config`, `/status`, `/evaluate`, `/tick`, `/control`, `/events`, `/usage` and resource operations under `/api/conductor/`. Reads return revisions; writes require expected revision and same-origin validation. Invalid drafts return field errors, conflicts return 409, missing identities 404, and errors never echo credentials. Simulation uses POST with explicit draft/sample context but has no side effects. Stream payloads and all filesystem text are escaped in HTML.

The web root presents All projects. `/project?project=<canonical>&tab=plans&filter=queued` owns Plans/Automations/Activity; `/settings?section=subscriptions&subscription=<id>` owns global settings and retains return context. Existing `/plan?project=...&name=...` and `/archive?project=...&file=...` render full timelines inside that same shell. Archived status is historical. Missing files retain project navigation and explain the unavailable source.

`docs/design-guidelines.md` governs compact rows, widths, action links, dialogs and bars. Browser history restores project, filter and settings context. Opening a dialog keeps background scroll; cancel discards only its draft. Testers show one results table, one light-green eligible winner row, and neutral alternatives. Overdrive samples choose only genuinely available enabled windows across participating subscriptions at one common sample instant.

## Behavior ownership map

| Existing operation | Binary responsibility | Bounded AI responsibility |
|---|---|---|
| Scan projects and staged plans | Discovery, canonical identities, eligibility | None |
| Choose account and launch time | Freshness, bands, recurrence, concurrency | None |
| Work a plan | Pair selection, immutable request, result validation | One task/validation pair |
| Diagnose failed validation | Retry accounting and observation dedupe | One explicit diagnosis |
| Recover stalled agent | Process ownership and checkpoint reconciliation | Only ambiguous work requiring judgment |
| Unblock or park | State transitions, reason and next-attempt record | Bounded diagnosis when necessary |
| Intake issue/mailbox | Deduplicate and persist source identity | Bounded triage, not autonomous messaging |
| Complete and deliver | Verify result, preserve evidence, archive | Explicit delivery assignment |
| Pause, wind down, kill | Lifecycle and owned resource cleanup | None |
| Show status and live portal | Durable state, events, timeline and queue | None |
| Record run history | The [run record](run-record.md) observes leases, assignment phases, results and hooks; it never decides admission | None |
| Configure through skill | Revisioned API and validation | Translate the user's request into one atomic edit |

The compatibility mapping retains these behaviors:

- Project launch policy and conductor/project windows intersect. Enabling a conductor supplies standing launch authorization, while schedules still apply. Release plans require a typed release invocation and remain excluded from unattended admission.
- Unblocked plans precede resumable executing plans and oldest staged plans. Paused and human-gated plans remain parked; an operator unblock records the reason and makes the plan eligible again. A triage assignment runs at most once for an unchanged blocked-state digest. The no-progress timer defaults to 45 minutes and resets on validated advancement; an optional wall-clock cap is separate.
- Retry attempts default to five per failing state. Diagnosis can select a different approach, but cannot manufacture authorization, credentials or validated completion. Partial landing uses a separate delivery assignment and records its proof; the original plan and branch history remain attached to unfinished work.
- Empty queues leave rules enabled. Runtime exposes working, watching and stood-down states. A bounded watch revisits only objective self-resolving conditions; human gates cause a deduplicated operator notice and no repeated AI invocation. Staging or unblocking work reactivates admission. Notifications have durable source keys and delivery acknowledgments.
- Opt-in issue intake reads GitHub/GitLab through the existing installed integration, imports at most ten unseen issues per batch by default, and records provider/repository/issue keys. A bounded intake assignment creates the resulting plan and provenance. It never closes, comments on or labels issues. Missing forge capability leaves an explicit intake status and does not block unrelated queued work.
- Existing enrollment and nightly controls call the binary service. A managed tick preserves source ownership and pauses admission during handover; it never spawns a whole AI conductor skill. Concurrent manual execution and stale process reconciliation use canonical project ownership, without changing another session's branch.

Installation reconciles the existing managed 8420 listener and active work before replacing a binary. Migration is repeatable, backs up source state and preserves enabled settings, identities, units and completed occurrences. Preview servers are development artifacts and stop after verified delivery. No acceptance run enables real token exhaustion or dispatches a user's queued project implicitly.
