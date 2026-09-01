# fileclerk-data

Maintain the Finance.xlsx data tab by importing consolidated bank like statements (Ramp CSV, Deel PDF, Chase ALLREC CSV, Apiary, onpay) and matching evidence documents (e.g. ) to transactions. Use when processing financial documents for spreadsheet entry or when matching receipts to existing transactions.

---

## Quick Reference

## Locations

| Item         | Location/Value                                   |
| ------------ | ------------------------------------------------ |
| Spreadsheet  | `/workspace/processing/spreadsheet/Finance.xlsx` |
| Input folder | `/workspace/processing/*-temp`                   |
| Tracking DB  | `data/processed_files.json`                      |
| Data tab     | Sheet named "data"                               |

## Types of Files

- Bank like statements (multiple credit/debit entries over a period) (Ramp CSV, Deel PDF, Chase, Apiary, onpay)
- Evidence documents (invoices, reciepts, contracts, forms etc.)

### Spreadsheet columns to Populate

- B (Details), C (Posting Date), D (Description), E (Amount)
- F (Type), G (Status=Actual), H (site), I (link), J (sublink)
- K (ACRUAL), R (Receipt = filename)

### Formula Columns (NEVER OVERWRITE)

- L (MQTR), P (Split Valid), Q (Spend)
- W (Acategory), X (Asub Category), Y (Asub2), Z (Asub3)

---

## Before Processing Any File

```
- [ ] Check data/processed_files.json for this filename
- [ ] If status="processed" → SKIP (already done)
- [ ] If status="pending_review" → Ask user before proceeding
```

---

## Workflow A: Ramp CSV Import

Use for files matching: `YYMMDD $x,xxx.xx Ramp.csv`

```
- [ ] Step 1: Read the Ramp CSV file
- [ ] Step 2: Find the "Transfer from Izuma Checking" row → this is parent amount
- [ ] Step 3: Create PARENT row in spreadsheet:
  - Details: DEBIT-IN1
  - Posting Date: Clearing Date of transfer row
  - Description: From CSV (the ACH description)
  - Amount: Transfer amount (negative)
  - Type: ACH_DEBIT or MISC_DEBIT
  - Status: Actual
  - site: ramp.split
  - link: Ramp
  - sublink: R[YYMM] (e.g., R2401 for Jan 2024)
  - ACRUAL: Same as Posting Date
- [ ] Step 4: For each Transaction row, create CHILD row:
  - Details: ramp_split
  - Posting Date: SAME as parent (inherit from parent)
  - Description: Merchant Name from CSV
  - Amount: -Amount (negate, Ramp positives are debits)
  - Type: CC_{User} (e.g., CC_Travis Mccollum)
  - Status: Actual
  - site: Map merchant to site (see REFERENCE.md)
  - link: Ramp
  - sublink: (leave blank for Ramp children)
  - ACRUAL: Clearing Date from CSV
  - Receipt: "yes" if Has Receipt="Yes" in CSV
- [ ] Step 5: Validate sum of child amounts = parent amount
- [ ] Step 6: Check spreadsheet for duplicate (date + amount match)
- [ ] Step 7: Append rows to data tab (parent first, then children)
- [ ] Step 8: Update processed_files.json with status="processed"
```

---

## Workflow B: Deel PDF Import

Use for files matching: `YYMMDD $x,xxx.xx Deel.pdf`

```
- [ ] Step 1: Read the Deel PDF
- [ ] Step 2: Extract total amount, employee names, salaries, fees
- [ ] Step 3: Create PARENT row:
  - Details: DEBIT-IN1
  - Posting Date: From filename YYMMDD
  - Description: "Deel payroll" or from bank description
  - Amount: Total (negative)
  - Type: ACH_DEBIT
  - site: deel.com
  - link: deel
  - sublink: D[YYMM] (e.g., D2306)
- [ ] Step 4: For each employee salary, create CHILD row:
  - Details: SPLIT
  - Posting Date: SAME as parent
  - Description: "{Employee} Salary"
  - Amount: Salary amount (negative)
  - Type: EMPLOYEE_SALARY
  - site: deel.{employee} (e.g., deel.pete, deel.jenia)
  - link: deel
  - sublink: D[YYMM]. (with period)
  - ACRUAL: End of month
- [ ] Step 5: For each Deel fee, create CHILD row:
  - Details: SPLIT
  - site: deel.fee
  - Type: Fee
- [ ] Step 6: Validate splits sum to parent
- [ ] Step 7: Check for duplicates, append, update JSON
```

---

## Workflow C: Evidence Matching

Use for receipt/invoice PDFs that are NOT bank statements.

```
- [ ] Step 1: Parse filename: YYMMDD $amount Vendor.pdf
- [ ] Step 2: Search data tab for matching transaction:
  - Amount within $0.01
  - Posting Date within ±7 days
  - site or Description contains vendor name
- [ ] Step 3: If single match found:
  - Update Receipt column (R) with filename
  - Update processed_files.json
- [ ] Step 4: If multiple matches:
  - Present candidates with row numbers
  - Ask user to select
- [ ] Step 5: If no match:
  - Report: "No matching transaction found for {filename}"
  - Mark as status="no_match" in JSON
```

---

## Workflow D: Chase ALLREC CSV Import

Use for files matching: `YYMMDD Chase XXXX ALLREC.csv`

These are 2-year activity exports from Chase bank. Most rows already exist in the spreadsheet — only **new entries since last import** should be added.

### Account Mapping

| Last4 | Bank | Entity | Account Name | Code | Details Prefix |
|-------|------|--------|--------------|------|----------------|
| 7505 | Chase | Izuma Networks | Main Checking | IN1 | DEBIT-IN1 / CREDIT-IN1 |
| 3839 | Chase | Izuma Networks | Secondary Checking | IN2 | DEBIT-IN2 / CREDIT-IN2 |
| 3211 | Chase | Izuma Networks | Savings | IS1 | DEBIT-IS1 / CREDIT-IS1 |
| 6557 | Chase | Izuma Tech | Checking | IT1 | DEBIT-IT1 / CREDIT-IT1 |

```
- [ ] Step 1: Detect ALLREC file in incoming-temp
  - Extract last4 from filename (the XXXX in "Chase XXXX ALLREC")
  - Look up account code from mapping table above
- [ ] Step 2: Read CSV and parse all rows
  - Skip header row
  - For each row extract: Details, Posting Date, Description, Amount, Type
- [ ] Step 3: Load existing spreadsheet data for deduplication
  - Read all rows from "data" sheet
  - Build lookup index: set of (posting_date, amount) tuples
    for the relevant Details codes (e.g., DEBIT-IN1 and CREDIT-IN1 for account 7505)
- [ ] Step 4: Deduplicate — identify new rows only
  - For each CSV row, check if (posting_date, amount) exists in lookup index
  - Match criteria: exact date AND exact amount (±$0.01)
  - If match found → skip (already in spreadsheet)
  - If no match → mark as new, queue for insertion
  - Report count: "X of Y rows are new"
- [ ] Step 5: Map CSV fields to spreadsheet columns
  For each new row:
  - B (Details): Map CSV Details field:
    DEBIT → DEBIT-{code}, CREDIT → CREDIT-{code}, DSLIP → CREDIT-{code}
  - C (Posting Date): CSV Posting Date (convert MM/DD/YYYY → date)
  - D (Description): CSV Description
  - E (Amount): CSV Amount (already signed)
  - F (Type): CSV Type (already matches spreadsheet format)
  - G (Status): "Actual"
  - H (site): Determine from Description (see Step 6)
  - I (link): Determine from site value (see Step 6b)
  - J (sublink): blank for normal rows; for transfers (link=XFER) set X<YYMMDD> to pair the two legs
  - K (ACRUAL): Same as Posting Date
  - R (Receipt): (blank)
- [ ] Step 6: Site determination from Description
  Apply these rules in order:
  1. Description contains "SERVICE CHARGES" → chase.com
  2. "Online Transfer from/to CHK ...####": if #### is an internal Chase acct (7505/3839/3211/6557) → chase.#### (link XFER, sublink X<YYMMDD>); else → funding
  3. Description contains "ODP TRANSFER" → funding
  4. Check site_keywords.json high_confidence mappings against Description
  5. If no match → leave blank
- [ ] Step 6b: Link determination from site (H) value
  Apply link_from_site() mapping:
  - ramp.split, trial.deposits → "Ramp"
  - onpay.split, onpay.* → "Onpay"
  - deel.com, deel.* → "deel"
  - funding, chase.NNNN, wells.* → "XFER"
  - chase.com → "Chase"
  - cogentISP → "cogent"
  - switchdf, switch.* → "switch"
  - apiaryx* → "apiaryx"
  - bacancy*, Bacancy.com → "bacancy"
  - BCBS → "bcbs"
  - TLIP* → "tlip"
  - safe.loan → "safe"
  - pelion → "pelion"
  - HCSC → "HCSC"
  - UHC → "UHC"
  - All other sites → "Chase" (default)
- [ ] Step 7: Append new rows to data tab
  - Insert at end of data tab
  - NEVER write to formula columns (L, P, Q, W, X, Y, Z)
- [ ] Step 8: Move CSV to processed folder
  - Move from incoming-temp/ to processed/
  - Update data/processed_files.json with status="processed"
- [ ] Step 9: Report summary
  === Chase ALLREC Processing ===
  File: {filename}
  Account: {code} ({entity} {account_name})
  Total rows in CSV: {total}
  Already in spreadsheet: {skipped}
  New rows inserted: {new}
  Moved to: processed/
```

---

## Split Rules Summary

| Rule                   | Value                                     |
| ---------------------- | ----------------------------------------- |
| Child Posting Date (C) | Always inherit from parent                |
| Parent sublink         | No period (e.g., D2306)                   |
| Child sublink          | Ends with period (e.g., D2306.)           |
| Validation             | Sum of child Split Valid = -parent Amount |
| ACRUAL default         | Same as Posting Date unless specified     |

---

## Site Mapping Quick Reference

When setting column H (site), use exact values from VARS tab. If uncertain, leave blank.

**High confidence mappings:**

- 1PASSWORD → `1password.com`
- ATLASSIAN → `Atlassian.com`
- CLOUDFLARE → `cloudflare.com`
- INTUIT → `intuit`
- MICROSOFT/MSFT → `microsoft.com`
- SLACK → `slack.com`

See `data/site_keywords.json` for complete mapping.

---

## Duplicate Prevention

Before inserting ANY row:

1. Check processed_files.json
2. Search spreadsheet for: same date (±1 day) AND same amount (±$0.01)
3. If potential duplicate found → ASK user before proceeding

---

## Error Handling

- **Unparseable filename**: Log to JSON with status="manual", ask user
- **Missing site mapping**: Leave H blank, formulas will show empty categories
- **Split validation fails**: STOP, report discrepancy, do not insert
- **Formula columns**: NEVER write to L, P, Q, W, X, Y, Z
