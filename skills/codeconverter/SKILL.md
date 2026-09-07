---
name: codeconverter
description: Use when someone asks to rewrite or convert a service to a new language/framework, run the codeconverter pipeline, resume a code conversion, start a new scan round, or check conversion status. Orchestrates 18 stages (guidance and provenance through the back-end Q&A) across multiple rounds by delegating to the codeconverter-* child skills.
argument-hint: [init <target-repo> | status | next | run <NN> | round | invalidate <NN>...]
---

# codeconverter — service rewrite orchestrator

A structured, AI-assisted process for safely rewriting a service from one language or
framework to another. The stages front-load all analysis and baseline work so that by
the time implementation begins, the scope is fully understood, all consumers are known,
and a verified behavioral baseline exists to test against.

Each stage is a child skill. The orchestrator routes, verifies uniformity, tracks
state, and commits. It does not do stage work itself.

## Stage table

| Stage | Child skill | Mode | Output dir (under `<target>/docs/codeconverter/`) |
|-------|------------|------|---------------------------------------------------|
| 00-guidance | `codeconverter-00-guidance` | **interactive** | `00-guidance/` |
| 00-source-provenance | `codeconverter-00-source-provenance` | fork | `00-source-provenance/` |
| 01-service-profile | `codeconverter-01-service-profile` | **interactive** | `01-service-profile/` |
| 02-codebase-analysis | `codeconverter-02-codebase-analysis` | fork | `02-codebase-analysis/` |
| 03-dependency-discovery | `codeconverter-03-dependency-discovery` | fork | `03-dependency-discovery/` |
| 04-test-baseline | `codeconverter-04-test-baseline` | fork | `04-test-baseline/` |
| 05-api-surface | `codeconverter-05-api-surface` | fork | `05-api-surface/` |
| 05a-endpoint-consumers | `codeconverter-05a-endpoint-consumers` | fork | `05a-endpoint-consumers/` |
| 05b-outbound-dependencies | `codeconverter-05b-outbound-dependencies` | fork | `05b-outbound-dependencies/` |
| 05c-datastore-peers | `codeconverter-05c-datastore-peers` | fork | `05c-datastore-peers/` |
| 06-domain-analysis | `codeconverter-06-domain-analysis` | **interactive** | `06-domain-analysis/` |
| 07-target-codebase | `codeconverter-07-target-codebase` | **interactive** | `07-target-codebase/` |
| 08-gap-validation | `codeconverter-08-gap-validation` | fork | updates `07-target-codebase/analysis.md` in place + own MANIFEST |
| 09-dependency-audit | `codeconverter-09-dependency-audit` | fork | `09-dependency-audit/` |
| 10-service-alignment | `codeconverter-10-service-alignment` | **interactive** | `10-service-alignment/` |
| 10a-pilot-slice | `codeconverter-10a-pilot-slice` | fork | `10a-pilot-slice/` + pilot code in the target's source tree |
| 11-migration-plan | `codeconverter-11-migration-plan` | fork | `11-migration-plan/` |
| 12-migration-qa | `codeconverter-12-migration-qa` | **interactive** | `12-migration-qa/` |

Plus one shared helper that is **not** a stage:

| Helper | Skill | Mode | Output |
|---|---|---|---|
| independent recount | `codeconverter-verify` | fork | a verification record appended to the **calling stage's** MANIFEST |

Stages run in order, with the lettered and zero-prefixed stages taking their place in
the sequence.

- **00-guidance runs first, before anything is analysed.** It establishes what the port
  actually *is* — scope, production status, field upgrades, store change, consumer
  lockstep, done — and its scope charter sets which later stages are `required`,
  `optional` or `not-applicable`. Read the charter before scheduling anything.
- **00-source-provenance runs immediately after**, and is re-invoked by any stage that
  reads an external tree. No stage may compute a figure from a repo with no provenance
  row, and no stage may cite a working-tree scan of a repo whose `working_tree` status
  is `unrelated`.
- **05a, 05b and 05c** all consume `05-api-surface/API.md` (05c also consumes stage 02's
  storage map) and all three feed 10-service-alignment. They run after 05 and before 10,
  are independent of each other and of 06–09, and may run concurrently.
- **05c is conditional**: required whenever the scope charter's Q4 says the store
  changes or is `unknown`; otherwise optional. That condition is written into the
  charter by stage 00, not decided here.
- **10a-pilot-slice** runs between 10 and 11. It is the pipeline's only empirical probe
  and stage 11 must consume its findings.
- **12-migration-qa runs last**, after the plan exists, and works the standing agenda of
  everything discovery surfaced but could not decide.

11 is blocked until 10 is complete; 10 only applies when the target is an existing
codebase (for greenfield, mark it complete with a note "greenfield — full absorption"
in its MANIFEST).

### What each discovery stage uniquely answers

Stage 05a answers "who calls each endpoint", 05b answers "what does the service call
out to", and 05c answers "who else touches our data, and by what route". None of these
is answered anywhere else: 09-dependency-audit deliberately hunts *hidden* coupling
repo-by-repo, so a consumer making ordinary REST calls is correctly outside its scope —
and therefore invisible until 05a runs — while 09's direct-database findings are
per-consumer risks, not the per-table map a store swap actually needs.

Any drop, merge or deprecate decision in stage 10 that is not backed by 05a's consumer
map is a guess. Any store change not backed by 05c's per-table verdict is a silent
cutover failure waiting to happen.

## Rounds

The pipeline is a **loop, not a single pass**. The linear order above is the order of a
round; it is not the shape of the whole job.

```
00-guidance (Q&A)  →  scan (01 … 09)  →  discovery (05a/05b/05c)  →  10 → 10a → 11
                                                                            ↓
                          re-scan  ←  invalidated stages  ←  12-migration-qa (Q&A)
```

Rounds exist because the pipeline's two interactive bookends ask the same questions at
different times. Stage 00 asks them before anything is known, and its answers are
necessarily provisional — often literally `unknown`, or `derived` from artifacts with no
human in the loop. Stage 12 asks them again once ten stages of facts exist. When an
answer changes, the stages that were built on the old answer are wrong, and the pipeline
has to say so rather than leaving stale output in place.

**Starting a round.** A new round begins when an answer changes scope. The trigger is
almost always one of:

- a stage-12 decision whose `Affects` list names a completed stage;
- a stage-00 re-interview that changes one of the six charter answers;
- a provenance finding that invalidates a stage's inputs (a stale tree, a rejected ref);
- a `codeconverter-verify` record with verdict `fail` on a figure a later stage consumed.

**What a round does to STATE.md:**

1. Increment **Current round** and add a **Round history** row: round number, date,
   trigger, stages invalidated, notes.
2. Set every invalidated stage's Status to `superseded` — not `not-started`. Its output
   still exists and is still readable, and a reader needs to be warned rather than
   left to assume it was never produced.
3. Re-run each superseded stage. On completion it gets Status `complete` and its Round
   column set to the current round.
4. Stages not invalidated keep their old Round number. A row whose round is lower than
   Current round has simply not been revisited, which is the normal case and is exactly
   what the column makes visible.

**Which stages a change invalidates.** The decision belongs to whoever made the change —
stage 12 records it in the decision's `Affects` field, stage 00 in its round-history row
— and the orchestrator executes it. The orchestrator does not infer invalidation, but it
does enforce two rules:

- A stage may not be `complete` at round N while an input it consumed is `superseded`.
  Either re-run it or mark it `superseded` too.
- A stage re-run in a later round must state in its MANIFEST what changed since its
  previous round and why the re-run was needed. A silent re-run loses the reason.

**Rounds are cheap on purpose.** Most re-runs touch two or three stages, not all
eighteen. The failure this structure prevents is the expensive one: a plan built on a
premise that was revised two stages later, with nothing recording that it had been.

## State

All pipeline state lives in the target repo at `docs/codeconverter/STATE.md`.
Templates for STATE.md, per-stage MANIFEST.md, and the artifact header block are in
`templates.md` in this skill's directory — read it before creating or verifying
anything.

## Commands

**`init <target-repo>`** — Start a conversion:
1. Verify the path is a git repo. Create `docs/codeconverter/` and `STATE.md` from
   the template in `templates.md`, with **Current round: 1**.
2. Ask for (or confirm) the conversion branch name; create it in the target repo if
   missing. Record it in STATE.md.
3. Run **stage 00-guidance** — the scope charter comes before any analysis, and it
   decides which later stages apply.
4. Run **stage 00-source-provenance** — pin every tree before anything reads one.
5. Run stage 01.

**`status`** — Read STATE.md and each existing stage MANIFEST.md. Report a table:
stage, round, status, completion date, open issues. Flag any mismatch between STATE.md
and the MANIFESTs (MANIFESTs are the source of truth; fix STATE.md to match). Also
flag, as errors:
- any stage `complete` whose MANIFEST carries a `codeconverter-verify` record with
  verdict `fail`;
- any stage `complete` at the current round that consumed a `superseded` input;
- any stage whose Round column is behind Current round, listed as *not revisited this
  round* — informational, not an error.

**`next`** — Find the first stage whose MANIFEST is missing, not `complete`, or
`superseded`; run it. Skip stages the scope charter marks `not-applicable`, and say
which were skipped and why.

**`run <NN>`** — Run a specific stage. If earlier stages are incomplete, say which
and get explicit confirmation before proceeding out of order.

**`round`** — Start a new round. Ask what triggered it and which stages it invalidates
(or read them from stage 12's `Affects` fields / stage 00's round-history row). Then:
increment **Current round**, append a **Round history** row, set every invalidated
stage to `superseded`, and report what will be re-run. Does not run anything itself —
`next` does that.

**`invalidate <NN> [<NN>...]`** — Mark specific stages `superseded` within the current
round, without starting a new one. Use when a single stage's inputs changed and the
scope did not. Requires a reason, which goes in the stage's Notes column.

No argument given: report `status`, then offer `next`.

## Running a stage

1. **Prerequisites:** confirm all earlier stages' MANIFEST.md show `Status: complete`.
2. **Delegate:** invoke the child via the Skill tool, passing the absolute target repo
   path as the argument, e.g. `Skill(codeconverter-04-test-baseline, args: "/path/to/target")`.
3. **Interactive stages (01, 06, 07, 10)** run inline in the conversation — the user
   must be present to answer questions. Warn them before starting one.
4. **Verify uniformity when the child returns** (do not skip this):
   - `MANIFEST.md` exists in the stage dir and matches the template.
   - Every artifact listed carries the standard header block with `Status: final`.
   - Exit criteria are all checked, or Status is honestly `in-progress`/`blocked`.
   - No files written outside the stage dir, except: STATE.md, stage 08's in-place
     update of `07-target-codebase/analysis.md`, stage 04's test-repo commits,
     stage 00-source-provenance's target-of-record block in STATE.md, and stage
     10a's pilot code in the target's source tree.
   - **No `codeconverter-verify` record in the MANIFEST has verdict `fail`.** A stage
     reporting `complete` with a `fail` record is malformed; send it back.
   - **Every claim about a foreign codebase carries an evidence line** (templates.md
     §4) whose `Ref` matches the provenance table, and every superseded figure carries
     a correction block (§5).
   - **If this is a re-run in a later round**, the MANIFEST says what changed since the
     previous round and why the re-run was needed.
5. **Record:** update the stage's row in STATE.md — Status, Completed, **and the Round
   column set to Current round**.
6. **Commit:** `git -C <target> add docs/codeconverter && git commit -m "codeconverter NN-name: <short summary>"`.
   One commit per stage — never mix stage outputs in one commit, so a later stage can
   be rolled back cleanly if it contradicts earlier findings.

## Rules

- **No journey/journal artifact.** This pipeline intentionally does not keep a
  narrative journal. Do not create one.
- **MANIFESTs are the source of truth** for stage completion, not STATE.md and not
  memory of the conversation.
- **Uniformity is enforced by the orchestrator.** A stage that returns without a
  conforming MANIFEST gets re-invoked to finish its paperwork before its commit.
- **Interactivity roadmap:** the long-term goal is that only stage 01 is interactive.
  Stages 06, 07, and 10 remain interactive until one full pipeline run has completed;
  after that, ask the user whether each should be converted to fork mode.
- **Restart vs continue:** if a forked stage stalls or dies, re-invoke it — each
  stage's instructions are written to resume from the last committed output in its
  stage dir. Restart from scratch only if output contradicts the stage instructions.
- **A re-run is not a restart.** Re-running a stage in a later round keeps its previous
  output as the baseline and records the delta. Deleting the old output loses the
  comparison, which is often the most useful thing the round produced.
- **Never infer invalidation.** Whoever changed the answer names the stages it affects;
  the orchestrator executes that list and enforces the two consistency rules under
  **Rounds**. Guessing at the blast radius produces both stale output and wasted re-runs.
- **Phase actuals.** Once stage 11's schedule is being executed, write a row into
  STATE.md's `Phase actuals` table at every phase boundary and recompute the remaining
  schedule from the observed ratio. Predicted-versus-actual is the only mechanism in
  this pipeline that would ever notice its estimates were wrong.
