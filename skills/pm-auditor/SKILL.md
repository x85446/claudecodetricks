---
name: pm-auditor
description: Detect stale entities when upstream changes invalidate downstream work. Uses base_version tracking to find and report cascade impacts.
argument-hint: [product|product epic <uuid>|product stale]
---

# PM Auditor — Dependency Chain Integrity

Detect when upstream changes invalidate downstream entities. Uses the `base_version` system to find staleness and cascade impacts.

## Invocation

```
/pm-auditor myriplay
/pm-auditor myriplay epic <uuid>
/pm-auditor myriplay stale
```

## Database

**Path:** `.claude/db/marketing.sqlite` | `PRAGMA foreign_keys=ON;`

## How Staleness Works

Every entity records which version of its parent it was derived from (`base_version`). When a parent's `version` > child's `base_version`, that child is **stale**. Staleness cascades: if a feature is stale, all its requirements and tests are implicitly stale too.

## Stale Entity Queries

```sql
-- Stale features (epic updated since feature was confirmed)
SELECT pf.id, pf.short_desc, pf.base_version AS based_on,
  e.version AS epic_now, e.name AS epic_name
FROM product_features pf
JOIN epics e ON e.id = pf.epic_id
WHERE pf.product_id = :pid AND e.version > pf.base_version;

-- Stale requirements (feature updated since req was confirmed)
SELECT r.id, r.title, r.base_version AS based_on,
  pf.version AS feature_now, pf.short_desc AS feature_name
FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
WHERE pf.product_id = :pid AND pf.version > r.base_version;

-- Stale tests (requirement updated since test was confirmed)
SELECT t.id, t.title, t.base_version AS based_on,
  r.version AS req_now, r.title AS req_name
FROM product_feature_tests t
JOIN requirements r ON r.id = t.requirement_id
WHERE r.feature_id IN (SELECT id FROM product_features WHERE product_id = :pid)
  AND r.version > t.base_version;

-- Cascade: features under stale epics (implicitly stale)
SELECT pf.id, pf.short_desc, 'cascade' AS reason
FROM product_features pf
JOIN epics e ON e.id = pf.epic_id
WHERE pf.product_id = :pid AND e.version > pf.base_version;
```

## Full Audit Output

```
Dependency Audit: Myriplay
──────────────────────────────
Stale entities: 4

[STALE] Feature "Topology rendering" (v1, based on Epic v3 -> Epic now v4)
  |-- [CASCADE] Req "Render HC containers" (v2) -- parent is stale
  |    |-- [CASCADE] Test "Verify HC count" (v1) -- grandparent is stale
  |-- [CASCADE] Req "Compact layout threshold" (v1) -- parent is stale

Actions:
  1. Review Feature "Topology rendering" against Epic v4 changes
  2. After updating feature, re-confirm 2 requirements and 1 test
```

## Epic Audit

Audit a single epic's tree — show all descendants and their staleness status:

1. Load epic and its current version
2. Find all features under it
3. For each feature, find requirements
4. For each requirement, find tests
5. Mark each as OK, STALE, or CASCADE

## Resolution

Resolution is automatic. When a human edits a stale entity (via WebTool or interact mode):
- Entity `version` auto-increments
- `base_version` is set to current parent `version`
- Staleness clears
- New version snapshot is created

The human just edits content. The system handles bookkeeping.

## Rules

1. Auditor is read-only. It reports but never modifies data.
2. Staleness = `parent.version > child.base_version`.
3. Cascade = if any ancestor is stale, descendants are implicitly stale.
4. Always show actionable next steps in output.
