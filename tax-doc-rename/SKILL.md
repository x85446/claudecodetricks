---
name: tax-doc-rename
description: Rename and organize tax documents in incoming folders. Use when processing Etrade, Morgan Stanley, or brokerage tax documents, renaming 1099s, organizing tax files, or when user mentions tax documents in an incoming folder.
allowed-tools: Read, Bash, Glob, Grep
---

# Tax Document Rename & Organize

Renames tax documents in `<YEAR>/incoming/` folders based on their contents and a consistent naming convention.

## Workflow

1. **List files** in the `<YEAR>/incoming/` folder
2. **Read each PDF** to extract account numbers, tax year, form type, and issuer
3. **Match to known accounts** using the account table below
4. **Rename files** following the naming convention
5. **Delete** any documents for tax years other than the folder year (e.g., 2024 docs in 2023 folder)
6. **Update CLAUDE.md** in the year folder if new accounts are discovered

## Naming Convention

**Format:** `YYYY <TYPE> Etrade -<LAST4> <ACCOUNT_NAME> <FORM>.pdf`

| Field | Description |
|-------|-------------|
| YYYY | Tax year (e.g., 2023) |
| TYPE | `BROKE` for brokerage/trading accounts, `BANK` for checking/savings accounts |
| LAST4 | Last 4 digits of account number |
| ACCOUNT_NAME | Friendly name for the account |
| FORM | Tax form type (e.g., 1099, 1099-INT, Supplemental, Trades) |

### Examples
```
2023 BROKE Etrade -9099 LOGI 1099.pdf
2023 BROKE Etrade -7027 MJoint 1099.pdf
2023 BROKE Etrade -3362 MRoth Trades.csv
2023 BANK Etrade -7800 Melissa 1099-INT.pdf
2023 BROKE Etrade -9099 LOGI Supplemental.pdf
```

## Known Accounts

| Last 4 | Type | Account Name | Owner | Notes |
|--------|------|--------------|-------|-------|
| 9099 | BROKE | LOGI | Travis | Logitech stock plan |
| 7027 | BROKE | MJoint | Joint | Joint brokerage |
| 3362 | BROKE | MRoth | Melissa | Melissa's Roth IRA |
| 7978 | BROKE | MRoth | Melissa | Melissa's Roth IRA |
| 0159 | BROKE | TRoth | Travis | Travis's Roth IRA |
| 9558 | BROKE | TBroke | Travis | Travis's brokerage |
| 7800 | BANK | Melissa | Melissa | Bank accounts (1099-INT) |
| 3611 | BROKE | FranklinZroth | ? | Franklin Templeton (076-509113611) |
| 4491 | BROKE | FranklinZ | ? | Franklin Templeton (076-508314491) |
| 7487 | BROKE | FranklinA | ? | Franklin Templeton (476-2747487) |

## 2023 E*TRADE to Morgan Stanley Migration

In 2023, E*TRADE accounts migrated to Morgan Stanley. Some accounts have **two 1099s** for the same tax year:
- One from E*TRADE (pre-migration, Jan-Sep 2023)
- One from Morgan Stanley (post-migration, Sep-Dec 2023)

**Both are needed for tax filing.**

### Migration Account Mapping

| Morgan Stanley Acct | Old E*TRADE Acct | Account Name |
|---------------------|------------------|--------------|
| 572 369099 209 (-9099) | 63933020 (-3020) | LOGI |
| 676 407027 205 (-7027) | 67315088 (-5088) | MJoint |

### Migration Naming Convention
When two 1099s exist for the same account due to migration:
```
2023 BROKE Etrade -9099 LOGI 1099 (Morgan Stanley).pdf
2023 BROKE Etrade -9099 LOGI 1099 (3020 Etrade Pre-Migration).pdf
2023 BROKE Etrade -7027 MJoint 1099 (Morgan Stanley).pdf
2023 BROKE Etrade -7027 MJoint 1099 (5088 Etrade Pre-Migration).pdf
```

## Detecting Document Type

### From Filename Patterns
| Original Pattern | Form Type |
|------------------|-----------|
| `MS_YYYY_1099-CONS_*` | 1099 (Morgan Stanley consolidated) |
| `TY YYYY-1099+CONSOLIDATED-*` | 1099 (E*TRADE consolidated) |
| `getSupplementalInformation*` | Supplemental |
| `taxreportpdf*` or `tsptaxreportpdf*` | 1099-INT |
| `tradesdownload*` | Trades |
| `tx1099int_*` | 1099-INT (TXF import file) |

### From PDF Contents
- **Account number**: Look for "Account Number", "Account No:", "Account number ending in:"
- **Tax year**: Look for "Tax Year YYYY", "YYYY FORM 1099", reporting period dates
- **Form type**: Look for "1099-DIV", "1099-INT", "1099-B", "1099-CONS"
- **Issuer**: "Morgan Stanley" vs "E*TRADE Securities LLC" determines migration status

## Instructions

### Step 1: Survey the incoming folder
```bash
ls -la "<YEAR>/incoming/"
```

### Step 2: Read each document
For PDFs, read and extract:
- Account number (last 4 digits)
- Tax year
- Form type
- Issuer (Morgan Stanley vs E*TRADE)

### Step 3: Determine new filename
1. Match account to known accounts table
2. If unknown account, ask user for account name and type
3. Apply naming convention
4. For migration docs, add (Morgan Stanley) or (<old_last4> Etrade Pre-Migration) suffix

### Step 4: Rename files
```bash
mv "old_name.pdf" "YYYY TYPE Etrade -LAST4 ACCOUNT_NAME FORM.pdf"
```

### Step 5: Clean up
- Delete documents for wrong tax year
- Delete obvious duplicates (same content, same size)
- Keep TXF files (TurboTax import) with matching names

### Step 6: Report results
List all renamed files and any issues found.

## Franklin Templeton Consolidated PDFs

Franklin Templeton sends a single consolidated 1099 PDF containing multiple accounts.
These must be split into separate files before renaming.

### Franklin Account Numbers

| Full Account | Last 4 | Account Name |
|--------------|--------|--------------|
| 076-509113611 | 3611 | FranklinZroth |
| 076-508314491 | 4491 | FranklinZ |
| 476-2747487 | 7487 | FranklinA |

### Splitting Workflow

1. **Detect consolidated PDF**: Look for Franklin files in incoming folder
2. **Read PDF and scan for account numbers**: Search text for patterns like `076-509113611`, `076-508314491`, `476-2747487`
3. **Identify page boundaries**: Each account section starts on a new page with its account number displayed
4. **Split PDF**: Use pypdf to extract pages for each account into separate files
5. **Rename each split file**: Apply standard naming convention

### Franklin Naming Convention
```
YYYY BROKE Franklin -<LAST4> <ACCOUNT_NAME> 1099.pdf
```

Examples:
```
2023 BROKE Franklin -3611 FranklinZroth 1099.pdf
2023 BROKE Franklin -4491 FranklinZ 1099.pdf
2023 BROKE Franklin -7487 FranklinA 1099.pdf
```

### Splitting Instructions

```python
# Use pypdf (already installed) to split
from pypdf import PdfReader, PdfWriter
import pdfplumber

# 1. Read PDF and find account boundaries
reader = PdfReader("consolidated.pdf")
account_pages = {}  # {account_number: [page_indices]}

with pdfplumber.open("consolidated.pdf") as pdf:
    for i, page in enumerate(pdf.pages):
        text = page.extract_text()
        for acct in ["076-509113611", "076-508314491", "476-2747487"]:
            if acct in text:
                # Found start of this account's section
                # Track pages until next account found

# 2. Write separate PDFs for each account
for acct, pages in account_pages.items():
    writer = PdfWriter()
    for page_idx in pages:
        writer.add_page(reader.pages[page_idx])
    writer.write(f"split_{acct[-4:]}.pdf")
```

## Incoming Folder Workflow

### Source
Documents to process are placed in `<YEAR>/incoming/`

### Processing Steps
1. List files in `<YEAR>/incoming/`
2. For each file:
   - **If Franklin consolidated**: Split first into separate account PDFs, then rename each
   - **If single document**: Rename directly
3. **Move renamed files to `<YEAR>/`** (root of year folder, not a subdirectory)
4. Delete or archive originals from incoming/ after confirming success

### Destination
Processed files are moved to the root `<YEAR>/` directory.

```bash
# Example: After processing
mv "2023 BROKE Franklin -3611 FranklinZroth 1099.pdf" "../"
# Results in: 2023/2023 BROKE Franklin -3611 FranklinZroth 1099.pdf
```

## Notes

- Always read PDFs before renaming to verify contents
- Ask user before deleting any files
- If account is unknown, ask user for the account name and whether it's BROKE or BANK
- Update the year's CLAUDE.md if new accounts are discovered
- Morgan Stanley Private Bank documents are BANK type (checking/savings interest)
- E*TRADE brokerage/stock plan documents are BROKE type
- Franklin Templeton documents use "Franklin" as the provider in the filename (not "Etrade")
