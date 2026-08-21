---
name: codeconverter-verify
description: Shared verification helper for the codeconverter pipeline — independently re-derive any count or inventory a stage inherited from a prior artifact, compare it to the claimed figure, and block the stage on a non-zero delta. Invoked by any codeconverter stage that consumes a prior stage's output or a ported legacy artifact.
context: fork
argument-hint: <target-repo> <stage> <claim-id>
---

# codeconverter-verify — independent recount (fork, shared)

**Goal:** No stage builds on a number it did not re-derive. Given a claim from a prior
artifact, this skill re-derives the same figure from source by an independent method,
records both numbers and both commands, and returns a pass/fail the calling stage
must honour.

This is not a pipeline stage. It is a helper every stage calls, and its output is
appended to the **calling stage's** MANIFEST, not to a directory of its own.

**Why this exists.** The IAM run is a catalogue of one failure repeated five times —
a ported artifact trusted without a recount:

| Artifact | Claimed | Actual |
|---|---|---|
| stage 02 `io_matrix` | 12 endpoints | **637** |
| stage 05 ported legacy `API.md` | complete | covered **194 of 637** |
| stage 06 legacy domain list | complete | missed **313 endpoints** (three whole domains) |
| stage 07 | built against ~400 | **637** |
| stage 08 re-run | — | found **269 unmentioned paths** and an inverted port mapping |

None of these were five unrelated mistakes. They were one systemic fault, discovered
five times. The striking part is that the pipeline already knew the fix: stage 05's
exit criteria demand "route count in code equals route count in the document, and the
verification method is shown", and stage 02 demands a count "independently verified
from source code". The capability existed but was stage-local, worded three different
ways, and was never applied to the *ported inputs* of stages 02, 06 and 07.

Making it a shared, callable skill is the whole fix. One procedure, one record format,
one blocking rule.

## Setup

1. Target repo: first argument, or the repo containing `docs/codeconverter/STATE.md`.
2. `<stage>` — the calling stage's directory name (`07-target-codebase`). Its MANIFEST
   receives the verification record.
3. `<claim-id>` — a short identifier for the figure under test (`endpoint-count`,
   `table-count`, `route-coverage`). Used to key the record so repeat runs are
   comparable.
4. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.
5. If the claim concerns an external repository, read
   `docs/codeconverter/00-source-provenance/provenance.json` first. A recount against
   an `unfetched` repo, or a working-tree scan of a repo whose `working_tree` is
   `unrelated`, is **not admissible** — return `blocked`, not a number.

## Execute

Follow `instructions.md`. For the claim under test:

1. Locate the claim: artifact, line, and the exact figure asserted.
2. Re-derive it from source by a method **independent of how the claim was produced**.
   Same-method re-derivation is not verification; it reproduces the original error.
3. Compare, and classify the delta.
4. Return a verification record for the calling stage's MANIFEST.

Write nothing except the record you hand back, plus — if the calling stage asks for
it — a `verification/<claim-id>.json` inside the calling stage's own directory.

## The verification record (mandatory format)

```markdown
### Verification — <claim-id>

| Field | Value |
|---|---|
| Claim | <figure> |
| Claimed by | <artifact>:<line> |
| Re-derived | <figure> |
| Method | <exact command, copy-pasteable> |
| Ref | <repo>@<sha> (<commit date>) — or "n/a, in-repo" |
| Excluded | <paths deliberately not counted, and why — or "no exclusions"> |
| Delta | <n> (<percentage>) |
| Verdict | pass \| fail \| blocked |
```

`Excluded` is the load-bearing field and may never be empty. A count that swept
vendored or generated paths without saying so is how this pipeline once "corrected"
114 → 189 when the answer was ~148.

## Verdict rules (not negotiable)

- **`pass`** — delta is zero, or the delta is explained by a stated exclusion and both
  figures are shown.
- **`fail`** — any unexplained non-zero delta. **The calling stage may not mark its
  MANIFEST `complete`.** It records the fail, corrects the inherited figure, and re-runs
  the verification.
- **`blocked`** — the recount could not be performed: source unavailable, provenance
  inadmissible, or the claim is not stated precisely enough to re-derive. `blocked` is
  never rounded to `pass`.

A stage that reports `complete` with a `fail` record in its MANIFEST is malformed and
the orchestrator sends it back.

## Exit criteria (append to the CALLING stage's MANIFEST)

- [ ] Every figure this stage inherited from a prior artifact has a verification record.
- [ ] Each record shows the claim, the re-derivation, both commands, and the delta.
- [ ] `Excluded` is non-empty on every record, or explicitly states "no exclusions".
- [ ] The re-derivation method differs from the method that produced the claim, and the
      record says how.
- [ ] Zero records have verdict `fail`. Any `blocked` record names what would unblock it.

## Tips from experience

- **Independence is the whole point.** Re-running the original script against the
  original input is not a recount. Count endpoints from route annotations if the claim
  came from a spec file; count from the spec if the claim came from annotations.
- Two methods disagreeing is a *result*, not a failure of the exercise. Record both
  with their commands and let the calling stage reconcile. The unrecoverable case is
  one number with no method.
- Vendored, generated and test paths are where deltas hide. Decide the exclusions
  **before** running the count, write them down, and apply them identically to both
  methods.
- A count of things that *have names* beats a count of lines. `sort -u` on identifiers
  survives reformatting; `wc -l` on matches does not.
- When the claim is an inventory rather than a number, diff the two sets and report
  members-only-in-A and members-only-in-B. "56 == 56" is much weaker evidence than
  "the two 56-element sets are identical", and costs one `diff`.
