---
name: codeconverter-08-gap-validation
description: Stage 08 of the codeconverter pipeline — exhaustively validate the target-codebase gap analysis against all prior outputs, both codebases, and all tests. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 08.
context: fork
---

# Stage 08 — gap-validation (fork)

**Goal:** Exhaustively validate `docs/codeconverter/07-target-codebase/analysis.md`
by cross-referencing it against all prior stage outputs, both codebases, and all
test cases. Every gap found is added directly to the analysis document.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` and the primary target:
   `docs/codeconverter/07-target-codebase/analysis.md`.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Run all **5 iterations × 9 cycles**. Add gaps to the
analysis document as found. Commit after each iteration (intermediate commits are
required by the playbook — make them).

## Uniform artifact contract (mandatory)

- This stage's writes: `docs/codeconverter/07-target-codebase/analysis.md` (updated
  in place — append this stage to the header's Produced-by line), its own
  `docs/codeconverter/08-gap-validation/MANIFEST.md`, and your row in STATE.md.
  Nothing else.
- Finish by writing the MANIFEST per the template, exit criteria below copied in and
  honestly checked. List the revised `analysis.md` under Artifacts produced.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] All 5 iterations × 9 cycles executed; iteration commits exist.
- [ ] Every gap found was written into `analysis.md`, not a side document.
- [ ] **Every cycle names the external command it ran and records that command's
      output** in the MANIFEST — a route count, a compiler, a test run, an exit status,
      a fetched ref. At least one external oracle per cycle, whose output this stage
      did not author.

      This replaces "the final iteration found no new gaps" as the stage's convergence
      signal, because that criterion cannot distinguish convergence from exhaustion.
      Huang et al. (arXiv:2310.01798): "LLMs struggle to self-correct their responses
      without external feedback, and at times, their performance even degrades after
      self-correction." Every validator in the industry literature is external —
      compile-and-test at Google, build/test-then-repair at AWS, lint/tsc/Jest per file
      at Airbnb, response diffing at Twitter. This stage is the pipeline's designated
      safety net; it must not be the one loop the evidence says does not work.
- [ ] Remaining gaps are listed under Open issues with reasons they cannot be closed
      here.
- [ ] **Every figure this stage changed carries a correction block** (templates.md §5),
      and the block's `Excluded` field is non-empty or explicitly states "no
      exclusions". A recount is not self-validating; only its method is reviewable, so
      only its method makes it a correction.
- [ ] Every figure this stage inherited carries a `codeconverter-verify` record.
      Verdict `fail` on any record blocks `complete`.

### `--adversarial` mode

When invoked with `--adversarial`, this stage runs as a **separate forked invocation
with no access to the reasoning that produced the artifact**, and its goal is to
falsify the analysis rather than complete it. Two extra criteria apply:

- [ ] The pass ran with no context from the producing session — state how that was
      ensured.
- [ ] **Every cluster of findings is generalized into a rule**, not a list of patches.
      The five recount failures on the IAM run were not five unrelated mistakes; they
      were one systemic fault (ported artifacts trusted without re-derivation)
      discovered five times. A pass whose deliverable is a rule names it once.

Run `--adversarial` at least once before stage 11 is treated as costed. Anthropic's
own guidance puts humans on "patterns, not individual failures" for exactly this
reason, and Salesforce staffed bug bashes with "engineers outside the project team".
