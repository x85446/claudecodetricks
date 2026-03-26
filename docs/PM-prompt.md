# PM — Product Manager Skill System

> Design document for the PM skill system.

---

## Architecture: PM + Focused Skills + WebTool

```
    Natural language → /pm scan this code and write the epics and features
                       /pm interview me about a new feature
                       /pm I need a new feature, when X happens we want Y
                       /pm webtool
                                    |
                              +-----+-----+
                              |    PM     |  (orchestrator)
                              +-----+-----+
                                    |
          +----------+---------+----+----+---------+----------+
          |          |         |         |         |          |
        epic     feature  requirement  test    iterator   auditor
        skill     skill     skill     skill    skill      skill
          |          |         |         |         |          |
          +----------+---------+---------+---------+----------+
                                    |
                              +-----+-----+
                              |  shared   |
                              |  database |
                              +-----------+
                             /      |      \
                      preflight  publish   status
                                    |
                              +-----+-----+
                              | WebTool  |  (web UI — Python + Vite + SQLite)
                              +-----------+
```

Each box is a separate SKILL.md — small, focused, and independently testable.
WebTool is a standalone web app that reads/writes the same database.

**Product auto-detection:** When only one product exists in the database, the product code is optional. `/pm status` works the same as `/pm myriplay status`.

---

## Modes

### Mode 1: Crawl

PM is pointed at code, documents, or websites. AI does the heavy lifting.

```
/pm scan this codebase and write the epics and features
/pm crawl ./src/ ./docs/architecture.md https://docs.example.com/api
```

PM dispatches work:
1. Calls **epic** skill to identify epics from source material
2. Calls **feature** skill to break epics into features
3. Calls **requirement** skill to derive requirements from features
4. Calls **test** skill to write test criteria for each requirement
5. Calls **iterator** skill whenever repeated value lists are detected
6. Calls **auditor** skill to verify dependency chain integrity

All results land in DB with `human_approved = 0`. Human reviews in WebTool.

### Mode 2: Interact

PM interviews a human. Human leads the conversation.

```
/pm interview me about a new feature
/pm I need a new feature. When x happens we want .....
/pm I need to add requirements to feature <uuid>
```

PM dispatches to the appropriate skill which runs its own h/ai or ai/h iteration loop. PM tracks what's been covered and what's missing.

### Mode 3: Voice

Same as Interact, but the human speaks instead of types. Voice is the natural way to describe features — faster, more fluid, captures nuance that gets lost when typing shorthand.

```
/pm voice
/pm voice new feature
```

PM activates voice capture, human speaks freely, speech is transcribed, then PM processes the transcript the same way it would typed input. The human can also speak feedback during iteration loops ("yeah that's good", "no, change the second requirement to...").

Voice works in two places:
1. **Claude Code terminal** — via VoiceMode (existing infrastructure). PM tells VoiceMode to listen, gets transcript back, processes it.
2. **WebTool** — browser-based voice input via Web Speech API. Click a mic icon on any entity to speak feedback, edits, or new content. No server-side STT needed — the browser handles it.

---

## Skill Inventory

### 1. PM (Product Manager) — Orchestrator

**File:** `skills/pm/SKILL.md`

**Role:** Coordinator. Never writes to the DB directly. Delegates to sub-skills and tracks completion.

**Responsibilities:**
- Parse natural language intent (crawl vs interact, scope)
- Dispatch to sub-skills in correct order (epics → features → requirements → tests)
- Track coverage: which epics have features? which features have requirements? which requirements have tests?
- Surface gaps: "3 features have no requirements, 7 requirements have no test criteria"
- Run preflight before any work begins
- Coordinate review workflows — but NEVER set `human_approved = 1` itself

**Does NOT do:**
- Write epics, features, requirements, or tests
- Make content decisions
- Set `human_approved` on anything — only WebTool or explicit human action does that
- Talk to the database directly (except for coverage queries)

---

### 2. Epic Skill — Big-Picture Thinking

**File:** `skills/pm-epic/SKILL.md`

**Role:** Big-picture product capability researcher and writer.

**What is an epic?**
An epic is a large product capability that decomposes into multiple features. Examples:
- "Multi-tenant cluster visualization" → topology rendering, detail levels, interactive controls, export
- "Device lifecycle management" → provisioning, monitoring, decommissioning, certificate rotation

**Crawl mode:**
- Read source material (code, docs, URLs)
- Identify top-level product capabilities
- Write: name, description (2-3 sentences), rationale (why this matters)
- Tag with high-level categories
- Insert with `human_approved = 0`

**Interact mode (h/ai iterate):**
- Ask: "What's the big-picture capability you want to describe?"
- Human gives rough description
- AI structures it into: name, description, rationale
- Present for approval
- Insert with `human_approved = 1`

**DB table:** `epics` (new)

```sql
CREATE TABLE IF NOT EXISTS epics (
    id              TEXT PRIMARY KEY,
    product_id      INTEGER NOT NULL REFERENCES our_products(id),
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,
    rationale       TEXT,
    version         INTEGER NOT NULL DEFAULT 1,
    base_version    INTEGER NOT NULL DEFAULT 1,   -- version this was derived from (self-ref for root)
    human_approved  INTEGER NOT NULL DEFAULT 0,
    source          TEXT NOT NULL DEFAULT 'interview',
    status          TEXT NOT NULL DEFAULT 'draft',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS epic_versions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    epic_id     TEXT NOT NULL REFERENCES epics(id),
    version     INTEGER NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    rationale   TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(epic_id, version)
);
```

**Relationship:** One epic has many features. Every feature MUST belong to an epic.

---

### 3. Feature Skill — Consumable Feature Writer

**File:** `skills/pm-feature/SKILL.md`

**Role:** Takes an epic and breaks it into specific, testable, implementable features.

**What is a feature?**
A feature is a single deliverable capability. Small enough to implement in one PR or sprint. Large enough to be meaningful to a user.

**Crawl mode:**
- Given an epic UUID, read its description
- Read relevant source material
- Propose features that decompose the epic
- Each feature: short_desc, detailed_desc, target_release, tags
- Insert all with `human_approved = 0`, linked to parent epic

**Interact mode (h/ai iterate):**
- Show the parent epic for context
- Ask: "What specific capabilities make up this epic?"
- Human describes, AI structures into features
- Each feature iterated individually until approved

**DB changes:**
- Add `epic_id` FK to `product_features` table (**NOT NULL** — every feature must belong to an epic)
- Add `base_version` to track which version of the parent epic this feature was derived from

**Iterator awareness:** When describing a feature that applies across a set (architectures, protocols, regions), reference or create an iterator instead of listing values inline.

---

### 4. Requirement Skill — Bite-Sized Derivations

**File:** `skills/pm-requirement/SKILL.md`

**Role:** Takes a feature and derives specific, atomic requirements. These are the building blocks — small enough that each one has a clear pass/fail.

**What is a requirement?**
A requirement is a single, testable statement about system behavior. It's smaller than a feature. Multiple requirements sum up to a feature being "done."

Examples (for feature "Render multi-tenant cluster topology diagrams"):
- REQ: "System renders Host Cluster containers with CP and worker node counts"
- REQ: "System switches to compact layout when cluster count exceeds 50"
- REQ: "System supports rendering for each CLUSTER_TYPES" (uses iterator)

**Crawl mode:**
- Given a feature UUID, read its description
- Derive requirements from the description and source material
- Each requirement: title, description, acceptance_criteria
- Insert with `human_approved = 0`

**Interact mode (h/ai iterate):**
- Show the parent feature for context
- Ask: "What must be true for this feature to be complete?"
- Human gives rough criteria, AI structures into formal requirements
- Iterate until approved

**DB table:** `requirements` (new)

```sql
CREATE TABLE IF NOT EXISTS requirements (
    id                  TEXT PRIMARY KEY,
    feature_id          TEXT NOT NULL REFERENCES product_features(id),
    title               TEXT NOT NULL,
    description         TEXT NOT NULL,
    acceptance_criteria TEXT,
    version             INTEGER NOT NULL DEFAULT 1,
    base_version        INTEGER NOT NULL DEFAULT 1,  -- parent feature version this was derived from
    human_approved      INTEGER NOT NULL DEFAULT 0,
    source              TEXT NOT NULL DEFAULT 'interview',
    status              TEXT NOT NULL DEFAULT 'draft',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS requirement_versions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    requirement_id      TEXT NOT NULL REFERENCES requirements(id),
    version             INTEGER NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL,
    acceptance_criteria TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(requirement_id, version)
);
```

**Relationship:** One feature has many requirements. Each requirement belongs to exactly one feature.

---

### 5. Test Requirement Skill — The Meticulous Tester

**File:** `skills/pm-test/SKILL.md`

**Role:** Writes human-readable test criteria. This person does not write code. Does not understand programming. Thinks in terms of what to verify, not how to verify it.

**Who is this person?**
A meticulous QA lead who has never written a line of code but knows the product inside and out:

> "Verify that the system renders exactly N Host Cluster containers where N matches the input data. For each CLUSTER_TYPES, confirm the container displays the cluster name and node count."

They said `CLUSTER_TYPES` — not "kubernetes, docker, nomad." They reference the iterator.

**Characteristics of this author:**
- **Succinct.** Every word earns its place. No filler.
- **Iterator-native.** References the iterator name, never the expanded list. Test criteria stay stable when the list changes.
- **Imperative mood.** "Verify that...", "Confirm that...", "Ensure that..."
- **Observable outcomes only.** What you can see/measure, not internal implementation. "The response arrives in under 100ms" not "the cache is hit."
- **No code, no pseudocode, no technical implementation.** "Send a request" not "curl -X POST."
- **Thinks in completeness.** For every test: "Does this cover all the cases?" — the answer is always an iterator.

**Crawl mode:**
- Given a requirement UUID, read it
- Write test title + test description using the rules above
- Reference existing iterators; propose new ones when repeated lists are detected
- Insert with `human_approved = 0`

**Interact mode (h/ai iterate):**
- Human can provide a test description at the **feature level** — AI derives requirement-level tests from it
- Show the parent requirement
- Ask: "How would you know this requirement is met?"
- AI structures into the meticulous tester's voice: succinct, iterator-aware, observable
- Iterate until approved

**Every requirement MUST have a test.** No exceptions.

**DB changes:**
- Add `requirement_id` FK to `product_feature_tests` (NOT NULL for requirement-level tests)
- Add `base_version` to track which version of the parent requirement this test was derived from

---

### 6. Iterator Skill — Reusable Named Lists

**File:** `skills/pm-iterator/SKILL.md`

**Role:** Manages named, reusable value lists that other skills reference instead of hardcoding.

**What is an iterator?**
An iterator is a named list of values — like a C macro or an enum:
- Name: `SOFTWARE_ARCHS`, `CLUSTER_TYPES`, `SUPPORTED_REGIONS`
- Values: `["x86", "amd64", "arm64", "mips"]`
- Scope: which product it belongs to
- Description: what this list represents and why these specific values

**Why iterators matter:**
1. **Single source of truth.** Change the list once, every reference updates.
2. **Explicit exclusions.** If `armv9` is NOT in `SOFTWARE_ARCHS`, that's a deliberate decision documented once.
3. **Test completeness.** "Test against each SOFTWARE_ARCHS" — the list is defined, not implied.
4. **Requirement precision.** "Support each SUPPORTED_PROTOCOLS" — if the list grows, the requirement automatically covers the new entry.

**Commands:**
```
/pm-iterator myriplay create SOFTWARE_ARCHS "Target build architectures" x86,amd64,arm64,mips
/pm-iterator myriplay list
/pm-iterator myriplay show SOFTWARE_ARCHS
/pm-iterator myriplay add SOFTWARE_ARCHS riscv64
/pm-iterator myriplay remove SOFTWARE_ARCHS mips
```

**DB tables:**

```sql
CREATE TABLE IF NOT EXISTS iterators (
    id          TEXT PRIMARY KEY,
    product_id  INTEGER NOT NULL REFERENCES our_products(id),
    name        TEXT NOT NULL,                 -- UPPER_SNAKE_CASE
    description TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_id, name)
);

CREATE TABLE IF NOT EXISTS iterator_values (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    iterator_id TEXT NOT NULL REFERENCES iterators(id),
    value       TEXT NOT NULL,
    position    INTEGER NOT NULL,
    UNIQUE(iterator_id, value)
);
```

**Display rules:** Iterator names are stored as-is in text fields (e.g., "test against each SOFTWARE_ARCHS"). They are NEVER auto-expanded inline — the reader sees the exact name. A glossary at the top of any output lists all referenced iterators with their current values.

---

### 7. Auditor Skill — Dependency Chain Integrity

**File:** `skills/pm-auditor/SKILL.md`

**Role:** Detects when upstream changes invalidate downstream entities. The cascade tracker.

**The problem:** Epic Av3 has features A1, A2, A3 derived from it. Requirements and tests are built on those features. Someone updates Epic A to v4. Now every downstream entity needs re-confirmation — but nobody knows what changed or what's affected.

**How it works — the `base_version` system:**

Every entity records which version of its parent it was derived from:
- Feature records `base_version` = the epic version it was written against
- Requirement records `base_version` = the feature version it was written against
- Test records `base_version` = the requirement version it was written against

When a parent's `version` > child's `base_version`, that child is **stale**.

**Example cascade:**
```
Epic A (v4)                          ← updated
  └─ Feature A1 (v1, base_version: 3) ← STALE (epic is v4, feature based on v3)
       └─ Req R1 (v2, base_version: 1)  ← OK (feature still v1)
            └─ Test T1 (v1, base_version: 2) ← OK (req still v2)
  └─ Feature A2 (v1, base_version: 4) ← OK (based on current epic)
```

**Commands:**
```
/pm-auditor myriplay                    # Full audit
/pm-auditor myriplay epic <uuid>        # Audit one epic's tree
/pm-auditor myriplay stale              # Show only stale entities
```

**Output:**
```
Dependency Audit: Myriplay
──────────────────────────────
Stale entities: 4

[STALE] Feature "Topology rendering" (v1, based on Epic v3 → Epic now v4)
  └─ [CASCADE] Req "Render HC containers" (v2) — parent is stale
       └─ [CASCADE] Test "Verify HC count" (v1) — grandparent is stale
  └─ [CASCADE] Req "Compact layout threshold" (v1) — parent is stale

Actions:
  1. Review Feature "Topology rendering" against Epic v4 changes
  2. After updating feature, re-confirm 2 requirements and 1 test
```

**Staleness propagates:** If a feature is stale, all its requirements and tests are implicitly stale too (cascade), even if their own `base_version` matches their direct parent. The auditor shows the full cascade.

**After human review in WebTool:** When someone updates a stale entity, they bump its version and set `base_version` to the current parent version. This clears the staleness.

---

### 8. Preflight Skill — Dependency Guardian

**File:** `skills/pm-preflight/SKILL.md`

**Role:** Runs before ANY other skill. Validates the environment is ready.

**Checks:**
1. `sqlite3` CLI available (or Python sqlite3 fallback)
2. `.claude/db/` directory exists
3. Database has all required tables (runs schema migrations if needed)
4. Schema version matches expected version

**Hard stop on failure.** If sqlite3 is missing, print install instructions and STOP. No fallbacks. No markdown workarounds. No alternative formats.

**Schema migration:** Preflight handles adding new tables to existing databases without data loss. It's the only skill that runs DDL.

---

### 9. Publish Skill — Markdown Generator

**File:** `skills/pm-publish/SKILL.md`

**Role:** Generates markdown documents from database contents. The ONLY thing that writes markdown feature docs.

**Commands:**
```
/pm-publish myriplay features docs/features.md
/pm-publish myriplay features docs/features.md security,api,performance
/pm-publish myriplay requirements docs/requirements.md
/pm-publish myriplay full docs/product-spec.md
```

**Output types:**
- **features** — tag-based sections, features with tests
- **requirements** — epic → feature → requirement → test hierarchy
- **full** — complete product spec: everything

**Publishes ALL items** regardless of `human_approved` status. Approval state is visible in WebTool, not in published markdown.

**Iterator handling:**
- Body text keeps iterator references as-is — never auto-expanded
- Glossary at the **top** of the document lists all referenced iterators with current values

**Rules:**
- Database is source of truth — never reads from existing markdown
- Overwrites target file completely on each publish

---

### 10. Status Skill — Coverage Dashboard

**File:** `skills/pm-status/SKILL.md`

**Role:** Reports on completeness and health of the product database.

**Commands:**
```
/pm-status myriplay
/pm-status myriplay coverage
/pm-status myriplay stale
```

**Dashboard output:**
```
Product: Myriplay
──────────────────────────────
Epics:            4 total (3 approved, 1 pending)
Features:        23 total (18 approved, 5 pending)
  Orphaned:       0 (all belong to an epic)
Requirements:    45 total (30 approved, 15 pending)
  Without tests:  8
Iterators:        6 defined, 4 referenced, 0 unreferenced
Tests:           37 total

Coverage:
  Epics → Features:         100% (4/4 have features)
  Features → Requirements:   78% (18/23 have requirements)
  Requirements → Tests:      82% (37/45 have tests)
  End-to-end:                64% (features with full req+test chain)

Staleness:
  Stale features:     2 (parent epic updated)
  Cascade affected:   5 reqs, 3 tests

Missing:
  [!] Feature "WebSocket streaming" has 0 requirements
  [!] Requirement "Compact layout threshold" has no test
```

---

### 11. WebTool — Human Review Web UI

**Implemented.** FastAPI + vanilla JS single-page app at `webtool/`. Reads/writes `.claude/db/marketing.sqlite`. Has a companion skill `skills/pm-webtool/SKILL.md`.

**Launch:** `/pm webtool` or directly:
```bash
python3 .claude/webtool/serve.py --project "$(pwd)"
# Opens http://localhost:8420
```

**Deployed by installer** to `.claude/webtool/` in each project. Never reference the claudecodetricks backup repo from skills — skills must be self-contained at the deploy target.

**Dependencies:** `pip3 install fastapi uvicorn`

**Core capabilities (built):**
- **Browse** — collapsible tree view: Epics → Features → Requirements → Tests
- **Direct edit** — click any entity name to open inline edit panel, version auto-bumps on save
- **Approve/Disapprove** — single click toggle per entity (green checkmark / red X)
- **Bulk approve** — approve an epic or feature and all descendants cascade
- **AI feedback** — text area + mic icon per entity, stored in `feedback` table
- **Staleness view** — yellow/orange left border on stale nodes, tooltip shows "Based on v3, now v4"
- **Iterator glossary** — modal with all iterators, add/remove values inline, iterator names highlighted in entity text with value tooltips
- **Voice input** — Web Speech API mic icon on feedback fields, click to record

**Tech stack (actual):**
- Backend: Python FastAPI, direct SQLite, 18 API endpoints (`webtool/serve.py`, 657 lines)
- Frontend: vanilla JS SPA, dark theme (`webtool/static/`, ~1500 lines total)
- Voice: Web Speech API (browser-native, no server-side STT)
- Port: 8420, auto-opens browser, no authentication

---

## Dependency Tracking System — `base_version`

Every entity in the hierarchy records which version of its parent it was derived from:

```
Entity          | Parent        | Tracks
----------------|---------------|---------------------------
Feature         | Epic          | epic_id + base_version
Requirement     | Feature       | feature_id + base_version
Test            | Requirement   | requirement_id + base_version
```

**Staleness rule:** An entity is stale when `parent.version > child.base_version`.

**Cascade rule:** If an entity is stale, all its descendants are implicitly stale too.

**Resolution is automatic.** The human never manually bumps versions or cascades. Any edit — whether direct in WebTool or via AI feedback in interact mode — auto-increments the entity's `version` and snapshots to the version history table. When saving, `base_version` is set to the current parent version, clearing staleness. The human just edits content; the system handles the bookkeeping.

**Schema columns added to all entity tables:**

```sql
version         INTEGER NOT NULL DEFAULT 1,    -- this entity's version
base_version    INTEGER NOT NULL DEFAULT 1,    -- parent's version when this was created/confirmed
```

---

## Database Schema Changes Summary

**New tables:**
1. `epics` — epic-level features (replaces meta_features concept)
2. `epic_versions` — version history for epics
3. `epic_tags` — tag associations for epics
4. `requirements` — derived from features
5. `requirement_versions` — version history for requirements
6. `iterators` — named value lists
7. `iterator_values` — values within iterators

**Modified tables:**
1. `product_features` — add `epic_id` (NOT NULL), `base_version`
2. `product_feature_tests` — add `requirement_id`, `base_version`

---

## Skill File Size Target

Each SKILL.md should be under **150 lines**. If it's longer, it's doing too much.

---

## Complete Skill Inventory

| # | Skill | File | Purpose |
|---|-------|------|---------|
| 1 | PM | `skills/pm/SKILL.md` | Orchestrator — parses intent, dispatches, tracks coverage |
| 2 | Epic | `skills/pm-epic/SKILL.md` | Big-picture capabilities |
| 3 | Feature | `skills/pm-feature/SKILL.md` | Breaks epics into deliverables |
| 4 | Requirement | `skills/pm-requirement/SKILL.md` | Atomic pass/fail criteria from features |
| 5 | Test | `skills/pm-test/SKILL.md` | Human-readable test criteria, iterator-native |
| 6 | Iterator | `skills/pm-iterator/SKILL.md` | Named reusable value lists |
| 7 | Auditor | `skills/pm-auditor/SKILL.md` | Dependency chain integrity, staleness detection |
| 8 | Preflight | `skills/pm-preflight/SKILL.md` | Environment validation, schema migrations |
| 9 | Publish | `skills/pm-publish/SKILL.md` | Markdown generation from DB |
| 10 | Status | `skills/pm-status/SKILL.md` | Coverage dashboard |
| 11 | WebTool | `skills/pm-webtool/SKILL.md` + `webtool/` | Web UI for human review (FastAPI + vanilla JS) |

---

## Decisions Log

| # | Question | Decision |
|---|----------|----------|
| 1 | Review workflow | WebTool handles review. PM coordinates but never sets human_approved. |
| 2 | Epic versioning | Full version history + base_version dependency tracking. |
| 3 | Requirement versioning | Same — full history + base_version. |
| 4 | Test-to-requirement linking | Every requirement MUST have a test. AI can derive from feature-level descriptions. |
| 5 | Iterator expansion | Never expand inline. Glossary at top. WebTool uses hover/alt text. |
| 6 | Draft publish mode | No draft mode. Publish publishes everything. WebTool manages approval state. |
| 7 | API key check | Not needed — we're inside Claude Code. |
| 8 | Standalone features | Not allowed. Every feature must belong to an epic. |
| 9 | Naming | meta-feature → **epic**. More accepted industry term. |
| 10 | Voice input | Mode 3 for CLI (via VoiceMode), Web Speech API in WebTool. Speak instead of type anywhere. |
| 11 | Product auto-detect | Product code is optional when only one product exists in the DB. No need to type "myriplay" every time. |
| 12 | WebTool invocation | `/pm webtool` launches the web review UI. Recognized as a mode keyword by PM orchestrator. |
