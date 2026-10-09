---
name: product-milestones
description: "PRODUCT child (invoked via /product): the build sequence — cuts each roadmap release into exactly 10 ordered milestones (M1-01…M3-10), each a shippable slice sized for one /ip plan."
version: 1.2.1
---

# /product-milestones — stage 8: the order it gets built in

The roadmap says what each release contains. The engineering spec says what
it's built with. This stage says **in what order**: exactly ten milestones to
reach v1, ten more to reach v2, and ten more to reach v3. Each milestone ends
with something a person can run and see, and leaves main shippable. Each is
sized so that `/ip` can turn it into one plan.

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

2. **Cut v1 into ten.**
   - **M1-01 is the walking skeleton**: the thinnest path through the v1
     journey that runs end to end on the real stack. That might be one
     platform, one hard-coded document, and one command. Its job is to prove
     the containers connect before anything is deepened.
   - **M1-02 to M1-09 each deepen the skeleton** by one capability the
     release's end-to-end check needs. Order them by **risk first, then
     dependency**. The thing most likely to force a redesign goes as early as
     its dependencies allow, so architecture risks and unproven external
     dependencies come before polish.
   - **M1-10 closes v1.** After it, the roadmap's v1 end-to-end check passes
     exactly as narrated, and every v1 requirement is met. Release work the
     engineering spec names (packaging, signing, CI gates) lands here if no
     earlier milestone needed it.

3. **Cut v2 and v3 the same way**, ten each, starting from the release before
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
**Milestones:** 30 (v1 10 · v2 10 · v3 10)  ·  **Requirements assigned:** <n> of <n>

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
**Depends on:** —
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
- Milestones per release: v1 10 · v2 10 · v3 10
- Requirements assigned: <n> of <n>. Unassigned: <none | ids — a defect>. Assigned twice: <none | ids — a defect>
- Features covered: <n> of <n> per release. Missing: <none | ids>
- Dependencies point backwards only: <yes | the forward edges — a defect>
- Layer-shaped milestones (nothing runnable at the end): <none | ids — re-cut>
- Last milestone of each release passes that release's end-to-end check: <yes | what's missing>

## Open questions
- [NEEDS CLARIFICATION] <question>

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

## Rules

1. **Exactly ten per release.** Not nine because a release is small, and not
   twelve because it is large. The fixed count is what keeps milestones the
   same size across releases and products. A release too small for ten slices
   is cut thinner, and one too big for ten makes each slice wider. Either way,
   say so in the release's intro line.
2. **A vertical slice, never a layer.** Each milestone ends with something
   runnable and a demo. "Schema", "API", "UI" and "tests" are not milestones.
3. **Main stays shippable after every milestone.** Unfinished capability is
   gated or absent, never half-wired into a user-visible path.
4. **Every requirement is assigned exactly once, and every feature appears.**
   Audit both directions. Unassigned requirements are omissions, and
   requirements assigned twice are ambiguity about when something is done.
5. **Dependencies point backwards only**, within a release and across
   releases. A forward edge is a sequencing defect.
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
