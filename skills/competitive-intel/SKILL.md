---
name: competitive-intel
description: Maintain competitive analysis matrices per product using SQLite. Crawl competitor sites, score features, manage categorized feature rows with tagging for downstream outputs (web pages, presentations). Use when the user mentions competitors, competitive analysis, feature comparison, market positioning, or competitive matrix.
argument-hint: [product | product audit | product add-competitor <name> | product crawl <competitor>]
disable-model-invocation: false
---

# Competitive Intelligence

Maintain detailed, fair competitive analysis matrices per product. The kitchensink database is the single source of truth — every competitive fact lives here, whether we win or lose. Internal only.

## Invocation

```
/competitive-intel <product>                          # Show status of product's competitive matrix
/competitive-intel <product> audit                    # Audit for gaps, stale data, missing competitors
/competitive-intel <product> add-competitor <name>    # Add a new competitor
/competitive-intel <product> add-feature <name>       # Add a feature row interactively
/competitive-intel <product> crawl <competitor>       # Research a competitor via web crawling
/competitive-intel <product> crawl all                # Research all competitors
/competitive-intel <product> tag <tag> [filters]      # Tag rows for downstream use
/competitive-intel <product> export <format>          # Export filtered view (web, ppt, tsv, xlsx)
/competitive-intel <product> categories               # Show/edit feature category hierarchy
/competitive-intel <product> init                     # Initialize a new product matrix
/competitive-intel <product> matrix [filters]         # Show the matrix (optionally filtered)
```

## Paths

```
<root>/                                    # ~/workspace/izuma/marketing/
├── products/
│   └── <product>-competitive-matrix-kitchensink.tsv  # Generated export (for humans/git)
├── .claude/
│   └── skills/
│       └── competitive-intel/
│           ├── SKILL.md
│           └── db/
│               └── <product>-competitive-kitchensink.sqlite  # THE competitive database
```

The SQLite database is the source of truth. TSV/XLSX exports are generated outputs to `products/`.

## Database

**Path:** `<root>/.claude/skills/competitive-intel/db/<product>-competitive-kitchensink.sqlite`

Use `sqlite3` CLI or Python `sqlite3` module. Always run `PRAGMA foreign_keys=ON;` before writes.

### Schema

```sql
PRAGMA foreign_keys=ON;

-- Products being tracked
CREATE TABLE IF NOT EXISTS products (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    code        TEXT NOT NULL UNIQUE,            -- 'izuma-edge', 'izuma-cloud'
    name        TEXT NOT NULL,                    -- 'Izuma Edge'
    description TEXT,
    is_ours     BOOLEAN DEFAULT 0,               -- 1 = our product, 0 = competitor
    website     TEXT,
    tier        TEXT DEFAULT 'primary',           -- 'ours', 'primary', 'secondary', 'emerging'
    last_crawled TEXT,                            -- ISO date of last research
    notes       TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 3-tier feature category hierarchy
CREATE TABLE IF NOT EXISTS categories (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    category      TEXT NOT NULL,                  -- Tier 1: 'Performance', 'Security'
    sub           TEXT NOT NULL DEFAULT '',        -- Tier 2: 'Latency', 'Encryption'
    sub2          TEXT NOT NULL DEFAULT '',        -- Tier 3: 'P99', 'At-Rest'
    description   TEXT,
    display_order INTEGER DEFAULT 0,
    UNIQUE(category, sub, sub2)
);

CREATE INDEX IF NOT EXISTS idx_cat_order ON categories(display_order);

-- Features (rows in the matrix)
CREATE TABLE IF NOT EXISTS features (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    category_id   INTEGER NOT NULL REFERENCES categories(id),
    name          TEXT NOT NULL,                   -- 'Edge inference P99 latency'
    description   TEXT,                            -- Longer explanation if needed
    importance    TEXT DEFAULT 'medium',            -- 'critical', 'high', 'medium', 'low', 'nice-to-have'
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(category_id, name)
);

-- Assessments (the cells — one per feature × product)
CREATE TABLE IF NOT EXISTS assessments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id    INTEGER NOT NULL REFERENCES features(id),
    product_id    INTEGER NOT NULL REFERENCES products(id),
    status        TEXT NOT NULL DEFAULT 'UNKNOWN',  -- YES, NO, PARTIAL, BETTER, WORSE, EQUAL, UNKNOWN, N/A, PLANNED, BETA
    detail        TEXT,                              -- Supporting evidence / explanation
    source_url    TEXT,                              -- Where this info came from
    researched_at TEXT,                              -- When this was last verified
    confidence    TEXT DEFAULT 'medium',             -- 'high', 'medium', 'low', 'unverified'
    notes         TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(feature_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_assess_feature ON assessments(feature_id);
CREATE INDEX IF NOT EXISTS idx_assess_product ON assessments(product_id);
CREATE INDEX IF NOT EXISTS idx_assess_status ON assessments(status);

-- Tags (many-to-many on features)
CREATE TABLE IF NOT EXISTS tags (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    name  TEXT NOT NULL UNIQUE                    -- 'web-detailed', 'ppt', 'sales-battlecard'
);

CREATE TABLE IF NOT EXISTS feature_tags (
    feature_id  INTEGER NOT NULL REFERENCES features(id),
    tag_id      INTEGER NOT NULL REFERENCES tags(id),
    PRIMARY KEY (feature_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_ft_tag ON feature_tags(tag_id);

-- Crawl log — track research history
CREATE TABLE IF NOT EXISTS crawl_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id    INTEGER NOT NULL REFERENCES products(id),
    source        TEXT,                            -- URL or source name
    findings      TEXT,                            -- What was discovered
    features_added INTEGER DEFAULT 0,
    features_updated INTEGER DEFAULT 0,
    crawled_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Status Values

| Status | Meaning | Use For |
|---|---|---|
| `YES` | Fully supported | Feature exists, works well |
| `NO` | Not supported | Feature does not exist |
| `PARTIAL` | Limited support | Exists but with caveats |
| `BETTER` | We objectively win | Use on OUR product's assessment when we beat a specific competitor |
| `WORSE` | They objectively win | Use on OUR product's assessment when competitor beats us |
| `EQUAL` | Roughly equivalent | No meaningful difference |
| `UNKNOWN` | Not yet researched | Needs investigation |
| `N/A` | Not applicable | Feature doesn't apply to this product |
| `PLANNED` | On roadmap, not shipped | Announced but not available |
| `BETA` | In preview/beta | Available but not GA |

**Important:** `BETTER` and `WORSE` are relative assessments on OUR product row. Competitor rows use `YES`/`NO`/`PARTIAL` to state facts. The comparison is derived.

### Reserved Tags

```sql
INSERT OR IGNORE INTO tags (name) VALUES
('web-detailed'),       -- Detailed competitive web page
('web-summary'),        -- High-level competitive summary page
('ppt'),                -- Presentation-worthy wins (top 5-10)
('ppt-risk'),           -- Where competitors beat us (internal strategy)
('sales-battlecard'),   -- Key differentiators for sales
('rfp'),                -- Common RFP questions
('roadmap-gap'),        -- Gaps we should close (feed product roadmap)
('stale');              -- Needs re-research
```

Custom tags are freeform — just INSERT into `tags` and link via `feature_tags`.

## Key Queries

### Full Matrix (Kitchensink View)

```sql
-- Pivot the matrix: one row per feature, columns per product
SELECT
    c.category, c.sub, c.sub2,
    f.name AS feature,
    f.importance,
    GROUP_CONCAT(DISTINCT t.name) AS tags,
    -- Our product assessment
    MAX(CASE WHEN p.is_ours = 1 THEN a.status || ' | ' || COALESCE(a.detail, '') END) AS ours,
    -- Each competitor (dynamic — build one CASE per competitor)
    MAX(CASE WHEN p.code = 'competitor-a' THEN a.status || ' | ' || COALESCE(a.detail, '') END) AS competitor_a,
    MAX(CASE WHEN p.code = 'competitor-b' THEN a.status || ' | ' || COALESCE(a.detail, '') END) AS competitor_b
FROM features f
JOIN categories c ON c.id = f.category_id
LEFT JOIN assessments a ON a.feature_id = f.id
LEFT JOIN products p ON p.id = a.product_id
LEFT JOIN feature_tags ft ON ft.feature_id = f.id
LEFT JOIN tags t ON t.id = ft.tag_id
GROUP BY f.id
ORDER BY c.display_order, c.category, c.sub, c.sub2, f.name;
```

**Note:** The competitor columns in this query are dynamic. When generating the matrix, first query `SELECT code FROM products WHERE is_ours = 0 ORDER BY tier, code` to get the competitor list, then build the CASE statements dynamically.

### Win/Loss Summary

```sql
-- How do we stack up against each competitor?
SELECT
    p.name AS competitor,
    p.tier,
    SUM(CASE WHEN a_ours.status = 'BETTER' THEN 1
             WHEN a_ours.status IN ('YES') AND a_comp.status IN ('NO', 'PARTIAL') THEN 1
             ELSE 0 END) AS we_win,
    SUM(CASE WHEN a_ours.status = 'WORSE' THEN 1
             WHEN a_ours.status IN ('NO', 'PARTIAL') AND a_comp.status = 'YES' THEN 1
             ELSE 0 END) AS they_win,
    SUM(CASE WHEN a_ours.status = 'EQUAL' THEN 1
             WHEN a_ours.status = a_comp.status AND a_ours.status IN ('YES', 'NO') THEN 1
             ELSE 0 END) AS tie,
    SUM(CASE WHEN a_ours.status = 'UNKNOWN' OR a_comp.status = 'UNKNOWN' THEN 1 ELSE 0 END) AS unknown
FROM products p
JOIN products ours ON ours.is_ours = 1
JOIN features f
LEFT JOIN assessments a_ours ON a_ours.feature_id = f.id AND a_ours.product_id = ours.id
LEFT JOIN assessments a_comp ON a_comp.feature_id = f.id AND a_comp.product_id = p.id
WHERE p.is_ours = 0
GROUP BY p.id
ORDER BY p.tier, p.name;
```

### Coverage Gaps

```sql
-- Features where we haven't assessed a competitor yet
SELECT
    p.name AS competitor,
    c.category, c.sub,
    f.name AS feature,
    COALESCE(a.status, 'MISSING') AS status
FROM features f
JOIN categories c ON c.id = f.category_id
CROSS JOIN products p
LEFT JOIN assessments a ON a.feature_id = f.id AND a.product_id = p.id
WHERE p.is_ours = 0
  AND (a.status IS NULL OR a.status = 'UNKNOWN')
ORDER BY p.name, c.display_order;
```

### Stale Assessments

```sql
-- Assessments older than 30 days
SELECT
    p.name, f.name AS feature,
    a.status, a.researched_at,
    JULIANDAY('now') - JULIANDAY(a.researched_at) AS days_old
FROM assessments a
JOIN features f ON f.id = a.feature_id
JOIN products p ON p.id = a.product_id
WHERE a.researched_at IS NOT NULL
  AND JULIANDAY('now') - JULIANDAY(a.researched_at) > 30
ORDER BY days_old DESC;
```

### Tagged Features for Export

```sql
-- Get all features tagged for web-detailed
SELECT
    c.category, c.sub, c.sub2,
    f.name AS feature, f.importance,
    a.status, a.detail, a.source_url,
    p.name AS product_name, p.is_ours
FROM features f
JOIN categories c ON c.id = f.category_id
JOIN feature_tags ft ON ft.feature_id = f.id
JOIN tags t ON t.id = ft.tag_id
LEFT JOIN assessments a ON a.feature_id = f.id
LEFT JOIN products p ON p.id = a.product_id
WHERE t.name = 'web-detailed'
ORDER BY c.display_order, f.name, p.is_ours DESC, p.tier;
```

### Category Stats

```sql
-- Feature count per category
SELECT
    c.category, c.sub,
    COUNT(DISTINCT f.id) AS feature_count,
    SUM(CASE WHEN a.status = 'UNKNOWN' THEN 1 ELSE 0 END) AS unknown_count
FROM categories c
LEFT JOIN features f ON f.category_id = c.id
LEFT JOIN assessments a ON a.feature_id = f.id
GROUP BY c.category, c.sub
ORDER BY c.display_order;
```

## Workflows

### Init a New Product

```
/competitive-intel izuma-edge init
```

1. Create the database at `products/izuma-edge-competitive-kitchensink.sqlite`
2. Run the full schema DDL
3. Insert our product: `INSERT INTO products (code, name, is_ours, tier) VALUES ('izuma-edge', 'Izuma Edge', 1, 'ours')`
4. Insert reserved tags
5. Ask the user: "Who are the main competitors for Izuma Edge?"
6. For each competitor, ask for: name, website, tier (primary/secondary/emerging)
7. INSERT each competitor into `products`
8. Ask: "What are the main feature categories? (e.g., Performance, Security, Integrations, Pricing)"
9. Create initial category hierarchy
10. Generate the empty TSV export for git tracking

### Add a Competitor

```
/competitive-intel izuma-edge add-competitor acme-ml
```

1. Ask for: full name, website, tier, notes
2. INSERT into `products`
3. For each existing feature, INSERT an assessment with status `UNKNOWN`
4. Suggest: "Run `/competitive-intel izuma-edge crawl acme-ml` to research them?"

### Add a Feature

```
/competitive-intel izuma-edge add-feature "WebSocket streaming"
```

1. Show existing categories, ask which one (or create new)
2. If new category/sub/sub2, INSERT into `categories`
3. Ask for importance level
4. INSERT into `features`
5. For each product (ours + competitors), ask for status or set UNKNOWN
6. Ask for tags
7. INSERT `feature_tags` links

### Crawl a Competitor

```
/competitive-intel izuma-edge crawl competitor-a
```

1. Read competitor from `products` table — get website, last_crawled
2. Launch research agent(s):

```
Agent(subagent_type="general-purpose", prompt="Research <competitor> at <url>. Find:
  1. Product features and capabilities (product pages, docs, API reference)
  2. Pricing tiers, limits, free tier
  3. Security certifications (SOC2, HIPAA, ISO27001, compliance page)
  4. Performance claims (latency, uptime SLA, status page)
  5. Integration ecosystem (count, key partners, marketplace)
  6. Recent announcements (blog, changelog, last 6 months)
  7. Developer experience (SDK languages, documentation quality)
  8. Customer reviews (G2, Capterra, Reddit sentiment)
  Return findings as structured list: feature name, status (YES/NO/PARTIAL), detail, source URL.")
```

3. Map findings to existing features or propose new ones
4. Present to user with AskUserQuestion before writing to DB
5. UPDATE/INSERT assessments with `researched_at = date('now')`
6. UPDATE `products SET last_crawled = date('now')`
7. INSERT into `crawl_log`

For `crawl all`, launch one agent per competitor **in parallel**.

### Audit

```
/competitive-intel izuma-edge audit
```

Run the key queries and present a dashboard:

```
=== Competitive Matrix Audit: izuma-edge ===

Coverage:  127 features × 4 competitors = 508 assessments
  Filled:    412 (81%)
  Unknown:    96 (19%)

Win/Loss vs Primary Competitors:
  vs Competitor A:  45 wins / 23 losses / 31 ties / 28 unknown
  vs Competitor B:  38 wins / 31 losses / 25 ties / 33 unknown

Staleness:
  Competitor A: last crawled 12 days ago (OK)
  Competitor B: last crawled 47 days ago (STALE — suggest crawl)

Category Balance:
  Performance:    28 features
  Security:       22 features
  Integrations:   18 features
  Pricing:         8 features  (LOW — consider expanding)
  Developer UX:    5 features  (LOW)

Tag Coverage:
  web-detailed:    45 features
  ppt:             8 features
  sales-battlecard: 12 features
  untagged:        62 features (49% — consider tagging)

Suggested Actions:
  1. Crawl Competitor B (47 days stale)
  2. Fill 96 UNKNOWN assessments
  3. Expand Pricing and Developer UX categories
  4. Tag 62 untagged features
```

### Tag Features

```
/competitive-intel izuma-edge tag ppt category=Performance status=BETTER
```

1. Query features matching the filters
2. Show the matching features
3. Ask for confirmation
4. INSERT into `feature_tags` for each match

### Export: TSV (Kitchensink)

```
/competitive-intel izuma-edge export tsv
```

1. Query the full pivot matrix (dynamic CASE per competitor)
2. Write to `products/izuma-edge-competitive-matrix-kitchensink.tsv`
3. Columns: `category  sub  sub2  feature  importance  tags  our_product  competitor_a  competitor_b  ...`
4. Cell format: `STATUS | detail`
5. Sorted by category display_order

This TSV is a **generated artifact** for human review and git tracking. The database is the source of truth.

### Export: Web Detailed

```
/competitive-intel izuma-edge export web-detailed
```

1. Query features tagged `web-detailed`
2. Generate markdown grouped by category
3. Include all competitors with status indicators
4. Write to `products/izuma-edge-competitive-web-detailed.md`

### Export: PPT

```
/competitive-intel izuma-edge export ppt
```

1. Query features tagged `ppt` where our status is a win
2. Limit to top 10 most impactful (by importance)
3. Generate a concise comparison table
4. Write to `products/izuma-edge-competitive-ppt.md`

### Export: XLSX

```
/competitive-intel izuma-edge export xlsx
```

1. Query the full pivot matrix
2. Use Python openpyxl to create XLSX with:
   - Header row with competitor names
   - Conditional formatting: green=YES/BETTER, red=NO/WORSE, yellow=PARTIAL, gray=UNKNOWN
   - Frozen header row and category columns
   - Auto-width columns
3. Write to `products/izuma-edge-competitive-matrix-kitchensink.xlsx`

## Research Sources

| Source | What to Look For | Agent Instructions |
|---|---|---|
| Product website | Features, pricing, integrations list | Crawl /features, /pricing, /integrations pages |
| Documentation | API capabilities, SDK languages, limits | Check /docs, developer portal |
| Blog/Changelog | Recent launches, roadmap signals | Last 6 months of posts |
| Status page | Uptime SLA, incident history | Look for status.competitor.com |
| G2/Capterra | User reviews, satisfaction scores | Search "competitor name" reviews |
| GitHub | Open source components, community activity | Check their org, star counts, activity |
| LinkedIn | Hiring signals (roles = what they're building) | Search company jobs page |
| Crunchbase | Funding, company size, growth signals | Company profile |

## Fairness Rules

**This matrix must be honest and fair. It is internal.**

1. **Never exaggerate our advantages.** Use objective evidence.
2. **Never downplay competitor strengths.** If they beat us, say so with honest detail.
3. **Always cite sources** — set `source_url` and `researched_at` on every assessment.
4. **Mark uncertainty.** If unsure, use `UNKNOWN` — never guess.
5. **Date-stamp research.** Competitive landscapes change fast.
6. **Include context.** "They have 200+ integrations vs our 45 (as of 2026-03)" is better than just "they win".
7. **Separate fact from opinion.** Features are facts. "Better UX" is opinion — quantify if possible.
8. **Set confidence.** Use `high` for verified from official source, `medium` for inferred, `low` for secondhand, `unverified` for unconfirmed.

## Important Rules

1. **SQLite is the source of truth.** TSV/XLSX/MD exports are generated artifacts.
2. **Always `PRAGMA foreign_keys=ON`** before any write.
3. **Kitchensink is internal** — brutally honest, includes where we lose.
4. **Tags drive downstream** — the matrix is the source, exports are filtered views.
5. **Fair assessment** — if a competitor beats us, document it clearly.
6. **Always validate** category hierarchy before adding features.
7. **Crawl regularly** — audit flags staleness at 30 days.
8. **Use AskUserQuestion** before writing crawl findings to the DB.
9. **Parallel research** — launch multiple crawler agents simultaneously.
10. **Export to TSV after changes** — keep a git-trackable copy alongside the DB.
11. **Dynamic competitor columns** — never hardcode competitor names in queries. Always build from `SELECT code FROM products WHERE is_ours = 0`.
