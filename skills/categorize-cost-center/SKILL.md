---
name: categorize-cost-center
description: Use when someone asks to find business expenses in personal transactions, review cost center assignments, check for misclassified cost centers, or find items that belong to a different cost center. Also runs as the categorize skill's cost-center stage (writes suggestions to cc_suggestions only; never edits transactions).
argument-hint: [year]
---

# Cost Center Review

Scans Personal transactions for a given year and uses AI logic to identify
items that likely belong to a business cost center. Writes suggestions to
`cc_suggestions` table for review in the native app (Tools → Cost Center Review).

## Invocation

```
/categorize-cost-center 2023
"find business expenses in personal for 2024"
"check cost center assignments for 2023"
```

## Database

```python
import sqlite3
conn = sqlite3.connect('db/personaldb.sqlite')
conn.execute("PRAGMA foreign_keys=ON")
conn.row_factory = sqlite3.Row
```

### Tables

- `cc_suggestions` — where suggestions are written
- `transactions` — source data
- `companies` — cost center definitions

## Reference: Cost Center Profiles

Read `docs/tier-system.md` § Cost Centers before running. Key business profiles:

| Code | What it does | Signature items |
|---|---|---|
| TMCTECH | Consulting, software, crypto mining | AWS, GoDaddy, Microsoft, Shutterstock, Dropbox, Google Workspace, domain names, hosting, server hardware |
| IZUMA | Software company (C-Corp). Japan travel 1-4x/year | GitHub, ChatGPT, Google Workspace, Uber/taxi in Japan, hotels/restaurants during travel, Starbucks Japan |
| MAPTTW | eBay resale (formed 2024-06-10) | Goodwill, HomeGoods, thrift stores, shipping supplies, postage, packing materials, USPS, FedEx, storage units, eBay fees |
| TETECH | Same as TMCTECH but with a partner | Hardware, building materials, Harbor Freight |
| MAPT | REIT participation | AirDNA, PropStream, wire transfers for real estate |
| GRAVHL | Minimal LLC | Adalo platform, Cogency Global |
| REDRIVER | Expense report tag (Travis+Melissa) | Home improvement items shared between members |
| 7207 | Rental property | Mortgage (Mr. Cooper/Dovenmuehle), HOA (Western Oaks), USAA insurance, repairs, maintenance |
| 305 | Rental property | Mortgage (Midland), rent received (Gold Star Real E) |
| 1028 | Rental property | Mortgage (Wells Fargo), rent received (Harrison-Pearson checks) |
| 1913 | Primary residence (future rental) | Property tax, capital improvements, fence, electrical |
| 6505 | New house build | Lowe's Bastrop, building materials, plumbing, electrical |

## Workflow

### Step 1: Load context

1. Read `docs/tier-system.md` for cost center definitions
2. Query existing business patterns:
```sql
SELECT co.code, t.item, COUNT(*) AS n
FROM transactions t
JOIN companies co ON co.id = t.company_id
WHERE t.company_id > 1 AND t.is_plumbing = 0
GROUP BY co.code, UPPER(TRIM(t.item))
HAVING n >= 2
ORDER BY co.code, n DESC
```

### Step 2: Load Personal transactions for the year

```sql
SELECT t.id, t.date, t.item, t.price, t.site, t.account,
       COALESCE(c1.name,'') AS category,
       COALESCE(c2.name,'') AS sub_category,
       COALESCE(m.name,'') AS merchant
FROM transactions t
LEFT JOIN categories_tier1 c1 ON c1.id = t.tier1_id
LEFT JOIN categories_tier2 c2 ON c2.id = t.tier2_id
LEFT JOIN merchants m ON m.id = t.merchant_id
WHERE t.year = ? AND t.company_id = 1 AND t.is_plumbing = 0
ORDER BY t.date
```

### Step 3: AI classification

For each transaction, evaluate whether it belongs to a business cost center.

**Signal sources (in priority order):**

1. **Exact merchant match** — same merchant already tagged to a business CC elsewhere
2. **Keyword match** — item description contains business-indicator words:
   - TMCTECH: AWS, GODADDY, SHUTTERSTOCK, DROPBOX, domain, hosting, mining, crypto, server
   - IZUMA: GITHUB, Japan, Tokyo, Narita, JAL, ANA, Haneda, ChatGPT (if IZUMA has it tagged)
   - MAPTTW: GOODWILL, HOMEGOODS, USPS, FEDEX, shipping, packing, storage (after 2024-06-10 only)
   - Properties: mortgage company names, HOA names, property addresses
3. **Category signal** — Travel items during periods when IZUMA travel occurs
4. **Web search** — for unknown vendors, search to learn what they sell
5. **Amount signal** — large wire transfers might be MAPT/GRAVHL capital contributions

**Formation date enforcement:**
- MAPTTW: only suggest for dates >= 2024-06-10
- All others: no date restriction

### Step 4: Write suggestions

For each suspected business transaction:

```sql
INSERT INTO cc_suggestions (transaction_id, suggested_company_id, suggested_company_code,
                             confidence, reason, status)
VALUES (?, ?, ?, ?, ?, 'pending')
```

Clear previous pending suggestions for this year first:
```sql
DELETE FROM cc_suggestions WHERE status = 'pending'
  AND transaction_id IN (SELECT id FROM transactions WHERE year = ?)
```

### Step 5: Report

```
=== Cost Center Review: [year] ===
  Personal transactions scanned: N
  Suggestions generated: M
  By cost center:
    TMCTECH:  N
    IZUMA:    N
    MAPTTW:   N
    ...
  View in app: Tools → Cost Center Review
```

## Confidence Levels

| Level | Meaning |
|---|---|
| 0.95 | Exact merchant match (same item already tagged to this CC) |
| 0.85 | Strong keyword match (AWS, GODADDY, etc.) |
| 0.70 | Category/pattern match (Travel during IZUMA period) |
| 0.55 | AI inference (web search identified the vendor) |
| 0.40 | Weak signal (might be personal, might be business) |

## Guardrails

- **NEVER auto-apply** — write to cc_suggestions only, status='pending'
- **NEVER suggest a cost center before its formation date** (MAPTTW < 2024-06-10)
- **Skip plumbing rows** — they're transfer legs, not real purchases
- **Skip already-assigned rows** — only review company_id = 1 (Personal)
- Reference `docs/tier-system.md` for valid categories per cost center
- Process ALL Personal transactions for the year — don't sample

## Native App Tool

The app has **Tools → Cost Center Review** which reads from
`cc_suggestions` and displays them in a table with:
- Transaction details (date, item, price, current category)
- Suggested cost center + confidence + reason
- Approve / Reject buttons per row
- Approve applies the cost center change to the transaction
- Reject marks the suggestion as 'rejected'

This tool is built into the app separately — the skill just populates
the `cc_suggestions` table.
