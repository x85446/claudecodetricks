<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase05-domain-analysis.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 06-domain-analysis**. When the text below says "Phase N",
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

# AI Coder Instructions — Phase 5: Domain-Specific Deep-Dive Analysis

## Mission (read carefully)

This phase is about understanding the *what* and *why* of every specialized feature this service implements, at a level of depth that allows a new implementation on a different platform to match or exceed the existing behavior. Generic migration analysis is insufficient — you must understand the domain.

This phase is **custom per repository**. You will ask the human to confirm your understanding and approve the document list before writing a single document. Then you will write each approved document with both a product manager perspective and a technical analysis. Then you will produce a gap analysis.

---

## Step A — Human Consultation (ask before acting)

Begin by presenting the following to the human. Do NOT start writing documents until you receive their response.

---

"Before I begin, I need to understand the domain of this service so I can recommend the right analysis documents. Please confirm or correct my understanding, and approve or modify the recommended document list below.

**My current understanding of this service:**

Based on my reading of the phase01 analysis, the codebase structure, and the API surface discovered in phase04, this appears to be an **Identity and Access Management (IAM) service** that serves as the authentication and authorization backbone for a multi-tenant IoT platform. Specifically, it appears to:

- Issue and validate access tokens (likely JWT-based) for human users and machine API keys
- Manage accounts, users, groups, and organizational hierarchy (parent/child accounts)
- Enforce role-based access control (RBAC) through a policy engine with attribute-based extensions
- Support federated identity via SAML 2.0 and OpenID Connect (external IdPs)
- Support multi-factor authentication (TOTP-based)
- Provide OpenID Connect discovery and token endpoints (acting as an identity provider itself)
- Manage applications (OAuth2 clients) for third-party integrations
- Handle invitations, registration, and onboarding flows
- Store branding configurations per account
- Publish events over RabbitMQ for downstream service notification

**Recommended document list:**

Based on this assessment, I recommend producing the following analysis documents. Each will be saved to `docs/codeconverter/06-domain-analysis/`.

| # | File | What it covers |
|---|---|---|
| 1 | `RBAC.md` | Role-based access control: roles, role hierarchy, group membership, API key role inheritance, admin delegation, template roles |
| 2 | `OPA.md` | Policy engine: attribute-based access control (ABAC) policies, evaluation cascade, policy types (user, group, account-level), conflict and mismatch behavior, URN resource model |
| 3 | `OIDC.md` | OpenID Connect flows: authorization code, implicit, PKCE, token exchange, discovery endpoint, claims mapping, acting as both OP and RP |
| 4 | `SAML2.md` | SAML 2.0 SSO: SP-initiated flow, IdP-initiated flow, assertion parsing, attribute mapping, session binding, multi-IdP per account |
| 5 | `MFA.md` | Multi-factor authentication: TOTP enrollment, enforcement policy (per-account, per-user), reauthentication rules, MFA bypass conditions |
| 6 | `MULTITENANCY.md` | Multi-tenancy model: account isolation, sub-account hierarchy, aggregator access model, tier limits and quotas, cross-account policy scoping |
| 7 | `JWT.md` | Token lifecycle: issuance, claims structure, refresh flow, revocation, key rotation, issuer types, token introspection |

**My recommendation:**

For this auth service, I recommend all seven documents above. The `OPA.md` and `RBAC.md` documents are especially critical regardless of target platform — they define the feature floor you must not fall below. The behavior of the policy engine (evaluation order, conflict resolution, inheritance model) is extremely easy to get wrong in a migration, and the consequences (privilege escalation or access denial) are severe. If the target platform provides OPA or RBAC natively, these documents become the specification for how to configure those features correctly.

Add, remove, or rename documents as appropriate for your context. For example:
- If this service also handles billing or subscription management, add a `BILLING.md`
- If the SAML integration is unused in your deployment, you may deprioritize `SAML2.md`
- If you are aware of additional specialized features not listed (e.g., certificate-based auth, device bootstrap), tell me and I will add them

**Please confirm:**
1. Is my understanding of the service domain correct? What is wrong or missing?
2. Which documents from the recommended list do you want me to write?
3. Are there any documents not on the list that you want added?
4. For the gap analysis (Step C), what is the target replacement platform? (e.g., a custom Go service, a managed identity platform, or an existing framework with built-in auth capabilities)"

---

Wait for the human's response before proceeding to Step B.

---

## Step B — Document Execution

After the human approves a document list (with any modifications they specify), execute the following process for each approved document. Do them in the order listed unless the human specifies otherwise.

### Process for each document

#### B.1 — Read all relevant source files

Do not write the document from memory or from phase01 summaries alone. Read the actual source code for the feature being documented. For each document, the relevant source locations are listed below. Use these as your starting points, then follow imports, superclasses, and factory calls to find everything.

| Document | Primary source locations to read |
|---|---|
| `RBAC.md` | `iam-access/`, `iam-policies/`, `iam-common/`, any class named `*Role*`, `*Group*`, `*Permission*`, `*PolicyGroup*` |
| `OPA.md` | `iam-policies-engine/`, `iam-policies/`, any class named `*Policy*`, `*Opa*`, `*Pdp*`, `*Pip*`, `*Pep*` |
| `OIDC.md` | `iam-federation/`, `iam-identity/`, any class named `*Oidc*`, `*OAuth*`, `*Token*`, `*Discovery*`, `*Authorization*` |
| `SAML2.md` | `iam-federation/`, any class named `*Saml*`, `*Sso*`, `*Assertion*`, `*Idp*`, `*IdentityProvider*` |
| `MFA.md` | `iam-identity/`, any class named `*Mfa*`, `*Totp*`, `*TwoFactor*`, `*Login*`, any endpoint named `/auth/...` |
| `MULTITENANCY.md` | `iam-identity/`, `iam-access/`, any class named `*Account*`, `*Aggregator*`, `*Tenant*`, `*SubAccount*`, `*Tier*` |
| `JWT.md` | `iam-federation/`, `iam-identity/`, `iam-common/`, any class named `*Token*`, `*Jwt*`, `*JwtVerifier*`, `*TokenService*`, `*KeyRotat*` |

Also read the relevant phase01 iteration files as a guide, but verify every claim against source code. Phase01 is a starting map, not a ground truth.

#### B.2 — Write the document

Every document must contain these two top-level sections, in this order. Do not omit either section. Do not merge them.

---

**Section 1: Product Manager Perspective**

Write this section as if explaining the feature to a product manager who will spec it for a new platform. They need to understand:

- What this feature does in plain language (no class names, no implementation details)
- Why it exists — what customer problem it solves, what business constraint it enforces
- What customers and operators directly depend on it for (who would be broken if it disappeared or changed behavior)
- What the user-visible behavior is: what a user can do, what happens when they do it, what error they see when something is wrong
- Edge cases that matter to customers (e.g., "an API key inherits the roles of the group it belongs to — if you remove the group, the API key loses those roles silently")
- Any behavior that is counterintuitive, surprising, or that has caused customer support issues in the past (if visible from code comments, test names, or issue references)

Write at a level of detail where someone could write a product requirements document for the replacement feature from this section alone.

---

**Section 2: Technical Analysis**

Write this section for the engineer who will implement the replacement. They need:

- Key classes and their responsibilities (exact class names + file paths)
- Data flow: how a request enters, what classes it passes through, what decisions are made, what is returned — in the order it happens
- Database tables involved: which tables are read, which are written, what the key fields are and what constraints matter
- External service calls: any calls to other services (cache lookups, queue publishes, inter-service HTTP) and when they happen in the flow
- Configuration: which config keys control the behavior of this feature; what happens at each configuration extreme
- Behavioral invariants that must be preserved exactly — express these as testable assertions:
  - Example: "A user with role ADMINISTRATOR can never be deleted via the public API — only via the admin API"
  - Example: "A group policy always overrides a user-level policy when both apply to the same resource action"
- Edge cases visible in code: error handling paths, retry logic, race conditions the code explicitly guards against
- Any behavior that is NOT obvious from the API documentation but is encoded in the implementation

Do not paraphrase. Use exact class names, exact field names, exact method names, exact config key names. If you say "the policy is evaluated by X", name the class X precisely.

---

#### B.3 — Save and verify

Save the document to `docs/codeconverter/06-domain-analysis/{NAME}.md` where `{NAME}` is the file name from the approved list (e.g., `RBAC.md`, `OIDC.md`).

After saving, re-read the saved file and verify:
- Both sections are present
- No section is empty or has placeholder text
- All class names and file paths cited are real (spot-check 3 randomly selected ones by reading the actual source file)
- No claims are made without evidence from source code

---

## Step C — Gap Analysis

After all approved documents are written and saved, produce a gap analysis.

Save it to `docs/codeconverter/06-domain-analysis/GAP_ANALYSIS.md`.

### Format

```markdown
# IAM Migration Gap Analysis

_Generated in Phase 5_
_Target platform: {PLATFORM — from human's Step A response}_
_Date: {DATE}_

---

## How to read this document

For each feature domain, this document describes:
- **Native**: the replacement platform provides this capability natively and the behavior matches
- **Extension needed**: the replacement platform provides a similar capability but requires configuration, extension, or customization to match the IAM service's behavior
- **Must port**: the replacement platform does not provide this capability; it must be implemented explicitly in the new service
- **Verify**: insufficient information to determine — requires investigation against the target platform

---

## RBAC

**Target platform capability**: {describe what the target platform provides}

| IAM behavior | Platform status | Notes |
|---|---|---|
| Template roles (built-in roles per account tier) | Native / Extension needed / Must port / Verify | ... |
| Group membership role inheritance | ... | ... |
| API key role inheritance from group | ... | ... |
| Admin delegation (granting admin rights to sub-users) | ... | ... |
| Role scoping to sub-accounts | ... | ... |

**Summary**: {1-3 sentence assessment of the gap for this domain}

**Risk level**: LOW / MEDIUM / HIGH
- LOW = platform handles it natively
- MEDIUM = some extension work required but straightforward
- HIGH = significant implementation required; behavior is complex and easy to get wrong

---

## OPA / Policy Engine

**Target platform capability**: {describe}

| IAM behavior | Platform status | Notes |
|---|---|---|
| URN-based resource model | ... | ... |
| Policy evaluation cascade (user → group → account) | ... | ... |
| ABAC conditions (device attributes, time-based) | ... | ... |
| Policy conflict resolution (deny overrides / most specific wins) | ... | ... |
| Policy inheritance across account hierarchy | ... | ... |

**Summary**: ...
**Risk level**: ...

---

## OIDC

**Target platform capability**: {describe}

| IAM behavior | Platform status | Notes |
|---|---|---|
| Authorization code flow | ... | ... |
| PKCE | ... | ... |
| Token exchange (RFC 8693) | ... | ... |
| Discovery endpoint (/.well-known/openid-configuration) | ... | ... |
| Custom claims mapping | ... | ... |
| Acting as Relying Party (consuming external OIDC IdPs) | ... | ... |

**Summary**: ...
**Risk level**: ...

---

## SAML 2.0

(same format as above)

---

## MFA

(same format)

---

## Multi-Tenancy

(same format)

---

## JWT / Token Lifecycle

(same format)

---

## Overall Migration Risk Summary

| Feature domain | Risk level | Key concern |
|---|---|---|
| RBAC | ... | ... |
| OPA / Policy engine | ... | ... |
| OIDC | ... | ... |
| SAML 2.0 | ... | ... |
| MFA | ... | ... |
| Multi-tenancy | ... | ... |
| JWT / Token lifecycle | ... | ... |

**Top 3 highest-risk items for the migration:**

1. {Item}: {reason it is high risk}
2. {Item}: {reason}
3. {Item}: {reason}

**Recommended implementation order** (address highest risk first):
1. {document name / feature domain}
2. ...
```

---

## Rules Applying to All Documents

- Do not write documents from memory. Every claim about behavior must be traceable to a source file, class, or method in this repo.
- Do not copy phase01 summaries verbatim. Phase01 is a discovery artifact. Phase05 documents are authoritative specifications. Rewrite in the appropriate register for each section.
- Do not speculate about intent without evidence. If you are uncertain, say so explicitly and explain what evidence you looked for and did not find.
- Class names, method names, and file paths must be exact. A reader must be able to open the file and find the thing you are describing.
- If the human's Step A response changes your understanding of the service domain, restate your updated understanding before beginning Step B.

---

## Exit Criteria

You may declare Phase 5 complete only when ALL of the following are true:

1. All documents approved by the human in Step A exist in `docs/codeconverter/06-domain-analysis/`.
2. Every document has both a "Product Manager Perspective" section and a "Technical Analysis" section. Neither is a stub or placeholder.
3. `docs/codeconverter/06-domain-analysis/GAP_ANALYSIS.md` exists, covers every approved document's feature domain, and has a risk level and summary for each.
4. You have spot-checked at least 3 specific class name or file path references in each document and confirmed they are correct by reading the actual source files.
5. All documents are committed to the working branch.

If the human rejects a document and asks for revisions, revise it and re-verify before marking Phase 5 complete.
