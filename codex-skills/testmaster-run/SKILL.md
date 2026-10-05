---
name: "testmaster-run"
description: "TESTMASTER child (invoked via $testmaster): runs a tier or named tests through the testmaster binary, which measures them and updates the registry, and acts only on what failed."
---


# $testmaster-run — run the suite without reading it

**Version:** 2.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$testmaster`'s SKILL.md. The `testmaster` binary does the running, timing and bookkeeping outside your context. This child picks the selection, invokes it once, and acts on the lines that come back. A passing test costs you nothing but a count.

## Usage

Argument: <fast | standard | slow | all | <test-id|glob> | --failed — bare/empty = fast+standard>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$testmaster` — ported.
- `$testmaster-prune` — ported.

## Steps

1. **Select** from `$1` and pass it straight through:
   - empty / `run` → `testmaster suite run` (fast + standard + every unmeasured test: the iterate-safe set)
   - a tier, `all`, an id, or a glob (`'iterrun.*'`) → `testmaster suite run <words>`
   - "rerun the failures" → `testmaster suite run --failed`

   Inside an executing iterate plan, a bare `all` becomes the default set with a one-line note, and `slow` runs only when named: by the tier word, or by a slow case id the plan's FFIV sweep named. That case runs without asking.
2. **Run it once.** From an iterate plan, wrap it for heartbeat (`iterate-run run -- testmaster suite run …`); otherwise run it bare. Exit 3 means nothing is registered: route to `$testmaster adopt`. If `testmaster` is not on PATH, stop with `testmaster is not installed — make install in claudecodetricks`. Never fall back to running tests one by one yourself: that is the token spend the binary exists to remove.
3. **Read the output, not the tests.** It is one summary line plus one line per problem:
   ```
   run fast+standard: 158 passed, 2 failed, 7.9s
   FAIL	iterrun.TestStatus	.claude/testmaster/failures/iterrun.TestStatus.log
   FAIL	3 tests in ./src/internal/git	.claude/testmaster/failures/git.TestA.log	package failed before any test ran
   TIMEOUT	e2e.login	.claude/testmaster/failures/e2e.login.log	killed after 10m0s
   MISSING	iterrun.TestGone	no test by this name in ./src/internal/iterrun
   BLOCKED	140 tests not run: blocking test build did not pass
   TIER	iterrun.TestSlowThing	standard→slow	2m40s avg	leaves the default selection
   ```
   All green is the first line alone. That is the whole report, so open nothing.
4. **Act per line:**
   - `FAIL` / `TIMEOUT`: open the log only when you are going to fix that failure (an iterate plan's validation gate, or the user asked). It holds that test's own output, the exit code, the commit, and a `rerun:` line that reruns just that test. After a fix, `testmaster suite run --failed` confirms it.
   - `MISSING`: the registry names a test its runner no longer has. That is stale state, not a failing product: hand it to `$testmaster-prune`.
   - `BLOCKED`: fix the blocking test first. Nothing else ran.
   - `TIER`: set that test's TESTMASTER header `tier=` to match. A move to `slow` leaves the default selection, so say so.
5. **Report** the summary line and the problem lines as they came. When invoked from an iterate plan, failures are the plan's to fix (its validation gate). Report them plainly and don't loop retries here.

## Rules

1. **Measured, never estimated.** Only the binary writes `avg_ms`, `last_ms`, `runs`, `tier`, `last_result`, `history.jsonl`, `failures/`, and the catalog's `last_validated_commit`. Never hand-edit them, and never time a test yourself.
2. **A pass is a count.** Never rerun a passing test to look at it, and never read a log for a test that passed.
3. **Tier changes are announced, not silent.**
4. **Blocking and serial are declared, not guessed**: `testmaster test set <id> --blocking` for a test whose failure makes the rest meaningless (a build, a fixture server), `--serial` for a header's `parallel=no`.
5. This child never edits test content, never deletes tests, never writes new ones. It reports gaps and failures to the invoker.
