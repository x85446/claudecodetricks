---
name: codeconverter-12-migration-qa
description: Stage 12 of the codeconverter pipeline — the back-end Q&A that runs after the migration plan and works through every decision discovery surfaced but could not make, from a standing agenda seeded with the facts each decision needs. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 12.
argument-hint: [target-repo] [--seed-only]
---

# Stage 12 — migration-qa (interactive)

**Goal:** Close the decisions the pipeline deliberately did not make. Stage 00 asked
what the port is before anything was known; this stage asks what it should *do* now
that everything is known. Its output is a decision log the migration plan is amended
against, and a re-scan request for anything that changed scope.

**Why this stage exists.** Several questions genuinely cannot be answered at stage 00.
"Should we keep publishing to the old message broker during cutover?" depends on the
consumer map (05a), the outbound map (05b) and the target's own capabilities (07) —
none of which exist when stage 00 runs. The pipeline used to end at stage 11 with a
plan containing open decisions embedded as caveats inside a 7-phase schedule, where
they are easy to miss and impossible to track.

Stage 12 pulls them out into a standing agenda. The critical property is that **each
agenda item arrives pre-briefed**: the facts discovery already established are on the
item, so the human is deciding, not being re-educated. An agenda item that makes the
user reconstruct context from scratch has failed at its only job.

The second output is the **re-scan request**. Some answers change scope — "yes, keep
serving the old store" adds work that stages 10 and 11 never planned. Stage 12 does
not silently absorb that; it names the stages the answer invalidates and hands them
back to the orchestrator for a new round.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report.
2. Prerequisite: `11-migration-plan/MANIFEST.md` shows `Status: complete`. This stage
   reads a finished plan; running it earlier produces an agenda missing the items the
   plan itself raises. If 11 is incomplete, say so and run `--seed-only`.
3. Read, in this order — these are the agenda's sources, not background reading:
   - `docs/codeconverter/00-guidance/scope-charter.md` — its `Carry to stage 12` list
     and every `unknown` answer are agenda items by construction.
   - `docs/codeconverter/11-migration-plan/` — every open decision, gate and Phase 0
     blocker.
   - `docs/codeconverter/10-service-alignment/` — every deferred alignment call.
   - `docs/codeconverter/05a-endpoint-consumers/` — the no-known-caller ledger; every
     proposed drop with an `unknown` consumer status.
   - `docs/codeconverter/05b-outbound-dependencies/` — every outbound dependency the
     target cannot currently reach.
   - `docs/codeconverter/05c-datastore-peers/` — the store-change verdict and every
     blocking table.
   - `docs/codeconverter/09-dependency-audit/` — every CRITICAL and HIGH finding with
     no owner or no pre-migration remediation.
   - `docs/codeconverter/07-target-codebase/analysis.md` — capabilities the target has
     and the source does not, which often *are* the answer to an open question.
4. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/12-migration-qa/`:

- `agenda.md` — the standing agenda: one section per open decision, each carrying its
  established facts, the options with their consequences, and a recommendation.
- `decisions.md` — the decision log: what was decided, by whom, on what date, with the
  reasoning and the stages the decision affects.
- `agenda.json` — machine-readable agenda + decision state, so a later round knows
  what is still open.
- `MANIFEST.md` — per the template.

**The agenda is standing, not consumed.** An item stays on it across rounds until it
is decided or explicitly withdrawn. Deciding an item does not delete it; it moves to
`decisions.md` and the agenda row records the decision reference.

### `--seed-only` (non-interactive)

Builds and updates the agenda from artifacts without asking anything. Every item is
seeded with its facts, options, recommendation and consequences, and `status` stays
`open`. This is the mode to run when the human is not present, when the pipeline is
being exercised, or when seeding the agenda ahead of a scheduled decision session.
A `--seed-only` run never writes to `decisions.md`.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/12-migration-qa/`, plus your row in STATE.md.
  Amendments to the migration plan are **requested**, not applied — stage 11 owns its
  own files and re-runs to absorb a decision.
- `agenda.md` and `decisions.md` each start with the standard artifact header block;
  Status `final` when the round closes. The JSON artifact is header-exempt.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] The agenda is **seeded from every source listed in Setup step 3**, and each
      source's contribution is shown in two columns — *contributed* (once per source)
      and *new items* (once per item). A source that produced no items is listed with
      zero, never omitted. The **new-items column sums to the distinct item count**,
      and the check that proves it is shown; a merged item counted twice fails this.
- [ ] Every agenda item carries all five of: **what was deferred**, **the facts
      discovery established** (with artifact citations), **the options**, **the
      consequence of each option**, and **a recommendation with its reasoning**. An
      item missing any of the five is not ready to be asked.
- [ ] Every scope-charter `unknown` and every `Carry to stage 12` entry appears as an
      agenda item, and the check that proves it is shown.
- [ ] Every decision in `decisions.md` names the stages it affects and whether it
      requires a re-scan; the re-scan requests are collected in one place for the
      orchestrator.
- [ ] Items still open at the end of the round are listed as open with the reason —
      an unanswered question is a valid outcome, an omitted one is not.
- [ ] No agenda item re-briefs the user from scratch: each states its facts before its
      question, so the user is deciding rather than reconstructing.

## Tips from experience

- Order the agenda by **what it unblocks**, not by severity. A small decision that
  gates Phase 1 outranks a large one that gates Phase 6, because the plan cannot start
  without it.
- Give every item a recommendation, including the ones you are unsure about. A
  recommendation the user overrules produces a better conversation than a neutral menu
  — it makes the disagreement explicit and the reasoning reviewable.
- The target codebase is frequently the answer. Before framing an item as "how should
  we build X", check whether the target already has X in some form; a partial in-tree
  answer changes the question from "design this" to "is this enough".
- Watch for decisions that are really the same decision. "Keep publishing to the old
  broker during cutover" and "keep serving the old store during cutover" are one
  question — *does the old outbound surface stay up while consumers migrate?* — asked
  about two dependencies. Group them, and let the user answer the shape once.
- When a decision changes scope, say which stages it invalidates **in the decision
  itself**, not in a summary at the end. The invalidation is part of the decision's
  cost and the user is entitled to see it while deciding.
