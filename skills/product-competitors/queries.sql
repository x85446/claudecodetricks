-- Competitive Intelligence Key Queries
-- All queries take :product_code parameter (e.g., 'izuma-edge')

-- ═══════════════════════════════════════════════════════════════
-- Full Matrix (Kitchensink View)
-- Competitor columns are DYNAMIC. First query the competitor list:
--   SELECT code FROM competitors WHERE product_id = :pid ORDER BY tier, code
-- Then build one CASE per competitor in the SELECT below.
-- ═══════════════════════════════════════════════════════════════
-- TEMPLATE (replace CASE lines dynamically):
SELECT
    c.category, c.sub, c.sub2,
    f.name AS feature,
    f.importance,
    GROUP_CONCAT(DISTINCT t.name) AS tags,
    oa.status || ' | ' || COALESCE(oa.detail, '') AS ours
    -- ADD: MAX(CASE WHEN comp.code = '<code>' THEN ca.status || ' | ' || COALESCE(ca.detail, '') END) AS <code>
FROM features f
JOIN our_products op ON op.id = f.product_id
JOIN categories c ON c.id = f.category_id
LEFT JOIN our_assessments oa ON oa.feature_id = f.id
LEFT JOIN competitor_assessments ca ON ca.feature_id = f.id
LEFT JOIN competitors comp ON comp.id = ca.competitor_id
LEFT JOIN feature_tags ft ON ft.feature_id = f.id
LEFT JOIN tags t ON t.id = ft.tag_id
WHERE op.code = :product_code
GROUP BY f.id
ORDER BY c.display_order, c.category, c.sub, c.sub2, f.name;


-- ═══════════════════════════════════════════════════════════════
-- Win/Loss Summary
-- ═══════════════════════════════════════════════════════════════
SELECT
    comp.name AS competitor,
    comp.tier,
    SUM(CASE WHEN v.verdict = 'BETTER' THEN 1 ELSE 0 END) AS we_win,
    SUM(CASE WHEN v.verdict = 'WORSE' THEN 1 ELSE 0 END) AS they_win,
    SUM(CASE WHEN v.verdict = 'EQUAL' THEN 1 ELSE 0 END) AS tie,
    SUM(CASE WHEN v.verdict = 'UNKNOWN' THEN 1 ELSE 0 END) AS unknown
FROM v_comparison v
JOIN competitors comp ON comp.id = v.competitor_id
JOIN our_products op ON op.id = v.product_id
WHERE op.code = :product_code
GROUP BY comp.id
ORDER BY comp.tier, comp.name;


-- ═══════════════════════════════════════════════════════════════
-- Coverage Gaps (features with UNKNOWN or MISSING competitor assessments)
-- ═══════════════════════════════════════════════════════════════
SELECT
    comp.name AS competitor,
    c.category, c.sub,
    f.name AS feature,
    COALESCE(ca.status, 'MISSING') AS status
FROM features f
JOIN our_products op ON op.id = f.product_id
JOIN categories c ON c.id = f.category_id
CROSS JOIN competitors comp ON comp.product_id = op.id
LEFT JOIN competitor_assessments ca ON ca.feature_id = f.id AND ca.competitor_id = comp.id
WHERE op.code = :product_code
  AND (ca.status IS NULL OR ca.status = 'UNKNOWN')
ORDER BY comp.name, c.display_order;


-- ═══════════════════════════════════════════════════════════════
-- Stale Competitors (not crawled in 30+ days)
-- ═══════════════════════════════════════════════════════════════
SELECT name, tier, last_crawled,
    CAST(JULIANDAY('now') - JULIANDAY(last_crawled) AS INTEGER) AS days_old
FROM competitors
WHERE product_id = (SELECT id FROM our_products WHERE code = :product_code)
  AND last_crawled IS NOT NULL
  AND JULIANDAY('now') - JULIANDAY(last_crawled) > 30
ORDER BY days_old DESC;


-- ═══════════════════════════════════════════════════════════════
-- Tagged Features (for filtered exports)
-- Set :tag_name to 'web-detailed', 'ppt', etc.
-- ═══════════════════════════════════════════════════════════════
SELECT
    c.category, c.sub, c.sub2,
    f.name AS feature, f.importance,
    oa.status AS our_status, oa.detail AS our_detail,
    comp.name AS competitor, ca.status AS their_status, ca.detail AS their_detail
FROM features f
JOIN our_products op ON op.id = f.product_id
JOIN categories c ON c.id = f.category_id
JOIN feature_tags ft ON ft.feature_id = f.id
JOIN tags t ON t.id = ft.tag_id
LEFT JOIN our_assessments oa ON oa.feature_id = f.id
LEFT JOIN competitor_assessments ca ON ca.feature_id = f.id
LEFT JOIN competitors comp ON comp.id = ca.competitor_id
WHERE op.code = :product_code AND t.name = :tag_name
ORDER BY c.display_order, f.name, comp.tier;


-- ═══════════════════════════════════════════════════════════════
-- Category Stats
-- ═══════════════════════════════════════════════════════════════
SELECT
    c.category, c.sub,
    COUNT(DISTINCT f.id) AS feature_count
FROM categories c
LEFT JOIN features f ON f.category_id = c.id
    AND f.product_id = (SELECT id FROM our_products WHERE code = :product_code)
GROUP BY c.category, c.sub
ORDER BY c.display_order;


-- ═══════════════════════════════════════════════════════════════
-- Audit Summary (run all counts for dashboard)
-- ═══════════════════════════════════════════════════════════════
SELECT
    (SELECT COUNT(*) FROM features WHERE product_id = op.id) AS total_features,
    (SELECT COUNT(*) FROM competitors WHERE product_id = op.id) AS total_competitors,
    (SELECT COUNT(*) FROM competitor_assessments ca
     JOIN features f ON f.id = ca.feature_id WHERE f.product_id = op.id) AS filled_assessments,
    (SELECT COUNT(*) FROM competitor_assessments ca
     JOIN features f ON f.id = ca.feature_id
     WHERE f.product_id = op.id AND ca.status = 'UNKNOWN') AS unknown_assessments,
    (SELECT COUNT(*) FROM features f
     LEFT JOIN feature_tags ft ON ft.feature_id = f.id
     WHERE f.product_id = op.id AND ft.tag_id IS NULL) AS untagged_features
FROM our_products op
WHERE op.code = :product_code;
