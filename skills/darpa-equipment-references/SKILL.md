---
name: darpa-equipment-references
description: "Use when someone asks to update, build, regenerate, add to, modify, or sync the DARPA equipment list / equipment references docx from a cost-volume spreadsheet or a pasted equipment table."
argument-hint: "[<path-to-xlsx-or-paste-file>] [<output-docx>]"
disable-model-invocation: true
---

## What This Skill Does

Builds the DARPA proposal **Equipment List `.docx`** from either:

- the `Base (Year 1)` tab of a DoW Cost Volume `.xlsx` (default source), or
- a pasted equipment table saved to a text file.

The output is the same shape as the canonical reference at
`~/workspace/izuma/marketing/opportunities/darpa/Equipment List.docx` — one
`EQ###` block per item with **Product / Vendor / Cost & Quantity /
Explanation / Link / product-page screenshot**, in the order the rows
appear in the source.

The `Link:` value is a real clickable hyperlink in Word, and a screenshot
of the product page is embedded below it (5.5″ wide). URLs are
**canonicalised** before use — the query string (e.g. Google's
`?srsltid=...` tracking tail) is stripped, and the short URL is verified
live before it replaces the original. If the short form doesn't resolve,
the original URL is kept.

The skill supports **add / delete / modify / reorder** in one shot: just edit
the source spreadsheet (or paste) and re-run. Items that still match an
existing entry by URL or title keep their hand-written `Explanation` and
canonical `Product:` name; new items get a fresh Explanation written by
Claude using `templates/CONTEXT.md`.

This skill has side effects (writes a .docx). It is `disable-model-invocation: true` —
invoke it explicitly via `/darpa-equipment-references` or by quoting the
trigger phrase.

## Inputs

- **Source** (first arg, optional): path to either
  - a `.xlsx` (treated as a DoW Cost Volume → reads the `Base (Year 1)` tab,
    equipment block starts at row 97 header / row 98 data), OR
  - any other file (`.txt`/`.tsv`/`.md`) containing pasted rows. Pasted rows
    use whitespace or TAB columns in this order:
    `No  Description  Vendor  Qty  Cost  Total  Exclusive?  Competitive?  Supporting?  Link`.
- **Output docx** (second arg, optional). Default: the existing
  `opportunities/darpa/Equipment List.docx` (overwritten in place; the file
  is read first to preserve existing Explanations).

If the user invokes the skill with no args, ask them:

1. "Source — path to the `.xlsx` (with a `Base (Year 1)` tab) or a file with
   pasted rows? You can also paste the rows in chat and I'll save them to a
   temp file."
2. (Only if the default output exists and they want a different target)
   "Write to `Equipment List.docx` or a different path?"

If the user **pastes** rows in chat instead of giving a path, write the paste
verbatim to `/tmp/darpa-equipment-paste.txt` (Write tool) and use that as the
source.

## Workflow

Run these steps strictly in order. Do not skip the existing-extract step
(otherwise hand-written explanations will be lost).

**Step 1 — Parse the source into items JSON (with URL canonicalization).**

```bash
python3 .claude/skills/darpa-equipment-references/scripts/parse_input.py \
  "<SOURCE>" --out /tmp/darpa-equipment-items.json
```

This emits one record per row with fields: `no, description, vendor, qty,
cost_per_item, total_cost, exclusive, competitive, supporting, link,
link_original`. The `no` field is re-numbered 1..N — **source row order
becomes `EQ001`..`EQ###` in the output** regardless of what numbers the user
pasted.

`link` is the canonical URL (query string + fragment stripped) **only when
the short URL has been verified live with a GET**. If the short URL fails,
`link` falls back to the original (kept in `link_original` for reference).
Pass `--no-check-urls` to skip the network check (offline mode); the script
still strips the query string.

If the script aborts with a parse error, show the user the error and stop.

**Step 2 — Extract existing entries from the output docx (if it exists).**

```bash
python3 .claude/skills/darpa-equipment-references/scripts/extract_existing.py \
  "<OUTPUT_DOCX>" --out /tmp/darpa-equipment-existing.json
```

If `<OUTPUT_DOCX>` does not yet exist, skip this step — there's nothing to
preserve.

**Step 3 — Merge & inspect (read-only) so you know which items need new
explanations.**

Read both JSON files. The build script will auto-merge by link (canonicalised
by stripping the query string) or by description→title match. For any item
where the match yields an empty `explanation`, Claude must write one before
the final build.

**Step 4 — Write missing explanations.**

Read `templates/CONTEXT.md` once. Then, for each item in the items JSON that
has no matching existing entry, **edit the items JSON in place** to add:

- `"product"`: the full canonical product name (e.g. "Kvaser USBcan Pro
  2xHS v2"). The xlsx `Description` column is usually a shortened form
  ("dual channel CAN/FD /USB") — derive the real product name from the link
  or vendor catalogue when obvious. If unsure, leave it equal to
  `description` and flag it to the user.
- `"explanation"`: 1–3 sentence paragraph following the voice in
  `templates/CONTEXT.md`. Anchor on how the item supports Izuma Edge
  testing in the relevant environment (vehicle / industrial / edge compute /
  security). Do **not** repeat the product name verbatim.

Use the Edit tool to write these in. Don't regenerate the entire JSON file
or re-run parse_input.py — that would clobber the explanations you just
wrote.

**Step 5 — Cache product-page screenshots.**

```bash
python3 .claude/skills/darpa-equipment-references/scripts/cache_screenshots.py \
  --items /tmp/darpa-equipment-items.json
```

This uses Chrome headless (`/Applications/Google Chrome.app/...`) to capture
a 1280×1600 PNG of each item's `link` and writes it to
`.claude/skills/darpa-equipment-references/cache/screenshots/<sha1>.png`.
The script annotates each item with `"screenshot": "<absolute-path>"` (or
`null` on failure) and rewrites the JSON in place. Cached images are reused
on subsequent runs; pass `--force` to refetch.

If Chrome is missing, the script aborts with an install hint. In that case,
skip this step and the build will simply omit the embedded images.

**Step 5b — Recapture bot-blocked pages via real Chrome (macOS).**

```bash
python3 .claude/skills/darpa-equipment-references/scripts/recapture_blocked.py \
  --items /tmp/darpa-equipment-items.json
```

Some vendor pages (DigiKey, Toradex, Contemporary Controls, anything fronted
by Cloudflare or CloudFront) reject headless Chrome with an anti-bot
challenge. This step:

  1. OCR-scans each cached screenshot for known interstitial phrases
     (`"Performing security verification"`, `"Just a moment"`,
     `"Verifying you are human"`, `"Generated by cloudfront"`, etc.) and
     marks the item as `cloudflare_blocked: true`.
  2. For each blocked item, invokes `chrome_capture.sh <url> <png>` which
     drives the user's **real, already-running Google Chrome** via
     AppleScript: opens a new window pinned to a known screen region,
     waits for the CF challenge to auto-clear (polls the tab title until
     it stops being `"Just a moment..."`), then uses
     `screencapture -R<x,y,w,h>` to write a real product-page PNG into
     the cache slot.

Requirements: macOS, `tesseract`, Chrome installed and running. Do NOT
move/minimise Chrome windows while this step is running — the capture is
positional. The whole pass takes ~10–15 seconds per blocked item.

After this step, run **`crop_screenshots.py --force`** so the cropped
versions get refreshed from the new originals.

**Step 6 — Crop screenshots to the product hero.**

```bash
python3 .claude/skills/darpa-equipment-references/scripts/crop_screenshots.py \
  --items /tmp/darpa-equipment-items.json
```

Content-aware crop pass over the cache: trims left/right whitespace gutters,
then finds the first sustained whitespace gap below the 30% line and crops
just above it. Falls back to 55% of image height when no gap is found
(typical for Amazon-style densely-packed PDPs). Writes `<sha1>_crop.png`
beside the original and updates `item["screenshot"]` to point at the
cropped file. Always derives the source from the original (un-cropped) PNG,
so re-running is idempotent and `--force` re-crops from scratch.

Tunables: `--gap-rows`, `--row-threshold`, `--col-threshold`,
`--min-top-fraction` (default 0.30), `--max-top-fraction` (default 0.55).

**Step 7 — OCR-verify that each page surfaces a price.**

```bash
python3 .claude/skills/darpa-equipment-references/scripts/find_prices.py \
  --items /tmp/darpa-equipment-items.json
```

Runs Tesseract OCR on each cropped screenshot and classifies it as:

  * **`ok`** — at least one currency token (`$\d+(\.\d{1,2})?`, €, £) AND a
    buy-button keyword ("Add to cart" / "Buy Now" / "In Stock" / etc).
  * **`suspect`** — currency tokens found, but no buy-button keyword. The
    page may have surfaced "alternative items" or related products instead
    of the real listing. Treat as a likely missing-price case.
  * **`missing`** — no currency tokens at all.

Annotates each item with `prices_found`, `has_buy_button`, `price_status`,
and prints a stderr summary of every `suspect`/`missing` item.

For each non-`ok` item, the skill MUST in Step 8:
  a. Find an alternative URL on a different vendor (use WebSearch / WebFetch)
     that shows a real listed price for the same product.
  b. Surface the suggested URL to the user in the final report and ask
     whether to swap. Do NOT silently rewrite the source xlsx.

Tesseract is required (`brew install tesseract`). If missing, the script
aborts; skip this step if you must, but the user won't get the price
check.

**Step 8 — Build the docx.**

```bash
python3 .claude/skills/darpa-equipment-references/scripts/build_docx.py \
  --items /tmp/darpa-equipment-items.json \
  --template "<OUTPUT_DOCX>" \
  --existing /tmp/darpa-equipment-existing.json \
  --out "<OUTPUT_DOCX>" \
  --strict
```

`--template` and `--out` can be the same path — the script reads the template
into memory before writing. The `--strict` flag aborts if any item still has
no `product` or `explanation`; if it errors, fix the items JSON and re-run.

The `Link:` line becomes a real clickable Word hyperlink, and the cached
screenshot (if any) is embedded immediately below the link at 5.5″ wide.

If the output docx does not exist yet, point `--template` at the canonical
reference at
`~/workspace/izuma/marketing/opportunities/darpa/Equipment List.docx` so the
new file inherits the same header/footer/images, and omit `--existing`.

**Step 9 — Report.**

Tell the user:

- Path of the written docx
- Item count and the EQ range (e.g. "EQ001..EQ020")
- Items where the explanation was newly written (so they can review)
- Items where the explanation was preserved verbatim from the old doc
- Any items that were deleted vs the previous version (by EQ heading)
- Any screenshots that failed to capture (so the user can refetch manually)
- Any URLs where the canonical (short) form failed verification and the
  original (tracking) URL was kept
- **Every item with `price_status` ∈ {`missing`, `suspect`}** — list the
  EQ#, title, current URL, and a suggested replacement URL from a vendor
  that does show the price clearly (Adafruit, PiShop.us, Digi-Key,
  Mouser, the vendor's own product page, etc.). For each suggestion, run
  WebFetch to confirm the listed price before recommending.

## Add / Delete / Modify / Reorder semantics

The source (xlsx or paste) is authoritative for ordering, presence, vendor,
qty, and cost. The output docx is authoritative for `product` (full name)
and `explanation` text where the link / title still matches.

- **Add** — a new row in the source with a link not seen before. Claude
  writes a fresh Explanation in Step 4.
- **Modify** — an existing row's description, qty, or cost changes. The
  explanation is kept (Step 2 + 3 match by link).
- **Delete** — an item disappears from the source. It vanishes from the
  output. Claude must mention removed EQ headings in the final report so
  the user can confirm.
- **Reorder** — the source row order rewrites `EQ###` numbering. `EQ001`
  always = source row 1. There is no stable "EQ ID" across edits.

If the user wants stable EQ numbering across edits (e.g. "don't renumber
EQ005 just because I deleted EQ003"), tell them this skill renumbers and ask
whether they still want to proceed.

## Guardrails

- **Never** edit the source xlsx. This skill only reads from it.
- The output docx is overwritten in place. Before Step 5, ensure the user
  has the previous version backed up if they care — note the `.bakN.xlsx`
  files alongside the template suggest the user keeps backups manually. If
  the output target exists and was modified within the last 60 seconds, ask
  before overwriting.
- `--strict` must be passed to `build_docx.py` on the final run. The
  literal string `[EXPLANATION NEEDED — fill in items JSON]` must never
  appear in a delivered docx.
- Do not invoke API-paid summarization (anthropic, openai, etc.) for the
  explanations — Claude writes them in conversation using `CONTEXT.md`.
- If `python-docx` or `openpyxl` is missing, surface the install hint
  emitted by the script and stop.

## Outputs

- `<OUTPUT_DOCX>` (default: `~/workspace/izuma/marketing/opportunities/darpa/Equipment List.docx`)
- Side-effect temp files (safe to delete):
  - `/tmp/darpa-equipment-items.json`
  - `/tmp/darpa-equipment-existing.json`
  - `/tmp/darpa-equipment-paste.txt` (only if the user pasted rows)
- Persistent cache (keep — re-used across runs):
  - `.claude/skills/darpa-equipment-references/cache/screenshots/<sha1>.png`

## Notes for Claude

- The xlsx column layout is fixed by the DoW Cost Volume template. If a
  future template moves columns, edit `scripts/parse_input.py` `COLUMNS`
  rather than special-casing in this file.
- When writing new explanations, prefer **specificity** over generality.
  "Used to interface with automotive Ethernet networks" beats "Used for
  networking". The reviewer reads dozens of these — each one should make
  the line item's job obvious.
- The `Product:` line is where the *long* vendor name lives. The heading
  uses the short `Description` from the spreadsheet. Don't conflate them.
