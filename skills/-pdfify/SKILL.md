---
name: -pdfify
description: Use when captured HTML documents (or lowes ereceipt JSON containers) need converting to the supported document formats — runs scripts/html_to_pdf.py (headless Chrome), verifies each PDF contains its document ref via pdftotext, and deletes the HTML only after verification. Orphan utility; documents are csv/xlsx/pdf ONLY.
argument-hint: "[files/glob | --render-json <container>]"
---

# -pdfify — HTML captures → verified PDFs

Orphan utility skill (no meta owner). Enforces the document format rule:
**mass and per-item documents are csv, xlsx, or pdf.** HTML is only ever a
capture intermediate — convert it, verify it, remove it.

## Tool

`scripts/html_to_pdf.py` — headless Chrome (`--headless --print-to-pdf`,
no added dependencies), 10s virtual-time budget per page.

```bash
# convert staged captures in place, delete html after verification
python3 scripts/html_to_pdf.py --delete-html data/processed/*-ebay-order-*.html

# render lowes ereceipt bodies out of a JSON container
python3 scripts/html_to_pdf.py --render-json data/processed/lowes-ereceipts-b1.json
```

## Verification contract (why this exists)

A conversion counts ONLY when `pdftotext` output contains the document's ref
(parsed from the `YYMMDD-<site>-<kind>-<ref>.html` filename). This catches
garbage captures: error pages echo the ref in URLs/scripts but not in
rendered text (2026-07-29: 80 ebay "Order not found" pages were exposed
exactly this way). Failed/unverified conversions keep their HTML, delete the
bad PDF, and set a nonzero exit code.

## Rules

- Filenames must match a `build_doc_catalog.py` FN pattern (FN_AMZ_INV,
  FN_EBAY_ORD, FN_HD_ORD, FN_LOWES_ORD, FN_GD_RCPT) — both .html and .pdf
  register, so tracker have-counts survive the swap.
- Always finish with `python3 scripts/refresh_doc_trackers.py` (updates
  file_paths to the .pdf, reverts anything whose file vanished).
- Never convert check/deposit images — image-class, exempt from the rule.
- A PDF that fails ref-verification is deleted, never kept "close enough".
