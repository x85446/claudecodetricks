---
name: product-roadmap
description: "PRODUCT child (invoked via /product): the staged roadmap — draws release lines across the feature map to fix v1/MVP, v2 and v3, each slice a journey usable end to end on its own."
version: 1.0.0
---

# /product-roadmap — stage 4: what ships when

Release lines drawn horizontally across the story map from stage 3. This is where the MVP stops being an opinion: it is the top slice, and it is defensible because every feature in it traces to a step in the user's journey.

**The rule that makes a slice real:** a release slice must deliver a complete, useful outcome on its own — the user can get from the start of the journey to a result. A slice that implements half of one activity beautifully is not a release, it is an unshippable fragment. Cut *depth* across the whole journey, never *breadth* within one part of it.

## Steps

1. **Read** `docs/features.md` (the backbone and every `F-NN` with its priority) and `docs/prd.md` (success signal, non-goals, constraints). The success signal decides the MVP more than any priority column does: **the MVP is the smallest slice that can move it.**

2. **Draw the v1/MVP line.** Walk the backbone left to right and take the thinnest viable feature at each step — the walking skeleton. Then test it: can a real user complete the journey end to end using only what is above the line? If not, the line is wrong, not the user.

3. **Draw v2 and v3.** v2 deepens what v1 proved and adds the Shoulds that hurt most to omit. v3 is the horizon — where the product is going, stated clearly enough to steer architecture at stage 6 without pretending it's committed.

4. **Place every feature.** Every `F-NN` lands in exactly one of v1 / v2 / v3 / Won't. An unplaced feature is an unmade decision; a feature in two slices is a bug.

5. **State what each release is for** in one sentence, and what it does *not* yet do. The second half is what keeps the next slice honest.

## Output — `docs/roadmap.md`

```markdown
# <Product> — Roadmap

**Date:** <YYYY-MM-DD>  ·  **Horizon:** v3.0  ·  **Features placed:** <n> of <n>

## v1.0 — MVP · *Now*
**For:** <the one outcome this release delivers>
**Moves the needle on:** <the PRD success signal, and how>
**Not yet:** <what a user will notice is missing, named honestly>

| Step | Features |
|---|---|
| Discover → <step> | F-01, F-04 |
| Set up → <step> | F-07 |

**End-to-end check:** <the journey a user completes with only v1, narrated in one or two sentences.>

## v2.0 — *Next*
**For:** <outcome>
**Depends on v1 having proved:** <the assumption v1 tests — if it fails, v2 changes>
**Not yet:** <…>

| Step | Features |
|---|---|

## v3.0 — *Later*
**For:** <outcome>
**Direction this sets:** <what stage 6 must not preclude>
**Not yet:** <…>

| Step | Features |
|---|---|

## Won't build
| id | Feature | Why not |
|---|---|---|
| F-22 | <name> | traces to PRD non-goal: <which> |

## Placement audit
- Features placed: <n> of <n>
- Unplaced: <none | the list — an unmade decision>
- In more than one slice: <none | the list — a defect>
- Must-priority features below the v1 line: <list, each with why it can wait>

## Open questions
- [NEEDS CLARIFICATION] <question>

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

## Rules

1. **Each slice is usable end to end.** The end-to-end check is written out, not asserted. If you cannot narrate the user's complete journey on that slice alone, the line is in the wrong place.
2. **Slice thin across the whole backbone, never deep into one part of it.** This is the entire technique.
3. **Every `F-NN` is placed exactly once.** Report unplaced and double-placed features as defects, not as notes.
4. **The MVP is the smallest slice that can move the PRD's success signal.** Not the smallest buildable thing, and not everything marked Must.
5. **A Must feature below the v1 line needs a reason** stated in the audit. That's allowed — "must eventually" and "must at launch" are different claims — but it must be said out loud.
6. **No dates and no estimates.** Now/Next/Later is the axis. Ordering is a product decision; scheduling is not this family's job.
7. **v3 steers stage 6 without committing anyone.** Its purpose is to stop the architecture from painting v3 into a corner.
8. **Never invent a feature here.** If a slice needs something absent from the inventory, that is a stage-3 gap — go back and add it there with an id.
