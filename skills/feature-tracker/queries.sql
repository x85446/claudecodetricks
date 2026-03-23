-- Feature Tracker Key Queries
-- All queries take :product_code parameter (e.g., 'izuma-edge')

-- ═══════════════════════════════════════════════════════════════
-- List all features for a product
-- ═══════════════════════════════════════════════════════════════
SELECT
    pf.id AS uuid,
    pf.short_desc,
    pf.version,
    pf.target_release,
    pf.status,
    GROUP_CONCAT(t.name, ', ') AS tags
FROM product_features pf
JOIN our_products op ON op.id = pf.product_id
LEFT JOIN product_feature_tags pft ON pft.feature_id = pf.id
LEFT JOIN tags t ON t.id = pft.tag_id
WHERE op.code = :product_code
GROUP BY pf.id
ORDER BY pf.target_release, pf.short_desc;


-- ═══════════════════════════════════════════════════════════════
-- Tag query: AND (features that have ALL specified tags)
-- Example: features tagged BOTH 'security' AND 'api'
-- Pass :tag1, :tag2, :tag_count = 2
-- ═══════════════════════════════════════════════════════════════
SELECT
    pf.id AS uuid,
    pf.short_desc,
    pf.version,
    pf.target_release,
    pf.status,
    GROUP_CONCAT(DISTINCT all_tags.name, ', ') AS tags
FROM product_features pf
JOIN our_products op ON op.id = pf.product_id
JOIN product_feature_tags pft ON pft.feature_id = pf.id
JOIN tags t ON t.id = pft.tag_id
LEFT JOIN product_feature_tags all_pft ON all_pft.feature_id = pf.id
LEFT JOIN tags all_tags ON all_tags.id = all_pft.tag_id
WHERE op.code = :product_code
  AND t.name IN (:tag1, :tag2)  -- expand as needed
GROUP BY pf.id
HAVING COUNT(DISTINCT t.name) = :tag_count  -- must match ALL tags
ORDER BY pf.target_release;


-- ═══════════════════════════════════════════════════════════════
-- Tag query: OR (features that have ANY of the specified tags)
-- ═══════════════════════════════════════════════════════════════
SELECT DISTINCT
    pf.id AS uuid,
    pf.short_desc,
    pf.version,
    pf.target_release,
    pf.status,
    GROUP_CONCAT(DISTINCT all_tags.name, ', ') AS tags
FROM product_features pf
JOIN our_products op ON op.id = pf.product_id
JOIN product_feature_tags pft ON pft.feature_id = pf.id
JOIN tags t ON t.id = pft.tag_id
LEFT JOIN product_feature_tags all_pft ON all_pft.feature_id = pf.id
LEFT JOIN tags all_tags ON all_tags.id = all_pft.tag_id
WHERE op.code = :product_code
  AND t.name IN (:tag1, :tag2)  -- expand as needed
GROUP BY pf.id
ORDER BY pf.target_release;


-- ═══════════════════════════════════════════════════════════════
-- Feature detail (full view with tests)
-- ═══════════════════════════════════════════════════════════════
SELECT
    pf.id AS uuid,
    pf.short_desc,
    pf.detailed_desc,
    pf.version,
    pf.target_release,
    pf.status,
    GROUP_CONCAT(DISTINCT t.name, ', ') AS tags,
    pftest.id AS test_uuid,
    pftest.title AS test_title,
    pftest.detailed_desc AS test_desc,
    pftest.version AS test_version
FROM product_features pf
JOIN our_products op ON op.id = pf.product_id
LEFT JOIN product_feature_tags pft ON pft.feature_id = pf.id
LEFT JOIN tags t ON t.id = pft.tag_id
LEFT JOIN product_feature_tests pftest ON pftest.feature_id = pf.id
WHERE op.code = :product_code AND pf.id = :feature_uuid
GROUP BY pf.id, pftest.id;


-- ═══════════════════════════════════════════════════════════════
-- Version history for a feature
-- ═══════════════════════════════════════════════════════════════
SELECT
    version,
    short_desc,
    target_release,
    implemented_in,
    created_at
FROM product_feature_versions
WHERE feature_id = :feature_uuid
ORDER BY version DESC;


-- ═══════════════════════════════════════════════════════════════
-- Features by release
-- ═══════════════════════════════════════════════════════════════
SELECT
    pf.target_release,
    COUNT(*) AS feature_count,
    SUM(CASE WHEN pf.status = 'implemented' THEN 1 ELSE 0 END) AS implemented,
    SUM(CASE WHEN pf.status = 'approved' THEN 1 ELSE 0 END) AS approved,
    SUM(CASE WHEN pf.status = 'draft' THEN 1 ELSE 0 END) AS draft
FROM product_features pf
JOIN our_products op ON op.id = pf.product_id
WHERE op.code = :product_code
GROUP BY pf.target_release
ORDER BY pf.target_release;


-- ═══════════════════════════════════════════════════════════════
-- Tag cloud (all tags with feature count for a product)
-- ═══════════════════════════════════════════════════════════════
SELECT
    t.name AS tag,
    COUNT(pft.feature_id) AS feature_count
FROM tags t
JOIN product_feature_tags pft ON pft.tag_id = t.id
JOIN product_features pf ON pf.id = pft.feature_id
JOIN our_products op ON op.id = pf.product_id
WHERE op.code = :product_code
GROUP BY t.id
ORDER BY feature_count DESC;


-- ═══════════════════════════════════════════════════════════════
-- Features without tests
-- ═══════════════════════════════════════════════════════════════
SELECT
    pf.id AS uuid,
    pf.short_desc,
    pf.target_release,
    pf.status
FROM product_features pf
JOIN our_products op ON op.id = pf.product_id
LEFT JOIN product_feature_tests pftest ON pftest.feature_id = pf.id
WHERE op.code = :product_code AND pftest.id IS NULL
ORDER BY pf.target_release;
