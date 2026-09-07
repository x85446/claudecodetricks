<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase08-bad-actors.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 09-dependency-audit**. When the text below says "Phase N",
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

# Phase 8 — External Dependency Audit ("Bad Actors")

> **Prerequisite:** Phases 1–7 must all be complete.
> Required inputs: `docs/codeconverter/02-codebase-analysis/`, `docs/codeconverter/03-dependency-discovery/references.md`,
> `docs/codeconverter/05-api-surface/API.md`, `docs/codeconverter/07-target-codebase/stack.md`,
> and the sibling repos directory (human provides path).

---

## Mission

Find every external system that depends on the source service in ways that would
break if the service is replaced. The replacement cannot ship until every bad actor
is identified and has a remediation plan.

A "bad actor" is any external system that bypasses the source service's public API
to depend on its internals — its database, its libraries, its wire formats, its
deployment topology, or its infrastructure. These hidden couplings are invisible
during normal development but will cause failures on cutover day.

Phase 2 (dependency discovery) found the obvious consumers. This phase finds the
non-obvious ones: the init container that queries the database directly, the sibling
service that parses a JWT claim name, the Helm chart that hardcodes a hostname.

---

## Step 0 — Confirm Readiness

Before starting, verify:

```bash
# Phase 01 output exists
ls docs/codeconverter/02-codebase-analysis/

# References documented
grep -c "^|" docs/codeconverter/03-dependency-discovery/references.md

# API surface documented
wc -l docs/codeconverter/05-api-surface/API.md

# Phase 06 stack decisions exist
cat docs/codeconverter/07-target-codebase/stack.md

# Sibling repos are cloned and accessible
ls {SIBLING_REPOS_PATH}
```

If any of the above is missing or incomplete, stop and complete the prerequisite
phase first. Document what you found in a "Readiness Check" section of your output.

---

## Step 1 — Define Scope

The human must provide the following values. These are used as search parameters
throughout the audit. Record them at the top of the output document.

| Parameter | Description | Example |
|-----------|-------------|---------|
| `{SIBLING_REPOS_PATH}` | Path to cloned sibling repositories | `/workspace/repos/` |
| `{DEPLOYMENT_MANIFESTS_PATH}` | Path to Helm charts / K8s manifests / Terraform | `/workspace/repos/platform/charts/` |
| `{SOURCE_DB_NAMES}` | Database name(s) owned by the source service | `myservicedb` |
| `{SOURCE_DB_USERS}` | Database user(s) used by the source service | `myservice_user` |
| `{SOURCE_LIBRARY_NAMES}` | Published library / package names | `myservice-client`, `myservice-sdk` |
| `{SOURCE_PACKAGE_IMPORTS}` | Import paths in source language | `com.example.myservice`, `github.com/org/myservice` |
| `{SOURCE_HOSTNAMES}` | Service hostnames, DNS names, service mesh names | `myservice.default.svc.cluster.local` |
| `{SOURCE_PORTS}` | Port numbers used by the source service | `8080`, `9187` |
| `{SOURCE_EXCHANGE_NAMES}` | Message bus exchange / topic / queue names | `myservice_events`, `myservice_accounts` |
| `{SOURCE_TABLE_NAMES}` | Key database table names (for cross-repo grep) | `accounts`, `users`, `policies` |
| `{SOURCE_SECRET_NAMES}` | Kubernetes secret names used by the source service | `myservice.postgres.user` |
| `{SOURCE_CONFIGMAP_NAMES}` | Kubernetes configmap names | `myservice-config` |
| `{SOURCE_CLAIM_NAMES}` | JWT claim names, header names, cookie names produced by the source service | `X-Account-Id`, `sub`, `scope` |

---

## Step 2 — Automated Sweep (Pass 1)

For each of the 11 categories below, run the specified search patterns across all
sibling repos at `{SIBLING_REPOS_PATH}` and the deployment manifests at
`{DEPLOYMENT_MANIFESTS_PATH}`. Record every match.

### Category 1 — Direct Database Access

**What to find:** Other services connecting to the source service's database directly,
bypassing its API.

**Search patterns:**
```bash
# Database name references
grep -rn "{SOURCE_DB_NAMES}" {SIBLING_REPOS_PATH} --include="*.yaml" --include="*.yml" --include="*.properties" --include="*.json" --include="*.sh" --include="*.sql" --include="*.py" --include="*.java" --include="*.go" --include="*.js" --include="*.ts" --include="*.tf"

# Database user/credential references
grep -rn "{SOURCE_DB_USERS}" {SIBLING_REPOS_PATH}

# Database secret references in K8s manifests
grep -rn "{SOURCE_SECRET_NAMES}" {DEPLOYMENT_MANIFESTS_PATH}

# JDBC/connection strings referencing the source database
grep -rn "jdbc.*{SOURCE_DB_NAMES}\|datasource.*{SOURCE_DB_NAMES}\|PGDATABASE.*{SOURCE_DB_NAMES}" {SIBLING_REPOS_PATH}

# SQL queries referencing source table names (outside the source service itself)
grep -rn "SELECT.*FROM.*{SOURCE_TABLE_NAMES}\|INSERT INTO.*{SOURCE_TABLE_NAMES}\|UPDATE.*{SOURCE_TABLE_NAMES}\|DELETE FROM.*{SOURCE_TABLE_NAMES}" {SIBLING_REPOS_PATH}
```

**True positive indicators:** A non-source-service repo mounting the source service's
database credentials, connecting to the source database by name, or running SQL
against tables owned by the source service.

**False positive indicators:** A service with its own table that happens to share a
name, documentation referencing the source database, monitoring/metrics queries.

---

### Category 2 — Git Submodule / Subtree References

**What to find:** Repos that include the source service's repository as a Git
submodule or subtree.

**Search patterns:**
```bash
# .gitmodules referencing the source repo
grep -rn "{SOURCE_REPO_NAME}" {SIBLING_REPOS_PATH} --include=".gitmodules"

# git subtree references
grep -rn "subtree.*{SOURCE_REPO_NAME}\|{SOURCE_REPO_URL}" {SIBLING_REPOS_PATH} --include="*.sh" --include="Makefile" --include="*.md"
```

**True positive indicators:** A `.gitmodules` entry pointing at the source service
repo, or a documented subtree pull from the source repo.

**False positive indicators:** Documentation mentioning the source repo by name.

---

### Category 3 — Source Code / Package Dependencies

**What to find:** Repos importing packages or modules from the source service's
codebase directly (not via a published library).

**Search patterns:**
```bash
# Import statements referencing source packages
grep -rn "{SOURCE_PACKAGE_IMPORTS}" {SIBLING_REPOS_PATH} --include="*.java" --include="*.go" --include="*.py" --include="*.js" --include="*.ts"

# Build file dependencies referencing source artifacts
grep -rn "{SOURCE_REPO_NAME}\|{SOURCE_ARTIFACT_NAME}" {SIBLING_REPOS_PATH} --include="pom.xml" --include="build.gradle" --include="go.mod" --include="package.json" --include="requirements.txt" --include="Cargo.toml"
```

**True positive indicators:** A build file declaring a dependency on the source
service's artifact, or source code importing from the source service's package
namespace.

**False positive indicators:** A shared library published by the source service
(those belong in Category 4).

---

### Category 4 — Shared Library Consumers

**What to find:** Repos depending on libraries published by the source service
(e.g., auth token libraries, SDK clients, shared model packages).

**Search patterns:**
```bash
# Published library names in build files
grep -rn "{SOURCE_LIBRARY_NAMES}" {SIBLING_REPOS_PATH} --include="pom.xml" --include="build.gradle" --include="go.mod" --include="package.json" --include="requirements.txt" --include="Cargo.toml" --include="*.csproj"

# Transitive dependency references
grep -rn "{SOURCE_LIBRARY_NAMES}" {SIBLING_REPOS_PATH} --include="*.lock" --include="go.sum" --include="yarn.lock" --include="package-lock.json"
```

**True positive indicators:** A build file listing a library published by the source
service as a direct or transitive dependency.

**False positive indicators:** A vendored copy of the library that is self-contained
and does not depend on the source service at runtime.

**Important:** For each library found, also audit the library's own source code. If
the library contains database access, wire format assumptions, or hardcoded service
URLs, every consumer of that library inherits those dependencies.

---

### Category 5 — Wire Format Dependencies

**What to find:** Services that parse specific claim names, header values, cookie
formats, or protocol fields produced by the source service, without using an
official client library.

**Search patterns:**
```bash
# JWT claim names, header names, cookie names
grep -rn "{SOURCE_CLAIM_NAMES}" {SIBLING_REPOS_PATH} --include="*.java" --include="*.go" --include="*.py" --include="*.js" --include="*.ts" --include="*.rb"

# Token format prefixes or suffixes unique to the source service
grep -rn "{SOURCE_TOKEN_PREFIX}\|{SOURCE_TOKEN_SUFFIX}" {SIBLING_REPOS_PATH}

# Custom header parsing
grep -rn "getHeader.*{SOURCE_HEADER_NAME}\|request.headers.*{SOURCE_HEADER_NAME}" {SIBLING_REPOS_PATH}
```

**True positive indicators:** Source code that parses a specific claim name, checks
a specific header value, or validates a specific token format that only the source
service produces.

**False positive indicators:** Standard claims (e.g., `sub`, `iss`, `exp`) that
follow an industry standard and would be produced identically by the replacement.

---

### Category 6 — Message Bus / Event Consumers

**What to find:** Services consuming events published by the source service (queue
exchanges, topic subscriptions, event payload format expectations).

**Search patterns:**
```bash
# Exchange/topic/queue name references
grep -rn "{SOURCE_EXCHANGE_NAMES}" {SIBLING_REPOS_PATH} --include="*.yaml" --include="*.yml" --include="*.properties" --include="*.json" --include="*.java" --include="*.go" --include="*.py" --include="*.js"

# Event type or routing key references
grep -rn "{SOURCE_EVENT_TYPES}\|{SOURCE_ROUTING_KEYS}" {SIBLING_REPOS_PATH}
```

**True positive indicators:** A service subscribing to an exchange/topic published
by the source service, or parsing event payloads in a format defined by the source
service.

**False positive indicators:** The source service's own subscription configuration.

**Important:** For each consumer found, document the expected event payload schema.
If the replacement changes the event format, every consumer must be updated.

---

### Category 7 — Internal API Contract Dependencies

**What to find:** Services calling the source service's internal or private API
endpoints (not part of the public API surface).

**Search patterns:**
```bash
# Internal endpoint paths
grep -rn "{SOURCE_INTERNAL_PATHS}" {SIBLING_REPOS_PATH} --include="*.java" --include="*.go" --include="*.py" --include="*.js" --include="*.ts" --include="*.yaml" --include="*.yml" --include="*.properties"

# Source service hostname + port in HTTP client configurations
grep -rn "{SOURCE_HOSTNAMES}.*{SOURCE_PORTS}\|{SOURCE_HOSTNAMES}:{SOURCE_PORTS}" {SIBLING_REPOS_PATH}

# Service name references in config files (service mesh, nginx, etc.)
grep -rn "{SOURCE_SERVICE_NAME}" {SIBLING_REPOS_PATH} --include="*.yaml" --include="*.yml" --include="*.conf" --include="*.properties" --include="*.json"
```

**True positive indicators:** A service making HTTP calls to an endpoint documented
as "internal" in `docs/codeconverter/05-api-surface/API.md`.

**False positive indicators:** References in API documentation or test fixtures.

---

### Category 8 — Hardcoded Service URLs / Ports / Hostnames

**What to find:** Services with the source service's hostname, port number, or
service name baked into configuration or code. Critical if the replacement changes
its deployment topology.

**Search patterns:**
```bash
# Hostnames
grep -rn "{SOURCE_HOSTNAMES}" {SIBLING_REPOS_PATH}

# Port numbers (filter for context to avoid noise)
grep -rn ":{SOURCE_PORTS}\|port.*{SOURCE_PORTS}\|PORT.*{SOURCE_PORTS}" {SIBLING_REPOS_PATH}

# Service discovery names
grep -rn "{SOURCE_SERVICE_DISCOVERY_NAME}" {SIBLING_REPOS_PATH} --include="*.yaml" --include="*.yml" --include="*.json" --include="*.properties" --include="*.conf" --include="*.tf"
```

**True positive indicators:** A config file or source code containing a hardcoded
hostname, port, or URL that points at the source service. If the replacement runs
on a different hostname or port, this will break.

**False positive indicators:** Documentation, comments, or example configurations.

---

### Category 9 — Mock / Stub Implementations

**What to find:** Repos containing mock or fake versions of the source service that
encode behavioral assumptions. These will drift when the replacement changes behavior.

**Search patterns:**
```bash
# Mock/stub/fake class or file names
grep -rln "mock.*{SOURCE_SERVICE_NAME}\|fake.*{SOURCE_SERVICE_NAME}\|stub.*{SOURCE_SERVICE_NAME}" {SIBLING_REPOS_PATH} -i

# Mock response fixtures
grep -rln "{SOURCE_SERVICE_NAME}.*mock\|{SOURCE_SERVICE_NAME}.*stub\|{SOURCE_SERVICE_NAME}.*fixture" {SIBLING_REPOS_PATH} -i

# Test doubles referencing the source service
grep -rn "Mock{SOURCE_CLASS_NAME}\|Fake{SOURCE_CLASS_NAME}\|Stub{SOURCE_CLASS_NAME}" {SIBLING_REPOS_PATH}
```

**True positive indicators:** A test helper that returns hardcoded responses
mimicking the source service, or a mock server that reimplements source service
endpoints with assumed behavior.

**False positive indicators:** Generic test framework mocking (e.g., `mock.patch`)
that does not encode source-service-specific behavior.

---

### Category 10 — Shared Infrastructure Access

**What to find:** Services accessing caches, queues, or other infrastructure owned
by the source service (e.g., a shared Redis instance, a shared message broker vhost).

**Search patterns:**
```bash
# Redis key prefixes or database numbers used by the source service
grep -rn "{SOURCE_REDIS_PREFIX}\|{SOURCE_REDIS_DB}" {SIBLING_REPOS_PATH}

# Shared cache or queue infrastructure names
grep -rn "{SOURCE_CACHE_NAME}\|{SOURCE_QUEUE_VHOST}" {SIBLING_REPOS_PATH} --include="*.yaml" --include="*.yml" --include="*.properties" --include="*.json" --include="*.conf"

# Infrastructure secrets shared across services
grep -rn "{SOURCE_INFRA_SECRET_NAMES}" {DEPLOYMENT_MANIFESTS_PATH}
```

**True positive indicators:** A service reading from or writing to a cache, queue,
or storage bucket that is owned and managed by the source service.

**False positive indicators:** Shared infrastructure that both services access
independently (e.g., a shared message broker where each service has its own vhost).

---

### Category 11 — Deployment Coupling

**What to find:** Helm charts, Kubernetes manifests, Terraform modules, or CI/CD
configs that reference the source service's secrets, configmaps, service names,
network policies, or health check paths.

**Search patterns:**
```bash
# K8s secret references
grep -rn "{SOURCE_SECRET_NAMES}" {DEPLOYMENT_MANIFESTS_PATH} --include="*.yaml" --include="*.yml"

# K8s configmap references
grep -rn "{SOURCE_CONFIGMAP_NAMES}" {DEPLOYMENT_MANIFESTS_PATH} --include="*.yaml" --include="*.yml"

# Service name in network policies, ingress rules, etc.
grep -rn "{SOURCE_SERVICE_NAME}" {DEPLOYMENT_MANIFESTS_PATH} --include="*.yaml" --include="*.yml" --include="*.tf" --include="*.json"

# Health check paths
grep -rn "{SOURCE_HEALTH_PATH}" {DEPLOYMENT_MANIFESTS_PATH}

# CI/CD pipeline references
grep -rn "{SOURCE_REPO_NAME}\|{SOURCE_SERVICE_NAME}" {SIBLING_REPOS_PATH} --include="*.yml" --include="*.yaml" --include="Jenkinsfile" --include="Makefile" --include=".gitlab-ci.yml" --include="*.tf"
```

**True positive indicators:** A deployment manifest for a different service that
mounts the source service's secrets, references its configmaps, or includes it in
a network policy. If the source service is renamed, redeployed, or has its secrets
rotated, the dependent manifest will break.

**False positive indicators:** The source service's own deployment manifests.

---

## Step 3 — Deep Verification (Pass 2)

For each finding from Pass 1:

1. **Read the actual source file** — do not rely on grep context alone.
2. **Classify** as TRUE POSITIVE or FALSE POSITIVE.
3. **For true positives**, document:
   - The exact file and line number
   - What the dependency is (quote the relevant code)
   - Which category it belongs to (1–11)
   - The severity (see Severity Classification below)
   - What would break if the source service is replaced without addressing this
   - The **solution** — what the replacement service must do (or preserve) to avoid the breakage
4. **For false positives**, document why it is a false positive (one sentence).

---

## Step 4 — Cross-Reference Audit (Pass 3)

Cross-reference the verified findings against prior phase outputs to catch anything
Pass 1 may have missed:

### 4a — Phase 2 cross-reference

Read `docs/codeconverter/03-dependency-discovery/references.md`. For every consumer listed there:
- Is it represented in the Pass 1 findings? If not, search for it specifically.
- Does it have dependencies beyond what Phase 2 documented (Phase 2 found API
  consumers; this phase looks for deeper couplings)?

### 4b — Phase 4 cross-reference

Read `docs/codeconverter/05-api-surface/API.md`. For every internal endpoint:
- Is every known caller represented in the Pass 1 findings?
- Are there internal endpoints with no known callers? These may have undiscovered
  consumers.

### 4c — Phase 1 cross-reference

Read `docs/codeconverter/02-codebase-analysis/` storage and messaging analysis. For every:
- Database table: is there a non-source-service accessor?
- Message exchange/topic: is there an undiscovered consumer?
- Cache key namespace: is there a shared accessor?

### 4d — Phase 6 cross-reference

Read `docs/codeconverter/07-target-codebase/bad-actors.md` if it exists (preliminary audit from
Phase 6). Verify that all findings from that preliminary audit are included in this
comprehensive audit. Flag any discrepancies.

---

## Step 5 — Remediation Planning

For each true positive finding, assign a remediation timeline and document the
specific change needed.

### Remediation timelines

| Timeline | Meaning |
|----------|---------|
| **Pre-migration** | Must be fixed before the replacement goes live. The consuming service must be updated first. |
| **Co-migration** | Can be fixed as part of the migration cutover. Coordinated change across services. |
| **Post-migration** | Can be fixed after the replacement is live. Low risk of failure during cutover. |
| **Accept risk** | The dependency is acknowledged but will not be fixed. Document the blast radius. |

### For each finding, document

| Field | Description |
|-------|-------------|
| Finding ID | Sequential identifier (BA-001, BA-002, ...) |
| Category | Which of the 11 categories |
| Consuming service | The repo/service that has the dependency |
| Dependency description | What exactly the dependency is |
| Evidence | File path, line number, code snippet |
| Severity | CRITICAL / HIGH / MEDIUM / LOW |
| What breaks | Specific failure mode if the dependency is not addressed |
| Solution | What the replacement service must do (or preserve) to prevent the breakage — the action taken by the migration team, not the consuming team |
| Remediation | The specific change needed in the consuming service (if any, beyond what the solution covers) |
| Timeline | Pre-migration / Co-migration / Post-migration / Accept risk |
| Owner | Team or person responsible for the consuming service (if known) |

---

## Severity Classification

| Severity | Definition |
|----------|------------|
| **CRITICAL** | Will cause production outage on cutover day |
| **HIGH** | Will cause feature breakage visible to customers |
| **MEDIUM** | Will cause internal tooling or operational breakage |
| **LOW** | Cosmetic or test-only impact |

---

## Deliverables

When Phase 8 is complete, produce:

1. `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md` — the full audit containing:
   - Scope parameters (from Step 1)
   - All findings organized by category (from Steps 2–3)
   - Cross-reference audit results (from Step 4)
   - Remediation plan for every true positive (from Step 5)
   - Summary statistics (total findings, by category, by severity, by timeline)

---

## Exit Criteria

Phase 8 is complete when:

- [ ] All sibling repos at `{SIBLING_REPOS_PATH}` have been scanned across all 11 categories
- [ ] Every finding from Pass 1 has been verified as TRUE POSITIVE or FALSE POSITIVE in Pass 2
- [ ] Cross-reference audit (Pass 3) has been completed against Phases 1, 2, 4, and 6
- [ ] Every true positive has a severity classification
- [ ] Every true positive has a remediation timeline and specific remediation action
- [ ] `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md` exists and is committed
- [ ] Human has reviewed the findings and confirmed completeness
