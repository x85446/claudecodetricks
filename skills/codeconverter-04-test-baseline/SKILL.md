---
name: codeconverter-04-test-baseline
description: Stage 04 of the codeconverter pipeline — get every runnable test suite of the source service to 0 failures, establishing the behavioral baseline the rewrite must match. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 04.
context: fork
---

# Stage 04 — test-baseline (fork)

**Goal:** Every test suite that can possibly run locally passes at 0 failures before
the rewrite starts. This baseline is what the replacement code must match.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` — especially the **Environment notes** section
   (current setup) and the conversion branch.
3. Read `docs/codeconverter/03-dependency-discovery/references.md` and any existing
   `docs/codeconverter/04-test-baseline/tests.md` to learn what suites exist.
4. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Documentation output goes to
`docs/codeconverter/04-test-baseline/tests.md`.

**Hard boundary:** fix test bugs, config, and environment issues only. **Never modify
production source code.** Failures split into four buckets — test bug, missing
config, environment issue, real service bug. Only bucket 4 (real service bug) gets
escalated: document it in the MANIFEST's Open issues and do not fix or hide it.

## Uniform artifact contract (mandatory)

- Documentation writes only into `docs/codeconverter/04-test-baseline/`, plus your
  row and updated Environment notes in STATE.md.
- **Exception:** test-repo/submodule fixes are committed in those repos on the
  conversion branch — that is part of this stage's job.
- `tests.md` starts with the standard artifact header block; Status `final` when done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit of `docs/` belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] Every suite in the inventory was run at least once during this stage (no
      relying on previous run results).
- [ ] All runnable suites are at 0 failures; permanently-skipped suites are
      documented with the reason and enablement steps.
- [ ] `tests.md` has final counts and a baseline-run header; submodule branches are
      committed.
- [ ] STATE.md Environment notes describe the working setup a next session needs.

## Tips from experience

- Common config failures: missing config keys, wrong account tier (free-tier limits
  block user-creation tests), missing RSA public key in test config.
- Common test-bug: Python 2 test code never updated for a Python 3 framework.
- nginx gateways need `location ~ ^/path/?$` (regex, optional trailing slash), not
  `location = /path`, for paths that clients send with a trailing slash.
