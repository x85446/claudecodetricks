---
name: testmaster-run
description: TESTMASTER child — execute tests by tier or by id, measure REAL wall-clock durations, update the timing registry, and re-derive tiers from the measurements. The only writer of avg_ms/last_ms/runs. Invoked by /testmaster, by iterate plans' standing end-of-plan test step ("run" = fast+standard, never slow), and by the nightly schedule ("run all").
argument-hint: <fast | standard | slow | all | <test-id> — bare/empty = fast+standard>
version: 1.0.0
---

# /testmaster-run — execute and measure

Obey the shared contracts in `/testmaster`'s SKILL.md. This child is the ONLY thing that writes measured timing into `registry.json` — every duration in the system traces back to a run it performed.

## Steps

1. **Select** from `$1`: a tier name, `all`, a test id, or empty/`run` = **fast+standard** (the iterate-safe set — never include `slow` unless explicitly named or `all`). Unmeasured tests (`tier: "?"`) are included in every selection — running them is how they get a tier.
2. **Plan the batch**: within the selection, group `parallel=yes` tests to run concurrently; `parallel=no` tests run serially after. Wrap anything expected >1min through `iterate-run run` when invoked from inside an iterate plan (heartbeat visibility), plain execution otherwise.
3. **Execute and time each test for real** — capture wall-clock ms per test (the test's own execution, not batch overhead) and the batch total.
4. **Record**, per test:
   - Append `{ts, id, ms, result, tier, runner}` to `./.claude/testmaster/history.jsonl`.
   - Update its registry entry: `last_ms`, `runs += 1`, `avg_ms` (running average over history), `last_result`, `updated`.
   - **Re-derive `tier`** from the new `avg_ms` (fast ≤10s, standard ≤2min, slow >2min). If the tier CHANGED, flag it in the report (`⚠ <id> promoted to slow — 2m40s avg, will leave the iterate-safe set`) and update the test's header comment to match.
5. **Report**: `run <selection>: N passed, M failed, batch <wall-clock> (P parallel / S serial)`, one line per failure with its error gist, plus any tier-change flags. When invoked from an iterate plan, failures are the plan's problem to fix (its validation gate) — report them plainly, don't loop retries here.
6. If invoked with a selection that includes `slow` while an iterate plan is `phase: executing` in this project, say so and require the tier to have been explicitly named — bare `all` during a live plan downgrades to fast+standard with a one-line note.

## Rules

1. **Measured, never estimated.** No duration enters the registry except from an execution this skill performed and timed.
2. **Tier changes are announced, not silent** — a test crossing into `slow` leaves the iterate-safe set, and the maintainer must see that happen.
3. **Respect `parallel=no` absolutely** — it declares side effects measurement can't see.
4. This child never edits test content, never deletes tests, never writes new ones — report gaps/failures to the invoker.
