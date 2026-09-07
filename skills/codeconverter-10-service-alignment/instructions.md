<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase09-service-alignment.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 10-service-alignment**. When the text below says "Phase N",
> translate with this table:
>
> | Legacy phase | codeconverter stage |
> |---|---|
> | (service profile interview) | 01-service-profile |
> | Phase 1 | 02-codebase-analysis |
> | Phase 2 | 03-dependency-discovery |
> | Phase 3 | 04-test-baseline |
> | Phase 4 | 05-api-surface |
> | Phase 5 | 06-domain-analysis |
> | Phase 6 | 07-target-codebase |
> | Phase 7 | 08-gap-validation |
> | Phase 8 (bad actors) | 09-dependency-audit |
> | Phase 9 | 10-service-alignment |
> | Phase 10 (also titled "Phase 8 — Migration Plan") | 11-migration-plan |
>
> All file paths in this document have been rewritten to the
> `docs/codeconverter/` layout. There is no journey.md/journal in this pipeline —
> ignore any journaling instructions. Where this document conflicts with the
> stage's SKILL.md output contract (uniform headers, MANIFEST.md, output
> directory), **SKILL.md wins**.

---

# Phase 9 — Service Alignment

> **Prerequisite:** Phases 1–8 must all be complete.
> Required inputs: `docs/codeconverter/05-api-surface/API.md`,
> `docs/codeconverter/06-domain-analysis/*.md` (all domain analysis documents),
> `docs/codeconverter/07-target-codebase/stack.md`, `docs/codeconverter/07-target-codebase/analysis.md`,
> `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md`,
> and the target codebase (location from `docs/codeconverter/07-target-codebase/stack.md`).

---

## Mission

Determine whether the replacement service should absorb **all** features of the
source service, or whether some features should be **split out** into separate
services. This phase is only relevant when the replacement is an **existing
codebase** being extended — not a greenfield rewrite.

The source service may be a monolith that accumulated features over years. Just
because they all lived in one binary doesn't mean they should continue to. The
replacement is an opportunity to draw better boundaries — if the human wants to.

**The key insight:** API consumers do not care about internal service boundaries.
As long as `GET /v3/{resource}/{id}` returns the right response, they don't care
whether it's served by one binary or five. The question this phase answers is:
what belongs in the replacement, what doesn't, and how do you route traffic so
consumers see no difference?

---

## Step 0 — Confirm Readiness

Before starting, verify:

```bash
# Domain analysis documents exist
ls docs/codeconverter/06-domain-analysis/

# API surface documented
wc -l docs/codeconverter/05-api-surface/API.md

# Gap analysis exists
cat docs/codeconverter/07-target-codebase/analysis.md | head -20

# Bad actors analysis exists
cat docs/codeconverter/09-dependency-audit/bad-actors-analysis.md | head -20

# Target codebase accessible
cat docs/codeconverter/07-target-codebase/stack.md
```

If any prerequisite is missing, stop and complete it first.

---

## Step 1 — Build the Feature Domain Inventory

From the Phase 5 domain analysis documents (`docs/codeconverter/06-domain-analysis/*.md`) and
the Phase 4 API surface (`docs/codeconverter/05-api-surface/API.md`), build a complete
inventory of every feature domain the source service implements.

For each domain, document:

| Field | Description |
|-------|-------------|
| Domain name | Short name (e.g., "Account management", "Policy engine", "Federation") |
| Endpoint count | How many API endpoints belong to this domain (from API.md) |
| Audience | Who calls these endpoints: external customers, internal services, admin, or a mix |
| Data ownership | What tables/stores does this domain own exclusively |
| Domain complexity | LOW (CRUD), MEDIUM (business rules), HIGH (specialized protocol or engine) |
| Coupling to other domains | Which other domains does this one depend on or call |

Present the inventory as a table and wait for human confirmation before proceeding.

---

## Step 2 — Map Domains to the Target Codebase

Read the target codebase (from `docs/codeconverter/07-target-codebase/stack.md`) and the gap
analysis (`docs/codeconverter/07-target-codebase/analysis.md`). For each domain from Step 1,
assess:

| Field | Description |
|-------|-------------|
| Target coverage | Does the target codebase already implement this domain? (Full / Partial / None) |
| Natural fit | Does this domain fit naturally into the target's architecture? (Yes / Forced / No) |
| Implementation effort | Relative effort to add this domain to the target (S / M / L / XL) |
| Risk if absorbed | What could go wrong if this domain is forced into the target |
| Risk if split | What could go wrong if this domain runs as a separate service |

**"Natural fit" criteria:**
- **Yes** — the target was designed for this kind of work (e.g., the target is an
  auth service and the domain is authentication)
- **Forced** — the domain could be added but doesn't match the target's design
  philosophy (e.g., adding a policy engine to a service designed for user management)
- **No** — the domain is fundamentally incompatible with the target (e.g., adding
  a SAML2 IdP to a service that has no web SSO concept)

Present the mapping table and wait for human confirmation.

---

## Step 3 — Present Alignment Options

Based on Steps 1 and 2, present the human with concrete alignment options. Every
option must preserve the same API surface — consumers must not notice the change.

### Option A — Full Absorption

All source service domains are absorbed into the target codebase. One binary
replaces one binary.

**When this makes sense:**
- The target was purpose-built to replace the source
- All domains have "Natural fit = Yes"
- The team wants operational simplicity (one service to deploy, monitor, debug)

**When this is dangerous:**
- Domains with "Natural fit = Forced" or "No" will bloat the target
- The replacement inherits the monolith problem the source already has
- Implementation effort is XL for domains that don't fit

### Option B — Split by Consumer Audience

External-facing endpoints go to the target. Internal-only, admin-only, or
specialized-protocol endpoints go to a separate service (or stay in the source
service running alongside during transition).

**Routing strategy:** The API gateway or ingress controller routes by path prefix.
Consumers call the same URLs — the gateway decides which backend serves them.

**When this makes sense:**
- The target handles the core product API well but shouldn't own operational/admin
  concerns
- Internal APIs have different scaling, security, or deployment requirements

### Option C — Split by Feature Domain

Each domain (or cluster of related domains) becomes a separate service. The source
service is decomposed, not just replaced.

**Routing strategy:** Same as Option B but with more backends. The gateway routes
by path prefix to the appropriate service.

**When this makes sense:**
- The source service is genuinely several services mashed into one binary
- Different domains have different natural owners (different teams, different
  tech stacks)
- The organization wants microservice boundaries

**When this is dangerous:**
- Splitting creates operational complexity (more services to deploy and monitor)
- Cross-domain transactions become distributed transactions
- Domains with tight coupling (e.g., accounts + policies) are hard to split

### Option D — Hybrid

Some domains go to the target, some go to a new standalone service, some stay in
the source service during a transition period.

**When this makes sense:**
- Most domains fit the target but one or two don't
- The team wants to migrate incrementally (move domains one at a time)

For each option, present:
1. Which domains go where
2. How the API gateway routes traffic
3. What the deployment topology looks like
4. The operational cost (number of services to maintain)
5. The migration risk (what could go wrong during transition)

**Wait for the human to choose an option (or propose a variant) before proceeding.**

---

## Step 4 — Define the Alignment Decision

Based on the human's choice, produce the alignment decision document:

### For each domain, record:

| Field | Description |
|-------|-------------|
| Domain | The feature domain name |
| Destination | Which service this domain goes to (target, new service, source during transition) |
| Endpoint list | All API endpoints from this domain (from API.md) |
| Routing rule | How the gateway routes these endpoints to the destination |
| Data ownership transfer | Whether database tables move with the domain |
| Timeline | When this domain migrates (Phase 1 of migration, Phase 2, etc.) |

### Routing strategy:

Document how the API gateway (or ingress controller, or service mesh) routes
traffic so that consumers see a single API surface regardless of how many backends
serve it.

```
Example:
  /v3/accounts/*     → target-service:8080
  /v3/users/*        → target-service:8080
  /v3/policies/*     → policy-service:9186
  /internal/v2/*     → policy-service:9186
  /auth/login        → target-service:8080
  /v3/identity-providers/* → federation-service:8090  (new, or deferred)
```

The routing table must cover 100% of the endpoints in `API.md`. No endpoint may
be unrouted.

---

## Step 5 — Assess Impact on Bad Actors

Read `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md`. For each bad actor finding:

- Does the alignment decision change the solution?
- If a domain is split out, do the bad actor's consumers now need to talk to a
  different service?
- Are there new bad actors created by the split (e.g., if domain A and domain B
  are split, and domain A calls domain B internally, that internal call is now a
  cross-service dependency)?

Document any changes to the bad actors analysis as an addendum.

---

## Step 6 — Document Constraints and Trade-offs

For the chosen alignment:

### Hard constraints (non-negotiable)
- List any domain that **must** stay together (e.g., accounts and policies if
  policy evaluation requires real-time account data)
- List any domain that **must** be split (e.g., regulatory requirement for
  separation of concerns)

### Trade-offs the human accepted
- For each "Forced fit" domain absorbed into the target: document what was
  sacrificed (architectural purity, maintainability, etc.) and why the human
  accepted it
- For each split: document the operational cost accepted

### Implications for migration planning
- How does the alignment decision affect the implementation order?
- If domains migrate incrementally, what is the recommended order?
- What is the minimum viable cutover (which domains must go first)?

---

## Deliverables

When Phase 9 is complete, produce:

1. `docs/codeconverter/10-service-alignment/alignment-decision.md` — the full alignment document:
   - Feature domain inventory (Step 1)
   - Target codebase mapping (Step 2)
   - Chosen alignment option with rationale (Steps 3–4)
   - Routing table covering 100% of API endpoints (Step 4)
   - Bad actors impact assessment (Step 5)
   - Constraints and trade-offs (Step 6)
   - Recommended migration order

---

## Exit Criteria

Phase 9 is complete when:

- [ ] All feature domains from Phase 5 are inventoried
- [ ] Each domain is mapped to the target codebase (fit assessment)
- [ ] Alignment options have been presented to the human
- [ ] Human has chosen an alignment option
- [ ] Every API endpoint from Phase 4 is assigned to a destination service
- [ ] A routing table exists covering 100% of endpoints
- [ ] Bad actors impact has been assessed against the alignment decision
- [ ] Constraints and trade-offs are documented
- [ ] `docs/codeconverter/10-service-alignment/alignment-decision.md` is committed
- [ ] Human has confirmed the alignment decision
