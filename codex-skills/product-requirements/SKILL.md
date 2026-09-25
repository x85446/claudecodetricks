---
name: "product-requirements"
description: "PRODUCT child (invoked via $product): the requirements — testable EARS statements (WHEN/SHALL) with acceptance criteria, each numbered R-NN and traced to its feature and its release."
---


# $product-requirements — stage 5: testable behavior

**Version:** product family 1.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


The bridge between product and engineering. Every requirement is one sentence a tester can pass or fail, tied to the feature it serves and the release it ships in. This is the last WHAT stage — still no technology, but now precise enough to verify.

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$product` — ported.
- `$testmaster-derive` — ported.

## EARS

Easy Approach to Requirements Syntax. The shape forces a trigger and an observable response, which is what makes a requirement testable instead of aspirational.

| Pattern | Form | Use for |
|---|---|---|
| **Event** | WHEN `<trigger>` THE SYSTEM SHALL `<response>` | Something happens |
| **State** | WHILE `<state>` THE SYSTEM SHALL `<response>` | Continuous conditions |
| **Unwanted** | IF `<condition>` THEN THE SYSTEM SHALL `<response>` | Errors and edge cases |
| **Optional** | WHERE `<feature is present>` THE SYSTEM SHALL `<response>` | Configuration-dependent behavior |
| **Ubiquitous** | THE SYSTEM SHALL `<response>` | Always-true invariants |

"The system should be fast" is none of these. "WHEN a search is submitted THE SYSTEM SHALL return results within 500ms at the 95th percentile" is an Event requirement with a number in it.

## Steps

1. **Read** `docs/features.md` (every `F-NN`), `docs/roadmap.md` (each feature's release), and `docs/prd.md` (constraints and success metrics, which become the non-functional requirements).

2. **Write requirements feature by feature**, in roadmap order so v1 is complete first. For each feature ask, in this order — and the last three are where requirements are usually missing:
   - The happy path.
   - Every alternate path a user can take.
   - **The negative case** — invalid input, missing permission, absent dependency.
   - **The interrupted case** — what happens when it fails halfway, and what state the user is left in.
   - **The restore case** — how the user recovers, and what the system must not have lost.

3. **Write acceptance criteria** under each requirement: concrete, checkable, and specific enough that two people would agree whether it passed.

4. **Derive the non-functional requirements** from the PRD's constraints and metrics — performance, availability, security, accessibility, licensing and cost constraints. Each gets a number, not an adjective.

5. **Audit traceability both ways** and report it. Every `R-NN` names a real `F-NN`; every `F-NN` in v1 has at least one requirement. Both directions matter: the first catches invention, the second catches omission.

## Output — `docs/requirements.md`

```markdown
# <Product> — Requirements

**Date:** <YYYY-MM-DD>  ·  **Requirements:** <n>  ·  **v1:** <n>  ·  **v2:** <n>  ·  **v3:** <n>

## v1.0 — MVP

### F-07 — <Feature name>

#### R-14 — <short title>
**Traces to:** F-07 · v1.0  ·  **Type:** functional

WHEN a user submits a form with invalid data
THE SYSTEM SHALL display validation errors next to the relevant fields.

**Acceptance:**
- [ ] Error text names the offending field
- [ ] Valid fields retain the values already entered
- [ ] No submission is recorded

#### R-15 — <short title>
**Traces to:** F-07 · v1.0  ·  **Type:** functional

IF the upstream service is unreachable
THEN THE SYSTEM SHALL preserve the user's entered data and surface a retry.

**Acceptance:**
- [ ] Entered data survives the failure
- [ ] Retry re-submits without re-entry
- [ ] The failure is logged with a correlation id

## Non-functional

#### R-40 — Search latency
**Traces to:** prd:success-metric-2  ·  **Type:** performance

WHEN a search is submitted
THE SYSTEM SHALL return results within 500ms at p95 for catalogs up to 100k items.

**Acceptance:**
- [ ] Measured under the stated catalog size
- [ ] p95, not mean
- [ ] Degradation beyond 100k items is documented, not silent

## Traceability audit
| Direction | Result |
|---|---|
| Requirements naming a nonexistent feature | none \| <list — invented requirements> |
| v1 features with no requirement | none \| <list — unspecified features> |
| Requirements with no release | none \| <list> |
| Acceptance criteria that are not checkable | none \| <list> |

## Open questions
- [NEEDS CLARIFICATION] <question>

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

## Rules

1. **Every requirement is one EARS sentence.** Two behaviors means two requirements. A paragraph is not a requirement.
2. **Stable `R-NN` ids.** Never renumber, never reuse a retired id.
3. **Both traces are mandatory** — the feature and the release. A requirement that serves no feature was invented here, and that is the defect this audit exists to catch.
4. **No technology.** "THE SYSTEM SHALL persist the draft" — not "SHALL write the draft to Redis". Stage 6 chooses the mechanism.
5. **Numbers, not adjectives**, in every non-functional requirement. "Fast", "secure" and "scalable" are not requirements.
6. **The negative, interrupted and restore cases are not optional.** A feature with only a happy path is under-specified, and that gap is where real defects live.
7. **Acceptance criteria must be checkable by someone who didn't write them.** If passing depends on the author's intent, rewrite it.
8. **v1 requirements come first and must be complete** before v2 gets any. The audit checks this.

## Downstream

These requirements are the natural input to `$testmaster-derive`, which turns a stated requirement into the cases it implies — including the negative, every-path, restore-state and interrupted cases this stage already asked for. That is the user's call to make when they start building; this stage stops at the document.
