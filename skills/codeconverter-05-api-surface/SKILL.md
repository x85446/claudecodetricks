---
name: codeconverter-05-api-surface
description: Stage 05 of the codeconverter pipeline — produce the complete API surface contract (every endpoint the replacement must implement). Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 05.
context: fork
---

# Stage 05 — api-surface (fork)

**Goal:** A complete, structured list of every API endpoint the replacement must
implement. This document is the contract — nothing in it is optional.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` and
   `docs/codeconverter/02-codebase-analysis/` for the existing analysis.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce `docs/codeconverter/05-api-surface/API.md`,
organized by audience and direction: external/customer-facing, admin/root-only,
aggregator/multi-tenant, internal cross-service, and intra-codebase message bus.

Cross-check against stage 02 output but don't trust it blindly — it may have missed
routes. Always verify from source.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/05-api-surface/`, plus your row in STATE.md.
- `API.md` starts with the standard artifact header block; Status `final` when done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] A summary count table is present.
- [ ] Delta verification passes: route count in code equals route count in the
      document, and the verification method is shown.
- [ ] The intra-codebase message bus section exists (for Java Vert.x sources: EventBus
      address strings and `consumer()` registrations — this section is easy to miss).
