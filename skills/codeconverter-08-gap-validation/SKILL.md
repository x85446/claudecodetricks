---
name: codeconverter-08-gap-validation
description: Stage 08 of the codeconverter pipeline — exhaustively validate the target-codebase gap analysis against all prior outputs, both codebases, and all tests. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 08.
context: fork
---

# Stage 08 — gap-validation (fork)

**Goal:** Exhaustively validate `docs/codeconverter/07-target-codebase/analysis.md`
by cross-referencing it against all prior stage outputs, both codebases, and all
test cases. Every gap found is added directly to the analysis document.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` and the primary target:
   `docs/codeconverter/07-target-codebase/analysis.md`.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Run all **5 iterations × 9 cycles**. Add gaps to the
analysis document as found. Commit after each iteration (intermediate commits are
required by the playbook — make them).

## Uniform artifact contract (mandatory)

- This stage's writes: `docs/codeconverter/07-target-codebase/analysis.md` (updated
  in place — append this stage to the header's Produced-by line), its own
  `docs/codeconverter/08-gap-validation/MANIFEST.md`, and your row in STATE.md.
  Nothing else.
- Finish by writing the MANIFEST per the template, exit criteria below copied in and
  honestly checked. List the revised `analysis.md` under Artifacts produced.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] All 5 iterations × 9 cycles executed; iteration commits exist.
- [ ] Every gap found was written into `analysis.md`, not a side document.
- [ ] The final iteration found no new gaps (or remaining gaps are listed under Open
      issues with reasons they can't be closed here).
