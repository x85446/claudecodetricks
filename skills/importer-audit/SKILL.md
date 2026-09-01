---
name: importer-audit
context: fork
description: Use when someone asks to audit imports, validate source data, black-box audit, verify imported records, or reconcile source files against the database. Runs as the importer skill's post-import audit stage.
argument-hint: [source] e.g. venmo
---

# Importer Auditor (Black-Box Validator)

Independently re-parse original source files and compare against the
`src_*` tables in the database. This is a **black-box audit** — it
intentionally does NOT share any code, logic, or parsing rules with
the `/importer` skill. If the auditor and the importer agree, the
data is trustworthy. If they disagree, the difference is a bug in
one of them (or both).

## Native App Tool

The macOS app has a built-in visual audit tool: **Tools → Source Audit
(Venmo)**. It runs a Rust-native independent parser (`audit.rs` — no
shared code with the importer) and displays results in a side-by-side
panel with color-coded rows:

- **Red** = CSV row missing from DB
- **Orange** = DB row missing from CSV
- **Yellow** = field mismatch (disagreeing field highlighted in bold red)
- **Blue** = duplicate venmo_id

Features: filter checkboxes per kind, right-click → Open Source
Document / Copy Venmo ID / Copy Row Detail.

**When to use the app tool vs this skill:**
- Use the **app tool** for interactive visual review — see both sides
  at a glance, right-click to open source files.
- Use this **skill** (`/importer-audit`) for CLI-driven audits,
  scripted reconciliation, or when the app isn't available.
- Both produce identical results — they're two independent
  implementations of the same audit logic.

**Current vendor support in the app:** venmo only. When a new vendor
is added to `native/personaldb-core/src/audit.rs`, it automatically
becomes available in the app's Tools menu. The Rust module exports
`pdb_audit_<source>()` → Swift calls `RustBridge.audit<Source>()` →
the app shows the same AuditSheet panel with vendor-specific columns.

Adding a new vendor to the app requires:
1. A new `audit_<source>()` function in `audit.rs` (independent CSV/PDF
   parser + comparison against `src_<source>`)
2. A new FFI export in `lib.rs`: `pdb_audit_<source>()`
3. A `RustBridge.audit<Source>()` wrapper
4. A menu item in `MainWindowController.swift` → Tools dropdown
5. The `AuditSheet` is generic — it works for any `AuditResult`, so no
   Swift UI changes are needed per vendor.

## Critical Rule: No Shared Code

**DO NOT** read, reference, import, or copy from:
- `.claude/skills/importer/SKILL.md`
- Any `scripts/*_import*.py` file
- Any existing parser module

You must write your own parsing logic from scratch by reading the raw
source files with the `Read` tool and extracting fields yourself. The
whole point is that two independent implementations catching the same
result = high confidence.

## Database

```
db/personaldb.sqlite
```

Use python3 for all DB operations:
```python
import sqlite3
conn = sqlite3.connect('db/personaldb.sqlite')
conn.execute("PRAGMA foreign_keys=ON")
conn.row_factory = sqlite3.Row
```

## Workflow

### Step 0: Parse the argument

The user invokes `/importer-audit venmo` (or `audit venmo imports`,
etc.). Extract the source name. Currently only `venmo` is supported;
fail with a clear message for other sources.

### Step 1: Discover source files

```python
import glob
files = sorted(glob.glob('data/processed/*-venmo-*.csv'))
```

Report: `"Found N Venmo CSV files to audit."`

### Step 2: Create the temp table

```sql
DROP TABLE IF EXISTS temp_src_venmo;
CREATE TABLE temp_src_venmo (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    venmo_id TEXT,
    datetime TEXT,
    type TEXT,
    status TEXT,
    note TEXT,
    from_user TEXT,
    to_user TEXT,
    amount REAL,
    source_file TEXT,
    row_num INTEGER
);
```

### Step 3: Parse each file independently

For each CSV file, read it with the `Read` tool (or via python
`csv.reader` in a Bash block). Venmo CSVs have this structure:

- Row 0: `Account Statement - (@handle)`
- Row 1: `Account Activity`
- Row 2: Headers (leading empty column): `,ID,Datetime,Type,...`
- Row 3: Balance-only row (skip)
- Row 4+: Transaction data (leading empty column)
- Last rows: footer/disclaimer (skip — no numeric ID in column 1)

For each transaction row:
1. Column 1 = `ID` (venmo_id) — must be a 15-20 digit string
2. Column 2 = `Datetime` — keep as-is (full timestamp)
3. Column 3 = `Type` — Payment, Charge, Standard Transfer, etc.
4. Column 4 = `Status` — Complete, Pending, etc.
5. Column 5 = `Note` — free text
6. Column 6 = `From` — sender name
7. Column 7 = `To` — recipient name
8. Column 8 = `Amount (total)` — parse: strip `$`, `+`, `-`, commas,
   apply sign. `"- $80.00"` → `-80.0`, `"+ $66.00"` → `66.0`.

**Parsing rules (independent of importer):**
- Use `csv.reader` — it always yields strings, no type inference
- venmo_id MUST be stored as a string, never converted to float
- Skip rows where column 1 is not a 15-20 digit number
- Skip rows where all data columns are empty (balance/footer rows)

INSERT each parsed row into `temp_src_venmo`.

### Step 4: Run the comparison

#### 4a. Row counts

```sql
SELECT COUNT(*) FROM temp_src_venmo;
SELECT COUNT(*) FROM src_venmo;
SELECT COUNT(DISTINCT venmo_id) FROM temp_src_venmo;
SELECT COUNT(DISTINCT venmo_id) FROM src_venmo;
```

#### 4b. Missing from DB (in CSV but not in src_venmo)

```sql
SELECT t.venmo_id, t.datetime, t.amount, t.from_user, t.to_user,
       t.note, t.source_file
FROM temp_src_venmo t
LEFT JOIN src_venmo s ON s.venmo_id = t.venmo_id
WHERE s.id IS NULL
ORDER BY t.datetime;
```

#### 4c. Missing from CSV (in src_venmo but not in any CSV)

```sql
SELECT s.venmo_id, s.datetime, s.amount, s.from_user, s.to_user,
       s.note
FROM src_venmo s
LEFT JOIN temp_src_venmo t ON t.venmo_id = s.venmo_id
WHERE t.id IS NULL
ORDER BY s.datetime;
```

#### 4d. Field-level mismatches (both sides have the row)

```sql
SELECT s.venmo_id,
       CASE WHEN s.datetime != t.datetime THEN 'datetime' END AS d1,
       CASE WHEN ROUND(s.amount, 2) != ROUND(t.amount, 2) THEN 'amount' END AS d2,
       CASE WHEN COALESCE(s.from_user,'') != COALESCE(t.from_user,'') THEN 'from_user' END AS d3,
       CASE WHEN COALESCE(s.to_user,'') != COALESCE(t.to_user,'') THEN 'to_user' END AS d4,
       CASE WHEN COALESCE(s.note,'') != COALESCE(t.note,'') THEN 'note' END AS d5,
       s.datetime AS db_dt, t.datetime AS csv_dt,
       s.amount AS db_amt, t.amount AS csv_amt,
       s.from_user AS db_from, t.from_user AS csv_from,
       s.to_user AS db_to, t.to_user AS csv_to,
       s.note AS db_note, t.note AS csv_note
FROM src_venmo s
JOIN temp_src_venmo t ON t.venmo_id = s.venmo_id
WHERE s.datetime != t.datetime
   OR ROUND(s.amount, 2) != ROUND(t.amount, 2)
   OR COALESCE(s.from_user,'') != COALESCE(t.from_user,'')
   OR COALESCE(s.to_user,'') != COALESCE(t.to_user,'')
   OR COALESCE(s.note,'') != COALESCE(t.note,'')
ORDER BY s.datetime;
```

#### 4e. Duplicate venmo_ids in either table

```sql
-- In temp (CSV side)
SELECT venmo_id, COUNT(*) AS cnt FROM temp_src_venmo
GROUP BY venmo_id HAVING cnt > 1;

-- In src_venmo (DB side)
SELECT venmo_id, COUNT(*) AS cnt FROM src_venmo
GROUP BY venmo_id HAVING cnt > 1;
```

### Step 5: Export report

Write `temp/importer_audit_venmo.csv` with one row per discrepancy:

```
kind,venmo_id,field,db_value,csv_value,source_file
```

Kinds: `csv_missing_in_db`, `db_missing_in_csv`, `mismatch_<field>`,
`dup_in_csv`, `dup_in_db`.

### Step 6: Present summary

```
=== Importer Audit: venmo ===
  CSV files scanned:       N
  CSV rows parsed:         N
  DB src_venmo rows:       N
  Unique venmo_ids (CSV):  N
  Unique venmo_ids (DB):   N

  Missing from DB:         N  (CSV rows with no src_venmo match)
  Missing from CSV:        N  (src_venmo rows with no CSV match)
  Field mismatches:        N
  Duplicate IDs (CSV):     N
  Duplicate IDs (DB):      N

  Report: temp/importer_audit_venmo.csv
```

### Step 7: Cleanup

```sql
DROP TABLE IF EXISTS temp_src_venmo;
```

If the user wants to keep the temp table for inspection, skip this
step and note: `"temp_src_venmo left in DB for inspection. DROP TABLE
temp_src_venmo when done."`

## Output Format

Present the summary (Step 6) as a markdown table. For each category
with >0 discrepancies, show up to 5 example rows inline. The full
detail is in the CSV.

## Matching Rules

The Rust comparison engine (`audit.rs`) uses a three-tier matching
strategy. Date and amount are **always exact**; descriptions get a
lighter touch.

### Tier 1: Primary key (exact match)

Sources with authoritative unique IDs (`venmo_id`, `receipt_number`,
`order_id|asin`) match on those keys. If the key exists on both
sides, the row is "found" regardless of description differences.

### Tier 2: Composite key (date + amount)

Sources without unique IDs (chase, amex, etrade) use
`date|amount` as the match key. Description is NOT part of the key
because CSV exports and PDF imports format descriptions differently
(truncation, `&amp;` vs `&`, trailing suffixes like city/state).

### Tier 3: Field-level comparison (fuzzy for text)

Once two rows are paired (by tier 1 or 2), each field is compared:

- **Date**: exact match required
- **Amount**: exact to the penny (rounded to 2 decimals)
- **Text fields** (description, from/to, note): **fuzzy match** —
  considered "close enough" if:
  - One string contains the other (handles truncation)
  - 80%+ of words overlap between the two strings
  - After normalizing: lowercase, decode HTML entities (`&amp;`→`&`),
    collapse whitespace

  Only text differences that fail ALL of these checks are reported
  as mismatches. This prevents hundreds of false positives from
  minor formatting differences between source formats.

### Duplicates

Only flagged when the count of rows with the same source_id
**disagrees** between CSV and DB. If both sides have 4 rows with
the same `order_id|asin`, that's a legitimate multi-quantity order,
not a duplicate. Only `csv x4 vs db x3` (count mismatch) is flagged.

## Guardrails

- **NEVER modify** `src_venmo`, `transactions`, or any production table
- **NEVER reference** importer code — write all parsing from scratch
- **ALWAYS use `temp_*` prefix** for any tables you create
- **ALWAYS DROP** temp tables at the end (unless user asks to keep)
- The audit WILL find differences — that's the point. Present them
  neutrally. Don't assume the importer is wrong; the auditor might
  be wrong too. Label everything as "discrepancy", not "error".
- For amount parsing: `"- $2,019.40"` has a comma AND a minus AND a
  dollar sign. Handle all of them.
- venmo_id is a 19-digit string. If you see scientific notation like
  `3.17e+18` in the DB, that's a known historical bug — report it
  as a discrepancy but don't try to fix it.

## Expanding to Other Sources

When adding a new source (e.g., `amex`, `chase`), update **both** the
CLI skill path and the native app tool:

### CLI skill path (this file)

Add a new section (or supporting file) with:
1. The CSV/PDF structure for that source
2. The `temp_src_<source>` schema
3. The comparison queries (adapted for that source's unique ID field —
   e.g., `src_amex.reference`, `src_ebay.order_number`)
4. Source-specific parsing notes
5. Amount sign conventions (AMEX: positive in PDF = charge → negate)

### Native app path (`audit.rs`)

1. Add a new `pub fn audit_<source>()` in
   `native/personaldb-core/src/audit.rs` with an independent
   CSV/PDF parser for that source (remember: no shared importer code)
2. Add FFI export in `lib.rs`:
   `pub extern "C" fn pdb_audit_<source>() -> *mut c_char`
3. Add `RustBridge.audit<Source>()` in `Bridge/RustBridge.swift`
4. Add a menu item in `MainWindowController.swift`:
   `"Source Audit (<Source>)"` → `#selector(showSourceAudit<Source>)`
5. The `AuditSheet.swift` panel is **generic** — it accepts any
   `AuditResult` and displays it. No per-vendor UI changes needed.
   The `AuditRow` struct carries CSV-side and DB-side fields that
   map to whatever the vendor's columns are (the current field names
   like `csvFrom`/`csvTo` are venmo-specific but the pattern extends
   to `csvDescription`/`csvCardMember`/etc. for AMEX).

### Source-specific unique ID fields

| Source | Unique ID column | Table |
|---|---|---|
| venmo | `venmo_id` | `src_venmo` |
| amex | `reference` | `src_amex` |
| chase | *(none — use date+amount+description)* | `src_chase` |
| amazon | `order_id + item` composite | `src_amazon` |
| ebay | `order_number + item_id` | `src_ebay` |
| paypal | `pp_transaction_id` | `src_paypal` |
| etrade | *(none — use date+amount+description)* | `src_etrade` |

Sources without a unique ID (chase, etrade) require fuzzy matching
by (date, amount, description prefix). These are harder to audit
cleanly — expect more false-positive mismatches.

The pattern is the same everywhere: parse independently, load into
temp, diff against `src_*`, report. Two implementations agreeing =
high confidence.
