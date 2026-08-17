---
name: codeconverter
description: Use when someone asks to rewrite or convert a service to a new language/framework, run the codeconverter pipeline, resume a code conversion, or check conversion status. Orchestrates 11 stages (service profile through migration plan) by delegating to the codeconverter-NN-* child skills.
argument-hint: [init <target-repo> | status | next | run <NN>]
---

# codeconverter — service rewrite orchestrator

A structured, AI-assisted process for safely rewriting a service from one language or
framework to another. The stages front-load all analysis and baseline work so that by
the time implementation begins, the scope is fully understood, all consumers are known,
and a verified behavioral baseline exists to test against.

Each stage is a child skill. The orchestrator routes, verifies uniformity, tracks
state, and commits. It does not do stage work itself.

## Stage table

| Stage | Child skill | Mode | Output dir (under `<target>/docs/codeconverter/`) |
|-------|------------|------|---------------------------------------------------|
| 01-service-profile | `codeconverter-01-service-profile` | **interactive** | `01-service-profile/` |
| 02-codebase-analysis | `codeconverter-02-codebase-analysis` | fork | `02-codebase-analysis/` |
| 03-dependency-discovery | `codeconverter-03-dependency-discovery` | fork | `03-dependency-discovery/` |
| 04-test-baseline | `codeconverter-04-test-baseline` | fork | `04-test-baseline/` |
| 05-api-surface | `codeconverter-05-api-surface` | fork | `05-api-surface/` |
| 06-domain-analysis | `codeconverter-06-domain-analysis` | **interactive** | `06-domain-analysis/` |
| 07-target-codebase | `codeconverter-07-target-codebase` | **interactive** | `07-target-codebase/` |
| 08-gap-validation | `codeconverter-08-gap-validation` | fork | updates `07-target-codebase/analysis.md` in place + own MANIFEST |
| 09-dependency-audit | `codeconverter-09-dependency-audit` | fork | `09-dependency-audit/` |
| 10-service-alignment | `codeconverter-10-service-alignment` | **interactive** | `10-service-alignment/` |
| 11-migration-plan | `codeconverter-11-migration-plan` | fork | `11-migration-plan/` |

Stages run in order. 11 is blocked until 10 is complete; 10 only applies when the
target is an existing codebase (for greenfield, mark it complete with a note
"greenfield — full absorption" in its MANIFEST).

## State

All pipeline state lives in the target repo at `docs/codeconverter/STATE.md`.
Templates for STATE.md, per-stage MANIFEST.md, and the artifact header block are in
`templates.md` in this skill's directory — read it before creating or verifying
anything.

## Commands

**`init <target-repo>`** — Start a conversion:
1. Verify the path is a git repo. Create `docs/codeconverter/` and `STATE.md` from
   the template in `templates.md`.
2. Ask for (or confirm) the conversion branch name; create it in the target repo if
   missing. Record it in STATE.md.
3. Run stage 01.

**`status`** — Read STATE.md and each existing stage MANIFEST.md. Report a table:
stage, status, completion date, open issues. Flag any mismatch between STATE.md and
the MANIFESTs (MANIFESTs are the source of truth; fix STATE.md to match).

**`next`** — Find the first stage whose MANIFEST is missing or not `complete`; run it.

**`run <NN>`** — Run a specific stage. If earlier stages are incomplete, say which
and get explicit confirmation before proceeding out of order.

No argument given: report `status`, then offer `next`.

## Running a stage

1. **Prerequisites:** confirm all earlier stages' MANIFEST.md show `Status: complete`.
2. **Delegate:** invoke the child via the Skill tool, passing the absolute target repo
   path as the argument, e.g. `Skill(codeconverter-04-test-baseline, args: "/path/to/target")`.
3. **Interactive stages (01, 06, 07, 10)** run inline in the conversation — the user
   must be present to answer questions. Warn them before starting one.
4. **Verify uniformity when the child returns** (do not skip this):
   - `MANIFEST.md` exists in the stage dir and matches the template.
   - Every artifact listed carries the standard header block with `Status: final`.
   - Exit criteria are all checked, or Status is honestly `in-progress`/`blocked`.
   - No files written outside the stage dir, except: STATE.md, stage 08's in-place
     update of `07-target-codebase/analysis.md`, and stage 04's test-repo commits.
5. **Record:** update the stage's row in STATE.md.
6. **Commit:** `git -C <target> add docs/codeconverter && git commit -m "codeconverter NN-name: <short summary>"`.
   One commit per stage — never mix stage outputs in one commit, so a later stage can
   be rolled back cleanly if it contradicts earlier findings.

## Rules

- **No journey/journal artifact.** This pipeline intentionally does not keep a
  narrative journal. Do not create one.
- **MANIFESTs are the source of truth** for stage completion, not STATE.md and not
  memory of the conversation.
- **Uniformity is enforced by the orchestrator.** A stage that returns without a
  conforming MANIFEST gets re-invoked to finish its paperwork before its commit.
- **Interactivity roadmap:** the long-term goal is that only stage 01 is interactive.
  Stages 06, 07, and 10 remain interactive until one full pipeline run has completed;
  after that, ask the user whether each should be converted to fork mode.
- **Restart vs continue:** if a forked stage stalls or dies, re-invoke it — each
  stage's instructions are written to resume from the last committed output in its
  stage dir. Restart from scratch only if output contradicts the stage instructions.
