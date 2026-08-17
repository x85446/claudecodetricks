---
name: codeconverter-03-dependency-discovery
description: Stage 03 of the codeconverter pipeline — find every repository that tests, consumes, or borrows code from the source service. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 03.
context: fork
---

# Stage 03 — dependency-discovery (fork)

**Goal:** Find every repository with a relationship to this codebase — testing,
consuming, or borrowing. You cannot safely rewrite until every hidden dependency is
known.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` — it carries the GitHub orgs to scan and the
   conversion branch (recorded by stage 01). If the orgs are missing, stop and report.
3. Read `docs/codeconverter/01-service-profile/service-profile.md`.
4. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Scan the recorded GitHub orgs; any cloned repos go on the
conversion branch. Output goes to `docs/codeconverter/03-dependency-discovery/references.md`.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/03-dependency-discovery/`, plus your row in
  STATE.md.
- `references.md` starts with the standard artifact header block; Status `final` when
  done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] At least 3 rounds of discovery were run (round 1: obvious consumers; round 2:
      shared libraries those repos depend on; round 3: verification and strays).
- [ ] Code *borrowing* was searched, not just API consumption — grepped for package
      names, class names, and utility names unique to this repo.
- [ ] `references.md` is a ranked inventory organized by relationship type.

## Notes

- Every repo listed must state its relationship type and evidence (file/line or
  search hit), not just a name.
