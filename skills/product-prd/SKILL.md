---
name: product-prd
description: "PRODUCT child (invoked via /product): the PRD — problem, customer, positioning, non-goals and success signals, written strictly as WHAT and WHY with no technology in it."
version: 1.0.0
---

# /product-prd — stage 2: what we are building and why

The document every later stage is checked against. It answers what the product is for and who decides it succeeded. It does not answer how anything works.

Inherits the context-first pattern from the `prd-generator` skill this forked from: read before asking, and tell the user what you found.

## Steps

1. **Read what already exists** before asking anything: `docs/brief.md` (the verdict, the press release, the non-goals), `docs/competitors.md` (table stakes vs differentiators, where we lose), and any `context/*.md` the project keeps — `product.md`, `personas.md`, `company.md`. In brownfield mode also read the code and `/user-docs` output for what the product does today.

   **Say what you found**, concretely: *"The brief names ops leads as the customer and the landscape says audit trails are table stakes — I'll build on both. Nothing in the repo describes pricing, so I'll ask."* This is what makes the chain feel like it accumulates rather than re-interrogates.

2. **Ask only for what is missing.** Two inputs are load-bearing and must be asked for if absent:
   - What problem are we solving?
   - Who has it — specifically enough to recognize one?

   These four are generated with stated assumptions if absent, never blocked on: evidence the problem is real · why now · how we win against the landscape · what success looks like in numbers.

3. **Write the PRD.** Every claim traces to the brief, the landscape, or a stated assumption.

4. **Fill the contract block last** — five fields, and if any cannot be filled in one or two sentences, the PRD is not done.

## Output — `docs/prd.md`

```markdown
# <Product> — PRD

**Status:** draft | approved  ·  **Date:** <YYYY-MM-DD>  ·  **Brief verdict:** go

## Contract
| Field | |
|---|---|
| **Why** | <the problem, one or two sentences> |
| **Capabilities** | <what it must be able to do, 3–6 bullets, no implementation> |
| **Constraints** | <what bounds the solution: compliance, cost, platform, license> |
| **Non-goals** | <what this deliberately is not — the most valuable row here> |
| **Success signal** | <the observable that tells us it worked, with a number> |

## Problem
<The customer's world today. What it costs them. What they do instead now.>

**Evidence:** <research, data, feedback — or the assumption, marked as one.>
**Why now:** <what changed that makes this the moment.>

## Customer
### <Persona name> — <role>
**Context:** <where they sit, what their day looks like>
**Pain:** <the specific moment this product intervenes in>
**Today:** <their current workaround and why it fails>
**Success for them:** <what they'd notice getting better>

## Positioning
**For** <customer> **who** <problem>, **this is a** <category> **that** <key benefit>, **unlike** <the alternative from docs/competitors.md>, **it** <the differentiator>.

**Table stakes we must meet:** <from the landscape — non-negotiable to be considered>
**Where we win:** <the differentiators, each tied to a landscape finding>
**Where we will lose, knowingly:** <honest, and fine — it defines the non-goals>

## Success metrics
| Metric | Baseline | Target | How measured |
|---|---|---|---|

## Non-goals
<Explicit. Each with one line on why not — "not now" and "not ever" are different answers.>

## Risks
| Risk | Impact | What would tell us early |
|---|---|---|

## Open questions
- [NEEDS CLARIFICATION] <question>

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

## Rules

1. **No technology. At all.** No stack, no database, no framework, no API shape, no library. A named technology here is a decision that skipped the architecture stage. Say "must work offline", never "must use SQLite".
2. **No feature list.** Capabilities are what the product must be able to do; features are stage 3's inventory. If you're writing bullets that look like a backlog, stop.
3. **Non-goals are mandatory and specific.** "We won't boil the ocean" is not a non-goal. "No mobile client in any release through v3" is.
4. **Every success metric needs a number and a measurement method.** A metric nobody can compute is decoration.
5. **Positioning must name a real alternative** from `docs/competitors.md` — not "legacy solutions", not "doing nothing" unless doing nothing is genuinely the competition.
6. **Read before asking, and say what you read.** Re-asking what the brief already settled is the failure this stage most easily falls into.
7. **The contract block fills last and must fill completely.** An unfillable field means an unfinished PRD, not an optional row.
