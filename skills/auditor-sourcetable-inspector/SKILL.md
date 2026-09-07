---
name: auditor-sourcetable-inspector
description: Use to inspect a src_* source table (src_lowes, src_amex, src_amazon, …) for data-quality errors — missing fields, price math violations, duplicates, orphans, source-file mismatches — and write a punch list. Child of the auditor meta skill. Informs only, never repairs.
argument-hint: [src_lowes | lowes | all]
---

# Auditor: Source-Table Inspector

Black-box quality inspection of one `src_*` table (or all of them). Compares the table against `transactions`, against itself (internal math), and spot-checks against original source files. Output is a punch list; **no repairs, ever** (see the auditor meta's Hard rules — they apply verbatim here).

DB: `db/personaldb.sqlite` (WAL; read-only access — do not open write connections while the app may be running).

## Steps

1. **Resolve the target.** Map an arg like `lowes` to `src_lowes`. `all` loops every `src_*` table. Read the table's schema (`PRAGMA table_info`) and its join key to `transactions` (usually `transaction_id`).

2. **Run the registry first.** Read `.claude/data/auditor/error-types.md`. Execute the detection SQL of EVERY entry that is applicable to this table (entries say which sources they've been seen in — applicable means the SQL's columns exist here, not only sources where it was previously found; that cross-source sweep is the point). Record current counts vs. counts at discovery.

3. **Generic battery** (adapt column names to the table):
   - **Field census by year cohort:** for each column, `SUM(col IS NULL OR col='')` grouped by `substr(t.date,1,4)`. Cohort-shaped gaps (one year fully missing a field others have) indicate an importer regression, not source-data absence.
   - **Price math vs transactions:** compare `abs(t.price)` to the source's net+tax (mind the sign convention: `transactions.price` is negative for expenses, src amounts positive). Tolerance $0.02.
   - **Internal line math:** `quantity × unit_price` vs line total, allowing for `discount_amount`.
   - **Duplicates:** group by natural key (date, price, item id, order/transaction number) having count > 1.
   - **Orphans, both directions:** src rows whose `transaction_id` is NULL/dangling, and `transactions` rows with `site = '<source>'` lacking a src row.
   - **Date sanity:** rows outside the source's plausible date range; date drift between src and transactions where both exist.
   - **Document coverage (source_documents tracker):** every distinct src order/receipt ref has a source_documents row (`SELECT ... FROM src_<t> WHERE ref NOT IN (SELECT doc_ref FROM source_documents WHERE site=...)` = 0 — re-run scripts/migrate_source_documents.py if not); every status='have' row's file_path exists on disk (container_path rows: the container exists); statement files on disk all appear in account_documents (path-drift check: dangling file_path = 0).
   - **Source-file spot check:** if original files exist under `data/`, pick ~10 random rows and verify field-by-field against the file. Systematic mismatch → new error type.

4. **Exploratory pass.** Sample 20–30 rows across cohorts and eyeball for anything the battery missed (truncated text, encoding junk, swapped columns, unit weirdness). This is where NEW error types come from.

5. **Write the punch list** to `.claude/data/auditor/punchlists/YYMMDD-<table>.md` using the template below. Then **update the registry**: add an `ET-NNN` entry for each new error type (next free number), with detection SQL and a `Sweep:` line naming other tables to check it against; update `Last checked` / current counts on existing entries.

6. **Report** a summary to the user/meta: per error type — ET id, severity, count, 3 sample row ids, and which skill owns the fix. Do not paste the whole punch list into chat.

## Punch list template

```markdown
# Punch list — <table> — YYYY-MM-DD
Rows: N | Distinct txns: N | Auditor run by: auditor-sourcetable-inspector

## Findings
### ET-001 <name> — SEVERITY — N rows
Detection: `<sql>`
Sample ids: ...
Owner: /importer (or /categorize, /downloader)
Recommendation: <what the owning skill should do — NOT done here>

## New error types registered this run
- ET-00X <name> → registered in error-types.md, sweep scheduled for: <tables>

## Carried from previous punch lists
- ET-00Y — still N rows (was M)

## A-flags
(none | a3 set on N rows, reasons in a3_notes)
```

## Notes

- **Money-shape findings require the funding-instrument cross-check before HIGH confirmation** (registry LESSON-001): match the candidate group's totals against the actual card/bank/paypal charge (±3 days). Sum-of-lines matches → data correct, close the finding. A same-value-repeated column may be line-level, not receipt-level (godaddy ET-008 false positive).
- Severity: HIGH = wrong values stored (bad price math, swapped fields); MED = missing data the source file contains; LOW = missing optional metadata (urls, payment method) or cosmetic.
- One row can hit several error types; count it in each.
- If a registered error type's count DROPPED to zero, say so — that's a fix confirmation the owning skill will want.
- Never assume a NULL is an error: check whether the source file even carries that field for that era before calling it MED.
