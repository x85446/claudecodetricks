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
/feature-tracker <product> edit <uuid>                # Edit a feature (new version created)
/feature-tracker <product> list                       # List all features
/feature-tracker <product> list tag=X AND tag=Y       # Query by tags (AND/OR)
/feature-tracker <product> list release=2.0           # Filter by target release
/feature-tracker <product> show <uuid>                # Full detail + test + version history
/feature-tracker <product> test <uuid>                # Add/edit human-described test
/feature-tracker <product> tag <uuid> <tags>          # Add tags to a feature
/feature-tracker <product> release <version>          # Show release summary
/feature-tracker <product> status                     # Dashboard
/feature-tracker <product> init                       # Initialize product
```

## Database

**Path:** `.claude/db/marketing.sqlite` (shared with competitive-intel)

Schema is in [schema.sql](schema.sql). Queries are in [queries.sql](queries.sql). Reuses `our_products` and `tags` tables from competitive-intel.

Always run `PRAGMA foreign_keys=ON;` before writes.

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
3. INSERT into `product_features` (version = 1)
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
UUID (short)  | Short Description              | Release | Status      | Tags
a1b2c3d4      | Sub-ms edge inference          | 1.0     | implemented | performance, edge, latency
e5f6a7b8      | Mutual TLS for device auth     | 1.0     | approved    | security, api, tls
```

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
```

### Init

```
/feature-tracker izuma-edge init
```

1. Run [schema.sql](schema.sql) against `.claude/db/marketing.sqlite`
2. Ensure product exists in `our_products` (reuse if already there from competitive-intel)
3. Confirm ready

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
