<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase04-api-surface.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 05-api-surface**. When the text below says "Phase N",
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

# AI Coder Instructions — Phase 4: Complete API Surface Reference

## Mission (read carefully)

Produce the definitive, complete list of every HTTP endpoint this service exposes. This document is the contract specification for the Go replacement. Every endpoint in this document must be implemented in Go. Every endpoint absent from this document is not required.

No guessing. No "probably". No entries marked TBD. Every endpoint must be verified against actual route registration code in this repository.

You will not stop until the count of endpoints in the output document matches the count of routes registered in the source code.

---

## Step 0 — Read Prior Analysis

Before scanning source code, read the phase01 output. It contains previously discovered endpoints organized by module and iteration. Use it as your starting reference list — you will verify, complete, and re-organize it, not discard it.

Read these files and take note of every endpoint already discovered:

```
docs/codeconverter/02-codebase-analysis/01_external_api_surface.md
docs/codeconverter/02-codebase-analysis/iteration8_admin_internal_aggregator.md
docs/codeconverter/02-codebase-analysis/iteration10_v1_rest_resources.md
docs/codeconverter/02-codebase-analysis/iteration13_admin_api.md
docs/codeconverter/02-codebase-analysis/iteration14_aggregator_api.md
docs/codeconverter/02-codebase-analysis/iteration15_internal_api.md
docs/codeconverter/02-codebase-analysis/iteration25_v1_remaining.md
```

Also read the v2 versions of those files in `docs/codeconverter/02-codebase-analysis/v2/` — they contain corrections and additions.

Build a working list: `{METHOD} {PATH}` for every endpoint found in the phase01 docs. This is your starting inventory. Count them.

---

## Step 1 — Enumerate All Routes from Source Code

Scan the actual source code for all route registrations. This is the authoritative source. Phase01 analysis may have missed some.

### Step 1.1 — Find all JAX-RS `@Path` annotations

```bash
# Find all REST resource classes
find . -name '*.java' -path '*/main/*' | xargs grep -l '@Path\|@GET\|@POST\|@PUT\|@DELETE\|@PATCH' 2>/dev/null

# Extract all path annotations with their enclosing class
grep -rn '@Path\|@GET\|@POST\|@PUT\|@DELETE\|@PATCH' --include='*.java' \
  $(find . -name '*.java' -path '*/main/*' -print) | grep -v 'import ' | grep -v '//' \
  > /tmp/all_routes_raw.txt

wc -l /tmp/all_routes_raw.txt
```

### Step 1.2 — Find all router/application registration points

```bash
# Find Application classes (JAX-RS registration)
grep -rn 'extends Application\|ResourceConfig\|register(' --include='*.java' -l .

# Find Vert.x router if used
grep -rn 'router\.route\|router\.get\|router\.post\|router\.put\|router\.delete' \
  --include='*.java' -l .

# Find any programmatic route registration
grep -rn 'addResource\|registerResource\|bind.*Resource' --include='*.java' .
```

Read every file found by the above commands. For each registration point, extract the full path (combine the class-level `@Path` with the method-level `@Path` to get the complete route).

### Step 1.3 — Find Vert.x EventBus registrations (intra-codebase message bus)

```bash
# Find EventBus consumer addresses
grep -rn 'eventBus\(\)\.consumer\|eventBus\(\)\.localConsumer\|\.consumer(' \
  --include='*.java' . | grep -v 'import\|//'

# Find EventBus send/publish calls
grep -rn 'eventBus\(\)\.send\|eventBus\(\)\.publish\|\.send(\|\.publish(' \
  --include='*.java' . | grep -v 'import\|//' | grep -v 'System\.out'
```

### Step 1.4 — Find RabbitMQ consumers and publishers

```bash
grep -rn 'basicConsume\|basicPublish\|queue\.declare\|exchange\.declare\|routingKey' \
  --include='*.java' . | grep -v 'import\|//'

grep -rn '@RabbitListener\|@RabbitHandler\|AmqpTemplate\|RabbitTemplate' \
  --include='*.java' . | grep -v 'import'
```

### Step 1.5 — Produce the definitive route inventory

After all scans, produce a flat list:
```
{METHOD} {FULL_PATH} — {CLASS}#{METHOD_NAME} — {FILE}:{LINE}
```

Sort by path. Count total. This count is your target: the output document must contain this many endpoint entries.

---

## Step 2 — Classify Each Endpoint

For each endpoint, assign it to exactly one of these five categories based on its path prefix and purpose:

### Category 1: External / Customer-Facing APIs

Path prefixes: `/v3/`, `/auth/`, `/v1/` (note: some `/v1/` paths are actually internal — read the handler to determine whether they are customer-facing or not; when in doubt, check whether the handler's auth check allows regular user credentials).

These are the APIs that customers and their applications call directly. They are exposed through the API gateway. Auth is typically via access token or API key.

### Category 2: Admin / Root-Only APIs

Path prefix: `/admin/v3/`

These are internal operator tooling endpoints. They are not exposed in the public API documentation. They are accessible only to operators with root-level or admin-level credentials. They are still exposed through the gateway but behind additional auth checks.

### Category 3: Aggregator / Multi-Tenant APIs

Path prefix: `/v3/accounts/{accountId}/` (operations performed by a parent account on a sub-account) or similar multi-tenant scoped paths.

These are endpoints where the caller acts on behalf of a child account. Identify these by looking for path parameters containing `accountId` or `tenantId` where the calling account is different from the target account, combined with authorization checks for aggregator/parent-account role.

### Category 4: Internal Cross-Service APIs

Path prefixes: `/internal/v1/`, `/internal/v2/`, or any path that requires an internal service JWT or mTLS client certificate rather than a user access token.

These are called by other backend services, not by customers. Identify by checking the auth filter applied — internal endpoints will validate an internal service token rather than a user session token.

### Category 5: Intra-Codebase / Message Bus

These are not HTTP endpoints. They are:
- Vert.x EventBus addresses (internal async message passing between modules in this repo)
- RabbitMQ topics consumed or published by this service (internal async communication between service instances or between this service and others on the same message bus)

For message bus entries, document: address/topic name, direction (consume/publish), message schema summary, and which module handles it.

---

## Step 3 — Write the Output Document

Write `docs/codeconverter/05-api-surface/API.md`.

The document must use this exact structure:

```markdown
# IAM Service — Complete API Surface Reference

_Generated in Phase 4 of Java-to-Go migration_
_Based on: source code route registration + phase01 analysis_
_Date: {DATE}_

## Summary

| Category | Endpoint count |
|---|---|
| External / customer-facing | {N} |
| Admin / root-only | {N} |
| Aggregator / multi-tenant | {N} |
| Internal cross-service | {N} |
| Intra-codebase / message bus | {N} |
| **Total** | **{TOTAL}** |

---

## 1. External / Customer-Facing APIs

These endpoints are part of the public API contract. The Go replacement must implement all of them with identical request/response shapes, identical auth requirements, and identical error codes.

### 1.1 Account Management

| Method | Path | Auth required | Description | Handler class |
|---|---|---|---|---|
| GET | /v3/accounts/me | access token | Get the calling account's details | AccountsResource |
| ... | ... | ... | ... | ... |

### 1.2 User Management

| Method | Path | Auth required | Description | Handler class |
|---|---|---|---|---|
| ... | ... | ... | ... | ... |

### 1.3 API Keys

(continue organizing sub-sections by domain)

### 1.4 Groups and Policy Management

### 1.5 Applications and OAuth2 Clients

### 1.6 Identity Providers (SAML, OIDC federation)

### 1.7 Certificates

### 1.8 Branding

### 1.9 Agreements and Terms of Service

### 1.10 Invitations and Registration

### 1.11 Authentication (/auth/ prefix)

### 1.12 OpenID Connect Discovery and Token Endpoints

---

## 2. Admin / Root-Only APIs

These endpoints are not part of the public API. They are for internal operator use. The Go replacement must implement them, but they do not need to match public documentation — they must match the behavior the operations team depends on.

| Method | Path | Auth required | Description | Handler class |
|---|---|---|---|---|
| ... | ... | root/admin token | ... | ... |

---

## 3. Aggregator / Multi-Tenant APIs

These endpoints allow a parent account (aggregator) to perform operations on behalf of child accounts. The Go replacement must preserve the multi-tenant security model: a caller can only act on a child account they own.

| Method | Path | Auth required | Description | Handler class |
|---|---|---|---|---|
| ... | ... | aggregator token for account {accountId} | ... | ... |

---

## 4. Internal Cross-Service APIs

These endpoints are called by other backend microservices, not by customers. They are not in the public API documentation. The Go replacement must implement them — breaking these will break other services.

| Method | Path | Auth required | Description | Calling service(s) |
|---|---|---|---|---|
| ... | /internal/v1/... | internal service JWT | ... | {service name from phase01 or references.md} |

---

## 5. Intra-Codebase / Message Bus

These are not HTTP endpoints. They represent async communication internal to this service.

### 5.1 Vert.x EventBus Addresses

| Address | Direction | Producer module | Consumer module | Message schema summary |
|---|---|---|---|---|
| ... | publish | ... | ... | ... |

### 5.2 RabbitMQ Topics

| Exchange / Queue | Direction | Routing key | Producer | Consumer | Message schema summary |
|---|---|---|---|---|---|
| ... | consume | ... | external | IAM module | ... |
| ... | publish | ... | IAM module | external | ... |

---

## Appendix A: Route-to-Handler Index

Complete flat list sorted by path for quick lookup:

| Method | Path | Class | Method name | Source file |
|---|---|---|---|---|
| DELETE | /admin/v3/accounts/{accountId} | AdminAccountsResource | deleteAccount | iam-identity/.../AdminAccountsResource.java |
| ... | ... | ... | ... | ... |

---

## Appendix B: Source of Truth Verification

| Count type | Count |
|---|---|
| Routes registered in source code (from Step 1.5) | {N} |
| Routes in this document | {N} |
| Delta (must be 0) | 0 |

If the delta is not 0, Phase 4 is not complete. Identify and account for the discrepancy before declaring done.
```

---

## Rules for Each Endpoint Entry

Every endpoint in the document must have:

- **Method**: exact HTTP method (GET, POST, PUT, DELETE, PATCH)
- **Path**: exact path including path parameter names (e.g., `/v3/users/{userId}`)
- **Auth required**: what credential the caller must present — e.g., "access token", "API key", "admin token", "internal service JWT", "none (public)"
- **Description**: one sentence describing what the endpoint does, written in present tense
- **Handler class**: the Java class that handles the request (not the interface — the implementation)

Entries must NOT say:
- "probably filters by X"
- "might return 404"
- "similar to the other endpoint"
- "TBD"
- "see source"

If you cannot fill in a field with verified evidence, that endpoint is not yet documented. Go find the evidence.

---

## Special Cases

### Endpoints with multiple auth modes

Some endpoints accept both an access token AND an API key. Document both:

```
Auth required: access token OR API key (both accepted; different permission scopes apply)
```

### Deprecated endpoints still in code

If an endpoint has a `@Deprecated` annotation or a deprecation notice in its Javadoc, include it in the document with a `[DEPRECATED]` marker. The Go replacement must still implement it unless the human explicitly says to drop it.

### Endpoints guarded by feature flags

If an endpoint is conditionally registered based on a config flag or feature toggle, document the flag:

```
Auth required: access token
Feature flag: iam.feature.oidc.enabled=true (endpoint not registered if false)
```

### Wildcard or catch-all routes

If there are catch-all routes (`@Path("{anything:.*}")`), document what they do and under what conditions they match.

---

## Cross-Check Against Phase01 Output

After completing the source code scan and writing the document, run this cross-check:

1. Take every endpoint mentioned in any phase01 iteration file.
2. Confirm it appears in `docs/codeconverter/05-api-surface/API.md`.
3. If a phase01 entry is missing from the output document, either: (a) add it with evidence, or (b) document why it was excluded (e.g., the route was removed in a recent commit — show the git log evidence).

```bash
# Extract all path-like strings from phase01 docs
grep -rh '`/v3/\|`/auth/\|`/admin/\|`/internal/' docs/codeconverter/02-codebase-analysis/ \
  | grep -oP '`/[^`]+`' | sort -u > /tmp/phase01_paths.txt

# Compare against your final document
grep -oP '\| (GET|POST|PUT|DELETE|PATCH) \| /[^ |]+' docs/codeconverter/05-api-surface/API.md \
  | awk '{print $3}' | sort -u > /tmp/api_md_paths.txt

diff /tmp/phase01_paths.txt /tmp/api_md_paths.txt
```

Any paths in phase01 that are not in the output document must be explained.

---

## Exit Criteria

You may declare Phase 4 complete only when ALL of the following are true:

1. `docs/codeconverter/05-api-surface/API.md` exists and is committed to the working branch.
2. Appendix B shows zero delta between routes registered in source code and routes in the document.
3. Every endpoint entry has all required fields filled with verified evidence. No "TBD", no "probably", no "similar to".
4. The cross-check against phase01 output is complete with zero unexplained discrepancies.
5. Every intra-codebase message bus address (EventBus and RabbitMQ) is documented in section 5.
6. The summary count table at the top accurately reflects the counts in the body of the document.

If the source code scan reveals routes not in phase01, that is expected — add them. If phase01 mentions routes not found in current source code, check git history to determine if they were removed; document your finding either way.
