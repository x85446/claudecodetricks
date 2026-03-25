---
name: pm-feature
description: Create and manage features under epics. Break epics into specific, testable, implementable capabilities. Every feature must belong to an epic.
argument-hint: [product crawl <epic-uuid>|add <epic-uuid>|list|show <uuid>|edit <uuid>]
---

# PM Feature — Consumable Feature Writer

Break epics into specific, testable, implementable features. Every feature MUST belong to an epic.

## Invocation

```
/pm-feature myriplay crawl <epic-uuid>
/pm-feature myriplay add <epic-uuid>
/pm-feature myriplay list
/pm-feature myriplay list epic=<uuid>
/pm-feature myriplay show <uuid>
/pm-feature myriplay edit <uuid>
```

## Database

**Path:** `.claude/db/marketing.sqlite` | `PRAGMA foreign_keys=ON;`

Uses existing `product_features` table with added columns: `epic_id` (required), `base_version`.

## Crawl Mode

1. Load parent epic by UUID — read its name, description, rationale
2. Read relevant source material
3. Propose features that decompose the epic
4. For each feature: short_desc (<=10 words), detailed_desc, target_release, tags
5. When a feature applies across a set (architectures, protocols, regions), reference or create an iterator via `/pm-iterator` instead of listing values inline
6. INSERT each into `product_features` with `epic_id`, `base_version` = parent epic's current `version`, `human_approved = 0`
7. INSERT version 1 into `product_feature_versions`
8. INSERT tags into `product_feature_tags`
9. Show summary

## Interact Mode (h/ai iterate)

1. Show parent epic for context
2. Ask: "What specific capabilities make up this epic?"
3. Human describes — AI structures into features
4. Iterate each feature individually: short_desc, detailed_desc, tags, release
5. Accept: y/yes/ok/good
6. INSERT with `human_approved = 1`, `base_version` = parent epic's current `version`
7. Show UUID: `Created feature: <uuid> "short_desc"`

## Edit

1. Show current feature detail with parent epic context
2. Ask what to change (description, tags, release, etc.)
3. Run h/ai iterate on changed fields
4. Auto-increment `version` on `product_features`
5. Set `base_version` = current parent epic `version` (clears staleness)
6. INSERT new snapshot into `product_feature_versions`
7. `updated_at` = CURRENT_TIMESTAMP

## Queries

```sql
-- List features for a product
SELECT pf.id, pf.short_desc, pf.version, pf.target_release, pf.status,
  pf.epic_id, pf.base_version, e.name AS epic_name, e.version AS epic_version,
  GROUP_CONCAT(t.name, ', ') AS tags
FROM product_features pf
JOIN our_products op ON op.id = pf.product_id
LEFT JOIN epics e ON e.id = pf.epic_id
LEFT JOIN product_feature_tags pft ON pft.feature_id = pf.id
LEFT JOIN tags t ON t.id = pft.tag_id
WHERE op.code = :product_code
GROUP BY pf.id ORDER BY pf.target_release, pf.short_desc;

-- Features for a specific epic
SELECT pf.id, pf.short_desc, pf.version, pf.base_version
FROM product_features pf WHERE pf.epic_id = :epic_uuid;
```

## Iterator Awareness

When describing features that apply across a set, check existing iterators:
```sql
SELECT i.name, i.description, GROUP_CONCAT(iv.value, ', ') AS values
FROM iterators i JOIN iterator_values iv ON iv.iterator_id = i.id
WHERE i.product_id = :pid GROUP BY i.id;
```
Reference iterators by name in descriptions. Propose new ones via `/pm-iterator` when repeated lists appear.

## Rules

1. **Every feature MUST have an `epic_id`.** No standalone features.
2. `base_version` = parent epic's `version` at time of creation/confirmation.
3. Every edit auto-increments version and snapshots.
4. Always show UUID after create/edit.
5. Reference iterators, never hardcode value lists.
