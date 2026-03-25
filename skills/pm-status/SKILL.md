---
name: pm-status
description: Coverage dashboard for PM data. Shows epics, features, requirements, tests, iterators, staleness, and gaps.
argument-hint: [product|product coverage|product stale]
---

# PM Status — Coverage Dashboard

Report on completeness and health of the product database.

## Invocation

```
/pm-status myriplay
/pm-status myriplay coverage
/pm-status myriplay stale
```

## Database

**Path:** `.claude/db/marketing.sqlite` | `PRAGMA foreign_keys=ON;`

## Dashboard Query

```sql
-- Counts
SELECT
  (SELECT COUNT(*) FROM epics WHERE product_id = :pid) AS total_epics,
  (SELECT COUNT(*) FROM epics WHERE product_id = :pid AND human_approved = 1) AS approved_epics,
  (SELECT COUNT(*) FROM product_features WHERE product_id = :pid) AS total_features,
  (SELECT COUNT(*) FROM product_features WHERE product_id = :pid AND human_approved = 1) AS approved_features,
  (SELECT COUNT(*) FROM product_features WHERE product_id = :pid AND epic_id IS NULL) AS orphaned_features,
  (SELECT COUNT(*) FROM requirements r JOIN product_features pf ON pf.id = r.feature_id WHERE pf.product_id = :pid) AS total_reqs,
  (SELECT COUNT(*) FROM requirements r JOIN product_features pf ON pf.id = r.feature_id WHERE pf.product_id = :pid AND r.human_approved = 1) AS approved_reqs,
  (SELECT COUNT(*) FROM product_feature_tests t JOIN product_features pf ON pf.id = t.feature_id WHERE pf.product_id = :pid) AS total_tests,
  (SELECT COUNT(*) FROM iterators WHERE product_id = :pid) AS total_iterators;

-- Features without requirements
SELECT COUNT(*) FROM product_features pf
LEFT JOIN requirements r ON r.feature_id = pf.id
WHERE pf.product_id = :pid AND r.id IS NULL;

-- Requirements without tests
SELECT COUNT(*) FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE pf.product_id = :pid AND t.id IS NULL;

-- Stale features
SELECT COUNT(*) FROM product_features pf
JOIN epics e ON e.id = pf.epic_id
WHERE pf.product_id = :pid AND e.version > pf.base_version;

-- Stale requirements
SELECT COUNT(*) FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
WHERE pf.product_id = :pid AND pf.version > r.base_version;

-- Stale tests
SELECT COUNT(*) FROM product_feature_tests t
JOIN requirements r ON r.id = t.requirement_id
JOIN product_features pf ON pf.id = r.feature_id
WHERE pf.product_id = :pid AND r.version > t.base_version;
```

## Output Format

```
Product: Myriplay
----------------------------------------------
Epics:            4 total (3 approved, 1 pending)
Features:        23 total (18 approved, 5 pending)
  Orphaned:       0 (all belong to an epic)
Requirements:    45 total (30 approved, 15 pending)
  Without tests:  8
Iterators:        6 defined
Tests:           37 total

Coverage:
  Epics -> Features:         100% (4/4 have features)
  Features -> Requirements:   78% (18/23 have requirements)
  Requirements -> Tests:      82% (37/45 have tests)
  End-to-end:                64% (features with full req+test chain)

Staleness:
  Stale features:     2 (parent epic updated)
  Stale requirements: 3 (parent feature updated)
  Stale tests:        1 (parent requirement updated)
  Cascade affected:   5 reqs, 3 tests

Missing:
  [!] Feature "WebSocket streaming" has 0 requirements
  [!] Requirement "Compact layout threshold" has no test
```

## Coverage Subcommand

Detailed coverage showing which specific entities are missing children:

```sql
-- Epics without features
SELECT e.id, e.name FROM epics e
LEFT JOIN product_features pf ON pf.epic_id = e.id
WHERE e.product_id = :pid AND pf.id IS NULL;

-- Features without requirements (with epic context)
SELECT pf.id, pf.short_desc, e.name AS epic
FROM product_features pf
JOIN epics e ON e.id = pf.epic_id
LEFT JOIN requirements r ON r.feature_id = pf.id
WHERE pf.product_id = :pid AND r.id IS NULL;

-- Requirements without tests (with feature context)
SELECT r.id, r.title, pf.short_desc AS feature
FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE pf.product_id = :pid AND t.id IS NULL;
```

## Stale Subcommand

Delegates to `/pm-auditor` for detailed staleness report.

## Rules

1. Status is read-only. It reports but never modifies data.
2. Always show coverage percentages.
3. Always show missing entities with names (not just counts).
4. Flag orphaned features (no epic) — these violate the "every feature needs an epic" rule.
