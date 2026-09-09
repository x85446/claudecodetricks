---
name: testmaster-maintain
description: TESTMASTER child (invoked via $testmaster): writes new test cases and updates existing ones to match current product behavior.
---


# $testmaster-maintain — keep test cases true to the product

**Version:** 1.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$testmaster`'s SKILL.md (state paths, tiers, headers, real-world mandate) — read it if not already in context.

## Usage

Argument: <what changed / what needs coverage, e.g. "cover the new export endpoint">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$testmaster` — ported.
- `$testmaster-prune` — ported.
- `$testmaster-run` — ported.

## Steps

1. **Find the suite.** Locate the project's existing test layout (`make test` target, `tests/` dir, `*_test.go`, `spec/`, etc.). Follow the project's own conventions — never introduce a second test framework beside a working one.
2. **Scope the work** from `$1` (or the iterate plan's changed steps / branch diff when invoked mid-plan): which behaviors are new, which changed, which existing tests now assert stale behavior.
3. **Write/update cases — as real-world as possible.** Each test exercises the product the way a caller does: invoke the real CLI, hit the running endpoint, drive the real script against a local/test target. Mocks only where the real dependency is genuinely unavailable, and say so in the test's comment.
4. **Stamp the header** on every test you touch: `# TESTMASTER: id=<stable-id> tier=? parallel=<yes|no>` (comment syntax per language). Choose `parallel=no` whenever the test binds ports, mutates shared state, or uses global fixtures — when unsure, `no`. `tier=?` is correct for a new test: the tier is measured, not guessed.
5. **Register**: add/update the entry in `./.claude/testmaster/registry.json` (`runs: 0`, `avg_ms: null`, `tier: "?"` for new tests — `$testmaster-run` fills in reality).
6. **Prove each new/updated test executes**: run it once right now (this also gives the registry its first real measurement — record it via the same update `$testmaster-run` would make). A test that was never executed is not maintained, it's decoration.
7. Report one line per case touched: `+ <id> (new|updated, first run <result> in <ms>)`.

## Rules

1. New behavior without a test, and changed behavior with an un-updated test, are both incomplete work — cover both directions every invocation.
2. Never delete or consolidate tests here — that's `$testmaster-prune`'s job (removal needs its dead-code evidence standard).
3. Never mark `parallel=yes` without checking the test's side effects.
