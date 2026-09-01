---
name: apiaryx-tab
description: Maintain the apiaryx ledger tab in Finance.xlsx. Use when someone asks to update the apiaryx tab, sync the apiaryx ledger, refresh apiaryx vendor balance, or rebuild apiaryx rows from the data tab.
disable-model-invocation: true
argument-hint: [--apply | --reset-from-row N]
---

# apiaryx-tab

Append new charge & payment rows to the `apiaryx` tab in `/workspace/processing/spreadsheet/Finance.xlsx`, sourced from the `data` tab. Idempotent — invoices already present in the apiaryx tab Notes column (col H) are skipped.

## Locations

| Item | Location |
|---|---|
| Spreadsheet | `/workspace/processing/spreadsheet/Finance.xlsx` |
| Worker script | `scripts/sync_apiaryx_tab.py` (relative to this skill) |
| Backup output | `/workspace/processing/spreadsheet/backups/Finance_pre_apiaryx_sync_<UTC>.xlsx` |
| apiaryx tab xml | `xl/worksheets/sheet18.xml` (rId18, sheetId 104) |
| data tab xml | `xl/worksheets/sheet33.xml` (rId33, sheetId 5) |

## apiaryx tab schema (columns A-I)

| Col | Header | Type | Source |
|---|---|---|---|
| A | date | date | data tab K (ACRUAL) for charges; C (Posting Date) for payments |
| B | qtr | number | computed: `ROUNDUP(MONTH(A)/3, 0)` |
| C | year | number | computed: `YEAR(A)` |
| D | entity | string | `Abhishek` / `Suprith` / `Utkarsh` / `Anirudh` / `commission` / `payment` |
| E | charge | number | abs(data tab E) for SPLIT children |
| F | payment | number | data tab E (negative) for DEBIT-* parents |
| G | total | formula | `=G{prev}+SUM(En:Fn)` running balance |
| H | Notes | string | invoice number(s) — single per charge, comma-joined per payment |
| I | month | string | full month name (`January`, `February`, ...) |

## Source mapping (data tab → apiaryx tab)

**Charge rows** (one per invoice):
- Filter: `link == 'apiaryx'` AND `Details == 'SPLIT'` AND R column matches `INV-(?:COM-)?\d\d-\d{5}`
- entity: `site == 'apiaryx.fee'` → `commission`; otherwise Title-case the part after `apiaryx.` (e.g., `apiaryx.abhishek` → `Abhishek`)
- date: data tab K (ACRUAL)
- Notes (col H): invoice# parsed from data tab R (Receipt filename)

**Payment rows** (one per ACH/wire to ApiaryX):
- Filter: `link == 'apiaryx'` AND `Details` starts with `DEBIT-` AND has a non-empty `sublink` (col J, e.g., `APX2601`)
- entity: `payment`
- date: data tab C (Posting Date)
- Notes (col H): comma-joined invoice numbers from all SPLIT children whose sublink matches `J + '.'` (e.g., parent `APX2601` → children `APX2601.`)

## Workflow

1. **Run dry-run first** to preview changes:
   ```
   python scripts/sync_apiaryx_tab.py
   ```

   Output lists every row that would be added (date, entity, charge/payment, Notes) and prints `would add N rows` without writing.

2. **Apply when the dry-run looks right**:
   ```
   python scripts/sync_apiaryx_tab.py --apply
   ```

   - Backs up Finance.xlsx to `backups/Finance_pre_apiaryx_sync_<UTC>.xlsx` first.
   - Appends new rows after the current last filled row.
   - Reports the inserted row range.

3. **Reset & rebuild rows 36+** (preserves rows 1–35):
   ```
   python scripts/sync_apiaryx_tab.py --reset-from-row 36 --apply
   ```

   - Deletes rows 36 through the current last row.
   - Then proceeds as `--apply`, repopulating from the data tab.
   - Use this once to clear the half-finished rows 36–40.

## Idempotency / dedup logic

The script builds a "seen" set of invoice numbers from the apiaryx tab's column H BEFORE deciding what to add. The set is built by:

1. Finding every `INV(?:-COM)?-\d\d-\d{5}` substring in any cell of column H.
2. Expanding shorthand forms like `INV-25-00001,2,3,4,5,6,7` into the full set `{INV-25-00001, INV-25-00002, …, INV-25-00007}`.

A new charge/payment row is only added if its invoice number(s) are NOT in the "seen" set. Re-running the script produces no changes.

## Edge cases & guardrails

- **Rows 1–35 are preserved.** The script's `--reset-from-row` flag rejects values < 36.
- **Older manually-entered rows (no R filename in data tab)** cannot be reconstructed — the skill leaves them alone. New SPLIT rows lacking a parseable `INV-…` invoice# in R are logged and skipped.
- **Orphan SPLIT children** (no parent yet, J=blank) are still added as charge rows. The corresponding payment row is added later, after `apiaryx_splits.py adopt-orphans <parent-row>` populates J on both parent and children.
- **Adopted parents** (J=`APX{YYMM}`) generate a payment row only when at least one matching child invoice# is parseable from the children's R columns.
- **Formula columns (B qtr, C year):** the script writes computed values, not formulas — same convention as later existing rows. Column G is written as a formula (`=G{prev}+SUM(En:Fn)`) to match the running-balance pattern.
- **No formatting/styles applied** to new rows. Excel inherits column-level number/date formatting; if cells display as raw numbers, manually format the new range to match row 35's styles.
- **Never modifies the data tab.** Only reads from it.

## Output report

```
=== apiaryx-tab sync ===
Mode: dry-run | apply | apply (reset from row 36)
Existing apiaryx rows: 178 (last row: 178)
Already-seen invoices: 47
Candidate rows from data tab: 14
  → 11 new charges, 1 new payment, 0 already seen, 2 skipped (no invoice# in R)
Would insert at rows 179–190 (12 new).
Backup: backups/Finance_pre_apiaryx_sync_20260504T031205Z.xlsx
Saved.
```

## Notes

- Companion skill: `fileclerk-data` (which feeds the data tab from incoming-temp). Run `fileclerk-data` first when new ApiaryX invoices are processed.
- Companion script: `fileclerk-data/scripts/apiaryx_splits.py` (orphan add-children & adopt-orphans). The current skill *consumes* what that script produces.
- This skill never deletes from rows 1–35 or from the data tab.
