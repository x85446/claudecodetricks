---
name: "product-competitors"
description: "PRODUCT child (invoked via $product): knows the market — deep-crawls competitor and own-product sites, maintains a fair competitive feature matrix, and renders docs/competitors.md. Also standalone for marketing: competitor matrix, feature scoring, tagging, TSV/web/ppt/xlsx/collateral export."
---



<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# $product-competitors — know the market

**Version:** product family 1.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Two jobs, one body of knowledge: the competitive landscape for the **product** family's `docs/competitors.md`, and the detailed matrix that marketing works from. Internal only, and brutally honest — every competitive fact lives here whether we win or lose.

## Usage

Argument: "<product> [init|discover <url>|add-competitor <name>|crawl <competitor|all>|audit|tag|matrix|render|export <format>]". `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$product` — ported.
- `$product-features` — ported.
- `$product-roadmap` — ported.

## Invocation

```
$product-competitors <product>                          # Show status
$product-competitors <product> init                     # Initialize a product
$product-competitors <product> discover <url>           # Deep-crawl a site into structured knowledge
$product-competitors <product> add-competitor <name>    # Add a competitor
$product-competitors <product> add-feature <name>       # Add a feature row
$product-competitors <product> crawl <competitor>       # Research one competitor
$product-competitors <product> crawl all                # Research all, in parallel
$product-competitors <product> audit                    # Gaps, staleness, win/loss
$product-competitors <product> tag <tag> [filters]      # Tag rows for downstream use
$product-competitors <product> matrix [filters]         # Show the matrix
$product-competitors <product> categories               # Show/edit category hierarchy
$product-competitors <product> render                   # Write docs/competitors.md + docs/competitive_features.md
$product-competitors <product> export <format>          # tsv|web-detailed|web-summary|ppt|xlsx|one-pager|multi-pager|whitepaper
```

## Two storage modes — detect before doing anything

**Decide once, at the top of every invocation, and say which mode you're in.**

| Mode | When | Storage | Verbs available |
|---|---|---|---|
| **Matrix** | `.claude/db/marketing.sqlite` exists, or the project has a `products/` tree (the marketing repo) | SQLite, shared with `feature-tracker` | all of them |
| **Document** | Anything else — a normal code repo running the `product` family | `docs/competitors.md`, `docs/competitive_features.md` + `.claude/product/competitors/research/*.md` | `discover`, `crawl`, `audit`, `render` |

Document mode is not a degraded fallback to apologize for — it is the mode the `product` family runs in. Never create `marketing.sqlite` in a repo that didn't already have it, and never require it before answering.

## Database (matrix mode)

**Path:** `.claude/db/marketing.sqlite` (shared with `feature-tracker`). Schema in [schema.sql](schema.sql), key queries in [queries.sql](queries.sql).

Always `PRAGMA foreign_keys=ON;` before writes. Initialize with `schema.sql` on first run.

| Table | Purpose |
|---|---|
| `our_products` | Our products being tracked (e.g. izuma-edge) |
| `competitors` | Competitor products, scoped to one of ours (`product_id` FK) |
| `features` | Competitive feature rows, scoped to one of our products |
| `our_assessments` | How OUR product does on each feature (one per feature) |
| `competitor_assessments` | How each COMPETITOR does (one per feature × competitor) |
| `tags` | Tag registry (web-detailed, ppt, sales-battlecard, …) |
| `feature_tags` | Many-to-many link between features and tags |
| `crawl_log` | Research history per competitor |
| `v_comparison` | VIEW — derives BETTER/WORSE/EQUAL from assessment pairs |

### Status values

| Status | Meaning |
|---|---|
| `YES` | Fully supported |
| `NO` | Not supported |
| `PARTIAL` | Limited / caveats |
| `UNKNOWN` | Not yet researched |
| `N/A` | Not applicable |
| `PLANNED` | On roadmap, not shipped |
| `BETA` | In preview |

Competitor assessments carry factual statuses only. `v_comparison` derives the BETTER/WORSE/EQUAL verdict by comparing ours against theirs — never hand-write a verdict.

### Tags

| Tag | Purpose |
|---|---|
| `web-detailed` | Detailed competitive web page |
| `web-summary` | High-level summary page |
| `ppt` | Presentation-worthy wins (top 5–10) |
| `ppt-risk` | Where competitors beat us (internal strategy) |
| `sales-battlecard` | Key differentiators for sales |
| `rfp` | Common RFP questions |
| `roadmap-gap` | Gaps to close — feeds `$product-roadmap` |
| `stale` | Needs re-research |

Custom tags are freeform — INSERT into `tags`.

## The research engine

`discover` and `crawl` share one engine: **three agents in parallel**, each scoped to the same domain rules. This is the same team for our own product and for a competitor's — the only difference is where the findings land.

Launch all three in a single message so they run concurrently. Each receives the target URL, the product name, existing research for context, and the domain rules below.

**Agent 1 — Site crawler** (`general-purpose`):
> Fetch [url]. Extract all meaningful content (text, headings, structure). Find internal links and links to sibling domains, follow them, and follow links from those pages too — aim for comprehensive coverage. Pace yourself between fetches; do not hammer the site. Skip images, video, PDFs, login pages and forms. Return a structured dump organized by page URL: title, URL, full text, links found (noting which you followed), and a site navigation map.

**Agent 2 — Feature extractor** (`general-purpose`):
> Fetch [url] and follow links within the same domain realm. Catalog every product feature, capability and technical specification. For each: name, what it does, technical detail if available, and which tier/edition it belongs to. Look at feature pages, documentation, API references, changelogs and comparison pages. Organize into logical categories. Return a structured list with a source URL for every feature, and a status for each — YES, NO, PARTIAL — never a guess.

**Agent 3 — Positioning analyst** (`general-purpose`):
> Fetch [url] and follow links within the same domain realm. Extract messaging and positioning: customer benefits (outcomes, not features), target personas, use cases, testimonials and case studies, differentiation claims, pricing model if public, and the tone of the copy. Look at landing pages, about pages, case studies, blog posts, press releases and pricing. Return a structured analysis by value proposition, persona, use case, positioning and tone, with source URLs.

### Domain rules (all three agents)

- **In scope:** the target domain and sibling domains of the same organization (`izumanetworks.com` and `izuma.com` are both in scope).
- **Out of scope:** unrelated third parties (github.com, kubernetes.io, docker.com). For well-known technologies, note what was referenced and use your own knowledge instead of crawling.
- **Pacing:** brief pauses between fetches.
- **No interaction:** never submit a form, sign up, or contact anyone.
- **Skip:** login-gated content, binary downloads, audio/video.

### Where findings land

1. Compile all three agents' output into one research file: `.claude/product/competitors/research/<name>.md` (document mode) or the same path in the marketing repo (matrix mode), with sections Site map · Features & capabilities (categorized, source-cited) · Positioning (value props, personas, use cases, differentiation, tone) · Raw content index.
2. **Present findings with a plain numbered-list question before writing any assessment.** Research is a proposal, never a commit.
3. Matrix mode: map findings onto existing features or propose new ones, then UPDATE/INSERT assessments with `researched_at = date('now')`, `UPDATE competitors SET last_crawled = date('now')`, and INSERT into `crawl_log`.
4. Document mode: the research file is the record; `render` turns it into `docs/competitors.md` and `docs/competitive_features.md`.

If a crawl is blocked or fails, report exactly what was reachable and what wasn't. Never silently skip content.

## Workflows

### Init (matrix mode)

1. Create the database, run [schema.sql](schema.sql).
2. INSERT our product into `our_products`.
3. Ask who the main competitors are — name, website, tier (primary/secondary/emerging) for each — and INSERT them.
4. Ask for feature categories, INSERT into `categories`.
5. Generate the empty TSV export.

### Add competitor

1. Ask for full name, website, tier, notes.
2. INSERT into `competitors`, linked to our product.
3. INSERT `competitor_assessments` at `UNKNOWN` for every existing feature.
4. Suggest a crawl.

### Add feature

1. Show existing categories, ask which (or create one).
2. Ask importance: critical / high / medium / low / nice-to-have.
3. INSERT into `features`; INSERT `our_assessments` with our status.
4. INSERT `competitor_assessments` per competitor — ask, or `UNKNOWN`.
5. Ask for tags, INSERT `feature_tags`.

### Audit

Run the queries in [queries.sql](queries.sql) and present:

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

Document mode audits the research files instead: which competitors have no research, which research is older than 90 days, and which claims in `docs/competitors.md` carry no source URL.

### Tag

```
$product-competitors izuma-edge tag ppt category=Performance verdict=BETTER
```

Query matching features, show the matches, confirm, then INSERT `feature_tags`.

### Render — `docs/competitors.md`

The `product` family's stage-1 output. One page, written for whoever reads `docs/prd.md` next:

```markdown
# Competitive Landscape — <product>

**As of:** <date>  ·  **Competitors assessed:** <n>

## Market summary
<3–6 sentences: who plays here, how they segment, where the gaps are.>

## Competitors
### <Name> — <tier>
**What they are:** <one line>  ·  **Site:** <url>
**Strengths:** <bulleted, specific, quantified where possible>
**Weaknesses:** <same standard — no strawmen>
**Pricing:** <model, or "not public">

## Feature comparison
| Capability | Us | <A> | <B> | Notes |
|---|---|---|---|---|
| … | YES | PARTIAL | NO | quantified detail + source |

## Where we lose today
<The honest list. This section is the reason the doc exists.>

## Table stakes vs differentiators
**Table stakes** (must have to be considered): …
**Differentiators** (why they'd pick us): …

## Open questions
- [NEEDS CLARIFICATION] <what research could not settle>

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

In matrix mode, `render` derives every cell from `v_comparison` rather than restating prose. Features tagged `roadmap-gap` are listed under "Where we lose today" and carried into `$product-roadmap`.

The "Table stakes vs differentiators" section of this document is a summary of the next file, never an independent opinion.

### Render — `docs/competitive_features.md`

The second output of `render`, and the one `$product-features` actually reads. A capability × competitor matrix with one derived number per row — **coverage**, the share of competitors that have it — and a class that follows mechanically from the number. Table stakes stops being a judgment call and becomes arithmetic.

**Coverage** = (YES + ½·PARTIAL) ÷ (competitors assessed), where assessed excludes `UNKNOWN` and `N/A`. A row with more than 30% of competitors `UNKNOWN` is classed `insufficient evidence` regardless of its number — it needs a crawl, not a verdict.

| Class | Coverage | Meaning |
|---|---|---|
| **table-stakes** | ≥ 70% | Required to be considered at all. Every one becomes a `Must` feature at stage 3. |
| **contested** | 30–70% | Common but not universal — a real choice. |
| **differentiator** | < 30% | Rare in the market. Where we can win, or something nobody wants. |
| **insufficient evidence** | — | Too many `UNKNOWN`s to classify. |

The 70% line is the default and is stated in the file header; a product with two competitors or twenty may set it differently, but the number used is always written down.

```markdown
# Competitive Features — <product>

**As of:** <date>  ·  **Competitors assessed:** <n>  ·  **Table-stakes threshold:** 70%

| id | Capability | <A> | <B> | <C> | <D> | Coverage | Class |
|---|---|---|---|---|---|---|---|
| CF-01 | Maintains a table of contents | YES | YES | YES | PARTIAL | 88% | table-stakes |
| CF-02 | Concurrent multi-user editing | YES | NO | YES | UNKNOWN | 67% | contested |
| CF-03 | Corpus-derived section templates | NO | NO | NO | NO | 0% | differentiator |
| CF-04 | Offline mode | UNKNOWN | UNKNOWN | YES | NO | — | insufficient evidence |

## Table stakes (≥ 70%)
CF-01, …  — <one line each: what "has it" concretely means, with the source>

## Contested (30–70%)
…

## Differentiators (< 30%)
…

## Insufficient evidence
CF-04 — <which competitors are UNKNOWN, and the crawl that would settle it>

## Open questions
- [NEEDS CLARIFICATION] <a capability whose definition competitors interpret so differently that one row can't hold it>

## Deferred validations
- <what can only be checked once the product or its data exists — carried forward, never a marker>
```

`CF-NN` ids are stable and never reused. `$product-features` cites them as the source of every table-stakes feature (`competitive_features:CF-01`), so a capability that changes class on a re-crawl is traceable forward to the feature it drove.

**Matrix mode:** run the `Competitive Features` query in [queries.sql](queries.sql) with `:stakes = 0.70` (or the threshold the header states) — one row per `features` entry, coverage computed over that product's competitors. Never hand-compute the classes; the query and the file header must agree.

**Document mode:** the feature extractor already returns YES / NO / PARTIAL per capability per site. Take the union of capabilities across every research file, normalize names that are the same thing said differently (state each merge), fill the grid, and mark anything a research file never mentions `UNKNOWN` — never `NO`. Absence of evidence is a crawl gap, not a competitor weakness.

### Export

| Format | Output | Description |
|---|---|---|
| `tsv` | `products/<product>-competitive-matrix-kitchensink.tsv` | Full pivot matrix, git-trackable. Cell format: `STATUS \| detail` |
| `web-detailed` | `products/<product>-competitive-web-detailed.md` | Features tagged `web-detailed`, grouped by category |
| `web-summary` | `products/<product>-competitive-web-summary.md` | High-level summary |
| `ppt` | `products/<product>-competitive-ppt.md` | Top wins tagged `ppt`, by importance |
| `xlsx` | `products/<product>-competitive-matrix-kitchensink.xlsx` | Full matrix, conditional formatting (green/red/yellow) |
| `one-pager` | `products/<product>/one-pager.md` | Value prop, core capabilities, differentiators, comparison table. Dense but readable |
| `multi-pager` | `products/<product>/multi-pager.md` | Features, use cases, architecture, benefits. 4–8 pages equivalent |
| `whitepaper` | `products/<product>/whitepaper.md` | Problem, solution architecture, features in depth, use cases, landscape, deployment. 10+ pages equivalent |

The three collateral formats are written by a writer agent reading the research file plus any existing version of the target document. They emit clean markdown with no brand styling or CSS — `/marketing-doc-formatter` handles styling and PDF conversion separately. Export TSV after any matrix change so a git-trackable copy stays current.

## Research sources

| Source | What to look for |
|---|---|
| Product website | Features, pricing, integrations |
| Documentation | API capabilities, SDK languages, limits |
| Blog / changelog | Recent launches, roadmap signals (6 months) |
| Status page | Uptime SLA, incident history |
| G2 / Capterra | Reviews, satisfaction scores |
| GitHub | Open source presence, community activity |
| LinkedIn | Hiring signals — open roles say what they're building |

## Fairness rules

1. **Never exaggerate our advantages.** Objective evidence only.
2. **Never downplay a competitor's strengths.** If they beat us, document it.
3. **Always cite sources** — `source_url` and `researched_at` on every assessment.
4. **Mark uncertainty — then go clear it.** `UNKNOWN` is a cell that needs a crawl, and this is the stage that crawls. A `[NEEDS RESEARCH]` is a to-do for *you*; it never leaves this stage. `[NEEDS CLARIFICATION]` is only for what the user alone can answer — which competitors matter to *them*, which tier, which market. "Research I owe; still open" is not a disposition this stage may emit: research is done, or its absence is cited with the sources checked and the date.
5. **Include context.** "200+ integrations vs our 45 (2026-03)" beats "they win".
6. **Separate fact from opinion.** Quantify where possible.
7. **Set confidence.** `high` = official source, `medium` = inferred, `low` = secondhand.

## Rules

1. **Declare the mode first.** Matrix or document — say which, then act.
2. **The store is the source of truth; every export and render is a derived artifact.** Never hand-edit an export.
3. **`PRAGMA foreign_keys=ON`** before any write.
4. **The kitchensink is internal.** It exists to be honest, not flattering.
5. **a plain numbered-list question before writing crawl findings.** Always.
6. **Parallel research** — all three agents in one message; one agent set per competitor for `crawl all`.
7. **Never hardcode competitor names in queries** — columns are dynamic.
8. **Never create `marketing.sqlite`** in a repo that didn't have it.
