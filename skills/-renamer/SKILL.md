---
name: renamer
description: Use when processing incoming financial documents (PDFs, CSVs, XLSX, images) to identify vendors and dates, then rename and move them to a standardized YYMMDD-vendor.ext format. Handles bank statements, credit card statements, receipts, invoices, and transaction exports.
argument-hint: [folder or filename]
disable-model-invocation: false
---

# Document Renamer

Process incoming financial documents, identify the vendor/institution and date, then rename them with a standardized format: `YYMMDD-<vendor>.<ext>`.

If `$ARGUMENTS` is provided, interpret it as a specific file or folder to process (e.g., "chase-statement.pdf", "data/incoming/2024/").

## Database Location

```
db/personaldb.sqlite
```

Use sqlite3 CLI via Bash for all database queries. Enable headers in every query:
```bash
sqlite3 db/personaldb.sqlite "PRAGMA foreign_keys=ON; .headers on; .mode column; <QUERY>"
```

## Folder Paths

- **Incoming**: `data/incoming/` — unprocessed documents land here
- **Renamed**: `data/renamed/` — documents moved here after renaming

## Supported File Types

| Extension | How to read | What to look for |
|---|---|---|
| `.pdf` | Read tool (supports PDF natively) | Institution letterhead, account numbers, statement dates, period dates |
| `.csv` | Read tool | Header row patterns, column names, first few data rows |
| `.xlsx` | Read tool | Same as CSV — headers and first rows |
| `.png`, `.jpg`, `.jpeg` | Read tool (supports images natively) | Receipts, statement screenshots, transaction confirmations |

## Known Source Institutions

These are the primary financial institutions and services. They map to `src_*` tables in the database:

| Vendor key | Patterns to look for |
|---|---|
| `chase` | "JPMorgan Chase", "Chase Bank", chase.com, account numbers starting with specific prefixes |
| `amex` | "American Express", amex.com, card numbers starting with 3, "Card Member Since" |
| `etrade` | "E*TRADE", "Morgan Stanley", etrade.com, brokerage statements |
| `mint` | "Mint", "Intuit Mint", mint.com, transaction exports with mint_category column |
| `paypal` | "PayPal", paypal.com, "Transaction ID" with PP- prefix |
| `venmo` | "Venmo", venmo.com, @usernames, "Standard Transfer" |
| `amazon` | "Amazon", "Your Orders", ASIN/ISBN, order IDs (###-#######-#######) |
| `ebay` | "eBay", item IDs, seller IDs, order numbers |
| `godaddy` | "GoDaddy", domain names, receipt numbers |
| `aws` | "Amazon Web Services", "AWS", invoice IDs, service charges |
| `lowes` | "Lowe's", store numbers, item SKUs |
| `homedepot` | "Home Depot", "Order #" H-prefix, "Receipt #", SKU numbers, `Order_Details_` filename prefix, store numbers like #6570 |
| `usaa` | "USAA Federal Savings Bank", "USAA CLASSIC CHECKING", "USAA SAVINGS", account numbers 0003353982 (checking) or 0022781064 (savings) |

### E*Trade PDF Monthly Statements

E*Trade monthly bank statements are PDFs with a consistent format across all years (2007–present). They may arrive:
- **Already named**: `YYMMDD-etrade.pdf` (e.g., `260228-etrade.pdf`) where the date is the statement end date
- **Raw download**: generic filenames from Morgan Stanley / E*Trade website

**Detection**: Look for "E*TRADE", "Morgan Stanley Private Bank", "Max-Rate Checking", or "Account: 2017321452" in PDF text.

**Date extraction**: Use the statement period end date from the header:
```bash
pdftotext "data/incoming/FILENAME.pdf" - | grep -oP 'Through \K\d{2}-\d{2}-\d{2}'
```
This gives `MM-DD-YY` — convert to `YYMMDD` for the filename.

**Batch handling**: When many E*Trade PDFs are already correctly named (`YYMMDD-etrade.pdf`), offer to batch-move them all at once:
```
── etrade: 221 monthly statements (071031 through 260228) ──────

  All files already named in YYMMDD-etrade.pdf format.
  Move all 221 files to data/renamed/? (yes / individually / skip)
```

If the user says `yes`, move them all without individual prompts. Verify no collisions in `data/renamed/` first.

### American Express Statements (Downloaded via Browser Script)

Amex statements downloaded via `scripts/browser/amex_dl_statements.js` arrive as:
- `YYMMDD-amex-amex.xlsx` — Excel format (April 2024 and newer, richer transaction data)
- `YYMMDD-amex-amex.pdf` — PDF format (all dates, especially pre-April 2024)

The double "amex" comes from the downloader using "amex" as the account suffix. These need the trailing `-amex` stripped.

**Detection**: Filename matches `\d{6}-amex-amex\.(xlsx|pdf)`.

**Rename**: `YYMMDD-amex-amex.ext` → `YYMMDD-amex.ext` (drop the duplicate suffix). No need to read the file — the naming is unambiguous.

**Batch handling**: Group all amex statement files together by type and offer to batch-rename:
```
── amex: 24 Excel statements + 218 PDF statements ──────

  XLSX: 240418-amex-amex.xlsx → 240418-amex.xlsx (24 files, Apr 2024 – Mar 2026)
  PDF:  080218-amex-amex.pdf → 080218-amex.pdf (218 files, Feb 2008 – Mar 2026)

  Rename all 242 files? (yes / xlsx-only / pdf-only / individually / skip)
```

When both XLSX and PDF exist for the same date, keep both — the importer prefers XLSX for transaction data but PDF is retained for archival.

### Venmo Statement CSVs

Venmo statement CSVs downloaded via `scripts/browser/venmo_dl_statements.js` arrive with generic filenames (e.g., `statement.csv`, `venmo_statement.csv`, or timestamped names). There are two Venmo accounts — Travis and Melissa — so the account owner must be embedded in the filename.

**Detection**: CSV file with Venmo-style headers (`ID, Datetime, Type, Status, Note, From, To`). The first cell (A1 / row 1) contains account identification text — look for the username or account holder name (e.g., `@Travis-McCollum`, `@Melissa-McCollum`, `Travis McCollum`, `Melissa McCollum`).

**Account extraction**: Read cell A1 (first line of the CSV). Parse the account owner:
- If contains `travis` (case-insensitive) → account suffix = `travis`
- If contains `melissa` (case-insensitive) → account suffix = `melissa`

**Date extraction**: Read the CSV data rows to find the earliest transaction date for the YYMMDD prefix.

**Output format**: `YYMMDD-venmo-travis.csv` or `YYMMDD-venmo-melissa.csv`

The account suffix is critical — the importer uses it to set the `account` field and distinguish between the two Venmo accounts.

**Batch handling**: Multiple Venmo CSVs may arrive at once (one per month per account). Group by account, auto-rename all:
```
── venmo: 12 Travis CSVs + 12 Melissa CSVs ──────

  Travis: statement.csv → 240101-venmo-travis.csv (etc.)
  Melissa: statement(1).csv → 240101-venmo-melissa.csv (etc.)
```

### Chase Statements (Downloaded via Browser Script)

Chase statements downloaded via `scripts/browser/chase_dl_statements.js` arrive as `YYYYMMDD-statements-NNNN-.pdf` where YYYYMMDD is the statement date and NNNN is the last 4 of the account.

**Detection**: Filename matches `\d{8}-statements-\d{4}-\.pdf`.

**4 accounts:**

| Last 4 | Account Type | Rename to |
|--------|-------------|-----------|
| 9956 | Personal Checking | `YYMMDD-chase-9956.pdf` |
| 8073 | Personal Savings | `YYMMDD-chase-8073.pdf` |
| 0095 | Managed Brokerage | `YYMMDD-chase-0095.pdf` |
| 9878 | Southwest Priority CC | `YYMMDD-chase-9878.pdf` |

**Rename**: `YYYYMMDD-statements-NNNN-.pdf` → `YYMMDD-chase-NNNN.pdf` (strip 4-digit year to 2-digit, replace `statements` with `chase`, drop trailing hyphen). No need to read the file.

**Batch handling**: Group all Chase files and auto-rename:
```
── chase: 40 CC (9878) + 32 checking (9956) + 32 savings (8073) + 16 brokerage (0095) ──
```

### Amazon Your Payments PDFs

Amazon "Your Payments" pages are exported as PDFs from `amazon.com/cpe/yourpayments/transactions`. They contain payment method + order linkage data: which credit card was used for each Amazon order.

**Detection**: PDF content contains "Your Payments" AND "amazon.com/cpe/yourpayments" OR entries like "American Express ****1009" / "Visa ****6388" with "Order #" lines.

**Content structure**: Each page has ~10-20 transaction entries. Each entry has:
- Date (e.g., "April 19, 2024")
- Card type + last 4 (e.g., "American Express ****1009", "Visa ****6388")
- Order number (e.g., "Order #114-3824803-3683466")
- Amount (e.g., "-$11.93")
- Merchant (e.g., "AMZN Mktp US", "Amazon.com", "Audible")

**Date extraction**: Read the PDF and find the earliest transaction date. Use that for the YYMMDD prefix.

**Output format**: `YYMMDD-amazon-payments.pdf`. Use sequence numbers for collisions: `240101-amazon-payments.pdf`, `240101-amazon-payments-2.pdf`.

**Batch handling**: Multiple PDFs often arrive at once (user exports pages one at a time with junk filenames). Group all detected Amazon Payments PDFs together:
```
── amazon-payments: 32 PDFs ──────

  Your Payments.pdf → 240419-amazon-payments.pdf
  asdfasdfsa.pdf → 240216-amazon-payments.pdf
  asdfasdfsdfafd.pdf → 240526-amazon-payments.pdf
  ... (29 more)

  Rename all 32 files? (yes / individually / skip)
```

When the user says `yes`, rename and move them all. Each PDF gets its own date based on the earliest transaction date found inside.

### Home Depot PDF Batch Pattern

Home Depot exports order PDFs in bulk with filenames like `Order_Details_April-02-2026_02_38-AM-N.pdf`. The filename date is the **download date**, not the order date. Always extract the actual order date from inside the PDF using `pdftotext`:
```bash
pdftotext "data/incoming/FILENAME.pdf" - | grep -A1 "Ordered" | tail -1
```
Each PDF is a separate order. Rename each using its internal order date: `YYMMDD-homedepot.pdf`. Use sequence numbers for same-date collisions: `250801-homedepot.pdf`, `250801-homedepot-2.pdf`.

### USAA Statements

USAA checking and savings statements arrive with filenames like `YYYY-MM-DD_CHECKING_3982_STATEMENT.pdf` or `YYYY-MM-DD_SAVINGS_1064_STATEMENT.pdf`.

**Detection**: Filename matches `\d{4}-\d{2}-\d{2}_(CHECKING|SAVINGS)_\d{4}_STATEMENT\.pdf`.

**2 accounts:**

| Last 4 | Account Type | Rename to |
|--------|-------------|-----------|
| 3982 | Classic Checking | `YYMMDD-usaa-3982.pdf` |
| 1064 | Savings | `YYMMDD-usaa-1064.pdf` |

**Rename**: `YYYY-MM-DD_TYPE_NNNN_STATEMENT.pdf` → `YYMMDD-usaa-NNNN.pdf` (convert date, replace TYPE with usaa, keep account suffix). No need to read the file — the naming is unambiguous.

**Skip non-statement PDFs**: Files like `*_Revised_USAA_Depository_Agreement*`, `*_Changes_to_Overdraft*`, or other non-statement USAA documents should be skipped (leave in `data/incoming/`).

**Batch handling**: Group all USAA statement files and auto-rename:
```
── usaa: 12 checking (3982) + 5 savings (1064) ──────

  2023-01-13_CHECKING_3982_STATEMENT.pdf → 230113-usaa-3982.pdf
  2023-01-07_SAVINGS_1064_STATEMENT.pdf → 230107-usaa-1064.pdf
  ... (15 more)

  Rename all 17 files? (yes / checking-only / savings-only / individually / skip)
```

### E*TRADE Check Images

Check, bill pay check, and deposit images downloaded via `scripts/browser/etrade_dl_checks.js` or `etrade_dl_deposits.js` arrive as:
- `YYMMDD_etchk_ACCT_NNN_$N,NNN.NN_front.jpg` — front of check (e.g., `260302_etchk_1452_770_$3,800.00_front.jpg`)
- `YYMMDD_etchk_ACCT_NNN_$N,NNN.NN_back.jpg` — back of check
- `YYMMDD_etbillpaycheck_ACCT_NNN_$N,NNN.NN_front.png` — bill pay check front
- `YYMMDD_etbillpaycheck_ACCT_NNN_$N,NNN.NN_back.png` — bill pay check back
- `YYMMDD_etdeposit_ACCT_N_$N,NNN.NN_slip.png` — deposit slip
- `YYMMDD_etdeposit_ACCT_N_$N,NNN.NN_check1.png` — deposited check

Where `ACCT` is the last 4 digits of the E*TRADE account (e.g., `1452`).

**Detection**: Filename matches `\d{6}_(etchk|etbillpaycheck|etdeposit)_\d+_`.

**These do NOT get renamed.** They keep their original filenames. The check number and account are embedded in the name and the importer needs them.

**Batch handling**: Group all check/deposit images together and offer to batch-move:
```
── etrade check/deposit images: 16 files (8 checks) ──────

  260302_etchk_1452_770_$3,800.00_front.jpg, ..._back.jpg
  260224_etchk_1452_920_$3,200.00_front.jpg, ..._back.jpg
  ... (6 more)

  Move all 16 files to data/renamed/? (yes / individually / skip)
```

If the user says `yes`, move them all without individual prompts. Do NOT rename them — just `mv` as-is to `data/renamed/`.

## CSV Header Patterns

Use header rows to identify the source institution:

| Source | Header signature |
|---|---|
| amex | `Date,Description,Card Member,Account #` or `Date,Description,Amount` with amex-style descriptions |
| chase | `Transaction Date,Post Date,Description,Category,Type,Amount` |
| mint | `Date,Description,Original Description,Amount,Transaction Type,Category,Account Name` |
| amazon | `Order Date,Order ID,Title,Category,ASIN/ISBN,Website` or similar |
| ebay | `Order Number,Buyer,Item Title,Item ID,Sale Date` or similar |
| paypal | `Date,Time,Name,Type,Status,Amount,Transaction ID` |
| etrade | `TransactionDate,TransactionType,SecurityType,Symbol,Description` or similar |
| venmo | `ID,Datetime,Type,Status,Note,From,To,Amount` or similar. **A1 cell contains account ID** — check for `travis` or `melissa` to determine account suffix. |
| godaddy | `Product,Start Date,Expiration Date,Receipt Number` or similar |
| aws | `InvoiceId,PayerAccountId,LinkedAccountId,RecordType,BillingPeriodStartDate` or similar |

## Vendor Matching

When a document does not match a known source institution, attempt to identify the vendor:

### Step 1: Check the merchants table
```sql
SELECT name, name_normalized
FROM merchants
WHERE name_normalized LIKE '%<search_term>%'
LIMIT 10;
```

### Step 2: Check merchant_patterns table
```sql
SELECT mp.pattern, m.name, m.name_normalized
FROM merchant_patterns mp
JOIN merchants m ON mp.merchant_id = m.id
WHERE '<document_text>' LIKE mp.pattern
LIMIT 10;
```

### Step 3: If no match found
Ask the user to identify the vendor using `AskUserQuestion`. Offer to add the vendor to the `merchants` table if it is a new one.

## Output Filename Format

**CRITICAL: The output format is exactly `YYMMDD-vendor.ext` — 6-digit date with 2-digit year, a single hyphen, lowercase vendor, then extension. Nothing else.**

```
YYMMDD-vendor.ext
```

**YYMMDD** (6 characters, 2-digit year — NOT 4-digit):
- `260107` = January 7, 2026 (NOT `20260107`)
- `241231` = December 31, 2024
- `230915` = September 15, 2023
- Date priority: statement period start > statement date > earliest transaction date > filename date

**vendor** (lowercase, short):
- Use the source institution key from the Known Sources table: `chase`, `amex`, `amazon`, `etrade`, `ebay`, `paypal`, `mint`, `venmo`, `godaddy`, `aws`, `lowes`
- For non-institution vendors, use a short lowercase slug: `costco`, `target`, `apple`
- No spaces, no underscores — just the vendor name

**ext** (lowercase): `.pdf`, `.csv`, `.xlsx`, `.png`, `.jpg`

### Examples with real incoming files:

| Incoming filename | Detected | Output |
|---|---|---|
| `20260107-statements-9878-.pdf` | amex (account 9878), date 2026-01-07 | `260107-amex.pdf` |
| `20250407-statements-9878-.pdf` | amex (account 9878), date 2025-04-07 | `250407-amex.pdf` |
| `amazon_order_history.csv` | amazon (from headers), earliest date in file | `241231-amazon.csv` |
| `apple software paste queue.png` | apple (from image content), date from image | `250301-apple.png` |
| `Statement_Dec2023.pdf` | chase (from PDF content), period start Dec 1 | `231201-chase.pdf` |
| `transactions-2024.csv` | mint (from CSV headers), earliest date Jan 5 | `240105-mint.csv` |

**Wrong formats — NEVER produce these:**
- ~~`20260107-amex.pdf`~~ (4-digit year)
- ~~`260107-amex-statement.pdf`~~ (extra words)
- ~~`260107_amex.pdf`~~ (underscore instead of hyphen)
- ~~`260107-AMEX.pdf`~~ (uppercase vendor)
- ~~`260107-american-express.pdf`~~ (full name instead of short key)

## Workflow — Autonomous Mode

**This skill operates autonomously.** Rename and move files without asking for permission. Only ask the user when a file truly cannot be identified (unknown vendor, no date found).

### Step 1: List and classify incoming files

```bash
ls -la data/incoming/
```

If `$ARGUMENTS` specifies a file or subfolder, scope to that instead.

For each file, classify it:
- **Auto-rename**: vendor and date are clear from filename patterns, CSV headers, or PDF content → rename and move immediately
- **Unknown**: cannot determine vendor or date → queue for user input

### Step 2: Auto-rename known patterns

Process these without asking. Just do it and log the result.

**Filename-based patterns** (no need to read the file):

| Pattern | Vendor | Date extraction |
|---|---|---|
| `YYMMDD-amex-amex.{xlsx,pdf}` | amex | from filename, strip duplicate `-amex` |
| `YYMMDD-etrade.pdf` | etrade | from filename |
| `YYMMDD_(etchk\|etbillpaycheck\|etdeposit)_*` | etrade images | DO NOT rename, move as-is |
| `statement-Mon-YYYY.pdf` | paypal (or as user specified) | 1st of month |
| `YYYYMMDD-statements-NNNN-.pdf` | amex | strip 4-digit year to 2-digit |
| `Order_Details_*.pdf` | homedepot | extract from PDF content (order date) |
| `Ebay_Purchase_History_YYYY.xlsx` | ebay | earliest date in file |
| `godaddy*history*.csv` | godaddy | earliest order date in file |
| `YYYYMMDD-statements-NNNN-.pdf` | chase | strip 4-digit year to 2-digit, account = NNNN |
| `YYYY-MM-DD_(CHECKING\|SAVINGS)_NNNN_STATEMENT.pdf` | usaa | convert date YYYY-MM-DD → YYMMDD, account = NNNN |
| PDF with "Your Payments" + card entries | amazon-payments | earliest transaction date in PDF |

**Content-based detection** (read first page / headers):

For CSVs/XLSX: match header row against CSV Header Patterns table → auto-rename.
For PDFs: read first 1-2 pages, match against Known Source Institutions → auto-rename.

### Step 3: Rename and move

For each identified file:

```bash
mv "data/incoming/<original_name>" "data/renamed/YYMMDD-<vendor>.<ext>"
```

**Before moving, check for collisions:**
- If target exists in `data/renamed/`, append sequence number: `-2`, `-3`, etc.

Log each rename as a one-liner:
```
  Statement_Dec2023.pdf → 231201-chase.pdf ✓
  20260107-statements-9878-.pdf → 260107-amex.pdf ✓
  240418-amex-amex.xlsx → 240418-amex.xlsx ✓
```

### Step 4: Handle unknowns (only time to ask)

If a file cannot be identified after reading it, ask the user:

```
  Could not identify: mystery_doc.pdf
  Content preview: (key text or image description)
  Best guess: possibly a utility bill
```

Then use `AskUserQuestion` for just the unknowns — vendor name, date, or skip.

### Step 5: Summary

After all files are processed, show:

```
Renaming complete:
  Auto-renamed: 25 files
  Unknown:       2 files (asked user)
  Skipped:       1 file

  amex: 4 files
  chase: 3 files
  paypal: 15 files
  etrade: 3 files
```

## Guardrails

- **Never overwrite existing files** in `data/renamed/` — use sequence numbers for collisions
- **Never delete files** — only move (rename) them
- **Never modify file contents** — only rename and move
- **Do NOT ask for permission** on files with clear vendor+date — just rename them
- **Only ask** when vendor or date is genuinely unknown after reading the file
- **Use the Read tool** for PDFs and images (it supports multimodal reading natively)
- **Large PDFs**: Use the `pages` parameter (e.g., `pages: "1-3"`) to avoid reading entire large documents
- **Preserve originals**: If something goes wrong with a move, the file should remain in `data/incoming/`

## Summary Output

After processing all files (or when the user stops), show a final summary:

```
Renaming complete:
  Processed: 25 files
  Renamed:   20 files (moved to data/renamed/)
  Skipped:    3 files (still in data/incoming/)
  Noted:      2 files (still in data/incoming/, with notes)

  New merchants added: 1 (Costco Wholesale -> costco)

Skipped files:
  - mystery_doc.pdf — could not identify vendor
  - corrupted.csv — file appears empty
  - scan_blurry.jpg — image too blurry to read

Notes:
  - scan_20231201.pdf — "might be a duplicate of the Dec statement"
  - partial_export.csv — "only has 3 rows, check if complete"
```
