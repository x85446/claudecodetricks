-- PM Skill System Key Queries
-- All queries take :product_code or :pid parameter

-- ═══════════════════════════════════════════════════════════════
-- UUID generation (use in INSERT statements)
-- ═══════════════════════════════════════════════════════════════
-- SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' ||
--   substr(hex(randomblob(2)),2) || '-' ||
--   substr('89ab',abs(random()) % 4 + 1, 1) ||
--   substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6)));

-- ═══════════════════════════════════════════════════════════════
-- Get product_id from code
-- ═══════════════════════════════════════════════════════════════
SELECT id FROM our_products WHERE code = :product_code;

-- ═══════════════════════════════════════════════════════════════
-- List epics
-- ═══════════════════════════════════════════════════════════════
SELECT e.id, e.name, e.version, e.status, e.human_approved,
  GROUP_CONCAT(t.name, ', ') AS tags
FROM epics e
LEFT JOIN epic_tags et ON et.epic_id = e.id
LEFT JOIN tags t ON t.id = et.tag_id
WHERE e.product_id = :pid
GROUP BY e.id ORDER BY e.name;

-- ═══════════════════════════════════════════════════════════════
-- Epic detail
-- ═══════════════════════════════════════════════════════════════
SELECT e.*, GROUP_CONCAT(t.name, ', ') AS tags
FROM epics e
LEFT JOIN epic_tags et ON et.epic_id = e.id
LEFT JOIN tags t ON t.id = et.tag_id
WHERE e.id = :uuid GROUP BY e.id;

-- ═══════════════════════════════════════════════════════════════
-- Epic tree (epic + all features + counts)
-- ═══════════════════════════════════════════════════════════════
SELECT e.id AS epic_id, e.name AS epic_name, e.version AS epic_version,
  pf.id AS feature_id, pf.short_desc, pf.version AS feature_version,
  pf.base_version,
  COUNT(DISTINCT r.id) AS req_count,
  COUNT(DISTINCT t.id) AS test_count
FROM epics e
LEFT JOIN product_features pf ON pf.epic_id = e.id
LEFT JOIN requirements r ON r.feature_id = pf.id
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE e.id = :uuid
GROUP BY pf.id;

-- ═══════════════════════════════════════════════════════════════
-- List features with epic context
-- ═══════════════════════════════════════════════════════════════
SELECT pf.id, pf.short_desc, pf.version, pf.target_release, pf.status,
  pf.base_version, pf.human_approved,
  e.id AS epic_id, e.name AS epic_name, e.version AS epic_version,
  GROUP_CONCAT(DISTINCT t.name, ', ') AS tags
FROM product_features pf
LEFT JOIN epics e ON e.id = pf.epic_id
LEFT JOIN product_feature_tags pft ON pft.feature_id = pf.id
LEFT JOIN tags t ON t.id = pft.tag_id
WHERE pf.product_id = :pid
GROUP BY pf.id ORDER BY e.name, pf.target_release, pf.short_desc;

-- ═══════════════════════════════════════════════════════════════
-- Feature detail with requirements and tests
-- ═══════════════════════════════════════════════════════════════
SELECT pf.id, pf.short_desc, pf.detailed_desc, pf.version, pf.target_release,
  pf.status, pf.base_version, pf.epic_id,
  e.name AS epic_name, e.version AS epic_version,
  GROUP_CONCAT(DISTINCT tg.name, ', ') AS tags
FROM product_features pf
LEFT JOIN epics e ON e.id = pf.epic_id
LEFT JOIN product_feature_tags pft ON pft.feature_id = pf.id
LEFT JOIN tags tg ON tg.id = pft.tag_id
WHERE pf.id = :uuid GROUP BY pf.id;

-- Then separately:
SELECT r.id, r.title, r.version, r.base_version, r.status, r.human_approved,
  t.id AS test_id, t.title AS test_title, t.detailed_desc AS test_desc,
  t.version AS test_version, t.base_version AS test_base_version
FROM requirements r
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE r.feature_id = :uuid
ORDER BY r.title;

-- ═══════════════════════════════════════════════════════════════
-- List requirements for a feature
-- ═══════════════════════════════════════════════════════════════
SELECT r.id, r.title, r.version, r.base_version, r.status, r.human_approved
FROM requirements r WHERE r.feature_id = :feature_uuid
ORDER BY r.title;

-- ═══════════════════════════════════════════════════════════════
-- Requirement detail with test
-- ═══════════════════════════════════════════════════════════════
SELECT r.*, pf.short_desc AS feature_name, pf.version AS feature_version
FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
WHERE r.id = :uuid;

-- ═══════════════════════════════════════════════════════════════
-- Iterators for a product
-- ═══════════════════════════════════════════════════════════════
SELECT i.id, i.name, i.description,
  GROUP_CONCAT(iv.value, ', ') AS vals
FROM iterators i
LEFT JOIN iterator_values iv ON iv.iterator_id = i.id
WHERE i.product_id = :pid
GROUP BY i.id ORDER BY i.name;

-- ═══════════════════════════════════════════════════════════════
-- Staleness: features with outdated base_version
-- ═══════════════════════════════════════════════════════════════
SELECT pf.id, pf.short_desc, pf.base_version AS based_on,
  e.version AS epic_now, e.name AS epic_name
FROM product_features pf
JOIN epics e ON e.id = pf.epic_id
WHERE pf.product_id = :pid AND e.version > pf.base_version;

-- ═══════════════════════════════════════════════════════════════
-- Staleness: requirements with outdated base_version
-- ═══════════════════════════════════════════════════════════════
SELECT r.id, r.title, r.base_version AS based_on,
  pf.version AS feature_now, pf.short_desc AS feature_name
FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
WHERE pf.product_id = :pid AND pf.version > r.base_version;

-- ═══════════════════════════════════════════════════════════════
-- Staleness: tests with outdated base_version
-- ═══════════════════════════════════════════════════════════════
SELECT t.id, t.title, t.base_version AS based_on,
  r.version AS req_now, r.title AS req_name
FROM product_feature_tests t
JOIN requirements r ON r.id = t.requirement_id
JOIN product_features pf ON pf.id = r.feature_id
WHERE pf.product_id = :pid AND r.version > t.base_version;

-- ═══════════════════════════════════════════════════════════════
-- Coverage gaps
-- ═══════════════════════════════════════════════════════════════

-- Epics without features
SELECT e.id, e.name FROM epics e
LEFT JOIN product_features pf ON pf.epic_id = e.id
WHERE e.product_id = :pid AND pf.id IS NULL;

-- Features without requirements
SELECT pf.id, pf.short_desc, e.name AS epic
FROM product_features pf
LEFT JOIN epics e ON e.id = pf.epic_id
LEFT JOIN requirements r ON r.feature_id = pf.id
WHERE pf.product_id = :pid AND r.id IS NULL;

-- Requirements without tests
SELECT r.id, r.title, pf.short_desc AS feature
FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE pf.product_id = :pid AND t.id IS NULL;

-- ═══════════════════════════════════════════════════════════════
-- Full publish query (hierarchical)
-- ═══════════════════════════════════════════════════════════════
SELECT
  e.id AS epic_id, e.name AS epic_name, e.description AS epic_desc,
  e.rationale, e.version AS epic_version, e.status AS epic_status,
  pf.id AS feature_id, pf.short_desc, pf.detailed_desc,
  pf.version AS feature_version, pf.target_release, pf.status AS feature_status,
  GROUP_CONCAT(DISTINCT tg.name, ', ') AS feature_tags,
  r.id AS req_id, r.title AS req_title, r.description AS req_desc,
  r.acceptance_criteria, r.version AS req_version,
  t.id AS test_id, t.title AS test_title, t.detailed_desc AS test_desc,
  t.version AS test_version
FROM epics e
LEFT JOIN product_features pf ON pf.epic_id = e.id
LEFT JOIN product_feature_tags pft ON pft.feature_id = pf.id
LEFT JOIN tags tg ON tg.id = pft.tag_id
LEFT JOIN requirements r ON r.feature_id = pf.id
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE e.product_id = :pid
GROUP BY e.id, pf.id, r.id, t.id
ORDER BY e.name, pf.target_release, pf.short_desc, r.title, t.title;
