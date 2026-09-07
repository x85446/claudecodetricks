---
name: codeconverter-02-codebase-analysis
description: Stage 02 of the codeconverter pipeline — exhaustive I/O matrix, call graph, and storage map of the source service. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 02.
context: fork
---

# Stage 02 — codebase-analysis (fork)

**Goal:** Understand every input, output, and internal call in the source codebase
before touching it. Produces the structured analysis every later stage builds on.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` and
   `docs/codeconverter/01-service-profile/service-profile.md`.
3. Read `.claude/skills/codeconverter/templates.md` (uniform artifact templates).
4. Read `instructions.md` in this skill's directory — the full stage playbook (its
   preamble maps legacy phase numbering).
5. Work on the conversion branch recorded in STATE.md.

## Execute

Follow `instructions.md`. All output goes to `docs/codeconverter/02-codebase-analysis/`,
including `build_inventory.json` + its companion markdown: every build file, CI
pipeline, packaging script, container definition and deployment manifest in the source
repo, counted by category. Stage 11 schedules the build and release surface from it.

**Resume behavior:** if the stage dir already has committed output, pick up from the
last completed iteration — do not start over. Restarts mid-stage are normal.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/02-codebase-analysis/`, plus your row in STATE.md.
- Every markdown artifact starts with the standard header block (JSON artifacts are
  covered by their companion markdown doc's header). Flip Status to `final` when done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- Intermediate commits are allowed where instructions.md requires them; the
  stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] `io_matrix.json`, `call_graph.json`, and `storage_map.json` exist with
      companion markdown docs for each deliverable.
- [ ] The endpoint count in the output matches a count independently verified from
      source code — do not accept "done" until they match. Any count inherited from a
      prior or ported artifact carries a `codeconverter-verify` record (templates.md
      §6) with verdict `pass`.
- [ ] Every service, schema, and class in the codebase appears in the analysis.
- [ ] `build_inventory.json` + companion markdown exist, enumerating every build file,
      CI pipeline, packaging script, container definition and deployment manifest in
      the source repo, with a count for each category.

      This inventory is not bookkeeping. The only large-N dataset available on real
      migrations — Google's ISA migration, 38,156 categorized commits
      (arXiv:2510.14928) — found **"Build, Packaging & CI/CD Configuration" was 32,204
      commits, roughly 84% of the total**, while code-adaptation categories together
      were "1% of commits and less than 4% of lines of code", explicitly "refuting the
      conventional wisdom that code translation accounts for most of a migration".
      Stage 11 schedules from this inventory; without it, the plan sizes the 4% and
      omits the 84%.

## Tips from experience

- This stage exhausts single-session context after 10–15 iterations; expect to be
  re-invoked and resume from committed output.
- Evidence over summary: every claim in the analysis should cite file paths.
