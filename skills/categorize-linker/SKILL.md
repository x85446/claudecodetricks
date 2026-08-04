---
name: categorize-linker
description: Use when someone asks to build links, find links, link transactions, resolve chain-of-custody orphans, fix broken link_groups, connect CC charges to purchases, or link paypal/venmo chains. Child of the categorize meta skill.
argument-hint: [year] [source]
---

# Categorize-Linker — find and build transaction links

Child skill of the **categorize** meta skill. Resolves chain-of-custody links by
connecting PB (plumbing) charges to the real purchases they funded, so every
dollar is reported in exactly ONE real category.

## The chain model (why links exist)

A purchase routed through a payment platform produces MULTIPLE transaction rows
but only ONE real expense. `is_plumbing` (PB) + `Finance / transfer-out/<site>`
mask the pass-through legs from spending reports; `link_group` ties the legs
together so ⌘L ("Show Linked Transactions") walks the chain.

**PB invariant: every `Finance / transfer-out / <site>` row must have
`is_plumbing = 1`.** A transfer-out row without PB double-counts spending. The
reverse holds too — a PB row should carry a transfer/fee category, never a real
one. Enforce both directions on every run.

**3-leg chains (paypal, venmo — platforms that act like a secondary bank
account):**

```
amex  "PAYPAL *TEACHERSPAYTEA"  -$1.63   PB  Finance/transfer-out/paypal   ─┐
paypal "transfer from a to b"    ±$1.63   PB  (internal shuffle)            ─┼─ one link_group
paypal "TeachersPayTeachers NY"  -$1.63       Education/School supplies     ─┘  ← the REAL spend
```

Only the last leg carries the real category; the $1.63 is reported once.

**2-leg chains (lowes, homedepot, godaddy, amazon, ebay — sites whose line
items we import directly):**

```
amex  "LOWE'S AUSTIN TX"  -$84.12  PB  Finance/transfer-out/lowes  ─┐
lowes  <line items for the order>       real categories             ─┘  one link_group
```

**4-leg "quad" chains (ebay paid THROUGH paypal — card descriptor `PAYPAL *EBAY`):**
every leg is a transfer except the last; categorization happens on the ebay
line items (ebay → seller):

```
amex   "PAYPAL *EBAY US"                          -$27.88  PB  Finance/transfer-out/ebay    merchant PayPal          ─┐
paypal "General Credit Card Deposit"              +$27.88  PB  Finance/transfer-out/paypal-internal  merchant PayPal-internal ─┼─ one
paypal "Express Checkout Payment: eBay Commerce"  -$27.88  PB  Finance/transfer-out/ebay    merchant eBay            ─┤  link_group
ebay   <line items for the order>                              real categories (the seller/item decides)             ─┘
```

The paypal statement ties the two middle legs via `Ref ID` (the deposit's
`reference_txn_id` = the payout's `pp_transaction_id`). Match the ebay side on
`src_ebay.order_total` (the charged amount), not `item_price` — coupons make
them differ. A card `PAYPAL *EBAY` row with no paypal-side legs means that
month's paypal statement mis-imported (see the wrapped-date warning in the
importer skill) — recover the statement rows, don't fall back to a 2-leg link.

**The `*<suffix>` in a card descriptor ("PAYPAL *QUANTECHDAT") names the END
merchant — it is the LINK HINT for finding the paypal-side payout row, NOT the
merchant of the card row.** See the merchant rule below.

## Merchant rule (card legs belong to the platform)

A card row paying a platform has merchant = the PLATFORM (PayPal), not the end
vendor. Amex/chase pay PayPal; PayPal later pays Artome/GoDaddy/Quantech — the
end-vendor merchant belongs on the paypal-side payout row, where the real
category lives. When this skill (or the categorize) finds a card
`PAYPAL *X` row carrying merchant X, re-point it to the PayPal merchant and use
X only to locate/label the payout leg. Same principle for venmo.

**Only two paypal-platform merchants exist (2021+): `PayPal` and
`PayPal-internal`.** `PayPal` goes on card/bank legs paying the platform
(`PAYPAL *X`, `PAYPAL INST XFER`, `PAYPAL PURCHASE`). `PayPal-internal` goes on
paypal-site internal legs: `Ref ID:` rows, General Card/Credit Card Deposits,
Bank Deposit to PP Account, currency conversions, and funding credits (the
`+$X` twin of a same-amount payout). Payout legs to a tracked site carry that
site's merchant (eBay, GoDaddy); payout legs to an end vendor carry the vendor.
Never create `PayPal *Vendor` or `PayPal Instant Transfer`-style merchants.
**Merchant `PayPal-internal` IMPLIES category `Finance / transfer-out /
paypal-internal`** (tier3 425; `- paypal-internal` 426 in the dash tree) and an
item equal to the statement's own description ("General Credit Card Deposit"),
never the funded purchase's product title.

## Database

```
db/personaldb.sqlite
```

Use python3 for all DB operations.

## Invocation

```
/categorize-linker 2023              # Link all orphans for 2023
/categorize-linker 2023 amazon       # Link amex→amazon orphans only
/categorize-linker 2023 venmo        # Link etrade→venmo orphans only
/categorize-linker status 2023       # Show health report without making changes
/categorize-linker all               # Link all orphans across all years
```

## Pass 0: Exception Corrections (run first, every time)

Before linking, fix known mis-classifications:

### AMZNPHARMA — NOT plumbing
AMZNPHARMA (Amazon Pharmacy) charges are **direct purchases**, not pass-through plumbing. They appear on amex as "AMZNPHARMA AMZN.COM/BILL WA". Fix:
```python
# Unmark PB, set category to Health / Rx
UPDATE transactions SET is_plumbing = 0, tier1_id = <Health>, tier2_id = <Rx>
WHERE item LIKE '%AMZNPHARMA%' AND is_plumbing = 1
```

### Citi payments — NOT plumbing
Citi credit card payments from etrade are real bill payments, not internal transfers. We don't import Citi statements so there's no matching side. Fix:
```python
# Unmark PB, set category to Debt & Insurance / Credit card payments
UPDATE transactions SET is_plumbing = 0, tier1_id = <Debt & Insurance>, tier2_id = <Credit card payments>, tier3_id = NULL
WHERE tier3_id = (SELECT id FROM categories_tier3 WHERE name = 'citi')
  AND is_plumbing = 1
```

### Audible — NOT plumbing
Audible subscriptions are direct purchases. Fix:
```python
UPDATE transactions SET is_plumbing = 0
WHERE (item LIKE '%AUDIBLE%' OR item LIKE '%Audible%') AND is_plumbing = 1
```

### PB invariant enforcement
Every `Finance / transfer-out / <site>` row must be PB; every PB row must
carry a transfer/fee category. **Exemptions (deliberate, never "fix"):**
- `transfer-out / cash` (and `izuma-cash`): NEVER PB — cash leaves tracked
  systems, it is real untracked outflow. ATM withdrawals stay visible.
- `transfer-out / citi` pre-2021: legacy rows, leave as-is. 2021+ citi
  payments are `Debt & Insurance / Credit card payments` (non-PB) — we don't
  import citi, so they're real bill payments (fixed 2026-07-26).
- Card refund/credit rows (+price) mirror their purchase's real category
  (net-zero policy), non-PB — they are not transfer-out violations.
```python
# transfer-out without PB -> set is_plumbing = 1  (EXCLUDING the exemptions above)
UPDATE transactions SET is_plumbing = 1, updated_at = datetime('now')
WHERE tier2_id IN (SELECT id FROM categories_tier2
                   WHERE REPLACE(name,'- ','') = 'transfer-out')
  AND tier3_id NOT IN (SELECT id FROM categories_tier3
                       WHERE REPLACE(name,'- ','') IN ('cash','izuma-cash','citi'))
  AND price < 0
  AND is_plumbing = 0
# PB rows with a real (non-Finance) category -> list for review, don't guess
```
A row that is LINKED to site line items but not PB double-counts — always PB
the card leg once it joins a chain (the 2024-25 verified GglPay LOWE'S rows
were exactly this bug).

### Merchant rule sweep
Card rows `PAYPAL *X` (any card site) with merchant != PayPal → re-point
merchant to PayPal (record X as the link hint; the end-vendor merchant belongs
on the paypal payout leg). This includes `PAYPAL *EBAY`: the card leg is still
a transfer TO paypal (merchant PayPal) — only its CATEGORY follows the ebay
convention (`Finance/transfer-out/ebay`), and it links as a 4-leg quad chain
(see chain model). Also sweep paypal-site internal legs (Ref ID, deposits,
funding credits, conversions) onto `PayPal-internal`.

### Amazon Music — NOT plumbing
Amazon Music charges are direct purchases. Fix:
```python
UPDATE transactions SET is_plumbing = 0
WHERE item LIKE '%AMAZON MUSIC%' AND is_plumbing = 1
```

## Execution Order

Run passes in this order — each pass resolves dependencies for the next:

0. **Exception corrections** — fix AMZNPHARMA, Citi, Audible, Amazon Music; enforce the PB invariant; apply the merchant rule
1. **Amazon order grouping** — group all amazon items by order_id
2. **Amazon Payments PDF linking** — use card+order data to link CC charges
3. **Amazon gift card linking** — link gift card transactions to orders
4. **Business card marking** — mark BC on business-card-paid orders
5. **Venmo linking** — match etrade→venmo with venmo transactions
6. **eBay linking** — match chase/amex→ebay with ebay purchases
7. **E*Trade inter-account transfers** — match self-transfers
8. **E*Trade CC payments** — match etrade→amex/chase with CC transfer-in
9. **PayPal linking** — match chase→paypal and internal PayPal pairs
10. **General fallback** — date+amount matching for remaining orphans

## Pass 1: Amazon Order Grouping

Group all `src_amazon` items by `order_id` under a shared `link_group`:

```python
# For each unique order_id, set link_group = MIN(transaction_id) across all items
orders = query("SELECT order_id, GROUP_CONCAT(transaction_id) FROM src_amazon GROUP BY order_id")
for order_id, txn_ids in orders:
    existing_lg = first link_group from any item in this order
    lg = existing_lg or min(txn_ids)
    UPDATE all items to link_group = lg
```

## Pass 2: Amazon Payments PDF Linking

Use `src_amazon_payments` to connect CC charges to amazon orders.

**Card-to-site mapping:**
- 1009, 2007, 2023, 6016 → site = 'amex'
- 9878 → site = 'chase'

**For each payment entry with a personal card and order_id:**
1. Get the order's `link_group` (from Pass 1)
2. Find the matching CC PB charge:
   ```sql
   SELECT t.id FROM transactions t
   LEFT JOIN categories_tier3 c3 ON c3.id = t.tier3_id
   WHERE t.site = '<cc_site>' AND t.is_plumbing = 1
     AND (c3.name = 'amazon' OR t.item LIKE '%AMAZON%' OR t.item LIKE '%AMZN%')
     AND ABS(julianday(t.date) - julianday('<date>')) <= 7
     AND ABS(ABS(t.price) - ABS(<amount>)) < 0.10
   ORDER BY ABS(ABS(t.price) - ABS(<amount>)), ABS(julianday(t.date) - julianday('<date>'))
   LIMIT 1
   ```
3. Set the CC PB charge's `link_group` to the order's `link_group`

## Pass 3: Amazon Gift Card Linking

Link gift card transactions to their orders:
```python
# Gift cards have order_id embedded in item text: "Gift Card applied to Amazon.com order 112-XXXXXXX-XXXXXXX"
# Extract order_id via regex, look up the order's link_group, set it
```

## Pass 4: Business Card Marking

For `src_amazon_payments` entries on BC cards (2999, 8574, 6979, 5055, 6388):
```python
BC_CARDS = {'2999':'2999-IZUMA', '8574':'8574-IZUMA', '6979':'6979-IZUMA',
            '5055':'5055-GRAVHL', '6388':'6388-REDRIVER'}
# Find all amazon items for the order, set business_card on each
```

## Pass 5: Venmo Linking

**Critical: Venmo has many legitimate duplicate amounts.** Three $70 charges on the same day are three real different payments to different people. You CANNOT match solely on date+amount.

**Matching strategy — use consumption tracking:**

```python
# 1. Get all unlinked etrade PB charges with dest=venmo
etrade_venmo = query("""
    SELECT t.id, t.date, t.price FROM transactions t
    LEFT JOIN categories_tier3 c3 ON c3.id = t.tier3_id
    WHERE t.site = 'etrade' AND t.is_plumbing = 1
      AND c3.name = 'venmo'
      AND (t.link_group IS NULL OR t.link_group = 0)
    ORDER BY t.date
""")

# 2. Get all unlinked venmo transactions
venmo_pool = query("""
    SELECT t.id, t.date, t.price, t.item FROM transactions t
    WHERE t.site = 'venmo' AND t.is_plumbing = 0
      AND (t.link_group IS NULL OR t.link_group = 0)
    ORDER BY t.date
""")

# 3. Build a consumable pool (each venmo txn can only match ONCE)
available = set(venmo_pool ids)

# 4. For each etrade PB, find the CLOSEST unmatched venmo by date then amount
for et_id, et_date, et_price in etrade_venmo:
    best_match = None
    best_score = 999
    for v_id, v_date, v_price, v_item in venmo_pool:
        if v_id not in available: continue
        if abs(abs(v_price) - abs(et_price)) > 0.10: continue
        date_diff = abs(julianday(et_date) - julianday(v_date))
        if date_diff > 2: continue  # ±2 days max
        score = date_diff  # prefer closest date
        if score < best_score:
            best_match = v_id
            best_score = score
    if best_match:
        link(et_id, best_match)
        available.remove(best_match)
```

**Key rule: consume matches.** Once a venmo transaction is linked to an etrade PB, remove it from the pool. This prevents three $70 etrade charges from all matching the same venmo $70.

**Date tolerance:** ±2 days. Etrade posts transfers 1-2 days after the venmo transaction.

## Pass 6: eBay Linking

```python
# Match chase/amex PB dest=ebay against src_ebay by date ±3 days + amount
# eBay has unique item_ids so duplicates are rare — simple matching works
# Match on src_ebay.order_total (the charged amount), NOT item_price —
# coupons/discounts make them differ.
```

**Tax rule for card→order matching (lowes/homedepot/amazon — pre-tax line
items):** never assume flat 8.25%. Groceries/drinks are tax-exempt in TX, so
mixed orders tax at an intermediate effective rate (H6542-714983: $549.84
pre-tax → $594.83 charged ≠ ×1.0825). Match in this order: (1) exact grand
total = sum(pre_tax) + sum(tax_amount) ±0.02 when tax columns exist, (2) the
band `sum(pre_tax) ≤ charge ≤ sum(pre_tax)×1.0825 + 0.15`, unique candidate
only.

**Card descriptor `PAYPAL *EBAY` → this is a QUAD chain, not a 2-leg:** the
payment routed through paypal, so the paypal statement holds two more legs
(deposit + Express Checkout payout to "eBay Commerce Inc."). Link all four in
Pass 9's quad handling; if the paypal legs are absent, recover the statement
before linking (wrapped-date importer bug).

## Pass 7: E*Trade Inter-Account Transfers

Match pairs within etrade:
```python
# "TRANSFER MONEY TO BANK XXXXXX1460" (-$500) on account 1452
# "TRANSFER MONEY FROM BANK XXXXXX1452" (+$500) on account 1460
# Match by: same date ±1 day, same abs(amount), opposite signs
# Consume matches (same pool logic as venmo)
```

Also match etrade↔etrade between different account suffixes (1452, 1460, 0939).

## Pass 8: E*Trade CC Payments

Match etrade outbound CC payments to the receiving side:
```python
# etrade: "AMEX EPAYMENT ACH PMT" -$14,867 (dest=amex)
# amex: "AUTOPAY PAYMENT - THANK YOU" +$14,867 (transfer-in)
# Match by: date ±5 days, amount ±$5 tolerance (payments can have slight fee differences)
```

**Card mapping:**
- dest `amex` → match on site='amex', transfer-in
- dest `chase-9878` → match on site='chase', transfer-in
- dest `citi` → NO MATCH (we don't import citi). These are `Debt & Insurance / Credit Card Payments`, not PB.

## Pass 9: PayPal Linking (3-leg and quad chains)

PayPal is a secondary bank account: card leg → internal paypal transfer(s) →
payout to the end merchant. Build the full chain under one link_group.

```python
# 1. Card legs: amex/chase PB rows, tier3 paypal (or PAYPAL *X descriptor)
# 2. For each, extract the *SUFFIX hint (e.g. QUANTECHDAT) from the item
# 3. Find paypal-side rows within ±4 days at the same abs(amount):
#    - internal legs (deposits, Ref ID rows): PB, merchant PayPal-internal,
#      category Finance/transfer-out/paypal-internal, item = statement's own
#      description; group by pp_transaction_id / reference_txn_id when present
#    - the payout row whose description matches the suffix hint -> this is the
#      REAL leg: keep/assign its real category and set merchant = end vendor
# 4. link_group = MIN(id) across all legs; consume matches
# 5. Card leg keeps merchant PayPal (merchant rule above)
```

**Quad variant — the payout targets a tracked site (suffix `EBAY`, paypal
description "Express Checkout Payment: eBay Commerce Inc."):** the payout leg
is NOT the real leg. It stays PB (`Finance/transfer-out/ebay`, merchant eBay)
and the chain extends one more hop to the site's line items, which carry the
real categories (ebay → seller). Match the ebay side on `src_ebay.order_total`
= the charged amount. All four legs share one link_group; the same applies if
another tracked platform ever shows up as a paypal payout target.

**When the paypal side has NO rows for a card leg** (e.g. 2026-03-11 amex
"PAYPAL *QUANTECHDAT" −$9.95, ⌘L shows "No Linked Transactions"): the paypal
activity export for that period is missing, mis-imported (wrapped-date bug —
a statement with activity that imported 0 rows), or the row predates coverage.
Do NOT invent a leg. Check the statement PDF for the period first and recover
missed rows; otherwise report `missing paypal-side data` with date ranges; the
card row stays PB + transfer-out/paypal awaiting its chain.

## Pass 10: General Fallback

For any remaining unlinked PB transactions:
```python
# Match against ANY transaction on the destination site
# by date ±3 days and amount ±$0.10
# Use consumption pool to avoid double-matching
# Only link if exactly 1 match — skip ambiguous
```

## Health Report Format

```
=== 2023 Chain Health ===

Source-side (PB charges):
  amex:    225/233 linked (8 orphaned)
  chase:    14/14  linked (0 orphaned) ✓
  etrade:   90/168 linked (78 orphaned)
  paypal:   12/24  linked (12 orphaned)

Destination-side (real purchases):
  amazon:  305/305 linked (0 unlinked) ✓
  ebay:     15/15  linked (0 unlinked) ✓
  venmo:   400/450 linked (50 unlinked)

Business cards: 32 transactions marked

Overall: 84% PB resolved
```

## Guardrails

- **ALWAYS set link_group** — every linked pair MUST have a shared `link_group`. Use `MIN(txn_id_a, txn_id_b)` as the value. Without link_group, the Link Tool can't display the relationship.
- **Never delete transactions** — only set `link_group` or `business_card`
- **Never overwrite existing link_groups** — only fill NULL/0 ones
- **Consume matches** — each transaction can only be linked once per pass. Use a pool set.
- **Skip ambiguous** — if multiple equally-good matches exist, skip and log
- **Log every link** — print `Linked: etrade#8396 ↔ venmo#44236 ($2700, 1 day apart)`
- **Business card only for known BC cards**: 2999-IZUMA, 8574-IZUMA, 6979-IZUMA, 5055-GRAVHL, 6388-REDRIVER
- **Citi payments are NOT PB** — they're `Debt & Insurance / Credit Card Payments`. If you find citi dest, recategorize.
- **AMZNPHARMA and Audible are NOT PB** — they're normal CC purchases. If found marked PB, unmark.
