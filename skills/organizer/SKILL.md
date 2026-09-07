---
name: organizer
description: Use when the user says "process my finances", "check for new data", "run the pipeline", or wants to orchestrate the full personal finance data pipeline (rename, import, categorize). Coordinates the renamer, importer, and categorize skills end-to-end.
argument-hint: [action]
disable-model-invocation: false
---

# Finance Pipeline Organizer

Orchestrator for the personal finance data pipeline. Coordinates `/-renamer`, `/importer`, `/-check-images`, `/categorize`, and `/categorize-recurring`.

If `$ARGUMENTS` is provided, interpret it as the action to perform (e.g., "status", "process", "rename", "import", "audit", "backfill").

## Attitude

**Be methodical and relentless.** Process everything in the pipeline without hesitation. Do not:
- Warn the user about how much work there is
- Ask if they're sure or want to stop
- Suggest saving work for another session
- Describe tasks as "big", "complex", or "ambitious"
- Offer escape hatches ("want to stop here?", "this is a lot")

Just show the status dashboard, then start processing. Work through every file, every vendor, every phase. If you hit an actual error, report it and keep going. The only time to ask the user a question is when you genuinely need a decision (e.g., unknown vendor, ambiguous file) — not to seek permission to continue working.

**One question per decision.** Don't stack multiple questions. Don't re-ask things that were already decided. If the user said "process incoming", that means process ALL of it.

## Database

```
db/personaldb.sqlite
```

Use python3 for all database operations (sqlite3 CLI may not be available):
```bash
python3 -c "import sqlite3; ..."
```

## Pipeline Folders

```
data/incoming/    → data/renamed/    → data/processed/
```

One direction only. Never skip rename. Never move files backward.

## Actions

### `status` — Dashboard

Show pipeline state. Always run this first, but don't ask what to do next — just proceed with whatever action was requested.

```
Pipeline Status:
  incoming/:   N files (N check images, N other)
  renamed/:    N files (breakdown by vendor)
  processed/:  N files

Database:
  transactions: N | uncategorized: N | merchant_id NULL: N
```

### `process` — Full Pipeline Run

Run everything end-to-end. No confirmation needed — the user already asked for it.

1. **Dashboard** — show status (brief, no questions).
2. **Rename** — move incoming → renamed. Check images move as-is. Other files get identified and renamed via `/-renamer` logic. Batch-move same-type files without individual prompts.
3. **Check images** — if renamed/ has `etchk`/`etbillpaycheck`/`etdeposit` files:
   ```bash
   python3 scripts/import_check_images.py data/renamed/
   python3 scripts/import_check_images.py --scan
   # symlink + cleanup
   python3 scripts/stitch_check_images.py
   ```
4. **Import** — process remaining files in renamed/ by vendor. Back up DB first if >50 rows expected.
   - Work through vendors in order: Amazon CSV, eBay XLSX, Chase CSV/PDF, Amex CSV, E*Trade PDF, Lowe's PDF, Home Depot PDF, PayPal CSV, Venmo CSV, Mint CSV, GoDaddy CSV, AWS CSV.
   - For each file: parse it, insert rows, move to processed/. Use `/importer` skill logic for vendor-specific parsing.
   - Show brief progress per file: `✓ 250103-amazon.csv: 271 rows imported`
   - Do NOT ask for confirmation between files. Just process them.
5. **Merchant reconciliation** — run the recurring transaction matcher:
   ```bash
   python3 scripts/recurring_matcher.py
   ```
   This does three things automatically:
   - **Backfill**: resolves merchant_id for any unresolved transactions using existing merchant_patterns
   - **Gap detection**: finds recurring merchant charges with date gaps and checks for name-variant candidates
   - **Unresolved clusters**: identifies recurring charges with no merchant — these need new merchants + patterns
   
   For unresolved clusters: create the merchant and patterns, then re-run backfill:
   ```python
   # Create merchant
   INSERT INTO merchants (name, name_normalized, default_tier1_id, default_tier2_id, default_tier3_id, created_at)
   VALUES ('Merchant Name', 'merchant name', <t1_id>, <t2_id>, <t3_id>, datetime('now'));
   
   # Add patterns (use the suggested patterns from the report)
   INSERT INTO merchant_patterns (merchant_id, pattern, pattern_type, priority, source, created_at)
   VALUES (<id>, '%PATTERN%', 'like', 0, 'recurring_matcher', datetime('now'));
   ```
   
   Then apply with: `python3 scripts/recurring_matcher.py --apply-pattern <merchant_id> '<pattern>'`
   
   Process all unresolved clusters until none remain with obvious patterns.
6. **AI Categorize** — run the full AI suggestion pipeline:
   ```bash
   python3 scripts/suggest_categories.py
   ```
   Then invoke `/categorize suggest` to review and apply with AI judgment.
   The AI reviews each suggestion, approves good ones, overrides wrong ones, skips uncertain ones.
7. **Summary** — show final counts.

**Back up before bulk imports:**
```bash
cp db/personaldb.sqlite db/personaldb.sqlite.bak
```

### `rename` — Rename Only

Process incoming → renamed. No questions unless a file can't be identified.

### `import` — Import Only

Process renamed → DB → processed. No questions unless parsing fails.

### `audit` — Post-Import Audit

Delegate to `/categorize` for: uncategorized transactions, low confidence, company misattribution, plumbing detection, category conflicts.

### `backfill` — Historical Enrichment

Resolve merchant_id, confidence, is_plumbing, company for historical rows.

## When to Ask the User

Only ask when you **genuinely cannot proceed** without input:
- Unknown vendor (file can't be identified)
- Parsing failure (file format unexpected)
- Duplicate conflict that needs a judgment call

Do NOT ask:
- "Ready to continue?" — just continue
- "This is a lot of files, want to split it up?" — no, process them all
- "Which vendor first?" — follow the standard order
- "Are you sure?" — yes, they already said process

## Delegation

| Task | Delegate to |
|------|-------------|
| Vendor identification, file renaming | `/-renamer` logic |
| Check/deposit image pipeline | `/-check-images` scripts |
| File parsing, row import, merchant resolution | `/importer` logic |
| Merchant backfill, recurring gap detection, new patterns | `python3 scripts/recurring_matcher.py` |
| Category audit, company fixes | `/categorize` logic |
