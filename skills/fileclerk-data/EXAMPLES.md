# fileclerk-data Examples

## Example 1: Ramp CSV Import

**Input file**: `230120 $650.76 Ramp.csv`

**CSV contents** (excerpt):
```csv
"Transaction Date","Clearing Date","Type","Amount","User","Merchant Name","Has Receipt"
"12/25/22","01/03/23","Transfer from Izuma Checking","-6321.82","","","No"
"01/01/23","01/01/23","Transaction","8.52","Travis Mccollum","Microsoft Store","No"
"01/08/23","01/09/23","Transaction","21.27","Travis Mccollum","1PASSWORD","No"
```

**Generated rows**:

| Details | Posting Date | Description | Amount | Type | site | link | sublink | ACRUAL |
|---------|--------------|-------------|--------|------|------|------|---------|--------|
| DEBIT-IN1 | 2023-01-03 | ORIG CO NAME:RAMP... | -650.76 | ACH_DEBIT | ramp.split | Ramp | R2301 | 2023-01-03 |
| ramp_split | 2023-01-03 | Microsoft Store | -8.52 | CC_Travis Mccollum | microsoft.com | Ramp | | 2023-01-01 |
| ramp_split | 2023-01-03 | 1PASSWORD | -21.27 | CC_Travis Mccollum | 1password.com | Ramp | | 2023-01-09 |

**Notes**:
- Parent row uses Clearing Date of transfer as Posting Date
- Children inherit parent's Posting Date in column C
- Children use their own Clearing Date in ACRUAL (K)
- site values mapped from Merchant Name

---

## Example 2: Deel PDF Import

**Input file**: `230804 $63,958.99 Deel.pdf`

**Extracted from PDF**:
- Total: $63,958.99
- Jenia Salary: $18,750.97
- Janne Salary: $16,974.86
- Pete Salary: $15,177.46
- Gabi Salary: $5,211.92
- Eran Salary: $4,762.04
- Deel fees: ~$3,081 total

**Generated rows**:

| Details | Posting Date | Description | Amount | Type | site | link | sublink | ACRUAL |
|---------|--------------|-------------|--------|------|------|------|---------|--------|
| DEBIT-IN1 | 2023-08-04 | Deel payroll | -63,958.99 | ACH_DEBIT | deel.com | deel | D2308 | 2023-08-04 |
| SPLIT | 2023-08-04 | Jenia Salary | -18,750.97 | EMPLOYEE_SALARY | deel.jenia | deel | D2308. | 2023-08-31 |
| SPLIT | 2023-08-04 | Janne salary | -16,974.86 | EMPLOYEE_SALARY | deel.janne | deel | D2308. | 2023-08-31 |
| SPLIT | 2023-08-04 | Pete Salary | -15,177.46 | EMPLOYEE_SALARY | deel.pete | deel | D2308. | 2023-08-31 |
| SPLIT | 2023-08-04 | Gabi salary | -5,211.92 | EMPLOYEE_SALARY | deel.gabi | deel | D2308. | 2023-08-31 |
| SPLIT | 2023-08-04 | Eran Salary | -4,762.04 | EMPLOYEE_SALARY | deel.eran | deel | D2308. | 2023-08-31 |
| SPLIT | 2023-08-04 | DEEL Fee Jenia | -621.76 | Fee | deel.fee | deel | D2308. | 2023-08-31 |
| ... | | | | | | | | |

**Notes**:
- Parent sublink: D2308 (no period)
- Child sublinks: D2308. (with period)
- ACRUAL for salaries: end of month (2023-08-31)
- All children inherit parent's Posting Date (2023-08-04)

---

## Example 3: Evidence Matching

**Input file**: `220724 $91.24 PrintEZ.pdf`

**Parsed from filename**:
- Date: 2022-07-24
- Amount: $91.24
- Vendor: PrintEZ

**Search criteria**:
- Amount: $91.24 ± $0.01
- Date: 2022-07-17 to 2022-07-31
- site/Description contains "print"

**Match found**:
- Row 35: Posting Date=2022-07-27, Amount=-91.24, site=Printez.com

**Action**:
- Update Receipt column (R) in row 35 with: `220724 $91.24 PrintEZ.pdf`

---

## Example 4: No Match Found

**Input file**: `230115 $5,000.00 Unknown.pdf`

**Search result**: No transactions match date + amount

**Action**:
- Report to user: "No matching transaction for 230115 $5,000.00 Unknown.pdf"
- Update processed_files.json with status="no_match"
- User can:
  1. Manually add to spreadsheet
  2. Mark as evidence-only
  3. Investigate further

---

## Example 5: Chase ALLREC CSV Import

**Input file**: `260218 Chase 7505 ALLREC.csv`

**CSV contents** (excerpt):
```csv
Details,Posting Date,Description,Amount,Type,Balance,Check or Slip #
DEBIT,01/27/2026,"Online ACH Payment 11200884273 To CogentInternet",-662.50,ACH_PAYMENT,23902.43,,
CREDIT,02/02/2026,"Online Transfer from CHK ...6557 transaction#: 16720338547",100000.00,ACCT_XFER,121531.13,,
DSLIP,11/28/2025,"FOREIGN REMITTANCE CREDIT B/O: IZUMA LTD",317480.47,DEPOSIT,332874.99,,
DEBIT,02/01/2026,"SERVICE CHARGES",-25.00,FEE_TRANSACTION,121506.13,,
```

**Account lookup**: last4=7505 → Code=IN1 (Izuma Networks Main Checking)

**Deduplication result**: 204 total rows, 198 already in spreadsheet, 6 new

**Generated rows** (for the new entries only):

| Details | Posting Date | Description | Amount | Type | Status | site | link | sublink | ACRUAL |
|---------|--------------|-------------|--------|------|--------|------|------|---------|--------|
| DEBIT-IN1 | 2026-01-27 | Online ACH Payment 11200884273 To CogentInternet | -662.50 | ACH_PAYMENT | Actual | cogentISP | cogent | | 2026-01-27 |
| CREDIT-IN1 | 2026-02-02 | Online Transfer from CHK ...6557 transaction#: 16720338547 | 100000.00 | ACCT_XFER | Actual | funding | XFER | | 2026-02-02 |
| CREDIT-IN1 | 2025-11-28 | FOREIGN REMITTANCE CREDIT B/O: IZUMA LTD | 317480.47 | DEPOSIT | Actual | | Chase | | 2025-11-28 |
| DEBIT-IN1 | 2026-02-01 | SERVICE CHARGES | -25.00 | FEE_TRANSACTION | Actual | chase.com | Chase | | 2026-02-01 |

**Notes**:
- DSLIP maps to CREDIT-IN1 (deposit slips are credits)
- "CogentInternet" in description → site=`cogentISP` → link=`cogent`
- "Online Transfer from CHK" → site=`funding` → link=`XFER`
- "SERVICE CHARGES" → site=`chase.com` → link=`Chase`
- "FOREIGN REMITTANCE" has no keyword match → site blank → link=`Chase` (default)
- link is derived from site via `link_from_site()` (see SKILL.md Step 6b)
- sublink is always blank (these are not split transactions)

**Summary output**:
```
=== Chase ALLREC Processing ===
File: 260218 Chase 7505 ALLREC.csv
Account: IN1 (Izuma Networks Main Checking)
Total rows in CSV: 204
Already in spreadsheet: 198
New rows inserted: 6
Moved to: processed/
```

---

## Validation Example

After creating Deel splits:

```
Parent Amount: -63,958.99
Children Sum:
  -18,750.97 (Jenia)
  -16,974.86 (Janne)
  -15,177.46 (Pete)
  -5,211.92 (Gabi)
  -4,762.04 (Eran)
  -621.76 (Fee Jenia)
  -621.76 (Fee Gabi)
  -621.76 (Fee Jenia)
  -605.98 (Fee Janne)
  -605.48 (Fee Pete)
  -5.00 (Wire Fee)
  = -63,958.99

Validation: PASS (sum matches parent)
```

If validation fails, DO NOT insert rows. Report discrepancy to user.
