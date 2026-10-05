---
name: testmaster
description: "TESTMASTER — the SQA test-suite meta. Owns the whole test lifecycle through its children: adopt (onboard an existing suite), derive (turn a requirement into the cases it implies), catalog (organizing index + validity as code changes), maintain, prune, run (real measured timing), report (HTML report card). Route ALL test-suite work here; the meta picks the child."
when_to_use: "Use for any test-suite work: \"testmaster\", \"testmaster init\", \"adopt this suite\", \"onboard testmaster\", \"set up testmaster here\", \"run the tests\", \"maintain the test suite\", \"add tests for <x>\", \"derive tests for <requirement>\", \"should this have a test\", \"what tests does this need\", \"prune the tests\", \"consolidate duplicate tests\", \"clean up the suite\", \"test report card\", \"how are the tests\", \"what's untested\", \"test coverage\", \"what did this change invalidate\", \"which tests drifted\", \"set up nightly tests\". Iterate plans call it as their standing end-of-plan test task (fast+standard tiers only — never the slow/nightly tier mid-plan). All timing comes from real measured runs, never estimates."
argument-hint: <adopt | maintain | prune | run [tier] | report | nightly | status>
version: 1.5.0
---
<!-- version: bump on EVERY behavioral change (minor additions, major schema/contract changes, patch wording). -->

# /testmaster — SQA suite orchestrator (TESTMASTER)

Meta skill. Routes to one child per concern and owns the shared contracts below. Do the routed work via the Skill tool — never inline a child's job here.

| Child | Job |
|---|---|
| `/testmaster-adopt` | One-time onboarding: discover an existing suite, seed the catalog, compute `covers` |
| `/testmaster-derive` | Read a requirement in the user's words and derive the test cases it implies |
| `/testmaster-catalog` | The organizing index: requirement → cases → covered code, plus validity as code changes |
| `/testmaster-maintain` | Write new test cases, update existing ones to match current behavior |
| `/testmaster-prune` | Prune dead tests, consolidate duplicates, conform headers ↔ registry |
| `/testmaster-run` | Execute a tier (or named tests), measure real durations, update the registry |
| `/testmaster-report` | Regenerate the HTML report card from registry + run history |

## Shared contracts (all children obey these)

**The runner is the `testmaster` binary** (`src/cmd/testmaster/` in claudecodetricks, installed to `~/go/bin` by `make install`). It runs tests outside the model's context: a pass is a count, and each failure is one line naming a log. Children drive it and never run, time, or record a test themselves.

```
testmaster suite run [fast|standard|slow|all|<id>|<glob>] [--failed]   # one summary line + one per problem
testmaster suite discover        # ask go / cargo / pytest what tests exist; register the new ones
testmaster suite status          # tiers, results, last run, slowest
testmaster test add <id> --kind shell --cmd '<command>'                # any other language
testmaster test set <id>… --blocking | --serial | --parallel
testmaster test list [--failing|--unmeasured|--tier T] · test show <id> · test remove <id>…
testmaster runner set <kind> -- <argv…>   # the project's real invocation: env, wrapper, venv
```

Kinds: `go` and `pytest` batch a package or directory into one invocation and read per-test results from `go test -json` / JUnit XML. `cargo` builds once and runs each test straight from its binary. `shell` runs any command (exit 0 pass, 77 skip, else fail), which is how every other language gets in.

**State lives at `./.claude/testmaster/`** (project-local, created on first touch):
- `registry.json`: the database. One entry per test: `{kind, <identity: pkg+name | nodeid | target+name | cmd>, dir, args, cmd, file, blocking, parallel_safe, avg_ms, last_ms, runs, tier, last_result, updated}`, plus top-level `runners` (per-kind command overrides), `sources` (where discovery looks) and `last_run` (the whole batch's wall-clock and counts). `cmd` is always the command that reruns that one test. **Measured truth**: `avg_ms`, `last_ms`, `runs` and `tier` come only from the binary's real executions of passing and failing runs (never skips or timeouts), never estimated, never hand-edited.
- `history.jsonl`: an append-only run log. Each test result is `{ts, id, result, tier, runner}`, plus `ms` only when the test really executed. One batch line per run is `{ts, batch, ms, passed, failed, skipped, missing, not_run}`. This is the raw data behind every timing claim.
- `failures/<id>.log`: one per **currently** failing test, holding its full output, exit code, commit and `rerun:` command. It is removed when the test passes again, so the directory is the red set. The directory ignores itself in git.
- `report/index.html`: the self-contained report card (see `/testmaster-report`).

**Tiers are derived from measured `avg_ms`, recomputed by the binary after every run:**
- `fast`: avg ≤ 10s. Always safe to run, any time, including every iterate plan.
- `standard`: avg ≤ 2min. Runs in iterate plans' end-of-plan test task.
- `slow`: avg > 2min. **Not in the default selection** (`run` = fast+standard). A full slow pass belongs to the nightly schedule or an explicit `run slow` / `run all`. The exception is a slow case the iterate plan's own work has drifted: the plan runs it inside its FFIV sweep, wrapped in `iterate-run run` for heartbeat, without asking. Unattended time is where slow tests belong; parking them for the operator's say-so is the failure this tier used to cause.

**Per-test header**: every test function or file carries a TESTMASTER header comment the children keep in sync:
```
# TESTMASTER: id=<stable-id> tier=fast parallel=yes  (avg 3.2s over 41 runs)
```
Division of authority: the **registry is authoritative for timing** (measured), and the **header is authoritative for `parallel=`** (a declared property of the test's side effects that measurement can't infer: a shared DB, port binds, global fixtures). `testmaster test set <id> --serial` carries a header's `parallel=no` into the registry, and nobody ever overrides it.

**Order of a run**: blocking tests first, one at a time, where the first that fails stops the run and everything else is reported not run. Then every parallel-safe invocation runs concurrently, longest first. Then the serial tests run one at a time. Independent tests always all run, however many of them fail. A test binary that crashes partway (a Go panic, a timeout) has its unrun tests run again, so one crash never hides the rest of its package.

**Real-world mandate**: tests exercise the product the way a user/caller does (run the CLI, hit the endpoint, load the page) — the same interactive-testing mandate as `/iterate`. A "test" that only inspects code doesn't survive `/testmaster-prune`.

## Router — parse `$1`

0. **derive** ("derive tests for <requirement>", "should this have a test", "what tests does this need") → Skill tool: `testmaster-derive`, args verbatim. Also the entry point `/iterate-planner` uses on every plan.
0.25. **adopt** ("adopt", "init", "initialize", "onboard", "set up testmaster here", "bring this suite into testmaster", "conform this project") → Skill tool: `testmaster-adopt`, args verbatim. The one-time pass a project runs before anything else here is meaningful.
0.5. **catalog** ("catalog", "status", "drift", "coverage", "impact <plan>", "what did this change invalidate", "what's untested") → Skill tool: `testmaster-catalog`, args verbatim.
1. **maintain** ("maintain", "add/update tests for <x>") → Skill tool: `testmaster-maintain`, args verbatim.
2. **prune** ("prune", "consolidate", "conform", "clean up the suite") → Skill tool: `testmaster-prune`, args verbatim.
3. **run** ("run", "run fast", "run standard", "run slow", "run <test-id>") → Skill tool: `testmaster-run`, args verbatim. Bare "run" = fast+standard (the iterate-safe set).
4. **report** ("report", "report card", "how are the tests") → Skill tool: `testmaster-report`.
5. **nightly** ("nightly", "set up nightly tests") → arm a scheduled full run (all tiers incl. slow) at local midnight via the harness's cron/schedule mechanism, prompt `/testmaster run all`. **Record the mechanism + job id in `./.claude/testmaster/nightly.json`** and tell the user the exact cancel command (a cron needs CronDelete — a /loop stop will NOT kill it). Never arm a second nightly if `nightly.json` already records a live one.
6. **status** ("status") → `testmaster suite status` and relay it: counts per tier, results, last run, failing tests with their logs, slowest 5. Read-only. For the *organized* view (by requirement, with validity) route to `catalog` instead.
7. **default** (anything else describing test work) → decide maintain vs prune vs run by the work's nature and route as above.

## Rules

1. **The slow tier is never in the default selection.** The plan's standing test step is `run` (= fast+standard); a full slow pass is nightly or an explicit "run slow"/"run all". A drifted slow case attributable to the executing plan is named by its drift membership and runs in-plan — never deferred to the operator.
2. **All timing claims trace to `history.jsonl`.** No estimated durations anywhere — a test with `runs: 0` reports "unmeasured", not a guess.
3. **Registry writes go through the binary.** Children call `testmaster`; the meta routes and never edits state itself.
4. **One nightly per project.** Check `nightly.json` before arming; canceling uses the exact recorded mechanism.
5. **Every test traces to a requirement.** A case exists because something was asked for — `/testmaster-derive` records the ask in the user's words, `/testmaster-catalog` keeps the link, and a test that traces to nothing is a `coverage` gap to resolve, not a test to keep by default.
6. **Green is not the same as valid.** A test that passed before the code it covers changed is `drifted`, not `valid` — only a green run against the current tree clears it. Report drift whenever asked for suite health.
7. **An unadopted project answers everything wrong.** With no `catalog.json` — or a catalog whose `covers` are empty — drift is undetectable, `impact` reports `could affect: 0`, and every plan looks safe. When a routed child finds no catalog, say so and route to `adopt` first rather than returning a clean-looking zero.
