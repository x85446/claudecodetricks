---
name: testmaster-prune
description: TESTMASTER child (invoked via $testmaster): deletes dead tests, consolidates duplicates, and conforms headers to the registry.
---


# $testmaster-prune — keep the suite true

Obey the shared contracts in `$testmaster`'s SKILL.md. This child is the suite's gardener: it removes and merges, it never authors new coverage.

## Usage

Argument: <optional scope, e.g. "the export tests" — default whole suite>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$testmaster` — ported.
- `$testmaster-maintain` — ported.
- `$testmaster-run` — ported.

## Steps

1. **Dead-test sweep.** For each test (scoped by `$1`, else all): does the behavior it asserts still exist in the product? A test for removed code gets deleted — test file/function, registry entry, and header all together. Evidence standard: the feature is demonstrably gone from the code (grep the symbols/endpoints/flags it exercises), not merely "test is failing".
2. **Duplicate consolidation.** Tests asserting the same behavior through the same path merge into one (keep the most real-world variant; fold in any unique assertions from the others). Registry entries merge too — keep the survivor's measured history.
3. **Truth check.** Every remaining test must actually run and assert current behavior. A test that's skipped, commented out, or permanently red is either fixed (behavior drifted → update the assertion to current truth), tiered correctly, or deleted with the dead-test evidence above — never left rotting.
4. **Conform pass** — three-way sync per test:
   - Header `id=` ↔ registry entry exist for each other (orphans on either side get repaired).
   - Registry's measured `tier` is written into the header (registry wins on timing — it's measured).
   - Header's `parallel=` is written into the registry (header wins on parallelism — it's declared). A header missing `parallel=` gets `no` until someone verifies otherwise.
5. **Prove the suite still runs**: execute the fast tier once end-to-end after any deletion/merge. A prune that breaks the runner isn't a prune.
6. Report: `pruned: N deleted (dead), M consolidated → K, C conformed; fast tier green`. List each deleted test with its one-line evidence.

## Rules

1. **Deletion requires evidence the feature is gone** — a red test alone is a fix-or-tier decision, never a delete justification.
2. Never author new test cases here — hand gaps you notice to `$testmaster-maintain` (name them in the report).
3. Never edit measured timing fields — only `$testmaster-run` writes `avg_ms`/`last_ms`/`runs`.
