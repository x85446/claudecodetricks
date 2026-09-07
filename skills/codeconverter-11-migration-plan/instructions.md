<!-- ADAPTED for the codeconverter pipeline from codeplanner/phasePause-migration.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 11-migration-plan**. When the text below says "Phase N",
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

# Phase 8 — Migration Plan

> **Read before starting:** Phases 1–7 must all be complete before this phase begins.
> Required inputs: `docs/codeconverter/07-target-codebase/stack.md`, `docs/codeconverter/07-target-codebase/analysis.md`,
> `docs/codeconverter/02-codebase-analysis/`, `docs/codeconverter/03-dependency-discovery/references.md`, `docs/codeconverter/04-test-baseline/tests.md`,
> `docs/codeconverter/05-api-surface/API.md`, and all domain analysis documents in
> `docs/codeconverter/06-domain-analysis/`.

---

## Mission

Produce a concrete, ordered implementation plan for rebuilding this service in the target
language and framework (from `docs/codeconverter/07-target-codebase/stack.md`). The plan must be
specific enough that any engineer can pick it up and execute without referring to the
source service's code.

You are **not rewriting** the source service. You are **replacing** it with a behavioral
equivalent that passes the same test suite, honors the same API contracts, and takes
advantage of the target framework's native capabilities where they are stronger than the
original implementation.

The analogy: you are swapping a **red 4×2 Lego with a blue 4×2 Lego**. Same connectors.
Same shape. Different material. The tests don't care about the material.

---

## Step 0 — Confirm Readiness

Before writing any migration plan, verify:

```bash
# Phase01 output exists
ls docs/codeconverter/02-codebase-analysis/

# References documented
grep -c "^|" docs/codeconverter/03-dependency-discovery/references.md

# All tests passing
grep "100% pass\|pass rate" docs/codeconverter/04-test-baseline/tests.md

# API surface documented
wc -l docs/codeconverter/05-api-surface/API.md

# Domain analysis complete
ls docs/codeconverter/06-domain-analysis/
```

If any of the above is missing or incomplete, stop and complete the prerequisite phase first.
Document what you found in a "Readiness Check" section of your output.

---

## Step 1 — Behavioral Inventory

Consolidate the phase01–05 outputs into a single behavioral inventory. This is the
"what the replacement must do" document. Do not start implementation planning until
this exists.

### 1a — API Contract Table

From `docs/codeconverter/05-api-surface/API.md`, produce a consolidated table:

| Priority | Path | Method | Auth | Category | Go handler module (proposed) |
|----------|------|--------|------|----------|-------------------------------|

Priority is: 1 = external/customer-facing, 2 = admin, 3 = aggregator, 4 = internal cross-service,
5 = message bus / async.

Assign each endpoint to a proposed Go handler module (one module per domain:
`auth`, `accounts`, `users`, `apikeys`, `groups`, `policies`, `certificates`,
`applications`, `idps`, `agreements`, `branding`, `notifications`).

### 1b — State That Must Be Preserved

From the phase01 storage matrix and domain analysis:

| Table / Store | Owned by | Schema critical points | Migration strategy |
|---------------|----------|------------------------|-------------------|

Migration strategies:
- **Shared**: both Java and Go read/write the same table — schema must not change
- **Shadow**: Go writes a parallel table, switch on feature flag
- **Migrate**: Go takes ownership after cutover, one-time migration script

### 1c — Behavioral Invariants

From domain analysis documents, list every invariant the replacement must preserve.
Format each as a testable assertion:

```
GIVEN account status = SUSPENDED
WHEN any API call arrives with a token from that account
THEN policy evaluation returns FALSE (system policy: restricted_account)
```

Target: at least 30 invariants covering RBAC, policy evaluation, session management,
multi-tenancy isolation, and token lifecycle.

---

## Step 2 — Target Framework Capability Mapping

For each domain in the gap analysis (`docs/codeconverter/06-domain-analysis/GAP_ANALYSIS.md`),
map the source implementation to the target framework equivalent. Read
`docs/codeconverter/07-target-codebase/stack.md` to determine the target framework.

| Domain | Source implementation | Target framework native | Delta to implement | Risk |
|--------|-----------------------|-------------------------|--------------------|------|
| RBAC | {source class, e.g. IdentityUtils.java} | {target native capability} | Multi-tenancy isolation, group membership cascade | Medium |
| Policy engine | {source module, e.g. iam-policies-engine} | {target policy engine} | Port evaluation cascade logic; translate policy DB schema | High |
| OIDC | {source classes} | {target OIDC support} | Claims mapping, session binding | Medium |
| SAML2 | {source classes} | {target SAML2 support} | Attribute mapping, IdP-initiated flow | Medium |
| MFA | {source module} | {target MFA support} | Enforcement policy, reauthentication rules | Low |
| JWT | {source library} | {target JWT middleware} | Legacy reference token format | High |
| Multi-tenancy | Account isolation checks | Custom middleware | Root → aggregator → sub-account hierarchy | High |

Expand this table from the gap analysis. For every "High" risk item, produce a spike
plan (1–3 paragraphs on what to prototype first and why).

---

## Step 3 — Implementation Order

Produce a phased implementation schedule. Each phase must be runnable as an independent
increment — the service must be able to start after each phase, even if not all
endpoints are implemented.

### Recommended phase order (adjust based on your context):

**Phase 8.1 — Skeleton and infrastructure**
- Project bootstrap (target framework), config, logging, metrics
- Database migrations (Postgres — identical schema to Java)
- Health check endpoints
- Authentication middleware (JWT validation, API key lookup, reference token resolution)
- Exit criteria: service starts, health check returns 200, JWT middleware validates tokens

**Phase 8.2 — Core auth flows**
- Login / logout / token refresh (iam-access equivalent)
- Session management (Redis sorted set, 5-session limit, Lua atomic)
- API key authentication (PBKDF2 verification, account isolation)
- Exit criteria: `pelion-system-tests` login tests pass against new service

**Phase 8.3 — Account and user management**
- All `/v3/accounts/*` and `/v3/users/*` endpoints (external tier)
- Multi-tenancy isolation checks
- Group CRUD, group membership
- Default group creation on account creation
- Exit criteria: core CRUD tests in `test-flow/accounts/` and `test-flow/users/` pass

**Phase 8.4 — API keys, applications, certificates**
- All `/v3/api-keys/*` endpoints
- Application and access key management
- Certificate management (trusted certs, developer certs)
- Exit criteria: `ManageApikeysTest`, `ManageApplicationsTest`, `TrustedCertificateTest` pass

**Phase 8.5 — Policy engine (high risk)**
- OPA integration (or equivalent policy engine in target framework)
- Port 4-tier cascade logic to Rego policies
- Policy CRUD endpoints
- Policy evaluation endpoint (`/internal/v2/authorize`)
- Exit criteria: all `test-flow/policies/*` pass; policy evaluation metrics populate

**Phase 8.6 — Admin and aggregator surface**
- `/admin/v3/*` endpoints (root-only)
- `/v3/accounts/{id}/*` aggregator endpoints
- Root-only field enforcement (tier, status, limits)
- Exit criteria: `RootAccountTest`, `AggregatorAccountTest` pass

**Phase 8.7 — Identity providers, agreements, branding**
- OIDC/SAML2 IdP management endpoints
- Agreements and signed agreements
- Branding colors and images
- User invitations
- Exit criteria: `IdentityProviderManagementTest`, `AgreementsTest`, `UserInvitationTest` pass

**Phase 8.8 — Hardening and full test suite**
- Run all test suites against the Go implementation
- Fix failures
- Performance baseline (compare against Java using NFT test plans from `mbed-nft-systemtest`)
- Exit criteria: identical pass rate to Java baseline in `docs/codeconverter/04-test-baseline/tests.md`

---

## Step 4 — Test Strategy

The test suite from Phase 3 is the acceptance gate. Document:

### Regression test matrix

| Suite | Run against Java? | Run against Go? | How to point at Go |
|-------|:-----------------:|:---------------:|--------------------|
| Unit tests (`test-flow`, `test-integration`) | YES (baseline) | YES (acceptance) | `env.json` service URLs |
| pelion-system-tests | YES | YES | `configs/localhost.json` host/port |
| mbed-clitest-systemtest | YES | YES | `configs/localhost.json` host/port |

### Feature flag strategy (for incremental cutover)

If running Java and Go side by side:
- Route by account tier: Go handles tier=0 (free) accounts first
- Route by feature flag: use nginx upstream toggle in `docker/nginx-gateway.conf`
- Shadow mode: Go receives all traffic but Java's response is canonical; log diffs

### Non-regression rule

**No test that passes against Java may fail against Go.** Any failure is a bug in the
Go implementation, not a test to be disabled.

---

## Step 5 — Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| OPA Rego policy port is incomplete | High | High | Start spike in Phase 8.1; policy unit tests first |
| `rt_` reference token format incompatible | Medium | High | Implement token proxy in gateway; resolve tokens at edge |
| Redis session schema mismatch | Low | High | Share Redis instance; identical key format |
| PBKDF2 API key hashes need recompute | Low | High | Go reads existing hashes directly — PBKDF2 is portable |
| Multi-tenancy isolation regression | Medium | Critical | Dedicated isolation test class; automated account-crossing checks |
| mbed-billing-client integration | Medium | Medium | Replace with direct HTTP call; same event format |
| Email (GreenMail SMTP) integration | Low | Low | SMTP protocol is standard; any SMTP client works |

---

## Deliverables

When Phase 8 is complete, produce:

1. `docs/codeconverter/11-migration-plan/behavioral_inventory.md` — consolidated behavioral spec
2. `docs/codeconverter/11-migration-plan/framework_mapping.md` — target framework capability mapping table
3. `docs/codeconverter/11-migration-plan/implementation_schedule.md` — phased plan with exit criteria
4. `docs/codeconverter/11-migration-plan/risk_register.md` — full risk table
5. Updated `docs/codeconverter/STATE.md` — next action is Phase 8.1 or whichever phase is next

---

## Exit Criteria

Phase 8 planning is complete when:
- [ ] Behavioral inventory exists with ≥ 30 testable invariants
- [ ] Every "High" risk item has a spike plan
- [ ] Implementation phases cover 100% of `docs/codeconverter/05-api-surface/API.md` endpoints
- [ ] Test matrix documents how every suite runs against Go
- [ ] `docs/codeconverter/STATE.md` updated with concrete next action

Phase 8 *implementation* is complete when:
- [ ] All test suites pass against the Go service at the same rate as the Java baseline
- [ ] Performance within 2× of Java baseline (NFT tests)
- [ ] Java service can be shut down with no customer-visible difference
