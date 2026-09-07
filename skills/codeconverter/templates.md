# codeconverter — Uniform Artifact Templates

Every stage of the codeconverter pipeline MUST use these templates exactly.
Children read this file before writing any output. Do not improvise variants.

**Contents:** 1. artifact header block · 2. MANIFEST.md · 3. STATE.md ·
4. evidence line · 5. correction block · 6. verification record.

Templates 4–6 exist because this pipeline has already shipped wrong numbers three
separate ways: a claim about a foreign tree with no command attached, a "correction"
that made a figure worse, and five inherited counts nobody re-derived. Each of those is
cheap to prevent and expensive to find later.

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
- `NN-stage-name` is the stage's directory name exactly as it appears in the
  orchestrator's stage table, including a letter suffix where the stage has one
  (`05a-endpoint-consumers`, `05b-outbound-dependencies`).
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
**Sibling repos path:** /absolute/path (for stages 03, 05a, 05c, 09)
**Deployment manifests path:** /absolute/path (for stages 05c, 09)

**Current round:** 1
**Round started:** YYYY-MM-DD

## Target-of-record

Owned by `00-source-provenance`. Every claim about the target codebase cites this ref.

**Authoritative ref:** <ref>
**Commit SHA:** <sha>
**Dated:** <commit date> — <subject>
**Pinned:** <date>, after `git fetch`
**Working tree:** same | ahead | behind | unrelated — <if not `same`, what that forbids>

```
$ git -C <target> rev-parse <ref>
<sha>
```

| Stage | Round | Status | Completed | Notes |
|-------|-------|--------|-----------|-------|
| 00-guidance | 1 | not-started | — | |
| 00-source-provenance | 1 | not-started | — | |
| 01-service-profile | 1 | not-started | — | |
| 02-codebase-analysis | 1 | not-started | — | |
| 03-dependency-discovery | 1 | not-started | — | |
| 04-test-baseline | 1 | not-started | — | |
| 05-api-surface | 1 | not-started | — | |
| 05a-endpoint-consumers | 1 | not-started | — | |
| 05b-outbound-dependencies | 1 | not-started | — | |
| 05c-datastore-peers | 1 | not-started | — | required only if the store changes (see 00-guidance Q4) |
| 06-domain-analysis | 1 | not-started | — | |
| 07-target-codebase | 1 | not-started | — | |
| 08-gap-validation | 1 | not-started | — | |
| 09-dependency-audit | 1 | not-started | — | |
| 10-service-alignment | 1 | not-started | — | |
| 10a-pilot-slice | 1 | not-started | — | |
| 11-migration-plan | 1 | not-started | — | |
| 12-migration-qa | 1 | not-started | — | |

Status values: not-started | in-progress | blocked | complete | superseded

`superseded` means the stage completed in an earlier round and a later decision
invalidated it. It is not `complete` and it is not `not-started`; the distinction
matters, because a superseded stage still has readable output that a reader must be
warned about.

The **Round** column is the round in which that row was last written. A row whose
round is lower than **Current round** has not been revisited this round — which is
fine, and is exactly what the column exists to make visible.

## Round history

| Round | Started | Trigger | Stages invalidated | Notes |
|-------|---------|---------|--------------------|-------|
| 1 | YYYY-MM-DD | initial run | — | |

A new round starts when a stage-00 or stage-12 answer changes in a way that
invalidates completed output. The row records what changed and which stages were reset
to `superseded`.

## Phase actuals

Written by the orchestrator at each implementation-phase boundary, once stage 11's
schedule is being executed. Empty until then.

| Phase | Endpoints predicted | Endpoints actual | Effort predicted | Effort actual | Ratio | Schedule correction applied |
|-------|--------------------|------------------|------------------|---------------|-------|------------------------------|

This table exists because self-assessment does not converge on reality on its own.
METR's RCT measured developers predicting a 24% speedup, delivering **19% slower**, and
still believing afterwards they had been sped up by 20%. Without a measured ratio,
nothing in this pipeline would ever notice its estimates were wrong. Recompute the
remaining schedule from the observed ratio at every phase boundary, and record that the
correction was applied.

## Environment notes
Setup needed to run tests, service configs, credentials locations, gateway quirks.
(Whatever the next session needs to continue — the old continue.md content.)
```

---

## 4. Evidence line

**Required for every factual assertion about a codebase the pipeline does not own** —
the target repo, sibling repos, deployment manifests. A prose claim about a foreign
tree is not admissible; an evidence line is.

```markdown
**Claim:** <the assertion, stated so it can be true or false>
**Command:** <exact, copy-pasteable>
**Ref:** <repo>@<sha> (<commit date>)
**Source:** ref | working-tree
**Result:** <the literal output>
```

Rules:

- `Ref` must match the row for that repo in
  `docs/codeconverter/00-source-provenance/provenance.json`. A ref that is not in the
  provenance table is not a ref, it is a memory.
- `Source` is mandatory and is not decorative. A stage greps the **working tree** but
  cites a **ref**, and those are frequently not the same tree. A `working-tree` claim
  against a repo whose `working_tree` status is `behind` or `unrelated` is inadmissible.
- Negative claims need evidence lines too, and they are the ones most often skipped.
  "The target has no MFA" is a claim; `rg -ni 'mfa|totp|webauthn' → 0` at a named ref is
  evidence.

Worked example — the claim that a stale tree got wrong:

```markdown
**Claim:** izcr implements no MFA.
**Command:** git -C izcr ls-tree -r --name-only origin/main | xargs -I{} git -C izcr show origin/main:{} 2>/dev/null | rg -ci 'mfa|totp|webauthn'
**Ref:** izcr@d3a0ca5 (2026-08-13)
**Source:** ref
**Result:** 0
```

---

## 5. Correction block

**Required whenever a stage replaces a previously published figure.** A correction with
no method is not a correction; it is a second unverified claim wearing the authority of
a fix.

```markdown
**Corrected:** <old value> → <new value>
**Method:** <exact command>
**Ref:** <repo>@<sha> (<commit date>)
**Excluded:** <vendored/generated/test paths deliberately not counted, and why — or "no exclusions">
**Superseded:** <artifact + line the old figure came from>
```

`Excluded` is the load-bearing field and may never be empty. It is what a bare number
cannot show and what breaks corrections in practice: this pipeline's izcr endpoint
count went **114 → 189 → 148**, and the 189 was wrong *because* it swept vendored
`protoext/google/api/*` — Google's own protos that izcr carries and does not implement.
That single unstated exclusion was invisible in the number and produced a "fix" more
wrong than the stale figure it replaced.

A correction block is also required when the new figure is the *same* as the old one.
Confirming a figure by an independent method is a result worth recording, and it is the
only way a reader can tell "checked and unchanged" from "never rechecked".

---

## 6. Verification record

**Required for every figure a stage inherits from a prior artifact**, produced by the
shared `codeconverter-verify` skill and pasted into the consuming stage's MANIFEST
under `## Verification records`.

```markdown
### Verification — <claim-id>

| Field | Value |
|---|---|
| Claim | <figure> |
| Claimed by | <artifact>:<line> |
| Re-derived | <figure> |
| Method | <exact command> |
| Ref | <repo>@<sha> (<commit date>) — or "n/a, in-repo" |
| Excluded | <paths not counted, and why — or "no exclusions"> |
| Delta | <n> (<percentage>) |
| Verdict | pass \| fail \| blocked |
```

A stage MANIFEST may not be `complete` while it carries a `fail` record. The
orchestrator's uniformity check enforces this.
