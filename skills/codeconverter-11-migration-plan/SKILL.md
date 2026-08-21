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

- `behavioral_inventory.md` — testable behavioral invariants (the *what*)
- `parity_harness.md` — how old and new will be proven equivalent at cutover (the
  *how*): replay corpus, comparison method, noise cancellation, write handling, and the
  mismatch rate that authorizes cutover, with a parity method per endpoint
- `framework_mapping.md` — source → target framework capability table
- `implementation_schedule.md` — phased plan with exit criteria per phase
- `build_and_release_scope.md` — every build file, CI pipeline, release manifest and
  chart from stage 02's `build_inventory.json`, assigned to a phase
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
- [ ] All artifacts exist and are mutually consistent (schedule references the
      inventory and risk register).
- [ ] **Every dropped endpoint cites its `05a-endpoint-consumers` row**, and zero
      endpoints are dropped whose consumer status is `unknown`. Same rule as stage 10,
      enforced again here because the plan is where a drop becomes real work not done.
- [ ] **`10a-pilot-slice/findings.md` was consumed**, and the schedule reflects each
      finding — or states, per finding, why it does not. A plan that ignores the only
      empirical evidence available to it is a plan that chose not to look.
- [ ] **`parity_harness.md` exists** and assigns every endpoint in the routing table a
      parity method: `replay-and-diff`, `test-suite-only`, or `manual-with-rationale`.

      `behavioral_inventory.md` says *what* must hold; this says *how* it will be
      proven at cutover. It must specify which traffic is replayed, how responses are
      compared, **how noise is cancelled**, how write endpoints are handled, and what
      mismatch rate authorizes cutover. Diffy's three-instance design is the reference:
      two instances of the *old* code define the noise floor (timestamps, random
      values, request IDs) that is subtracted before the candidate is judged.
      tap-compare names the three things that make a naive diff useless on writes —
      replica **state drift**, **write ordering** under concurrency, and
      **idempotency**. An auth service whose responses carry tokens, session IDs and
      timestamps is close to the worst case for naive response diffing, and the
      noise-cancellation design is not something to improvise on cutover day.
- [ ] **`build_and_release_scope.md` exists**, seeded from stage 02's
      `build_inventory.json`, assigning every build file, CI pipeline, release manifest
      and chart to a phase. Endpoints are not the work; on the only large-N migration
      dataset available, build/packaging/CI was **84% of commits** (arXiv:2510.14928).
      A schedule organized only around endpoints has sized the small half.
- [ ] **The schedule states its estimating basis** — what the estimates are derived
      from, and in what units — so a later actual can be compared against it. Stage 11
      also creates the empty `Phase actuals` table in STATE.md if it does not exist.

      Estimates produced without a feedback loop do not self-correct: METR's RCT
      measured developers predicting a 24% speedup, delivering **19% slower**, and
      still believing afterwards they had been sped up by 20%. Without a stated basis
      there is nothing for an actual to falsify.
