---
name: testmaster-adopt
description: TESTMASTER child (invoked via /testmaster): brings an existing repo's test suite into the framework — discovers every test, seeds the catalog, and computes which source files each test covers.
argument-hint: <empty = full adopt | --seed-only | --covers-only | <test-id...>>
version: 1.0.0
---
<!-- version: bump on EVERY behavioral change (minor additions, major schema/contract changes, patch wording). -->

# /testmaster-adopt — make an existing suite conform

Obey the shared contracts in `/testmaster`'s SKILL.md. This is the **one-time onboarding pass** every project runs before the rest of TESTMASTER means anything: a repo with hundreds of tests and no catalog gets an index, real tiers, and — the part nothing else produces — a `covers` list per test.

**Why this exists:** `/testmaster-catalog rebuild` re-derives a catalog from TESTMASTER headers plus the registry. A repo that has never run TESTMASTER has no headers, so `rebuild` has nothing to read. Adoption is the step that creates what `rebuild` later maintains.

**Without `covers`, nothing downstream works.** Drift detection is `git diff ∩ covers`; the catalog's own rule says a test with an empty `covers` cannot drift. `/testmaster-catalog impact` — and the FFIV decision `/iterate-planner` renders from it — read the same field. An unadopted project reports `could affect: 0` forever and every plan looks safe.

## Steps

### 1. Detect the runner and enumerate every test

Identify the toolchain, then list the **actual** tests — never infer them from file names:

| Toolchain | Enumerate with |
|---|---|
| Go | `go test -list '.*' ./...` |
| Python | `pytest --collect-only -q` |
| Node | `jest --listTests` / `vitest list` |
| Rust | `cargo test -- --list` |

### 2. Reconcile against `registry.json`

- In the tree but not the registry → **register it** (`runs: 0`, no timing, tier unset).
- In the registry but not the tree → **report as an orphan candidate**. Never delete: that is `/testmaster-prune`'s call under its evidence standard.
- Report both counts. A large orphan set means the registry was hand-maintained and has drifted from reality.

### 3. Measure — delegate, never estimate

Skill tool: `testmaster-run` over the newly registered tests. Tiers come from measured `avg_ms` and nothing else. A test that has not run is `unmeasured`, never guessed into a tier.

State the cost before starting. The registry's existing timing gives a real estimate; if the suite is large, say so and let the user choose `--seed-only`.

### 4. Compute `covers` — the core of adoption

Two sources, and **they are never conflated**:

**a. Ground truth — per-test coverage profile.** Each test is run alone with coverage over the whole module, and the profile names the source files it actually executed:

```bash
go test -run '^TestName$' -coverpkg=./... -coverprofile=<tmp> ./...
# every file in <tmp> with a nonzero count → covers
```
Equivalents: `pytest --cov --cov-report=term-missing`, `jest --coverage --collectCoverageFrom`, `cargo llvm-cov`.

This costs one test-run per test. For a 350-test suite that is 350 runs — bounded, but real. Report the projected wall-clock from registry timing first.

**b. Cheap seed — naming convention.** Where a profile hasn't been computed, map the test file to its subject:

| Language | Convention |
|---|---|
| Go | `foo_test.go` → `foo.go` |
| Python | `test_foo.py` / `foo_test.py` → `foo.py` |
| Node | `foo.spec.ts` / `foo.test.ts` → `foo.ts` |
| Rust | `tests/foo.rs` → `src/foo.rs` |

Only record the mapping when the target file **exists**. A convention guess that resolves to nothing is not a cover.

**Record which one you used.** Every test gains `covers_source`:

- `"coverage"` — derived from a real profile. Trustworthy.
- `"convention"` — inferred from the file name. A starting point, not evidence.
- `"manual"` — set by `/testmaster-catalog link`. Trustworthy.
- absent/empty — unknown coverage, which the catalog reports as a gap.

A `convention` cover is a hypothesis about what a test exercises. It will miss every file a test touches indirectly, which for integration tests is most of them. Never let it pass as measured.

### 5. Classify into requirements

Adopted tests predate any recorded ask, so they have no requirement in the user's words. Do **not** invent one.

- Group by subject (the source file/subsystem in `covers`), one requirement per coherent group, `source: "adopted"` and a `statement` taken from the test's own name/doc comment.
- Anything that won't group lands in `req-unclassified`.
- Both are visible gaps for `/testmaster-derive` to replace with a real requirement later — that is the point of marking them, not a defect to hide.

### 6. Write the catalog and report

Write `./.claude/testmaster/catalog.json` in the schema `/testmaster-catalog` owns. Then print the conformance scorecard:

```
adopt — <project>
  tests discovered:      357
  newly registered:      0      (357 already in registry)
  orphan candidates:     2      → /testmaster-prune
  measured (real tiers): 357    fast 340 · standard 15 · slow 2
  covers: coverage 357 · convention 0 · unknown 0
  requirements:          41 adopted · 0 in the user's words
  → catalog written; /testmaster-catalog status now meaningful
```

Every number is measured or counted. None is estimated.

## Arguments — parse `$1`

- *(empty)* — full adopt: steps 1–6.
- `--seed-only` — steps 1, 2, 5, 6 with convention-only covers. Fast, no test execution. The right first pass on a large suite.
- `--covers-only` — step 4 only, for a project already registered and measured.
- `<test-id...>` — adopt just these (a handful of new tests joining an adopted project).

## Rules

1. **Idempotent.** Re-running adopts only what is new and never downgrades a `coverage` cover to a `convention` one. Safe to run on every project, repeatedly.
2. **Never estimate timing** — delegate to `/testmaster-run`. An unmeasured test gets no tier.
3. **Never delete anything.** Orphans are reported for `/testmaster-prune`.
4. **`covers_source` is mandatory** on every entry this skill writes. An unlabeled cover is indistinguishable from a measured one, which is the exact failure mode this field prevents.
5. **Never fabricate a requirement statement.** Adopted tests are marked `adopted`; only `/testmaster-derive` writes requirements in the user's words.
6. **Report the cost before a full covers pass** and offer `--seed-only` instead. Silently spending 350 test-runs of wall-clock is not acceptable.
