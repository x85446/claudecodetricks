---
name: codeconverter-10a-pilot-slice
description: Stage 10a of the codeconverter pipeline — before the full migration plan is written, actually migrate one thin vertical slice end to end in the target stack and feed what breaks back into the plan. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 10a.
context: fork
argument-hint: [target-repo] [--domain <name>]
---

# Stage 10a — pilot-slice (fork)

**Goal:** Migrate one small domain end to end, in the target stack, and write down
every assumption it broke. The plan that follows is corrected by measurement instead
of resting on estimates.

**Why this stage exists.** The pipeline runs ten stages of analysis and then produces
a phased plan for hundreds of endpoints with **no empirical probe in between**. Every
framework mapping, effort figure and sequencing decision in the schedule stays
theoretical until Phase 1 is underway — which is the most expensive moment to discover
a wrong premise.

Anthropic's own migration guidance puts a mini-migration at step 2 for exactly this
reason: *run a mini-migration on sample files to catch critical issues before
full-scale work*. And the prior probability that an untested plan is right is not
high here: on the IAM run, **five load-bearing premises turned out wrong** — an
endpoint count off by 52×, a ported API doc covering 194 of 637, a domain list missing
313 endpoints, a target analysis built against ~400, and an inverted port mapping. All
five were discovered by later analysis, not by trying anything.

The output is deliberately small: a `findings.md` of rule corrections. This stage is
not the migration and does not try to be. It is the cheapest available experiment that
can falsify the plan before the plan exists.

## Setup

1. Target repo: the argument passed by the orchestrator, or the repo containing
   `docs/codeconverter/STATE.md`.
2. Prerequisite: stage 10 complete. This stage probes decisions stage 10 made; running
   it earlier probes decisions that may still change.
3. Read `docs/codeconverter/00-guidance/scope-charter.md` — the production-status
   answer sets how much the pilot must prove, and the definition of done sets what
   "working" means for the slice.
4. Read `docs/codeconverter/10-service-alignment/routing-table.md` (which endpoints go
   where), `docs/codeconverter/07-target-codebase/` (the target's real current state at
   the pinned ref), and `docs/codeconverter/04-test-baseline/` (the tests the slice can
   be measured against).
5. Read `docs/codeconverter/00-source-provenance/provenance.json` — if the target's
   `working_tree` is `unrelated` or `unfetched`, stop and report. A pilot built against
   the wrong tree teaches the wrong lessons.
6. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/10a-pilot-slice/`:

- `findings.md` — what broke, what each break implies as a **rule**, and which stage-11
  assumption it corrects.
- `findings.json` — the same, machine-readable, so stage 11 can assert it consumed them.
- `slice.md` — what was built, where the code lives, and how to re-run it.
- `MANIFEST.md` — per the template.

**The code the pilot produces is real and belongs in the target repo**, on the
conversion branch, behind whatever gate the target uses for unreleased work. It is not
a throwaway spike in a scratch directory: half the findings come from integrating with
the real build, the real router and the real store.

## Choosing the slice

The slice is chosen to be **maximally informative per unit of work**, not to be easy:

- **3–8 endpoints**, one coherent domain, at least one write among them.
- Touches the real persistence layer. A read-only slice over static data proves nothing
  about the store change, which is usually the largest unknown.
- Crosses at least one framework boundary the plan depends on — the router, the auth
  middleware, the serialization path.
- Has existing tests in the stage 04 baseline, so "it works" has a definition that is
  not a judgement call.
- Is **not** the hardest domain. The pilot must finish; a domain chosen for maximum
  risk usually only proves that it was risky.

Record why the chosen slice satisfies each criterion, and name what it deliberately
does not exercise.

## Uniform artifact contract (mandatory)

- Write documentation only into `docs/codeconverter/10a-pilot-slice/`, plus your row in
  STATE.md. Pilot **code** goes in the target repo's normal source tree — say exactly
  where, in `slice.md`.
- Never delete or rewrite existing target functionality to make the pilot fit. If the
  slice cannot be built without changing existing behaviour, that is a finding, and a
  large one.
- `findings.md` and `slice.md` each start with the standard artifact header block.
- Finish by writing `MANIFEST.md` per the template, exit criteria below copied in and
  honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] A slice of **3–8 endpoints including at least one write** was actually
      implemented in the target stack, and `slice.md` names the files and the command
      that runs it.
- [ ] The slice **runs**: the command, its exit status and its output are recorded. A
      pilot that was designed but not executed does not satisfy this stage — the entire
      point is the empirical result.
- [ ] The slice was exercised against the stage 04 baseline tests for its domain, or —
      where those cannot run against the target — an equivalent check is defined and
      run, and the substitution is justified.
- [ ] Every finding is written as a **rule**, not an incident: the general statement
      that would have prevented it, plus the specific break that revealed it.
- [ ] Each finding names the stage-11 assumption it corrects, or states explicitly that
      it corrects none.
- [ ] The **effort ratio** is recorded: endpoints in the slice, elapsed effort, and the
      resulting per-endpoint figure — with the caveats that make it a floor rather than
      an estimate.
- [ ] What the slice deliberately did **not** exercise is listed, so stage 11 does not
      treat the pilot as broader evidence than it is.

## Tips from experience

- **A finding that stays an incident is wasted.** "The router dropped the trailing
  slash on `/v3/accounts/`" is an incident. "grpc-gateway normalizes trailing slashes
  and the source service does not — every path in the contract with an optional
  trailing slash needs an explicit binding" is a rule, and it applies to the other 600
  endpoints.
- Record what was *easy* as well as what broke. An assumption that a phase would be
  hard, disproved, is worth as much schedule correction as a nasty surprise.
- Time the work honestly and publish the per-endpoint figure as a **floor**. The first
  slice pays one-time setup costs the rest will not; it is also the simplest domain, so
  it under-states the mean. Say both, so nobody multiplies it by the endpoint count.
- The most valuable findings come from the boundaries: auth middleware, serialization
  of the awkward types, error-response shape, and pagination. Pick a slice that has at
  least one of each if you can.
- If the slice cannot be finished, say so and report why — that is a first-class result
  and much more useful than a plan written as if it had succeeded. `blocked` with a
  named obstacle beats `complete` with a fabricated one.
