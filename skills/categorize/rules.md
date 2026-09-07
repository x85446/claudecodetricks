# Categorizer rule book

Read at the start of every `/categorize` run. Deterministic rules first, then
heuristics, then learned rules the AI appends over time (Stage 4). Every learned
rule carries a date and evidence count so it can be challenged later.

## Hard rules (never violate)

- `human_verified = 1` rows are read-only. Flag suspected mistakes; never edit.
- Cost-center formation dates: no transaction may be assigned to a cost center
  before it existed. MAPT 2021-03-09, TMCTECH 2021-12-28, T&ETECH 2022,
  IZUMA 2022, GRAVHL 2022-03-31, MAPTTW 2024-06-10. Pre-formation → Personal.
  **TMCTECH exception (Travis, 2026-07-24): effective start is 2021-01-01** —
  pre-2021 → Personal; 2021+ recurring business-shaped entries (domain
  renewals, registrations) → TMCTECH even though the LLC registered 2021-12-28.
- Cost-center generalization ban: NEVER infer a merchant→cost-center rule from
  isolated charges at a consumer venue (one IZUMA McDonald's = a business trip,
  a row-level fact carried by card/ER/trip context — not a merchant fact).
  Merchant-level pinning exists as `merchants.pinned_company_id` and is set
  ONLY for merchants whose identity IS the cost center (GoDaddy-tmctech → 2,
  GoDaddy-izuma → 6). Pinned merchant → always that cost center.
- Vacation food tracking: during a tracked trip, food spending may be
  categorized under Travel (vacation) instead of Food — verified rows doing
  this are intentional (e.g. Bora Bora). Use the correctly-parented Travel
  tier3 for the trip; don't "fix" verified trip-food rows back to Food.
- Category tree must belong to the row's cost center (`categories_tier1.company_id`;
  properties share MAPT's tree).
- Tier1+2+3 is atomic — write all three together.
- Plumbing (CC payments to platforms, inter-account transfers) →
  `Finance / transfer-in|out / <target>` + `is_plumbing = 1`.
- **Card→tracked-site payments (Travis, 2026-07-25, hard — merchant defaults
  never override):** a credit-card transaction paying **paypal, ebay,
  homedepot, lowes, or godaddy** is `Finance / transfer-out / <site>` +
  `is_plumbing = 1` whenever the site's own imported data covers the
  transaction date — paypal, lowes, homedepot from **2021-01-01**; godaddy
  from **2003-01-01**; ebay from **2018-01-01**. Before the site's coverage
  start the card row is the only record and keeps its real category.
  Positive-price rows (refunds/credits FROM the site) are not payments —
  classify per their nature. `scripts/tracked_platforms.py` implements the
  patterns and coverage dates.
- **Bank→credit-card payments (same directive):** a bank transaction paying a
  credit card is `Finance / transfer-out / <credit-card-account>` +
  `is_plumbing = 1`, tier3 per the established names (`amex`, `chase-9878`,
  `citi`, …).

## Price-split rules

- **Gas station** (venue = gas station): `< $25` → Food / Snacks; `>= $25` →
  Transportation / Fuel. CC Personal unless card says otherwise.
  (`scripts/gas_or_food.py` implements this.)
- **Restaurant / fast food / food truck** (venue): `<= $25` → Food / Dining out,
  tier3 Lunch; `> $25` → tier3 Dinner. Overlap between lunch/dinner is acceptable —
  don't agonize near the boundary.

## Location heuristics

- Charges in **Bastrop, Kyle, Cedar Park, or Buda TX** → Melissa at work →
  restaurant/fast-food charges there are almost certainly **Lunch** regardless of amount.

## Merchant heuristics

- Brand over location: "EXXON BURPY MARKET AUSTIN TX" → merchant **Exxon**
  (the site name is noise). Same for SHELL/VALERO/CHEVRON co-branded stores,
  TST*/SQ* processor prefixes (the merchant is what follows the prefix).
- Generic-noun fallback: when a one-off descriptor resists identification, search
  the DB on fewer and fewer words ("TAQUERIA EL TROMPO MAYOR" → "TAQUERIA") and
  trust verified kin rows as precedent (a taqueria is a restaurant → lunch/dinner split).
- Dedicated ER card: Amex **2023** charges likely need an expense report.

## Learned rules (AI-appended; format: `- [YYYY-MM-DD, n=N] rule`)

<!-- Stage 4 appends here. Keep newest at the bottom. -->
- [2026-07-24, n=717] Venmo person-to-person rows ("PAID <name>", "RECEIVED FROM <name>") stay unmerchanted — people are not merchants. Same for outgoing personal checks ("Check #NNN").
- [2026-07-24, n=352] GoDaddy domain renewals route by domain owner: personal names → GoDaddy-personal, izumanetworks → GoDaddy-izuma, tmctech/framescontrols/framezcontrolz/russelsview → GoDaddy-tmctech. Category = that cost center's `- Software / - Domain Names` (business) or `Office & Software / Domain Names` (personal).
- [2026-07-24, n=8] MAPTTW eBay supply purchases split: packing machines/dispensers → `Shipping/handling / tools`; film & cushion rolls → `Shipping/handling / supplies`; refurb parts (belts, batteries) → `- Repair / parts`; test media → `- Repair / testing`; resale lots → `- Procurement / - Auctions`.
- [2026-07-24, n=2] ATM withdrawals get the bank as merchant (venue `financial`); the paired "FOREIGN TRANSACTION FEE - <bank>" row takes the same merchant.
- [2026-07-24, n=2] A refund row mirrors its purchase row's category (Zenni refund → Health / Eye care, matching the verified charge).
- [2026-07-24, n=1] `human_verified=1` with NULL tier1 is a verify-toggle oversight, not a decision. In an INTERACTIVE session, fill from the row's own verified precedent (same item, other years) and A-flag it. In an autonomous/background run, the verified-rows-read-only guardrail wins: flag only, list in the report.
- [2026-07-24, n=10] Employer/city PPD PAYROLL ACH credits (e.g. "CITY OF KYLE PPDPAYROLL", "IZUMA NETWORKS PAYROLL") are income → Personal `Finance / Paycheck` (tier3 = the job when a matching one exists, e.g. Izuma). The paired "AP … Per-Diem" line is a `Finance / reimbursement`, not a paycheck.
- [2026-07-24, n=16] Named life-insurance carriers (AFBA, Protective/PLIC-SBD, Unum) → `Debt & Insurance / Insurance / life`. Auto carriers like Progressive (Prog County Mut) → `Transportation / Car Insurance`, NOT Debt & Insurance.
- [2026-07-24, n=8] ATM refinement: only ATM rows whose description NAMES a bank ("capital one,…", "ufcu,…", "a+ federal cred,…") get that bank as merchant; bare-street-address ATM rows ("6600 S Mo Pac Expy", "4970 Hwy 290 West") have no identifiable bank and stay unmerchanted (they are already verified `Finance / transfer-out`).
- [2026-07-24, n=40] The gas-station <$25 → Food/Snacks split is a DEFAULT for *uncategorized* rows only. Do NOT flag verified sub-$25 gas rows tagged `Transportation / Fuel` as mistakes — small fuel fill-ups ($8-$24) are legitimately Fuel; verified human judgment overrides the price heuristic.
- [2026-07-24, caution] `merchant_hunter.py tag` with a broad LIKE pattern re-tags rows that already have a *different* merchant (retag_others). A generic catch-all pattern (e.g. "%.com domain renewal%") will STEAL rows from more-specific merchants tagged earlier — always tag specific/business patterns AFTER any catch-all, or re-run the specific ones last to reclaim.
- [2026-07-25, n=220] Any credit-card/bank payment TO GoDaddy (descriptor names GODADDY on amex/chase/paypal/usaa/etrade) is plumbing: `Finance / transfer-out / godaddy` (dash-prefixed tree for non-Personal cost centers), `is_plumbing=1` — never a real expense; the GoDaddy line items are the canonical records. Positive-price rows (refunds/credits FROM GoDaddy) are NOT payments — classify per their nature. Directed by Travis.
- [2026-07-25, directed] Card rows paying a platform get the PLATFORM as merchant: `PAYPAL *X` on amex/chase is a transfer TO PayPal → merchant = PayPal, never X (Artome, GoDaddy, Quantech…). The `*X` suffix is only the link hint; the end-vendor merchant belongs on the paypal-side payout row where the real category lives. (Exception: rows tracked_platforms classifies to another site, e.g. PAYPAL *EBAY → ebay conventions.) Same principle for venmo.
- [2026-07-25, n=17] USAA "ACH WITHDRAWAL … PAYPAL INST XFER" (and "PAYPAL PURCHASE") rows are always PB `Finance / transfer-out / paypal`, merchant PayPal Instant Transfer. The bank leg posts 1–3 days AFTER the paypal-side transaction; link it into the paypal row's EXISTING link_group (which already ties GoDaddy chains to godaddy line items). When two same-amount candidates tie, pair FIFO — bank ACH posts preserve initiation order.
- [2026-07-25, directed] Merchant PayPal-internal ⇒ category `Finance / transfer-out / paypal-internal` (tier3 425 Personal; `- paypal-internal` tier3 426 in the MAPT dash tree) and item = the paypal statement's own description ("General Credit Card Deposit", "Bank Deposit to PP Account", "Ref ID: …"), NEVER the funded purchase's product title — the product text lives only on the real payout leg. Applied retroactively to all 78 PayPal-internal rows.
- [2026-07-26, directed] `transfer-out / cash` is NEVER PB — cash is real untracked outflow (321 rows, deliberate). Citi 2021+ = `Debt & Insurance / Credit card payments` non-PB (61 rows fixed); pre-2021 citi rows stay `transfer-out/citi` legacy. A card leg LINKED to site line items must be PB — linked-but-not-PB double-counts (19 verified lowes rows fixed).
