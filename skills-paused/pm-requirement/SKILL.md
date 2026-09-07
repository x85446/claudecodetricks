---
name: pm-requirement
description: Derive atomic, testable requirements from features. Each requirement is a single pass/fail statement. Every requirement must belong to a feature.
argument-hint: [product crawl <feature-uuid>|add <feature-uuid>|list|show <uuid>|edit <uuid>]
---

# PM Requirement — Bite-Sized Derivations

Derive specific, atomic requirements from features. Each requirement is a single testable statement — small enough for a clear pass/fail.

## Invocation

```
/pm-requirement myriplay crawl <feature-uuid>
/pm-requirement myriplay add <feature-uuid>
/pm-requirement myriplay list
/pm-requirement myriplay list feature=<uuid>
/pm-requirement myriplay show <uuid>
/pm-requirement myriplay edit <uuid>
```

## Database

**Path:** `.claude/db/marketing.sqlite` | `PRAGMA foreign_keys=ON;`

Uses `requirements` and `requirement_versions` tables.

## What is a Requirement?

A single, testable statement about system behavior. Smaller than a feature. Multiple requirements sum up to a feature being "done."

Examples:
- "System renders Host Cluster containers with CP and worker node counts"
- "System switches to compact layout when cluster count exceeds 50"
- "System supports rendering for each CLUSTER_TYPES" (uses iterator)

## Crawl Mode

1. Load parent feature by UUID — read its short_desc, detailed_desc, tags
2. Read relevant source material if available
3. Derive requirements — each one: title (imperative statement), description, acceptance_criteria
4. When a requirement applies across a set, reference iterators by name
5. INSERT each into `requirements` with `base_version` = parent feature's current `version`, `human_approved = 0`
6. INSERT version 1 into `requirement_versions`
7. Show summary with UUIDs

## Interact Mode (h/ai iterate)

1. Show parent feature for context
2. Ask: "What must be true for this feature to be complete?"
3. Human gives rough criteria — AI structures into formal requirements
4. Iterate each: title, description, acceptance_criteria
5. Accept: y/yes/ok/good
6. INSERT with `human_approved = 1`, `base_version` = parent feature's `version`
7. Show UUID: `Created requirement: <uuid> "title"`

## Edit

1. Show current requirement with parent feature context
2. Ask what to change
3. Run h/ai iterate on changed fields
4. Auto-increment `version` on `requirements`
5. Set `base_version` = current parent feature `version` (clears staleness)
6. INSERT new snapshot into `requirement_versions`

## Queries

```sql
-- List requirements for a feature
SELECT r.id, r.title, r.version, r.base_version, r.status, r.human_approved
FROM requirements r WHERE r.feature_id = :feature_uuid
ORDER BY r.title;

-- Requirements without tests
SELECT r.id, r.title FROM requirements r
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE r.feature_id = :feature_uuid AND t.id IS NULL;
```

## Rules

1. Every requirement belongs to exactly one feature.
2. `base_version` = parent feature's `version` at creation/confirmation.
3. Every edit auto-increments version and snapshots.
4. Reference iterators by name, never expand inline.
5. Always show UUID after create/edit.
6. Every requirement MUST eventually have a test (flag missing via `/pm-status`).
