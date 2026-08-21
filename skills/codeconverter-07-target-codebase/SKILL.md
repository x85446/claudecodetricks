---
name: codeconverter-07-target-codebase
description: Stage 07 of the codeconverter pipeline — establish the target codebase (new or existing), define the stack, and quantify the implementation gap. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 07.
---

# Stage 07 — target-codebase (interactive)

**Goal:** Establish the replacement codebase. Decide new project vs existing, define
the technology stack, clone and branch the repo, and produce a gap analysis of what
must be built or added. **Be present — this stage asks questions and waits.**

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, ask the user. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md`, `docs/codeconverter/05-api-surface/API.md`,
   and `docs/codeconverter/06-domain-analysis/GAP_ANALYSIS.md`.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Begin with Step 0: ask whether this is a new project or an
existing project, and **wait for the answer**. The interview covers: language and
framework (with a sensible default), data store choices (defaults match the source),
the replacement repo URL, branch name (default: the conversion branch in STATE.md),
and containerization/CI preferences.

**What it produces**, in `docs/codeconverter/07-target-codebase/`:
- `stack.md` — technology decisions and project coordinates
- `analysis.md` — new project: scope statement (N endpoints, domain features
  required); existing project: gap analysis (what's there, what must be added, what
  must not be changed)

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/07-target-codebase/`, plus your row and the
  Target-stack field in STATE.md.
- `stack.md` and `analysis.md` start with the standard artifact header block; Status
  `final` when done. (Stage 08 will later revise `analysis.md` in place — it appends
  itself to the header's Produced-by line.)
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] New-vs-existing decision recorded with the user's confirmation.
- [ ] `stack.md` has complete technology decisions and repo coordinates.
- [ ] `analysis.md` quantifies the implementation gap (endpoint counts, domain
      features), consistent with API.md and GAP_ANALYSIS.md.
- [ ] The replacement repo exists locally **and appears in
      `00-source-provenance/provenance.json` with a fetch timestamp no older than this
      stage's start date**. "Exists locally on its branch" is not sufficient and never
      was: a clone that has never been fetched satisfies it perfectly, which is how
      this pipeline once analysed a six-month-old target tree while upstream had moved
      306 commits.
- [ ] The target's `working_tree` status is recorded. Where it is not `same`, every
      figure in `analysis.md` states whether it came from **the ref** or **the disk**,
      and no figure is taken from the disk when the status is `unrelated`.
- [ ] **Every factual claim about the target codebase carries an evidence line**
      (templates.md §4): claim, exact command, ref, source, literal result. This
      applies to negative claims — "the target has no X" — which are the ones most
      often asserted from memory and were the ones that propagated wrongly into stages
      10 and 11 on the IAM run.
- [ ] Every figure inherited from a prior artifact carries a `codeconverter-verify`
      record with verdict `pass`, and every superseded figure carries a correction
      block (templates.md §5) whose `Excluded` field is non-empty.

## Interactivity note

This stage is interactive for now (stack and repo decisions). After one full
pipeline run, the orchestrator will ask whether to convert it to fork mode with
defaults taken from the service profile.
