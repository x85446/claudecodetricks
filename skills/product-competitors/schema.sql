-- Competitive Intelligence Database Schema
-- Path: .claude/db/marketing.sqlite (shared with feature-tracker)

PRAGMA foreign_keys=ON;

-- Our products (shared with feature-tracker)
CREATE TABLE IF NOT EXISTS our_products (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    code        TEXT NOT NULL UNIQUE,            -- 'izuma-edge', 'izuma-cloud'
    name        TEXT NOT NULL,                    -- 'Izuma Edge'
    description TEXT,
    website     TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tags (shared with feature-tracker — one tag table for everything)
CREATE TABLE IF NOT EXISTS tags (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    name  TEXT NOT NULL UNIQUE
);

-- Competitors (scoped to one of our products)
CREATE TABLE IF NOT EXISTS competitors (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id    INTEGER NOT NULL REFERENCES our_products(id),
    code          TEXT NOT NULL,                  -- 'acme-ml', 'deepfoo'
    name          TEXT NOT NULL,                  -- 'Acme ML'
    website       TEXT,
    tier          TEXT DEFAULT 'primary',         -- 'primary', 'secondary', 'emerging'
    last_crawled  TEXT,                           -- ISO date of last research
    notes         TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_id, code)
);

-- Competitive features (rows in the matrix, scoped to one of our products)
-- Tagged with tags instead of categories. Use tags like 'cat:performance',
-- 'sub:latency', 'sub2:p99' for grouping, or flat tags like 'security', 'api'.
CREATE TABLE IF NOT EXISTS features (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id    INTEGER NOT NULL REFERENCES our_products(id),
    name          TEXT NOT NULL,                   -- 'Edge inference P99 latency'
    description   TEXT,
    importance    TEXT DEFAULT 'medium',            -- 'critical', 'high', 'medium', 'low', 'nice-to-have'
    display_order INTEGER DEFAULT 0,               -- for matrix row ordering
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_id, name)
);

CREATE INDEX IF NOT EXISTS idx_features_product ON features(product_id);

-- Competitive feature tags (many-to-many)
CREATE TABLE IF NOT EXISTS feature_tags (
    feature_id  INTEGER NOT NULL REFERENCES features(id),
    tag_id      INTEGER NOT NULL REFERENCES tags(id),
    PRIMARY KEY (feature_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_ft_tag ON feature_tags(tag_id);

-- Our assessment (how OUR product does on this feature)
CREATE TABLE IF NOT EXISTS our_assessments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id    INTEGER NOT NULL UNIQUE REFERENCES features(id),
    status        TEXT NOT NULL DEFAULT 'UNKNOWN',
    detail        TEXT,
    source_url    TEXT,
    researched_at TEXT,
    confidence    TEXT DEFAULT 'medium',
    notes         TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Competitor assessments (how each COMPETITOR does on this feature)
CREATE TABLE IF NOT EXISTS competitor_assessments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id    INTEGER NOT NULL REFERENCES features(id),
    competitor_id INTEGER NOT NULL REFERENCES competitors(id),
    status        TEXT NOT NULL DEFAULT 'UNKNOWN',
    detail        TEXT,
    source_url    TEXT,
    researched_at TEXT,
    confidence    TEXT DEFAULT 'medium',
    notes         TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(feature_id, competitor_id)
);

CREATE INDEX IF NOT EXISTS idx_ca_feature ON competitor_assessments(feature_id);
CREATE INDEX IF NOT EXISTS idx_ca_competitor ON competitor_assessments(competitor_id);
CREATE INDEX IF NOT EXISTS idx_ca_status ON competitor_assessments(status);

-- Derived comparison view
CREATE VIEW IF NOT EXISTS v_comparison AS
SELECT
    f.id AS feature_id,
    f.product_id,
    ca.competitor_id,
    c_comp.name AS competitor_name,
    oa.status AS our_status,
    ca.status AS their_status,
    CASE
        WHEN oa.status = 'YES' AND ca.status = 'NO' THEN 'BETTER'
        WHEN oa.status = 'NO' AND ca.status = 'YES' THEN 'WORSE'
        WHEN oa.status = 'YES' AND ca.status = 'PARTIAL' THEN 'BETTER'
        WHEN oa.status = 'PARTIAL' AND ca.status = 'YES' THEN 'WORSE'
        WHEN oa.status = ca.status THEN 'EQUAL'
        WHEN oa.status = 'UNKNOWN' OR ca.status = 'UNKNOWN' THEN 'UNKNOWN'
        ELSE 'EQUAL'
    END AS verdict
FROM features f
JOIN our_assessments oa ON oa.feature_id = f.id
JOIN competitor_assessments ca ON ca.feature_id = f.id
JOIN competitors c_comp ON c_comp.id = ca.competitor_id;

-- Crawl log
CREATE TABLE IF NOT EXISTS crawl_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    competitor_id   INTEGER NOT NULL REFERENCES competitors(id),
    source          TEXT,
    findings        TEXT,
    features_added  INTEGER DEFAULT 0,
    features_updated INTEGER DEFAULT 0,
    crawled_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Reserved tags
INSERT OR IGNORE INTO tags (name) VALUES
('web-detailed'),
('web-summary'),
('ppt'),
('ppt-risk'),
('sales-battlecard'),
('rfp'),
('roadmap-gap'),
('stale');
