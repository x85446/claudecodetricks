-- PM Skill System Schema (additive — shares DB with competitive-intel + feature-tracker)
-- DB Path: .claude/db/marketing.sqlite
-- Run AFTER competitive-intel schema.sql (reuses: our_products, tags)
-- Run AFTER feature-tracker schema.sql (extends: product_features, product_feature_tests)

PRAGMA foreign_keys=ON;

-- ═══════════════════════════════════════════════════════════════
-- Schema version tracking
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS schema_versions (
    skill       TEXT PRIMARY KEY,
    version     INTEGER NOT NULL DEFAULT 1,
    applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_versions (skill, version) VALUES ('pm', 1);

-- ═══════════════════════════════════════════════════════════════
-- Epics
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS epics (
    id              TEXT PRIMARY KEY,
    product_id      INTEGER NOT NULL REFERENCES our_products(id),
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,
    rationale       TEXT,
    version         INTEGER NOT NULL DEFAULT 1,
    base_version    INTEGER NOT NULL DEFAULT 1,
    human_approved  INTEGER NOT NULL DEFAULT 0,
    source          TEXT NOT NULL DEFAULT 'interview',
    status          TEXT NOT NULL DEFAULT 'draft',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_epics_product ON epics(product_id);

CREATE TABLE IF NOT EXISTS epic_versions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    epic_id     TEXT NOT NULL REFERENCES epics(id),
    version     INTEGER NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    rationale   TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(epic_id, version)
);

CREATE TABLE IF NOT EXISTS epic_tags (
    epic_id TEXT NOT NULL REFERENCES epics(id),
    tag_id  INTEGER NOT NULL REFERENCES tags(id),
    PRIMARY KEY (epic_id, tag_id)
);

-- ═══════════════════════════════════════════════════════════════
-- Extend product_features with epic linkage + base_version
-- ═══════════════════════════════════════════════════════════════
-- NOTE: SQLite cannot ADD NOT NULL without default. We add nullable
-- then enforce via application logic. Preflight checks for column existence.

-- Add epic_id if not present
SELECT CASE WHEN COUNT(*) = 0 THEN
    'ALTER TABLE product_features ADD COLUMN epic_id TEXT REFERENCES epics(id)'
END FROM pragma_table_info('product_features') WHERE name = 'epic_id';

-- Add base_version if not present
SELECT CASE WHEN COUNT(*) = 0 THEN
    'ALTER TABLE product_features ADD COLUMN base_version INTEGER NOT NULL DEFAULT 1'
END FROM pragma_table_info('product_features') WHERE name = 'base_version';

-- ═══════════════════════════════════════════════════════════════
-- Requirements
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS requirements (
    id                  TEXT PRIMARY KEY,
    feature_id          TEXT NOT NULL REFERENCES product_features(id),
    title               TEXT NOT NULL,
    description         TEXT NOT NULL,
    acceptance_criteria TEXT,
    version             INTEGER NOT NULL DEFAULT 1,
    base_version        INTEGER NOT NULL DEFAULT 1,
    human_approved      INTEGER NOT NULL DEFAULT 0,
    source              TEXT NOT NULL DEFAULT 'interview',
    status              TEXT NOT NULL DEFAULT 'draft',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_req_feature ON requirements(feature_id);

CREATE TABLE IF NOT EXISTS requirement_versions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    requirement_id      TEXT NOT NULL REFERENCES requirements(id),
    version             INTEGER NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL,
    acceptance_criteria TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(requirement_id, version)
);

-- ═══════════════════════════════════════════════════════════════
-- Extend product_feature_tests with requirement linkage + base_version
-- ═══════════════════════════════════════════════════════════════

-- Add requirement_id if not present
SELECT CASE WHEN COUNT(*) = 0 THEN
    'ALTER TABLE product_feature_tests ADD COLUMN requirement_id TEXT REFERENCES requirements(id)'
END FROM pragma_table_info('product_feature_tests') WHERE name = 'requirement_id';

-- Add base_version if not present
SELECT CASE WHEN COUNT(*) = 0 THEN
    'ALTER TABLE product_feature_tests ADD COLUMN base_version INTEGER NOT NULL DEFAULT 1'
END FROM pragma_table_info('product_feature_tests') WHERE name = 'base_version';

-- ═══════════════════════════════════════════════════════════════
-- Iterators
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS iterators (
    id          TEXT PRIMARY KEY,
    product_id  INTEGER NOT NULL REFERENCES our_products(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_id, name)
);

CREATE TABLE IF NOT EXISTS iterator_values (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    iterator_id TEXT NOT NULL REFERENCES iterators(id),
    value       TEXT NOT NULL,
    position    INTEGER NOT NULL,
    UNIQUE(iterator_id, value)
);

-- ═══════════════════════════════════════════════════════════════
-- Publish tag ordering (carried over from feature-tracker)
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS publish_tag_order (
    product_id  INTEGER NOT NULL REFERENCES our_products(id),
    tag_id      INTEGER NOT NULL REFERENCES tags(id),
    position    INTEGER NOT NULL,
    PRIMARY KEY (product_id, tag_id)
);
