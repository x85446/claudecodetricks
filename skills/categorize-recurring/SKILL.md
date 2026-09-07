---
name: categorize-recurring
description: Find name-fragmented recurring transactions, detect gaps, suggest vendor linking, and normalize item names. Use when someone says 'find recurring', 'match transactions', 'vendor linking', 'name fragmentation', or 'normalize names'.
argument-hint: [scan|backfill|suggest|gaps]
---

# Recurring Transaction Matcher

Merchant-aware recurring charge analysis. Backfills merchant_ids, detects gaps in recurring patterns, and suggests new merchant_patterns for unresolved transactions.

If `$ARGUMENTS` is provided, interpret it as the action.

## Quick Reference

```bash
# Full scan — backfill + report gaps + unresolved clusters
python3 scripts/recurring_matcher.py

# Just backfill merchant_ids using existing patterns
python3 scripts/recurring_matcher.py --backfill-only

# Show suggested new merchant_patterns for gap-filler transactions
python3 scripts/recurring_matcher.py --suggest

# Apply a specific pattern to a merchant (inserts pattern + backfills matching txns)
python3 scripts/recurring_matcher.py --apply-pattern <merchant_id> '<pattern>'

# JSON output
python3 scripts/recurring_matcher.py --json

# Limit output
python3 scripts/recurring_matcher.py --max 10
```

## Database

- **Path**: `db/personaldb.sqlite`
- **Key tables**: `merchants`, `merchant_patterns`, `transactions`
- Uses python3 (sqlite3 CLI may not be available)

## How It Works

### Phase 1: Backfill (always runs first)
Resolves `merchant_id` for transactions where it's NULL by running all `merchant_patterns` (LIKE + exact) against unresolved transactions. Uses priority ordering and JOIN to skip orphaned patterns.

### Phase 2: Merchant-grouped recurring detection
Groups resolved transactions by `merchant_id + ROUND(price, 2)` — transactions from the same merchant at the same price are treated as the same recurring charge regardless of item name variations.

### Phase 3: Gap detection
For each merchant+price group with 3+ transactions, calculates median date interval and flags gaps > 2x median. Only considers charges with interval >= 7 days (weekly or less frequent).

### Phase 4: Gap-filler search
For each gap, searches for unresolved transactions at the same price in the gap period **whose item name is similar** to the merchant's known item names. This prevents false positives like matching random $25 charges to Starbucks.

### Phase 5: Unresolved clustering (fuzzy fallback)
For transactions with NO merchant_id, clusters by price + name similarity (Jaccard tokens + prefix matching). These represent potential new merchants that need to be created.

## Workflow

### `scan` (default)
Run full analysis. Output has three sections:
1. **Backfill results** — how many newly resolved
2. **Merchant groups with gaps** — sorted by number of gap-filler candidates
3. **Unresolved name clusters** — recurring charges needing new merchants

### `suggest`
Show INSERT statements for new merchant_patterns that would capture gap-filler transactions.

### For unresolved clusters
Create new merchants and patterns manually:
```sql
-- Create merchant
INSERT INTO merchants (name, name_normalized, default_tier1_id, default_tier2_id, default_tier3_id, created_at)
VALUES ('ATM Fee Refund', 'atm fee refund', <t1_id>, <t2_id>, <t3_id>, datetime('now'));

-- Add patterns
INSERT INTO merchant_patterns (merchant_id, pattern, pattern_type, priority, source, created_at)
VALUES (<id>, '%ATM FEE REFUND%', 'like', 0, 'recurring_matcher', datetime('now'));
INSERT INTO merchant_patterns (merchant_id, pattern, pattern_type, priority, source, created_at)
VALUES (<id>, '%ATM Refund%', 'like', 0, 'recurring_matcher', datetime('now'));
```

Then re-run: `python3 scripts/recurring_matcher.py --backfill-only`

Or use the shortcut: `python3 scripts/recurring_matcher.py --apply-pattern <merchant_id> '%PATTERN%'`

## Important Rules

- Always backfill before reporting (the script does this automatically)
- **Dual-column sync**: When `--apply-pattern` sets categories from merchant defaults, it updates BOTH text columns (category, sub_category, sub_sub) AND FK columns (tier1_id, tier2_id, tier3_id)
- Gap-filler suggestions require item name similarity to avoid false positives
- The script skips orphaned merchant_patterns (merchant deleted but pattern remains)
- After changes, remind user to restart the web server to see updates
