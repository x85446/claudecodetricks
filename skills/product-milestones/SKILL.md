---
name: product-milestones
description: "PRODUCT child (invoked via /product): the build sequence — groups each roadmap release into coherent milestones with runnable acceptance outcomes and scope-derived counts."
version: 1.7.0
---

# /product-milestones — stage 8: the order it gets built in

The roadmap defines release scope. This stage groups that scope into coherent
milestones with runnable acceptance outcomes. The count follows implementation
context, dependencies and risk. A milestone identifies an acceptance checkpoint;
several related checkpoints can share one execution plan and one branch.

**The rule that makes a milestone real:** it is a thin vertical slice, never a
layer. "Set up the database" and "build the API" are not milestones. Nothing
works at the end of either, so neither can be checked. "A learner opens one
bundled tutorial and runs its first command" is a milestone. It is thin, but
it crosses every component it needs, and it either works or it doesn't. This
is the roadmap's rule (cut depth across the whole journey, never breadth in
one part) applied a second time, inside each release.

## Steps

1. **Read** `docs/roadmap.md` (the three slices, each release's end-to-end
   check, and its *Not yet* list), `docs/features.md` (every `F-NN`),
   `docs/requirements.md` (every `R-NN`, its release, and its acceptance
   criteria), `docs/architecture.md` (containers, ADRs, risks) and
   `docs/engineering.md` (the stack, make targets, test tiers, and what v1
   depends on that doesn't exist yet).

2. **Group v1 by coherent implementation context.**
   - **M1-01 is the walking skeleton**: the thinnest path through the v1
     journey that runs end to end on the real stack, on every target OS
     (each on the guest stage 9 proves for it), with one hard-coded document
     and one command. Its job is to prove the containers connect, everywhere the
     product runs, before anything is deepened. No later milestone is one
     OS's port of another's work (`/product` rule 11).
   - **Subsequent milestones deepen the skeleton** by one capability the
     release's end-to-end check needs. Order them by **risk first, then
     dependency**. The thing most likely to force a redesign goes as early as
     its dependencies allow, so architecture risks and unproven external
     dependencies come before polish.
   - **The final milestone closes v1.** After it, the roadmap's v1 end-to-end check passes
     exactly as narrated, and every v1 requirement is met. Release work the
     engineering spec names (packaging, signing, CI gates) lands here if no
     earlier milestone needed it.

3. **Group v2 and v3 by the same criteria**, starting from the release before
   it. M2-01 is the first slice of v2's journey on top of v1, not a second
   skeleton. **v1 is specified in full. v2 and v3 are cut at feature level**:
   the requirements they cite, an exit stated as an outcome, and dependencies.
   Their sequence is re-cut in maintain mode once the release before them
   ships, because detail beyond the committed release turns into fiction.

4. **Assign every requirement.** Each `R-NN` in a release is delivered by
   exactly one milestone of that release. That is the milestone where its
   acceptance criteria first pass and stay passing. A non-functional
   requirement lands where its budget is first measured and enforced, not
   where the code first exists. Every `F-NN` in the release appears in at
   least one of its milestones.

5. **Write the exit criteria** from the assigned requirements' acceptance
   criteria plus one demo, the thing a person does to see the milestone work.
   An exit criterion that is not checkable by a command or an observable
   action is not finished.

6. **Name the risk each milestone retires**: the assumption, ADR or external
   dependency it proves or disproves. A milestone that retires nothing and
   ships nothing user-visible probably doesn't exist. Merge it into its
   neighbor and re-cut.

7. **Audit**, then fill the audit block. Every count comes from the document,
   never from memory.

## Output — `docs/milestones.md`

```markdown
# <Product> — Milestones

**Date:** <YYYY-MM-DD>  ·  **Built from:** roadmap <date>, requirements <date>, engineering <date>
**Milestones:** <total> (v1 <n> · v2 <n> · v3 <n>)  ·  **Requirements assigned:** <n> of <n>

## v1.0 — <the release's one-line "For:">

| # | Milestone | Delivers | Depends on |
|---|---|---|---|
| M1-01 | Walking skeleton — <the thin path> | F-07, F-42 · R-001, R-014 | — |
| M1-02 | <…> | <…> | M1-01 |

### M1-01 — <title>
**Goal:** <one sentence: what works after this that did not before>
**Demo:** <what a person does, and sees, to know it works>
**Delivers:** F-07 (partial), F-42 · R-001, R-014
**Touches:** <containers from architecture.md>
**Retires:** <the risk, ADR or external dependency this proves>
**Depends on:** — <or the internal ids, comma-separated, then any outside dependency marked (external): `M1-04, M1-05; Papyrus spec 1.0 (external)`>
**Exit criteria:**
- [ ] <from R-001's acceptance criteria, as a command or observable check>
- [ ] <…>
- [ ] `make test` green on fast + standard tiers; main shippable

### M1-02 — <title>
…

## v2.0 — <"For:">
<same table, then one block per milestone: Goal · Delivers · Depends on · Exit (as an outcome)>

## v3.0 — <"For:">
<same as v2>

## Audit
- Milestones per release: v1 <n> · v2 <n> · v3 <n>
- Requirements assigned: <n> of <n>. Unassigned: <none | ids — a defect>. Assigned twice: <none | ids — a defect>
- Features covered: <n> of <n> per release. Missing: <none | ids>
- Dependencies point backwards only: <yes | the forward edges — a defect>
- Dependency ids resolve and form no cycle: <yes | the unknown ids or the cycle — a defect>
- Layer-shaped milestones (nothing runnable at the end): <none | ids — re-cut>
- Last milestone of each release passes that release's end-to-end check: <yes | what's missing>

## Open questions
- [NEEDS CLARIFICATION] <question>

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

## Rules

1. **Scope determines count.** Group by coherent implementation context, risk,
   dependencies and runnable acceptance. Justify every split in writing. A
   subsystem shares a plan unless an independent delivery or risk warrants a split.
2. **A vertical slice, never a layer.** Each milestone ends with something
   runnable and a demo. "Schema", "API", "UI" and "tests" are not milestones.
3. **Main stays shippable after every milestone.** Unfinished capability is
   gated or absent, never half-wired into a user-visible path.
4. **Every requirement is assigned exactly once, and every feature appears.**
   Audit both directions. Unassigned requirements are omissions, and
   requirements assigned twice are ambiguity about when something is done.
5. **Dependencies point backwards only**, within a release and across
   releases. A forward edge is a sequencing defect. Each `**Depends on:**`
   line lists internal milestone ids separated by commas, then any outside
   dependency marked `(external)`; `/product-iteratemilestones` copies the
   internal ids onto each plan's `depends:` line, so an id that names no
   milestone, or a cycle, fails the audit. Name only the milestones whose
   work this one needs: two milestones that both need only M1-03 both name
   M1-03, and the conductor then runs them in either order.
6. **Risk first.** What could force a redesign goes as early as its
   dependencies allow.
7. **No dates and no estimates.** Order is this document's axis; scheduling
   is not this family's job. That matches the roadmap's rule.
8. **Never invent scope.** A milestone that needs a feature or requirement
   missing from the earlier documents is a gap in that stage. Name it in the
   analyze block so the user can unlock that stage. Never add it here.
9. **Each milestone is `/ip`-ready, and this stage never writes the plan.**
   The goal, delivers, exit criteria and depends-on lines are what `/ip`
   needs. Handing a milestone to `/ip` is the user's move.

## Grouping audit

Keep lifecycle work that shares implementation context together. Split only for
an independent delivery outcome, a real dependency, a distinct risk, or an
operational boundary. Write a reason for every split, including a subsystem
spread across milestones. Do not create a plan boundary for each acceptance
checkpoint. Assign each requirement and acceptance identifier exactly once.
Audit both directions: every requirement has one owner, and every owned item
exists in the requirements. Small and large scopes can produce different counts.
Fold a closure check with no independently owned requirement into the milestone
that delivers its implementation; an acceptance gate alone supplies no milestone.
Publication remains a distinct release plan under `/iterate-release`.

## Input completeness

Treat stated product names, priority order and output criteria as authoritative.
A minimal output is complete when the requirement defines it; extra fields are
new scope. A release explicitly excluded from the approved roadmap needs no
invented milestone or scope question. Write `None` under Open questions when no
operator-only fact is missing. Milestone count and grouping are planning work,
not operator questions. Record supplied risk priorities directly.
