---
name: tax-organizer
description: Maintain year-by-year tax document folders, track which institution documents are missing, file incoming documents automatically, and provide URLs to retrieve missing documents.
argument-hint: [year | status | file | setup]
disable-model-invocation: false
---

# Tax Organizer

Maintain a flat, year-by-year tax document folder structure. Track which institutions need documents each year, identify what's missing, file incoming documents, and provide retrieval URLs.

If `$ARGUMENTS` is provided, interpret as an action:
- A year (e.g., `2025`) — show status for that tax year
- `status` — show status for all years
- `file` — process incoming folder
- `setup` — initialize a new tax year
- `setup 2025` — initialize a specific year

## Paths

```
BASE_DIR = ~/Library/CloudStorage/GoogleDrive-travis.mccollum@gmail.com/My Drive/TRAVIS_Taxes/
```

| Path | Purpose |
|---|---|
| `BASE_DIR/<YYYY>/` | Tax documents for that year (flat — no subfolders) |
| `BASE_DIR/incoming/` | Drop zone for new documents to be filed |
| `BASE_DIR/.tax-organizer/tax_tracker.sqlite` | Tracking database |

## Folder Structure (Flat)

Each year folder is **flat** — no subfolders. Files are named with a consistent prefix pattern:

```
<YYYY>/
  <institution>-<doc_type>-<detail>.pdf
  <institution>-<doc_type>-<detail>.pdf
  ...
```

### File Naming Convention

```
<institution>-<doc_type>[-<detail>].<ext>
```

**institution** (lowercase): `amex`, `chase`, `etrade`, `schwab`, `fidelity`, `paypal`, `venmo`, `amazon`, `godaddy`, `aws`, `lowes`, `usaa`, `irs`, `texas`, `travis-county`, `hays-county`, `williamson-county`, etc.

**doc_type** (lowercase):
| Type | Description |
|---|---|
| `1099-int` | Interest income |
| `1099-div` | Dividends |
| `1099-b` | Brokerage/capital gains |
| `1099-r` | Retirement distributions |
| `1099-misc` | Miscellaneous income |
| `1099-nec` | Nonemployee compensation |
| `1099-k` | Payment card/third-party network |
| `1098` | Mortgage interest |
| `1098-t` | Tuition |
| `w2` | Wage statement |
| `1095-a/b/c` | Health coverage |
| `5498` | IRA contributions |
| `statement-annual` | Year-end statement |
| `statement-q1/q2/q3/q4` | Quarterly statements |
| `statement-01..12` | Monthly statements |
| `property-tax` | Property tax bill/receipt |
| `property-insurance` | Insurance declaration page |
| `hoa` | HOA annual statement |
| `k1` | Partnership/S-corp schedule |
| `receipt` | Business receipt |
| `summary` | Annual summary |
| `sales-report` | Sales tax report (for eBay/resale) |

**detail** (optional): account last 4, property address slug, etc.

### Examples

```
2024/
  amex-statement-annual-9878.pdf
  amex-1099-int.pdf
  chase-statement-annual-4589.pdf
  chase-1099-int.pdf
  etrade-1099-b.pdf
  etrade-1099-div.pdf
  etrade-5498.pdf
  paypal-1099-k.pdf
  ebay-1099-k.pdf
  irs-1040-filed.pdf
  texas-franchise-tax.pdf
  travis-county-property-tax-7207.pdf
  hays-county-property-tax-1913.pdf
  usaa-1098-7207.pdf
  usaa-1098-1913.pdf
  gravhl-k1.pdf
  tmctech-k1.pdf
```

## Database

**Path**: `BASE_DIR/.tax-organizer/tax_tracker.sqlite`

Use `sqlite3` CLI or Python `sqlite3` module.

### Schema

```sql
CREATE TABLE IF NOT EXISTS institutions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    code            TEXT NOT NULL UNIQUE,        -- 'amex', 'chase', 'etrade'
    name            TEXT NOT NULL,                -- 'American Express', 'Chase Bank'
    institution_type TEXT NOT NULL,               -- 'bank', 'brokerage', 'payment', 'government', 'insurance', 'employer', 'property'
    portal_url      TEXT,                         -- URL to log in and download docs
    doc_retrieval_notes TEXT,                     -- step-by-step instructions
    first_year      INTEGER,                      -- first year this institution appears
    last_year       INTEGER,                      -- NULL = still active
    discontinued_at TEXT,                         -- date user said "no longer have"
    ask_until_year  INTEGER,                      -- stop asking after this year (discontinued_at year + 3)
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS institution_docs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    institution_id  INTEGER NOT NULL REFERENCES institutions(id),
    doc_type        TEXT NOT NULL,                -- '1099-int', 'statement-annual', 'property-tax'
    description     TEXT,                         -- 'Interest income statement'
    is_required     BOOLEAN DEFAULT 1,            -- required for tax filing?
    is_conditional  BOOLEAN DEFAULT 0,            -- only needed if threshold met (e.g., 1099-K > $600)
    condition_notes TEXT,                         -- 'Only issued if payments > $600'
    UNIQUE(institution_id, doc_type)
);

CREATE TABLE IF NOT EXISTS year_institutions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    year            INTEGER NOT NULL,
    institution_id  INTEGER NOT NULL REFERENCES institutions(id),
    status          TEXT DEFAULT 'expected',      -- 'expected', 'received', 'not-applicable', 'waived'
    notes           TEXT,
    UNIQUE(year, institution_id)
);

CREATE TABLE IF NOT EXISTS year_documents (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    year            INTEGER NOT NULL,
    institution_id  INTEGER NOT NULL REFERENCES institutions(id),
    doc_type        TEXT NOT NULL,
    filename        TEXT,                         -- actual filename in the year folder
    status          TEXT DEFAULT 'missing',       -- 'missing', 'received', 'not-applicable', 'waived'
    received_at     TEXT,                         -- date filed
    notes           TEXT,
    UNIQUE(year, institution_id, doc_type)
);

CREATE TABLE IF NOT EXISTS filing_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    year            INTEGER NOT NULL,
    filename        TEXT NOT NULL,
    institution_code TEXT,
    doc_type        TEXT,
    action          TEXT NOT NULL,                -- 'filed', 'renamed', 'skipped'
    source_filename TEXT,                         -- original name from incoming/
    filed_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS discontinuation_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    institution_id  INTEGER NOT NULL REFERENCES institutions(id),
    year_asked      INTEGER NOT NULL,
    response        TEXT NOT NULL,                -- 'confirmed-gone', 'still-active', 'deferred'
    asked_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(institution_id, year_asked)
);
```

### Seed Data — Known Institutions

Seed these on first run (adjust URLs to match actual portals):

```sql
-- Financial Institutions
INSERT INTO institutions (code, name, institution_type, portal_url, doc_retrieval_notes, first_year) VALUES
('amex',     'American Express',  'bank',      'https://www.americanexpress.com/us/customer-service/statements-tax-forms.html', 'Login > Statements & Activity > Tax Forms > Download 1099', 2021),
('chase',    'Chase Bank',        'bank',      'https://www.chase.com/digital/login', 'Login > Statements > Tax Documents > Download', 2021),
('etrade',   'E*TRADE / Morgan Stanley', 'brokerage', 'https://us.etrade.com/etx/pxy/my-account/tax-center', 'Login > Tax Center > Tax Forms > Download all', 2021),
('paypal',   'PayPal',            'payment',   'https://www.paypal.com/myaccount/taxes/', 'Login > Activity > Statements & Tax > Tax Documents', 2021),
('venmo',    'Venmo',             'payment',   'https://venmo.com/account/settings/tax-documents', 'Login > Settings > Tax Documents', 2021),
('ebay',     'eBay',              'payment',   'https://www.ebay.com/sh/reports/tax', 'Login > Seller Hub > Payments > Tax Documents', 2021),
('amazon',   'Amazon',            'payment',   'https://www.amazon.com/gp/css/order-history', 'Login > Orders > Download Order Reports (for business deductions)', 2021),
('godaddy',  'GoDaddy',           'payment',   'https://account.godaddy.com/receipts', 'Login > Account > Billing > Receipts', 2021),
('aws',      'Amazon Web Services','payment',  'https://us-east-1.console.aws.amazon.com/billing/home#/bills', 'Login > Billing > Bills > Download invoices', 2021),
('lowes',    'Lowes',             'payment',   'https://www.lowes.com/mylowes/purchasehistory', 'Login > My Purchases > Download receipts', 2021),
('usaa',     'USAA',              'insurance', 'https://www.usaa.com/inet/wc/insurance-documents', 'Login > Documents > Insurance > Declaration pages', 2021);

-- Government / Tax Authorities
INSERT INTO institutions (code, name, institution_type, portal_url, doc_retrieval_notes, first_year) VALUES
('irs',              'IRS',                    'government', 'https://www.irs.gov/individuals/get-transcript', 'Get Transcript > Online > Select year', 2021),
('texas',            'Texas Comptroller',      'government', 'https://comptroller.texas.gov/taxes/franchise/', 'Franchise tax filing portal', 2021),
('travis-county',    'Travis County Tax',      'government', 'https://tax-office.traviscountytx.gov/property-taxes', 'Property tax search > Enter address > View/print bill', 2021),
('hays-county',      'Hays County Tax',        'government', 'https://tax.co.hays.tx.us/', 'Search by property > View statement', 2021),
('williamson-county','Williamson County Tax',  'government', 'https://tax.wilco.org/', 'Search by address > View/print', 2021);

-- Business Entities (K-1s, etc.)
INSERT INTO institutions (code, name, institution_type, portal_url, doc_retrieval_notes, first_year) VALUES
('tmctech',  'TMCTECH',   'business', NULL, 'K-1 from CPA or generated in tax software', 2021),
('mapttw',   'MAPTTW',    'business', NULL, 'K-1 from CPA or generated in tax software', 2021),
('mapt',     'MAPT',      'business', NULL, 'K-1 from CPA or generated in tax software', 2021),
('gravhl',   'GRAVHL',    'business', NULL, 'K-1 from CPA or generated in tax software', 2021),
('izuma',    'IZUMA',     'business', NULL, 'K-1 from CPA or generated in tax software', 2021),
('tetech',   'TETECH',    'business', NULL, 'K-1 from CPA or generated in tax software', 2021);
```

### Expected Documents Per Institution

```sql
-- Amex
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='amex'), '1099-int', 'Interest income (if savings/high-yield)'),
((SELECT id FROM institutions WHERE code='amex'), 'statement-annual', 'Year-end summary');

-- Chase
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='chase'), '1099-int', 'Interest income'),
((SELECT id FROM institutions WHERE code='chase'), 'statement-annual', 'Year-end credit card summary');

-- E*TRADE
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='etrade'), '1099-b', 'Capital gains/losses'),
((SELECT id FROM institutions WHERE code='etrade'), '1099-div', 'Dividend income'),
((SELECT id FROM institutions WHERE code='etrade'), '1099-int', 'Interest income'),
((SELECT id FROM institutions WHERE code='etrade'), '5498', 'IRA contribution statement'),
((SELECT id FROM institutions WHERE code='etrade'), 'statement-annual', 'Year-end brokerage summary');

-- PayPal
INSERT INTO institution_docs (institution_id, doc_type, description, is_conditional, condition_notes) VALUES
((SELECT id FROM institutions WHERE code='paypal'), '1099-k', 'Payment card transactions', 1, 'Only if gross payments > $600');

-- Venmo
INSERT INTO institution_docs (institution_id, doc_type, description, is_conditional, condition_notes) VALUES
((SELECT id FROM institutions WHERE code='venmo'), '1099-k', 'P2P payment transactions', 1, 'Only if gross payments > $600');

-- eBay
INSERT INTO institution_docs (institution_id, doc_type, description, is_conditional, condition_notes) VALUES
((SELECT id FROM institutions WHERE code='ebay'), '1099-k', 'Marketplace sales', 1, 'Only if gross sales > $600'),
((SELECT id FROM institutions WHERE code='ebay'), 'sales-report', 'Annual sales report for Schedule C');

-- Amazon
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='amazon'), 'summary', 'Annual order summary / report');

-- GoDaddy
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='godaddy'), 'summary', 'Annual receipt summary');

-- AWS
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='aws'), 'summary', 'Annual invoice summary');

-- USAA
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='usaa'), '1098', 'Mortgage interest (per property)'),
((SELECT id FROM institutions WHERE code='usaa'), 'property-insurance', 'Homeowner policy declaration page (per property)');

-- County property taxes
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='travis-county'), 'property-tax', 'Property tax receipt (per property)'),
((SELECT id FROM institutions WHERE code='hays-county'), 'property-tax', 'Property tax receipt (per property)'),
((SELECT id FROM institutions WHERE code='williamson-county'), 'property-tax', 'Property tax receipt (per property)');

-- IRS
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='irs'), '1040-filed', 'Filed tax return (keep copy)'),
((SELECT id FROM institutions WHERE code='irs'), 'transcript', 'Account transcript for verification');

-- Texas
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='texas'), 'franchise-tax', 'Franchise tax filing confirmation');

-- Business entities
INSERT INTO institution_docs (institution_id, doc_type, description) VALUES
((SELECT id FROM institutions WHERE code='tmctech'), 'k1', 'Schedule K-1'),
((SELECT id FROM institutions WHERE code='mapttw'), 'k1', 'Schedule K-1'),
((SELECT id FROM institutions WHERE code='mapt'), 'k1', 'Schedule K-1'),
((SELECT id FROM institutions WHERE code='gravhl'), 'k1', 'Schedule K-1'),
((SELECT id FROM institutions WHERE code='izuma'), 'k1', 'Schedule K-1'),
((SELECT id FROM institutions WHERE code='tetech'), 'k1', 'Schedule K-1');
```

## Workflow

### Action: `status` (default)

Show a dashboard of all tax years with document completion status.

```
=== Tax Document Status ===

2025 (filing due: Apr 15, 2026)
  Received:  8/23 docs
  Missing:  15 docs
    amex       1099-int          https://www.americanexpress.com/...
    chase      1099-int          https://www.chase.com/...
    etrade     1099-b            https://us.etrade.com/.../tax-center
    etrade     1099-div          https://us.etrade.com/.../tax-center
    ...

2024 (FILED)
  All 23 docs received

2023 (FILED)
  All 21 docs received
```

For each missing document, show the portal URL so the user can click and go get it.

### Action: `status <year>`

Show detailed status for one year:

```
=== 2025 Tax Documents ===

  RECEIVED (8):
    amex-statement-annual-9878.pdf           received 2026-01-15
    chase-statement-annual-4589.pdf          received 2026-01-20
    ...

  MISSING (15):
    amex       1099-int
               URL: https://www.americanexpress.com/...
               Note: Usually available by end of January

    etrade     1099-b
               URL: https://us.etrade.com/.../tax-center
               Note: Consolidated 1099, available mid-February

    travis-county  property-tax-7207
               URL: https://tax-office.traviscountytx.gov/...
               Note: Search by address: 7207 ...

  NOT APPLICABLE (2):
    venmo      1099-k             (no payments over threshold)

  DISCONTINUED INSTITUTIONS (ask until 2028):
    lowes      Discontinued 2025. Still asking (year 1 of 3). Need docs? (yes/no)
```

Use `AskUserQuestion` for any discontinued institution confirmations.

### Action: `file`

Process the `incoming/` folder. For each file:

1. **Read the file** using the Read tool (PDFs, images, CSVs)
2. **Identify the institution** from content (letterhead, headers, account numbers)
3. **Identify the doc type** (1099-INT, statement, property tax bill, etc.)
4. **Identify the tax year** from the document content
5. **Propose a filename** following the naming convention
6. **Present to the user** with `AskUserQuestion`:

```
incoming/ConsolidatedTaxStatement_2025.pdf
  Detected: E*TRADE 1099-B for tax year 2025
  Proposed: etrade-1099-b.pdf → 2025/etrade-1099-b.pdf

  yes | rename "new-name" | skip | stop
```

On confirmation:
```bash
mv "BASE_DIR/incoming/<original>" "BASE_DIR/<year>/<new_name>"
```

Update `year_documents` status to `received` and log in `filing_log`.

### Action: `setup <year>`

Initialize a new tax year:

1. Create the year folder if it doesn't exist
2. Copy active institutions from the previous year into `year_institutions`
3. Create expected `year_documents` rows from `institution_docs`
4. Check for discontinued institutions — prompt the 3-year confirmation cycle
5. Ask if any new institutions were added this year

```
Setting up 2025:
  Copied 18 active institutions from 2024
  Created 23 expected document entries

  Discontinued institutions check:
    lowes — discontinued in 2024 (year 1 of 3). Still no docs needed? (yes/no)

  Any new institutions for 2025? (name or "none")
```

## Discontinuation Protocol (3-Year Safety)

When a user says they no longer have an institution:

1. Set `institutions.last_year` to the current tax year
2. Set `institutions.discontinued_at` to today's date
3. Set `institutions.ask_until_year` to current year + 3
4. Log in `discontinuation_log`

Each year during `setup`, for each discontinued institution where `ask_until_year >= current_year`:

```
lowes — You said you no longer have this account (discontinued 2024).
  This is year 2 of 3 safety reminders.
  Still don't need Lowe's docs for 2026? (yes, confirmed / actually I do need it)
```

- If "confirmed": log confirmation, continue asking next year until year 3
- If "actually need it": reactivate — set `last_year = NULL`, `discontinued_at = NULL`, `ask_until_year = NULL`
- After year 3: stop asking, institution stays inactive

Present the remaining count: "I'll stop asking about Lowe's after 2027 (1 more year)."

## Institution Portal URLs

When showing missing documents, always include the portal URL. Format:

```
  etrade     1099-b
             URL: https://us.etrade.com/.../tax-center
             Steps: Login > Tax Center > Tax Forms > Download all
```

If the user adds a new institution, prompt for:
- Portal URL (optional)
- Retrieval steps (optional)
- What document types to expect

Store in `institutions` and `institution_docs`.

## Filing Intelligence

When processing incoming files, use these patterns to identify documents:

| Pattern in document | Institution | Doc type |
|---|---|---|
| "1099-INT", "Interest Income" | (from letterhead) | `1099-int` |
| "1099-DIV", "Dividend Income" | (from letterhead) | `1099-div` |
| "1099-B", "Proceeds From Broker" | (from letterhead) | `1099-b` |
| "1099-K", "Payment Card and Third Party" | (from letterhead) | `1099-k` |
| "1098", "Mortgage Interest Statement" | (from letterhead) | `1098` |
| "Schedule K-1", "Partner's Share" | (from entity name) | `k1` |
| "W-2", "Wage and Tax Statement" | (from employer) | `w2` |
| "5498", "IRA Contribution Information" | (from letterhead) | `5498` |
| "Annual Statement", year-end date | (from letterhead) | `statement-annual` |
| "Property Tax Statement" | (from county name) | `property-tax` |
| "Declaration Page", "Homeowner" | (from insurer) | `property-insurance` |

For PDFs: read the first 2-3 pages with the Read tool.
For images: read with the Read tool (multimodal).

## Adding a New Institution

When the user says "I opened an account at Schwab":

```sql
INSERT INTO institutions (code, name, institution_type, portal_url, doc_retrieval_notes, first_year)
VALUES ('schwab', 'Charles Schwab', 'brokerage', 'https://client.schwab.com/app/accounts/taxforms', 'Login > Tax Forms', 2026);
```

Then ask: "What documents should I expect from Schwab? (e.g., 1099-b, 1099-div, statement-annual)"

Create `institution_docs` entries for each.

## Year-Over-Year Comparison

When running `status`, also highlight changes from the previous year:

```
Changes from 2024 → 2025:
  NEW:     schwab (opened 2025)
  REMOVED: lowes (discontinued 2024, asking until 2027)
  SAME:    16 institutions carried forward
```

## Important Rules

1. **Flat folders** — no subfolders within a year. Everything is a file in `YYYY/`.
2. **Never delete documents** — only move from `incoming/` to `YYYY/`.
3. **Always confirm** before filing a document.
4. **Always show URLs** for missing documents so the user can go get them.
5. **3-year safety** on discontinuations — never silently stop tracking an institution.
6. **Read every incoming file** before proposing a name — don't rely on the filename alone.
7. **Log every filing action** in `filing_log` for audit trail.
8. **Use AskUserQuestion** for all confirmations and decisions.
9. **Year folders are created on demand** during `setup` or `file` actions.
10. **Database lives in `.tax-organizer/`** subdirectory (hidden, won't clutter the Drive folder).
