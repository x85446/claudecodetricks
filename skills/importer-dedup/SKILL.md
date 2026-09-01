---
name: importer-dedup
description: Find and resolve duplicate transactions across sources. Runs as the importer skill's post-import dedup stage. Use when someone says 'find duplicates', 'duplicate detection', 'dedup', or 'reconcile transactions'.
argument-hint: [scan|resolve]
---

# Duplicate Transaction Detector

Finds and resolves duplicate transactions in the personaldb finance database.

## Database

- Path: `db/personaldb.sqlite`
- Use `python3` for all DB operations (sqlite3 CLI may not be available)

## Two Separate Problems

This skill handles two distinct issues that must NOT be conflated:

### Problem 1: CC → Platform Plumbing (is_plumbing = 1)

When you buy something on a tracked platform (Amazon, eBay, etc.) and pay with a credit card, both the CC statement AND the platform's order history record the purchase. The CC charge is just the money movement — the platform record has the real product detail. Mark the CC leg as `is_plumbing = 1`.

**`is_plumbing` means: "this transaction is a financial transfer, not a real purchase."** It is ONLY used for CC→platform transfer legs. Never for anything else.

### Problem 2: Re-import Duplicates (DELETE the duplicate row)

When the same transaction appears twice from the same source (e.g., PDF import + CSV import of the same Amex statement), the duplicate row should be **deleted**. These are data errors, not plumbing.

For Amex specifically, use the `reference` field in `src_amex` to identify exact duplicates — same reference = same transaction.

## Tracked Platforms (Plumbing Candidates)

Only these platforms create CC→platform duplicate patterns. This is an exhaustive list:

| Platform | Source Table | CC Description Patterns |
|----------|-------------|------------------------|
| Amazon | src_amazon | `AMAZON.COM`, `AMZN.COM`, `AMAZON MARKETPLACE`, `AMAZON DIGITAL`, `AMZN MKTP` |
| eBay | src_ebay | `EBAY O*`, `EBAY INC` |
| Lowe's | src_lowes | `LOWE'S`, `LOWES`, `GglPay LOWE'S` |
| Home Depot | src_homedepot | `HOME DEPOT`, `HOMEDEPOT` |
| GoDaddy | src_godaddy | `GODADDY.COM`, `GODADDY` |
| PayPal | src_paypal | `PAYPAL *`, `PAYPAL,` |

### CC/Bank Sources (financial statement records)

| Source | Table | Role |
|--------|-------|------|
| Amex | src_amex | Credit card statement |
| Chase | src_chase | Credit card statement |
| E*Trade | src_etrade | Bank statement |
| Mint | src_mint | Aggregated bank data |
| Venmo | src_venmo | Payment platform |

## Plumbing Resolution Rules

### Rule 1: CC → Tracked Platform Match

When a CC charge matches a tracked platform record (same date ± 1 day, same amount ± $0.02):
- The **CC record** → `is_plumbing = 1` (it's just the payment leg)
- The **platform record** → keeps the real product category (canonical)

The platform record has item-level detail (ASIN, SKU, item name, qty) that the CC record doesn't.

**Examples:**
```
Amex: "EBAY O*20-11389-8424" $23.77     → is_plumbing=1
eBay: "3pcs Bent Nozzles for ATTEN"      → Tools & Hardware (canonical)

Amex: "AMAZON.COM AMZN.COM/BILL" $29.99  → is_plumbing=1
Amazon: "USB C Cable 10ft 2Pack" $29.99   → Electronics (canonical)

Chase: "LOWES #2133" $150.30              → is_plumbing=1
Lowes: "PowerMark Gold 200-Amp Panel"     → Building / Electrical (canonical)

Amex: "HOME DEPOT 6570" $87.42           → is_plumbing=1
Home Depot: "1 in. x 48 in. Insulating"  → Building / Insulation (canonical)
```

### Rule 2: 3-Way PayPal Pattern (older years, pre-2020)

When PayPal was the payment method between CC and merchant:
- **CC** → "PAYPAL *EBAY" → `is_plumbing = 1`
- **PayPal** → "eBay purchase" → `is_plumbing = 1`
- **eBay** → "Actual product name" → real category (canonical)

All intermediary legs become plumbing. Only the merchant endpoint (eBay, Amazon) keeps the category.

### Rule 3: No Platform Match = Direct Purchase

If a CC charge does NOT match any tracked platform record, it's a direct purchase. **Leave it alone.** Gas stations, restaurants, subscriptions, GoDaddy, AWS — these are all canonical as-is.

## Re-import Duplicate Resolution

### Credit Card Re-imports (MANDATORY: use reference field)

Every credit card transaction has a unique reference/transaction ID assigned by the card issuer. This is the **primary and mandatory** dedup key for all CC sources. Date+amount matching is NOT sufficient — the reference field is authoritative.

| CC Source | Reference Field | Location |
|-----------|----------------|----------|
| Amex | `reference` | `src_amex.reference` — 18-digit Amex transaction ID |
| Chase | `reference` | `src_chase.reference` — Chase transaction reference |

**The reference field MUST be populated for every CC transaction.** If a row is missing its reference, that's an import bug to fix — not a reason to fall back to fuzzy matching.

**Finding duplicates by reference:**

```sql
-- Find CC rows with duplicate references (same reference = same transaction)
SELECT s1.transaction_id as keep_id, s2.transaction_id as delete_id, 
       s1.reference, t1.date, t1.price
FROM src_amex s1
JOIN src_amex s2 ON s1.reference = s2.reference AND s1.id < s2.id
JOIN transactions t1 ON s1.transaction_id = t1.id
JOIN transactions t2 ON s2.transaction_id = t2.id
WHERE s1.reference IS NOT NULL AND s1.reference != ''
  AND t1.is_plumbing IS NOT 1 AND t2.is_plumbing IS NOT 1;
```

**Resolution**: Keep the row with richer `src_amex` data (has `extended_details`, `address`, `card_member`, etc.). DELETE the other row from both `transactions` and its `src_*` table.

**When reference is missing:** If a CC import produced rows without references (e.g., a PDF parser that didn't extract them), the importer should be fixed to extract references. Do NOT silently fall back to fuzzy matching. Flag these to the user as import quality issues.

### Non-CC Re-imports (fuzzy matching)

For non-CC sources that don't have institution-assigned reference IDs, match on: same site + same date + same amount (± $0.02) + same description prefix (first 15 chars).

```sql
SELECT t1.id as keep_id, t2.id as delete_id, t1.date, t1.price, t1.item
FROM transactions t1
JOIN transactions t2 ON t1.date = t2.date 
    AND ABS(t1.price - t2.price) < 0.02
    AND t1.site = t2.site
    AND t1.id < t2.id
    AND t1.is_plumbing IS NOT 1 AND t2.is_plumbing IS NOT 1
    AND SUBSTR(UPPER(t1.item), 1, 15) = SUBSTR(UPPER(t2.item), 1, 15)
WHERE t1.site = '<site>';
```

**Resolution**: Keep the row with the lower ID (earlier import, likely richer). DELETE the duplicate from both `transactions` and its `src_*` table.

## Detection Script

```bash
# Full candidate list
python3 scripts/find_duplicates.py

# Summary stats only
python3 scripts/find_duplicates.py --report

# Filter by year
python3 scripts/find_duplicates.py --year 2024

# JSON output
python3 scripts/find_duplicates.py --json
```

## Commands

### `scan` (default)

Run detection and review results. Group by type:

1. **CC → Platform matches** (Rule 1) — show the CC charge and its matching platform record side by side. Verify the match makes sense (same date, same amount, CC description mentions the platform).

2. **3-way PayPal chains** (Rule 2) — identify the full chain and show all legs.

3. **Re-import duplicates** — same-source rows with matching reference or date+amount+description. Show which row is richer.

4. **Ambiguous** — same date+amount across different sources with no clear platform relationship. Present for human decision.

### `resolve`

Apply resolutions:

1. **CC → Platform**: `UPDATE transactions SET is_plumbing = 1, link_group = <platform_txn_id>, notes = COALESCE(notes || '; ', '') || 'Plumbing: CC payment for <platform> txn #<platform_id>' WHERE id = <cc_id>` — also set `link_group = <platform_txn_id>` on the platform record itself.
2. **PayPal chains**: mark both CC and PayPal legs as `is_plumbing = 1`, set `link_group` on all legs to the canonical (merchant endpoint) transaction ID
3. **Re-imports**: DELETE the duplicate row from `transactions` and its `src_*` table (keep the richer one)
4. **Ambiguous**: ask the user

After resolving, report:
- How many CC charges reclassified as plumbing
- How many re-import duplicates deleted
- Estimated spending impact (how much was double-counted)

## Important Rules

- **`is_plumbing = 1`** is ONLY for CC→platform transfer legs and PayPal intermediary legs
- **NEVER use `is_plumbing` for re-import duplicates** — DELETE those instead
- **NEVER modify source tables** when marking plumbing (only update `transactions`)
- **DO delete from source tables** when removing re-import duplicates (delete from both `transactions` and `src_*`)
- A direct CC purchase (no matching platform record) is NOT a duplicate and NOT plumbing
- Tracked platforms are ONLY: Amazon, eBay, Lowe's, Home Depot
- When in doubt, FLAG for human review rather than auto-resolving
- Run `python3 scripts/find_duplicates.py --report` after resolving to verify counts dropped
