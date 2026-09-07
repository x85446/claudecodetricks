#!/usr/bin/env python3
"""
apiaryx_splits.py - Manage ApiaryX split children in Finance.xlsx

Handles the case where ApiaryX invoices arrive before the Chase payment.
Invoices are added as orphan SPLIT children (no parent row yet). When the
Chase statement is later imported, orphans are adopted by the parent.

Commands:
  add-children <directory>       Add ApiaryX invoices as orphan SPLIT rows
  list-orphans                   Show orphan SPLIT rows (no parent yet)
  adopt-orphans <parent-row>     Link orphan children to a Chase parent row

Examples:
  python apiaryx_splits.py add-children /workspace/processing/incoming-temp
  python apiaryx_splits.py list-orphans
  python apiaryx_splits.py adopt-orphans 2736
"""

import os
import re
import shutil
import sys
import tempfile
import xml.etree.ElementTree as ET
import zipfile
from datetime import datetime, timedelta
from pathlib import Path

SPREADSHEET = "/workspace/processing/spreadsheet/Finance.xlsx"
DATA_SHEET_XML = "xl/worksheets/sheet33.xml"
SHARED_STRINGS_XML = "xl/sharedStrings.xml"
SS_NS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
EXCEL_EPOCH = datetime(1899, 12, 30)

APIARYX_RE = re.compile(
    r"^(\d{6})\s+"
    r"[~]?\$([0-9,]+\.\d{2})\s+"
    r"ApiaryX\s+"
    r"(INV-(?:COM-)?25-\d{5})\s+"
    r"(\w+)\.pdf$"
)


def date_to_serial(dt):
    return (dt - EXCEL_EPOCH).days


def serial_to_date(s):
    return EXCEL_EPOCH + timedelta(days=int(s))


def parse_filename(filename):
    m = APIARYX_RE.match(filename)
    if not m:
        return None
    date_str, amount_str, invoice, tag = m.groups()
    date = datetime(2000 + int(date_str[:2]), int(date_str[2:4]), int(date_str[4:6]))
    amount = float(amount_str.replace(",", ""))
    if tag == "eor":
        return dict(date=date, amount=amount, invoice=invoice, tag=tag,
                     site="apiaryx.fee", type="FEE", description="Commission",
                     filename=filename)
    return dict(date=date, amount=amount, invoice=invoice, tag=tag,
                 site=f"apiaryx.{tag}", type="EMPLOYEE_SALARY",
                 description=f"{tag.capitalize()} Salary", filename=filename)


# ── Shared Strings (add-only, via string manipulation) ─────────────

def _load_shared_strings():
    """Load shared strings from the xlsx. Returns (list_of_strings, raw_xml_bytes)."""
    with zipfile.ZipFile(SPREADSHEET, "r") as z:
        raw = z.read(SHARED_STRINGS_XML)
    ET.register_namespace("", SS_NS)
    root = ET.fromstring(raw)
    ns = {"m": SS_NS}
    strings = []
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


def _add_shared_strings(raw_xml, existing_strings, new_strings):
    """Add new strings to shared strings XML. Returns (updated_xml, updated_list).

    Works by splicing XML directly to avoid namespace mangling.
    """
    all_strings = list(existing_strings)
    index_map = {s: i for i, s in enumerate(all_strings)}
    to_add = []
    for s in new_strings:
        if s not in index_map:
            idx = len(all_strings)
            all_strings.append(s)
            index_map[s] = idx
            to_add.append(s)

    if not to_add:
        return raw_xml, all_strings

    # Build XML fragments for new <si> elements
    fragments = []
    for s in to_add:
        escaped = s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        fragments.append(
            f'<si xmlns="{SS_NS}"><t xml:space="preserve">{escaped}</t></si>'
        )
    insert_xml = "".join(fragments)

    # Insert before closing </sst> tag
    close_tag = "</sst>"
    pos = raw_xml.rfind(close_tag.encode("utf-8"))
    if pos < 0:
        raise ValueError("Cannot find </sst> in shared strings XML")

    # Update count and uniqueCount attributes
    new_count = str(len(all_strings))
    updated = raw_xml[:pos] + insert_xml.encode("utf-8") + raw_xml[pos:]
    updated = re.sub(
        rb'count="[^"]*"',
        f'count="{new_count}"'.encode("utf-8"),
        updated,
        count=1,
    )
    updated = re.sub(
        rb'uniqueCount="[^"]*"',
        f'uniqueCount="{new_count}"'.encode("utf-8"),
        updated,
        count=1,
    )
    return updated, all_strings


# ── Data Sheet (direct XML string manipulation) ────────────────────

def _load_data_sheet_raw():
    """Load data sheet raw XML bytes."""
    with zipfile.ZipFile(SPREADSHEET, "r") as z:
        return z.read(DATA_SHEET_XML)


def _build_row_xml(row_num, cells):
    """Build a <row> XML string.

    cells: list of (col_letter, value, type) where type is 's' or 'n'.
    For type 's', value is the shared string index (int).
    For type 'n', value is the numeric value.
    """
    cell_xmls = []
    for col, val, typ in cells:
        ref = f"{col}{row_num}"
        if typ == "s":
            cell_xmls.append(f'<c r="{ref}" t="s"><v>{val}</v></c>')
        else:
            cell_xmls.append(f'<c r="{ref}"><v>{val}</v></c>')
    cells_str = "".join(cell_xmls)
    return f'<row r="{row_num}">{cells_str}</row>'


def _find_marker_in_xml(raw_xml, strings):
    """Find the byte position of the marker row (<row> containing 'Details' in col B).

    Returns (position_of_row_start, row_number) or (None, None).
    """
    details_idx = None
    for i, s in enumerate(strings):
        if s == "Details":
            details_idx = i
            break
    if details_idx is None:
        return None, None

    # Find the cell <c r="BNNNN" ...t="s"><v>IDX</v></c>
    # Cells may have style attrs like s="699" before or after t="s"
    cell_pattern = re.compile(
        rb'<c r="B(\d+)"[^>]*t="s"[^>]*><v>'
        + str(details_idx).encode()
        + rb'</v></c>'
    )
    m = cell_pattern.search(raw_xml)
    if not m:
        # Try with t before other attrs
        cell_pattern2 = re.compile(
            rb'<c r="B(\d+)" t="s"[^>]*><v>'
            + str(details_idx).encode()
            + rb'</v></c>'
        )
        m = cell_pattern2.search(raw_xml)
    if m:
        row_num = int(m.group(1))
        # Find the <row> tag for this row number
        row_start = re.compile(
            rb'<row r="' + str(row_num).encode() + rb'"[^>]*>'
        )
        rm = row_start.search(raw_xml)
        if rm:
            return rm.start(), row_num

    return None, None


def _find_receipts_in_xml(raw_xml, strings):
    """Extract receipt filenames from column R."""
    receipts = set()
    # Find all <c r="RNNNN" ...t="s"...><v>IDX</v></c>
    pattern = re.compile(rb'<c r="R\d+"[^>]*t="s"[^>]*><v>(\d+)</v></c>')
    for m in pattern.finditer(raw_xml):
        idx = int(m.group(1))
        if 0 <= idx < len(strings):
            receipts.add(strings[idx])
    return receipts


def _find_orphans_in_xml(raw_xml, strings):
    """Find orphan SPLIT rows (apiaryx link, no sublink, no posting date).

    Returns list of dicts with row info.
    """
    # Build index lookups
    split_idx = next((i for i, s in enumerate(strings) if s == "SPLIT"), None)
    apiaryx_idx = next((i for i, s in enumerate(strings) if s == "apiaryx"), None)
    if split_idx is None or apiaryx_idx is None:
        return []

    orphans = []
    # Find all <row> elements
    row_re = re.compile(rb'<row r="(\d+)"[^>]*>(.*?)</row>', re.DOTALL)
    # Match cells: capture column letter, detect t="s", capture <v> value
    # Handles cells with style attrs like <c r="B123" s="634" t="s"><v>199</v></c>
    # and cells without style: <c r="B123" t="s"><v>199</v></c>
    # and cells without type: <c r="E123"><v>-100</v></c>
    cell_re = re.compile(rb'<c r="([A-Z]+)\d+"([^>]*)>(?:<v>([^<]*)</v>)?')

    for rm in row_re.finditer(raw_xml):
        row_num = int(rm.group(1))
        row_content = rm.group(2)
        cells = {}
        for cm in cell_re.finditer(row_content):
            col = cm.group(1).decode()
            attrs = cm.group(2)  # everything after r="XN" and before >
            val = cm.group(3)
            is_string = b't="s"' in attrs if attrs else False
            if is_string and val:
                idx = int(val)
                cells[col] = strings[idx] if idx < len(strings) else ""
            elif val:
                cells[col] = val.decode()
            else:
                cells[col] = None

        if (
            cells.get("B") == "SPLIT"
            and cells.get("I") == "apiaryx"
            and not cells.get("J")
            and not cells.get("C")
        ):
            amt_str = cells.get("E", "0")
            try:
                amt = float(amt_str)
            except (ValueError, TypeError):
                amt = 0
            k_str = cells.get("K", "")
            orphans.append({
                "row_num": row_num,
                "row_start": rm.start(),
                "row_end": rm.end(),
                "desc": cells.get("D", ""),
                "amount": amt,
                "site": cells.get("H", ""),
                "acrual_serial": k_str,
                "receipt": cells.get("R", ""),
            })
    return orphans


def _renumber_row(row_xml_bytes, old_num, new_num):
    """Renumber a row and its cell references in raw XML bytes."""
    old = str(old_num).encode()
    new = str(new_num).encode()
    # Replace row number: <row r="OLD" → <row r="NEW"
    result = re.sub(
        rb'<row r="' + old + rb'"',
        b'<row r="' + new + b'"',
        row_xml_bytes,
    )
    # Replace cell refs: r="XOLD" → r="XNEW" for all column letters
    result = re.sub(
        rb'r="([A-Z]+)' + old + rb'"',
        rb'r="\g<1>' + new + b'"',
        result,
    )
    return result


def _save_modified(sheet_xml, ss_xml):
    """Write modified sheet and shared strings back to the xlsx."""
    tmp_fd, tmp_path = tempfile.mkstemp(suffix=".xlsx")
    os.close(tmp_fd)
    try:
        with zipfile.ZipFile(SPREADSHEET, "r") as zin:
            with zipfile.ZipFile(tmp_path, "w", zipfile.ZIP_DEFLATED) as zout:
                for item in zin.infolist():
                    if item.filename == DATA_SHEET_XML:
                        zout.writestr(item, sheet_xml)
                    elif item.filename == SHARED_STRINGS_XML:
                        zout.writestr(item, ss_xml)
                    else:
                        zout.writestr(item, zin.read(item.filename))
        shutil.move(tmp_path, SPREADSHEET)
    except Exception:
        if os.path.exists(tmp_path):
            os.unlink(tmp_path)
        raise


# ── Commands ───────────────────────────────────────────────────────


def add_children(directory):
    directory = Path(directory)
    if not directory.is_dir():
        print(f"Error: {directory} is not a directory")
        sys.exit(1)

    invoices = []
    for f in sorted(directory.iterdir()):
        if not f.name.lower().endswith(".pdf"):
            continue
        parsed = parse_filename(f.name)
        if parsed:
            invoices.append(parsed)

    if not invoices:
        print("No ApiaryX invoice files found in", directory)
        return

    print(f"Found {len(invoices)} ApiaryX invoice(s):")
    for inv in invoices:
        print(f"  {inv['filename']}")
        print(f"    {inv['description']}, ${inv['amount']:.2f}, "
              f"{inv['date'].strftime('%Y-%m-%d')}")

    # Load data
    print(f"\nLoading {SPREADSHEET}...")
    strings, ss_raw = _load_shared_strings()
    sheet_raw = _load_data_sheet_raw()

    # Check for duplicates
    existing_receipts = _find_receipts_in_xml(sheet_raw, strings)
    to_add = []
    for inv in invoices:
        if inv["filename"] in existing_receipts:
            print(f"  SKIP (duplicate): {inv['filename']}")
        else:
            to_add.append(inv)

    if not to_add:
        print("\nNo new rows to add.")
        return

    # Collect all new strings we'll need
    new_string_values = set()
    for inv in to_add:
        new_string_values.update([
            "SPLIT", "Actual", "apiaryx",
            inv["description"], inv["type"], inv["site"], inv["filename"],
        ])
    ss_updated, strings = _add_shared_strings(ss_raw, strings, new_string_values)
    str_idx = {s: i for i, s in enumerate(strings)}

    # Find marker row position
    marker_pos, marker_row_num = _find_marker_in_xml(sheet_raw, strings)
    if marker_pos is None:
        print("Error: could not find marker row in data sheet")
        sys.exit(1)
    print(f"  Marker row at row {marker_row_num}, inserting before it")

    # Build new row XML strings
    new_rows_xml = []
    for offset, inv in enumerate(to_add):
        row_num = marker_row_num + offset
        cells = [
            ("B", str_idx["SPLIT"], "s"),
            ("D", str_idx[inv["description"]], "s"),
            ("E", -inv["amount"], "n"),
            ("F", str_idx[inv["type"]], "s"),
            ("G", str_idx["Actual"], "s"),
            ("H", str_idx[inv["site"]], "s"),
            ("I", str_idx["apiaryx"], "s"),
            ("K", date_to_serial(inv["date"]), "n"),
            ("R", str_idx[inv["filename"]], "s"),
        ]
        row_xml = _build_row_xml(row_num, cells)
        new_rows_xml.append(row_xml)
        print(f"  ADD row {row_num}: {inv['description']} = -${inv['amount']:.2f}")

    bump = len(to_add)

    # Find all rows at or after the marker and renumber them
    # Extract marker row and subsequent rows from XML
    # Pattern: find all <row r="N"> where N >= marker_row_num
    row_re = re.compile(rb'<row r="(\d+)"[^>]*>.*?</row>', re.DOTALL)
    pieces = []
    last_end = 0
    for m in row_re.finditer(sheet_raw):
        rn = int(m.group(1))
        if rn >= marker_row_num:
            # Add everything before this row
            pieces.append(sheet_raw[last_end:m.start()])
            # Renumber and add
            new_rn = rn + bump
            pieces.append(_renumber_row(m.group(0), rn, new_rn))
            last_end = m.end()
    pieces.append(sheet_raw[last_end:])
    sheet_updated = b"".join(pieces)

    # Insert new rows before the (now renumbered) marker
    new_marker_pos, _ = _find_marker_in_xml(sheet_updated, strings)
    if new_marker_pos is None:
        print("Error: lost marker row after renumbering")
        sys.exit(1)

    insert_bytes = "".join(new_rows_xml).encode("utf-8")
    sheet_final = (
        sheet_updated[:new_marker_pos]
        + insert_bytes
        + sheet_updated[new_marker_pos:]
    )

    # Update dimension ref if present
    dim_re = re.compile(rb'<dimension ref="([A-Z]+\d+):([A-Z]+)(\d+)"')
    dim_m = dim_re.search(sheet_final)
    if dim_m:
        old_max = int(dim_m.group(3))
        new_max = old_max + bump
        sheet_final = (
            sheet_final[:dim_m.start(3)]
            + str(new_max).encode()
            + sheet_final[dim_m.end(3):]
        )

    # Save
    print(f"\nSaving {SPREADSHEET}...")
    _save_modified(sheet_final, ss_updated)
    print(f"Saved. Added {len(to_add)} orphan SPLIT row(s).")


def list_orphans():
    strings, _ = _load_shared_strings()
    sheet_raw = _load_data_sheet_raw()
    orphans = _find_orphans_in_xml(sheet_raw, strings)

    if not orphans:
        print("No orphan ApiaryX SPLIT rows found.")
        return

    print(f"Orphan ApiaryX SPLIT rows ({len(orphans)}):")
    total = 0.0
    for o in orphans:
        total += o["amount"]
        k_str = ""
        if o["acrual_serial"]:
            try:
                k_str = serial_to_date(float(o["acrual_serial"])).strftime("%Y-%m-%d")
            except (ValueError, TypeError):
                k_str = o["acrual_serial"]
        print(
            f"  Row {o['row_num']:5d}: "
            f"{o['desc']:25s} "
            f"{o['amount']:>10.2f}  "
            f"site={o['site']:20s} "
            f"K={k_str}  "
            f"R={o['receipt']}"
        )
    print(f"  {'TOTAL':>32s} {total:>10.2f}")
    print(f"\nExpected parent amount: {total:.2f}")


def adopt_orphans(parent_row_num, only_rows=None):
    strings, ss_raw = _load_shared_strings()
    sheet_raw = _load_data_sheet_raw()
    orphans = _find_orphans_in_xml(sheet_raw, strings)

    if only_rows is not None:
        only_set = set(only_rows)
        before = len(orphans)
        orphans = [o for o in orphans if o["row_num"] in only_set]
        missing = only_set - {o["row_num"] for o in orphans}
        if missing:
            print(f"Warning: requested rows not currently orphans (already adopted, or not apiaryx SPLIT): {sorted(missing)}")
        print(f"Filtered to {len(orphans)} of {before} orphans (--rows filter)")

    if not orphans:
        print("No orphan children found to adopt.")
        return

    # Find parent row
    row_re = re.compile(rb'<row r="(\d+)"[^>]*>(.*?)</row>', re.DOTALL)
    cell_re = re.compile(rb'<c r="([A-Z]+)\d+"([^>]*)>(?:<v>([^<]*)</v>)?')
    parent_match = None
    parent_cells = {}
    for rm in row_re.finditer(sheet_raw):
        if int(rm.group(1)) == parent_row_num:
            parent_match = rm
            for cm in cell_re.finditer(rm.group(2)):
                col = cm.group(1).decode()
                attrs = cm.group(2) or b""
                val = cm.group(3)
                is_string = b't="s"' in attrs
                if val is None:
                    parent_cells[col] = None
                elif is_string:
                    try:
                        idx = int(val)
                        parent_cells[col] = strings[idx] if 0 <= idx < len(strings) else ""
                    except ValueError:
                        parent_cells[col] = ""
                else:
                    parent_cells[col] = val.decode()
            break

    if parent_match is None:
        print(f"Error: Row {parent_row_num} not found")
        sys.exit(1)

    if "apiaryx" not in (parent_cells.get("I", "") or "").lower():
        print(f"Error: Row {parent_row_num} link={parent_cells.get('I')}, not ApiaryX")
        sys.exit(1)

    if "DEBIT" not in (parent_cells.get("B", "") or ""):
        print(f"Error: Row {parent_row_num} details={parent_cells.get('B')}, expected DEBIT-IT1")
        sys.exit(1)

    date_serial = parent_cells.get("C", "")
    if not date_serial:
        print(f"Error: Row {parent_row_num} has no posting date")
        sys.exit(1)

    parent_date = serial_to_date(float(date_serial))
    yymm = parent_date.strftime("%y%m")
    sublink_parent = f"APX{yymm}"
    sublink_child = f"APX{yymm}."

    print(f"Parent row {parent_row_num}:")
    print(f"  Details:  {parent_cells.get('B')}")
    print(f"  Date:     {parent_date.strftime('%Y-%m-%d')}")
    print(f"  Amount:   {parent_cells.get('E')}")
    print(f"  Site:     {parent_cells.get('H')} -> apiaryx.com")
    print(f"  Sublink:  -> {sublink_parent}")

    child_total = 0.0
    print(f"\nOrphan children ({len(orphans)}):")
    for o in orphans:
        child_total += o["amount"]
        print(f"  Row {o['row_num']}: {o['desc']:25s} {o['amount']:>10.2f}")

    parent_amt = float(parent_cells.get("E", "0"))
    diff = abs(parent_amt) - abs(child_total)
    print(f"\n  Parent amount:  {parent_amt:>10.2f}")
    print(f"  Children sum:   {child_total:>10.2f}")
    print(f"  Difference:     {diff:>10.2f}")

    if abs(diff) > 0.01:
        print(f"\n  Warning: children don't sum to parent (diff=${diff:.2f}).")
        print("  Proceeding - additional children may be needed later.")

    # Collect new strings
    new_strings = {sublink_parent, sublink_child, "apiaryx.com"}
    ss_updated, strings = _add_shared_strings(ss_raw, strings, new_strings)
    str_idx = {s: i for i, s in enumerate(strings)}

    # Modify parent row: set J=sublink_parent, H=apiaryx.com
    sheet_modified = sheet_raw
    parent_row_xml = parent_match.group(0)
    new_parent = _set_cell_s(parent_row_xml, "J", parent_row_num, str_idx[sublink_parent])
    new_parent = _set_cell_s(new_parent, "H", parent_row_num, str_idx["apiaryx.com"])
    sheet_modified = sheet_modified.replace(parent_row_xml, new_parent, 1)

    # Modify orphan rows: set C=date_serial, J=sublink_child
    for o in orphans:
        old_xml = sheet_raw[o["row_start"]:o["row_end"]]
        new_xml = _set_cell_n(old_xml, "C", o["row_num"], date_serial)
        new_xml = _set_cell_s(new_xml, "J", o["row_num"], str_idx[sublink_child])
        sheet_modified = sheet_modified.replace(old_xml, new_xml, 1)

    print(f"\nSaving {SPREADSHEET}...")
    _save_modified(sheet_modified, ss_updated)
    print(f"Saved.")
    print(f"  Parent: sublink={sublink_parent}, site=apiaryx.com")
    print(f"  {len(orphans)} children: posting_date={parent_date.strftime('%Y-%m-%d')}, "
          f"sublink={sublink_child}")


def _set_cell_s(row_xml, col, row_num, ss_index):
    """Add or update a string cell in row XML bytes."""
    ref = f"{col}{row_num}".encode()
    cell_pattern = re.compile(
        rb'<c r="' + ref + rb'"[^>]*>.*?</c>',
        re.DOTALL,
    )
    new_cell = f'<c r="{col}{row_num}" t="s"><v>{ss_index}</v></c>'.encode()

    m = cell_pattern.search(row_xml)
    if m:
        return row_xml[:m.start()] + new_cell + row_xml[m.end():]
    # Insert before </row>
    close = row_xml.rfind(b"</row>")
    return row_xml[:close] + new_cell + row_xml[close:]


def _set_cell_n(row_xml, col, row_num, value):
    """Add or update a numeric cell in row XML bytes."""
    ref = f"{col}{row_num}".encode()
    cell_pattern = re.compile(
        rb'<c r="' + ref + rb'"[^>]*>.*?</c>',
        re.DOTALL,
    )
    new_cell = f'<c r="{col}{row_num}"><v>{value}</v></c>'.encode()

    m = cell_pattern.search(row_xml)
    if m:
        return row_xml[:m.start()] + new_cell + row_xml[m.end():]
    close = row_xml.rfind(b"</row>")
    return row_xml[:close] + new_cell + row_xml[close:]


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "add-children":
        if len(sys.argv) < 3:
            print("Usage: apiaryx_splits.py add-children <directory>")
            sys.exit(1)
        add_children(sys.argv[2])
    elif cmd == "list-orphans":
        list_orphans()
    elif cmd == "adopt-orphans":
        if len(sys.argv) < 3:
            print("Usage: apiaryx_splits.py adopt-orphans <parent-row> [--rows N1,N2,...]")
            sys.exit(1)
        parent_row = int(sys.argv[2])
        only_rows = None
        if "--rows" in sys.argv:
            i = sys.argv.index("--rows")
            if i + 1 >= len(sys.argv):
                print("Error: --rows requires a comma-separated list")
                sys.exit(1)
            only_rows = [int(r) for r in sys.argv[i + 1].split(",") if r.strip()]
        adopt_orphans(parent_row, only_rows=only_rows)
    else:
        print(f"Unknown command: {cmd}")
        print(__doc__)
        sys.exit(1)


if __name__ == "__main__":
    main()
