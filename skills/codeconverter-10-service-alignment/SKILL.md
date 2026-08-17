---
name: codeconverter-10-service-alignment
description: Stage 10 of the codeconverter pipeline — decide which feature domains the replacement absorbs and which split out, producing the routing table. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 10.
---

# Stage 10 — service-alignment (interactive)

**Goal:** Decide whether the replacement absorbs all features of the source service
or some split out. The source may be a monolith that accumulated features for years;
the rewrite is the chance to draw better boundaries. API consumers don't care about
internal boundaries — as long as endpoints exist at the same paths, the gateway can
route to different backends. **The AI presents options; the human decides.**

**Applicability:** only relevant when the replacement is an existing codebase. For a
greenfield rewrite, write a MANIFEST with note "greenfield — full absorption" and
mark the stage complete.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, ask the user. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md`,
   `docs/codeconverter/05-api-surface/API.md`, all domain docs in
   `docs/codeconverter/06-domain-analysis/`,
   `docs/codeconverter/07-target-codebase/stack.md` and `analysis.md`, and
   `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md`.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Begin with Step 0 (confirm readiness), then Step 1:
present the feature domain inventory and **wait for the user's confirmation**.

Produce `docs/codeconverter/10-service-alignment/alignment-decision.md` — which
domains go where, the routing table, bad-actors impact, and the rationale.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/10-service-alignment/`, plus your row in
  STATE.md.
- `alignment-decision.md` starts with the standard artifact header block; Status
  `final` when done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] Every feature domain has a decided destination, confirmed by the user.
- [ ] The routing table covers every endpoint in API.md — each has a destination and
      a gateway rule.
- [ ] Tightly coupled domains (e.g. accounts + policies needing real-time account
      data) are flagged with their coupling analysis.

## Tips from experience

- Don't assume full absorption is right — if the source has 12 domains and the
  replacement naturally handles 8, the other 4 may be better standalone or deferred.
- The routing table is the deliverable that matters most.

## Interactivity note

This stage is interactive for now (alignment decisions). After one full pipeline
run, the orchestrator will ask whether to convert it to fork mode.
