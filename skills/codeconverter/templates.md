# codeconverter — Uniform Artifact Templates

Every stage of the codeconverter pipeline MUST use these three templates exactly.
Children read this file before writing any output. Do not improvise variants.

---

## 1. Artifact header block

Every artifact document (every `.md` file a stage produces) starts with this block,
before any other content. JSON artifacts are exempt (JSON has no comment syntax);
their companion markdown doc carries the header instead.

```markdown
<!-- codeconverter artifact -->
**Stage:** NN-stage-name
**Artifact:** filename.md — one-line purpose
**Status:** draft | final
**Produced by:** codeconverter-NN-stage-name on YYYY-MM-DD
**Inputs:** comma-separated paths this artifact was derived from

---
```

Rules:
- `Status: draft` while the stage is in progress; flip to `final` before the stage's
  MANIFEST is marked complete.
- An updated-in-place artifact (e.g. stage 08 revising `07-target-codebase/analysis.md`)
  appends the reviser to `Produced by:` rather than replacing it.

---

## 2. MANIFEST.md

Every stage output directory ends with exactly one `MANIFEST.md`. A stage is not
done until this file exists, is accurate, and every artifact it lists carries the
header block above.

```markdown
# NN-stage-name — MANIFEST

**Status:** complete | in-progress | blocked
**Started:** YYYY-MM-DD
**Completed:** YYYY-MM-DD (or —)
**Branch:** <conversion branch>

## Inputs read
- path — why it was needed

## Artifacts produced
- path — one-line description

## Exit criteria
- [x] each criterion from the stage SKILL.md, checked honestly
- [ ] unmet criteria stay unchecked and force Status: in-progress or blocked

## Open issues / escalations
- Anything a human must decide, or "none"
```

Rules:
- Never check an exit criterion you did not verify this run.
- `blocked` requires at least one entry under Open issues explaining the blocker.

---

## 3. STATE.md

Lives at `<target-repo>/docs/codeconverter/STATE.md`. Created by `/codeconverter init`,
updated by every stage (its own row only) and by the orchestrator. This file replaces
the old `continue.md` — environment/setup notes live here.

```markdown
# codeconverter — STATE

**Target repo:** /absolute/path
**Source service:** name + language/framework
**Target stack:** filled in by stage 07 (— until then)
**Conversion branch:** <branch>
**Sibling repos path:** /absolute/path (for stage 09)
**Deployment manifests path:** /absolute/path (for stage 09)

| Stage | Status | Completed | Notes |
|-------|--------|-----------|-------|
| 01-service-profile | not-started | — | |
| 02-codebase-analysis | not-started | — | |
| 03-dependency-discovery | not-started | — | |
| 04-test-baseline | not-started | — | |
| 05-api-surface | not-started | — | |
| 06-domain-analysis | not-started | — | |
| 07-target-codebase | not-started | — | |
| 08-gap-validation | not-started | — | |
| 09-dependency-audit | not-started | — | |
| 10-service-alignment | not-started | — | |
| 11-migration-plan | not-started | — | |

Status values: not-started | in-progress | blocked | complete

## Environment notes
Setup needed to run tests, service configs, credentials locations, gateway quirks.
(Whatever the next session needs to continue — the old continue.md content.)
```
