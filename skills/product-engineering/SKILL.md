---
name: product-engineering
description: "PRODUCT child (invoked via /product): the engineering spec — stack and versions, interface contracts, performance budgets, and the build, test and deploy story, specific enough to start work against."
version: 1.0.0
---

# /product-engineering — stage 7: enough to start building

The last stage. Where the architecture says *what shape*, this says *what exactly* — named versions, real interface signatures, budgets with numbers, and how the thing gets built, tested and shipped. The test of this document: an engineer who has read only this and the two docs before it can start on Monday without asking what to use.

## Steps

1. **Read** `docs/architecture.md` (containers, dependencies, ADRs, ranked attributes), `docs/requirements.md` (every non-functional requirement becomes a budget with an owner), and `docs/roadmap.md` (v1 is what gets specified in full).

2. **Pin the stack.** Exact versions, not ranges, and a line on why each. Every entry inherits stage 6's License and Cost ruling — restate the verdict here rather than making the reader go look, and flag anything that drifted.

3. **Specify the interfaces** that cross a container boundary: the real signature, the error cases, and what is versioned. Internal interfaces are the implementer's business; boundaries are not.

4. **Turn every non-functional requirement into a budget** — a number, where it's measured, and what happens when it's exceeded. A budget nobody measures is a wish.

5. **Write the build, test and deploy story**, against this repo's conventions rather than inventing parallel ones:
   - **Build** — every repeatable dev task is a make target. `/dev-makefiles` governs the Makefile; name the targets this product needs, don't hand-roll shell.
   - **Test** — `/testmaster` owns the suite; requirements feed `/testmaster-derive`. Name the tiers (fast ≤10s, standard ≤2min, slow nightly-only) and what belongs in each.
   - **Deploy** — the environments, what promotes between them, and how a release is rolled back.

6. **State the observability contract** concretely enough to be implemented: what is logged at what level, what metrics exist with what labels, what is traced, and what an on-call person does at 3am with only this.

## Output — `docs/engineering.md`

```markdown
# <Product> — Engineering Spec

**Date:** <YYYY-MM-DD>  ·  **Specified through:** v1.0

## Stack
| Layer | Choice | Version | License | Cost | Why |
|---|---|---|---|---|---|
| Language | <…> | <exact> | <…> | free | <ADR-00N> |
| Runtime | | | | | |
| Storage | | | | | |
| Build | | | | | |

<Anything whose License or Cost verdict changed since stage 6, named explicitly.>

## Interfaces
### <Container A> → <Container B>
**Transport:** <protocol>  ·  **Versioned by:** <how>

<The actual signature — endpoint, message shape, or function contract.>

**Errors:** <every case a caller must handle, and what it should do>
**Compatibility:** <what may change without a version bump, and what may not>

## Data
<Schema or model, keyed and typed. Migration approach. What is soft-deleted vs hard-deleted, and retention in real units.>

## Budgets
| Requirement | Budget | Measured where | On breach |
|---|---|---|---|
| R-40 | p95 < 500ms @ 100k items | <the probe> | <alert / fail the build / degrade> |

## Build
| Target | Does |
|---|---|
| `make build` | <…> |
| `make test` | <…> |
| `make check` | <…> |

<Per /dev-makefiles: every repeatable task is a target. Complex logic lives in a helper script with a clean target as its entry point, not inline in the Makefile.>

## Test strategy
| Tier | Budget | Contains | When |
|---|---|---|---|
| fast | ≤10s | <…> | every change |
| standard | ≤2min | <…> | pre-merge |
| slow | >2min | <…> | nightly only |

**Requirement coverage:** every v1 `R-NN` maps to at least one case. Derive them with `/testmaster-derive`; `/testmaster-adopt` seeds the catalog if this repo has no suite yet.

## Deploy
| Environment | Purpose | Promotes from | Rollback |
|---|---|---|---|

## Observability
**Logs:** <levels, structure, correlation id>
**Metrics:** <names, types, labels>
**Traces:** <span boundaries>
**On-call:** <the first three things to check, in order>

## Open questions
- [NEEDS CLARIFICATION] <question>
```

## Rules

1. **Exact versions, never ranges.** "^18" is not a decision; it's deferring one into someone's lockfile.
2. **Every stack entry restates its License and Cost verdict**, and flags drift from stage 6. The standing default holds: free and permissive, and anything else is the user's explicit call.
3. **Interface specs are real signatures** with their error cases. A description of an endpoint is not an endpoint.
4. **Every budget has a number, a probe and a breach action.** Unmeasured budgets are the most common way a performance requirement dies.
5. **Build work goes through make targets** per `/dev-makefiles` — never ad-hoc shell in the spec.
6. **Every v1 requirement maps to at least one test case.** Report the unmapped ones; that list is the real coverage gap.
7. **Specify v1 in full; sketch v2 and v3 only where they constrain a v1 decision.** Detail beyond the committed release ages into fiction.
8. **This is the last stage. It does not start building.** The family ends with documents; `/ip` turns them into an executable plan when the user decides to.
