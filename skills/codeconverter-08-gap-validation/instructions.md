<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase07-gap-validation.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 08-gap-validation**. When the text below says "Phase N",
> translate with this table:
>
> | Legacy phase | codeconverter stage |
> |---|---|
> | (service profile interview) | 01-service-profile |
> | Phase 1 | 02-codebase-analysis |
> | Phase 2 | 03-dependency-discovery |
> | Phase 3 | 04-test-baseline |
> | Phase 4 | 05-api-surface |
> | Phase 5 | 06-domain-analysis |
> | Phase 6 | 07-target-codebase |
> | Phase 7 | 08-gap-validation |
> | Phase 8 (bad actors) | 09-dependency-audit |
> | Phase 9 | 10-service-alignment |
> | Phase 10 (also titled "Phase 8 — Migration Plan") | 11-migration-plan |
>
> All file paths in this document have been rewritten to the
> `docs/codeconverter/` layout. There is no journey.md/journal in this pipeline —
> ignore any journaling instructions. Where this document conflicts with the
> stage's SKILL.md output contract (uniform headers, MANIFEST.md, output
> directory), **SKILL.md wins**.

---

# Phase 7 — Gap Validation

> **Prerequisite:** Phase 6 must be complete.
> Required inputs: `docs/codeconverter/07-target-codebase/analysis.md` (the primary target),
> `docs/codeconverter/02-codebase-analysis/` through `docs/codeconverter/06-domain-analysis/` (validation sources),
> the source codebase (location from `docs/codeconverter/02-codebase-analysis/`),
> the target codebase (location from `docs/codeconverter/07-target-codebase/stack.md`),
> and the test suite (location from `docs/codeconverter/04-test-baseline/tests.md`).

---

## Mission

The analysis document (`docs/codeconverter/07-target-codebase/analysis.md`) is the most critical
output of the planning process. **If anything is missing from it, the replacement
service will be incomplete.** A missed endpoint, behavioral contract, or architectural
constraint discovered during implementation costs ten times more to fix than one caught
here.

This phase exhaustively validates that analysis document by cross-referencing it
against every prior phase output, every line of the source codebase, every line of
the target codebase, and every test case — repeatedly.

The output of this phase is a fully validated, gap-free analysis document, committed
and ready for Phase 8.

---

## Method — 5 Iterations × 9 Cycles

Run **5 full iterations**. Each iteration runs all 9 cycles in order. Within each
cycle, compare the analysis document against the source named and add any gap found.

Do not stop at the first iteration. Gaps found in iteration 3 are normal. The goal is
convergence: by iteration 5, the gap count per cycle should be zero or near zero.

After each iteration, commit additions to `docs/codeconverter/07-target-codebase/analysis.md`.

---

## The 9 Cycles

### Cycle 1 — Phase 01 Output Validation

**Source:** All documents in `docs/codeconverter/02-codebase-analysis/`.

Cross-reference every item in the phase01 output against the analysis document:

- Every storage table, schema, and ownership decision listed in phase01 must appear
  in the analysis document's "Infrastructure Specifications" or "Hard Constraints"
  sections.
- Every data store (databases, caches, message queues) must be documented with its
  compatibility implications for the target codebase.
- Every data flow or dependency identified in phase01 must have a corresponding note
  in the analysis document.

For each item missing from the analysis document, add it.

---

### Cycle 2 — Phase 02 Output Validation

**Source:** `docs/codeconverter/03-dependency-discovery/references.md` and all documents in `docs/codeconverter/03-dependency-discovery/`.

Cross-reference every reference document listed in phase02 against the analysis
document:

- Every external system, library, or protocol dependency in `docs/codeconverter/03-dependency-discovery/references.md` must
  have an entry in the analysis document's compatibility or gap sections.
- Every behavioral specification referenced in phase02 (e.g. RFCs, vendor docs) must
  be traceable to a requirement in the analysis document.
- Any dependency that requires special handling in the target framework must be flagged
  in the "Conflicts requiring human resolution" or "Addition Strategy" sections.

For each item missing from the analysis document, add it.

---

### Cycle 3 — Phase 03 Output Validation

**Source:** `docs/codeconverter/04-test-baseline/tests.md` and all documents in `docs/codeconverter/04-test-baseline/`.

Cross-reference the test baseline against the analysis document:

- Every test file listed in `docs/codeconverter/04-test-baseline/tests.md` must correspond to at least one behavioral
  contract in the analysis document.
- Every behavioral contract implied by a test (error shapes, HTTP status codes, field
  names, ordering, pagination, timing) must be explicitly stated in the analysis
  document's "Behavioral Contracts" section.
- Any test that validates a behavioral nuance not derivable from the API surface alone
  (e.g. rate limiting, session limits, FIFO semantics) must be documented.

For each implied behavior not in the analysis document, add it to the "Behavioral
Contracts" section with the test file as source.

---

### Cycle 4 — Phase 04 Output Validation

**Source:** `docs/codeconverter/05-api-surface/API.md` and all documents in
`docs/codeconverter/05-api-surface/`.

Cross-reference the full API surface against the analysis document:

- Every endpoint in the API surface document must appear in either the "What the
  Existing Codebase Already Has" or "Missing Endpoints" sections of the analysis
  document. No endpoint may be unaccounted for.
- Every request parameter, response field, and error response documented in phase04
  must have a corresponding note in the analysis document.
- Admin, aggregator, and internal endpoint groups must all be represented — not just
  external/customer-facing ones.

For each unaccounted endpoint or field, add it.

---

### Cycle 5 — Phase 05 Output Validation

**Source:** All documents in `docs/codeconverter/06-domain-analysis/` (domain analysis files).

Cross-reference every domain analysis document against the analysis document:

- Every domain feature documented in phase05 must appear in the analysis document's
  "Existing Domain Coverage" table or "Missing Domain Features" section.
- For domains marked "Not present" in the analysis, verify the phase05 document
  contains enough detail to implement from scratch. If not, flag the gap.
- If any phase05 domain document has not yet been written, add a warning to the
  analysis document identifying the missing document and the risk it poses to Phase 8.

For each gap, add it.

---

### Cycle 6 — Phase 06 Output Validation

**Source:** `docs/codeconverter/07-target-codebase/stack.md` and prior sections of
`docs/codeconverter/07-target-codebase/analysis.md` itself.

Perform an internal consistency check on the analysis document:

- Every item in "What Needs to Be Added" must have a corresponding entry in either
  "Missing Endpoints" or "Missing Domain Features".
- Every conflict listed in "Conflicts requiring human resolution" must have an explicit
  statement of both the source behavior and the target constraint that creates the
  conflict.
- Every row in the "Addition Strategy" table must be traceable to a specific gap.
- The implementation order must cover all high-risk items identified in the gap tables.
- Stack decisions in `stack.md` (language, framework, data stores) must all be
  reflected in the "Compatibility Notes" and "Decisions Log" of the analysis document.

For each inconsistency or untraceable entry, resolve it in the analysis document.

---

### Cycle 7 — Source Codebase Deep Scan

**Source:** The source codebase being replaced (path from `docs/codeconverter/02-codebase-analysis/`).

Read the actual source code — do not rely on prior phase outputs alone. Look for:

- Endpoints or handlers not present in `docs/codeconverter/05-api-surface/API.md`.
- Behavioral logic (validation, error handling, side effects) not captured in any
  phase output.
- Internal APIs (cross-service, admin, operational) that are not documented.
- Data model fields, constraints, or defaults that differ from what is documented.
- Security checks, rate limits, or access control rules embedded in code but not in
  any specification document.
- Event types, message formats, or async behaviors not documented in prior phases.
- Configuration values (timeouts, limits, TTLs, thresholds) that must be replicated
  exactly in the target.

For each item found in the source code but absent from the analysis document, add it.

---

### Cycle 8 — Target Codebase Deep Scan

**Source:** The target codebase (path and branch from `docs/codeconverter/07-target-codebase/stack.md`).

Read the actual target code — do not assume what the target does or does not have.
Look for:

- Existing functionality that overlaps with the source service (partial implementations
  that may need extension rather than full construction).
- Existing patterns (routing, middleware, auth, storage) that constrain how new
  functionality must be added.
- Conflicts between existing target behavior and required source behavior — for example,
  if the target's auth model differs from the source's.
- Native capabilities in the target that can substitute for source implementations
  (e.g. a built-in policy engine, an ORM, an auth middleware).
- Constraints imposed by the target framework (e.g. protocol requirements, schema
  limitations, configuration conventions) that affect how source behavior is ported.

For each finding that affects the gap analysis or the addition strategy, update the
analysis document.

---

### Cycle 9 — Test Case Deep Scan

**Source:** All test files in the source test suite (from `docs/codeconverter/04-test-baseline/tests.md` and any
test directories identified in phase03).

Read test code directly — not just test names. Look for:

- Assertions on specific field names, values, or types not documented elsewhere.
- Assertions on error message text that must be reproduced exactly.
- Assertions on HTTP status codes for edge cases (rate limits, conflicts, permissions).
- Multi-step flows (enrollment, confirmation, session lifecycle) implied by test
  sequences but not captured as a linear behavioral contract.
- Timing or ordering constraints (FIFO eviction, TOTP windows, rate-limit cooldowns)
  verified by test logic.
- Response fields that are present on creation but absent on subsequent reads (secrets,
  one-time values).
- Duplicate-handling behavior, case-sensitivity rules, and reserved-name enforcement
  verified by negative test cases.

For each test-verified behavior not in the analysis document's "Behavioral Contracts"
section, add it with the test file as source.

---

## How to Add Gaps

When a cycle finds something missing:

1. Determine the correct section of `docs/codeconverter/07-target-codebase/analysis.md`:
   - New endpoint → "Missing Endpoints"
   - Partially different behavior → "Partially Covered Endpoints"
   - Domain feature → "Missing Domain Features"
   - Test-verified behavior → "Behavioral Contracts (Test-Verified)"
   - Infrastructure detail → "Infrastructure Specifications"
   - Architectural constraint → "Hard Constraints"
   - Unresolvable conflict → "Conflicts requiring human resolution"

2. Add the entry with enough specificity that an engineer can implement it without
   reading the source service's code.

3. Cite the source (file path and line number where relevant).

4. After completing all 9 cycles for an iteration, commit the additions to
   `docs/codeconverter/07-target-codebase/analysis.md` with a message indicating the iteration
   number and which cycles produced additions.

---

## Exit Criteria

Phase 7 is complete when:

- [ ] All 5 iterations × 9 cycles have been run
- [ ] The final iteration produces zero new gaps across all 9 cycles
- [ ] `docs/codeconverter/07-target-codebase/analysis.md` is committed with all additions
- [ ] A summary has been presented to the human listing:
  - Total gaps found per iteration
  - Most significant finds (what would have been missed without this phase)
  - Confirmation that the final iteration was clean (zero new gaps)
- [ ] Human has confirmed the analysis document is ready for Phase 8

---

## Proceeding to Phase 8

After human confirmation, the next step is:

**Phase 8 — External Dependency Audit ("Bad Actors")**
Prompt: read `.claude/skills/codeconverter-09-dependency-audit/instructions.md` and follow the instructions.
Read `docs/codeconverter/03-dependency-discovery/references.md` and `docs/codeconverter/05-api-surface/API.md` for context.
The human will provide the path to sibling repos and deployment manifests.
