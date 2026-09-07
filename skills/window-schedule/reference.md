# window-schedule reference

Technical details for the `window-schedule` skill and its worker `draw_windows.py`.

## Unit conversion

All inputs are converted to **millimetres** internally, then formatted for output.

| Input unit   | Meaning                          | Example          | Parsed as |
|--------------|----------------------------------|------------------|-----------|
| `mm`         | numeric millimetres              | `901`            | 901 mm |
| `cm`         | numeric centimetres              | `90.14`          | 901.4 mm |
| `in`         | numeric inches                   | `35.5`           | 901.7 mm |
| `ftin`       | feet-inch **string**             | `2'11"`          | 889 mm |
| `ftin_frac`  | feet-inch-fraction **string**    | `2'10"15/16`     | 887.4 mm |

`ftin` and `ftin_frac` are parsed by the same tolerant regex, which accepts:
`2'11"8/16`, `2'11"`, `5'9"6/16`, `11 1/2"`, `36"`, `4'0"`, and `ft` as a substitute
for `'`. A value that can't be parsed is treated as missing (the row is flagged, not
guessed).

### Output formatting

With `"units": "dual"` (default) every dimension is shown twice on the drawing and in
the title block:
- **Primary:** feet-inch-fraction, e.g. `5'10"13/16`. The fraction is reduced
  (`8/16` → `1/2`) and dropped when zero (`6'0"`). Precision is set by
  `fraction_denominator` (16 by default; 8 or 32 also supported). Carry is handled
  (e.g. `11.999"` → `1'0"`).
- **Secondary:** whole millimetres, e.g. `1799 mm`.

With `"units": "mm"` the ft-in primary is dropped everywhere (dimension labels, scale
bar, and title block) and dimensions read as whole millimetres only.

## Column mapping

`inspect` mode emits a `proposed_mapping` using header keyword heuristics:

| Mapping key   | Matched by (case-insensitive)                         |
|---------------|--------------------------------------------------------|
| `room`        | header containing `room` (else first column)           |
| `id`          | `id` / `opening` / `item` / `mark`                     |
| `qty`         | `qty` / `quantity`                                     |
| `style`       | exactly `style`                                        |
| `rooms`       | not auto-proposed; set manually for a title-block room list |
| `layout`      | not auto-proposed; multi-panel assembly spec (see below) |
| `quote`       | not auto-proposed; truthy → drawn as a quote option, excluded from totals |
| `ro_width`    | `opening`+`width`, `rough`+`width`, `ro`+`width`       |
| `ro_height`   | `opening`+`height`, `rough`+`height`, `ro`+`height`    |
| `win_width`   | `order`/`unit`/`window`/`net` + `width`                |
| `win_height`  | `order`/`unit`/`window`/`net` + `height`               |

Unit is guessed from the header (`mm`, `cm`, `(in)`, `ft-in-16` → `ftin_frac`, …);
when no hint is found it defaults to `cm` and **must be confirmed by the user**.

For multi-sheet workbooks, each sheet is scored by how many of the four dimension
columns matched; the highest-scoring sheet is `recommended_sheet`. Always confirm.

## Config schema (draw mode)

```json
{
  "file": "abs/path/to/input.xlsx",   // required
  "sheet": "Windows",                  // xlsx only; omit/ignore for CSV
  "output": "abs/path/out.pdf",        // optional; defaults to <stem>_window_schedule.pdf
  "fraction_denominator": 16,          // 8 | 16 | 32
  "units": "dual",                     // "dual" (ft-in + mm, default) | "mm" (mm-only)
  "source_label": "Project 6505",      // optional; overrides the page-header/cover source text (defaults to the input filename)
  "screens": "operable",               // optional; insect screens: omit/false | "operable" (operable windows only) | "all_windows" | "all" (incl. doors)
  "mullion_mm": 90,                    // to-scale width of a double-hung center mullion
  "cover": {                           // optional; omit for no cover page
    "project": "6505-2026",            // all keys optional, shown only if present
    "color": "Matte black",            // frame color
    "ship_to": "123 St\nCity, ST",     // address; split on \n, else on commas
    "title": "Window Order — Cover Sheet"  // optional override
  },
  "columns": {
    "room": "Room",   // identity columns; null/omit if absent
    "id": "ID",
    "qty": "Qty",
    "style": "Style",
    "rooms": "Rooms", // optional; comma-separated room list → "Rooms:" line in title block
    "dims": {
      "ro_width":  {"col": "Opening width",  "unit": "cm"},  // null pair = window-size-only
      "ro_height": {"col": "Opening height", "unit": "cm"},
      "win_width":  {"col": "order width",  "unit": "cm"},  // null pair = RO-only
      "win_height": {"col": "order height", "unit": "cm"}
    }
  }
}
```

A dimension pair set to `null` is omitted from the drawing: only RO → RO-only page;
only window → single-rectangle page (used for size-only / by-group schedules). The
"… not provided" warning is **only** raised for a pair that was mapped but came back
empty — a deliberately `null`-mapped pair is silent. A row with no usable dimensions
at all is listed on a trailing "Skipped rows" page.

The `rooms` cell is split on `", "` and word-wrapped to fit the title-block width.
`"units": "mm"` switches all labels (ticks, scale bar, title block) to mm-only.

### Window-type symbols (`style` column)

The `style` value is matched (case-insensitive) to draw an operation symbol inside the
window unit, and the full text prints as a wrapped "Type:" line in the title block:

| `style` contains…                                  | Drawn symbol |
|----------------------------------------------------|--------------|
| `double hung` + (`mullion`/`pillar`/`side-by-side`/`side by side`) | to-scale center-mullion band (`mullion_mm`, default 90) splitting two double hungs (rail + ↕ each side) |
| `double hung` / `double-hung` / `single hung`      | horizontal meeting rail + ↕ operation arrows |
| `slider` / `sliding`                               | vertical meeting stile + ↔ operation arrows |
| `fold` / `accordion` (bi/tri/quad-fold, etc.)      | N equal panels + dashed accordion zig-zag + stack-side arrow + `N-PANEL FOLD · OUTSWING · STACK R/L` label |
| anything else / blank                              | plain unit (no symbol) |

For a folding door the panel count is parsed from the style text (`4-panel`, `quad-fold`, …; defaults to 4), the stack/hinge side from `left`/`right` (defaults to right), and the swing from `outswing`/`inswing`. Per-panel dividers are drawn to scale (panel width = unit width ÷ N).

In a `layout` assembly a `DOOR` panel's swing triangle is **dashed for outswing, solid for inswing** (`swing=in|out`), its base sits on the hinge stile (`hinge=L|R`), and an optional `note=<text>` adds a caption inside the leaf (e.g. `note=42in from hinge`; the note may contain spaces but not `:` `,` or `|`).

A **pivot door** is a `DOOR` panel with `pivot=<offset>` (offset in the same units as
the panel width) and `pivotside=L|R`: instead of a hinge triangle it draws a vertical
**pivot axis** (with round pivot markers top & bottom) set that offset in from the named
edge, plus swing triangles on both leaves (the long clear-passage side and the short
back-swing). Captioned `PIVOT` instead of `DOOR`.

Arrows are omitted when the unit is too small (< ~60 pt in that axis) to keep it clean.
Different types at the same size are different products — give each its own row.

### Multi-panel assemblies (`layout` column)

For a door/window assembly with several panels (e.g. an entry door with sidelites),
map a `layout` column whose cell is a left-to-right spec:

```
SL:24 | DOOR:107:hinge=R,swing=out | SL:24
```

Panels are `TYPE:width[:k=v,...]` separated by `|`. Widths are **relative** (any unit;
normalised to fill the unit rectangle, so they need not sum exactly). `TYPE`:
`SL`/`sidelite`/`fixed` → fixed glass (labeled `FIXED`); `DOOR` → a swing leaf drawn
with a dashed swing triangle whose full-height base sits on the hinge stile
(`hinge=L|R`, `swing=in|out`); anything else → plain glass. Mullion posts are drawn
between panels and the full spec prints as a wrapped `Layout:` line in the title block.
When `layout` is present it replaces the single-style operation symbol for that row.
Map the rough-opening pair to the structural opening and the window pair to the frame
outer size to show both nested (as on an entry-door page).

### Quote options (`quote` column)

A row whose `quote` cell is truthy (`yes`/`true`/`1`/`x`) is drawn as a normal page but
headed **"Quote Option"** with a "not included in order totals" banner, and is
**excluded** from every cover aggregation (unit/door/screen counts, area, manifest).
Use it for design alternatives where only one will ultimately be ordered.

Note: the "rough opening / window unit not provided" warning fires only on **partial**
data (one of a mapped pair present, the other blank); a row with both blank is treated
as intentionally that-shape-only and stays silent — so window-only rows can coexist
with RO-bearing rows in one sheet without spurious warnings.

### Cover sheet (`cover`)

When `cover` is present, a summary page is prepended (before page 1 of the schedule):

- **Header block:** `project`, `color`, `ship_to` — each printed only if provided.
  `ship_to` is split on newlines (`\n`), or on commas if it has none, one line each.
- **Order totals:** **total window units** and (if any) **total door units** are
  reported separately — a row counts as a *door* when its `style` contains `door`,
  `fold`, or `accordion`, otherwise it's a *window*. **Total area (all units)** =
  Σ (win_w × win_h × qty) across everything, shown in **sqm**, then converted to
  **sqft** (× 10.76391) and **sqin** (× 1550.0031). Area uses the window pair, falling
  back to the rough-opening pair when window dims are absent.
- **Count by window type / Count by door type:** units per `style` value
  (blank → `(unspecified)`), descending; doors get their own table when present.
- **`cover.extra_doors`** (optional list of `{"label","qty","type","w_mm","h_mm"}`):
  door units not drawn as an order row — e.g. a front entry whose final design is still
  being quoted. Each adds to **Total door units** and the count-by-door-type table. If
  `w_mm`/`h_mm` are given it is also area-weighted into **Total area** and gets an
  **Order-manifest** line (`type` is its manifest type label).
- **Screens required** (only when `screens` is set): total screen count = Σ `qty` over
  the units that take a screen. `"operable"` excludes fixed glass (style containing
  `picture`/`fixed`) and doors; `"all_windows"` includes fixed windows; `"all"` adds
  doors. Each screened unit's title block also shows a `Screens: Required (qty)` line.
- **Order manifest:** one row per line item — label, type, qty, size (mm), area (sqm).

Because totals are qty-weighted, **map the `qty` column** for accurate counts/area.
The cover does not carry a "page n of N" footer; that numbering covers the unit pages.

## Drawing layout (per page, Letter portrait 612×792 pt)

- Page header: "Window Schedule" + source file · sheet · page n of N.
- Drawing area (≈ x 90–542, y 200–712): rough opening = outer rectangle (1.6 pt),
  window unit = nested centred rectangle (1.0 pt) with a thin inset frame line.
- Dimension lines with 45° architectural ticks: RO width below, RO height left;
  window width above, window height right. Each carries ft-in-fraction + mm.
- Scale bar (nice round mm length ≤ ~45% of the opening) + printed `Scale 1:N`.
- Title block (bottom of page, **auto-height** — grows upward to fit its content):
  label (Room + ID), Qty/Scale, then Rough Opening / unit-size rows, Type, Layout,
  Screens, and a `Rooms w/ rough opening:` block. The unit-size row is labelled
  **"Door Unit"** when the row is a door (style has `door`/`fold`/`accordion`),
  otherwise **"Window Unit"**. The Rough Opening row is omitted when there is no RO.
  The `rooms` value (typically `Room ID (W×H mm), …` per member) prints as a wrapped
  `Rooms w/ rough opening:` list — but **only when the page has more than one element**
  (a single-element page just shows its own Rough Opening row, no rooms list). When a
  page has many elements there is no single rough opening, so the Rough Opening row
  reads **"see description below"** and the per-room openings are in that list.
  Per-window warnings print in red inside the block. When a rough opening is drawn the
  block also shows an **Opening area:** line (the RO area in sqm + sqft) — the glass/area
  needed to cover that opening. The drawing area above sizes itself to whatever the title
  block leaves.

Scale is chosen per page from `[5,10,15,20,25,30,40,50,60,75,100,125,150,200,250]` —
the smallest ratio (largest drawing) whose opening fits the drawing area.

## Troubleshooting

- **Missing packages** → `python3 -m pip install --quiet pandas openpyxl reportlab`.
- **Blank text when rasterizing** → use `magick`/Ghostscript, not `pdftoppm`; confirm
  text is present with `pdftotext out.pdf -`. The PDF itself is fine in normal viewers.
- **Wrong sizes drawn** → almost always a wrong `unit` in the mapping (e.g. cm vs mm).
  Re-inspect and confirm units before drawing.
- **No windows drawn** → the dimension columns mapped to empty/non-numeric data; check
  the `inspect` sample rows and re-map.
