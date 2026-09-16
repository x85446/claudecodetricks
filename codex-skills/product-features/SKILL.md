---
name: "product-features"
description: "PRODUCT child (invoked via $product): the feature inventory — a Jeff Patton story-map backbone of activities and steps, every feature under it as a numbered F-NN with a MoSCoW priority."
---


# $product-features — stage 3: everything the product could do

**Version:** product family 1.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


A story map, not a list. The structure is the work: features hung under the user's actual journey can be sliced into releases that each make sense, and a flat backlog cannot. Stage 4 draws the release lines across what this stage builds.

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$product` — ported.

## Steps

1. **Read** `docs/prd.md` (capabilities, non-goals, personas) and `docs/competitors.md` (table stakes must all appear as features; `roadmap-gap` items too). Brownfield: inventory what the code already does first and mark those features `built`.

2. **Build the backbone — horizontally, in the order the user lives it.** 3–7 **activities** left to right in chronological sequence, each decomposed into 3–7 observable **steps**. The backbone is user behavior, never system structure: "Get paid" is an activity, "Billing service" is not.

3. **Hang features under each step, prioritized vertically** — must-have at the top, future at the bottom. Every feature gets `F-NN` (never renumber; retired ids stay retired) and a MoSCoW priority.

4. **Sweep for gaps** in three passes, which is where a map earns its cost over a list:
   - **Missing steps** — a step nobody named because it's boring, and the journey breaks without it (signup, recovery, cancellation, the empty state, the error).
   - **Table stakes** — everything the landscape marked as required to be considered.
   - **Uncovered pain** — each persona's pain from the PRD must be addressed by at least one feature, or it's an unserved promise.

5. **Report the counts** by activity and by MoSCoW, and name anything from the PRD's capabilities with no feature under it.

## Output — `docs/features.md`

```markdown
# <Product> — Feature Inventory

**Date:** <YYYY-MM-DD>  ·  **Features:** <n>  ·  **Activities:** <n>

## Backbone
<The journey in one line, left to right:>
`Discover → Set up → <Activity> → <Activity> → Maintain`

## <A1> Discover
*The user is <what they're doing and why>.*

### <A1.S1> <Step name>
| id | Feature | Priority | Detail | Source |
|---|---|---|---|---|
| F-01 | <name> | Must | <what it does, one line> | prd:capability-2 |
| F-02 | <name> | Should | <…> | competitors:table-stakes |
| F-03 | <name> | Could | <…> | — |

### <A1.S2> <Step name>
…

## <A2> Set up
…

## Coverage
| Persona pain (from PRD) | Addressed by |
|---|---|
| <pain> | F-04, F-11 |

| Table stake (from landscape) | Addressed by |
|---|---|
| <capability> | F-07 |

## Counts
| Activity | Must | Should | Could | Won't | Total |
|---|---|---|---|---|---|

## Gaps found
<What the sweep surfaced that nobody had named.>

## Open questions
- [NEEDS CLARIFICATION] <question>
```

Brownfield adds a `Status` column: `built` · `partial` · `none`.

## MoSCoW, meaning what it says

| Priority | Meaning |
|---|---|
| **Must** | The product is not the product without it. If it slips, the release slips. |
| **Should** | Important, painful to omit, but the product still works without it. |
| **Could** | Genuinely nice. First thing cut under pressure, with no argument. |
| **Won't** | Decided against for the horizon this family covers. Traces to a PRD non-goal. |

`Must` is the priority that gets abused. If more than roughly half the inventory is Must, the prioritization hasn't happened yet — say so rather than recording it.

## Rules

1. **The backbone is user activity, in chronological order.** Not modules, not teams, not screens. A backbone that mirrors the system architecture has stopped being a story map.
2. **Every feature gets a stable `F-NN`.** Stage 5 traces requirements to these ids and stage 4 slices on them. Never renumber; retired ids are never reused.
3. **No implementation.** "Export to CSV" is a feature; "CSV writer with streaming encoder" is stage 7's problem.
4. **Every feature cites its source** — a PRD capability, a landscape finding, or a persona pain. A feature nobody asked for is a finding: name it and ask.
5. **Table stakes are all Must**, by definition. If one isn't, either the landscape was wrong or we've chosen not to compete — and that belongs in the PRD's non-goals, not quietly in this table.
6. **Do not draw release lines here.** Vertical priority is this stage; horizontal release slices are stage 4.
7. **Report an over-full Must column** rather than recording it silently.
