---
name: codeconverter-11-migration-plan
description: Stage 11 of the codeconverter pipeline — the concrete phased implementation plan, assigning every endpoint to a module and implementation phase with risks spiked first. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 11.
context: fork
---

# Stage 11 — migration-plan (fork)

**Goal:** Produce a concrete, phased implementation plan for the replacement
service. Assign every endpoint to a handler module and implementation phase.
Identify and spike high-risk items first.

**Blocked until stage 10 is complete** — you cannot plan implementation phases
without the alignment decision.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Confirm readiness: MANIFESTs for stages 01–10 all show `complete`. If not, stop
   and report which are missing.
3. Read `docs/codeconverter/STATE.md`,
   `docs/codeconverter/07-target-codebase/stack.md` and `analysis.md`,
   `docs/codeconverter/05-api-surface/API.md`,
   `docs/codeconverter/06-domain-analysis/GAP_ANALYSIS.md` and all domain docs,
   `docs/codeconverter/10-service-alignment/alignment-decision.md`,
   `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md`, and
   `docs/codeconverter/04-test-baseline/tests.md`.
4. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/11-migration-plan/`:

- `behavioral_inventory.md` — testable behavioral invariants
- `framework_mapping.md` — source → target framework capability table
- `implementation_schedule.md` — phased plan with exit criteria per phase
- `risk_register.md` — risk table with mitigations

Update STATE.md's Environment notes with anything the implementation sessions need.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/11-migration-plan/`, plus your row and
  Environment notes in STATE.md.
- All four artifacts start with the standard artifact header block; Status `final`
  when done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] Every endpoint from the alignment routing table is assigned to a handler
      module and an implementation phase.
- [ ] High-risk items are identified and scheduled as early spikes.
- [ ] All four artifacts exist and are mutually consistent (schedule references the
      inventory and risk register).
