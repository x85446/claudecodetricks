---
name: competitive-intel
description: Maintain competitive analysis matrices per product using SQLite. Crawl competitor sites, score features, manage categorized feature rows with tagging for downstream outputs (web pages, presentations). Use when the user mentions competitors, competitive analysis, feature comparison, market positioning, or competitive matrix.
argument-hint: [product | product audit | product add-competitor <name> | product crawl <competitor>]
---

# Competitive Intelligence

Maintain detailed, fair competitive analysis matrices per product. The kitchensink database is the single source of truth — every competitive fact lives here, whether we win or lose. Internal only.

## Invocation

```
/competitive-intel <product>                          # Show matrix status
/competitive-intel <product> audit                    # Audit gaps, stale data
/competitive-intel <product> add-competitor <name>    # Add a competitor
/competitive-intel <product> add-feature <name>       # Add a feature row
/competitive-intel <product> crawl <competitor>       # Research via web crawling
/competitive-intel <product> crawl all                # Research all competitors
/competitive-intel <product> tag <tag> [filters]      # Tag rows for downstream use
/competitive-intel <product> export <format>          # Export (web, ppt, tsv, xlsx)
/competitive-intel <product> categories               # Show/edit category hierarchy
/competitive-intel <product> init                     # Initialize new product
/competitive-intel <product> matrix [filters]         # Show the matrix
```

## File Structure

```
<root>/                                    # ~/workspace/izuma/marketing/
├── products/
│   └── <product>-competitive-matrix-kitchensink.tsv  # Generated export (for humans/git)
├── .claude/
│   └── skills/
│       └── competitive-intel/
│           ├── SKILL.md              # This file
│           ├── schema.sql            # Database schema DDL
│           ├── queries.sql           # Key queries for reports
│           └── queries.sql           # Key queries for reports
│       └── feature-tracker/          # Sister skill (shares same DB)
```

## Database

**Path:** `.claude/db/marketing.sqlite` (shared with feature-tracker) Schema is in [schema.sql](schema.sql). Key queries are in [queries.sql](queries.sql).

Always run `PRAGMA foreign_keys=ON;` before writes. Initialize with `schema.sql` on first run.

### Core Tables

| Table | Purpose |
|---|---|
| `our_products` | Our products being tracked (e.g., izuma-edge) |
| `competitors` | Competitor products, scoped to one of ours (has `product_id` FK) |
| `features` | Competitive feature rows in the matrix, scoped to one of our products |
| `our_assessments` | How OUR product does on each feature (one per feature) |
| `competitor_assessments` | How each COMPETITOR does on each feature (one per feature × competitor) |
| `tags` | Tag registry (web-detailed, ppt, sales-battlecard, etc.) |
| `feature_tags` | Many-to-many link between features and tags |
| `crawl_log` | Research history per competitor |
| `v_comparison` | VIEW — derives BETTER/WORSE/EQUAL verdict from assessment pairs |

### Status Values

| Status | Meaning |
|---|---|
| `YES` | Fully supported |
| `NO` | Not supported |
| `PARTIAL` | Limited / caveats |
| `UNKNOWN` | Not yet researched |
| `N/A` | Not applicable |
| `PLANNED` | On roadmap, not shipped |
| `BETA` | In preview |

Competitor assessments use factual statuses (YES/NO/PARTIAL). The `v_comparison` view derives BETTER/WORSE/EQUAL by comparing our status vs theirs.

### Tags

Tags mark features for downstream exports. Multiple tags per feature via `feature_tags`.

| Tag | Purpose |
|---|---|
| `web-detailed` | Detailed competitive web page |
| `web-summary` | High-level summary page |
| `ppt` | Presentation-worthy wins (top 5-10) |
| `ppt-risk` | Where competitors beat us (internal strategy) |
| `sales-battlecard` | Key differentiators for sales |
| `rfp` | Common RFP questions |
| `roadmap-gap` | Gaps to close (feed product roadmap) |
| `stale` | Needs re-research |

Custom tags are freeform — just INSERT into `tags`.

## Workflows

### Init

1. Create database, run [schema.sql](schema.sql)
2. INSERT our product into `our_products`
3. Ask user: "Who are the main competitors?" — for each, get name, website, tier
4. INSERT competitors
5. Ask: "What feature categories?" — INSERT into `categories`
6. Generate empty TSV export

### Add Competitor

1. Ask for: full name, website, tier (primary/secondary/emerging), notes
2. INSERT into `competitors` (linked to our product)
3. For each existing feature, INSERT `competitor_assessments` with status `UNKNOWN`
4. Suggest crawl

### Add Feature

1. Show existing categories, ask which (or create new)
2. Ask for importance (critical/high/medium/low/nice-to-have)
3. INSERT into `features`
4. INSERT `our_assessments` — ask for our status
5. For each competitor, INSERT `competitor_assessments` (ask or set UNKNOWN)
6. Ask for tags, INSERT `feature_tags`

### Crawl

1. Read competitor from `competitors` table — get website, last_crawled
2. Launch research agent:

```
Agent(subagent_type="general-purpose", prompt="Research <name> at <website>. Find:
  1. Product features and capabilities (product pages, docs, API reference)
  2. Pricing tiers, limits, free tier
  3. Security certifications (SOC2, HIPAA, ISO27001)
  4. Performance claims (latency, uptime SLA, status page)
  5. Integration ecosystem (count, key partners)
  6. Recent announcements (blog, changelog, last 6 months)
  7. Developer experience (SDK languages, docs quality)
  8. Customer reviews (G2, Capterra, Reddit)
  Return structured list: feature name, status (YES/NO/PARTIAL), detail, source URL.")
```

3. Map findings to existing features or propose new ones
4. Present to user with AskUserQuestion before writing
5. UPDATE/INSERT assessments with `researched_at = date('now')`
6. UPDATE `competitors SET last_crawled = date('now')`
7. INSERT into `crawl_log`

For `crawl all`, launch one agent per competitor **in parallel**.

### Audit

Run queries from [queries.sql](queries.sql) and present dashboard:

```
=== Competitive Matrix Audit: izuma-edge ===

Coverage:  127 features × 4 competitors = 508 assessments
  Filled: 412 (81%)  |  Unknown: 96 (19%)

Win/Loss:
  vs Competitor A:  45W / 23L / 31T / 28?
  vs Competitor B:  38W / 31L / 25T / 33?

Staleness:
  Competitor B: 47 days (STALE)

Suggested Actions:
  1. Crawl Competitor B
  2. Fill 96 UNKNOWN assessments
  3. Tag 62 untagged features
```

### Tag

```
/competitive-intel izuma-edge tag ppt category=Performance verdict=BETTER
```

1. Query matching features
2. Show matches, ask confirmation
3. INSERT `feature_tags`

### Export

| Format | Command | Output | Description |
|---|---|---|---|
| TSV | `export tsv` | `products/<product>-competitive-matrix-kitchensink.tsv` | Full pivot matrix, git-trackable |
| Web | `export web-detailed` | `products/<product>-competitive-web-detailed.md` | Features tagged `web-detailed`, grouped by category |
| PPT | `export ppt` | `products/<product>-competitive-ppt.md` | Top wins tagged `ppt`, by importance |
| XLSX | `export xlsx` | `products/<product>-competitive-matrix-kitchensink.xlsx` | Full matrix with conditional formatting (green/red/yellow) |

TSV cell format: `STATUS | detail text`

## Research Sources

| Source | What to Look For |
|---|---|
| Product website | Features, pricing, integrations |
| Documentation | API capabilities, SDK languages, limits |
| Blog/Changelog | Recent launches, roadmap signals (6 months) |
| Status page | Uptime SLA, incident history |
| G2/Capterra | Reviews, satisfaction scores |
| GitHub | Open source, community activity |
| LinkedIn | Hiring signals (roles = what they're building) |

## Fairness Rules

1. **Never exaggerate our advantages.** Use objective evidence.
2. **Never downplay competitor strengths.** If they beat us, document it.
3. **Always cite sources** — set `source_url` and `researched_at` on assessments.
4. **Mark uncertainty.** Use `UNKNOWN`, never guess.
5. **Include context.** "200+ integrations vs our 45 (2026-03)" > "they win".
6. **Separate fact from opinion.** Quantify where possible.
7. **Set confidence.** `high` = official source, `medium` = inferred, `low` = secondhand.

## Rules

1. **SQLite is the source of truth.** Exports are generated artifacts.
2. **Always `PRAGMA foreign_keys=ON`** before writes.
3. **Kitchensink is internal** — brutally honest.
4. **Tags drive downstream** — exports are filtered views of the matrix.
5. **Use AskUserQuestion** before writing crawl findings.
6. **Parallel research** — one agent per competitor simultaneously.
7. **Export TSV after changes** — keep a git-trackable copy.
8. **Dynamic columns** — never hardcode competitor names in queries.
