#!/usr/bin/env python3
"""
Batch evidence matcher (Workflow C, automated).

For each PDF/PNG/JPG in /workspace/processing/incoming-temp/, parse the
filename (YYMMDD $amount Vendor ...), search the data tab for matching
rows (amount ±$0.01, date ±7 days, vendor token in Description/site),
and:
  - 1 unique match  → write filename to R column, move file to processed/, log status="processed"
  - 0 matches       → leave file, log status="no_match"
  - >1 matches      → leave file, log status="pending_review", report candidates

Approximate amounts (~$X) widen the tolerance to ±15% and require vendor
token match.

Skips Ramp CSVs (Workflow A) and ALLREC CSVs (Workflow D).
"""

import argparse
import datetime
import json
import re
import shutil
import sys
import zipfile
from pathlib import Path
from xml.etree import ElementTree as ET

INCOMING_TEMP = Path("/workspace/processing/incoming-temp")
PROCESSED = Path("/workspace/processing/processed")
SPREADSHEET = Path("/workspace/processing/spreadsheet/Finance.xlsx")
TRACKING = Path("/workspace/fintool/.claude/skills/fileclerk-data/data/processed_files.json")
DATA_SHEET_XML = "xl/worksheets/sheet33.xml"
SHARED_STRINGS_XML = "xl/sharedStrings.xml"

EXCEL_EPOCH = datetime.date(1899, 12, 30)


def col_letter_to_index(letter):
    n = 0
    for c in letter:
        n = n * 26 + (ord(c) - ord("A") + 1)
    return n - 1


def index_to_col_letter(idx):
    s = ""
    n = idx + 1
    while n:
        n, r = divmod(n - 1, 26)
        s = chr(r + ord("A")) + s
    return s


def serial_to_date(s):
    return EXCEL_EPOCH + datetime.timedelta(days=int(s))


def date_to_serial(d):
    return (d - EXCEL_EPOCH).days


# ---------- xlsx loading ----------

def load_shared_strings(zf):
    try:
        with zf.open(SHARED_STRINGS_XML) as f:
            tree = ET.parse(f)
    except KeyError:
        return []
    ns = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
    return [
        "".join(t.text or "" for t in si.iter(f"{ns}t"))
        for si in tree.getroot().iter(f"{ns}si")
    ]


def load_data_rows(zf, strings):
    """Return list of dicts: {row, C: posting_date_serial, D: desc, E: amount, H: site, R: receipt}."""
    ns = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
    rows = {}
    with zf.open(DATA_SHEET_XML) as f:
        tree = ET.parse(f)
    for row_el in tree.getroot().iter(f"{ns}row"):
        r = int(row_el.get("r"))
        for c in row_el.iter(f"{ns}c"):
            ref = c.get("r")
            m = re.match(r"^([A-Z]+)\d+$", ref)
            if not m:
                continue
            col = m.group(1)
            if col not in {"B", "C", "D", "E", "H", "I", "K", "R"}:
                continue
            t = c.get("t")
            v_el = c.find(f"{ns}v")
            if v_el is None:
                val = ""
            elif t == "s":
                try:
                    val = strings[int(v_el.text)]
                except Exception:
                    val = ""
            else:
                val = v_el.text or ""
            rows.setdefault(r, {})[col] = val
    return rows


# ---------- filename parsing ----------

FILENAME_RE = re.compile(
    r"^(?P<date>\d{6})\s+(?:(?P<approx>~)?\$(?P<amount>-?[\d,.\-]+)\s+)?(?P<vendor>[A-Za-z][A-Za-z0-9]*)"
)


def parse_filename(fn):
    m = FILENAME_RE.match(fn)
    if not m:
        return None
    date_str = m.group("date")
    yy, mm, dd = int(date_str[:2]), int(date_str[2:4]), int(date_str[4:6])
    year = 2000 + yy if yy < 70 else 1900 + yy
    try:
        d = datetime.date(year, mm, dd)
    except ValueError:
        return None
    amount_str = m.group("amount")
    amount = None
    if amount_str:
        try:
            amount = float(amount_str.replace(",", ""))
        except ValueError:
            amount = None
    return {
        "date": d,
        "amount": amount,
        "approx": bool(m.group("approx")),
        "vendor": m.group("vendor"),
    }


# ---------- matching ----------

def find_matches(rows, parsed):
    """Return list of (row_num, row_dict) candidates."""
    target_date = parsed["date"]
    target_amount = parsed["amount"]
    vendor = parsed["vendor"].lower()
    is_approx = parsed["approx"]

    if target_amount is None:
        return []  # need amount to match

    date_window = 10 if is_approx else 7
    if is_approx:
        amount_lo = target_amount * 0.85
        amount_hi = target_amount * 1.15
    else:
        amount_lo = target_amount - 0.01
        amount_hi = target_amount + 0.01

    candidates = []
    for r, row in rows.items():
        c_serial = row.get("C", "")
        e_amount = row.get("E", "")
        if not c_serial or not e_amount:
            continue
        try:
            posting_date = serial_to_date(float(c_serial))
            amount = float(e_amount)
        except Exception:
            continue
        # Compare absolute values for amount (PDFs use absolute, data tab is signed)
        if not (amount_lo <= abs(amount) <= amount_hi or amount_lo <= amount <= amount_hi):
            continue
        if abs((posting_date - target_date).days) > date_window:
            continue
        # Vendor token must appear in D, H, or I
        haystack = " ".join([
            row.get("D", "") or "",
            row.get("H", "") or "",
            row.get("I", "") or "",
        ]).lower()
        if vendor not in haystack:
            # Try common vendor-token variants
            variants = {
                "perkinscoie": ["perkins", "coie", "perkins coie"],
                "tlip": ["tlip", "transamerica", "trans am", "transam"],
                "datafoundry": ["data foundry", "datafoundry", "datafound"],
                "cloudhealth": ["cloudhealth", "cloud health"],
                "phonespeak": ["phonespeak", "phone speak"],
                "hurricaneelectric": ["hurricane", "he.net"],
                "kastnergravelle": ["kastner", "gravelle"],
                "venetianexpo": ["venetian"],
                "perkins": ["perkins", "coie"],
                "synctix": ["synctix"],
                "murgitroyd": ["murgitroyd"],
                "firabarcelona": ["fira", "barcelona"],
                "nuernbergmesse": ["nuernberg", "messe"],
                "pingdom": ["pingdom"],
                "cogent": ["cogent"],
                "google": ["google"],
                "apple": ["apple"],
                "cloudflare": ["cloudflare"],
                "onpay": ["onpay"],
                "pimoroni": ["pimoroni"],
                "tlaloc": ["tlaloc"],
                "uspto": ["uspto", "patent"],
                "gsma": ["gsma"],
            }
            tokens = variants.get(vendor, [vendor])
            if not any(t in haystack for t in tokens):
                continue
        candidates.append((r, row))
    return candidates


# ---------- xlsx writing ----------

def write_receipt_to_data_tab(updates):
    """updates: list of (row_num, filename) — write filename to column R of given row in sheet33.xml."""
    if not updates:
        return
    # Read sheet xml as bytes for surgical edit
    import io
    backup_dir = SPREADSHEET.parent / "backups"
    backup_dir.mkdir(exist_ok=True)
    ts = datetime.datetime.utcnow().strftime("%Y%m%dT%H%M%SZ")
    backup_path = backup_dir / f"Finance_pre_workflowc_{ts}.xlsx"
    shutil.copy2(SPREADSHEET, backup_path)
    print(f"Backup: {backup_path}")

    # Load shared strings, append filenames, get indices
    with zipfile.ZipFile(SPREADSHEET) as zf:
        with zf.open(SHARED_STRINGS_XML) as f:
            ss_bytes = f.read()
    ss_text = ss_bytes.decode("utf-8")

    # Find or add each filename to shared strings
    ns = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
    tree = ET.ElementTree(ET.fromstring(ss_text))
    root = tree.getroot()
    existing = {}
    for i, si in enumerate(root.iter(f"{ns}si")):
        text = "".join(t.text or "" for t in si.iter(f"{ns}t"))
        if text:
            existing[text] = i
    next_idx = len(existing)
    name_to_idx = {}
    for _, fn in updates:
        if fn in existing:
            name_to_idx[fn] = existing[fn]
        elif fn in name_to_idx:
            pass
        else:
            name_to_idx[fn] = len(existing) + len([k for k in name_to_idx if name_to_idx[k] >= len(existing)])

    # Append new strings
    new_strings = []
    for fn in name_to_idx:
        if fn not in existing:
            new_strings.append(fn)
    # Rewrite shared strings
    if new_strings:
        si_template = '<si><t xml:space="preserve">{}</t></si>'
        # Simpler approach: parse, append, serialize
        tree2 = ET.ElementTree(ET.fromstring(ss_bytes.decode("utf-8")))
        root2 = tree2.getroot()
        for fn in new_strings:
            si = ET.SubElement(root2, f"{ns}si")
            t = ET.SubElement(si, f"{ns}t")
            t.text = fn
            t.set("{http://www.w3.org/XML/1998/namespace}space", "preserve")
        # Update count attributes
        n_si = len(list(root2.iter(f"{ns}si")))
        root2.set("count", str(n_si))
        root2.set("uniqueCount", str(n_si))
        ET.register_namespace("", "http://schemas.openxmlformats.org/spreadsheetml/2006/main")
        new_ss_bytes = b'<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n' + ET.tostring(root2)
        # Build name_to_idx fresh from updated tree
        name_to_idx = {}
        for i, si in enumerate(root2.iter(f"{ns}si")):
            text = "".join(t.text or "" for t in si.iter(f"{ns}t"))
            if text:
                name_to_idx[text] = i
    else:
        new_ss_bytes = ss_bytes

    # Read sheet33 bytes
    with zipfile.ZipFile(SPREADSHEET) as zf:
        with zf.open(DATA_SHEET_XML) as f:
            sheet_bytes = f.read()
    sheet_text = sheet_bytes.decode("utf-8")

    # For each update, find the <row r="N"> ... </row> and insert/replace <c r="RN">
    for row_num, fn in updates:
        ss_idx = name_to_idx[fn]
        # Find <row r="N" ...> block
        row_pattern = re.compile(rf'(<row [^>]*r="{row_num}"[^>]*>)(.*?)(</row>)', re.DOTALL)
        m = row_pattern.search(sheet_text)
        if not m:
            print(f"WARN: row {row_num} not found in sheet33.xml")
            continue
        prefix, body, suffix = m.group(1), m.group(2), m.group(3)
        # Look for existing <c r="RN" .../>
        c_pattern = re.compile(rf'<c r="R{row_num}"[^>]*>(?:<v>[^<]*</v>)?</c>')
        new_c = f'<c r="R{row_num}" t="s"><v>{ss_idx}</v></c>'
        if c_pattern.search(body):
            new_body = c_pattern.sub(new_c, body)
        else:
            # Insert new <c> in column-sorted position. For simplicity, append at end.
            new_body = body + new_c
        sheet_text = sheet_text[:m.start()] + prefix + new_body + suffix + sheet_text[m.end():]

    # Write back to xlsx
    new_sheet_bytes = sheet_text.encode("utf-8")
    tmp_path = SPREADSHEET.with_suffix(".xlsx.tmp")
    with zipfile.ZipFile(SPREADSHEET, "r") as zin:
        with zipfile.ZipFile(tmp_path, "w", zipfile.ZIP_DEFLATED) as zout:
            for item in zin.infolist():
                if item.filename == DATA_SHEET_XML:
                    zout.writestr(item, new_sheet_bytes)
                elif item.filename == SHARED_STRINGS_XML:
                    zout.writestr(item, new_ss_bytes)
                else:
                    zout.writestr(item, zin.read(item.filename))
    tmp_path.replace(SPREADSHEET)


# ---------- tracking ----------

def load_tracking():
    if not TRACKING.exists():
        return {"metadata": {}, "files": {}}
    return json.loads(TRACKING.read_text())


def save_tracking(d):
    d.setdefault("metadata", {})["last_updated"] = datetime.datetime.utcnow().isoformat()
    TRACKING.write_text(json.dumps(d, indent=2))


# ---------- main ----------

SKIP_PATTERNS = [
    re.compile(r"^\d{6} \$[\d,.\-]+ Ramp\.csv$"),  # Workflow A
    re.compile(r"^\d{6} Chase \d{4} ALLREC\.csv$"),  # Workflow D
    re.compile(r"^\d{6} \$[\d,.\-]+ Chase \d{4} (checking|savings)\.pdf$"),  # already filed
]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--apply", action="store_true", help="Apply matches (default dry-run).")
    args = ap.parse_args()

    if not SPREADSHEET.exists():
        print(f"ERROR: spreadsheet missing: {SPREADSHEET}", file=sys.stderr)
        sys.exit(2)

    files = sorted([p.name for p in INCOMING_TEMP.iterdir() if p.is_file() and not p.name.startswith(".")])
    candidates_in = [f for f in files if not any(p.match(f) for p in SKIP_PATTERNS)]
    skipped = [f for f in files if f not in candidates_in]

    print(f"=== batch_evidence_match (Workflow C) ===")
    print(f"Mode: {'apply' if args.apply else 'dry-run'}")
    print(f"incoming-temp/ scanned: {len(files)} files")
    print(f"  skipped (Workflow A/D/Chase-statement): {len(skipped)}")
    print(f"  candidates: {len(candidates_in)}")

    print("Loading data tab...")
    with zipfile.ZipFile(SPREADSHEET) as zf:
        strings = load_shared_strings(zf)
        rows = load_data_rows(zf, strings)
    print(f"  data tab rows: {len(rows)}")

    unique_matches = []   # (filename, row_num, row_dict)
    multi_matches = []    # (filename, [(row_num, row_dict), ...])
    no_matches = []       # (filename, parsed_or_None)
    unparseable = []      # filename
    timestamp = datetime.datetime.utcnow().isoformat()

    for fn in candidates_in:
        parsed = parse_filename(fn)
        if not parsed or parsed["amount"] is None:
            unparseable.append(fn)
            continue
        cands = find_matches(rows, parsed)
        if len(cands) == 1:
            unique_matches.append((fn, cands[0][0], cands[0][1]))
        elif len(cands) == 0:
            no_matches.append((fn, parsed))
        else:
            multi_matches.append((fn, cands))

    print(f"\n  → unique matches: {len(unique_matches)}")
    print(f"  → multiple matches (manual review): {len(multi_matches)}")
    print(f"  → no match: {len(no_matches)}")
    print(f"  → unparseable: {len(unparseable)}")

    print("\n--- Unique matches (would auto-apply) ---")
    for fn, r, row in unique_matches:
        print(f"  row {r}: {fn}")
        print(f"      → date={serial_to_date(float(row.get('C',0)))} amount={row.get('E')} desc={row.get('D','')[:60]} site={row.get('H','')}")

    print("\n--- Multi-match candidates (need manual review) ---")
    for fn, cands in multi_matches:
        print(f"  {fn}  → {len(cands)} candidates:")
        for r, row in cands:
            print(f"      row {r}: date={serial_to_date(float(row.get('C',0)))} amount={row.get('E')} site={row.get('H','')} desc={(row.get('D','') or '')[:50]}")

    print("\n--- No match ---")
    for fn, parsed in no_matches:
        print(f"  {fn}  (parsed: {parsed['date']} ${parsed['amount']} {parsed['vendor']}{' ~approx' if parsed['approx'] else ''})")

    if unparseable:
        print("\n--- Unparseable ---")
        for fn in unparseable:
            print(f"  {fn}")

    if not args.apply:
        print("\nDry-run only. Re-run with --apply to write matches.")
        return

    # APPLY
    # Aggregate by row: multiple files → comma-joined filenames in R
    by_row = {}
    for fn, r, _ in unique_matches:
        by_row.setdefault(r, []).append(fn)
    if unique_matches:
        print(f"\nApplying {len(unique_matches)} unique matches across {len(by_row)} rows...")
        updates = [(r, ", ".join(fns)) for r, fns in by_row.items()]
        write_receipt_to_data_tab(updates)

    tracking = load_tracking()
    tracking.setdefault("files", {})

    for fn, r, row in unique_matches:
        src = INCOMING_TEMP / fn
        dst = PROCESSED / fn
        if dst.exists():
            print(f"  COLLISION at processed/{fn} — leaving in incoming-temp")
            continue
        shutil.move(str(src), str(dst))
        tracking["files"][fn] = {
            "status": "processed",
            "match_info": f"Workflow C: matched data tab row {r}",
            "added": timestamp,
        }

    for fn, cands in multi_matches:
        tracking["files"][fn] = {
            "status": "pending_review",
            "match_info": f"Workflow C: {len(cands)} candidates - rows {[r for r,_ in cands]}",
            "added": timestamp,
        }

    for fn, parsed in no_matches:
        tracking["files"][fn] = {
            "status": "no_match",
            "match_info": f"Workflow C: no match in data tab (date={parsed['date']}, amount={parsed['amount']}, vendor={parsed['vendor']})",
            "added": timestamp,
        }

    save_tracking(tracking)
    print(f"\nApplied. {len(unique_matches)} files moved to processed/. {len(multi_matches)} pending review. {len(no_matches)} no_match.")


if __name__ == "__main__":
    main()
