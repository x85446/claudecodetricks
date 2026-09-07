---
name: window-schedule
description: Use when someone asks to draw window drawings, make a window schedule PDF, render scaled window/rough-opening drawings from a spreadsheet, turn a CSV/xlsx of window sizes into to-scale architectural PDFs, or produce a window order cover sheet (shipping address, frame color, project number, per-style counts, total area in sqm/sqft/sqin). Handles dimensions in mm, cm, in, ft-in, or ft-in-fraction.
argument-hint: [path to .csv or .xlsx]
disable-model-invocation: true
allowed-tools: Bash, Read, Write, Edit, AskUserQuestion
---

## What This Skill Does

Reads window sizes (rough opening + window unit) from a CSV or xlsx and produces a
to-scale architectural PDF — **one input row per page**. Each page draws the rough
opening as an outer rectangle with the window unit nested inside, dimension lines on
both, a scale bar, and a title block. By default every dimension is labelled in
**feet-inch-fraction (primary) and millimetres (secondary)**, no matter what units
the input used.

Three behaviours are opt-in via the config (all backward-compatible):
- **mm-only labels** — set `"units": "mm"` to drop ft-in and label everything in mm.
- **Window-size-only render** — leave the rough-opening pair `null` to draw just the
  unit (no outer rectangle, and no "rough opening not provided" warning).
- **Rooms list in the title block** — map a `rooms` column (a comma-separated string)
  to print "Rooms: …" wrapped under the title. This drives **one-page-per-size-group**
  schedules: one row per universal/order size, listing the rooms that take that size
  (see "Rendering one page per size group" below).
- **Window-type operation symbols** — the `style` column doubles as a window *type*:
  if its value contains `double hung`, `slider`/`sliding`, or `double hung` +
  `mullion`/`pillar`/`side-by-side`, the engine draws the matching elevation symbol
  inside the unit (meeting rail + vertical arrows for hung; meeting stile + horizontal
  arrows for slider; a to-scale center-mullion band splitting two hungs for the mullion
  case). The mullion is drawn at `mullion_mm` wide (default 90 mm, labeled on the post),
  and the two sashes size to `(width − mullion)/2`. A `fold`/`accordion` value (e.g.
  "Aluminum folding patio door — 4-panel, outswing, right stack") draws N equal panels
  with a dashed accordion zig-zag, a stack-side arrow, and an `N-PANEL FOLD · OUTSWING ·
  STACK R/L` label (panel count, stack side, and swing parsed from the text). The full
  text also prints as a "Type:" line. Any other/blank value just draws a plain unit.
- **Cover sheet** — add a `"cover"` block to prepend a summary first page: project #,
  frame color, shipping address (all optional, shown only if given), plus auto-computed
  order totals — **total window units** and (if any) **total door units** counted
  separately (a row is a *door* when its `style` has `door`/`fold`/`accordion`),
  **total area in sqm with sqft + sqin conversions**, **count-by-window-type** and
  **count-by-door-type** tables, and a by-size manifest.
- **Insect screens** — set `"screens"` to mark screened units: `"operable"` (operable
  windows only — excludes fixed `picture`/`fixed` glass and doors), `"all_windows"`, or
  `"all"` (incl. doors). Adds a **Screens required** total to the cover and a
  `Screens: Required (qty)` line to each screened unit's title block.
- **Multi-panel assemblies** — map a `layout` column (spec like
  `SL:24 | DOOR:107:hinge=R,swing=out | SL:24`) to draw an assembly of fixed sidelites
  and swing doors in one unit (mullion posts, `FIXED` lites, dashed door-swing
  triangles). Ideal for entry doors with sidelites. See [reference.md](reference.md).
- **Quote options** — map a `quote` column; truthy rows render as labeled
  **"Quote Option"** pages and are excluded from all cover totals/counts/manifest (for
  design alternatives where only one is ordered).

Input dimensions may be in any of: `mm`, `cm`, `in`, `ftin` (e.g. `2'11"`), or
`ftin_frac` (e.g. `2'10"15/16`). Everything is converted to millimetres internally.

All the parsing, conversion, and drawing lives in **`draw_windows.py`** (same folder
as this file). This skill orchestrates it: inspect → confirm mapping → draw.

## Context / Setup

- Worker script: `draw_windows.py` (in this skill's directory).
- Conversion + column-mapping details: see [reference.md](reference.md).
- Dependencies: `pandas`, `openpyxl`, `reportlab`. If a run reports missing packages,
  install them: `python3 -m pip install --quiet pandas openpyxl reportlab`.

## Steps

**0. Resolve the input file.**
Use the `$1` argument as the path if given. Otherwise look in the current directory
for a `.csv`/`.xlsx`; if there are several, ask the user which one with
AskUserQuestion. Set `SKILL_DIR` to this skill's directory (where `draw_windows.py`
lives).

**1. Inspect the file and propose a column mapping.**
Run:
```
python3 "$SKILL_DIR/draw_windows.py" inspect "<input file>"
```
This prints JSON: every sheet's headers, sample rows, a `recommended_sheet`, and a
heuristic `proposed_mapping` per sheet (room, id, qty, style, and the four dimension
columns `ro_width`/`ro_height`/`win_width`/`win_height` each with a guessed `unit`).

**2. Confirm the mapping with the user.**
Show the user the recommended sheet and proposed mapping in a compact form, e.g.:
```
Sheet: Windows
  Rough Opening W ← 'Opening width'  (cm)
  Rough Opening H ← 'Opening height' (cm)
  Window unit  W  ← 'order width'    (cm)
  Window unit  H  ← 'order height'   (cm)
  Room ← 'Room'   ID ← 'ID'   Qty ← 'Qty'   Style ← 'Style'
```
Ask the user to confirm or correct it (which sheet, which columns map to what, and
the **unit of each dimension column**). Use AskUserQuestion if a unit is unknown
(`null` in the proposal) or any dimension column is missing — never guess a unit
silently. If the file only has a window size (no rough opening) or vice-versa, that's
fine; leave the missing pair as `null` and the drawing adapts.

**3. Write the confirmed mapping config to a temp file** (e.g. `/tmp/window_schedule_config.json`):
```json
{
  "file": "<absolute path to input>",
  "sheet": "<sheet name, or omit for CSV>",
  "output": "<input dir>/<stem>_window_schedule.pdf",
  "fraction_denominator": 16,
  "units": "dual",
  "mullion_mm": 90,
  "cover": {"project": "6505-2026", "color": "Matte black", "ship_to": "123 St\nCity, ST"},
  "columns": {
    "room": "Room", "id": "ID", "qty": "Qty", "style": "Style", "rooms": null,
    "dims": {
      "ro_width":  {"col": "Opening width",  "unit": "cm"},
      "ro_height": {"col": "Opening height", "unit": "cm"},
      "win_width":  {"col": "order width",  "unit": "cm"},
      "win_height": {"col": "order height", "unit": "cm"}
    }
  }
}
```
- Set any unused identity column (room/id/qty/style/rooms) to `null` or omit it.
- Set a missing dimension pair to `null` (e.g. `"ro_width": null` for window-size-only).
- `fraction_denominator` defaults to `16`; change to `8` or `32` only if the user asks.
- Each dim `unit` is one of `mm | cm | in | ftin | ftin_frac`.
- `units` is `"dual"` (default — ft-in + mm) or `"mm"` (mm-only labels). Confirm with
  the user which they want; set `"mm"` when they say "everything in mm".
- `rooms` maps a column whose cells are a comma-separated room list; it prints as a
  wrapped "Rooms:" line in the title block. Leave `null` for normal one-window-per-row.
- `mullion_mm` (default `90`) is the to-scale width of the center mullion for
  double-hung-with-mullion types; the post is drawn at this width and labeled.
- `cover` (omit for none) prepends a cover/summary page. All keys optional:
  `project`, `color` (frame color), `ship_to` (address; split on newlines, else commas).
  The order totals, per-style counts, and area (sqm + sqft + sqin) are computed from the
  drawn rows — `qty` weights the counts and area, so map `qty` for accurate totals.
  If the user supplies a **shipping address, frame color, and/or project number** when
  invoking, put them here. `style` values drive the count-by-style table.

**4. Render the PDF.**
```
python3 "$SKILL_DIR/draw_windows.py" draw /tmp/window_schedule_config.json
```
It prints JSON: `output` path, `windows_drawn`, and any `skipped` rows (rows with no
usable dimensions). Relay the count and the output path to the user.

**5. Report.**
Tell the user the PDF path, how many windows were drawn, and list any skipped rows or
per-window warnings (e.g. "window width exceeds rough opening"). Offer to re-run with
a different unit/mapping or fraction precision if needed.

## Rendering one page per size group (universal sizes)

When the user wants **one page per distinct order/universal size** (not one per
window) — e.g. "draw the A1 window once and record the rooms that use it" — the input
needs **one row per size**, not one per window. The drawing engine does not group;
you build the grouped input first, then point the skill at it.

1. **Build a derived input** (CSV is simplest) with one row per size group, columns:
   - a label column (e.g. `label` = `A1`, `A2`, … or the room+id for one-off sizes),
   - `qty` = how many windows take that size,
   - `win_w`/`win_h` = the size to order for that group (e.g. the group's chosen/min
     order size; for a one-off, that window's own size),
   - `rooms` = the rooms that take it, e.g. `Dining Room ×8, Living Room ×4, Garage`.

   Aggregate the source sheet (read it with `pandas`/`openpyxl`) to produce this —
   group by the size-code column, count per group, and join the room names. Include
   any ungrouped one-off windows as their own single rows.

2. **Config it** as window-size-only + rooms + (usually) mm: map `id` → the label
   column, `qty` → `qty`, `rooms` → `rooms`, `win_width`/`win_height` → the size
   columns, and set both `ro_*` to `null`. Set `"units": "mm"` if requested.

3. **Render** as in step 4. Each page shows the unit drawn to scale with the label,
   qty, size, and the wrapped room list in the title block.

## Output

- A multi-page PDF at `<input dir>/<stem>_window_schedule.pdf` (or the `output` path
  in the config). One input row per page (a single window, or a size group), Letter
  portrait, drawn to a clean architectural scale (1:5 … 1:250, chosen per page to fit).

## Notes

- **Never invent dimensions.** Only draw what's in the file. Missing/zero/negative
  sizes are skipped or flagged, not fabricated.
- The window unit is centred inside the rough opening (the gap is assumed symmetric —
  the input rarely specifies side-by-side offsets).
- Scale is chosen per page so the larger of RO/window fills the drawing area; the
  exact ratio is printed and a scale bar is drawn, so the page is dimensionally honest.
- **Units:** default labels are ft-in + mm; `"units": "mm"` makes them mm-only (drawing
  ticks, scale bar, and title block all switch). Input `unit`s still convert as usual.
- **Rooms list:** a mapped `rooms` column prints a wrapped "Rooms:" line in the title
  block — keep each cell to room names (optionally with `×count`) so it stays short.
- **Verifying the PDF visually:** render with Ghostscript, not `pdftoppm` —
  `magick -density 150 'out.pdf[0]' page.png`. Some `pdftoppm` builds don't rasterize
  the base-14 fonts and show a blank-text page even though the PDF is fine (confirm
  text with `pdftotext out.pdf -`).
- For multi-sheet workbooks, always confirm the sheet — the recommended one is a guess
  based on how many dimension columns matched.
- This skill has side effects (writes a PDF) and is `disable-model-invocation: true`;
  invoke it explicitly with `/window-schedule`.
