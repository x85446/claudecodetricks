---
name: marketing-doc-formatter
description: "Use when someone asks to make a one-pager, create a marketing PDF, format a document for marketing, stylize a product doc, brand a document, generate a whitepaper PDF, create marketing material, produce a multi-pager, or format product documentation."
argument-hint: "<path-to-markdown> [one-pager|multi-pager|whitepaper] [--template business-classic|photo-ready]"
---

## What This Skill Does

Takes an existing markdown product document and produces a branded, styled PDF for Izuma Networks using the **canonical templates** in `templates/`. Supports three document types: one-pager, multi-pager, and whitepaper.

**This skill is a writer, not a designer.** It applies the canonical template as-is and does not prompt the user for layout, header, or footer preferences. If the user wants a different visual design, direct them to `marketing-template-studio`, which experiments with template variants and promotes a winner back into `templates/` — where this skill then picks it up automatically.

Never modifies the original source markdown.

## Pre-Flight Checks

Before doing anything else:

1. **Check assets symlink:** Verify `./assets/Logo-Branding` exists and is accessible.
   - If it does NOT exist, tell the user: "The Logo-Branding asset link is missing. Please read `./assets/README.md` for instructions on creating the symlink, then re-run this skill."
   - **Hard stop** — do not proceed without assets.

2. **Check PDF converter:** Verify `wkhtmltopdf` is available on PATH.
   - If missing, suggest: `brew install wkhtmltopdf`
   - **Hard stop** — do not proceed without a PDF converter.

3. **Check page tools:** Verify `pdfinfo` and `pdftoppm` are available (used in the iterative fit loop).
   - If missing, suggest: `brew install poppler`
   - **Hard stop** — do not proceed without these.

4. **Check pandoc (for multi-page docs):** Verify `pandoc` is available on PATH.
   - If missing, suggest: `brew install pandoc`
   - **Hard stop for multi-page docs** — multi-pagers and whitepapers use pandoc for markdown → HTML conversion.

## Inputs

- **Source markdown:** Path provided as argument or asked from user
- **Brand guide:** `guidelines/branding.md`
- **Logo:** `./assets/Logo-Branding/izuma-logo-horizontal.svg` (primary), `./assets/Logo-Branding/izuma-logo-horizontal.png` (fallback)
- **Brand colors:**
  - Primary blue: `#3684F5`
  - Accent orange: `#FF9C2B`
  - Cobalt (dark accent): `#205388`
  - Green (use sparingly): `#365129`
- **Typography:**
  - Headlines: Poppins Medium/Bold
  - Body: Raleway
  - Fallbacks: `"Poppins", "Futura", "Arial", sans-serif` / `"Raleway", "Arial", "Helvetica", sans-serif`

## Page Layout Rules (CRITICAL — READ FIRST)

These rules prevent the most common layout bugs (asymmetric margins, content cut-off at bottom, content not filling the page). Violating any of them will produce broken output.

### Rule 1: Always Set Page Margins via wkhtmltopdf CLI

**`wkhtmltopdf` ignores CSS `@page margin`.** Only its `--margin-top/-bottom/-left/-right` CLI flags actually set the printable area. Setting `@page { margin: 12mm }` in CSS has NO effect on the PDF margins — you will get zero margins regardless.

- **Do this:** Pass `--margin-top Xmm --margin-bottom Xmm --margin-left Xmm --margin-right Xmm` to wkhtmltopdf with symmetric values.
- **Do not this:** Write `@page { size: letter; margin: 12mm }` and expect it to work.
- **Do write:** `@page { size: letter; }` in CSS (for page size only). Leave margin handling to the CLI.

**One-pager command:**
```bash
wkhtmltopdf --enable-local-file-access --page-size Letter \
  --margin-top 12mm --margin-bottom 12mm --margin-left 15mm --margin-right 15mm \
  --disable-smart-shrinking \
  in.html out.pdf
```

**Multi-pager / whitepaper command (with wkhtmltopdf-rendered footer):**
```bash
wkhtmltopdf --enable-local-file-access --page-size Letter \
  --margin-top 18mm --margin-bottom 18mm --margin-left 18mm --margin-right 18mm \
  --disable-smart-shrinking \
  --footer-center "[page] of [topage]" --footer-right "Izuma Networks Inc." \
  --footer-font-size 8 --footer-spacing 4 \
  in.html out.pdf
```

### Rule 2: Symmetric Margins

Left margin MUST equal right margin. Top and bottom SHOULD be equal (small top/bottom differences OK for breathing room under a header). Use a single value where possible:

- **Good:** `margin: 12mm;` (all four sides equal)
- **Good:** `margin: 14mm 12mm;` (top/bottom 14mm, left/right 12mm — symmetric horizontally)
- **BAD:** `margin: 12mm 15mm 10mm 8mm;` (every side different — this is the bug we're preventing)

### Rule 3: No Per-Element Right Padding

Never add `padding-right: Xmm` or `margin-right: Xmm` to individual elements (p, hr, table, div, etc.). Elements should inherit their bounds from the page margin, not recreate them individually.

- **BAD:** `p { padding-right: 15mm; }` — creates implicit asymmetry
- **BAD:** `.accent-box { margin: 5px 15mm 4px 0; }` — same problem
- **Good:** Let the `@page` margin (or wkhtmltopdf CLI margin) define the content area bounds. Elements fill the full content width naturally.

If you need a narrower column, wrap in a container with equal left/right margin (e.g., `.col { margin: 0 auto; max-width: 80%; }`).

### Rule 4: No `position: fixed` Footers for Paginated Content

`position: fixed` does not reserve flow space. Content at the bottom of a page will flow UNDERNEATH a fixed footer and get occluded — this is why the bottom of one-pagers was getting cut off.

- **For one-pagers (single page):** Either put the footer inline at the end of the document as a normal block-flow element, OR use `wkhtmltopdf --footer-*` flags.
- **For multi-pagers/whitepapers:** Always use `wkhtmltopdf --footer-*` flags. Never put a `position: fixed` footer in the HTML.

### Rule 5: No `min-height: 100vh` in Print Styles

`100vh` means viewport height, which is not reliable in print media. Don't use it to try to force a page to "fill." Instead:
- Set page margins appropriately
- Size content to actually fit the available area
- Use `@media print` for print-only tweaks if needed

### Rule 6: Sidebar / Border-Left Accents

If you want a sidebar/border-left accent design, apply it to an **inner container** with equal margins on both sides — not to `body`. Applying it to `body` makes the left edge visually closer to the content than the right edge.

- **BAD:** `body { border-left: 5px solid #205388; padding-left: 12px; }` — creates asymmetry
- **Good:** Center the content and draw the sidebar as an absolute-positioned element inside the content area, OR skip the sidebar for a simpler symmetric layout.

### Rule 7: Always Pass `--disable-smart-shrinking` to wkhtmltopdf

`wkhtmltopdf` by default "smart shrinks" content to fit, which distorts typography at the margins. Always pass `--disable-smart-shrinking` so fonts and spacing render at the sizes specified in CSS.

### Rule 8: Content Must Fill the Page

One-pagers should occupy the full usable area of the printed page (within margins). If content is sparse:
- Increase font sizes
- Add vertical spacing between sections
- Expand descriptions
- Add a visual element (callout, stats strip, accent box) to use space

Do NOT let a one-pager finish at 60–70% of the page. The footer or a closing callout should reach near the bottom margin.

Conversely, if content overflows, condense per Step 4 — but never let the overflow silently cut off (Rule 4 and Rule 5 are what cause silent cut-off).

## Step-by-Step Workflow

### Step 1: Gather Inputs

- Read the source markdown file.
- Parse the argument for document type (`one-pager`, `multi-pager`, `whitepaper`). If not provided, ask the user.
- Parse the `--template <name>` flag. If not provided:
  - For one-pagers: default to `business-classic`
  - For multi-pagers / whitepapers / features: only one template per doc type exists currently
- Validate the template name exists at `templates/[doc-type]/[template-name]/template.html`. If it does not, list available templates and stop.
- Read `guidelines/branding.md` for latest brand rules.

**Available one-pager templates (list at runtime via `ls templates/one-pager/`):**
- `business-classic` — restrained minimalist, two-column text layout (default)
- `photo-ready` — reserves a top-right placeholder for a product image / architecture diagram

### Step 2: Generate Styled HTML

Create a single HTML file that embeds all styling inline (no external CSS files). Apply the Page Layout Rules above.

**Typography:**
```css
@import url('https://fonts.googleapis.com/css2?family=Poppins:wght@500;700&family=Raleway:wght@400;500;600&display=swap');

h1, h2, h3, h4 { font-family: "Poppins", "Futura", "Arial", sans-serif; }
body, p, li, td { font-family: "Raleway", "Arial", "Helvetica", sans-serif; }
```

**Color application:**
- `h1`: `#205388` (cobalt)
- `h2`: `#3684F5` (blue)
- `h3`, `h4`: `#205388` (cobalt)
- Body text: `#2D2D2D`
- Accent lines / borders / highlights: `#FF9C2B` (orange)
- Horizontal rules: 2px solid `#3684F5`

**Page setup (all document types — margins set via wkhtmltopdf CLI, NOT CSS):**
```css
@page { size: letter; }   /* ONLY size here — wkhtmltopdf ignores CSS @page margin */
body { margin: 0; padding: 0; }
```
Use `page-break-before: always` on `<h2>` elements for whitepapers if major-section breaks are desired.

**Logo:**
- Embed the SVG logo using an `<img>` tag pointing to the absolute path of `./assets/Logo-Branding/izuma-logo-horizontal.svg`
- The logo goes in an inline header block at the top of the document body (not `position: fixed`).

**Header and footer behavior is determined by the canonical templates** (`templates/one-pager.html`, `templates/multi-page.css`, `templates/multi-page-body.html`). Do NOT prompt the user for layout preferences — those decisions are made upstream in `marketing-template-studio` and baked into the templates. If the user wants different header/footer styling, direct them to `marketing-template-studio` to experiment with a new template variant and promote it.

- **Multi-pager / whitepaper footers** are rendered by wkhtmltopdf via `--footer-*` flags (see command template in Step 4).
- **One-pager footer** is an inline block at the end of the document body (already present in the template).

**Standardized Table Styling (applies to ALL documents uniformly):**
```css
table {
    width: 100%;
    border-collapse: collapse;
    margin: 1em 0;
    font-size: 0.9em;
}
thead th {
    background-color: #205388;
    color: #FFFFFF;
    font-family: "Poppins", "Futura", "Arial", sans-serif;
    font-weight: 600;
    padding: 10px 12px;
    text-align: left;
    border-bottom: 3px solid #FF9C2B;
}
tbody tr:nth-child(even) { background-color: #F0F4FA; }
tbody tr:nth-child(odd) { background-color: #FFFFFF; }
tbody td { padding: 8px 12px; border-bottom: 1px solid #E0E0E0; }
tbody td:first-child { font-weight: 600; color: #205388; }
```

For one-pagers where space is tight, scale table padding/font-size down proportionally but keep the same color/border scheme. This table style is canonical across all document types.

### Step 3: Starting Templates

Templates live in per-template subdirectories. Use the template selected in Step 1.

**One-pager templates** (under `templates/one-pager/[template-name]/`):
- `business-classic/template.html` — canonical minimalist layout (default)
- `photo-ready/template.html` — includes top-right image placeholder zone
- Each directory also contains a `design-notes.md` describing when to use that template

**Multi-page shared files** (under `templates/`):
- `multi-page.css` — shared CSS for multi-pager/whitepaper/features (inject via pandoc `--include-in-header`)
- `multi-page-body.html` — logo header snippet (inject via pandoc `--include-before-body`)

For **one-pagers**, start from `templates/one-pager/[template-name]/template.html` and hand-assemble the content blocks from the source markdown.

For **multi-pager/whitepaper/features**, convert the source markdown to HTML with pandoc:
```bash
pandoc <source>.md -f gfm -t html5 --standalone \
  --metadata title="<Title>" \
  --include-in-header <skill-dir>/templates/multi-page.css \
  --include-before-body <skill-dir>/templates/multi-page-body.html \
  -o <out>.html
```

### Step 4: Convert to PDF and Enter the Iterative Fit Loop

This step has two paths: one for one-pagers (tight fit-to-one-page target) and one for multi-page docs (pagination only).

#### 4a. Multi-page docs (multi-pager, whitepaper, features)

Render once with symmetric 18mm margins:
```bash
wkhtmltopdf --enable-local-file-access --page-size Letter \
  --margin-top 18mm --margin-bottom 18mm --margin-left 18mm --margin-right 18mm \
  --disable-smart-shrinking \
  --footer-center "[page] of [topage]" --footer-right "Izuma Networks Inc." \
  --footer-font-size 8 --footer-spacing 4 \
  output/[product-name]/[doc-type].html output/[product-name]/[doc-type].pdf
```
Record the page count and proceed to Step 6.

#### 4b. One-pager iterative fit loop

One-pagers MUST render as exactly 1 page AND fill ≥85% of the usable area with a body font between **9pt (floor) and 10pt (ceiling)**. The skill runs a self-correcting loop to hit this target.

**State variables:**
- `body_font` — starts at `10.0pt`, floor `9.0pt`, step `0.5pt`
- `iteration` — starts at 1, max 6
- `variant_suffix` — empty by default; set to `-abrv` when prose was shortened
- `changes_log` — running list of adjustments made (to report at the end)

**Loop algorithm (pseudocode — Claude executes this literally):**

```
for iteration in 1..6:
    write HTML with body font-size = body_font
    run wkhtmltopdf (one-pager command)
    pages = pdfinfo output.pdf | grep "^Pages:" | awk '{print $2}'

    if pages > 1:
        if body_font > 9.0:
            body_font -= 0.5
            changes_log += "Reduced body font to ${body_font}pt"
            continue   # next iteration
        else:
            # At 9pt floor and still overflowing — content is too long
            abbreviate content (see "Abbreviation Procedure" below)
            variant_suffix = "-abrv"
            body_font = 10.0   # restart at target font
            changes_log += "Content too long at 9pt floor — shortened prose"
            continue

    # pages == 1 — check fill ratio
    pdftoppm -r 100 -f 1 -l 1 -png output.pdf output/.tmp_page
    read output/.tmp_page-1.png as an image
    evaluate visually: does content fill ≥ 85% of the page vertically?

    if filled >= 85%:
        done — break out of loop
    else:
        # Underfilled
        if body_font < 10.0:
            body_font += 0.5
            changes_log += "Increased body font to ${body_font}pt to fill page"
            continue
        else:
            # At 10pt ceiling and still underfilled — expand content
            add breathing room (larger section gaps, extra callout box)
            changes_log += "At 10pt ceiling but underfilled — added callout/spacing"
            continue

# After loop
if variant_suffix:
    # Abbreviated version — save as output/[product]/one-pager-abrv.pdf
    rename/save PDF with suffix
```

**Output naming:**
- Default (content fit at 9–10pt without abbreviation): `output/[product-name]/one-pager.pdf`
- Prose had to be shortened to keep font ≥ 9pt: `output/[product-name]/one-pager-abrv.pdf`
- If other structural changes are needed, use a matching descriptive suffix (e.g., `one-pager-notable.pdf` if a less-critical table was dropped). Keep the suffix short and lowercase.

**Abbreviation Procedure (when font would go below 9pt):**

Apply in this order, stopping as soon as the content fits at 9pt:
1. Shorten capability paragraphs to ~1 sentence each (remove examples, specs)
2. Merge adjacent related capability sections (e.g., "Edge Computing" + "Monitoring")
3. Reduce the largest table by 1–2 of the least-critical rows
4. Drop a secondary table (e.g., "Platforms" list becomes inline text)
5. Replace the closing accent box with a shorter one-line tagline

Do NOT:
- Modify the source markdown
- Drop required sections (brand header, title, at least one capability block, at least one comparison table, closing differentiators)
- Reduce font below 9pt

After applying abbreviations, record what was cut in `changes_log` to report to the user.

**Cleanup:** Delete `output/.tmp_page-1.png` (and any other `.tmp_*` files) after the loop exits.

### Step 5: Report

Tell the user:
- Output file location and filename (including any variant suffix like `-abrv`)
- Page count
- Body font size the output landed at
- Number of iterations used
- Full `changes_log` — every font adjustment and content change, in order
- If an abbreviated variant was produced, explain why (content too long at 9pt floor)
- Suggest they review and re-run with adjustments if needed

## Output Location

```
output/[product-name]/
  [doc-type].html    # intermediate styled HTML
  [doc-type].pdf     # final output
```

Product name is derived from the source file's parent directory name (e.g., `products/myriplane/one-pager.md` -> `output/myriplane/`).

## Notes

- **Never modify the original source markdown.** All transformations happen in the generated HTML.
- **Tables must look identical** across all document types.
- Use the SVG logo by default; fall back to PNG only if SVG fails to render.
- Google Fonts are linked for Poppins and Raleway. If offline, the CSS fallback chain handles it.
- The green color (`#365129`) should only appear if explicitly needed — it is not part of the standard palette.
- If the user asks for a format not listed (e.g., "brochure"), treat it as a multi-pager with a note that the template may need refinement.
- When layout issues appear (margins off, content cut off, not filling page), return to the Page Layout Rules section and verify each rule is being followed.
- **Body font constraints:** One-pagers must land between 9pt (floor) and 10pt (ceiling). If 9pt is not enough to fit, the iterative loop triggers abbreviation and saves the output with a `-abrv` suffix. Multi-page docs use 10.5pt.
- **Templates:** One-pager templates are per-named-subdir under `templates/one-pager/[template-name]/template.html`. `templates/multi-page.css` and `templates/multi-page-body.html` are shared across multi-page doc types. Replace the `{{PLACEHOLDER}}` tokens with content for the current product. Do not edit the CSS or structural layout in these templates — adjust only the body font-size inside the iterative loop, and the content blocks. If the user wants visual changes, route them to `marketing-template-studio`.
- **Template selection:** `--template <name>` argument picks which one-pager template to use. Default is `business-classic`. Current options: `business-classic`, `photo-ready`.
- **Do not interview the user for header/footer/layout preferences.** Those decisions live in the canonical template and are edited via `marketing-template-studio`, not here.
- **`{{LOGO_ABS_PATH}}`:** Always substitute with the absolute path to the SVG logo (e.g., `/Users/travis/workspace/izuma/marketing/assets/Logo-Branding/izuma-logo-horizontal.svg` resolved via `realpath`).
- **Fit loop diagnostics:** If the loop exhausts all 6 iterations without landing a good fit, stop and report back to the user with the current state — don't loop forever.
