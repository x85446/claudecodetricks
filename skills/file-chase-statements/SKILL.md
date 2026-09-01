---
name: file-chase-statements
description: Auto-file Chase monthly statement PDFs from incoming-temp to processed. Use when someone asks to file Chase statements, move Chase PDFs, or auto-file monthly Chase statements.
disable-model-invocation: true
argument-hint: [--apply]
---

# file-chase-statements

Move Chase monthly statement PDFs (checking/savings balance summaries) from `/workspace/processing/incoming-temp/` to `/workspace/processing/processed/` without any spreadsheet action. These files are balance summaries — they are **not** transaction sources (those come from Chase ALLREC CSVs via `fileclerk-data` Workflow D) and they are not evidence for individual rows. They just need filing.

## Scope

**Strict pattern (only these files are touched):**

```
^\d{6} \$[\d,.\-]+ Chase \d{4} (checking|savings)\.pdf$
```

Examples that match:
- `220630 $0.00 Chase 3211 savings.pdf`
- `240131 $260,813.96 Chase 6557 checking.pdf`
- `230831 $0.00 Chase 3839 checking.pdf`

Anything else (CSVs, non-Chase PDFs, Chase Activity, Chase ALLREC, etc.) is **ignored** by this skill — those have data and belong in `fileclerk-data`.

## Locations

| Item | Location |
|---|---|
| Source folder | `/workspace/processing/incoming-temp/` |
| Destination | `/workspace/processing/processed/` |
| Tracking DB | `/workspace/fintool/.claude/skills/fileclerk-data/data/processed_files.json` |
| Worker script | `scripts/file_chase_statements.py` (relative to this skill) |

## Workflow

1. **Dry-run first** to preview matches:
   ```
   python3 scripts/file_chase_statements.py
   ```

   Lists every file that matches the pattern and would be moved. Reports count. No changes made.

2. **Apply when the dry-run looks right**:
   ```
   python3 scripts/file_chase_statements.py --apply
   ```

   For each match:
   - Move from `incoming-temp/` to `processed/`.
   - If destination already exists, log a collision and **skip** (do not overwrite).
   - Append entry to `processed_files.json` with `status="filed"`, `reason="chase-statement-pdf"`, and a UTC timestamp.

   Reports summary: `filed N, collisions M, skipped K`.

## What this skill does NOT do

- Does not write to `Finance.xlsx`.
- Does not match against data tab rows.
- Does not touch CSV files (those belong in `fileclerk-data` Workflow D).
- Does not touch non-Chase files.
- Does not overwrite existing files in `processed/` — collisions are logged and the source file stays in `incoming-temp/` for manual review.

## Output report

```
=== file-chase-statements ===
Mode: dry-run | apply
incoming-temp/ scanned: 217 files
Matched (Chase statement PDF): 137
  → would file: 137  (or "filed: 137" in apply mode)
  → collisions: 0
Non-matches left for fileclerk-data: 80
```

## Notes

- Run this **before** `fileclerk-data` on each batch. Chase statement PDFs would otherwise show up as `no_match` in Workflow C, which is misleading — they were never meant to match anything.
- The pattern is intentionally tight. If you find a Chase statement filename that doesn't match (e.g., `Chase 9999 statement.pdf` instead of `checking.pdf`), don't expand the regex blindly — first check whether it's actually a balance summary or an Activity export with transactions.
