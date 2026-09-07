<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase06-codebase-introduction.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 07-target-codebase**. When the text below says "Phase N",
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

# Phase 6 — Codebase Introduction

> **Prerequisite:** Phases 1–5 must be complete.
> Required reading before starting:
> `docs/codeconverter/05-api-surface/API.md`, `docs/codeconverter/06-domain-analysis/GAP_ANALYSIS.md`,
> all domain analysis documents in `docs/codeconverter/06-domain-analysis/`.

---

## Mission

Establish the target codebase where the replacement service will be built.

This is an **interactive phase**. The AI must ask questions and wait for answers before
proceeding. Do not assume. Do not skip the interview. The decisions made here define
the foundation for every subsequent phase.

At the end of this phase, two documents exist in `docs/codeconverter/07-target-codebase/`:
- `stack.md` — the technology decisions and project coordinates
- `analysis.md` — what the target codebase is starting from and what it needs to become

---

## Step 0 — Opening Question

Begin by asking the human exactly this:

---

**"We are ready to establish the target codebase. Before I proceed, I need to know:**

**Are we building into a new project, or an existing project?**

- **New project** — no code exists yet; we will define the stack and initialize the repository
- **Existing project** — there is already a Go codebase (or similar); we will clone it and analyze how to add the IAM replacement to it

**Please reply with `new` or `existing`.**"

---

Wait for the answer. Do not continue until you have it.

---

## Path A — New Project

### A1 — Stack Interview

Ask the following questions **one block at a time**. Wait for each answer before
presenting the next block. Do not present all questions at once.

---

**Block 1 — Language and framework:**

"I'll default to **Go** as the language. What framework or application platform should
the replacement be built on?

Common choices for a Go service:
- A specific framework (e.g. Gin, Echo, Fiber, Chi, standard `net/http`)
- A platform that provides built-in auth primitives (e.g. one with native RBAC, OPA, OIDC support)
- No framework — plain Go with hand-rolled routing

Tell me your choice, or say `Go` and I will ask follow-up questions to narrow it down.

If you want a different language entirely (Rust, TypeScript, Python, etc.), say so now."

---

**Block 2 — Data stores** (only ask after Block 1 is answered):

"The existing Java service uses:
- **PostgreSQL 11** for all persistent state (13 policy tables, user/account/group schema)
- **Redis 5** for sessions, API key cache, reference token lookup
- **RabbitMQ 3.8** for account lifecycle events (deleted, updated)

The replacement must be compatible with the same PostgreSQL schema — the database will be
shared during cutover.

Are you keeping all three? Any changes or additions?
(Default: keep all three as-is.)"

---

**Block 3 — Repository location** (only ask after Block 2 is answered):

"Please provide the URL of the **new repository** where the replacement will live.

This must be an accessible git remote. I will clone it, verify it is empty or nearly
empty, and establish the working branch.

Example: `https://github.com/YourOrg/your-new-service`

Also confirm the branch name to use (default: `2026Q1_update`)."

---

**Block 4 — Containerization and CI** (only ask after Block 3 is answered):

"Two quick questions about the deployment target:

1. Container format: Docker (default) or something else?
2. CI/CD: GitHub Actions, Jenkins, or none for now?

These go into `stack.md` for reference. Answer with your preferences or `defaults`."

---

### A2 — Repository Setup

After all blocks are answered:

1. Clone the repo to a **persistent location** — not `/tmp`.
   The working directory for the source repo is typically `/workspace/recode/IAM/auth` (or similar).
   Clone the target repo as a sibling of the source repo:
   ```bash
   # Determine the parent directory of the source repo
   SOURCE_PARENT=$(dirname $(git -C . rev-parse --show-toplevel))
   REPO_NAME=$(basename {REPO_URL} .git)
   git clone {REPO_URL} ${SOURCE_PARENT}/${REPO_NAME}
   ```
   Example: if the source repo is at `/workspace/recode/IAM/auth`, clone to `/workspace/{repo-name}`.
   **Do not use `/tmp/` — it is not persistent across sessions.**

2. Verify it is empty or contains only scaffold files (README, .gitignore, go.mod).
   If it contains substantial code, **stop** and tell the human:
   "This repository does not appear to be empty. It may be an existing project.
   Should we switch to the `existing` path instead?"

3. Create and switch to the working branch:
   ```bash
   git -C ${SOURCE_PARENT}/${REPO_NAME} checkout -b {BRANCH_NAME}
   ```

4. Record the result (full clone path goes into `stack.md`).

### A3 — Write `stack.md`

Write `docs/codeconverter/07-target-codebase/stack.md` with the following structure:

```markdown
# Target Stack

## Project Coordinates
- Repository: {REPO_URL}
- Branch: {BRANCH_NAME}
- Cloned at: {FULL_CLONE_PATH}  (sibling of source repo, not /tmp)

## Language and Framework
- Language: Go {version if determinable, else "latest stable"}
- Framework: {chosen framework}

## Data Stores
- Primary DB: PostgreSQL 11 — shared with Java service during cutover
  Schema ownership: Java writes, Go reads initially; Go takes over post-cutover
- Cache / Session store: Redis 5
  Key namespace: identical to Java service (no migration needed)
- Message bus: RabbitMQ 3.8
  Exchange: iam.accounts (existing); Go must publish same event format

## Infrastructure
- Container: Docker
- CI/CD: {choice}

## Decisions Log
| Decision | Choice | Reason |
|----------|--------|--------|
| Language | Go | Migration target |
| Framework | {chosen framework} | {reason for choice} |
| DB schema | Shared (no changes) | Cutover safety |
| Branch | {BRANCH_NAME} | Consistent with Java repo |
```

Fill in all fields from the interview answers.

### A4 — Write `analysis.md` (new project)

Write `docs/codeconverter/07-target-codebase/analysis.md` with the following structure:

```markdown
# Target Codebase Analysis

## Project Status
**New project.** No existing functionality. The repository was initialized on {date}
at {REPO_URL} on branch {BRANCH_NAME}.

## What Must Be Built (from scratch)
This service must implement the full replacement for the Java IAM service.
All functionality documented in `docs/codeconverter/05-api-surface/API.md` must be built.

Reference documents:
- API contract: `docs/codeconverter/05-api-surface/API.md` (~{N} endpoints)
- Behavioral spec: see all `docs/codeconverter/06-domain-analysis/*.md` domain documents
- Gap analysis: `docs/codeconverter/06-domain-analysis/GAP_ANALYSIS.md`
- Test baseline: `docs/codeconverter/04-test-baseline/tests.md` (must achieve identical pass rate)

## Implementation Starting Point
| Category | Count | Notes |
|----------|-------|-------|
| Existing endpoints | 0 | New project |
| Endpoints to implement | ~{N from API.md} | See API.md |
| Domain features to implement | {N from phase05} | See domain analysis docs |

## Recommended First Steps
See `.claude/skills/codeconverter-11-migration-plan/instructions.md` for the phased implementation schedule.
High-risk items to spike first (from GAP_ANALYSIS.md):
1. OPA policy engine — port 4-tier cascade to Rego
2. Multi-tenancy isolation — account-scoped RBAC
3. Reference token (`rt_`) format compatibility
```

---

## Path B — Existing Project

### B1 — Repository Interview

Ask:

---

"Please provide:

1. The URL of the **existing repository**
   Example: `https://github.com/YourOrg/existing-service`

2. The branch to create for this work (default: `2026Q1_update`)

3. Any authentication needed to clone it? (PAT, SSH key, public repo?)"

---

Wait for the answer.

### B2 — Clone and Explore

1. Clone the repository to a **persistent location** — not `/tmp`.
   Clone as a sibling of the source (`auth`) repo:
   ```bash
   SOURCE_PARENT=$(dirname $(git -C . rev-parse --show-toplevel))
   REPO_NAME=$(basename {EXISTING_REPO_URL} .git)
   git clone {EXISTING_REPO_URL} ${SOURCE_PARENT}/${REPO_NAME}
   ```
   Example: if the source repo is at `/workspace/recode/IAM/auth`, clone to `/workspace/{repo-name}`.
   **Do not use `/tmp/` — it is not persistent across sessions.**
   Record the full clone path. Use it everywhere below instead of a placeholder.

2. Create and switch to the working branch:
   ```bash
   git -C ${SOURCE_PARENT}/${REPO_NAME} checkout -b {BRANCH_NAME}
   ```

3. Perform a codebase orientation — **do not skip this**.
   In all commands below, replace `/workspace/{repo-name}` with the actual clone path:

   ```bash
   # Language and framework detection
   ls /workspace/{repo-name}
   cat /workspace/{repo-name}/go.mod 2>/dev/null || cat /workspace/{repo-name}/package.json 2>/dev/null || echo "no manifest found"

   # Entry points
   find /workspace/{repo-name} -name "main.go" -o -name "cmd" -type d | head -20

   # Existing route/endpoint registrations
   grep -r "router\.\|http\.Handle\|\.GET\|\.POST\|\.PUT\|\.DELETE" /workspace/{repo-name} --include="*.go" -l | head -20

   # Database connections
   grep -r "postgres\|pgx\|gorm\|sql.Open" /workspace/{repo-name} --include="*.go" -l | head -10

   # Existing test files
   find /workspace/{repo-name} -name "*_test.go" | wc -l
   ```

4. Read at minimum:
   - The main entry point (`main.go` or equivalent)
   - The router/handler registration
   - The go.mod (or equivalent manifest)
   - Any existing README

### B3 — Existing Endpoint Inventory

Extract every existing API endpoint in the target codebase and build a table:

| Method | Path | Handler | Notes |
|--------|------|---------|-------|

This will be compared against `docs/codeconverter/05-api-surface/API.md` in the next step.

### B4 — Write `stack.md`

Write `docs/codeconverter/07-target-codebase/stack.md` derived from what you found:

```markdown
# Target Stack

## Project Coordinates
- Repository: {EXISTING_REPO_URL}
- Branch: {BRANCH_NAME}
- Cloned at: {FULL_CLONE_PATH}  (sibling of source repo, not /tmp)

## Language and Framework
- Language: {detected — e.g. Go 1.22}
- Framework: {detected — e.g. Gin, Echo, Fiber, Chi, standard net/http}

## Data Stores
- Primary DB: {detected from config/code}
- Cache: {detected}
- Message bus: {detected}

## Compatibility Notes
The **Java IAM service** being replaced uses:
- PostgreSQL 11 (must share schema during cutover — no destructive migrations)
- Redis 5 (session + cache namespace must remain identical)
- RabbitMQ 3.8 (iam.accounts exchange — must publish identical event format)

If the existing project uses different data stores, document the migration plan here.

## Decisions Log
| Decision | Choice | Reason |
|----------|--------|--------|
| Branch | {BRANCH_NAME} | Consistent with Java repo |
| Schema strategy | {shared / migrated / TBD} | |
```

### B5 — Gap Analysis: Write `analysis.md` (existing project)

This is the most important output of Path B. The goal is to answer:
**"What does this existing codebase need to gain — not lose — to replace the Java IAM service?"**

Write `docs/codeconverter/07-target-codebase/analysis.md` with this structure:

---

```markdown
# Target Codebase Analysis

## Project Status
**Existing project.** Repository: {URL}, branch: {BRANCH_NAME}.
Analysis performed: {date}.

## What the Existing Codebase Already Has

### Existing Endpoints
{paste the table from B3}

Total existing endpoints: {N}

### Existing Domain Coverage
| Domain | Implemented? | Notes |
|--------|:------------:|-------|
| Authentication (login/logout/refresh) | {Y/N/Partial} | |
| Account management | {Y/N/Partial} | |
| User management | {Y/N/Partial} | |
| API key management | {Y/N/Partial} | |
| Groups | {Y/N/Partial} | |
| Policies / ABAC | {Y/N/Partial} | |
| RBAC | {Y/N/Partial} | |
| OIDC / federation | {Y/N/Partial} | |
| SAML2 | {Y/N/Partial} | |
| MFA | {Y/N/Partial} | |
| Multi-tenancy / sub-accounts | {Y/N/Partial} | |
| Certificates | {Y/N/Partial} | |
| Agreements | {Y/N/Partial} | |
| Branding | {Y/N/Partial} | |
| Admin / root-only surface | {Y/N/Partial} | |
| Internal cross-service APIs | {Y/N/Partial} | |

## What Needs to Be Added

Derived by comparing `docs/codeconverter/05-api-surface/API.md` (Java IAM surface)
against the existing endpoint inventory above.

### Missing Endpoints
{list every endpoint in API.md that has no counterpart in the existing project}

### Partially Covered Endpoints
{list endpoints where the path exists but behavior differs — auth model, fields, error shapes}

### Missing Domain Features
{list domain features from phase05 analysis docs that are absent or incomplete}

## Hard Constraints: What Must NOT Be Changed

The following existing functionality must be preserved. Adding the IAM replacement
must not break what is already there:

{list existing endpoints, data models, auth flows that must remain unchanged}

If any existing functionality conflicts with an IAM requirement, flag it here for
human resolution — do not silently choose one over the other.

## Addition Strategy

For each missing area, recommend how to add it without disrupting existing functionality:

| Area | Strategy | Risk |
|------|----------|------|
| New endpoints | Add new route groups under existing router | Low |
| Auth middleware | Layer IAM auth alongside existing auth | Medium |
| DB schema additions | Additive migrations only (no ALTER/DROP) | Low |
| Policy engine (OPA) | Mount as separate middleware layer | Medium |
| Multi-tenancy isolation | New middleware, no changes to existing handlers | High |

## Implementation Order

Based on the gap analysis and risk assessment, recommend the order in which missing
functionality should be added. Highest-risk items first (to surface integration
issues early):

1. {highest risk addition}
2. ...

See `.claude/skills/codeconverter-11-migration-plan/instructions.md` for the detailed implementation schedule.
```

---

## Step 3 — Confirm with Human

After writing both `stack.md` and `analysis.md`, present a summary to the human:

---

"Phase 6 is complete. Here is what I found and decided:

**Target repo:** {URL}
**Branch:** {BRANCH_NAME}
**Path taken:** New project / Existing project

**Stack:** {language} + {framework} + PostgreSQL + Redis + RabbitMQ

**`docs/codeconverter/07-target-codebase/stack.md`** — written ✓
**`docs/codeconverter/07-target-codebase/analysis.md`** — written ✓

For a **new project**: {N} endpoints need to be built from scratch.
For an **existing project**: {N} endpoints are already present, {M} need to be added,
{K} are partial matches requiring extension.

**Next step:** Phase 7 — Gap Validation
Prompt: read `.claude/skills/codeconverter-08-gap-validation/instructions.md` and follow the instructions.
The primary target is `docs/codeconverter/07-target-codebase/analysis.md`.

Do you want me to proceed to Phase 7, or are there corrections to make first?"

---

Wait for human confirmation before proceeding.

---

## Exit Criteria

Phase 6 is complete when:

- [ ] Human answered `new` or `existing`
- [ ] All interview blocks answered (Path A) OR repo cloned and oriented (Path B)
- [ ] `docs/codeconverter/07-target-codebase/stack.md` written with all fields populated — no `{TBD}`
- [ ] `docs/codeconverter/07-target-codebase/analysis.md` written:
  - New project: implementation scope clearly stated
  - Existing project: gap table complete, hard constraints listed, addition strategy defined
- [ ] Summary presented to human and confirmed
- [ ] Both files committed to `{BRANCH_NAME}` in the **source** (`auth`) repo
