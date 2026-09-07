#!/usr/bin/env python3
"""sync_apiaryx_tab.py - Maintain the apiaryx ledger tab in Finance.xlsx.

Appends charge & payment rows to the 'apiaryx' tab from the 'data' tab.
Idempotent: invoices already referenced in apiaryx column H are skipped.

Usage:
  python sync_apiaryx_tab.py                            # dry run (default)
  python sync_apiaryx_tab.py --apply                    # write changes
  python sync_apiaryx_tab.py --reset-from-row 36 --apply
                                                        # wipe rows 36+ and rebuild

See ../SKILL.md for the full design.
"""

from __future__ import annotations

import argparse
import os
import re
import shutil
import sys
import tempfile
import zipfile
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta

SPREADSHEET = "/workspace/processing/spreadsheet/Finance.xlsx"
BACKUP_DIR = "/workspace/processing/spreadsheet/backups"
APIARYX_SHEET_XML = "xl/worksheets/sheet18.xml"
DATA_SHEET_XML = "xl/worksheets/sheet33.xml"
SHARED_STRINGS_XML = "xl/sharedStrings.xml"
SS_NS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
EXCEL_EPOCH = datetime(1899, 12, 30)

PRESERVED_THROUGH_ROW = 35
INVOICE_RE = re.compile(r"INV(?:-COM)?-\d\d-\d{5}")
SHORTHAND_RE = re.compile(r"(INV(?:-COM)?-\d\d-)(\d{5})((?:,\d+)+)")
FILENAME_INV_RE = re.compile(r"(INV(?:-COM)?-\d\d-\d{5})")

MONTH_NAMES = [
    "January", "February", "March", "April", "May", "June",
    "July", "August", "September", "October", "November", "December",
]


# ── Date helpers ────────────────────────────────────────────────────

def serial_to_date(s) -> datetime | None:
    try:
        return EXCEL_EPOCH + timedelta(days=int(float(s)))
    except (ValueError, TypeError):
        return None


def date_to_serial(dt: datetime) -> int:
    return (dt - EXCEL_EPOCH).days


# ── Shared strings I/O ──────────────────────────────────────────────

def load_shared_strings(zf: zipfile.ZipFile) -> tuple[list[str], bytes]:
    raw = zf.read(SHARED_STRINGS_XML)
    ET.register_namespace("", SS_NS)
    root = ET.fromstring(raw)
    ns = {"m": SS_NS}
    strings: list[str] = []
    for si in root.findall("m:si", ns):
        t = si.find("m:t", ns)
        if t is not None and t.text:
            strings.append(t.text)
        else:
            parts = []
            for r in si.findall("m:r", ns):
                rt = r.find("m:t", ns)
                if rt is not None and rt.text:
                    parts.append(rt.text)
            strings.append("".join(parts))
    return strings, raw


def add_shared_strings(raw_xml: bytes, existing: list[str], to_add: list[str]) -> tuple[bytes, list[str]]:
    """Append new strings to shared strings XML. Returns updated bytes + updated list."""
    all_strings = list(existing)
    index_map = {s: i for i, s in enumerate(all_strings)}
    new = []
    for s in to_add:
        if s not in index_map:
            index_map[s] = len(all_strings)
            all_strings.append(s)
            new.append(s)
    if not new:
        return raw_xml, all_strings
    fragments = []
    for s in new:
        escaped = s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        fragments.append(f'<si xmlns="{SS_NS}"><t xml:space="preserve">{escaped}</t></si>')
    insert_xml = "".join(fragments).encode("utf-8")
    pos = raw_xml.rfind(b"</sst>")
    if pos < 0:
        raise ValueError("Cannot find </sst> in shared strings XML")
    new_count = str(len(all_strings)).encode()
    updated = raw_xml[:pos] + insert_xml + raw_xml[pos:]
    updated = re.sub(rb'count="[^"]*"', b'count="' + new_count + b'"', updated, count=1)
    updated = re.sub(rb'uniqueCount="[^"]*"', b'uniqueCount="' + new_count + b'"', updated, count=1)
    return updated, all_strings


# ── Sheet row parsing ───────────────────────────────────────────────

CELL_RE = re.compile(rb'<c r="([A-Z]+)\d+"([^>]*)>(?:<v>([^<]*)</v>|<f[^>]*>[^<]*</f>(?:<v>([^<]*)</v>)?)?')
ROW_RE = re.compile(rb'<row r="(\d+)"[^>]*>(.*?)</row>', re.DOTALL)


def parse_row_cells(row_content: bytes, strings: list[str]) -> dict[str, str | None]:
    """Decode all <c> cells in a row body. Returns column letter → value (str or None).
    String cells are dereferenced via shared strings."""
    cells: dict[str, str | None] = {}
    for cm in CELL_RE.finditer(row_content):
        col = cm.group(1).decode()
        attrs = cm.group(2) or b""
        v_simple = cm.group(3)
        v_formula = cm.group(4)
        val = v_simple if v_simple is not None else v_formula
        if val is None:
            cells[col] = None
            continue
        is_string = b't="s"' in attrs
        if is_string:
            try:
                idx = int(val)
                cells[col] = strings[idx] if 0 <= idx < len(strings) else None
            except ValueError:
                cells[col] = None
        else:
            cells[col] = val.decode()
    return cells


# ── Invoice number normalization ────────────────────────────────────

def expand_shorthand(text: str) -> set[str]:
    """Expand 'INV-25-00001,2,3,4,5,6,7' → {INV-25-00001, INV-25-00002, ..., INV-25-00007}.
    Returns the full set of invoice numbers referenced in the text."""
    invs: set[str] = set(INVOICE_RE.findall(text))
    for m in SHORTHAND_RE.finditer(text):
        prefix, base, tail = m.group(1), m.group(2), m.group(3)
        invs.add(f"{prefix}{base}")
        for n in tail.lstrip(",").split(","):
            n = n.strip()
            if n.isdigit():
                invs.add(f"{prefix}{n.zfill(5)}")
    return invs


def parse_invoice_from_filename(filename: str | None) -> str | None:
    if not filename:
        return None
    m = FILENAME_INV_RE.search(filename)
    return m.group(1) if m else None


# ── Reading apiaryx tab ─────────────────────────────────────────────

def read_apiaryx_state(sheet_xml: bytes, strings: list[str]) -> tuple[set[str], int]:
    """Returns (seen_invoice_numbers, last_row_number)."""
    seen: set[str] = set()
    last_row = 0
    for rm in ROW_RE.finditer(sheet_xml):
        rn = int(rm.group(1))
        last_row = max(last_row, rn)
        cells = parse_row_cells(rm.group(2), strings)
        notes = cells.get("H")
        if notes:
            seen |= expand_shorthand(notes)
    return seen, last_row


# ── Reading data tab for apiaryx-linked rows ────────────────────────

def collect_apiaryx_rows(data_xml: bytes, strings: list[str]) -> tuple[list[dict], list[dict]]:
    """Returns (charges, payments) — both as lists of dicts.
    Each charge: {date, entity, amount, invoice, source_row}
    Each payment: {date, amount, sublink, source_row}"""
    splits: list[dict] = []
    parents: list[dict] = []
    for rm in ROW_RE.finditer(data_xml):
        rn = int(rm.group(1))
        cells = parse_row_cells(rm.group(2), strings)
        link = (cells.get("I") or "").strip().lower()
        if link != "apiaryx":
            continue
        details = (cells.get("B") or "").strip()
        site = (cells.get("H") or "").strip().lower()
        if details == "SPLIT":
            invoice = parse_invoice_from_filename(cells.get("R"))
            date_serial = cells.get("K")  # ACRUAL
            try:
                amount = float(cells.get("E") or 0)
            except (ValueError, TypeError):
                amount = 0
            entity = "commission" if site == "apiaryx.fee" else _entity_from_site(site)
            splits.append({
                "row": rn,
                "date": serial_to_date(date_serial),
                "entity": entity,
                "charge": abs(amount),
                "invoice": invoice,
                "sublink": (cells.get("J") or "").strip(),  # 'APX2601.' once adopted
            })
        elif details.startswith("DEBIT-"):
            sublink = (cells.get("J") or "").strip()  # 'APX2601' once adopted
            if not sublink:
                continue  # unadopted parent has no link to children
            try:
                amount = float(cells.get("E") or 0)
            except (ValueError, TypeError):
                amount = 0
            date_serial = cells.get("C")  # Posting Date
            parents.append({
                "row": rn,
                "date": serial_to_date(date_serial),
                "payment": amount,  # already negative
                "sublink": sublink,
            })
    return splits, parents


def _entity_from_site(site: str) -> str:
    """apiaryx.abhishek → Abhishek; apiaryx → '' (skip)."""
    if "." not in site:
        return ""
    name = site.split(".", 1)[1]
    return name[:1].upper() + name[1:] if name else ""


# ── Build new rows ──────────────────────────────────────────────────

def build_new_entries(splits: list[dict], parents: list[dict], seen: set[str]) -> tuple[list[dict], list[str]]:
    """Returns (new_rows_sorted, skip_log)."""
    skipped: list[str] = []
    new_rows: list[dict] = []

    # Charge rows
    for s in splits:
        if not s["invoice"]:
            skipped.append(f"data row {s['row']}: SPLIT child has no parseable invoice# in R column → skipped")
            continue
        if s["invoice"] in seen:
            continue
        if not s["date"]:
            skipped.append(f"data row {s['row']}: SPLIT child {s['invoice']} has no acrual date (K) → skipped")
            continue
        if not s["entity"]:
            skipped.append(f"data row {s['row']}: SPLIT child {s['invoice']} has unrecognized site → skipped")
            continue
        new_rows.append({
            "kind": "charge",
            "date": s["date"],
            "entity": s["entity"],
            "charge": s["charge"],
            "payment": None,
            "notes": s["invoice"],
        })
        seen.add(s["invoice"])

    # Build parent-to-children map (sublink with period → list of child invoice#s)
    children_by_sublink: dict[str, list[str]] = {}
    for s in splits:
        sl = s["sublink"]
        inv = s["invoice"]
        if sl and inv:
            children_by_sublink.setdefault(sl, []).append(inv)

    # Payment rows
    for p in parents:
        child_sublink = p["sublink"] + "."  # parent APX2601 → children APX2601.
        invs = children_by_sublink.get(child_sublink, [])
        if not invs:
            skipped.append(f"data row {p['row']}: DEBIT-* parent sublink={p['sublink']} has no SPLIT children with parseable invoices → payment row skipped")
            continue
        joined = ", ".join(sorted(invs))
        # Idempotency: if every invoice this payment covers is already noted somewhere
        # in apiaryx (which it will be from the charge rows we just queued), the payment
        # row itself may still be missing — track it via a synthetic key.
        payment_key = f"PAY:{p['sublink']}"
        if payment_key in seen:
            continue
        if not p["date"]:
            skipped.append(f"data row {p['row']}: parent sublink={p['sublink']} has no posting date (C) → skipped")
            continue
        new_rows.append({
            "kind": "payment",
            "date": p["date"],
            "entity": "payment",
            "charge": None,
            "payment": p["payment"],
            "notes": joined,
        })
        seen.add(payment_key)

    new_rows.sort(key=lambda r: (r["date"], 0 if r["kind"] == "charge" else 1))
    return new_rows, skipped


def payment_already_in_apiaryx(sheet_xml: bytes, strings: list[str], invoice_set: set[str]) -> set[str]:
    """For payment dedup, find sublinks already noted (where Notes contains all of an invoice set)."""
    return set()  # currently we rely on the synthetic PAY:<sublink> key; a future enhancement
                  # could parse existing payment rows and reconstruct sublinks.


# ── Writing apiaryx tab ─────────────────────────────────────────────

def build_row_xml(row_num: int, entry: dict, str_idx: dict[str, int]) -> str:
    """Build a <row> element for one new entry. Uses formulas for B (qtr), C (year), G (total)."""
    dt = entry["date"]
    serial = date_to_serial(dt)
    qtr = (dt.month + 2) // 3
    year = dt.year
    month_name = MONTH_NAMES[dt.month - 1]

    cells: list[str] = []
    cells.append(f'<c r="A{row_num}"><v>{serial}</v></c>')
    # B: qtr (value, matches existing later rows)
    cells.append(f'<c r="B{row_num}"><v>{qtr}</v></c>')
    # C: year (value)
    cells.append(f'<c r="C{row_num}"><v>{year}</v></c>')
    # D: entity (string)
    cells.append(f'<c r="D{row_num}" t="s"><v>{str_idx[entry["entity"]]}</v></c>')
    # E: charge
    if entry["charge"] is not None:
        cells.append(f'<c r="E{row_num}"><v>{entry["charge"]}</v></c>')
    # F: payment
    if entry["payment"] is not None:
        cells.append(f'<c r="F{row_num}"><v>{entry["payment"]}</v></c>')
    # G: running total formula
    if row_num == 1:
        gformula = f"SUM(E{row_num}:F{row_num})"
    else:
        gformula = f"G{row_num - 1}+SUM(E{row_num}:F{row_num})"
    cells.append(f'<c r="G{row_num}"><f>{gformula}</f></c>')
    # H: Notes (string)
    cells.append(f'<c r="H{row_num}" t="s"><v>{str_idx[entry["notes"]]}</v></c>')
    # I: month (string)
    cells.append(f'<c r="I{row_num}" t="s"><v>{str_idx[month_name]}</v></c>')

    return f'<row r="{row_num}">{"".join(cells)}</row>'


def remove_rows_from(sheet_xml: bytes, from_row: int) -> bytes:
    """Delete every <row r="N">...</row> with N >= from_row from the sheet XML."""
    pieces = []
    last_end = 0
    for rm in ROW_RE.finditer(sheet_xml):
        rn = int(rm.group(1))
        if rn >= from_row:
            pieces.append(sheet_xml[last_end:rm.start()])
            last_end = rm.end()
    pieces.append(sheet_xml[last_end:])
    return b"".join(pieces)


def find_sheet_data_close(sheet_xml: bytes) -> int:
    """Position of </sheetData> close tag."""
    pos = sheet_xml.find(b"</sheetData>")
    if pos < 0:
        raise ValueError("Cannot find </sheetData> in apiaryx sheet XML")
    return pos


def update_dimension(sheet_xml: bytes, last_row: int) -> bytes:
    """Update dimension ref's max row to last_row (no-op if no dimension element)."""
    m = re.search(rb'<dimension ref="([A-Z]+\d+):([A-Z]+)(\d+)"', sheet_xml)
    if not m:
        return sheet_xml
    return sheet_xml[:m.start(3)] + str(last_row).encode() + sheet_xml[m.end(3):]


# ── Save xlsx ───────────────────────────────────────────────────────

def save_xlsx(updates: dict[str, bytes]) -> None:
    tmp_fd, tmp_path = tempfile.mkstemp(suffix=".xlsx")
    os.close(tmp_fd)
    try:
        with zipfile.ZipFile(SPREADSHEET, "r") as zin:
            with zipfile.ZipFile(tmp_path, "w", zipfile.ZIP_DEFLATED) as zout:
                for item in zin.infolist():
                    if item.filename in updates:
                        zout.writestr(item, updates[item.filename])
                    else:
                        zout.writestr(item, zin.read(item.filename))
        shutil.move(tmp_path, SPREADSHEET)
    except Exception:
        if os.path.exists(tmp_path):
            os.unlink(tmp_path)
        raise


def backup_xlsx() -> str:
    os.makedirs(BACKUP_DIR, exist_ok=True)
    ts = datetime.utcnow().strftime("%Y%m%dT%H%M%SZ")
    dest = os.path.join(BACKUP_DIR, f"Finance_pre_apiaryx_sync_{ts}.xlsx")
    shutil.copy2(SPREADSHEET, dest)
    return dest


# ── Main ────────────────────────────────────────────────────────────

def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--apply", action="store_true", help="Write changes (default: dry-run)")
    ap.add_argument("--reset-from-row", type=int, metavar="N",
                    help=f"Delete rows N..end before appending (N must be > {PRESERVED_THROUGH_ROW})")
    args = ap.parse_args()

    if args.reset_from_row is not None and args.reset_from_row <= PRESERVED_THROUGH_ROW:
        print(f"Error: --reset-from-row must be > {PRESERVED_THROUGH_ROW} (rows 1-{PRESERVED_THROUGH_ROW} are preserved)")
        return 1

    if not os.path.isfile(SPREADSHEET):
        print(f"Error: spreadsheet not found at {SPREADSHEET}")
        return 1

    with zipfile.ZipFile(SPREADSHEET, "r") as zf:
        strings, ss_raw = load_shared_strings(zf)
        apiaryx_xml = zf.read(APIARYX_SHEET_XML)
        data_xml = zf.read(DATA_SHEET_XML)

    # Optionally reset rows >= reset_from_row before reading state
    if args.reset_from_row is not None:
        apiaryx_xml = remove_rows_from(apiaryx_xml, args.reset_from_row)

    seen, last_row = read_apiaryx_state(apiaryx_xml, strings)

    splits, parents = collect_apiaryx_rows(data_xml, strings)
    new_rows, skipped = build_new_entries(splits, parents, seen)

    print(f"=== apiaryx-tab sync ===")
    mode = "apply" if args.apply else "dry-run"
    if args.reset_from_row is not None:
        mode += f" (reset from row {args.reset_from_row})"
    print(f"Mode: {mode}")
    print(f"Spreadsheet: {SPREADSHEET}")
    print(f"Existing apiaryx rows: last_row={last_row}, seen invoices={len(seen)}")
    print(f"Data tab apiaryx rows: {len(splits)} SPLIT children, {len(parents)} adopted parent(s)")
    print(f"Candidates: {len(new_rows)} new rows ({sum(1 for r in new_rows if r['kind']=='charge')} charges, "
          f"{sum(1 for r in new_rows if r['kind']=='payment')} payments)")
    if skipped:
        print(f"\nSkipped ({len(skipped)}):")
        for s in skipped:
            print(f"  {s}")

    if not new_rows and args.reset_from_row is None:
        print("\nNothing to do.")
        return 0

    insert_start = last_row + 1
    print(f"\nWould insert at rows {insert_start}–{insert_start + len(new_rows) - 1}:")
    for i, r in enumerate(new_rows):
        print(f"  row {insert_start + i}: {r['date'].strftime('%Y-%m-%d')}  "
              f"{r['entity']:12s}  charge={r['charge'] or '':>10}  "
              f"pay={r['payment'] or '':>10}  notes={r['notes']}")

    if not args.apply:
        print("\nDry-run only. Re-run with --apply to write.")
        return 0

    # Apply: gather new strings, build XML, splice into sheet, save
    needed_strings = set()
    for r in new_rows:
        needed_strings.add(r["entity"])
        needed_strings.add(r["notes"])
        needed_strings.add(MONTH_NAMES[r["date"].month - 1])
    ss_updated, strings = add_shared_strings(ss_raw, strings, list(needed_strings))
    str_idx = {s: i for i, s in enumerate(strings)}

    new_xml_pieces = []
    for i, r in enumerate(new_rows):
        new_xml_pieces.append(build_row_xml(insert_start + i, r, str_idx))
    insert_blob = "".join(new_xml_pieces).encode("utf-8")

    close_pos = find_sheet_data_close(apiaryx_xml)
    apiaryx_updated = apiaryx_xml[:close_pos] + insert_blob + apiaryx_xml[close_pos:]
    apiaryx_updated = update_dimension(apiaryx_updated, insert_start + len(new_rows) - 1)

    backup = backup_xlsx()
    print(f"\nBackup: {backup}")
    save_xlsx({
        APIARYX_SHEET_XML: apiaryx_updated,
        SHARED_STRINGS_XML: ss_updated,
    })
    print(f"Saved {SPREADSHEET}")
    print(f"Inserted {len(new_rows)} rows at {insert_start}–{insert_start + len(new_rows) - 1}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
