---
name: importer
description: Import financial data from renamed files into the personaldb SQLite database, with merchant resolution, auto-categorization, duplicate detection, and interactive review.
argument-hint: [filename]
disable-model-invocation: false
---

# Importer Skill

Import financial transaction data from renamed files in `data/renamed/` into the `db/personaldb.sqlite` database.

## Invocation

```
/import <filename>       # Import a specific file from data/renamed/
/import                  # Process the next file in data/renamed/ (alphabetical order)
/import fix source       # Backfill source_tab on existing records from processed files
/import fix source <site> # Fix source_tab for a specific site only (e.g. amex, chase)
/import fix account      # Backfill account on existing records
```

## Run modes

The importer supports two run modes. **This applies to all sources, not just Venmo.**

### 1. `fresh` (default)

INSERT new rows into `src_{site}` and `transactions`. The source's dedup key
prevents re-inserting rows that are already in the DB. Never modifies existing
rows' fields.

Dedup keys per source:

| Source | Dedup key |
|---|---|
| amex | `reference` |
| chase | `(account, date, amount)` + description prefix |
| venmo | `venmo_id` |
| amazon | `(order_id, asin)` |
| paypal | `pp_transaction_id` |
| ebay | `order_number` |
| lowes | `item_number` |
| homedepot | `sku` |
| etrade / godaddy / mint / aws / usaa | `(date, amount)` |

User invocations that trigger fresh mode: "import new statements", "process
data/renamed/", `/import <filename>`.

### 2. `update`

**Never inserts.** For every row parsed from the source, look up the matching
row in `src_{site}` using the source's dedup key. If found:

- UPDATE the `src_*` fields that the new parse produced (e.g. `description`,
  `destination`, `funding_source`, any newly captured columns).
- Regenerate `transactions.item` using the current rendering rules for that
  source — BUT **skip**:
  - rows where `human_verified = 1`
  - rows whose current `transactions.item` does **not** look machine-generated
    (i.e. likely user-edited).

If not found, count as `no-match` and move on.

User invocations that trigger update mode: "update the importer rules",
"re-parse without inserting", "backfill venmo item text", `--update-existing`
flag on per-source scripts.

**"Looks machine-generated" heuristic.** Each source defines the historical and
current shapes that the importer has produced, so update-mode can recognize
them and safely overwrite. **If uncertain, DO NOT overwrite.**

- **Venmo**: legacy patterns `to: {name} ({note})`, `from: {name} ({note})`,
  bare `payment`, `xfer...`, and `Standard Transfer: ... → ...`. New patterns:
  `Paid ...`, `Received from ...`, `Paid request from ...`, `Collected from ...`,
  `Transfer to bank...`, `Transfer from bank...`.
- **Chase**: legacy normalized forms ending in `CITY ST` or single-word
  truncations of the raw description. If the item contains punctuation or
  casing the old normalizer never produced, treat as user-edited.
- **Other sources**: if in doubt, leave untouched.

### Implementation notes

- Always open a transaction, compute would-update / would-skip counts first,
  print a dry-run summary, then commit.
- Always make a backup (`cp db/personaldb.sqlite db/personaldb.sqlite.bak`)
  before committing an update run that touches more than a handful of rows.
- Update mode MUST set `transactions.updated_at = datetime('now')` on every
  row it rewrites.

## Database

- **Path**: `db/personaldb.sqlite`
- **CLI**: Use `sqlite3` for all database operations
- **PRAGMA**: Always run `PRAGMA foreign_keys=ON;` before any write operation

### Key Tables

| Table | Purpose |
|---|---|
| `transactions` | Main ledger — columns: id, year, tab, site, date, order_id, item, price, company_id, tier1_id, tier2_id, tier3_id, project_id, source_tab, source_row, merchant_id, is_plumbing, confidence, notes, account, txn_type_id, image_key, business_card, link_group, human_verified, expense_report, row_created_at, created_at, updated_at |
| `src_amazon`, `src_amex`, `src_chase`, `src_etrade`, `src_etrades`, `src_ebay`, `src_paypal`, `src_mint`, `src_venmo`, `src_godaddy`, `src_aws`, `src_lowes`, `src_homedepot`, `src_usaa` | Source-specific raw data tables |
| `src_amazon_giftcard` | Amazon gift card activity (applied, refunds, additions) |
| `src_amazon_payments` | Amazon "Your Payments" card-to-order mapping (which CC paid for which order) |
| `merchants` | Merchant registry — name, name_normalized, default_tier1_id, default_tier2_id, default_tier3_id, is_multi_category |
| `merchant_patterns` | LIKE pattern matching rules for merchant resolution |
| `categories_tier1`, `categories_tier2`, `categories_tier3` | Category hierarchy |
| `company_category_rules` | Valid company + category combinations |
| `import_log` | Tracks each import run (filename, row count, timestamp) |
| `category_overrides` | Intentional exceptions to standard category rules |

### FK Columns Are the Source of Truth

On 2026-04-10 the text mirror columns (`category`, `sub_category`,
`sub_sub`, `company`) were dropped from `transactions`. Only the FK
columns remain: `tier1_id`, `tier2_id`, `tier3_id`, `company_id`.

When you need a human-readable name, **JOIN the lookup table** instead
of reading a text column:

```sql
SELECT t.item, t.price,
       c1.name AS category, c2.name AS sub_category, c3.name AS sub_sub,
       co.code AS company
FROM transactions t
LEFT JOIN categories_tier1 c1 ON c1.id = t.tier1_id
LEFT JOIN categories_tier2 c2 ON c2.id = t.tier2_id
LEFT JOIN categories_tier3 c3 ON c3.id = t.tier3_id
LEFT JOIN companies co        ON co.id = t.company_id
WHERE t.id = ?
```

When you INSERT or UPDATE a row, set only the FK columns:

```sql
UPDATE transactions SET
  tier1_id = ?, tier2_id = ?, tier3_id = ?,
  company_id = ?,
  updated_at = datetime('now')
WHERE id = ?
```

There is no text mirror to keep in sync — attempting to write to
`category` / `sub_category` / `sub_sub` / `company` will fail with
"no such column". The `project` column is still a text column (no
change there) because `project_id` is rarely used and `project` is
the canonical value.

## Workflow

### Step 0: Route Check/Deposit Images

**Before parsing**, scan `data/renamed/` for check/deposit image files:
- Pattern: `\d{6}_(etchk|etbillpaycheck|etdeposit)_\d+_.*\.(jpg|jpeg|png)`

If ANY such files exist, process them **first** using the check-images pipeline:

```bash
# 1. Install Pillow if needed
python3 -c "from PIL import Image; print('Pillow OK')" 2>/dev/null || python3 -m pip install Pillow --break-system-packages

# 2. Import: copy to assets/checks/, register in image_cache, symlink in processed/
python3 scripts/import_check_images.py data/renamed/
python3 scripts/import_check_images.py --scan

# 3. Stitch composites + generate thumbnails
python3 scripts/stitch_check_images.py
```

Show a summary:
```
── Check/deposit images ─────────────────────────
  Imported: N files (M checks, K deposits)
  Composites stitched: X
  Thumbnails generated: Y
```

Then continue to Step 1 for any remaining non-image files in `data/renamed/`.

### Step 1: Parse the Filename

Files in `data/renamed/` follow the pattern `YYMMDD-<vendor>.<ext>`.

Extract:
- **date** from the `YYMMDD` prefix (2-digit year: `250407` = 2025-04-07)
- **vendor** from the name between the hyphen and the extension
- **format** from the file extension (csv, pdf, xlsx, png, jpg)

If no filename argument is provided, pick the first file alphabetically from `data/renamed/`.

### Step 2: Open and Parse the File

Use the **Read tool** for all file types — it handles PDFs, CSVs, XLSX, and images natively. Do NOT use external Python scripts or libraries for parsing.

#### CSV Parsing

Read the file with the Read tool. The first row is the header. Use the header to confirm vendor and column positions.

#### PDF Parsing

Read with the Read tool using the `pages` parameter for large PDFs (e.g., `pages: "1-5"`). Extract transaction lines from the text content.

#### Image Parsing

Read with the Read tool (multimodal). Extract any visible transaction data, dates, amounts.

### Step 3: Vendor-Specific Parsing Rules

Each vendor has a distinct column layout and parsing quirks learned from real data.

#### Amazon CSV

**Headers**: `order date, order id, order url, quantity, description, item url, price, subscribe & save, ASIN, category`

| CSV Column | Maps To | Notes |
|---|---|---|
| `order date` | `date` | Format: `YYYY-MM-DD` |
| `description` | `item` | Product title |
| `price` | `price` | Parse `$29.40` → `-29.40` (negate — expenses are negative) |
| `quantity` | src_amazon.`quantity` | Default 1 |
| `order id` | `order_id` + src_amazon.`order_id` | Format: `###-#######-#######` |
| `ASIN` | src_amazon.`asin_isbn` | |
| `category` | src_amazon.`amazon_category` | Amazon's own category (not ours). Clean separators: `›` → ` › ` |
| `order url` | src_amazon.`order_id_link` | |
| `item url` | src_amazon.`page_url` | |

**Parsing notes:**
- Skip rows where `order date` is empty or doesn't start with a digit (formula rows at bottom)
- **Price formula**: `price = qty * unit_price * 1.0825 * -1` where `unit_price` is the CSV `price` column and `1.0825` is the 8.25% Texas sales tax multiplier. The CSV `price` is pre-tax per item — we store the after-tax total as a negative number.
- Extract `year` from the date: `int(order_date[:4])`
- Store the pre-tax unit price in `src_amazon.item_total` for reference

**Source table INSERT:**
```sql
INSERT INTO src_amazon (transaction_id, row_num, order_date, order_id, order_id_link,
                        title, amazon_category, asin_isbn, page_url, quantity, item_total)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

#### Amex CSV

**Headers**: `Date, Description, Card Member, Account #, Amount, Extended Details, Appears On Your Statement As, Address, City/State, Zip Code, Country, Reference, Category`

Some Amex CSVs have a simpler format: `Date, Description, Amount`

| CSV Column | Maps To | Notes |
|---|---|---|
| `Date` | `date` | Format varies: `MM/DD/YYYY` or `YYYY-MM-DD` — normalize to `YYYY-MM-DD` |
| `Description` | `item` | |
| `Amount` | `price` | Amex: positive = charge (negate to `-`), negative = credit (keep as-is) |
| `Card Member` | src_amex.`card_member` | |
| `Account #` | src_amex.`account_num` | Last 4-5 digits |
| `Extended Details` | src_amex.`extended_details` | Extra merchant info |
| `Appears On Your Statement As` | src_amex.`statement_as` | |
| `Address` | src_amex.`address` | |
| `City/State` | src_amex.`city_state` | |
| `Zip Code` | src_amex.`zip` | |
| `Country` | src_amex.`country` | |
| `Reference` | src_amex.`reference` | |
| `Category` | src_amex.`amex_category` | Amex's own category (not ours) |

**Parsing notes:**
- Amex amounts: charges are positive in CSV → negate to negative for DB. Credits are negative in CSV → keep as-is.
- Some exports include `Card Member` columns for multi-user accounts — store as `user` and `card` in src_amex.
- **MANDATORY: `reference` must be populated for every row.** The reference is Amex's unique transaction identifier and is the primary dedup key. If a CSV row has no reference, flag it as an import error.
- **MANDATORY: `card_member` must be populated for every row.** This identifies which cardholder made the purchase.

**Duplicate detection**: Use `src_amex.reference` as the authoritative dedup key. Before inserting, check: `SELECT transaction_id FROM src_amex WHERE reference = ?`. If a match exists, this is a duplicate — do NOT insert.

**Source table INSERT:**
```sql
INSERT INTO src_amex (transaction_id, row_num, date, description, card_member, account_num,
                      amount, extended_details, statement_as, address, city_state,
                      zip, country, reference, amex_category, user, card)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

#### Amex XLSX (preferred over PDF for April 2024+)

Amex XLSX files contain the same columns as CSV but are downloaded as Excel from the Amex statements page. The "Transaction Details" sheet has a 6-row header block before the data:

- Row 1: `Transaction Details, <card name> / Closing Date : <date>`
- Row 3: Card member name (e.g., `MELISSA MCCOLLUM`)
- Row 5: Account number (e.g., `XXXX-XXXXXX-32007`)
- Row 7: Column headers (same as CSV)
- Row 8+: Transaction data

**Parsing**: Use `python3` with `openpyxl` to read the "Transaction Details" sheet. Skip the first 6 rows (header block). Row 7 is headers, row 8+ is data. Columns are identical to the CSV format above:

`Date, Description, Card Member, Account #, Amount, Extended Details, Appears On Your Statement As, Address, City/State, Zip Code, Country, Reference, Category`

**Multi-card**: A single XLSX file contains transactions for ALL cards on the account (multiple card members and account numbers). Use the `Card Member` and `Account #` columns to distinguish.

**City/State and Extended Details** may contain embedded newlines (`\n`) — strip or replace with spaces when storing.

**XLSX vs PDF priority**: When both XLSX and PDF exist for the same statement period, always import from XLSX — it has structured transaction data that the PDF does not (the PDF requires OCR/text extraction and is less reliable). The PDF is retained for archival only.

**MANDATORY fields**: Same as CSV — `reference` and `card_member` must be populated for every row.

**Duplicate detection**: Same as CSV — use `src_amex.reference` as the authoritative dedup key before inserting.

**Source table INSERT**: Same as Amex CSV above — same `src_amex` table and columns.

#### Amex PDF Statements

Amex PDF statements span Feb 2008 – present. They contain transaction data in a tabular layout that can be extracted with `pdftotext -layout`.

**⚠️ CRITICAL: Use the canonical parser `scripts/amex_pdf_parser.py`.** Do NOT write ad-hoc regex-only parsing. The parser correctly handles per-card attribution which is the #1 source of data quality issues. A previous ad-hoc importer mis-attributed ~11,000 transactions to the primary account holder (Melissa) that should have been on Travis's card.

```python
from scripts.amex_pdf_parser import parse_amex_pdf
txns = parse_amex_pdf('data/processed/YYMMDD-amex.pdf')
# returns list of dicts with date, amount, description, card_number, card_member, etc.
```

**PDF structure (2015+ statements):**

Multi-cardholder Amex accounts (e.g., account holder Melissa with additional cardholders Travis, Alexander, Paxton) store transactions in **per-card sections**. The section structure is:

```
MELISSA MCCOLLUM                 ← Account Holder header (REPEATS on every page)
Account Ending 5-31009           ← Account number (same every page)
...
  Detail
  MELISSA MCCOLLUM
  Card Ending 5-31009            ← Start of Melissa's transaction section
    05/19/21  MURPHY7395  ...   $55.50
    05/20/21  WOODY'S SHAVE ICE ...  $23.00
    ... (Melissa's transactions)

  TRAVIS MCCOLLUM
  Card Ending 5-36016            ← Start of Travis's first card section
    05/20/21  SP*UBIQUITI ...  $756.68
    05/31/21  AIRDNA MARKET ...  $39.95  ← THIS IS TRAVIS, not Melissa!
    ... (Travis's transactions)

  TRAVIS MCCOLLUM
  Card Ending 5-32023            ← Start of Travis's second card section
    ...
```

**CRITICAL RULES**:

1. **The section boundary is `Card Ending X-YYYYY`, NOT the "MELISSA MCCOLLUM" page header.** Page headers repeat the account holder's name on every page but do NOT indicate which card the current transaction is on.

2. **Track a "current card" state** as you walk through the text lines. Update it every time you see `Card Ending X-YYYYY`. Attribute every subsequent transaction to that card until the next `Card Ending` marker.

3. **Card number formats differ between PDF and XLSX/CSV sources:**
   - PDF: `5-36016` (includes sequence prefix)
   - XLSX/CSV: `-36016` or `XXXX-XXXXXX-36016` (normalized)
   - When comparing for dedup: normalize to last 5 digits

4. **Transaction line regex**: `^\s*(\d{2}/\d{2}/\d{2})\*?\s+(.+?)\s+(-?\$?[\d,]+\.\d{2})\s*$`

5. **Old PDFs (2008-2014)** are single-cardholder and have NO `Card Ending` markers. For these, all transactions belong to the account holder (extract from "Prepared For" in header).

**Amount sign**: PDF shows charges as positive → negate to `-abs(amount)` for DB. Credits shown as negative → flip sign (positive in DB).

**MANDATORY: `card_member`**: Determined by the `Card Ending` section the transaction is in, NOT by text on the transaction line itself. Look up the card member name from the ALL CAPS name line immediately before the `Card Ending` marker.

**MANDATORY: `reference`**: Amex PDFs do NOT contain reference numbers in their text. **Prefer XLSX/CSV imports over PDF** whenever possible because they include the reference field. For PDF-only imports, leave `reference` NULL but log a warning. Do NOT invent references.

**Closing date extraction**:
- Newer format: `Closing Date MM/DD/YY` on a single line
- Old format (2008-2010): `Closing Date` header with date on the next line
- Fallback: infer from filename `YYMMDD-amex.pdf`

**Source table INSERT**: Same as Amex CSV — same `src_amex` table and columns. Leave `reference` NULL only for PDF imports where it's genuinely unavailable, but log a warning.

**Reconciliation tool**: `scripts/amex_reconciler.py` — re-runs the parser against existing DB records and fixes mis-attributed cards + removes phantom duplicates. Run `--dry-run` first, then `--apply`. Run this after any bulk AMEX PDF re-import to catch errors.

#### Chase (PDF statements + CSV exports)

Script: `scripts/chase_pdf_import.py` with subcommands `preview`, `import`,
`backfill`. Handles **both** document types; the filename tells which. See
[chase_parsing.md](./chase_parsing.md) for PDF layout samples and edge cases.

**Filename**: `YYMMDD-chase-NNNN.{pdf,csv}`. Parse the `NNNN` to derive
`transactions.account = "Chase-NNNN"` and route the parser. The leading
`YYMMDD` is the statement close date (PDFs) or the export date (CSVs).

**Accounts seen** (last4 → parser):

| Last4 | Type | Cost Center | Parser |
|---|---|---|---|
| 9878 | Credit Card (Southwest Visa) | Personal | credit-card |
| 9956 | Personal Checking | Personal | banking |
| 8073 | Personal Savings | Personal | banking |
| 8110 / 7071 / 7505 / 3839 / 3211 / 6007 / 8051 / 6557 / 7575 | LLC checking/savings | per CLAUDE.md | banking |

**Dedup key** (both doc types): `(account, date, ABS(amount))` plus a
cleaned-description prefix match. Chase documents carry no reliable unique
ID, so `row_num` is always `0` and CSV re-imports of the same period are
detected as duplicates.

**Run modes** — see the top "Run modes" section. The Chase importer
implements both `fresh` (default) and `update` (via `backfill` subcommand).

##### PDF parsing (PyMuPDF, `page.get_text()`)

Text comes out line-by-line. A transaction looks like:

```
MM/DD            <- line matches ^\d{2}/\d{2}\s*$
MERCHANT CITY ST <- one OR MORE lines of description
AMOUNT           <- ^-?[\d,]+\.\d{2}$
```

Rules:

- **Year**: statement close year from the filename `YYMMDD` (20YY). If a
  transaction's month is much greater than the statement month (e.g. Dec
  txns on a Jan 7 statement), subtract 1 from the year.
- **Multi-line descriptions**: keep collecting lines (skipping blanks)
  until an amount line appears. Join with a single space.
- **Sign**: PDFs display charges as positive and credits as negative;
  the importer negates so `price < 0` for charges.
- **Skip**: lines matching flight-detail noise like `^\d{6}\s+\d\s+[A-Z]`.

##### CSV parsing (two shapes)

**Credit-card CSV** (e.g. `240405-chase-9878.csv`)
`Transaction Date, Post Date, Description, Category, Type, Amount[, Memo]`

**Banking CSV** (e.g. `240408-chase-9956.csv`)
`Details, Posting Date, Description, Amount, Type, Balance, Check or Slip #`

For both: trust the `Description` column verbatim for
`src_chase.description` (already clean, no re-wrap needed). Only apply the
`normalize_item` rules below for `transactions.item`. Amount already has
the correct sign — do NOT re-negate.

##### Description rules (BOTH doc types)

- `src_chase.description` = the full description text from the source,
  whitespace collapsed to single spaces. Preserve everything between the
  date column and the amount column, including phone numbers, city/state
  trailers, store numbers, URL-ish fragments, etc.
- `transactions.item` = `normalize_item(description)` — a LIGHT
  normalization:
  1. Collapse whitespace.
  2. Strip trailing phone+state, e.g. ` 512-450-1300 TX`, ` 5128528358 TX`.
  3. Strip trailing ` CITY ST` **only** when the token immediately before
     CITY contains a non-alpha character (digit, `*`, `&`, `#`, `.`, `,`,
     dash). This keeps multi-word merchants intact (`TST* Michelinos Cafe
     Ole San Antonio TX` → unchanged; `RED RIVER BREWING CO RED RIVER NM`
     → unchanged).
  4. Strip trailing 4+ digit store numbers (` #12345`, ` 98765`).

Do NOT greedily eat on dot-containing descriptions (the old regex
`\s+[A-Z][A-Za-z. ]+\s+[A-Z]{2}$` was removed — don't reintroduce it).

##### source_tab assignment (critical — fixes "wrong statement opens" bug)

Chase statement filenames use the statement CLOSE date, so a transaction
dated D lives in the billing cycle that CLOSES on the first close date
`>= D` for that account's last4. Rule:

> `source_tab` = the smallest statement filename-date `>= txn_date`, among
> `data/processed/*-chase-{last4}.{pdf}` for the row's account.

CSV-sourced rows set `source_tab` to the CSV filename directly (CSVs are
themselves the attestation, not a statement cycle). The `backfill`
subcommand re-applies this rule to every existing row; user-verified rows
and CSV rows are skipped so manual fixes survive.

**Source table INSERT:**
```sql
INSERT INTO src_chase (transaction_id, row_num, date, description, amount,
                       post_date, chase_category)
VALUES (?, 0, ?, ?, ?, ?, ?);
```

#### E*TRADE CSV

**Headers**: `TransactionDate, TransactionType, Description, Categories, Amount`

| CSV Column | Maps To | Notes |
|---|---|---|
| `TransactionDate` | `date` | |
| `TransactionType` | src_etrade.`transaction_type` | "CHECK DEPOSIT", "ACH", "Wire", etc. |
| `Description` | `item` | |
| `Categories` | src_etrade.`categories` | |
| `Amount` | `price` | |

**Important:** For CHECK DEPOSIT entries, always show the check number and full description — the user needs this detail to identify the source.

**Source table INSERT:**
```sql
INSERT INTO src_etrade (transaction_id, row_num, transaction_date, transaction_type,
                        description, categories, amount)
VALUES (?, ?, ?, ?, ?, ?, ?);
```

#### E*TRADE PDF Monthly Statements

E*Trade monthly bank statements are PDFs with a consistent format across all years (2007–present). Each PDF covers one calendar month.

**Detection:** File ends in `.pdf` and vendor name is "etrade".

**Extract text using pdftotext:**
```bash
pdftotext "data/renamed/FILENAME.pdf" -
```

**PDF Structure (consistent across all years):**
```
Account: Max-Rate Checking
Statement: February 2026
Statement Summary: 02-01-26 Through 02-28-26
Account: 2017321452
...
Account Activity Summary
Date          Description                                          Amount($)    Balance($)
02/28/26      INTEREST                                             132.26       60,988.16
02/27/26      TRANSFER MONEY TO EXTERNAL Internet transfer to ...  -12,000.00   60,855.90
02/27/26      BILL PAY - WESTERN OAKS POA, INC                    -68.25       72,855.90
02/24/26      Check #920                                           -3,200.00    73,424.15
...
```

**Parsing rules:**
1. Extract statement period from `Statement Summary: MM-DD-YY Through MM-DD-YY` to determine year context
2. Transaction lines follow the pattern: `MM/DD/YY` + description + amount + balance
3. The `pdftotext` output may split each transaction across multiple lines due to column formatting. Reassemble by looking for:
   - Date pattern: `^\d{2}/\d{2}/\d{2}` — starts a new transaction
   - Amount pattern: a negative or positive number (with optional comma grouping) like `-12,000.00` or `132.26`
   - Balance pattern: the last number on the transaction's final line
4. Multi-line descriptions: Some descriptions wrap (e.g., "TRANSFER MONEY TO EXTERNAL Internet transfer to BANK ONE TX - DALLAS DDA account 836637071"). Collect all text between the date and the amount.
5. Extract check numbers from descriptions matching `Check #(\d+)`
6. Extract transaction type from description prefix:
   - `INTEREST` → transaction_type: "Interest"
   - `DIRECT DEBIT - ...` → transaction_type: "Direct Debit", description is everything after `DIRECT DEBIT - `
   - `DIRECT DEPOSIT - ...` → transaction_type: "Direct Deposit", description after `DIRECT DEPOSIT - `
   - `BILL PAY - ...` → transaction_type: "Bill Pay", description after `BILL PAY - `
   - `TRANSFER MONEY TO ...` → transaction_type: "Transfer", keep full description
   - `ATM WITHDRAWAL - ...` → transaction_type: "ATM Withdrawal", description after `ATM WITHDRAWAL - `
   - `ATM FEE REFUND` → transaction_type: "ATM Fee Refund"
   - `Check #NNN` → transaction_type: "Check", store check number
   - `INTEREST RATE CHANGE ...` → skip (not a transaction, amount is 0.00)
   - Other → transaction_type: "Other"

**Year from statement:** The statement header has the full year (e.g., "Statement: February 2026"). Transaction dates in the body use `MM/DD/YY` format — convert YY to full year using the statement year context.

**Amount sign:** PDF amounts already have correct sign — negative for debits, positive for credits. Use as-is for `price` (do NOT negate, unlike merchant purchases).

**Skip non-transaction lines:**
- Lines with amount `0.00` and description containing "INTEREST RATE CHANGE"
- Page headers/footers: "Page N of N", "Account Activity Summary", "Account: Max-Rate Checking"
- Balance Information, Interest Information, Misc Information sections
- Legal text sections ("In Case of Errors...", "PLEASE READ THE IMPORTANT DISCLOSURES")

**Batch processing:** E*Trade PDFs span 2007–2026 (221 files). When importing multiple sequential files:
- Offer to process in chronological batches (e.g., "Process all 2024 statements? (12 files)")
- For high-volume runs, auto-accept high-confidence transactions (confidence >= 0.8) without individual prompts
- Show progress: `[15/221] Processing 240131-etrade.pdf — 28 transactions`

**Source table INSERT:**
```sql
INSERT INTO src_etrade (transaction_id, row_num, transaction_date, transaction_type,
                        description, categories, amount)
VALUES (?, ?, ?, ?, ?, 'Unassigned', ?);
```

#### USAA PDF Statements

USAA checking and savings statements are PDFs. Files are named `YYMMDD-usaa-NNNN.pdf` where NNNN is the last 4 of the account (3982 = checking, 1064 = savings).

**Detection:** File ends in `.pdf` and vendor name starts with "usaa".

**Account from filename**: Parse `YYMMDD-usaa-NNNN.pdf` → account = `"USAA-NNNN"`.

**PDF Structure:**
```
                                                     USAA CLASSIC CHECKING
                                       for Account Number: 0003353982
                                Statement Period: 12/10/2022 to 01/11/2023

Transactions
  Date    Description                                  Debits     Credits    Balance
  12/10   Beginning Balance                                                  $34,969.30
  01/11   INTEREST PAID                                           $0.32      $34,969.62
  01/11   Ending Balance                                                     $34,969.62
```

**Parsing rules:**
1. Extract statement period from `Statement Period: MM/DD/YYYY to MM/DD/YYYY` — the end date determines the year context
2. Transaction lines: `MM/DD` + description + optional debit amount + optional credit amount + balance
3. **Skip** "Beginning Balance" and "Ending Balance" rows — these are not transactions
4. **Skip** rows with description "INTEREST PAID" where amount is $0.00
5. Amount sign: Debits are negative (expenses/withdrawals), Credits are positive (deposits/income). If a Debit amount is present, `price = -abs(debit)`. If a Credit amount is present, `price = +credit`.
6. Year from statement period: transaction dates are `MM/DD` only — append the year from the statement period end date. If a transaction date's month is greater than the end date's month, it belongs to the prior year (statement spans year boundary).

**Account type detection:**
- `USAA CLASSIC CHECKING` or `CHECKING` in filename → account_type = "checking"
- `USAA SAVINGS` or `SAVINGS` in filename → account_type = "savings"

**Common transaction types (extract from description prefix):**
- `USAA FUNDS TRANSFER ...` → transaction_type: "Transfer"
- `INTEREST PAID` → transaction_type: "Interest"
- `USAA P&C PAYMENT ...` → transaction_type: "Insurance Payment"
- `DIRECT DEPOSIT ...` → transaction_type: "Direct Deposit"
- `DEBIT CARD PURCHASE ...` → transaction_type: "Debit Card"
- `ATM WITHDRAWAL ...` → transaction_type: "ATM Withdrawal"
- `ACH ...` → transaction_type: "ACH"
- Other → transaction_type: "Other"

**Plumbing**: Transfers between USAA checking↔savings are plumbing (`is_plumbing = 1`, category = `Finance / transfer-in` or `Finance / transfer-out`). Look for "USAA FUNDS TRANSFER" descriptions.

**Create `src_usaa` table if it doesn't exist:**
```sql
CREATE TABLE IF NOT EXISTS src_usaa (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id INTEGER,
    row_num INTEGER,
    date TEXT,
    description TEXT,
    debit REAL,
    credit REAL,
    balance REAL,
    account_type TEXT,
    account_num TEXT,
    transaction_type TEXT,
    FOREIGN KEY (transaction_id) REFERENCES transactions(id)
);
```

**Duplicate detection**: Use date + amount + description prefix (first 20 chars) since USAA PDFs have no unique reference field. When the same statement period overlaps between sequential statements, check before inserting.

**Source table INSERT:**
```sql
INSERT INTO src_usaa (transaction_id, row_num, date, description, debit, credit,
                      balance, account_type, account_num, transaction_type)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

**Batch processing:** USAA statements cover 2023 (17 files: 12 checking + 5 savings). Process chronologically. Savings accounts are mostly INTEREST PAID entries — very low volume.

#### PayPal CSV

**Headers**: `Date, Time, TimeZone, Name, Type, Status, Currency, Gross, Fee, Net, From Email Address, To Email Address, Transaction ID, Shipping Address, Address Status, Item Title, Item ID, Shipping and Handling Amount, Insurance Amount, Sales Tax, Option 1 Name, Option 1 Value, Option 2 Name, Option 2 Value, Reference Txn ID, Invoice Number, Custom Number, Quantity, Receipt ID, Balance, Address Line 1, Address Line 2/District/Neighborhood, Town/City, State/Province/Region/County/Territory/Prefecture/Republic, Zip/Postal Code, Country, Contact Phone Number, Subject, Note, Country Code, Balance Impact`

**CRITICAL — PayPal Plumbing Detection:**

PayPal creates 4-6 entries per real purchase (authorization, hold, payment, transfer). Most are plumbing. Set `is_plumbing = 1` for:

| Type / Pattern | is_plumbing |
|---|---|
| "General Authorization" | 1 |
| "Temporary Hold" | 1 |
| "Payment Hold" | 1 |
| "Bank Transfer" or "General Withdrawal" | 1 |
| "Authorization" in type | 1 |
| Zero-amount entries | 1 |
| Reversal pairs (same amount, opposite sign, same day) | 1 on both |
| "eBay Auction Payment" | 0 (real purchase) |
| "Express Checkout Payment" | 0 (real purchase) |
| "Subscription Payment" | 0 (real purchase) |
| "Mobile Payment" | 0 (real purchase) |

**Amount handling:**
- `Gross` = the transaction amount to use as `price`
- `Fee` = PayPal fee (store in src_paypal, don't create separate transaction)
- `Net` = Gross - Fee (store in src_paypal)
- Negate for purchases: if gross is negative in CSV, it's a payment out → keep sign. If positive, it's money in → keep sign.
- For debit amounts (purchases), ensure `price` is negative.

**Source table INSERT:**
```sql
INSERT INTO src_paypal (transaction_id, row_num, date, time, timezone, name, type, status,
                        currency, gross, fee, net, from_email, to_email, pp_transaction_id,
                        shipping_address, address_status, item_title, item_id,
                        shipping_amount, insurance_amount, sales_tax,
                        option1_name, option1_value, option2_name, option2_value,
                        reference_txn_id, invoice_number, custom_number, quantity,
                        receipt_id, balance, address_line1, address_line2,
                        city, state, zip, country, phone, subject, note,
                        country_code, balance_impact)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

#### PayPal PDF Statements

PayPal monthly PDF statements contain a summary of account activity. They are low-volume (often 0-3 transactions per month, mostly GoDaddy auto-payments).

**Structure (2 pages per statement):**
- Page 1: "PAYPAL ACCOUNT" section with Account Activity table
- Page 2: "PAYPAL BALANCE ACCOUNT" section (usually empty)

**Transaction fields per row:**

| PDF Column | Maps To | Notes |
|---|---|---|
| `DATE` | `date` | Format: `MM/DD/YYYY` → normalize to `YYYY-MM-DD` |
| `DESCRIPTION` | `item` + src_paypal fields | Multi-line: type, merchant, funding source, amount, currency, transaction ID |
| `CURRENCY` | src_paypal.`currency` | Usually `USD` |
| `AMOUNT` | src_paypal.`gross` | Negative = payment out |
| `FEES` | src_paypal.`fee` | |
| `TOTAL*` | `price` | Amount + Fees. Negate if positive charge. |

**Description parsing**: The multi-line description contains structured data:
```
PreApproved Payment Bill User Payment:
GoDaddy.com, LLC
  USAA - Checking x-3982         5.32
USD
ID: 0KX421492D7360935
```
Extract:
- `type` = "PreApproved Payment Bill User Payment" → src_paypal.`type`
- `name` = "GoDaddy.com, LLC" → src_paypal.`name` + `item`
- `pp_transaction_id` = "0KX421492D7360935" → **dedup key**
- Funding source = "USAA - Checking x-3982" (informational)

**MANDATORY: `pp_transaction_id`** must be extracted from the `ID:` line. This is the PayPal-assigned unique transaction identifier and the primary dedup key.

**Duplicate detection**: Before inserting, check: `SELECT transaction_id FROM src_paypal WHERE pp_transaction_id = ?`. If a match exists, compare data richness using the smart merge rules (see Step 4e). The existing DB row likely has better categorization; the PDF may have a cleaner description.

**Plumbing**: These PDF statements only show completed transactions (no authorization holds, temporary holds, etc.). Most are "PreApproved Payment" type — these are real purchases, NOT plumbing. Only mark as plumbing if the description indicates a bank transfer or PayPal balance transfer.

**Source table INSERT**: Same as PayPal CSV — same `src_paypal` table. Only a subset of columns will be populated from PDF (date, name, type, currency, gross, fee, net, pp_transaction_id).

#### eBay CSV / XLSX

**CSV Headers**: `Order Number, Order Date, Item ID, Seller, Item Name, Item Price, Currency, Order Total, Order Notes, Tracking Number, View Order Detail`

**XLSX Headers** (newer exports): `OrderNumber, OrderDate, ItemID, Seller, ItemName, ItemPrice, Currency, Quantity, OrderTotal, OrderNotes, TrackingNumber, Image URL, View Order Detail`

The XLSX format has two extra columns: `Image URL` and `Quantity`. Both formats are supported.

| Column | Maps To | Notes |
|---|---|---|
| `Order Date` / `OrderDate` | `date` | Format varies: `YYYY-MM-DD` or `Mon DD, YYYY` |
| `Item Name` / `ItemName` | `item` | Product title |
| `Item Price` / `ItemPrice` | `price` | Negate: `-abs(price)` |
| `Order Number` / `OrderNumber` | `order_id` + src_ebay.`order_number` | |
| `Seller` | src_ebay.`seller` | |
| `Item ID` / `ItemID` | src_ebay.`item_id` | |
| `Quantity` | src_ebay.`quantity` | XLSX only. Default 1 for CSV |
| `Order Total` / `OrderTotal` | src_ebay.`order_total` | Parse `US $487.07` → `487.07` |
| `Tracking Number` / `TrackingNumber` | src_ebay.`tracking_number` | |
| `Image URL` | src_ebay.`image_url` | XLSX only. `i.ebayimg.com` CDN URL |
| `View Order Detail` | src_ebay.`view_order_detail` | |

**XLSX Parsing**: Use Python to read XLSX since the Read tool can't handle binary XLSX:
```bash
python3 -c "
import zipfile, xml.etree.ElementTree as ET, json
z = zipfile.ZipFile('data/renamed/FILENAME.xlsx')
sheet = z.read('xl/worksheets/sheet1.xml')
root = ET.fromstring(sheet)
ns = {'s': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
rows = []
for row in root.findall('.//s:row', ns):
    vals = []
    for c in row.findall('s:c', ns):
        is_t = c.find('s:is/s:t', ns)
        v = c.find('s:v', ns)
        vals.append((is_t.text if is_t is not None and is_t.text else v.text if v is not None and v.text else ''))
    rows.append(vals)
print(json.dumps(rows))
"
```

**Source table INSERT:**
```sql
INSERT INTO src_ebay (transaction_id, row_num, order_number, order_date, item_id,
                      seller, item_name, item_price, currency, quantity,
                      order_total, order_notes, tracking_number, image_url,
                      view_order_detail)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

**Image caching**: After importing, auto-cache eBay images into `image_cache`:
```sql
INSERT OR REPLACE INTO image_cache (asin, image_url)
SELECT 'ebay:' || item_id, REPLACE(image_url, 's-l500', 's-l1600')
FROM src_ebay
WHERE image_url IS NOT NULL AND image_url != ''
  AND image_url LIKE '%ebayimg.com%'
  AND 'ebay:' || item_id NOT IN (SELECT asin FROM image_cache WHERE image_url != '');
```
This upsizes thumbnails from `s-l500` to `s-l1600` for full resolution. The review UI will show these via `image_key = ebay:{item_id}`.

#### Home Depot PDF

Home Depot order details come as individual PDF files, one per order. Each PDF contains item-level detail.

**Extract text using pdftotext:**
```bash
pdftotext "data/renamed/FILENAME.pdf" -
```

**PDF Structure:**
```
Order # H6542-714983          (or Receipt # 6570-3-3393)
PO/Job Name: AUSTIN
WillCallFulfillment           (or In-Store Purchase)
Ordered: 1/15/2026

Item Description              Qty   Unit Price   Discount   Net Unit Price   Pre Tax Amount
1 in. x 48 in. Insulating     30    $77.97       $60.00     $0.00            $599.10
SKU 614637
Energy Drink Ultra Zero        2     $3.83        $0.70      $0.00            $6.96
SKU 1001061738

Subtotal / Discount / Tax / Order Total at bottom
```

**Parsing rules:**
1. Extract order/receipt number from `Order #` or `Receipt #` line
2. Extract date from line after `Ordered`
3. Items are multiline: description line, then numbers line, then `SKU NNNN` line
4. Parse each item: description, qty, unit_price, discount, net_unit_price, pre_tax_amount, SKU
5. One transaction per item line (not per order) — each item gets its own `transactions` row

| Extracted | Maps To | Notes |
|---|---|---|
| Date after "Ordered" | `date` | Format: `M/D/YYYY` |
| Item Description | `item` | |
| Pre Tax Amount | `price` | Negate: `-abs(price)` |
| Order/Receipt # | `order_id` + src_homedepot.`order_number` or `.receipt_number` | |
| SKU | src_homedepot.`sku` | Key for image lookup |
| Qty | src_homedepot.`quantity` | |
| Unit Price | src_homedepot.`unit_price` | |
| Discount | src_homedepot.`discount` | |
| "WillCallFulfillment" / "In-Store Purchase" | src_homedepot.`fulfillment_type` | |

**Source table INSERT:**
```sql
INSERT INTO src_homedepot (transaction_id, row_num, order_number, receipt_number,
                           order_date, description, sku, quantity, unit_price,
                           discount, net_unit_price, pre_tax_amount,
                           store_name, fulfillment_type)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

**Batch renaming**: Multiple Home Depot PDFs share the download timestamp pattern `Order_Details_April-02-2026_02_38-AM-N.pdf`. Each contains a different order. The renamer should use the order date from inside the PDF, not the filename timestamp.

**Image pipeline**: After import, run the image-fetcher skill for Home Depot:
- Stage 1: Google search `site:homedepot.com/p "{sku}" {description}` for product URLs
- Stage 2: Scrape product pages for `images.thdstatic.com` CDN URLs
- Stage 3: Generate thumbnails
- Image cache key: `hd:{sku}`

#### Venmo CSV

Venmo CSVs are downloaded per-month via the statement downloader (`make venmo`). There are **two accounts** — Travis and Melissa. The renamer embeds the account in the filename: `YYMMDD-venmo-travis.csv` or `YYMMDD-venmo-melissa.csv`.

**Filename format**: `YYMMDD-venmo-<account>.csv` where `<account>` is `travis` or `melissa`.

**Account extraction from filename**: Parse the third segment: `venmo-travis` → account = "Venmo-Travis", `venmo-melissa` → account = "Venmo-Melissa".

**CSV structure** (from Venmo statement downloads):
- Row 1: `Account Statement - (@Melissa-McCollum-12)` — account identification
- Row 2: `Account Activity`
- Row 3: Headers (leading empty column): `,ID,Datetime,Type,Status,Note,From,To,Amount (total),Amount (tip),Amount (tax),Amount (fee),Tax Rate,Tax Exempt,Funding Source,Destination,Beginning Balance,Ending Balance,...`
- Row 4: Balance-only row (no transaction data — skip)
- Row 5+: Transaction data

**Headers**: `,ID,Datetime,Type,Status,Note,From,To,Amount (total),Amount (tip),Amount (tax),Amount (fee),Tax Rate,Tax Exempt,Funding Source,Destination,Beginning Balance,Ending Balance,Statement Period Venmo Fees,Terminal Location,Year to Date Venmo Fees,Disclaimer`

Note the leading empty column — the `ID` column is index 1, not 0.

| CSV Column | Maps To | Notes |
|---|---|---|
| `ID` | src_venmo.`venmo_id` | Venmo transaction ID — **dedup key**. MUST be read as a **string**, never as a number (see below). |
| `Datetime` | `date` / src_venmo.`datetime` | Format: `YYYY-MM-DDTHH:MM:SS` — extract date portion for `transactions.date` |
| `Note` | src_venmo.`note` | Cleaned: newlines → spaces, internal whitespace collapsed; emojis preserved |
| `Amount (total)` | `price` | Parse `+ $50.00` or `- $2,019.40`. Remove `$`, `+`, commas, spaces. Negative = payment out. |
| `From` | src_venmo.`from_user` | |
| `To` | src_venmo.`to_user` | |
| `Type` | src_venmo.`type` | "Payment", "Charge", "Standard Transfer", "Merchant Transaction" |
| `Status` | src_venmo.`status` | |
| `Funding Source` | src_venmo.`funding_source` | Bank/card that funded the money out, or `Venmo balance` |
| `Destination` | src_venmo.`destination` | Where the money landed (bank account for transfers; `Venmo balance` for incoming) |

**Compound `item` rendering.** `transactions.item` is built from `Type`,
amount sign, `not_us`, `note`, `destination`, and `funding_source`:

| Type | Dir (amount) | `item` format |
|---|---|---|
| Payment | out (amt<0) | `Paid {not_us}: {note}` |
| Payment | in (amt>0) | `Received from {not_us}: {note}` |
| Charge | out (amt<0) | `Paid request from {not_us}: {note}` |
| Charge | in (amt>0) | `Collected from {not_us}: {note}` |
| Standard Transfer | out (amt<0) | `Transfer to bank: {destination}` |
| Standard Transfer | in (amt>0) | `Transfer from bank: {funding_source}` |
| Merchant Transaction | any | `Paid {not_us}` |

Rules:
- If `note` is empty/null, drop the `": {note}"` suffix entirely — no dangling
  colon or trailing space.
- If `destination` / `funding_source` is empty for a transfer, drop the
  `": {name}"` suffix.
- Collapse internal whitespace in `note` to single spaces; replace newlines
  with spaces; preserve case and emojis as-is.

⚠️ **CRITICAL: `ID` must be read as a string, never as a number.**
Venmo transaction IDs are **19 digits** (e.g., `2839767287773266625`), which exceeds
the precision of both Python `float` and SQLite `REAL`. If you read this column into
a numeric type and cast back to text, it comes out as scientific notation like
`3.17817e+18` — and the trailing 13+ digits are **gone forever**. A historical import
corrupted 378 of 564 venmo_id values this way; the recovery script is
`scripts/fix_venmo_ids.py`.

Correct parsing pattern (using the Read tool on the raw CSV, not pandas):
- Use `csv.reader` on the raw text and index the column directly — csv.reader always
  yields strings, no type inference.
- If you must use pandas (don't), pass `dtype={"ID": str}` and `keep_default_na=False`.
- When inserting into `src_venmo`, bind the value as `TEXT` with a Python `str`.
- Verify after insert: `SELECT venmo_id FROM src_venmo WHERE transaction_id = ?`
  should return exactly the 19-digit string from the CSV, character-for-character.

**Determining `not_us`**: The account holder is known from the filename (melissa or travis). The "other party" is whichever of From/To is NOT the account holder. Set `src_venmo.not_us` to that person.

**Dedup**: Use `venmo_id` as the **authoritative** dedup key (same role `src_amex.reference`
plays for AMEX). Before inserting any row, check:
```sql
SELECT t.id FROM src_venmo s
JOIN transactions t ON t.id = s.transaction_id
WHERE s.venmo_id = ?
```
If a row already exists, skip the insert (it's a re-import duplicate). Do NOT fall back
to `(date, amount, from, to)` matching — venmo_id is unique per real-world transaction
and is all you need. Only if `venmo_id` is missing from the CSV for some reason (it
shouldn't be) should you fall back to the old tuple-based match.

**Account**: Set `transactions.account` from the filename suffix: `"Venmo-Travis"` or `"Venmo-Melissa"`.

**Categorization — CRITICAL**: Venmo transactions are categorized primarily by **who was paid** (`not_us`), NOT by the note text. The same person almost always falls into the same category. Before importing, query existing categorizations for the same person:

```sql
SELECT category, sub_category, company, tier1_id, tier2_id, tier3_id, company_id, COUNT(*) as cnt
FROM transactions t
JOIN src_venmo s ON s.transaction_id = t.id
WHERE s.not_us = '<person_name>'
  AND t.category IS NOT NULL AND t.category != ''
GROUP BY category, sub_category, company
ORDER BY cnt DESC
LIMIT 1;
```

If a match exists, apply that category/company to the new transaction with `confidence = 0.85`. This is how the existing data works — Savannah Garcia is always "Kids > Education", Noelia Rodriguez is always "Bills & Utilities > Cleaning", etc.

If the person has multiple categories in history (e.g., Michelle McMillin has Food > Groceries, Travel > Vacations, and Kids > grades), set `confidence = 0.5` and use the most frequent one.

If the person is new (no existing rows), leave uncategorized with `confidence = 0.0`.

**Plumbing:** "Standard Transfer" (bank→Venmo or Venmo→bank) and "Venmo Cashout" are plumbing (`is_plumbing = 1`). Also mark `xfer` in the not_us field as plumbing.

**Source table INSERT:**
```sql
INSERT INTO src_venmo (transaction_id, row_num, venmo_id, datetime, type,
                       status, note, from_user, to_user, not_us, amount,
                       destination, funding_source)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

**Importer script:** `scripts/venmo_import.py`. Supports both run modes:
```
python3 scripts/venmo_import.py data/processed/240601-venmo-travis.csv           # fresh dry-run
python3 scripts/venmo_import.py --apply <csv>                                     # fresh apply
python3 scripts/venmo_import.py --update-existing data/processed/*venmo*.csv      # update dry-run
python3 scripts/venmo_import.py --update-existing --apply data/processed/*venmo*.csv  # update apply
```
The script auto-adds the `destination` and `funding_source` columns to
`src_venmo` on first run if they are missing.

#### Mint CSV

**Headers**: `Date, Description, Original Description, Amount, Transaction Type, Category, Account Name, Labels, Notes`

| CSV Column | Maps To | Notes |
|---|---|---|
| `Date` | `date` | |
| `Description` | `item` | Mint-cleaned description |
| `Original Description` | src_mint.`original_description` | Raw bank description |
| `Amount` | `price` | Negate debits: if `Transaction Type` = "debit" → `-abs(amount)`, if "credit" → `+abs(amount)` |
| `Transaction Type` | src_mint.`transaction_type` | "debit" or "credit" |
| `Category` | src_mint.`mint_category` | Mint's category (not ours) |
| `Account Name` | src_mint.`account_name` | Which bank account |

**Source table INSERT:**
```sql
INSERT INTO src_mint (transaction_id, row_num, date, description, original_description,
                      hide_flag, amount, transaction_type, mint_category, account_name,
                      labels, notes)
VALUES (?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?);
```

#### GoDaddy CSV (Full History Export)

GoDaddy exports a single CSV with the entire purchase history (2003–present). One row per line item — multiple rows share the same `Receipt number` when purchased together.

**Headers**: `Receipt number, Transaction Type, Order date, Product name, Name, ICANN fee, Length, Subtotal amount, Tax amount, Order total, Currency, First name, Last name, Address 1, Address 2, City, State/Province, ZIP/Postal code, Country, Phone, Email, Payment Category, Payment Sub-Category, Account Ending With, Customer number`

| CSV Column | Maps To | Notes |
|---|---|---|
| `Order date` | `date` | Format: `YYYY-MM-DDTHH:MM:SS.000Z` → extract date portion |
| `Product name` | `item` (prefix) | e.g., ".COM Domain Renewal" |
| `Name` | `item` (suffix) + categorization key | Domain name: `TMCTECH.US`, `IZUMANETWORKS.COM` |
| `Order total` | `price` | Negate: `-abs(total)`. This is the per-line-item total, NOT the receipt total. |
| `Receipt number` | `order_id` | Multiple line items share the same receipt number |
| `ICANN fee` | src_godaddy.`icann_fee` | |
| `Length` | src_godaddy.`length` | e.g., "1 Year", "2 Year" |
| `Subtotal amount` | src_godaddy.`subtotal` | |
| `Tax amount` | src_godaddy.`tax_amount` | |
| `Payment Category` | src_godaddy.`payment_category` | "CreditCard" or "Paypal" |
| `Payment Sub-Category` | src_godaddy.`payment_sub_category` | "Amex" or empty |
| `Account Ending With` | src_godaddy.`account_ending_with` | "8012" for Amex |

**Item format**: Combine product + domain: `"{Product name}: {Name}"` → e.g., ".COM Domain Renewal: IZUMANETWORKS.COM"

**Dedup**: Use `receipt_number` + `name` (domain) + `product_name` as a composite dedup key:
```sql
SELECT transaction_id FROM src_godaddy 
WHERE receipt_number = ? AND name = ? AND product_name = ?;
```

**Zero-amount rows**: Some line items have `Order total = 0` (bundled items like "Auctions Membership Renewal" or "Pro Domain Alert Renewal"). Still import them — they're real products, just included free with another purchase.

**Categorization — CRITICAL: domain-name-based**. The `Name` column (domain name) determines the company. Query existing categorizations by domain:

```sql
SELECT t.company, t.company_id, t.category, t.sub_category, t.tier1_id, t.tier2_id, COUNT(*) as cnt
FROM transactions t
JOIN src_godaddy s ON s.transaction_id = t.id
WHERE UPPER(s.name) = UPPER('<domain_name>')
  AND t.company IS NOT NULL AND t.company != ''
GROUP BY t.company
ORDER BY cnt DESC
LIMIT 1;
```

Known domain → company mappings:
- `IZUMANETWORKS.COM`, `IZUMANETWORKS.NET`, `IZUMA.NET` → IZUMA
- `TMCTECH.US`, `FULLLOOP.AI` → TMCTECH
- All family names (`*MCCOLLUM*`, `MELISSAANDTRAVIS.COM`, `PATRICKANDMICHELLE.COM`, etc.) → Personal (`-`)
- Non-domain items (memberships, alerts) → Personal (`-`)

**CC→GoDaddy plumbing**: When the `Payment Category` is "CreditCard" and `Payment Sub-Category` is "Amex", the corresponding Amex CC charge for GoDaddy is plumbing (the GoDaddy line items are the canonical records). The duplicate-detector skill handles this — the importer does NOT mark GoDaddy rows as plumbing.

**Source table INSERT:**
```sql
INSERT INTO src_godaddy (transaction_id, row_num, receipt_number, transaction_type,
                         order_date, product_name, name, icann_fee, length, subtotal,
                         tax_amount, order_total, currency, first_name, last_name,
                         address1, address2, city, state, zip, country, phone, email,
                         payment_category, payment_sub_category, account_ending_with,
                         customer_number)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

#### AWS CSV

**Headers**: `InvoiceId, PayerAccountId, LinkedAccountId, RecordType, BillingPeriodStartDate, BillingPeriodEndDate, InvoiceDate, PaymentMethod, ... (varies)`

| CSV Column | Maps To | Notes |
|---|---|---|
| `BillingPeriodStartDate` or `InvoiceDate` | `date` | |
| Service description | `item` | Constructed from service/charge columns |
| Total amount | `price` | Negate: `-abs(amount)` |
| `InvoiceId` | `order_id` | |

**Source table INSERT:**
```sql
INSERT INTO src_aws (transaction_id, row_num, transaction_date, invoice_id,
                     payment_method, type, currency, amount, billing_period,
                     service_provider, charge_type, purchase_order_id, payer_account_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

#### Lowe's PDF Receipts

Lowe's receipt PDFs (from lowes.com/mylowes/orders/details) contain rich per-item data. These are the **primary import source** for Lowes — they have Item #, Model #, quantity, unit pricing, and descriptions that enable categorization.

**Detection:** File ends in `.pdf` and vendor name contains "lowes".

**PDF structure (consistent across all receipts):**
```
Order Details
Transaction # NNNNNNNNN
Placed Month DD, YYYY
$TOTAL
Completed / Completed Date: ...

Store Name
Street Address,
City, ST, ZIP

[item blocks repeat]

Payment Method
Order Summary
Subtotal / Tax / Total Billed
```

**Order-level extraction (parse once per PDF):**

| Pattern | Field | Regex |
|---|---|---|
| Transaction # | `order_id` + src_lowes.`transaction_number` | `Transaction #\s*(\d+)` |
| Date placed | `date` | `Placed\s+(\w+ \d+,\s*\d{4})` → normalize to YYYY-MM-DD |
| Total billed | (validation only) | `Total Billed\s*\$?([\d,]+\.\d{2})` |
| Store name | src_lowes.`store_name` | First non-empty line after "Completed Date:" line |
| Store address | src_lowes.`store_address` | Lines between store name and first item (contains comma+state+ZIP pattern) |
| Order URL | `links` + src_lowes.`order_url` | `https://www\.lowes\.com/mylowes/orders/details\?[^\s]+` |

**Item-level extraction (repeating blocks):**

Each item follows this pattern (some fields optional):

1. **Description** — one or more lines of product name/specs (text before `Item #` line)
2. **Price(s)** — `$XX.XX` on description lines. If two prices appear, first is original, second is discounted
3. **Discount line** (optional) — `Saved $X.XXwith 10% Military Discount`
4. **Item + Model line** — `Item #NNNNNN Model #XXXXXXXX`
5. **Unit price + qty** — `$XX.XX /ea. QTY N`

**Parsing strategy:**

1. Read all pages of the PDF using the Read tool
2. Extract order-level fields (transaction #, date, store, URL)
3. Find all `Item #(\d+)\s+Model #(\S+)` matches — each marks an item block
4. For each item block, look backwards from the `Item #` line to collect description text
5. Look at the line after `Item #` for the `$XX.XX /ea. QTY N` pattern
6. Look for `Saved \$([\d.]+)` between description and Item # line
7. For prices: if two `$` amounts appear on the same line as description text, first = original_price, second = discounted (actual) price. If only one, it's both.

**Key regex patterns:**
- Item + Model: `Item #(\d+)\s+Model #(\S+)`
- Unit price + qty: `\$([\d,.]+)\s*/ea\.\s*QTY\s*(\d+)`
- Discount: `Saved \$([\d,.]+)`
- Line prices: `\$([\d,]+\.\d{2})` (may appear 1-2 times per description block)

**Price handling:**
- `no_tax_price` = discounted line total (or original if no discount). This is the actual amount paid before tax.
- `price` in transactions = `-abs(no_tax_price)` (expenses are negative)
- `unit_price` = from the `/ea.` line
- `original_price` = first price if two shown (pre-discount)
- `discount_amount` = from "Saved" line

**Edge cases:**
- Description may wrap across 2-3 lines — collect all text lines above `Item #` until hitting a previous item's `QTY` line or the store address block
- Some items have no discount line — `discount_amount` is NULL
- The "Order Details" and page headers (`3/13/26, 12:16 PM`) appear on multi-page PDFs — skip lines matching `^\d+/\d+/\d+,` and `^Order Details$`
- "Payment Method", "Order Summary", "Subtotal", "Tax", "Total Billed" mark end of items
- The `https://www.lowes.com/mylowes/...` URL appears on every page — extract it once

**Mapping:**

| Extracted | `transactions` column | `src_lowes` column |
|---|---|---|
| Description | `item` | `description` |
| Date placed | `date` | — |
| -abs(no_tax_price) | `price` | — |
| Transaction # | `order_id` | `transaction_number` |
| Item # | — | `item_number` |
| Model # | — | `model_number` |
| QTY | — | `quantity` |
| Unit price | — | `unit_price` |
| Original price | — | `original_price` |
| Saved amount | — | `discount_amount` |
| Store name | — | `store_name` |
| Store address | — | `store_address` |
| Order URL | `links` | `order_url` |

**Source table INSERT:**
```sql
INSERT INTO src_lowes (transaction_id, row_num, no_tax_price, item_number,
                       model_number, quantity, unit_price, original_price,
                       discount_amount, description, transaction_number,
                       store_name, store_address, order_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

**Duplicate detection for Lowes PDFs:** Check by transaction_number (Lowes order ID) — if `src_lowes` rows already exist with the same `transaction_number`, this PDF was already imported. Skip or ask user.

#### Lowe's Order-Level CSV

The Lowes CSV from the order history export (`YYMMDD LOWES Order History...csv`) is **order-level only** — it says "See Sheet 2" for item details. It provides order totals, dates, store locations, and payment methods.

**Detection:** File is CSV, first data line starts with `Platform ,LOWES`. Skip header rows (Platform, Period, Total Orders, etc.) until reaching the row starting with `Order Type,Order ID,...`.

**Headers:** `Order Type, Order ID, Order Date, No. of Shipment, No. of Items, Payment Methods, Currency, Sub Total, Promotion, Coupon, Gift Card, Additional Fees, Shipping & Handling, Total Before Tax, VAT (TAX), Grand Total, Refund Total, Purchased Item Description, Invoice URL, Order URL, Shipping Address`

| CSV Column | Maps To | Notes |
|---|---|---|
| `Order Date` | `date` | Format: `Mon DD, YYYY` → normalize to YYYY-MM-DD |
| `Grand Total` | `price` | Negate: `-abs(total)` |
| `Order ID` | `order_id` + src_lowes.`transaction_number` | |
| `Order URL` | `links` + src_lowes.`order_url` | |
| `Shipping Address` | src_lowes.`store_name` + `store_address` | Format: "Store Name, Address, City ST ZIP" |
| `Payment Methods` | src_lowes.`payment_method` | e.g., "AMEX - 8012" |
| `Sub Total` | src_lowes.`no_tax_price` | |
| `No. of Items` | — | Informational only |

**Import as one transaction per order.** Set `item` to a summary like `"Lowes Order #{order_id} ({num_items} items)"`. Set `confidence = 0.3` since we can't categorize without item descriptions.

**If a matching PDF exists:** When importing a Lowes PDF later with the same Transaction #, the PDF's per-item rows supersede. The importer should detect the existing order-level row and offer to replace it.

**Source table INSERT (order-level):**
```sql
INSERT INTO src_lowes (transaction_id, row_num, no_tax_price, transaction_number,
                       store_name, store_address, order_url, payment_method)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
```

#### Home Depot Order-Level CSV

The Home Depot CSV from the order history export (`YYMMDD HOMEDEPOT Order History...csv`) is also **order-level only** — same "See Sheet 2" limitation.

**Detection:** File is CSV, first data line starts with `Platform ,HOMEDEPOT`. Skip header rows until reaching the row starting with `Order Type,Order ID,...`.

**Headers:** Same structure as Lowes CSV: `Order Type, Order ID, Order Date, No. of Shipment, No. of Items, Payment Methods, Currency, Sub Total, Promotion, Coupon, Gift Card, Additional Fees, Shipping & Handling, Total Before Tax, VAT (TAX), Grand Total, Refund Total, Purchased Item Description, Invoice URL, Order URL, Shipping Address`

| CSV Column | Maps To | Notes |
|---|---|---|
| `Order Date` | `date` | Format: `Mon DD, YYYY` → normalize to YYYY-MM-DD |
| `Grand Total` | `price` | Negate: `-abs(total)` |
| `Order ID` | `order_id` + src_homedepot.`order_number` | e.g., `H6575-285497` |
| `Order URL` | `links` + src_homedepot.`order_url` | |
| `Invoice URL` | src_homedepot.`invoice_url` | |
| `Shipping Address` | src_homedepot.`store_name` + `store_address` | If present |
| `Payment Methods` | src_homedepot.`payment_method` | e.g., "AX 8012" |
| `Sub Total` | src_homedepot.`subtotal` | |
| `VAT (TAX)` | src_homedepot.`tax` | |
| `No. of Items` | src_homedepot.`num_items` | |
| `Promotion` | src_homedepot.`promotion` | |

**Import as one transaction per order.** Set `item` to `"Home Depot Order #{order_id} ({num_items} items)"`. Set `site = "homedepot"`, `source_tab = "homedepot"`. Set `confidence = 0.3`.

**Source table INSERT:**
```sql
INSERT INTO src_homedepot (transaction_id, row_num, order_number, order_date,
                           num_items, payment_method, subtotal, promotion,
                           tax, grand_total, store_name, store_address,
                           order_url, invoice_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

#### Amazon Your Payments PDFs

Amazon "Your Payments" PDFs link Amazon orders to the credit card used for payment. Renamed files: `YYMMDD-amazon-payments.pdf`.

**This is NOT a standard transaction import.** These PDFs don't create new transactions — they annotate EXISTING Amazon transactions with the payment card used.

**PDF structure**: Each entry has:
- Date line (e.g., "April 19, 2024")
- Card line (e.g., "American Express ****1009", "Visa ****6388")
- Order line (e.g., "Order #114-3824803-3683466")
- Amount line (e.g., "-$11.93")
- Merchant line (e.g., "AMZN Mktp US")

**Parsing**: Use `pdftotext -layout` or the Read tool to extract text. Parse entries by detecting date lines followed by card+order+amount groups.

**Known credit cards** (from CLAUDE.md "Credit Cards" section):

| Last 4 | Owner | Cost Center | Business Card? | Notes |
|--------|-------|-------------|----------------|-------|
| 1009 | Personal Amex | Personal | No | In amex statements, normal chain matching |
| 2007 | Personal Amex | Personal | No | In amex statements, normal chain matching |
| 2023 | Personal Amex | Personal | No | In amex statements. Used for expense reports to businesses. Normal chain matching. |
| 9878 | Chase Southwest | Personal | No | In chase statements, normal chain matching |
| 2999 | Izuma CC | IZUMA | **Yes** | Bill goes direct to business. No statement imported. |
| 8574 | Izuma Visa | IZUMA | **Yes** | Bill goes direct to business. No statement imported. |
| 6979 | Izuma Visa | IZUMA | **Yes** | Bill goes direct to business. No statement imported. |
| 5055 | Gravhl Visa | GRAVHL | **Yes** | Bill goes direct to business. No statement imported. |
| 6388 | Red River Navy Fed | REDRIVER | **Yes** | Bill goes direct to business. No statement imported. |

**Processing each entry:**

1. Extract order_id from the Order line
2. Find the matching `src_amazon` row: `SELECT transaction_id FROM src_amazon WHERE order_id = ?`
3. If found, update the transaction's `business_card` column:
   - For business cards (2999, 8574, 6979, 5055, 6388): set `business_card = '<last4>-<COSTCENTER>'` (e.g., "6388-REDRIVER", "2999-IZUMA")
   - For personal cards (1009, 2007, 2023, 9878): leave `business_card` empty — these are tracked via normal amex/chase chain matching. Card 2023 is a personal Amex used for expense reports; its statements are already imported.
4. If no match in src_amazon, log the order as unmatched

**The `business_card` column**: TEXT column on `transactions`. When set, it means:
- The item was paid with a business credit card we don't import statements for
- The chain matcher should NOT look for a matching CC charge (there won't be one in our data)
- Format: `<last4>-<LABEL>` (e.g., "6388-REDRIVER", "2999-IZUMA", "5055-GRAVHL")

**Database migration** (run once if column doesn't exist):
```sql
ALTER TABLE transactions ADD COLUMN business_card TEXT DEFAULT '';
```

**Source table**: No new src_ table needed. Store the raw parsed data in `src_amazon_payments`:
```sql
CREATE TABLE IF NOT EXISTS src_amazon_payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT,
    card_type TEXT,
    card_last4 TEXT,
    order_id TEXT,
    amount REAL,
    merchant TEXT,
    source_tab TEXT,
    row_num INTEGER,
    matched_transaction_id INTEGER
);
```

**After import**: Report summary:
```
Amazon Payments: 32 PDFs, 598 entries parsed
  Personal cards (1009, 2007, 9878): 540 entries (no action needed)
  Business cards: 58 entries
    IZUMA (2999, 8574, 6979): 15 entries → 14 matched, 1 unmatched
    GRAVHL (5055): 18 entries → 17 matched, 1 unmatched
    REDRIVER (6388): 14 entries → 13 matched, 1 unmatched
    Business Amex (2023): 11 entries → 10 matched, 1 unmatched
  business_card column updated on 54 transactions
```

#### E*TRADE Check/Deposit Images

Check and deposit images are **not transaction data** — they are routed through Step 0 above. If `etchk`, `etbillpaycheck`, or `etdeposit` files appear in `data/renamed/`, Step 0 handles them automatically before any transaction import begins.

### Step 4: Process Each Transaction

For every parsed line, perform these steps in order:

#### 4a. Normalize to Standard Columns

Map vendor-specific columns into the standard transaction columns.

Set these automatically:
- `site` = vendor name from the filename
- `source_tab` = the filename being imported
- `source_row` = the row number within the file
- `tab` = `<site>-<row_num>-<date>` (must be unique — verify with SELECT before INSERT)
- `row_created_at` = current datetime (`datetime('now')`)
- `year` = extracted from the transaction date
- `account` = derived from the filename and source data:

**Account derivation rules:**

| Source | How to derive account |
|--------|----------------------|
| Amex CSV | `src_amex.account_num` (last 5 digits of card) |
| Chase CSV/PDF | From filename suffix: `chase-9878` → "Chase-9878", `chase-9956` → "Chase-9956", etc. |
| E*Trade PDF | Parse from filename: `YYMMDD-etrade-NNNN.pdf` → "E*Trade-NNNN" |
| Amazon CSV | "Amazon.com" |
| eBay XLSX | "eBay.com" |
| Lowe's PDF | "Lowes.com" |
| Home Depot PDF | "HomeDepot.com" |
| PayPal CSV | "PayPal" |
| Venmo CSV | From filename: `venmo-travis` → "Venmo-Travis", `venmo-melissa` → "Venmo-Melissa" |
| GoDaddy CSV | "GoDaddy" |
| AWS CSV | "AWS" |
| Mint CSV | `src_mint.account_name` (from the Mint export) |

#### 4b. Resolve Merchant

Query `merchant_patterns` using the item description:

```sql
SELECT m.id, m.name, m.default_tier1_id, m.default_tier2_id, m.default_tier3_id, m.is_multi_category
FROM merchant_patterns mp
JOIN merchants m ON mp.merchant_id = m.id
WHERE '<item_description>' LIKE mp.pattern
ORDER BY mp.priority DESC, LENGTH(mp.pattern) DESC
LIMIT 1;
```

Set `merchant_id` on the transaction row. Priority ordering + length ensures specific patterns match before generic ones.

#### 4c. Categorize

Apply this logic in order:

| Condition | Action | Confidence |
|---|---|---|
| Merchant found, `is_multi_category = 0` | Use merchant defaults (default_tier1_id, default_tier2_id, default_tier3_id) | 0.9 |
| Merchant found, `is_multi_category = 1` | Use merchant defaults as suggestion, present for review | 0.5 |
| No merchant, but item matches a known pattern | Suggest category from pattern | 0.6 |
| No match at all | Leave uncategorized | 0.0 |

Set the FK columns on insert — the text names are resolved via JOIN when read. Example lookups when you need the name at insert time (e.g., to display to the user during confirmation):

```sql
SELECT name FROM categories_tier1 WHERE id = <tier1_id>;
SELECT name FROM categories_tier2 WHERE id = <tier2_id>;
SELECT name FROM categories_tier3 WHERE id = <tier3_id>;
```

#### 4d. Detect Company

Use these patterns to resolve `company` and `company_id`:

| Company | Detection Heuristics |
|---|---|
| TMCTECH | Software/SaaS, cloud services, dev tools, crypto, domain registrations |
| MAPT | Real estate expenses, mortgage, property tax, HOA, repairs/maintenance |
| MAPTTW | eBay resale inventory, shipping supplies, packing materials |
| IZUMA | Japan-related, Japanese merchants, specific vendor patterns |
| TETECH | Electronics holding, component purchases |
| GRAVHL | Specific vendor patterns |
| `-` (Personal) | Everything else |

Validate with `company_category_rules`:
```sql
SELECT 1 FROM company_category_rules WHERE company_id = ? AND tier1_id = ?;
```

#### 4e. Duplicate Detection & Smart Merge

When importing data for a source that already has existing records, detect and handle duplicates.

**CRITICAL: Sources with institution-assigned transaction IDs MUST use them as the primary dedup key.** Date+amount fuzzy matching is a fallback only for sources without unique IDs.

**Step 1: Find potential duplicates**

For **sources with unique transaction IDs** — use the authoritative ID:

| Source | Dedup Field | Table.Column |
|--------|------------|--------------|
| Amex | reference | `src_amex.reference` |
| Chase CSV | reference | `src_chase.reference` (CSVs have it; PDFs do NOT) |
| PayPal | pp_transaction_id | `src_paypal.pp_transaction_id` |
| eBay | order_number + item_id | `src_ebay.order_number` + `src_ebay.item_id` |

```sql
-- Amex: check by reference
SELECT s.transaction_id, t.id, t.item, t.price, t.date
FROM src_amex s
JOIN transactions t ON s.transaction_id = t.id
WHERE s.reference = '<incoming_reference>';

-- PayPal: check by pp_transaction_id
SELECT s.transaction_id, t.id, t.item, t.price, t.date
FROM src_paypal s
JOIN transactions t ON s.transaction_id = t.id
WHERE s.pp_transaction_id = '<incoming_pp_txn_id>';
```

For **sources without unique IDs** — fall back to date+amount+description:
```sql
SELECT id, tab, item, price, date, site, source_tab, notes
FROM transactions
WHERE site = '<site>' AND date = '<date>'
  AND ABS(ABS(price) - ABS(?)) < 0.02
LIMIT 5;
```
Then compare item descriptions: clean both (lowercase, strip punctuation/whitespace), compare first 12-20 characters.

**Step 2: If duplicate found, smart merge (asymmetric)**

When a match is found, don't just skip — merge the best of both records. The existing DB row typically has **better categorization** (human-reviewed categories, company, merchant). The incoming record typically has **richer raw data** (longer descriptions, more source fields, addresses).

**Merge principle: existing DB wins on categorization, incoming wins on raw data richness.**

| Field | Rule | Why |
|---|---|---|
| `item` (description) | Keep whichever is longer | Incoming often has full merchant name+location |
| `category` / tier FKs | **ALWAYS keep existing** if set | Existing was likely human-categorized |
| `sub_category` / `sub_sub` | **ALWAYS keep existing** if set | Same reason |
| `company` / company_id | **ALWAYS keep existing** if set | Human-assigned company |
| `merchant_id` | **ALWAYS keep existing** if set | Already resolved |
| `confidence` | Keep the **higher** value | Don't downgrade confidence |
| `notes` | Concatenate if both have values | Preserve both |
| `source_tab` | Keep existing (don't change provenance) | |
| `src_*` fields | Update blanks from incoming | Fill in address, extended_details, etc. |

**Step 3: Apply the merge via UPDATE**

If the incoming record has richer data for any field:
```sql
UPDATE transactions SET
  item = CASE WHEN LENGTH(?) > LENGTH(item) THEN ? ELSE item END,
  notes = CASE WHEN ? IS NOT NULL AND ? != '' THEN COALESCE(notes || '; ', '') || ? ELSE notes END
WHERE id = <existing_id>;
```

Also update the `src_*` table if the incoming record has fields the existing source row lacks.

**Step 4: Report the merge decision**

```
[DUPE ref:320242660223981752] 2024-09-22 | -$25.00 | "GglPay STARBUCKS"
  Matched existing txn #6640 (same Amex reference)
  → Keeping existing (has extended_details, address)
  → Skipped incoming (PDF import, no extended_details)
```

**Step 5: Batch duplicate handling**

When processing a file where >50% of rows are expected duplicates, offer batch mode:
```
This file covers 2024-09-01 to 2024-09-22.
Found 85/141 matching by reference (from XLSX import).
Auto-skip all matches and import 56 new rows? (yes / individually / skip file)
```

**Important:** Two records are duplicates if they share the same **reference** (CC sources) or the same **date + amount + description prefix** (non-CC sources). Different sources (e.g., a PayPal payment and a Chase charge) are NOT duplicates — those are potential plumbing (see duplicate-detector skill).

#### 4f. Plumbing Detection

Set `is_plumbing = 1` for payment processing noise. See PayPal and Venmo vendor sections above for specific rules. General plumbing patterns:
- Bank transfers between own accounts
- Authorization holds and temporary holds
- Zero-amount entries
- Reversal pairs
- "Autopay Payment" or "epayment" descriptions
- "Interest" charges that are account-level, not purchase-level

### Step 5: Interactive Review (Interview Pattern)

Present transactions one at a time using AskUserQuestion, sorted by company name.

#### Standard Display

**CRITICAL: Always show the `item` column.** Required column order: date, price, item, vendor details.

For each transaction, show:

```
[3/47] 2025-01-15 | $42.99 | "USB-C Hub 7-in-1" | Amazon
  Merchant: Amazon.com (id: 12)
  Category: Electronics > Computer Accessories > Cables & Adapters [confidence: 0.9]
  Company: TMCTECH
  Source: src_amazon — ASIN: B08QV5L1VY, Order: 114-1234567-8901234

  > yes | "Category / Sub" | company "NAME" | dupe | note "text" | skip | stop
```

**Always enrich from source table.** Join the appropriate `src_*` table and show key extra columns:

| Source Table | Key Extra Columns to Show |
|---|---|
| src_amazon | asin_isbn, order_id, amazon_category, quantity |
| src_amex | account_num, extended_details, amex_category |
| src_chase | chase_category, post_date |
| src_etrade | transaction_type, check number (from description) |
| src_ebay | seller, item_id, order_number, image_url, quantity |
| src_paypal | type, status, from_email, to_email, pp_transaction_id |
| src_mint | original_description, account_name, mint_category |
| src_venmo | from_user, to_user, type, note |
| src_godaddy | product_name, receipt_number |
| src_aws | invoice_id, service_provider, billing_period |
| src_lowes | item_number, model_number, quantity, unit_price, discount_amount, store_name |
| src_homedepot | order_number, num_items, grand_total, store_name |

#### User Response Options

| Input | Action |
|---|---|
| `yes` or `y` | Accept and insert as shown |
| `"Category / Sub / SubSub"` | Override category (resolve text to tier*_id FKs) |
| `company "NAME"` | Override company (resolve to company_id) |
| `dupe` | Skip this row, mark as duplicate in notes |
| `note "text"` | Add text to the notes column, then insert |
| `skip` | Skip this row entirely (do not insert) |
| `stop` | Stop the import at this point, log progress so far |

Parse responses naturally — all of these are valid:
```
yes
"Food / Restaurants"
company "TMCTECH"
yes, but company MAPT
note "might be personal, check later"
skip
```

#### High-Confidence Batch Mode

When the next N transactions (up to 5) all have confidence >= 0.8 and the same company, offer a batch shortcut:

```
Next 5 transactions are all high-confidence (>= 0.8), all Personal. Auto-import? (yes / individually)
```

If the user says `yes`, insert all 5 without individual prompts. If `individually`, present them one at a time as usual.

### Step 6: Insert into Database

For each accepted transaction:

1. **Verify tab uniqueness**: `SELECT id FROM transactions WHERE tab = ?` — if exists, append sequence number
2. **INSERT into `transactions`** with all resolved columns (text + FK, confidence, merchant_id, is_plumbing, etc.)
3. **INSERT into the appropriate `src_*` table** with raw vendor data, linked by `transaction_id`

### Step 7: Log the Import

After all rows are processed (or the user stops early):

```sql
INSERT INTO import_log (filename, vendor, rows_total, rows_imported, rows_skipped, rows_duped, imported_at)
VALUES ('<filename>', '<vendor>', <total>, <imported>, <skipped>, <duped>, datetime('now'));
```

### Step 7b: Verify Source Attribution (MANDATORY)

Immediately after the import, run the source-attribution auditor against
only the rows we just inserted and fail loudly if anything is off. This
catches statement-boundary drift, wrong-file attributions, and placeholder
`source_tab` values before the file gets moved out of `data/renamed/`.

```bash
python3 scripts/audit_source_attribution.py \
    --source-tab '<filename>' --strict --out temp/post_import_audit.csv
```

- `--source-tab` scopes the audit to rows whose `source_tab` matches the
  file we just imported.
- `--strict` makes the script exit non-zero if any `price_not_in_file`
  rows exist.
- On failure: inspect `temp/post_import_audit.csv`, fix the parser (or the
  row-level attribution) — do **not** proceed to Step 8 with mis-attributed
  rows in place. The one case this is acceptable is when the source file
  does not exist on disk (`file_missing`); handle that explicitly by
  setting `source_tab = ''` for those rows.

This check is non-optional. Historical imports produced thousands of
rows whose `source_tab` pointed to the wrong PDF — the fix was
`scripts/fix_source_attribution.py`, and this post-import audit is what
prevents the regression.

### Step 8: Move File

On successful completion (all rows processed or user chose `stop`):

```bash
mv data/renamed/<filename> data/processed/<filename>
```

## Fix Mode

Fix mode repairs specific fields on existing transactions without re-importing data. It matches existing transactions to their source files in `data/processed/` and updates only the targeted fields.

### Fix Source Documents (`/import fix source`)

Backfills `source_tab` from bare site names (e.g., `amex`, `chase`) to actual filenames (e.g., `080218-amex.pdf`, `220907-chase-9878.pdf`).

**Why:** The native macOS app uses `source_tab` to enable "Open Source Document" in the context menu. Older imports only stored the site name, not the filename. ~36,000 transactions need this fix.

**Strategy by source type:**

#### Sources with import_log entries

The `import_log` table records `source_file` for each import run. Cross-reference:

```sql
-- Find which processed file imported a given transaction
-- by matching the site and date range of the file to the transaction
SELECT il.source_file, il.rows_imported, il.import_started
FROM import_log il
WHERE il.source_file LIKE '%<site>%'
ORDER BY il.source_file;
```

#### Amex (29,024 bare txns → 242 processed files)

Amex PDFs are monthly statements. Match by date:
1. Parse statement date from filename: `080218-amex.pdf` → closing date Feb 18, 2008
2. Amex billing cycles are ~30 days. A transaction on 2008-01-20 belongs to the 080218 statement.
3. For each amex txn with `source_tab = 'amex'`:
   - Find the processed file whose closing date is the **first closing date on or after** the transaction date
   - UPDATE `source_tab` to that filename

```sql
UPDATE transactions SET source_tab = ?
WHERE id = ? AND source_tab = 'amex';
```

**If src_amex.reference exists:** Use it to find the exact import_log entry that created this transaction (match reference to rows in the file). This is more precise but only works for ~7,700 of the 29,024 rows.

#### Chase (1,385 bare txns → 129 processed files)

Chase files include the account suffix: `220907-chase-9878.pdf`. Match by:
1. Account (from `transactions.account` → `Chase-9878` maps to files matching `*chase-9878*`)
2. Date range (statement month)

#### E*Trade (1,144 bare txns → 442 processed files)

E*Trade PDFs are monthly per-account: `071031-etrade-1452.pdf`. Match by:
1. Account (from `transactions.account` → `E*Trade-1452` maps to files matching `*etrade-1452*`)
2. Statement month

#### Venmo (564 bare txns → 127 processed files)

Venmo CSVs are monthly per-account: `210101-venmo-melissa.csv`. Match by:
1. Account (from `transactions.account` → `Melissa` maps to `*venmo-melissa*`, `Travis` maps to `*venmo-travis*`)
2. Transaction month

#### Amazon (1,167 bare txns)

Amazon CSVs cover variable date ranges. Match by:
1. Order date falls within the date range of rows in the CSV
2. Or match by `src_amazon.order_id` against rows in the file

#### GoDaddy (523 bare txns → 1 processed file: `030530-godaddy.csv`)

All GoDaddy transactions came from a single full-history CSV export.

```sql
UPDATE transactions SET source_tab = '030530-godaddy.csv'
WHERE source_tab = 'godaddy';
```

#### PayPal (298 bare txns → 25 processed files)

Monthly CSVs. Match by account + transaction month.

#### eBay (179 bare txns → 10 processed files)

XLSX exports cover variable date ranges. Match by order date.

#### Lowes (160 bare txns → 51 processed files)

PDF receipts per order. Match by `src_lowes.transaction_number` against filename or order data.

#### Mint (910 bare txns)

Mint CSVs are bulk exports. If only one mint file exists in processed, assign all to it. Otherwise match by date range.

#### Other (ledger: 75, homedepot: 56, aws: 36, etrades: 92)

- `ledger`: Manual entries, no source file. Leave as-is.
- `homedepot`: Match by `src_homedepot.order_number` against PDF filenames.
- `aws`: Match by date range against processed AWS CSVs.
- `etrades`: Match by account + month against processed files.

**Execution flow:**

1. Scan `data/processed/` to build a map: `filename → (site, date, account)`
2. For each site with bare `source_tab` values:
   - Query transactions needing fix
   - Match each to a processed file using the strategy above
   - Batch UPDATE with confirmation: `"Fix 523 godaddy txns → 030530-godaddy.csv? (yes/skip)"`
3. Report summary:

```
── Source Document Fix ──────────────────────────
  godaddy:     523 →  030530-godaddy.csv
  amex:      29024 →  242 files (by statement date)
  chase:      1385 →  129 files (by account + date)
  venmo:       564 →  127 files (by account + month)
  ...
  Total fixed: 36,470
  Unresolvable: 75 (ledger — no source file)
```

### Fix Account (`/import fix account`)

Backfills `transactions.account` where it's NULL or generic. Uses the same filename-parsing rules defined in Step 4a account derivation.

## Important Rules

1. **Always** run `PRAGMA foreign_keys=ON;` before any write operation
2. **Always** write FK columns only (`tier1_id`, `tier2_id`, `tier3_id`, `company_id`). The text mirror columns were dropped — there's nothing to sync.
3. **Always** dedup on `(source_tab, source_row)` before insert. The legacy `tab` synthetic key column was dropped on 2026-04-10.
4. **Always** set `source_tab` to the **full filename** (e.g., `250103-amazon.csv`, NOT just `amazon`) and `site` to the vendor. This enables the native app to open the source document from `data/processed/`.
5. **Always** let `created_at` default to CURRENT_TIMESTAMP — the legacy `row_created_at` column was dropped on 2026-04-10.
6. **Always** record `confidence` on every transaction
7. **Always** show the `item` column in every transaction display
8. **Always** enrich displays with source table data (JOIN the appropriate `src_*` table)
9. **Never** insert a row that violates the UNIQUE(tab) constraint — SELECT to check first
10. **Never** silently skip errors — report them to the user
11. **Never** use external Python scripts for parsing — use the Read tool directly
12. **Always** log the import in `import_log` even if the user stops early
13. **Always** move the file to `data/processed/` after the import session ends
14. **Always** negate expense amounts (purchases should be negative in the DB)
15. **Always** flag PayPal/Venmo plumbing entries with `is_plumbing = 1`
16. **Always** run Step 7b (`scripts/audit_source_attribution.py --source-tab <filename> --strict`) after every import and fix any `price_not_in_file` failures before moving the file. Historical imports silently dropped ~2,500 transactions on the wrong source file due to statement-cycle drift — this check exists to prevent regression.
