-- Feature Tracker Schema (additive — shares DB with competitive-intel)
-- DB Path: .claude/db/marketing.sqlite
-- Run AFTER competitive-intel schema.sql (reuses: our_products, tags)

PRAGMA foreign_keys=ON;

-- Product features (our product's feature specifications)
CREATE TABLE IF NOT EXISTS product_features (
    id              TEXT PRIMARY KEY,            -- UUID (stable cross-DB reference)
    product_id      INTEGER NOT NULL REFERENCES our_products(id),
    short_desc      TEXT NOT NULL,               -- ≤10 words
    detailed_desc   TEXT NOT NULL,               -- paragraphs of text
    version         INTEGER NOT NULL DEFAULT 1,  -- auto-incremented on edits
    target_release  TEXT,                        -- '1.0', '2.0', etc.
    status          TEXT DEFAULT 'draft',        -- 'draft', 'approved', 'implemented', 'deprecated'
    human_approved  BOOLEAN DEFAULT 0,           -- 0 = AI-discovered (needs review), 1 = human approved
    source          TEXT DEFAULT 'interview',    -- 'interview', 'docs:<path>', 'code:<path>'
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pf_product ON product_features(product_id);
CREATE INDEX IF NOT EXISTS idx_pf_release ON product_features(target_release);

-- Version history — snapshot every version of a feature
CREATE TABLE IF NOT EXISTS product_feature_versions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id      TEXT NOT NULL REFERENCES product_features(id),
    version         INTEGER NOT NULL,
    short_desc      TEXT NOT NULL,
    detailed_desc   TEXT NOT NULL,
    target_release  TEXT,                        -- release this version targeted
    implemented_in  TEXT,                        -- release where this version actually shipped (NULL = not yet)
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(feature_id, version)
);

CREATE INDEX IF NOT EXISTS idx_pfv_feature ON product_feature_versions(feature_id);

-- Feature tags (many-to-many, reuses shared tags table)
CREATE TABLE IF NOT EXISTS product_feature_tags (
    feature_id  TEXT NOT NULL REFERENCES product_features(id),
    tag_id      INTEGER NOT NULL REFERENCES tags(id),
    PRIMARY KEY (feature_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_pft_tag ON product_feature_tags(tag_id);

-- Human-described tests
CREATE TABLE IF NOT EXISTS product_feature_tests (
    id              TEXT PRIMARY KEY,            -- UUID
    feature_id      TEXT NOT NULL REFERENCES product_features(id),
    title           TEXT NOT NULL,               -- ai/h iterated title
    detailed_desc   TEXT NOT NULL,               -- h/ai iterated test description
    version         INTEGER NOT NULL DEFAULT 1,  -- incremented when test is revised
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pft_feature ON product_feature_tests(feature_id);

-- Test version history
CREATE TABLE IF NOT EXISTS product_feature_test_versions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    test_id         TEXT NOT NULL REFERENCES product_feature_tests(id),
    version         INTEGER NOT NULL,
    title           TEXT NOT NULL,
    detailed_desc   TEXT NOT NULL,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(test_id, version)
);

-- Publish tag ordering (controls section order in generated markdown)
CREATE TABLE IF NOT EXISTS publish_tag_order (
    product_id  INTEGER NOT NULL REFERENCES our_products(id),
    tag_id      INTEGER NOT NULL REFERENCES tags(id),
    position    INTEGER NOT NULL,
    PRIMARY KEY (product_id, tag_id)
);
