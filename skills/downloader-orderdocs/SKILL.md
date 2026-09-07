---
name: downloader-orderdocs
description: Use when the Document Coverage bottom grid shows missing per-order transaction documents (source_documents status='missing') for ebay, amazon, homedepot, or lowes — fetches each order's document page from the signed-in site session and stages it under the tracker's filename convention. Child of the downloader meta.
argument-hint: [ebay|amazon|homedepot|lowes|status]
---

# Downloader: per-order documents

Child of the **downloader** meta. Closes `source_documents` gaps: one document
file per order/receipt ref, so the Document Coverage bottom grid (site × year)
reaches have-coverage. Statements are the meta's other children's job — this
skill is ONLY per-order docs.

## The document per site

| Site | Document | Fetch | Filename staged |
|---|---|---|---|
| ebay | Order-details page | same-origin fetch of `https://order.ebay.com/ord/show?orderid=<ref>` from a tab ON order.ebay.com (cross-subdomain fetch from www.ebay.com is CORS-blocked) | `YYMMDD-ebay-order-<ref>.html` |
| amazon | Print invoice | `/gp/css/summary/print.html?orderID=<ref>` (the unified userscript's "Download Amazon Invoices" button does this sweep) | `YYMMDD-amazon-invoice-<ref>.html` |
| homedepot | Order-details page | signed-in homedepot.com: open Purchase History (`/order/view/purchase-history`), each order row links to its details page; fetch same-origin per online order number (`W…`/`WA…`/`WM…` refs). Verify the body contains the ref. First live run: capture the detail-URL shape from a row's href before batching. | `YYMMDD-homedepot-order-<ref>.html` → pdf |
| lowes | MyLowe's purchase detail | signed-in lowes.com `/mylowes/orders?orderType=INSTOREBACKROOM&page=N&show=all` (and the online-orders type) — **the list is URL-paginated and the first page shows only a handful of rows; NEVER trust a single page's count as the account total** (2026-07-30: page 1 showed "5 transactions ever", page 14 held a 2023 order). Walk `page=1..N` until an empty page; site states order history is viewable for orders placed after 2021 (older refs are genuinely offline). Match refs (`L<store>-<yymmdd>-<time>` / long date-prefixed keys) by date+store+amount, open View Details, save rendered DOM. | `YYMMDD-lowes-receipt-<ref>.html` → pdf |
| godaddy | (complete) | all 271 receipt refs live inside the order CSVs (`source_documents` container rows) — nothing to fetch. `FN_GD_RCPT` (`YYMMDD-godaddy-receipt-<ref>`) exists for future one-off receipt-page saves. | — |

`YYMMDD` = the order date (2-digit year). `build_doc_catalog.py` recognizes
`FN_AMZ_INV`, `FN_EBAY_ORD`, `FN_HD_ORD`, `FN_LOWES_ORD`, `FN_GD_RCPT`; add a
matching regex there BEFORE staging a new site's files or they will not
register.

**Format rule: documents are csv/xlsx/pdf.** HTML captures are an
intermediate — after staging, convert with
`python3 scripts/html_to_pdf.py --delete-html <files>` (ref-verified via
pdftotext) so only the PDF remains. The FN patterns accept both extensions, so
tracker counts survive the swap.

## Steps

1. **Worklist from the tracker, never from the site**:
   `SELECT doc_ref, doc_date FROM source_documents WHERE site=? AND status='missing' AND doc_ref!='--'`.
   Split multi-account sites by account (ebay: personal vs eBay-MAPTTW — the
   signed-in account only serves its own orders; wrong-account fetches fail or
   mislabel. Verify whose session is live before fetching, venmo-guard style).
2. **Human-gated login** — Travis signs in; Chrome needs "allow multiple
   downloads" for the fetch origin (order.ebay.com, www.amazon.com, …) or all
   but the first save is silently dropped. ALWAYS count landed files vs
   fetched and re-fire after the allow.
3. **Batch fetch** in-page (javascript over the live session): fetch ref →
   verify 200 AND a REAL-CONTENT marker → Blob → `<a download>` click →
   ≥3.5s pause (polite, unattended-safe). Log FAIL/ERR per ref.
   **`body.includes(ref)` is NOT sufficient** — error pages echo the ref in
   the URL/script text (2026-07-29: 80 of 88 ebay captures were "Order not
   found" pages that passed the ref check; fetched under the wrong account).
   Require BOTH: a positive content marker (ebay: an item/price block, e.g.
   `/\$\d+\.\d{2}/` in the body AND the ref) and the ABSENCE of the site's
   error marker (ebay: `Order not found` / `error retrieving your order`).
   Verify the signed-in account is the ref's owner BEFORE the batch (fetch
   one known order and content-check it) — wrong-account fetches return
   error pages for every ref, silently.
4. **Stage** landed files to `data/processed/` (they are documents, not
   imports — nothing parses into src_* tables; the order data is already
   there). Strip Chrome's ` (1)` dupe suffixes while staging.
4.5. **PDF the captures**: `python3 scripts/html_to_pdf.py --delete-html
   data/processed/<staged files>` — every HTML becomes a ref-verified PDF and
   the HTML is removed. MANDATORY before the tracker refresh.
5. **Trackers (MANDATORY post-step)**: `python3 scripts/refresh_doc_trackers.py`
   (build_doc_catalog → migrate_source_documents → migrate_accounts --apply →
   gen_want_manifest, with per-stage deltas). Missing count for the site must
   drop by exactly the number of distinct refs staged.
6. **Disposition the unfetchable** (site retention, digital orders with no
   renderable doc, guest purchases): evidence first, then
   `status='ignored', approved=1` — same doctrine as statement phantoms. Never
   ignore a ref without stating the evidence in the run log.

## Rate limits (learned 2026-07-29, ebay)

eBay hard-limits the order-details endpoint at roughly **80-90 fetches per
burst**; past that every request errors and the session lands on
`pages.ebay.com/limitexceeded.html` ("Daily limit exceeded") for an extended
cooldown (hours). Plan multi-hundred backlogs as MULTI-SESSION work:
~80 orders per run at ≥3.5s spacing, oldest-first, then stop for the day.
Never burn retries against the limit page — check `location.hostname` before
each pass; if it is `pages.ebay.com`, end the run and log the remaining count.

## Guardrails

- Downloads end at the filesystem; nothing here writes src_* or transactions.
- The tracker is the only worklist authority (downloader meta rule).
- Consumable refs: each ref fetched once; re-runs skip refs already 'have'.
- A ref the site cannot render is DISPOSITIONED, not retried forever.
