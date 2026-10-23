---
name: "product-engineering"
description: "PRODUCT child (invoked via $product): the engineering spec — stack and versions, interface contracts, performance budgets, and the build, test and deploy story, specific enough to start work against."
---


# $product-engineering — stage 7: enough to start building

**Version:** product family 1.7.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


The last HOW stage. Where the architecture says *what shape*, this says *what exactly* — named versions, real interface signatures, budgets with numbers, and how the thing gets built, tested and shipped. The test of this document: an engineer who has read only this and the two docs before it can start on Monday without asking what to use.

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$accounts` — ported.
- `$dev-makefiles` — ported.
- `$ip` — ported.
- `$iterate-release` — ported.
- `$izmachine` — ported.
- `$product` — ported.
- `$product-environment` — ported.
- `$product-milestones` — ported.
- `$testmaster` — ported.
- `$testmaster-adopt` — ported.
- `$testmaster-derive` — ported.

## Steps

1. **Read** `docs/architecture.md` (containers, dependencies, ADRs, ranked attributes), `docs/requirements.md` (every non-functional requirement becomes a budget with an owner), and `docs/roadmap.md` (v1 is what gets specified in full).

2. **Pin the stack.** Exact versions, not ranges, and a line on why each. Every entry inherits stage 6's License and Cost ruling — restate the verdict here rather than making the reader go look, and flag anything that drifted.

3. **Specify the interfaces** that cross a container boundary: the real signature, the error cases, and what is versioned. Internal interfaces are the implementer's business; boundaries are not.

4. **Turn every non-functional requirement into a budget** — a number, where it's measured, and what happens when it's exceeded. A budget nobody measures is a wish.

5. **Write the build, test and deploy story**, against this repo's conventions rather than inventing parallel ones:
   - **Build** — every repeatable dev task is a make target. `$dev-makefiles` governs the Makefile; name the targets this product needs, don't hand-roll shell.
   - **Test** — `$testmaster` owns the suite; requirements feed `$testmaster-derive`. Name the tiers (fast ≤10s, standard ≤2min, slow nightly-only) and what belongs in each.
   - **Deploy** — the environments, what promotes between them, and how a release is rolled back.
   - **Machines** — every development, test and staging machine the story needs (a guest per supported OS for the install and integration tiers, a server to run against) is named by need, kind, OS, version and architecture, and lives as a `[machine.<key>]` definition in the project's `.iz` with a `needs` directive, kept by `$izmachine`. izmachine chooses the host; the spec never names one unless the user did. Stage 9 (`$product-environment`) brings each one up and proves the stack on it, so every OS version and architecture the product targets is named here: an unnamed one cannot be proven.
   - **Access** — every third-party account, credential, OAuth client, signing identity and deploy target the stack or the deploy story needs, named by what it is and which requirement uses it, never by its value. Stage 9 proves each one or lists the one operator act that clears it.
   - **Platform permissions** — any OS permission the product uses at runtime (on macOS the TCC set: Full Disk Access, Accessibility, Screen Recording) names the exact component that holds it, and that component ships with a **stable code identity**: a signed `.app` bundle with a fixed bundle id and a Developer ID designated requirement, never a bare, ad-hoc-signed binary. macOS binds a grant to the code requirement, so a stable requirement is what lets one grant outlive every rebuild; an ad-hoc signature changes each build and drops the grant with it. A launchd daemon or agent that needs the permission **is** that bundle, or carries the same requirement, because a background process does not inherit the grant of the terminal that would launch it. Stage 9 bakes each grant into the golden image against that identity.

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

<Per $dev-makefiles: every repeatable task is a target. Complex logic lives in a helper script with a clean target as its entry point, not inline in the Makefile.>

## Test strategy
| Tier | Budget | Contains | When |
|---|---|---|---|
| fast | ≤10s | <…> | every change |
| standard | ≤2min | <…> | pre-merge |
| slow | >2min | <…> | nightly only |

**Requirement coverage:** every v1 `R-NN` maps to at least one case. Derive them with `$testmaster-derive`; `$testmaster-adopt` seeds the catalog if this repo has no suite yet.

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

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

## Rules

1. **Exact versions, never ranges.** "^18" is not a decision; it's deferring one into someone's lockfile.
2. **Every stack entry restates its License and Cost verdict**, and flags drift from stage 6. The standing default holds: free and permissive, and anything else is the user's explicit call. A dependency stage 6 left unruled goes to `$accounts` before it goes to the user: a membership the user already holds is `Cost: held — <program>`, not a question.
3. **Interface specs are real signatures** with their error cases. A description of an endpoint is not an endpoint.
4. **Every budget has a number, a probe and a breach action.** Unmeasured budgets are the most common way a performance requirement dies.
5. **Build work goes through make targets** per `$dev-makefiles` — never ad-hoc shell in the spec.
6. **Every v1 requirement maps to at least one test case.** Report the unmapped ones; that list is the real coverage gap.
7. **Specify v1 in full; sketch v2 and v3 only where they constrain a v1 decision.** Detail beyond the committed release ages into fiction.
8. **Machines are needs, not hosts, and every target OS is tested.** The spec says what each environment needs and leaves placement to `$izmachine`. Every tier that touches the platform (install, integration, UI) runs on a guest of each target OS, from the first milestone. No OS waits for a later release because the team lacks that hardware.
9. **A macOS permission-holder has a stable code identity.** A component that holds an OS permission ships as a signed bundle with a fixed bundle id and a Developer ID designated requirement — never ad-hoc-signed, never a bare CLI binary, and never a daemon leaning on a terminal's inherited grant. The grant binds to the requirement, so only a stable requirement survives a rebuild, and stage 9 bakes it into the golden image against that identity. Signing is specified here; a release plan notarizes and staples, which is `$iterate-release`'s alone.
10. **It does not start building.** Stage 8 (`$product-milestones`) orders the work into milestones; the family ends with documents, and `$ip` turns a milestone into an executable plan when the user decides to.
