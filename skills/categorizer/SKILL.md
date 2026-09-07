---
name: categorizer
description: "Primary categorization orchestrator. Use when someone asks to categorize transactions, fix a merchant, classify descriptions, run categorization on a date range, find miscategorizations, or says 'categorize', 'fix all <merchant>', 'merchantify', 'clean up June', or 'run the categorizer'."
argument-hint: "<instruction: date range | merchant name | 'all' | free text>"
disable-model-invocation: false
---

# Categorizer — the one entry point for merchant, venue, and category assignment

Takes any instruction and drives every in-scope transaction through the full
pipeline: **description → merchant → venue → category**. Python tools do the
mechanical bulk work; **AI reviews every write and owns the final call**. AI may
verify, override, or entirely replace a Python proposal at any time.

**Database:** `db/personaldb.sqlite` (FK columns only: `tier1_id/tier2_id/tier3_id/company_id`;
names resolve via JOIN; `updated_at = datetime('now')` on every UPDATE).

**Child skill routing:** if the instruction mentions links, linking, chains,
link_group, or orphans (e.g. "links for 2026", "my links are needed for 2026
paypal"), invoke `/categorize-linker` via the Skill tool with the remaining
scope as its arguments — do not run the categorization pipeline for a linking
request. `/categorize-linker` builds chain-of-custody links, enforces the PB
invariant, and handles paypal/venmo 3-leg chains.

**Supporting files (read when needed):**
- `rules.md` (next to this file) — the learned rule book. Read it at the START of every run.
- `reference.md` (next to this file) — category tables, cost centers, lookup SQL.

## Interpreting $ARGUMENTS

| Instruction looks like | Scope WHERE clause |
|---|---|
| `2026-06` or `June 2026` or `2026-06-01 to 2026-06-30` | `date BETWEEN ...` |
| `fix all Exxon` / a merchant name | rows whose description matches that merchant's patterns OR merchant_id = that merchant |
| `all` | every row with missing merchant_id, venue, or tier1_id |
| free text | interpret; default to unfinished rows matching the text |
| *(empty)* | all unfinished rows, newest first |

"Unfinished" = `merchant_id IS NULL OR tier1_id IS NULL OR tier1_id = 0` (or the
merchant lacks venue_type). Always exclude nothing by default — but **never modify
a row where `human_verified = 1`** (Stage 5 may flag them, only).

## Pipeline (run stages in order; skip a stage only if its scope is empty)

### Stage 0 — Setup

1. Read `rules.md`.
2. Confirm the app is not writing: this skill writes the live DB. If >50 rows will
   change, back up first: `sqlite3 db/personaldb.sqlite "VACUUM INTO 'db/personaldb.pre-categorizer.$(date -u +%Y%m%dT%H%M%SZ).sqlite'"`.
3. Pick one free A-column (`python3 scripts/ai_flag.py count`) and set it on every
   row you touch this run. Report which column at the end.

### Stage 1 — Merchant (normalize descriptions to one true merchant)

The goal: every description variant collapses to a single merchant. "EXXON BURPY
MARKET AUSTIN TX", "EXXON NEIGHBORHOOD C-STOR", "EXXON TIME WISE # 835" are all
**Exxon**.

1. Survey: `python3 scripts/merchant_hunter.py stats`, then
   `python3 scripts/merchant_hunter.py discover --min-count 1 --limit 50` (scoped
   to the instruction when possible; for small scopes query the rows directly).
2. For each unmatched cluster, AI decides the true merchant:
   - **Existing merchant?** `SELECT id, name FROM merchants WHERE name LIKE '%kw%'` — reuse, never duplicate.
   - **Dwindling-word search** for one-offs: strip words from the description and
     search the DB with fewer and fewer words ("TAQUERIA EL TROMPO MAYOR" → "TAQUERIA")
     to find kin rows and any already-classified precedent.
   - **WebSearch** when the name is still opaque — find what the business actually is.
   - **Franchise vs location**: brand is the merchant (Exxon), not the site (Burpy Market).
3. Apply: `python3 scripts/merchant_hunter.py tag --name "Exxon" --pattern "%EXXON%" [--tier1 N --tier2 N]`
   — prefer broad LIKE patterns; give specific patterns higher priority than general ones.
4. Consolidate duplicates when found: `python3 scripts/merchant_hunter.py consolidate` (review, then `--fix`).

### Stage 2 — Venue

For every merchant in scope with `venue_type IS NULL`, invoke the
**`venue-classifier`** skill (Skill tool) scoped to those merchants. Its vocabulary
is `../venue-classifier/classification_map.json`; it is idempotent and only fills NULLs.
Venue is the keystone: merchant + venue makes categorization mostly mechanical.

### Stage 3 — Category (AI judgment is the mechanism; Python is a draft)

Tool output is a DRAFT, never a result. Nothing lands without an AI decision.

1. Run the pattern engine: `python3 scripts/suggest_categories.py` (writes
   suggestions to the transactions table).
2. Apply deterministic rules from `rules.md` — e.g. the gas-station price split
   (`python3 scripts/gas_or_food.py --dry-run`, review, then run for real).
3. **AI reviews EVERY write** — read the actual item text of every row (or
   every cluster of literally-identical rows) before accepting a tool
   suggestion. At scale, batch by cluster, but never bulk-accept a tool pass
   unread. Track and report the split: N tool-suggested-and-AI-approved,
   M AI-overridden, K AI-direct.
4. **Cost-center resolution** (in order):
   - Merchant `pinned_company_id` set → that cost center, always (pins are for
     merchants whose identity IS the cost center, e.g. GoDaddy-tmctech).
   - NEVER generalize a cost center from isolated charges at a consumer venue.
     One McDonald's charge marked IZUMA was a business trip — the signal is
     row-level (card, ER flag, trip dates), not the merchant. Future
     McDonald's rows stay Personal unless their own row context says otherwise.
   - Recurring business-shaped entries (annual renewals, SaaS, registrations)
     follow their verified recurring precedent even across years.
5. **AI-direct pass over EVERY remaining unfinished row — this is the core of
   the skill, not cleanup.** Item descriptions (Home Depot/Lowe's/eBay product
   text), Venmo memos ("… : groceries", "… : bball"), payees, amounts, dates,
   accounts are all evidence. WebSearch what you don't recognize. A row is only
   left uncategorized when evidence genuinely runs out — and then it is LISTED
   individually in the report with why.
6. Weight precedent from `human_verified = 1` rows; validate tree + formation
   dates; FK columns only; A-flag + `confidence` on every write.

### Stage 4 — Rule discovery

While reviewing, actively hunt for generalizable patterns ("every X at venue Y is
category Z"). When one holds across ≥5 rows with no counterexamples, **append it to
`rules.md`** under Learned rules with a date and the evidence count. Rules make the
next run cheaper — this is the skill's memory.

### Stage 5 — Mistake detection (suggest, never overwrite)

Travis's verified rows are the weights, but he makes mistakes too. Scan in-scope
`human_verified = 1` rows for:
- category conflicting with the merchant/venue majority (e.g. verified "Groceries" at a gas station for $60)
- cost-center formation-date violations
- rule violations from `rules.md`

For business-expense detection in Personal rows, invoke the **`cost-center-review`**
skill (Skill tool) scoped to the run's years — it writes suggestions to the
`cc_suggestions` table for review in the app (Tools → Cost Center Review) and
never edits transactions directly.

**Never change these rows.** Set the run's A-flag on them and list each in the
report as: row, current value, suggested value, why.

### Stage 6 — Report

```
== Categorizer run: <scope> ==
Stage 1 merchants:  N tagged, M consolidated, K new merchants
Stage 2 venues:     N classified (X via websearch, Y unknown)
Stage 3 categories: N python-applied + AI-approved, M AI-overridden, K AI-direct
Stage 4 rules:      N new rules appended to rules.md
Stage 5 flags:      N suspected mistakes in verified rows (listed above)
A-flag: A<n> set on N rows — review in app, then `python3 scripts/ai_flag.py clear A<n>`
Remaining unfinished in scope: N
```

## Toolset

| Tool | Role |
|---|---|
| `scripts/merchant_hunter.py` | discover / consolidate / audit / tag / stats for merchants |
| `venue-classifier` skill | fill merchants.venue_type (controlled vocabulary) |
| `cost-center-review` skill | suggest business cost centers for Personal rows (cc_suggestions) |
| `scripts/suggest_categories.py` | pattern-matched category suggestions |
| `scripts/gas_or_food.py` | gas-station $25 fuel/snacks split |
| `scripts/ai_flag.py` | A-column management |
| WebSearch | identify unknown businesses |
| AI (you) | verify, override, or replace ALL of the above; final authority |

## Guardrails

- **Never modify `human_verified = 1` rows** — flag and suggest only.
- **Never delete transactions; never modify src_* tables.**
- FK columns only; tier1+tier2+tier3 is an atomic triple — never change one alone.
- Category must belong to the row's cost center's tree; respect formation dates
  (see reference.md).
- Never use retired categories (FIXME, Payment, Household, Debt, Fitness & Sports,
  Pet-as-tier1, transfers).
- Venue vocabulary comes from classification_map.json only.
- A-flag every touched row; one A-column per run.
- Backup before >50 changes (Stage 0).
- The old `fox_categorize.py` emit/apply flow is retired — the AI is in-session; do
  the review directly.
