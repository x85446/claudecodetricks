# fileclerk-data Reference

## Column Definitions

| Col | Letter | Name | Description | Populate? |
|-----|--------|------|-------------|-----------|
| 1 | A | (empty) | Row marker | No |
| 2 | B | Details | Source code: DEBIT-IN1, CREDIT-IN1, SPLIT, ramp_split | Yes |
| 3 | C | Posting Date | Bank posting date | Yes |
| 4 | D | Description | Transaction description | Yes |
| 5 | E | Amount | Signed amount (-=debit, +=credit) | Yes |
| 6 | F | Type | Transaction type | Yes |
| 7 | G | Status | Actual, Planned, Pretend | Yes (Actual) |
| 8 | H | site | Vendor identifier (drives VARS lookup) | Yes |
| 9 | I | link | Source identifier (deel, Ramp, Onpay) | Yes |
| 10 | J | sublink | Split identifier [Prefix][YYMM] | Yes |
| 11 | K | ACRUAL | Accrual date | Yes |
| 12 | L | MQTR | Quarter (FORMULA) | NO |
| 13-15 | M-O | (unnamed) | Internal use | No |
| 16 | P | Split Valid | (FORMULA) | NO |
| 17 | Q | Spend | (FORMULA) | NO |
| 18 | R | Receipt | Filename of evidence document | Yes |
| 19-20 | S-T | (unnamed) | Internal use | No |
| 21 | U | no match | Matching notes | Optional |
| 22 | V | dno | Internal | No |
| 23 | W | Acategory | (FORMULA from VARS) | NO |
| 24 | X | Asub Category | (FORMULA from VARS) | NO |
| 25 | Y | Asub2 | (FORMULA from VARS) | NO |
| 26 | Z | Asub3 | (FORMULA from VARS) | NO |

---

## Details Column Values

| Value | Meaning | When to Use |
|-------|---------|-------------|
| DEBIT-IN1 | Debit from Izuma Networks Account 1 | Chase IN1 debits |
| CREDIT-IN1 | Credit to Izuma Networks Account 1 | Chase IN1 credits |
| DEBIT-IT1 | Debit from Izuma Tech Account 1 | Chase IT1 debits |
| CREDIT-IT1 | Credit to Izuma Tech Account 1 | Chase IT1 credits |
| WF | Wells Fargo transaction | Wells Fargo activity |
| SPLIT | Child row of a split transaction | Deel/Onpay/Bacancy salary breakdowns |
| ramp_split | Child row from Ramp CSV import | Ramp credit card line items |
| PLANNED | Planned future transaction | Forecasting |

---

## Type Column Values

| Type | Usage |
|------|-------|
| EMPLOYEE_SALARY | Salary payments (Deel, Onpay) |
| EMPLOYEE_REIMBURSEMENT | Employee expense reimbursements |
| Fee | Service fees (Deel fees, bank fees) |
| FEE_TRANSACTION | Bank transaction fees |
| ACH_DEBIT | ACH debit from bank |
| ACH_CREDIT | ACH credit to bank |
| ACH_PAYMENT | ACH payment sent |
| WIRE_OUTGOING | Outgoing wire transfer |
| WIRE_INCOMING | Incoming wire transfer |
| CC_Travis Mccollum | Ramp card - Travis |
| CC_Thomas Hemphill | Ramp card - Thomas |
| CC_DEBIT | Generic credit card debit |
| DEBIT_CARD | Debit card transaction |
| MISC_DEBIT | Miscellaneous debit |
| MISC_CREDIT | Miscellaneous credit |
| ACCT_XFER | Account transfer |
| Expense Report | Expense report line items |

---

## Sublink Format

Format: `[Prefix][YYMM]` with optional period for children

| Vendor | Prefix | Parent Example | Child Example |
|--------|--------|----------------|---------------|
| Deel | D | D2306 | D2306. |
| Onpay | OP | OP2309 | OP2309. |
| Ramp | R | R2401 | (children don't use sublink) |
| Bacancy | B | B2208 | B2208. |
| ApiaryX | APX | APX01 | APX01. |

**Rule**: Parent sublinks do NOT end with period. Child sublinks END with period.
This ensures sorting places parent row above children.

---

## Vendor-Specific Rules

### DEEL

**Source**: Deel payroll invoices (PDF)
**Frequency**: Monthly

| Field | Parent Row | Child Rows (Salaries) | Child Rows (Fees) |
|-------|------------|----------------------|-------------------|
| Details | DEBIT-IN1 | SPLIT | SPLIT |
| site | deel.com | deel.{employee} | deel.fee |
| link | deel | deel | deel |
| Type | ACH_DEBIT | EMPLOYEE_SALARY | Fee |
| ACRUAL | Posting date | End of month | End of month |

**Employee site values**: deel.pete, deel.jenia, deel.janne, deel.gabi, deel.eran
**Extract employee names from PDF content**

### ONPAY

**Source**: Onpay payroll records
**Frequency**: Bi-weekly or monthly

| Field | Parent Row | Child Rows |
|-------|------------|------------|
| Details | DEBIT-IN1 | SPLIT |
| site | onpay.split | onpay.{employee} or onpay.fee |
| link | Onpay | Onpay |
| Type | ACH_DEBIT | EMPLOYEE_SALARY |

**Employee site values**: onpay.ed, onpay.travis, onpay.yash
**Special**: `.r` suffix for reimbursements (e.g., onpay.ed.r)

### RAMP

**Source**: Ramp CSV statement export
**Frequency**: Monthly

| Field | Parent Row | Child Rows |
|-------|------------|------------|
| Details | DEBIT-IN1 | ramp_split |
| site | ramp.split | {merchant}.com |
| link | Ramp | Ramp |
| sublink | R[YYMM] | (blank) |
| Type | ACH_DEBIT/MISC_DEBIT | CC_{User} |
| ACRUAL | Posting date | Clearing Date from CSV |

**Child site**: Map from Merchant Name in CSV to known site values

### BACANCY

**Source**: Bacancy contractor payments
**Child sites**: Bacancy.abhisek, Bacancy.tulsi, Bacancy.vishal, Bacancy.prtik

### APIARYX

**Source**: ApiaryX contractor payments
**Child sites**: apiaryx.abhishek, apiaryx.suprith, apiaryx.utkarsh, apiaryx.commission

---

## Ramp CSV Column Mapping

| CSV Column | → Spreadsheet Column | Notes |
|------------|---------------------|-------|
| Clearing Date | ACRUAL (K) | For child rows |
| Merchant Name | Description (D) | |
| Amount | Amount (E) | Negate (positive → negative) |
| User | Type (F) | CC_{User} |
| Has Receipt | Receipt (R) | "Yes" → "yes" |
| Outstanding Balance | (last row) | Parent amount |

**Parent identification**: Row with Type="Transfer from Izuma Checking"

---

## Chase Account Mapping

| Last4 | Bank | Entity | Account Name | Code | Details Prefix |
|-------|------|--------|--------------|------|----------------|
| 7505 | Chase | Izuma Networks | Main Checking | IN1 | DEBIT-IN1 / CREDIT-IN1 |
| 3839 | Chase | Izuma Networks | Secondary Checking | IN2 | DEBIT-IN2 / CREDIT-IN2 |
| 3211 | Chase | Izuma Networks | Savings | IS1 | DEBIT-IS1 / CREDIT-IS1 |
| 6557 | Chase | Izuma Tech | Checking | IT1 | DEBIT-IT1 / CREDIT-IT1 |
| (any) | Wells Fargo | — | — | W1 | WF |

---

## Chase ALLREC CSV Column Mapping

**CSV format**: `Details,Posting Date,Description,Amount,Type,Balance,Check or Slip #`

| CSV Column | → Spreadsheet Column | Notes |
|------------|---------------------|-------|
| Details | Details (B) | Map: DEBIT→`DEBIT-{code}`, CREDIT→`CREDIT-{code}`, DSLIP→`CREDIT-{code}` |
| Posting Date | Posting Date (C) | Convert MM/DD/YYYY → date |
| Description | Description (D) | Use as-is |
| Amount | Amount (E) | Already signed (-=debit, +=credit) |
| Type | Type (F) | Use as-is (ACH_PAYMENT, ACCT_XFER, DEPOSIT, etc.) |
| Balance | (not used) | Skip |
| Check or Slip # | (not used) | Skip |

**Additional columns set per row**:

| Spreadsheet Col | Value |
|-----------------|-------|
| G (Status) | "Actual" |
| H (site) | From Description (see site rules) |
| I (link) | From site value (see link rules) |
| J (sublink) | (blank) |
| K (ACRUAL) | Same as Posting Date |
| R (Receipt) | (blank) |

### Chase ALLREC Site Rules (applied in order)

1. `SERVICE CHARGES` in Description → `chase.com`
2. `Online Transfer` with account reference → `funding`
3. `ODP TRANSFER` → `funding`
4. Match against `site_keywords.json` high_confidence mappings
5. No match → leave blank

### Chase ALLREC Link Rules (from site value)

After determining site (H), derive link (I) using `link_from_site()`:

| Site Pattern | Link Value | Reason |
|-------------|------------|--------|
| `ramp.split`, `trial.deposits` | Ramp | Ramp card payment |
| `onpay.split`, `onpay.*` | Onpay | OnPay payroll |
| `deel.com`, `deel.*` | deel | Deel payroll |
| `funding`, `chase.NNNN`, `wells.*` | XFER | Inter-account transfer |
| `chase.com` | Chase | Bank fees |
| `cogentISP` | cogent | ISP vendor |
| `switchdf`, `switch.*` | switch | DataFoundry/Switch |
| `apiaryx*` | apiaryx | ApiaryX contractor |
| `bacancy*`, `Bacancy.com` | bacancy | Bacancy contractor |
| `BCBS` | bcbs | Insurance |
| `TLIP*` | tlip | TLIP vendor |
| `safe.loan` | safe | SAFE loan |
| `pelion` | pelion | Pelion entity |
| `HCSC` | HCSC | Health care |
| `UHC` | UHC | United Healthcare |
| (everything else) | Chase | Default bank transaction |

---

## Site Keyword Mapping

When processing Description (D) to determine site (H):

### Direct Mappings (High Confidence)
```
1PASSWORD     → 1password.com
ATLASSIAN     → Atlassian.com
CLOUDFLARE    → cloudflare.com
HEROKU        → heroku.com
INTUIT        → intuit
MICROSOFT     → microsoft.com
MSFT          → microsoft.com
SEMRUSH       → Semrush.Com
SLACK         → slack.com
CIRCLECI      → circleci
HUBSPOT       → hubspot.com
WEWORK        → wework
UBIQUITI      → ubiquiti
EBAY          → ebay.com
GOOGLE        → google
ADOBE         → adobe
AHREFS        → Ahrefs
OPENAI        → openai
AWS           → aws
COGENT        → cogentISP
```

### Context-Dependent (Requires Analysis)
```
DEEL          → deel.com (parent) OR deel.{employee} OR deel.fee
ONPAY         → onpay.split (parent) OR onpay.{employee}
AMAZON        → amazon.it.hw OR aws OR travel.marketing
CHASE         → chase.com OR funding
RAMP          → ramp.split (parent only)
```

**Rule**: If no confident match, leave site (H) BLANK. Do not guess.

---

## Split Validation

After creating parent + children rows:

```python
parent_amount = abs(parent['Amount'])  # E column, make positive
children_sum = sum(abs(child['Amount']) for child in children)

if abs(parent_amount - children_sum) > 0.01:
    raise ValidationError(f"Split mismatch: parent={parent_amount}, children={children_sum}")
```

The formula in column P (Split Valid) will automatically:
- For non-SPLIT rows: negate Amount
- For SPLIT rows: keep Amount sign

Sum of children's P should equal negative of parent's E.
