#!/usr/bin/env python3
"""
window-schedule worker.

Two modes:
  inspect   Read a CSV/xlsx, print sheets + headers + sample rows + a PROPOSED
            column mapping as JSON (for the SKILL to show the user and confirm).
  draw      Read a confirmed mapping config (JSON) and render a to-scale
            architectural PDF: one window per page, rough opening drawn as an
            outer rectangle with the window unit nested inside, dimension lines
            on both, a scale bar, and a title block. Dimensions are labelled in
            feet-inch-fraction (primary) and millimetres (secondary).

Supported input dimension units (per column, in the mapping):
  mm | cm | in | ftin | ftin_frac
Numeric columns use mm/cm/in. String columns (e.g.  2'10"15/16 ) use ftin or
ftin_frac. All values are converted to millimetres internally.

Usage:
  python3 draw_windows.py inspect <file.csv|file.xlsx>
  python3 draw_windows.py draw    <config.json>
"""

import sys
import os
import re
import json
import math

# ---------------------------------------------------------------------------
# Dependency check
# ---------------------------------------------------------------------------
def _require_deps():
    missing = []
    try:
        import pandas  # noqa: F401
    except ImportError:
        missing.append("pandas")
    try:
        import openpyxl  # noqa: F401
    except ImportError:
        missing.append("openpyxl")
    try:
        import reportlab  # noqa: F401
    except ImportError:
        missing.append("reportlab")
    if missing:
        sys.stderr.write(
            "Missing Python packages: %s\n"
            "Install with:\n  python3 -m pip install --quiet %s\n"
            % (", ".join(missing), " ".join(missing))
        )
        sys.exit(3)


# ===========================================================================
# UNIT CONVERSION
# ===========================================================================
MM_PER_IN = 25.4

# Matches feet-inch strings:  2'11"8/16   2'11"   5'9"6/16   11 1/2"   11"
#   feet (optional), inches (optional), fraction num/den (optional)
_FTIN_RE = re.compile(
    r"""^\s*
        (?:(?P<ft>-?\d+(?:\.\d+)?)\s*['’f]\s*)?   # feet + ' or f
        (?:(?P<in>\d+(?:\.\d+)?)\s*)?              # whole inches
        (?:["”]\s*)?                                # optional inch mark
        (?:(?P<num>\d+)\s*/\s*(?P<den>\d+))?        # fraction n/d
        \s*["”]?\s*$
    """,
    re.VERBOSE,
)


def parse_ftin(text):
    """Parse a feet-inch(-fraction) string to inches. Returns float or None."""
    if text is None:
        return None
    s = str(text).strip()
    if not s:
        return None
    # Normalise common separators
    s = s.replace("ft", "'").replace("FT", "'")
    m = _FTIN_RE.match(s)
    if not m:
        return None
    ft = float(m.group("ft")) if m.group("ft") else 0.0
    inch = float(m.group("in")) if m.group("in") else 0.0
    if m.group("num") and m.group("den") and float(m.group("den")) != 0:
        inch += float(m.group("num")) / float(m.group("den"))
    return ft * 12.0 + inch


def to_mm(value, unit):
    """Convert a single raw cell value to millimetres given its declared unit."""
    if value is None:
        return None
    unit = (unit or "").lower()
    if unit in ("ftin", "ftin_frac", "ft-in", "ftinfrac"):
        inches = parse_ftin(value)
        return None if inches is None else inches * MM_PER_IN
    # numeric units
    try:
        x = float(value)
    except (TypeError, ValueError):
        # maybe a stringified number with units embedded; try ftin as a fallback
        inches = parse_ftin(value)
        return None if inches is None else inches * MM_PER_IN
    if math.isnan(x):
        return None
    if unit == "mm":
        return x
    if unit == "cm":
        return x * 10.0
    if unit in ("in", "inch", "inches"):
        return x * MM_PER_IN
    if unit in ("ft", "feet"):
        return x * 12.0 * MM_PER_IN
    # unknown unit -> assume mm
    return x


def mm_to_ftin_frac(mm, denom=16):
    """Format millimetres as feet-inch-fraction, e.g.  2'10"15/16  (fraction dropped if 0)."""
    if mm is None:
        return "—"
    total_in = mm / MM_PER_IN
    sign = "-" if total_in < 0 else ""
    total_in = abs(total_in)
    feet = int(total_in // 12)
    rem = total_in - feet * 12
    whole = int(rem)
    frac = rem - whole
    n = int(round(frac * denom))
    if n == denom:  # carry
        n = 0
        whole += 1
        if whole == 12:
            whole = 0
            feet += 1
    # reduce fraction
    if n != 0:
        g = math.gcd(n, denom)
        fr = "%d/%d" % (n // g, denom // g)
        return '%s%d\'%d"%s' % (sign, feet, whole, fr)
    return "%s%d'%d\"" % (sign, feet, whole)


def mm_to_str(mm):
    return "—" if mm is None else "%d mm" % int(round(mm))


# ===========================================================================
# INSPECT MODE
# ===========================================================================
UNIT_HINTS = [
    ("mm", "mm"), ("(mm)", "mm"),
    ("cm", "cm"), ("(cm)", "cm"),
    ("ft-in-16", "ftin_frac"), ("ftin", "ftin_frac"), ("ft-in", "ftin"),
    ("inch", "in"), ("(in)", "in"),
]


def guess_unit(header):
    h = header.lower()
    if "16)" in h or "ft-in" in h or "ftin" in h:
        return "ftin_frac"
    for token, unit in UNIT_HINTS:
        if token in h:
            return unit
    if re.search(r"\bin\b|_in$|\(in\)", h):
        return "in"
    return None  # unknown -> caller defaults / user confirms


def _norm(h):
    return re.sub(r"[^a-z0-9]+", " ", str(h).lower()).strip()


def propose_mapping(headers):
    """Heuristic column mapping from a list of header strings."""
    norm = {h: _norm(h) for h in headers}

    def find(*preds):
        for h in headers:
            n = norm[h]
            if all(p(n) for p in preds):
                return h
        return None

    has = lambda *words: (lambda n: all(w in n for w in words))
    no = lambda *words: (lambda n: not any(w in n for w in words))

    mapping = {}
    # identity columns
    mapping["room"] = find(has("room")) or (headers[0] if headers else None)
    mapping["id"] = (find(lambda n: n in ("id", "opening", "item", "mark"))
                     or find(has("opening"), no("width", "height", "ft"))
                     or find(has("item")))
    mapping["qty"] = find(has("qty")) or find(has("quantity"))
    mapping["style"] = find(lambda n: n == "style")

    # dimension columns (prefer "opening"/"rough" for RO, "order"/"unit"/"window" for the unit)
    def dim_unit(col):
        return guess_unit(col) if col else None

    ro_w = (find(has("opening", "width")) or find(has("rough", "width"))
            or find(has("ro", "width")))
    ro_h = (find(has("opening", "height")) or find(has("rough", "height"))
            or find(has("ro", "height")))
    win_w = (find(has("order", "width")) or find(has("unit", "width"))
             or find(has("window", "width")) or find(has("net", "width")))
    win_h = (find(has("order", "height")) or find(has("unit", "height"))
             or find(has("window", "height")) or find(has("net", "height")))

    cols = {}
    for key, col in (("ro_width", ro_w), ("ro_height", ro_h),
                     ("win_width", win_w), ("win_height", win_h)):
        if col:
            cols[key] = {"col": col, "unit": dim_unit(col) or "cm"}
        else:
            cols[key] = None
    mapping["dims"] = cols
    return mapping


def inspect(path):
    import pandas as pd
    ext = os.path.splitext(path)[1].lower()
    result = {"file": path, "type": ext, "sheets": []}

    def summarise(name, df):
        headers = [str(c) for c in df.columns]
        sample = []
        for _, row in df.head(4).iterrows():
            sample.append([("" if pd.isna(v) else v) for v in row.tolist()])
        return {
            "name": name,
            "n_rows": int(len(df)),
            "headers": headers,
            "sample_rows": sample,
            "proposed_mapping": propose_mapping(headers),
        }

    if ext in (".xlsx", ".xls", ".xlsm"):
        xls = pd.ExcelFile(path)
        best = None
        for name in xls.sheet_names:
            df = pd.read_excel(xls, sheet_name=name)
            df = df.dropna(axis=1, how="all")
            s = summarise(name, df)
            # score sheets by how many dim columns were found + presence of "width"/"height"
            score = sum(1 for v in s["proposed_mapping"]["dims"].values() if v)
            s["_score"] = score
            result["sheets"].append(s)
            if best is None or score > best["_score"]:
                best = s
        result["recommended_sheet"] = best["name"] if best else None
    else:
        df = pd.read_csv(path)
        df = df.dropna(axis=1, how="all")
        result["sheets"].append(summarise("(csv)", df))
        result["recommended_sheet"] = "(csv)"

    for s in result["sheets"]:
        s.pop("_score", None)
    print(json.dumps(result, indent=2, default=str))


# ===========================================================================
# DRAW MODE
# ===========================================================================
def load_rows(cfg):
    import pandas as pd
    path = cfg["file"]
    ext = os.path.splitext(path)[1].lower()
    if ext in (".xlsx", ".xls", ".xlsm"):
        df = pd.read_excel(path, sheet_name=cfg.get("sheet", 0))
    else:
        df = pd.read_csv(path)
    df.columns = [str(c) for c in df.columns]
    return df


def cell(row, col):
    import pandas as pd
    if not col or col not in row.index:
        return None
    v = row[col]
    if pd.isna(v):
        return None
    return v


def build_windows(cfg, df):
    dims = cfg["columns"]["dims"]
    idmap = cfg["columns"]
    windows = []
    skipped = []
    for _, row in df.iterrows():
        def dim_mm(key):
            spec = dims.get(key)
            if not spec:
                return None
            return to_mm(cell(row, spec["col"]), spec["unit"])

        ro_w = dim_mm("ro_width")
        ro_h = dim_mm("ro_height")
        win_w = dim_mm("win_width")
        win_h = dim_mm("win_height")

        room = cell(row, idmap.get("room"))
        wid = cell(row, idmap.get("id"))
        qty = cell(row, idmap.get("qty"))
        style = cell(row, idmap.get("style"))
        rooms = cell(row, idmap.get("rooms"))
        layout = cell(row, idmap.get("layout"))
        quote = cell(row, idmap.get("quote"))
        is_quote = str(quote).strip().lower() in ("1", "true", "yes", "y", "quote", "x") \
            if quote not in (None, "") else False

        label = " ".join(str(x) for x in (room, wid) if x not in (None, ""))

        # need at least one usable rectangle
        if not any(v and v > 0 for v in (ro_w, ro_h, win_w, win_h)):
            if label.strip():
                skipped.append((label, "no usable dimensions"))
            continue

        ro_mapped = bool(dims.get("ro_width") or dims.get("ro_height"))
        win_mapped = bool(dims.get("win_width") or dims.get("win_height"))
        warnings = []
        # warn only on PARTIAL data (one of a mapped pair present, the other not);
        # a row with both blank is intentionally that-shape-only and stays silent
        if ro_mapped and (bool(ro_w) != bool(ro_h)):
            warnings.append("rough opening not provided")
        if win_mapped and (bool(win_w) != bool(win_h)):
            warnings.append("window unit size not provided")
        if ro_w and win_w and win_w > ro_w + 0.5:
            warnings.append("window width exceeds rough opening")
        if ro_h and win_h and win_h > ro_h + 0.5:
            warnings.append("window height exceeds rough opening")

        windows.append({
            "room": room, "id": wid, "qty": qty, "style": style,
            "rooms": rooms, "layout": layout or "", "quote": is_quote,
            "label": label or "Window",
            "ro_w": ro_w, "ro_h": ro_h, "win_w": win_w, "win_h": win_h,
            "warnings": warnings,
        })
    return windows, skipped


# ---- geometry / scale ----
PT_PER_MM = 72.0 / 25.4
SCALES = [5, 10, 15, 20, 25, 30, 40, 50, 60, 75, 100, 125, 150, 200, 250]


def choose_scale(w_mm, h_mm, box_w_pt, box_h_pt):
    for s in SCALES:
        if w_mm * PT_PER_MM / s <= box_w_pt and h_mm * PT_PER_MM / s <= box_h_pt:
            return s
    return SCALES[-1]


def nice_bar_mm(span_mm):
    for cand in (1000, 500, 250, 200, 100, 50):
        if cand <= span_mm * 0.45:
            return cand
    return 50


def draw_style(c, style, x, y, w_pt, h_pt, mullion_pt=0.0, mullion_mm=0):
    """Draw a simple elevation operation symbol inside the window rect based on a
    style keyword: double hung (horizontal meeting rail + vertical arrows), slider
    (vertical meeting stile + horizontal arrows), or double-hung-with-mullion
    (a to-scale center mullion band splitting two double hungs). Unknown styles draw
    nothing. mullion_pt is the mullion width in points (drawn to scale); mullion_mm
    is its width in mm (for the label)."""
    s = (style or "").lower()
    cx = x + w_pt / 2.0
    cy = y + h_pt / 2.0

    def arrow_v(px, py, length):
        y0, y1 = py - length / 2, py + length / 2
        c.line(px, y0, px, y1)
        c.line(px, y1, px - 3, y1 - 4); c.line(px, y1, px + 3, y1 - 4)
        c.line(px, y0, px - 3, y0 + 4); c.line(px, y0, px + 3, y0 + 4)

    def arrow_h(px, py, length):
        x0, x1 = px - length / 2, px + length / 2
        c.line(x0, py, x1, py)
        c.line(x1, py, x1 - 4, py - 3); c.line(x1, py, x1 - 4, py + 3)
        c.line(x0, py, x0 + 4, py - 3); c.line(x0, py, x0 + 4, py + 3)

    def dh_half(left, right):
        hc = (left + right) / 2.0
        c.setLineWidth(0.8)
        c.line(left, cy, right, cy)                       # meeting rail
        if h_pt > 60:
            arrow_v(hc, cy + h_pt * 0.22, h_pt * 0.16)
            arrow_v(hc, cy - h_pt * 0.22, h_pt * 0.16)

    mullion = any(k in s for k in ("mullion", "side by side", "side-by-side", "pillar"))
    is_dh = ("double hung" in s) or ("double-hung" in s) or ("single hung" in s)
    is_slider = ("slider" in s) or ("sliding" in s)
    is_fold = any(k in s for k in ("fold", "accordion"))

    def fold_door():
        # Folding / bi-fold / multi-fold patio door, drawn in elevation:
        # the leaf is split into N equal panels with an accordion zig-zag (dashed)
        # showing the fold, an arrow toward the stack (hinge) side, and a label.
        # Panel count, stack side, and swing are parsed from the style text.
        words = {"bi": 2, "tri": 3, "quad": 4, "penta": 5, "hex": 6}
        n = 0
        m = re.search(r"(\d+)\s*[- ]?\s*(?:panel|fold|leaf|pane|section)", s)
        if m:
            n = int(m.group(1))
        else:
            for w_, v in words.items():
                if w_ + "fold" in s or w_ + "-fold" in s or (w_ + " fold") in s:
                    n = v; break
        if n < 2:
            n = 4
        stack_right = "left" not in s          # default stack/hinge to the right
        pw = w_pt / n                           # panel width
        c.setLineWidth(0.8)
        for i in range(1, n):                   # panel joints (hinge stiles)
            px = x + i * pw
            c.line(px, y, px, y + h_pt)
        # accordion zig-zag across the panels (dashed) — reads as "this folds"
        inset = min(h_pt * 0.14, pw * 0.5)
        top, bot = y + h_pt - inset, y + inset
        c.saveState()
        c.setDash(2, 2); c.setLineWidth(0.6)
        up = True
        for i in range(n):
            x0 = x + i * pw
            x1 = x + (i + 1) * pw
            if up:
                c.line(x0, bot, x1, top)
            else:
                c.line(x0, top, x1, bot)
            up = not up
        c.restoreState()
        # stack-direction arrow near the top, pointing to the hinge/stack side
        if w_pt > 60:
            ay = y + h_pt - inset * 0.55
            a_len = min(w_pt * 0.3, pw)
            if stack_right:
                xa, xb = cx + a_len / 2, cx - a_len / 2
            else:
                xa, xb = cx - a_len / 2, cx + a_len / 2
            c.setLineWidth(0.9)
            c.line(xb, ay, xa, ay)
            c.line(xa, ay, xa - 4 * (1 if stack_right else -1), ay - 3)
            c.line(xa, ay, xa - 4 * (1 if stack_right else -1), ay + 3)
        # label
        if h_pt > 50:
            c.setFont("Helvetica", 6); c.setFillGray(0.35)
            sw = "outswing" if "outswing" in s else ("inswing" if "inswing" in s else "")
            lbl = "%d-PANEL FOLD" % n
            if sw:
                lbl += " · " + sw.upper()
            lbl += " · STACK %s" % ("R" if stack_right else "L")
            c.drawCentredString(cx, y + inset * 0.35, lbl)
            c.setFillGray(0)

    c.saveState()
    if is_fold:
        fold_door()
    elif is_dh and mullion:
        half = (mullion_pt / 2.0) if mullion_pt and mullion_pt > 1 else 0.0
        if half:
            # mullion drawn to scale as a band (two stiles); sashes fill each side
            c.setLineWidth(1.0)
            c.rect(cx - half, y, mullion_pt, h_pt)        # the mullion post, to scale
            dh_half(x, cx - half)
            dh_half(cx + half, x + w_pt)
            if mullion_mm and h_pt > 70:                   # width label, rotated on the post
                c.saveState(); c.translate(cx, cy); c.rotate(90)
                c.setFont("Helvetica", 6); c.setFillGray(0.35)
                c.drawCentredString(0, -2, "%d mm mull" % round(mullion_mm))
                c.setFillGray(0); c.restoreState()
        else:
            c.setLineWidth(1.6)
            c.line(cx, y, cx, y + h_pt)                   # fallback: single line
            dh_half(x, cx)
            dh_half(cx, x + w_pt)
    elif is_dh:
        dh_half(x, x + w_pt)
    elif is_slider:
        c.setLineWidth(0.8)
        c.line(cx, y, cx, y + h_pt)                       # meeting stile
        if w_pt > 60:
            arrow_h(cx - w_pt * 0.22, cy, w_pt * 0.16)
            arrow_h(cx + w_pt * 0.22, cy, w_pt * 0.16)
    c.restoreState()


def draw_assembly(c, spec, x, y, w_pt, h_pt):
    """Draw a multi-panel door/window assembly elevation from a layout spec, e.g.
        'SL:24 | DOOR:107:hinge=R,swing=out | SL:24'
    Panels are listed left-to-right as TYPE:width[:k=v,...]; widths are normalised
    to fill w_pt (so they can be cm and need not sum exactly). TYPE: SL/sidelite/
    fixed → fixed glass; DOOR → swing leaf (hinge=L|R, swing=in|out shown as a
    dashed triangle whose full-height base sits on the hinge stile); anything else →
    plain glass. Mullion posts are drawn between panels and a small caption under
    each panel notes its type/width."""
    panels = []
    for raw in str(spec).split("|"):
        raw = raw.strip()
        if not raw:
            continue
        parts = [p.strip() for p in raw.split(":")]
        typ = parts[0].lower()
        try:
            wid = float(parts[1]) if len(parts) > 1 and parts[1] else 1.0
        except ValueError:
            wid = 1.0
        opts = {}
        if len(parts) > 2:
            for kv in parts[2].split(","):
                if "=" in kv:
                    k, v = kv.split("=", 1)
                    opts[k.strip().lower()] = v.strip().lower()
        panels.append((typ, wid, opts))
    if not panels:
        return
    total = sum(p[1] for p in panels) or 1.0
    cur = x
    c.saveState()
    for i, (typ, wid, opts) in enumerate(panels):
        pw = w_pt * (wid / total)
        px0, px1 = cur, cur + pw
        if i < len(panels) - 1:                       # mullion post between panels
            c.setLineWidth(1.2)
            c.line(px1, y, px1, y + h_pt)
        gi = min(4, pw * 0.10, h_pt * 0.04)           # glass inset
        if gi > 1:
            c.setLineWidth(0.4)
            c.rect(px0 + gi, y + gi, pw - 2 * gi, h_pt - 2 * gi)
        is_door = ("door" in typ) or typ in ("d", "leaf", "pivot")
        if is_door:
            sw = opts.get("swing", "")
            mid_y = y + h_pt / 2.0
            cxp = (px0 + px1) / 2.0
            is_pivot = ("pivot" in opts) or (typ == "pivot")
            c.saveState()
            if sw.startswith("in"):
                c.setDash()                           # solid = inswing
            else:
                c.setDash(2, 2)                       # dashed = outswing
            if is_pivot:
                # offset pivot: axis set in from one edge; both leaves rotate about it
                try:
                    off = float(opts.get("pivot"))
                except (TypeError, ValueError):
                    off = wid * 0.16
                ratio = max(0.05, min(0.95, (off / wid) if wid else 0.16))
                side = opts.get("pivotside", "r").lower()
                pvx = (px1 - pw * ratio) if side.startswith("r") else (px0 + pw * ratio)
                c.setLineWidth(1.0)
                c.line(pvx, y, pvx, y + h_pt)         # pivot axis
                c.setLineWidth(0.6)                   # swing triangles each side of pivot
                c.line(pvx, y + h_pt, px0, mid_y); c.line(pvx, y, px0, mid_y)
                c.line(pvx, y + h_pt, px1, mid_y); c.line(pvx, y, px1, mid_y)
                c.restoreState()
                c.saveState(); c.setLineWidth(0.8)    # pivot markers
                c.circle(pvx, y + h_pt - 6, 2, stroke=1, fill=0)
                c.circle(pvx, y + 6, 2, stroke=1, fill=0)
                c.restoreState()
                tag = "PIVOT"
            else:
                hinge = opts.get("hinge", "r")
                base_x = px1 if hinge.startswith("r") else px0
                apex_x = px0 if hinge.startswith("r") else px1
                c.setLineWidth(0.7)
                c.line(base_x, y + h_pt, apex_x, mid_y)
                c.line(base_x, y, apex_x, mid_y)
                c.restoreState()
                tag = "DOOR"
            if h_pt > 50:                              # caption + optional note
                c.setFont("Helvetica", 6); c.setFillGray(0.4)
                c.drawCentredString(cxp, mid_y + 3,
                                    tag + ((" · " + sw.upper()) if sw else ""))
                if opts.get("note"):
                    c.drawCentredString(cxp, mid_y - 6, opts["note"])
                c.setFillGray(0)
        else:
            if h_pt > 50:
                c.saveState()
                c.translate((px0 + px1) / 2.0, y + h_pt / 2.0); c.rotate(90)
                c.setFont("Helvetica", 6); c.setFillGray(0.45)
                c.drawCentredString(0, 0, "FIXED")
                c.setFillGray(0); c.restoreState()
        cur = px1
    c.restoreState()


def render(cfg, windows, skipped, out_path):
    from reportlab.lib.pagesizes import letter
    from reportlab.pdfgen import canvas
    from reportlab.lib.units import inch

    PAGE_W, PAGE_H = letter  # 612 x 792 pt (portrait)
    denom = int(cfg.get("fraction_denominator", 16))
    src = cfg.get("source_label") or os.path.basename(cfg["file"])
    sheet = cfg.get("sheet", "")
    units_mode = str(cfg.get("units", "dual")).lower()  # "mm" => mm-only labels

    def D(mm):
        """Return (primary, secondary) dimension labels per the units mode."""
        if units_mode == "mm":
            return (mm_to_str(mm), "")
        return (mm_to_ftin_frac(mm, denom), mm_to_str(mm))

    def is_door(win):
        st = (win.get("style") or "").lower()
        return ("door" in st) or ("fold" in st) or ("accordion" in st)

    def is_fixed(win):
        st = (win.get("style") or "").lower()
        return ("picture" in st) or ("fixed" in st)

    def needs_screen(win):
        """Whether a unit gets an insect screen, per cfg['screens']:
        omit/false → none; 'operable' → operable windows only (excludes fixed
        picture/fixed glass and doors); 'all_windows' → every window incl. fixed;
        'all' → windows + doors."""
        mode = str(cfg.get("screens", "") or "").lower().strip()
        if mode in ("", "false", "none", "off", "0"):
            return False
        if is_door(win):
            return mode == "all"
        if is_fixed(win):
            return mode in ("all_windows", "all")
        return mode in ("operable", "all_windows", "all")

    c = canvas.Canvas(out_path, pagesize=letter)

    # drawing area (leave room for dimension labels + title block)
    DRAW_L, DRAW_R = 90, PAGE_W - 70
    DRAW_B, DRAW_T = 200, PAGE_H - 80
    box_w = DRAW_R - DRAW_L
    box_h = DRAW_T - DRAW_B
    # reserve interior padding for dimension lines/labels
    pad = 60
    inner_w = box_w - 2 * pad
    inner_h = box_h - 2 * pad
    cx = (DRAW_L + DRAW_R) / 2.0
    cy = (DRAW_B + DRAW_T) / 2.0

    TICK = 5

    def tick(x, y, ang):
        a = math.radians(ang)
        dx, dy = math.cos(a) * TICK, math.sin(a) * TICK
        c.line(x - dx, y - dy, x + dx, y + dy)

    def hdim(x1, x2, y, feat_y, primary, secondary):
        """Horizontal dimension between x1,x2 placed at height y; extension lines from feat_y."""
        c.setLineWidth(0.4)
        c.line(x1, feat_y, x1, y)
        c.line(x2, feat_y, x2, y)
        c.line(x1, y, x2, y)
        tick(x1, y, 45)
        tick(x2, y, 45)
        c.setFont("Helvetica", 8)
        mid = (x1 + x2) / 2
        c.drawCentredString(mid, y + 3, primary)
        c.setFont("Helvetica", 6.5)
        c.setFillGray(0.35)
        c.drawCentredString(mid, y - 8, secondary)
        c.setFillGray(0)

    def vdim(y1, y2, x, feat_x, primary, secondary):
        c.setLineWidth(0.4)
        c.line(feat_x, y1, x, y1)
        c.line(feat_x, y2, x, y2)
        c.line(x, y1, x, y2)
        tick(x, y1, 45)
        tick(x, y2, 45)
        mid = (y1 + y2) / 2
        c.saveState()
        c.translate(x, mid)
        c.rotate(90)
        c.setFont("Helvetica", 8)
        c.drawCentredString(0, 3, primary)
        c.setFont("Helvetica", 6.5)
        c.setFillGray(0.35)
        c.drawCentredString(0, -8, secondary)
        c.restoreState()
        c.setFillGray(0)

    # --- optional cover sheet (first page) ---
    def render_cover(cover):
        SQFT_PER_SQM = 10.76391041671
        SQIN_PER_SQM = 1550.0031000062

        def as_qty(win):
            try:
                return max(1, int(round(float(win["qty"]))))
            except (TypeError, ValueError):
                return 1

        def unit_area_mm2(win):
            ww = win["win_w"] or win["ro_w"]
            hh = win["win_h"] or win["ro_h"]
            return (ww * hh) if (ww and hh) else 0.0

        def count_by_style(items):
            d = {}
            for w in items:
                key = w["style"] if w["style"] not in (None, "") else "(unspecified)"
                d[key] = d.get(key, 0) + as_qty(w)
            return sorted(d.items(), key=lambda kv: (-kv[1], kv[0]))

        # quote-option rows are drawn but excluded from all order aggregations
        order_windows = [w for w in windows if not w.get("quote")]
        win_items = [w for w in order_windows if not is_door(w)]
        door_items = [w for w in order_windows if is_door(w)]
        total_window_units = sum(as_qty(w) for w in win_items)
        total_door_units = sum(as_qty(w) for w in door_items)
        area_mm2 = sum(unit_area_mm2(w) * as_qty(w) for w in order_windows)
        by_window = count_by_style(win_items)
        by_door = count_by_style(door_items)
        # extra door units not represented by a drawn order row (e.g. a front entry
        # whose final design is still being quoted); a w_mm/h_mm makes it area-weighted
        # and gives it an Order-manifest line
        extra_rows = []
        for ed in (cover.get("extra_doors") or []):
            try:
                q = max(1, int(ed.get("qty", 1)))
            except (TypeError, ValueError):
                q = 1
            total_door_units += q
            by_door.append((ed.get("label", "(door)"), q))
            ew, eh = ed.get("w_mm"), ed.get("h_mm")
            ea = (ew * eh / 1.0e6 * q) if (ew and eh) else 0.0
            area_mm2 += (ew * eh * q) if (ew and eh) else 0.0
            extra_rows.append((ed.get("label", "(door)"),
                               ed.get("type", "(design TBD)"), q, ew, eh, ea))
        by_door = sorted(by_door, key=lambda kv: (-kv[1], kv[0]))
        sqm = area_mm2 / 1.0e6
        total_screens = sum(as_qty(w) for w in order_windows if needs_screen(w))

        M = 54
        y = PAGE_H - 60
        c.setFont("Helvetica-Bold", 18)
        c.drawString(M, y, cover.get("title") or "Window Order — Cover Sheet")
        y -= 15
        c.setFont("Helvetica", 9); c.setFillGray(0.4)
        c.drawString(M, y, "%s%s" % (src, ("  ·  " + str(sheet)) if sheet else ""))
        c.setFillGray(0); y -= 26

        for lbl, key in (("Project #", "project"), ("Frame color", "color")):
            if cover.get(key) not in (None, ""):
                c.setFont("Helvetica-Bold", 10); c.drawString(M, y, lbl + ":")
                c.setFont("Helvetica", 10); c.drawString(M + 95, y, str(cover[key]))
                y -= 15
        if cover.get("ship_to") not in (None, ""):
            c.setFont("Helvetica-Bold", 10); c.drawString(M, y, "Ship to:")
            c.setFont("Helvetica", 10)
            addr = str(cover["ship_to"])
            parts = addr.split("\n") if "\n" in addr else [s.strip() for s in addr.split(",")]
            for ln in parts:
                c.drawString(M + 95, y, ln); y -= 13
        y -= 16

        def section(title):
            nonlocal y
            c.setFont("Helvetica-Bold", 12); c.drawString(M, y, title); y -= 5
            c.setLineWidth(0.6); c.line(M, y, PAGE_W - M, y); y -= 16

        section("Order totals")
        c.setFont("Helvetica", 10)
        rows = [("Total window units", "%d" % total_window_units)]
        if total_door_units:
            rows.append(("Total door units", "%d" % total_door_units))
        if total_screens:
            rows.append(("Screens required", "%d" % total_screens))
        rows += [
            ("Total area (all units)", "%.3f sqm" % sqm),
            ("", "= %.2f sqft   ·   %.0f sqin" % (sqm * SQFT_PER_SQM, sqm * SQIN_PER_SQM)),
        ]
        for lbl, val in rows:
            if lbl:
                c.setFont("Helvetica-Bold", 10); c.drawString(M, y, lbl)
            c.setFont("Helvetica", 10); c.drawString(M + 175, y, val); y -= 15
        y -= 12

        def count_table(title, data):
            nonlocal y
            section(title)
            c.setFont("Helvetica-Bold", 9)
            c.drawString(M, y, "Type"); c.drawRightString(M + 380, y, "Units"); y -= 13
            c.setFont("Helvetica", 9)
            for style, cnt in data:
                c.drawString(M, y, str(style)); c.drawRightString(M + 380, y, str(cnt)); y -= 13
            y -= 14

        count_table("Count by window type", by_window)
        if by_door:
            count_table("Count by door type", by_door)

        section("Order manifest (by size)")
        c.setFont("Helvetica-Bold", 8)
        c.drawString(M, y, "Item"); c.drawString(M + 130, y, "Type")
        c.drawRightString(M + 330, y, "Qty"); c.drawString(M + 345, y, "Size (mm)")
        c.drawRightString(PAGE_W - M, y, "Area (sqm)"); y -= 12
        c.setFont("Helvetica", 8)
        for w in order_windows:
            if y < 60:
                break
            typ = (w["style"] or "")
            if len(typ) > 26:
                typ = typ[:25] + "…"
            ww = w["win_w"] or w["ro_w"] or 0
            hh = w["win_h"] or w["ro_h"] or 0
            a = (ww * hh) / 1.0e6 * as_qty(w)
            c.drawString(M, y, str(w["label"])[:22])
            c.drawString(M + 130, y, typ)
            c.drawRightString(M + 330, y, str(as_qty(w)))
            c.drawString(M + 345, y, "%d × %d" % (round(ww), round(hh)))
            c.drawRightString(PAGE_W - M, y, "%.3f" % a)
            y -= 12
        for label, typ, q, ew, eh, ea in extra_rows:
            if y < 60:
                break
            t = typ if len(typ) <= 26 else typ[:25] + "…"
            c.drawString(M, y, str(label)[:22])
            c.drawString(M + 130, y, t)
            c.drawRightString(M + 330, y, str(q))
            c.drawString(M + 345, y, ("%d × %d" % (round(ew), round(eh))) if (ew and eh) else "design TBD")
            c.drawRightString(PAGE_W - M, y, ("%.3f" % ea) if ea else "—")
            y -= 12

    cover_cfg = cfg.get("cover")
    if cover_cfg:
        render_cover(cover_cfg if isinstance(cover_cfg, dict) else {})
        c.showPage()

    n = len(windows)

    def wrap_tokens(text, sep, font, size, avail):
        out = []; buf = ""
        for tok in str(text).split(sep):
            trial = (buf + sep + tok) if buf else tok
            if buf and c.stringWidth(trial, font, size) > avail:
                out.append(buf); buf = tok
            else:
                buf = trial
        if buf:
            out.append(buf)
        return out

    for i, w in enumerate(windows, 1):
        has_ro = bool(w["ro_w"] and w["ro_h"])
        has_win = bool(w["win_w"] and w["win_h"])
        # rough opening is always described in the title block; drawing it on the
        # elevation is optional (cfg['draw_rough_opening'], default on)
        draw_ro = has_ro and cfg.get("draw_rough_opening", True)
        unit_label = "Door Unit" if is_door(w) else "Window Unit"

        # ---- assemble + pre-measure the title-block content (dynamic height) ----
        tb_l, tb_r = 54, PAGE_W - 54
        tb_b = 48
        LBL_X, TXT_X = tb_l + 8, tb_l + 50
        avail_inline = tb_r - 8 - TXT_X
        avail_full = tb_r - 8 - (tb_l + 12)

        # how many distinct elements this page represents (entries in the rooms list)
        rooms_text = w.get("rooms") or ""
        n_elem = (rooms_text.count(", ") + 1) if rooms_text.strip() else 0
        has_many = n_elem > 1

        # top rows: a Rough Opening line is always shown — the actual dim for a single
        # element, or "see description below" when there are many (each listed in rooms)
        top_rows = []
        if has_ro:
            top_rows.append(("dim", "Rough Opening", w["ro_w"], w["ro_h"]))
        elif has_many:
            top_rows.append(("text", "Rough Opening:", "see description below"))
        if has_win:
            top_rows.append(("dim", unit_label, w["win_w"], w["win_h"]))
        # area to cover the opening (shown whenever a rough opening is drawn)
        if has_ro:
            a_sqm = (w["ro_w"] * w["ro_h"]) / 1.0e6
            top_rows.append(("text", "Opening area:",
                             "%.3f sqm  (%.1f sqft)" % (a_sqm, a_sqm * 10.76391041671)))

        inline_rows = []
        if w["style"] not in (None, ""):
            inline_rows.append(("Type:", wrap_tokens(w["style"], " ", "Helvetica", 8, avail_inline)))
        if w.get("layout"):
            inline_rows.append(("Layout:", wrap_tokens(w["layout"], "|", "Helvetica", 8, avail_inline)))
        if needs_screen(w):
            try:
                scr = max(1, int(float(w["qty"])))
            except (TypeError, ValueError):
                scr = 1
            inline_rows.append(("Screens:",
                                ["Required (%d)" % scr if scr > 1 else "Required"]))

        # the rooms-with-opening block is only meaningful when there are several
        rooms_label, rooms_lines = None, []
        if has_many:
            rooms_label = "Rooms w/ rough opening:"
            rooms_lines = wrap_tokens(rooms_text, ", ", "Helvetica", 7.5, avail_full)

        consumed = sum(16 if r[0] == "dim" else 14 for r in top_rows)
        for _, lines in inline_rows:
            consumed += 10 * len(lines) + 4
        if rooms_label:
            consumed += 12 + 9 * len(rooms_lines) + 4
        warn_h = 14 if w["warnings"] else 0
        tb_t = tb_b + max(46 + consumed + warn_h, 70)

        # ---- per-page drawing geometry (above the title block) ----
        legend_y = tb_t + 8
        bar_y = tb_t + 20
        draw_b = tb_t + 36
        draw_t = PAGE_H - (74 if w.get("quote") else 64)
        cy = (draw_b + draw_t) / 2.0
        inner_h = (draw_t - draw_b) - 2 * pad

        ow = (w["ro_w"] if draw_ro else None) or w["win_w"]
        oh = (w["ro_h"] if draw_ro else None) or w["win_h"]
        scale = choose_scale(ow, oh, inner_w, inner_h)
        f = PT_PER_MM / scale

        ro_w_pt = ow * f
        ro_h_pt = oh * f
        ro_l, ro_b = cx - ro_w_pt / 2, cy - ro_h_pt / 2
        ro_r, ro_t = ro_l + ro_w_pt, ro_b + ro_h_pt

        # --- page header ---
        c.setFont("Helvetica-Bold", 12)
        c.drawString(54, PAGE_H - 48,
                     "Quote Option" if w.get("quote") else "Window Schedule")
        c.setFont("Helvetica", 8)
        c.setFillGray(0.4)
        c.drawRightString(PAGE_W - 54, PAGE_H - 48,
                          "%s%s   page %d of %d" %
                          (src, ("  ·  " + str(sheet)) if sheet else "", i, n))
        c.setFillGray(0)
        if w.get("quote"):
            c.setFont("Helvetica-Bold", 8); c.setFillColorRGB(0.10, 0.35, 0.65)
            c.drawString(54, PAGE_H - 62, "▶ QUOTE OPTION — not included in order totals")
            c.setFillGray(0)

        # --- rough opening rectangle (outer) ---
        if draw_ro:
            c.setLineWidth(1.6)
            c.rect(ro_l, ro_b, ro_w_pt, ro_h_pt)
            hdim(ro_l, ro_r, ro_b - 30, ro_b, *D(w["ro_w"]))
            vdim(ro_b, ro_t, ro_l - 32, ro_l, *D(w["ro_h"]))

        # --- window/door unit (nested, centred) ---
        if has_win:
            win_w_pt = w["win_w"] * f
            win_h_pt = w["win_h"] * f
            win_l, win_b = cx - win_w_pt / 2, cy - win_h_pt / 2
            win_r, win_t = win_l + win_w_pt, win_b + win_h_pt
            c.setLineWidth(1.0)
            c.rect(win_l, win_b, win_w_pt, win_h_pt)
            c.setLineWidth(0.4)
            inset = min(4, win_w_pt * 0.06, win_h_pt * 0.06)
            if inset > 1:
                c.rect(win_l + inset, win_b + inset,
                       win_w_pt - 2 * inset, win_h_pt - 2 * inset)
            if w.get("layout"):
                draw_assembly(c, w["layout"], win_l, win_b, win_w_pt, win_h_pt)
            elif w.get("style"):
                mull_mm = float(cfg.get("mullion_mm", 90))
                draw_style(c, w["style"], win_l, win_b, win_w_pt, win_h_pt,
                           mull_mm * f, mull_mm)
            hdim(win_l, win_r, ro_t + 30 if draw_ro else win_t + 24,
                 win_t, *D(w["win_w"]))
            vdim(win_b, win_t,
                 (ro_r + 34) if draw_ro else win_r + 26,
                 win_r, *D(w["win_h"]))

        # --- scale bar (just above the title block) ---
        bar_mm = nice_bar_mm(ow)
        bar_pt = bar_mm * f
        c.setLineWidth(1.0)
        c.line(DRAW_L, bar_y, DRAW_L + bar_pt, bar_y)
        tick(DRAW_L, bar_y, 90)
        tick(DRAW_L + bar_pt, bar_y, 90)
        c.setFont("Helvetica", 7)
        if units_mode == "mm":
            bar_label = "%d mm   Scale 1:%d" % (bar_mm, scale)
        else:
            bar_label = "%d mm  (%s)   Scale 1:%d" % (bar_mm, mm_to_ftin_frac(bar_mm, denom), scale)
        c.drawString(DRAW_L + bar_pt + 5, bar_y - 2, bar_label)

        # --- legend ---
        c.setFont("Helvetica", 6.5)
        c.setFillGray(0.4)
        c.drawString(DRAW_L, legend_y,
                     "outer = rough opening   ·   inner = unit"
                     if draw_ro else "unit (order size)")
        c.setFillGray(0)

        # --- title block (dynamic height) ---
        c.setLineWidth(1.0)
        c.rect(tb_l, tb_b, tb_r - tb_l, tb_t - tb_b)
        c.line(tb_l, tb_t - 24, tb_r, tb_t - 24)
        c.setFont("Helvetica-Bold", 12)
        c.drawString(LBL_X, tb_t - 18, w["label"])
        c.setFont("Helvetica", 9)
        meta = []
        if w["qty"] not in (None, ""):
            try:
                meta.append("Qty: %d" % int(float(w["qty"])))
            except (TypeError, ValueError):
                meta.append("Qty: %s" % w["qty"])
        meta.append("Scale 1:%d" % scale)
        c.drawRightString(tb_r - 8, tb_t - 18, "    ".join(meta))

        def dim_line(y, lbl, ww, hh):
            c.setFont("Helvetica-Bold", 9)
            c.drawString(LBL_X, y, lbl)
            if units_mode == "mm":
                c.setFont("Helvetica", 9)
                c.drawString(tb_l + 130, y, "%s  ×  %s" % (mm_to_str(ww), mm_to_str(hh)))
            else:
                c.setFont("Helvetica", 9)
                c.drawString(tb_l + 130, y, "%s  ×  %s" %
                             (mm_to_ftin_frac(ww, denom), mm_to_ftin_frac(hh, denom)))
                c.setFont("Helvetica", 8); c.setFillGray(0.4)
                c.drawString(tb_l + 300, y, "( %s × %s )" % (mm_to_str(ww), mm_to_str(hh)))
                c.setFillGray(0)

        yrow = tb_t - 42
        for r in top_rows:
            if r[0] == "dim":
                dim_line(yrow, r[1], r[2], r[3]); yrow -= 16
            else:
                c.setFont("Helvetica-Bold", 9); c.drawString(LBL_X, yrow, r[1])
                c.setFont("Helvetica-Oblique", 9); c.setFillGray(0.35)
                c.drawString(tb_l + 130, yrow, r[2]); c.setFillGray(0)
                yrow -= 16
        for lbl, lines in inline_rows:
            c.setFont("Helvetica-Bold", 8); c.drawString(LBL_X, yrow, lbl)
            c.setFont("Helvetica", 8)
            for ln in lines:
                c.drawString(TXT_X, yrow, ln); yrow -= 10
            yrow -= 4
        if rooms_label:
            c.setFont("Helvetica-Bold", 8); c.drawString(LBL_X, yrow, rooms_label); yrow -= 11
            c.setFont("Helvetica", 7.5)
            for ln in rooms_lines:
                c.drawString(tb_l + 12, yrow, ln); yrow -= 9
            yrow -= 4

        if w["warnings"]:
            c.setFont("Helvetica-Oblique", 8)
            c.setFillColorRGB(0.7, 0.2, 0.1)
            c.drawString(LBL_X, tb_b + 6, "⚠ " + "; ".join(w["warnings"]))
            c.setFillGray(0)

        c.showPage()

    # --- trailing notes page for skipped rows ---
    if skipped:
        c.setFont("Helvetica-Bold", 12)
        c.drawString(54, PAGE_H - 60, "Skipped rows (no usable dimensions)")
        c.setFont("Helvetica", 9)
        y = PAGE_H - 90
        for lbl, why in skipped:
            c.drawString(60, y, "• %s — %s" % (lbl, why))
            y -= 14
            if y < 60:
                c.showPage()
                y = PAGE_H - 60
        c.showPage()

    c.save()


def draw(config_path):
    with open(config_path) as fh:
        cfg = json.load(fh)
    df = load_rows(cfg)
    windows, skipped = build_windows(cfg, df)
    if not windows:
        sys.stderr.write("No drawable windows found. Check the column mapping.\n")
        sys.exit(2)
    out = cfg.get("output")
    if not out:
        stem = os.path.splitext(os.path.basename(cfg["file"]))[0]
        out = os.path.join(os.path.dirname(os.path.abspath(cfg["file"])),
                           stem + "_window_schedule.pdf")
    render(cfg, windows, skipped, out)
    print(json.dumps({
        "output": out,
        "windows_drawn": len(windows),
        "skipped": skipped,
    }, indent=2))


# ===========================================================================
def main():
    if len(sys.argv) < 3:
        sys.stderr.write(__doc__)
        sys.exit(1)
    _require_deps()
    mode, arg = sys.argv[1], sys.argv[2]
    if mode == "inspect":
        inspect(arg)
    elif mode == "draw":
        draw(arg)
    else:
        sys.stderr.write("Unknown mode '%s'. Use 'inspect' or 'draw'.\n" % mode)
        sys.exit(1)


if __name__ == "__main__":
    main()
