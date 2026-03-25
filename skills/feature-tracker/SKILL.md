---
name: feature-tracker
description: Maintain product feature specifications with versioning, tagging, and human-described tests. Use when adding features, listing features, querying by tags, tracking feature versions, writing feature tests, or managing product roadmaps.
argument-hint: [product action | product list tag=X AND tag=Y | product add]
---

# Feature Tracker

Maintain detailed product feature specifications in SQLite. Features are authored collaboratively through iteration loops, versioned on every edit, tagged for flexible querying, and paired with human-described tests.

## Invocation

```
/feature-tracker <product> add                        # Add a new feature (full interview)
/feature-tracker <product> scan-docs <path>           # Discover features from documentation
/feature-tracker <product> scan-code <path>           # Discover features from a codebase
/feature-tracker <product> review                     # Review AI-discovered features (approve/reject)
/feature-tracker <product> edit <uuid>                # Edit a feature (new version created)
/feature-tracker <product> list                       # List all features
/feature-tracker <product> list tag=X AND tag=Y       # Query by tags (AND/OR)
/feature-tracker <product> list release=2.0           # Filter by target release
/feature-tracker <product> list unapproved            # Show only AI-discovered, not yet approved
/feature-tracker <product> show <uuid>                # Full detail + test + version history
/feature-tracker <product> test <uuid>                # Add/edit human-described test
/feature-tracker <product> tag <uuid> <tags>          # Add tags to a feature
/feature-tracker <product> publish                    # Generate products/<product>-features.md
/feature-tracker <product> publish <tag-order>        # Custom section ordering
/feature-tracker <product> release <version>          # Show release summary
/feature-tracker <product> status                     # Dashboard
/feature-tracker <product> init                       # Initialize product
```

## Preflight Check (REQUIRED — run before ANY action)

Before doing anything else, run this check. **If it fails, STOP. Do not fall back to markdown files, flat files, or any workaround. Fix the dependency first.**

```bash
# 1. Check sqlite3 is available
which sqlite3 || python3 -c "import sqlite3; print('python3-sqlite3')"

# 2. Check DB directory exists
ls .claude/db/ 2>/dev/null || mkdir -p .claude/db

# 3. Check DB is initialized (has tables)
sqlite3 .claude/db/marketing.sqlite ".tables" 2>/dev/null | grep -q product_features
```

**If sqlite3 is not installed:**
```
STOP: sqlite3 is not available on this system.

Install it:
  macOS:    brew install sqlite3
  Ubuntu:   sudo apt-get install sqlite3
  Alpine:   apk add sqlite
  Python:   sqlite3 module is built-in — use python3 instead of sqlite3 CLI

Cannot proceed without SQLite. This skill MUST NOT write directly to markdown
or any other format as a workaround. The database is the source of truth.
```

Present this message to the user and stop. Do not attempt alternative approaches.

**If DB exists but tables are missing**, run [schema.sql](schema.sql) and the competitive-intel schema first.

## Database

**Path:** `.claude/db/marketing.sqlite` (shared with competitive-intel)

Schema is in [schema.sql](schema.sql). Queries are in [queries.sql](queries.sql). Reuses `our_products` and `tags` tables from competitive-intel.

Always run `PRAGMA foreign_keys=ON;` before writes.

**NEVER write feature data directly to markdown, JSON, or any other file.** The database is the only place feature data is stored. Markdown is only generated via the `publish` command from database contents.

## Iteration Patterns

Two collaboration modes used throughout:

**h/ai iterate** — Human leads. Human provides text → AI enhances and presents → human accepts or gives feedback → AI revises → repeat until accepted. Keep human input short — ask simple questions, don't require essays.

**ai/h iterate** — AI leads. AI proposes → human accepts or gives feedback → AI revises → repeat. AI does the heavy lifting, human just approves or nudges.

Always use `AskUserQuestion` for each iteration step. Keep questions short. Accept `y`, `yes`, `ok`, `good` as approval. Any other response is feedback to iterate on.

## Workflows

### Add a Feature

```
/feature-tracker izuma-edge add
```

Run these steps in order. Each step is one `AskUserQuestion` round.

**Step 1: Detailed Description (h/ai iterate)**

Interview the user to build the description. Ask short, focused questions:

```
What does this feature do? (one sentence is fine)
```

Then based on their answer, ask follow-up questions to fill gaps:
- Who uses it? (end user, admin, API consumer)
- What problem does it solve?
- Any constraints or requirements?

After gathering enough, write a polished multi-paragraph description and present:

```
Here's the detailed description I wrote from your input:

[paragraphs]

> accept | <your feedback>
```

Iterate until accepted.

**Step 2: Short Description (ai/h iterate)**

Propose a ≤10 word summary based on the detailed description:

```
Short description: "Real-time edge inference with sub-millisecond latency"

> accept | <your version>
```

**Step 3: Tags (ai/h iterate)**

Suggest tags based on the description content:

```
Suggested tags: performance, inference, edge, real-time, latency

> accept | <add/remove tags>
```

Tags are comma-separated. Any string is valid — no registration needed (auto-created in `tags` table).

**Step 4: Target Release (ai/h iterate)**

```
Target release? (e.g., 1.0, 2.0, backlog)
```

**Step 5: Human-Described Test**

**5a: Test Description (h/ai iterate)**

```
Describe how you'd test this feature. No code — just what must be true.
(e.g., "When I send a request from Tokyo, response arrives in under 1ms")
```

Enhance their description into a clear, structured test spec. Present and iterate.

**5b: Test Title (ai/h iterate)**

Propose a short title for the test:

```
Test title: "Verify sub-millisecond P99 latency from edge nodes"

> accept | <your version>
```

**Step 6: Commit to Database**

1. Generate UUID: `SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6)))`
2. Ensure all tags exist in `tags` table (INSERT OR IGNORE)
3. INSERT into `product_features` (version = 1, human_approved = 1, source = 'interview')
4. INSERT into `product_feature_versions` (snapshot version 1)
5. INSERT into `product_feature_tags`
6. Generate test UUID, INSERT into `product_feature_tests`
7. Show the completed feature with UUID:

```
✔ Feature created: a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d
  "Real-time edge inference with sub-ms latency"
  Tags: performance, inference, edge, real-time, latency
  Release: 1.0 | Version: 1 | Test: ✔
```

### Edit a Feature

```
/feature-tracker izuma-edge edit a1b2c3d4-...
```

1. Show current feature detail
2. Ask what to change (description, short_desc, tags, release, test)
3. Run the appropriate iteration loop for the changed fields
4. **Auto-increment version** on `product_features`
5. INSERT new snapshot into `product_feature_versions`
6. If test changed, increment test version too

### Mark as Implemented

```
/feature-tracker izuma-edge implement a1b2c3d4-... 1.0
```

1. UPDATE `product_features SET status = 'implemented'`
2. UPDATE `product_feature_versions SET implemented_in = '1.0' WHERE feature_id = ? AND version = ?`

### Tag Query

```
/feature-tracker izuma-edge list tag=security AND tag=api
/feature-tracker izuma-edge list tag=security OR tag=encryption
```

Parse the tag expression:
- **AND**: features must have ALL specified tags (use HAVING COUNT = tag_count)
- **OR**: features must have ANY specified tag (use IN)
- **Mixed**: parse left-to-right, AND binds tighter than OR

See [queries.sql](queries.sql) for the query templates.

Display results as a table:

```
UUID (short)  | Short Description              | Release | Status      | ✔ | Tags
a1b2c3d4      | Sub-ms edge inference          | 1.0     | implemented | ✔ | performance, edge, latency
e5f6a7b8      | Mutual TLS for device auth     | 1.0     | approved    | ✔ | security, api, tls
c9d0e1f2      | Batch device provisioning      | 2.0     | draft       | ⬡ | provisioning, batch, api
```

The `✔` column shows human approval status. `✔` = approved, `⬡` = AI-discovered, not yet reviewed.

### Status Dashboard

```
/feature-tracker izuma-edge status
```

```
=== Feature Tracker: izuma-edge ===

Features: 47 total
  Draft: 12 | Approved: 18 | Implemented: 15 | Deprecated: 2

By Release:
  1.0:  22 features (20 implemented, 2 approved)
  2.0:  18 features (0 implemented, 15 approved, 3 draft)
  backlog: 7 features (all draft)

Top Tags: security(14), api(12), performance(9), edge(8), ux(6)

Missing Tests: 5 features have no test defined

Sources: 31 interview, 10 docs, 6 code
```

### Init

```
/feature-tracker izuma-edge init
```

1. Run [schema.sql](schema.sql) against `.claude/db/marketing.sqlite`
2. Ensure product exists in `our_products` (reuse if already there from competitive-intel)
3. Confirm ready

### Scan Docs

```
/feature-tracker izuma-edge scan-docs ./docs/
```

AI autonomously reads documentation and extracts features. **All features added this way are `human_approved = 0`.**

1. Use the Agent tool to launch a research agent:

```
Agent(subagent_type="Explore", prompt="Read all documentation in <path>.
For each distinct product feature or capability described, extract:
  1. A short description (≤10 words)
  2. A detailed description (1-3 paragraphs, factual, from the docs)
  3. Suggested tags (comma-separated)
Return as a structured list. Do NOT include internal implementation details —
only user-facing features and capabilities.")
```

2. For each discovered feature, check for duplicates:
   - Query existing features by short_desc similarity and tags overlap
   - Skip if a close match exists (flag as "already tracked")
3. INSERT each new feature with `human_approved = 0`, `source = 'docs:<filepath>'`
4. Report summary:

```
Scanned: 23 documents
Discovered: 14 features (8 new, 6 already tracked)
All 8 new features are UNAPPROVED — run /feature-tracker izuma-edge review
```

### Scan Code

```
/feature-tracker izuma-edge scan-code ~/workspace/izuma/edge-runtime/
```

AI reads source code to discover implemented features. **All features added this way are `human_approved = 0`.**

1. Use the Agent tool to launch a research agent:

```
Agent(subagent_type="Explore", prompt="Explore the codebase at <path>.
Identify user-facing features and capabilities by examining:
  - API endpoints, route handlers, CLI commands
  - Public interfaces, exported functions
  - Feature flags, configuration options
  - README, inline documentation, comments describing behavior
For each feature found, extract:
  1. A short description (≤10 words)
  2. A detailed description of what it does (from the code's perspective)
  3. Suggested tags
  4. Source file(s) where it's implemented
Do NOT list internal utilities, helpers, or implementation details.")
```

2. Deduplicate against existing features
3. INSERT with `human_approved = 0`, `source = 'code:<filepath>'`
4. Report summary, prompt to review

### Review (Approve/Reject AI-Discovered Features)

```
/feature-tracker izuma-edge review
```

Present unapproved features one at a time using AskUserQuestion (Interview Pattern):

```
[1/8] UNAPPROVED — discovered from docs/api-reference.md

  "WebSocket streaming for real-time events"

  Detailed: The platform supports WebSocket connections for streaming
  real-time device events to client applications. Clients can subscribe
  to specific device topics and receive push notifications...

  Tags: api, websocket, real-time, streaming, events

  > approve | edit | reject | skip | stop
```

| Response | Action |
|---|---|
| `approve` / `y` | SET `human_approved = 1` |
| `edit` | Enter edit flow (h/ai iterate on description, tags, etc.), then approve |
| `reject` | DELETE the feature from the database |
| `skip` | Leave as unapproved, move to next |
| `stop` | Stop reviewing, remaining stay unapproved |

After review, show summary:

```
Reviewed: 8 features
  Approved: 5 | Edited+Approved: 2 | Rejected: 1
```

### Publish

```
/feature-tracker izuma-edge publish
/feature-tracker izuma-edge publish security,api,performance,ux
```

Generate `products/<product>-features.md` — organized by tags as sections. **Only `human_approved = 1` features are published.**

**Section ordering:** If tags provided as argument, use that order. Otherwise use stored order from `publish_tag_order` table (see [schema.sql](schema.sql)). Fallback: order by feature count descending. Each tag = one `##` section. A feature appears once, in its first matching section. Unmatched features go in "Other" at the end.

**Output format:**

```markdown
# Izuma Edge — Features

> Auto-generated by feature-tracker. Do not edit directly.
> 47 features | Last published: 2026-03-23

---

## Security

### Mutual TLS for device authentication
*Release: 1.0 | Version: 3 | Status: implemented*

The platform enforces mutual TLS (mTLS) for all device-to-cloud
communication. Each device presents a client certificate signed by
the organization's certificate authority during the TLS handshake...

**Test:** Verify mTLS handshake rejects unsigned device certificates
> Connect a device without a valid client certificate. The connection
> must be rejected at the TLS layer with a certificate_required alert.
> Verify the rejection is logged with the device ID and timestamp.

`Tags: security, tls, authentication, devices`

---

### End-to-end payload encryption
*Release: 1.0 | Version: 1 | Status: implemented*

All device payloads are encrypted end-to-end using AES-256-GCM...

**Test:** Verify payload encryption round-trip
> Send a payload from device to cloud. Intercept at the network layer
> and confirm the payload is not readable.

`Tags: security, encryption, devices`

---

## API

### WebSocket streaming for real-time events
*Release: 2.0 | Version: 1 | Status: approved*

...
```

**Format rules:**
- `# Product Name — Features` title
- `## Tag` section per tag in order
- `### Feature short description` per feature
- `*Release: X | Version: N | Status: STATUS*` metadata
- Detailed description as body paragraphs
- `**Test:** title` then test description as blockquote (`>`)
- `` `Tags: ...` `` at the bottom of each feature
- `---` between features
- Features within a section: sorted by release ASC, then short_desc ASC
- Each feature appears **once** — in its first matching section

**Stored ordering** — save tag order for reuse:

```sql
-- In schema.sql
CREATE TABLE IF NOT EXISTS publish_tag_order (
    product_id  INTEGER NOT NULL REFERENCES our_products(id),
    tag_id      INTEGER NOT NULL REFERENCES tags(id),
    position    INTEGER NOT NULL,
    PRIMARY KEY (product_id, tag_id)
);
```

When tags are provided as argument, save them to `publish_tag_order` for next time.

## Versioning Rules

1. **Every edit creates a new version.** The `product_features.version` field increments.
2. **Every version is snapshotted** in `product_feature_versions` with full text.
3. **`implemented_in`** records which release a specific version shipped in.
4. A feature can be implemented in 1.0 (version 1), then enhanced (version 2) targeting 2.0.
5. **Version history is append-only.** Never delete or modify old version snapshots.
6. **UUID is permanent.** A feature's UUID never changes across versions.

## Rules

1. **SQLite at `.claude/db/marketing.sqlite`** — shared with competitive-intel.
2. **Always `PRAGMA foreign_keys=ON`** before writes.
3. **UUID for every feature and test** — stable cross-database references.
4. **Version on every edit** — no silent overwrites.
5. **Tags over categories** — flat, flexible, queryable with AND/OR.
6. **h/ai iterate for human-led content** — keep user input short, AI enhances.
7. **ai/h iterate for AI-led content** — AI proposes, user approves.
8. **Accept `y`/`yes`/`ok`/`good`** as approval in iteration loops.
9. **Always show UUID** after create/edit so user can reference it.
10. **Tests are required** — prompt during add, flag missing in status dashboard.
11. **human_approved** — `add` (interview) → `1`. `scan-docs`/`scan-code` → `0`. Only `review` or `edit` flips 0 → 1.
12. **source tracking** — every feature records origin: `interview`, `docs:<path>`, or `code:<path>`.
13. **NEVER bypass SQLite** — if sqlite3 is unavailable, STOP and tell the user to install it. Do not write to markdown, JSON, CSV, or any alternative. No workarounds. No fallbacks. The database is the only data store.
