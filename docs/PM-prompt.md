# PM — Product Manager Skill System

> Design document for the feature-tracker rewrite.
> Travis: add comments inline with `<!-- TRAVIS: your comment -->` and I'll iterate.

---

## Why Rewrite

The current feature-tracker is one monolithic SKILL.md (~440 lines). Problems:

1. **Claude loses context** mid-execution on long workflows (scan + populate + publish)
2. **Scan workflows skip tests and requirements** — only features get populated
3. **No requirements concept** exists at all
4. **No iterator/macro system** — repeated lists get copy-pasted and drift
5. **No orchestration** — the skill tries to do everything itself
6. **sqlite3 dependency** not enforced — Claude falls back to writing markdown directly

---

## Architecture: PM + Focused Skills

```
                         /pm <product> <mode> <scope>
                                    |
                              +-----+-----+
                              |    PM     |  (orchestrator)
                              +-----+-----+
                                    |
                 +------------------+------------------+
                 |          |          |         |      |
           meta-feature  feature  requirement  test  iterator
              skill       skill     skill     skill   skill
                 |          |          |         |      |
                 +----------+----------+---------+------+
                                    |
                              +-----+-----+
                              |  shared   |
                              |  database |
                              +-----------+
                                    |
                          +---------+---------+
                          |         |         |
                       preflight  publish   status
```

Each box is a separate SKILL.md — small, focused, and independently testable.

---

## Modes

### Mode 1: Crawl

PM is pointed at code, documents, or websites. AI does the heavy lifting.

```
/pm myriplay crawl ./src/
/pm myriplay crawl ./docs/architecture.md
/pm myriplay crawl https://docs.example.com/api
```

PM dispatches work:
1. Calls **meta-feature** skill to identify epics from source material
2. Calls **feature** skill to break epics into features
3. Calls **requirement** skill to derive requirements from features
4. Calls **test** skill to write test criteria for each requirement
5. Calls **iterator** skill whenever repeated value lists are detected

All results land in DB with `human_approved = 0`. Nothing is "real" until reviewed.

### Mode 2: Interact

PM interviews a human. Human leads the conversation.

```
/pm myriplay interact
/pm myriplay interact meta-features
/pm myriplay interact requirements
```

PM dispatches to the appropriate skill which runs its own h/ai or ai/h iteration loop. PM tracks what's been covered and what's missing.

---

## Skill Inventory

### 1. PM (Product Manager) — Orchestrator

**File:** `skills/pm/SKILL.md`

**Role:** Coordinator. Never writes to the DB directly. Delegates to sub-skills and tracks completion.

**Responsibilities:**
- Parse user intent (crawl vs interact, scope)
- Dispatch to sub-skills in correct order (meta-features before features before requirements before tests)
- Track coverage: which meta-features have features? which features have requirements? which requirements have tests?
- Surface gaps: "3 features have no requirements, 7 requirements have no test criteria"
- Handle preflight before any work begins

**Does NOT do:**
- Write features, requirements, or tests
- Make content decisions
- Talk to the database directly (except for coverage queries)

<!-- TRAVIS: Should PM also handle review/approval workflows? Currently review is part of the old feature-tracker. PM could orchestrate "review all unapproved items" as a batch workflow. -->

---

### 2. Meta-Feature Skill — Epic-Level Thinking

**File:** `skills/pm-meta-feature/SKILL.md`

**Role:** Big-picture product capability researcher and writer.

**What is a meta-feature?**
A meta-feature is an epic — a large product capability that decomposes into multiple features. Examples:
- "Multi-tenant cluster visualization" (decomposes into: topology rendering, detail levels, interactive controls, export)
- "Device lifecycle management" (decomposes into: provisioning, monitoring, decommissioning, certificate rotation)

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

**DB table:** `meta_features` (new)
- id (UUID), product_id, name, description, rationale, tags, version, human_approved, source, status

**Relationship:** One meta-feature has many features.

<!-- TRAVIS: Do meta-features need their own versioning like features do? Or is a simple version counter enough? -->

---

### 3. Feature Skill — Consumable Feature Writer

**File:** `skills/pm-feature/SKILL.md`

**Role:** Takes a meta-feature and breaks it into specific, testable, implementable features.

**What is a feature?**
A feature is a single deliverable capability. Small enough to implement in one PR or sprint. Large enough to be meaningful to a user.

**Crawl mode:**
- Given a meta-feature UUID, read its description
- Read relevant source material
- Propose features that decompose the meta-feature
- Each feature: short_desc, detailed_desc, target_release, tags
- Insert all with `human_approved = 0`, linked to parent meta-feature

**Interact mode (h/ai iterate):**
- Show the parent meta-feature for context
- Ask: "What specific capabilities make up this epic?"
- Human describes, AI structures into features
- Each feature iterated individually until approved

**DB changes:**
- Add `meta_feature_id` FK to `product_features` table (nullable — standalone features still allowed)
- Existing schema otherwise unchanged

**Iterator awareness:** When describing a feature that applies across a set (architectures, protocols, regions), reference or create an iterator instead of listing values inline.

<!-- TRAVIS: Should features that don't belong to any meta-feature be allowed? The current system has standalone features. I'd say yes — keep it flexible. -->

---

### 4. Requirement Skill — Bite-Sized Derivations

**File:** `skills/pm-requirement/SKILL.md`

**Role:** Takes a feature and derives specific, atomic requirements. These are the building blocks — small enough that each one has a clear pass/fail.

**What is a requirement?**
A requirement is a single, testable statement about system behavior. It's smaller than a feature. Multiple requirements sum up to a feature being "done."

Examples (for feature "Render multi-tenant cluster topology diagrams"):
- REQ: "System renders Host Cluster containers with CP and worker node counts"
- REQ: "System switches to compact layout when cluster count exceeds 50"
- REQ: "System renders node labels using the cluster's display name, not internal ID"
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
    id                TEXT PRIMARY KEY,           -- UUID
    feature_id        TEXT NOT NULL REFERENCES product_features(id),
    title             TEXT NOT NULL,              -- short imperative statement
    description       TEXT NOT NULL,              -- detailed requirement
    acceptance_criteria TEXT,                     -- when is this "done"?
    version           INTEGER NOT NULL DEFAULT 1,
    human_approved    INTEGER NOT NULL DEFAULT 0,
    source            TEXT NOT NULL DEFAULT 'interview',
    status            TEXT NOT NULL DEFAULT 'draft',
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Relationship:** One feature has many requirements. Each requirement belongs to exactly one feature.

<!-- TRAVIS: Should requirements have their own version history table like features do? I'd say yes for consistency. -->

---

### 5. Test Requirement Skill — The Meticulous Tester

**File:** `skills/pm-test/SKILL.md`

**Role:** Writes human-readable test criteria. This person does not write code. Does not understand programming. Thinks in terms of what to verify, not how to verify it.

**Who is this person?**
Think of a meticulous QA lead who has never written a line of code but knows the product inside and out. They write test criteria like:

> "Verify that the system renders exactly N Host Cluster containers where N matches the input data. For each CLUSTER_TYPES, confirm the container displays the cluster name and node count."

Note: they said `CLUSTER_TYPES` — not "kubernetes, docker, nomad." They reference the iterator. They trust the iterator is defined elsewhere and complete.

**Characteristics of this author:**
- **Succinct.** Every word earns its place. No filler.
- **Iterator-native.** Whenever a test applies across a set, they reference the iterator name, never the expanded list. This keeps test criteria stable when the list changes.
- **Imperative mood.** "Verify that...", "Confirm that...", "Ensure that..."
- **Observable outcomes only.** They describe what you can see/measure, not internal implementation. "The response arrives in under 100ms" not "the cache is hit."
- **No code, no pseudocode, no technical implementation.** "Send a request" not "curl -X POST."
- **Thinks in completeness.** For every test, they ask: "Does this cover all the cases?" and the answer is always an iterator.

**Crawl mode:**
- Given a requirement UUID, read it
- Write test title + test description using the rules above
- Reference existing iterators; propose new ones when repeated lists are detected
- Insert with `human_approved = 0`

**Interact mode (h/ai iterate):**
- Show the parent requirement
- Ask: "How would you know this requirement is met? What would you check?"
- Human describes in rough terms
- AI structures into the meticulous tester's voice: succinct, iterator-aware, observable
- Iterate until approved

**DB:** Uses existing `product_feature_tests` table, but now linked to requirements too:

```sql
-- Add requirement_id to existing tests table
ALTER TABLE product_feature_tests ADD COLUMN requirement_id TEXT REFERENCES requirements(id);
```

A test can be linked to a feature (high-level) or a requirement (granular), or both.

<!-- TRAVIS: Should every requirement MUST have a test? Or is it acceptable for some requirements to be "tested by" a parent feature-level test? My instinct: every requirement gets its own test, but I want your call. -->

---

### 6. Iterator Skill — Reusable Named Lists

**File:** `skills/pm-iterator/SKILL.md`

**Role:** Manages named, reusable value lists that other skills reference instead of hardcoding.

**What is an iterator?**
An iterator is a named list of values — like a C macro or an enum. It has:
- A name: `SOFTWARE_ARCHS`, `CLUSTER_TYPES`, `SUPPORTED_REGIONS`
- Values: `["x86", "amd64", "arm64", "mips"]`
- A scope: which product it belongs to
- A description: what this list represents and why these specific values

**Why iterators matter:**
1. **Single source of truth.** Change the list once, every reference updates.
2. **Explicit exclusions.** If `armv9` is NOT in `SOFTWARE_ARCHS`, that's a deliberate decision documented once.
3. **Test completeness.** The test author writes "test against each SOFTWARE_ARCHS" — the list is defined, not implied.
4. **Requirement precision.** "Support each SUPPORTED_PROTOCOLS" — if the list grows, the requirement automatically covers the new entry.

**Commands:**
```
/pm-iterator myriplay create SOFTWARE_ARCHS "Target build architectures" x86,amd64,arm64,mips
/pm-iterator myriplay list
/pm-iterator myriplay show SOFTWARE_ARCHS
/pm-iterator myriplay add SOFTWARE_ARCHS riscv64
/pm-iterator myriplay remove SOFTWARE_ARCHS mips
/pm-iterator myriplay rename SOFTWARE_ARCHS BUILD_TARGETS
```

**DB table:** `iterators` (new)

```sql
CREATE TABLE IF NOT EXISTS iterators (
    id          TEXT PRIMARY KEY,              -- UUID
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
    position    INTEGER NOT NULL,              -- ordering
    UNIQUE(iterator_id, value)
);
```

**Resolution:** When publishing or displaying, `SOFTWARE_ARCHS` resolves to its current values. In the DB, text fields store the iterator name as-is (e.g., "test against each SOFTWARE_ARCHS"). The publish skill expands them if needed.

<!-- TRAVIS: Should iterators expand in published markdown, or stay as names with a glossary at the top? I'm thinking: keep names in the body, add an "Iterators" appendix that lists all expansions. Your call. -->

---

### 7. Preflight Skill — Dependency Guardian

**File:** `skills/pm-preflight/SKILL.md`

**Role:** Runs before ANY other skill. Validates the environment is ready.

**Checks:**
1. `sqlite3` CLI available (or Python sqlite3 fallback)
2. `.claude/db/` directory exists
3. Database has all required tables (runs schema migrations if needed)
4. Schema version matches expected version
5. Required iterators exist for the product (if any are referenced)

**Hard stop on failure.** If sqlite3 is missing, print install instructions and STOP. No fallbacks. No markdown workarounds.

**Schema migration:** Preflight also handles adding new tables (iterators, requirements, meta_features) to existing databases without data loss. It's the only skill that runs DDL.

<!-- TRAVIS: Should preflight also check for ANTHROPIC_API_KEY if crawl mode will need Claude API? Or keep that in the PM skill? -->

---

### 8. Publish Skill — Markdown Generator

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
- **features** — current publish format (tag sections, features with tests)
- **requirements** — feature > requirement > test hierarchy
- **full** — complete product spec: meta-features > features > requirements > tests

**Iterator handling in output:**
- Body text keeps iterator references as-is: "test against each SOFTWARE_ARCHS"
- Appendix at bottom expands all referenced iterators:

```markdown
---

## Iterators

| Name | Description | Values |
|------|-------------|--------|
| SOFTWARE_ARCHS | Target build architectures | x86, amd64, arm64, mips |
| CLUSTER_TYPES | Supported cluster types | kubernetes, docker, nomad |
```

**Rules:**
- Only publishes `human_approved = 1` items
- Database is source of truth — never reads from existing markdown
- Overwrites target file completely on each publish

<!-- TRAVIS: Should publish support a "draft" mode that includes unapproved items marked with a visual indicator? Could be useful for review cycles. -->

---

### 9. Status Skill — Coverage Dashboard

**File:** `skills/pm-status/SKILL.md`

**Role:** Reports on completeness and health of the product database.

**Command:**
```
/pm-status myriplay
/pm-status myriplay meta-features
/pm-status myriplay coverage
```

**Dashboard output:**
```
Product: Myriplay
──────────────────────────────
Meta-Features:    4 total (3 approved, 1 pending)
Features:        23 total (18 approved, 5 pending)
  Without meta-feature: 2 (orphaned)
Requirements:    45 total (30 approved, 15 pending)
  Without tests: 8
Iterators:        6 defined, 4 referenced
Tests:           37 total

Coverage:
  Features → Requirements:  78% (18/23 have requirements)
  Requirements → Tests:     82% (37/45 have tests)
  End-to-end:               64% (features with full req+test chain)

Missing:
  [!] Feature "WebSocket streaming" has 0 requirements
  [!] Requirement "Compact layout threshold" has no test
  [!] Iterator SUPPORTED_PROTOCOLS defined but never referenced
```

---

## Database Schema Changes Summary

New tables needed:
1. `meta_features` — epic-level features
2. `meta_feature_versions` — version history for meta-features
3. `requirements` — derived from features
4. `requirement_versions` — version history for requirements
5. `iterators` — named value lists
6. `iterator_values` — values within iterators

Modified tables:
1. `product_features` — add `meta_feature_id` FK (nullable)
2. `product_feature_tests` — add `requirement_id` FK (nullable)

---

## Skill File Size Target

Each SKILL.md should be under **150 lines**. If it's longer, it's doing too much. The old feature-tracker was 440 lines — that's exactly what we're avoiding.

---

## Open Questions

<!-- Mark these with your decision and I'll incorporate -->

1. **Review workflow** — Should PM orchestrate batch review of unapproved items? Or separate skill?
2. **Meta-feature versioning** — Full version history table, or simple version counter?
3. **Requirement versioning** — Same question.
4. **Test-to-requirement linking** — Must every requirement have a test? Or optional?
5. **Iterator expansion** — Expand in body text, or keep as names with appendix?
6. **Draft publish mode** — Include unapproved items with visual markers?
7. **API key check** — Preflight or PM responsibility?
8. **Standalone features** — Allow features without a parent meta-feature?
