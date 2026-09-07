#!/usr/bin/env python3
"""import_chase_allrec.py - Import Chase ALLREC CSV(s) into Finance.xlsx data tab.

Reads CSV(s) like '260503 Chase 7505 ALLREC.csv', deduplicates against existing
data-tab rows by (Details prefix, Posting Date, Amount), and inserts new rows
before the marker row. Idempotent.

Usage:
  python import_chase_allrec.py <csv1> [csv2 ...]                    # dry run
  python import_chase_allrec.py <csv1> [csv2 ...] --apply            # write
"""

import argparse
import csv
import json
import os
import re
import shutil
import sys
import tempfile
import zipfile
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta

SPREADSHEET = "/workspace/processing/spreadsheet/Finance.xlsx"
DATA_SHEET_XML = "xl/worksheets/sheet33.xml"
SHARED_STRINGS_XML = "xl/sharedStrings.xml"
SS_NS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
EXCEL_EPOCH = datetime(1899, 12, 30)

ACCOUNT_MAP = {
    "7505": ("IN1", "Izuma Networks Main Checking"),
    "3839": ("IN2", "Izuma Networks Secondary Checking"),
    "3211": ("IS1", "Izuma Networks Savings"),
    "6557": ("IT1", "Izuma Tech Checking"),
}

FILENAME_RE = re.compile(r"(\d{6})\s+Chase\s+(\d{4})\s+ALLREC\.csv$", re.IGNORECASE)


def date_to_serial(dt: datetime) -> int:
    return (dt - EXCEL_EPOCH).days


def parse_csv_date(s: str) -> datetime:
    return datetime.strptime(s.strip(), "%m/%d/%Y")


# ── Shared strings ──────────────────────────────────────────────────

def load_shared_strings(zf: zipfile.ZipFile) -> tuple[list[str], bytes]:
    raw = zf.read(SHARED_STRINGS_XML)
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


def add_shared_strings(raw_xml: bytes, existing: list[str], to_add: list[str]) -> tuple[bytes, list[str]]:
    all_strings = list(existing)
    index_map = {s: i for i, s in enumerate(all_strings)}
    new = [s for s in to_add if s not in index_map]
    for s in new:
        index_map[s] = len(all_strings)
        all_strings.append(s)
    if not new:
        return raw_xml, all_strings
    fragments = []
    for s in new:
        escaped = s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        fragments.append(f'<si xmlns="{SS_NS}"><t xml:space="preserve">{escaped}</t></si>')
    insert = "".join(fragments).encode("utf-8")
    pos = raw_xml.rfind(b"</sst>")
    new_count = str(len(all_strings)).encode()
    updated = raw_xml[:pos] + insert + raw_xml[pos:]
    updated = re.sub(rb'count="[^"]*"', b'count="' + new_count + b'"', updated, count=1)
    updated = re.sub(rb'uniqueCount="[^"]*"', b'uniqueCount="' + new_count + b'"', updated, count=1)
    return updated, all_strings


# ── Sheet parsing ───────────────────────────────────────────────────

CELL_RE = re.compile(rb'<c r="([A-Z]+)\d+"([^>]*)>(?:<v>([^<]*)</v>|<f[^>]*>[^<]*</f>(?:<v>([^<]*)</v>)?)?')
ROW_RE = re.compile(rb'<row r="(\d+)"[^>]*>(.*?)</row>', re.DOTALL)


def parse_row_cells(content: bytes, strings: list[str]) -> dict[str, str | None]:
    cells = {}
    for cm in CELL_RE.finditer(content):
        col = cm.group(1).decode()
        attrs = cm.group(2) or b""
        val = cm.group(3) if cm.group(3) is not None else cm.group(4)
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


def find_marker_row(sheet_xml: bytes, strings: list[str]) -> tuple[int, int]:
    """Find marker row (where col B is 'Details' header). Returns (start_pos, row_num)."""
    try:
        details_idx = strings.index("Details")
    except ValueError:
        raise ValueError("'Details' shared string not found")
    cell_pat = re.compile(
        rb'<c r="B(\d+)"[^>]*t="s"[^>]*><v>' + str(details_idx).encode() + rb'</v></c>'
    )
    m = cell_pat.search(sheet_xml)
    if not m:
        cell_pat2 = re.compile(
            rb'<c r="B(\d+)" t="s"[^>]*><v>' + str(details_idx).encode() + rb'</v></c>'
        )
        m = cell_pat2.search(sheet_xml)
    if not m:
        raise ValueError("Marker row (Details header) not found")
    row_num = int(m.group(1))
    row_start_pat = re.compile(rb'<row r="' + str(row_num).encode() + rb'"[^>]*>')
    rm = row_start_pat.search(sheet_xml)
    return rm.start(), row_num


def build_dedup_index(sheet_xml: bytes, strings: list[str]) -> set[tuple[str, int, str]]:
    """Build set of (details_value, posting_date_serial, amount_str) for dedup."""
    seen = set()
    for rm in ROW_RE.finditer(sheet_xml):
        cells = parse_row_cells(rm.group(2), strings)
        details = cells.get("B")
        c = cells.get("C")
        e = cells.get("E")
        if not (details and c and e):
            continue
        try:
            ds = int(float(c))
            amt = float(e)
        except (ValueError, TypeError):
            continue
        seen.add((details.strip(), ds, f"{amt:.2f}"))
    return seen


# ── Site / link mapping ─────────────────────────────────────────────

SITE_KEYWORDS = {
    "1PASSWORD": "1password.com",
    "ATLASSIAN": "Atlassian.com",
    "CLOUDFLARE": "cloudflare.com",
    "HEROKU": "heroku.com",
    "INTUIT": "intuit",
    "MICROSOFT": "microsoft.com",
    "MSFT": "microsoft.com",
    "SEMRUSH": "Semrush.Com",
    "SLACK": "slack.com",
    "CIRCLECI": "circleci",
    "HUBSPOT": "hubspot.com",
    "WEWORK": "wework",
    "UBIQUITI": "ubiquiti",
    "EBAY": "ebay.com",
    "GOOGLE": "google",
    "ADOBE": "adobe",
    "AHREFS": "Ahrefs",
    "OPENAI": "openai",
    "AWS": "aws",
    "COGENT": "cogentISP",
    "BCBS": "BCBS",
    "HEALTH CARE SERV": "BCBS",
    "HCSC": "HCSC",
    "UHC": "UHC",
    "SYNCTIX": "synctix",
    "RAMP": "ramp.split",
    "ONPAY": "onpay.split",
    "DEEL": "deel.com",
    "APIARYX": "apiaryx.com",
    "BACANCY": "bacancy.com",
    "JOHNSON CONTROLS": "sales.jci",
    "RAPID TECHNIC": "sales.rapid",
    "HURRICANE": "hurricaneelectric.com",
    "BYG ADVANTAGE": "BYG",
    "BLT": "blt.com",
    "UNITED HEALTHCAR": "UHC",
    "DATAFOUNDRY": "switch",
    "DATA FOUNDRY": "switch",
    "ALICAT": "alicat",
    "ZENI": "zeni",
    "EAGLE POINT FUNDING": "eagle",
    "NISHIYAMA": "sales.jci",
}

SPECIAL_PATTERNS = [
    ("SERVICE CHARGES", "chase.com"),
    ("WIRE FEE", "chase.com"),
    ("ODP TRANSFER", "funding"),
    ("Online Transfer from CHK", "funding"),
    ("Online Transfer to CHK", "funding"),
    ("Transfer from Izuma Checking", "ramp.split"),
    ("MSPBNA BANK", "trial.deposits"),
    ("FOREIGN REMITTANCE", "sales.sb"),
]


INTERNAL_ACCTS = set(ACCOUNT_MAP)          # {"7505","3839","3211","6557"}
XFER_RE = re.compile(r"Online Transfer (?:from|to) CHK \.\.\.(\d{4})", re.I)


def determine_site(description: str) -> str:
    # Internal account transfer -> site = chase.<counterparty last4> (link becomes XFER).
    # External / non-Chase transfers fall through to the "funding" special pattern below.
    m = XFER_RE.search(description)
    if m and m.group(1) in INTERNAL_ACCTS:
        return f"chase.{m.group(1)}"
    desc_upper = description.upper()
    for needle, site in SPECIAL_PATTERNS:
        if needle.upper() in desc_upper:
            return site
    for keyword, site in SITE_KEYWORDS.items():
        if keyword in desc_upper:
            return site
    return ""


def link_from_site(site: str) -> str:
    if not site:
        return "Chase"
    s = site.lower()
    if s in ("ramp.split", "trial.deposits") or s.startswith("ramp."):
        return "Ramp"
    if s.startswith("onpay"):
        return "Onpay"
    if s.startswith("deel"):
        return "deel"
    if s == "funding" or s.startswith("chase.") or s.startswith("wells."):
        return "XFER" if (s == "funding" or s.startswith("chase.") or s.startswith("wells.")) and s != "chase.com" else ("XFER" if s == "funding" else "Chase")
    if s == "chase.com":
        return "Chase"
    if s == "cogentisp":
        return "cogent"
    if s.startswith("switch"):
        return "switch"
    if s.startswith("apiaryx"):
        return "apiaryx"
    if s.startswith("bacancy"):
        return "bacancy"
    if s == "bcbs":
        return "bcbs"
    if s.startswith("tlip"):
        return "tlip"
    if s == "safe.loan":
        return "safe"
    if s == "pelion":
        return "pelion"
    if s == "hcsc":
        return "HCSC"
    if s == "uhc":
        return "UHC"
    if s == "synctix":
        return "Chase"
    if s == "intuit":
        return "Chase"
    if s == "blt.com":
        return "BLT"
    if s == "byg":
        return "Chase"
    if s == "alicat":
        return "alicat"
    if s == "zeni":
        return "zeni"
    if s == "eagle":
        return "eagle"
    # sales.jci (Nishiyama / Johnson Controls revenue) -> default "Chase", like sales.sb / sales.rapid
    return "Chase"


def map_details(csv_details: str, code: str) -> str:
    d = csv_details.strip().upper()
    if d == "DEBIT":
        return f"DEBIT-{code}"
    if d == "CREDIT":
        return f"CREDIT-{code}"
    if d == "DSLIP":
        return f"CREDIT-{code}"
    return f"{d}-{code}"


# ── Main ────────────────────────────────────────────────────────────

def process_csv(path: str, dedup: set, strings: list[str]) -> tuple[list[dict], int, int]:
    """Returns (new_entries, total_rows, dup_rows)."""
    name = os.path.basename(path)
    m = FILENAME_RE.search(name)
    if not m:
        raise ValueError(f"Filename does not match Chase ALLREC pattern: {name}")
    last4 = m.group(2)
    code, account_name = ACCOUNT_MAP.get(last4, (None, None))
    if not code:
        raise ValueError(f"Unknown account last4={last4}")

    new = []
    total = 0
    dups = 0
    with open(path, newline="") as f:
        reader = csv.DictReader(f)
        for row in reader:
            total += 1
            details_csv = row["Details"]
            posting = parse_csv_date(row["Posting Date"])
            desc = row["Description"]
            try:
                amount = float(row["Amount"].replace(",", ""))
            except ValueError:
                amount = 0
            typ = row["Type"]
            details_full = map_details(details_csv, code)
            key = (details_full, date_to_serial(posting), f"{amount:.2f}")
            if key in dedup:
                dups += 1
                continue
            site = determine_site(desc)
            link = link_from_site(site)
            # Transfers pair their two legs via sublink X<YYMMDD>
            sublink = f"X{posting:%y%m%d}" if link == "XFER" else ""
            new.append({
                "details": details_full,
                "posting": posting,
                "description": desc,
                "amount": amount,
                "type": typ,
                "site": site,
                "link": link,
                "sublink": sublink,
                "account": code,
                "account_name": account_name,
                "csv": name,
            })
            dedup.add(key)  # avoid intra-batch dups
    return new, total, dups


def build_row_xml(row_num: int, e: dict, str_idx: dict[str, int]) -> str:
    cells = []
    cells.append(f'<c r="B{row_num}" t="s"><v>{str_idx[e["details"]]}</v></c>')
    cells.append(f'<c r="C{row_num}"><v>{date_to_serial(e["posting"])}</v></c>')
    cells.append(f'<c r="D{row_num}" t="s"><v>{str_idx[e["description"]]}</v></c>')
    cells.append(f'<c r="E{row_num}"><v>{e["amount"]}</v></c>')
    cells.append(f'<c r="F{row_num}" t="s"><v>{str_idx[e["type"]]}</v></c>')
    cells.append(f'<c r="G{row_num}" t="s"><v>{str_idx["Actual"]}</v></c>')
    if e["site"]:
        cells.append(f'<c r="H{row_num}" t="s"><v>{str_idx[e["site"]]}</v></c>')
    cells.append(f'<c r="I{row_num}" t="s"><v>{str_idx[e["link"]]}</v></c>')
    if e.get("sublink"):
        cells.append(f'<c r="J{row_num}" t="s"><v>{str_idx[e["sublink"]]}</v></c>')
    cells.append(f'<c r="K{row_num}"><v>{date_to_serial(e["posting"])}</v></c>')
    return f'<row r="{row_num}">{"".join(cells)}</row>'


def renumber_row(row_xml: bytes, old: int, new: int) -> bytes:
    o, n = str(old).encode(), str(new).encode()
    result = re.sub(rb'<row r="' + o + rb'"', b'<row r="' + n + b'"', row_xml)
    result = re.sub(rb'r="([A-Z]+)' + o + rb'"', rb'r="\g<1>' + n + b'"', result)
    return result


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


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("csv_files", nargs="+", help="Chase ALLREC CSV files")
    ap.add_argument("--apply", action="store_true", help="Write changes (default: dry-run)")
    args = ap.parse_args()

    with zipfile.ZipFile(SPREADSHEET, "r") as zf:
        strings, ss_raw = load_shared_strings(zf)
        sheet_raw = zf.read(DATA_SHEET_XML)

    dedup = build_dedup_index(sheet_raw, strings)
    print(f"Dedup index: {len(dedup)} (Details, date, amount) keys from data tab.")

    all_new = []
    for path in args.csv_files:
        new, total, dups = process_csv(path, dedup, strings)
        print(f"\n{os.path.basename(path)}: {total} rows, {dups} duplicates, {len(new)} new")
        for e in new:
            print(f"  {e['details']:14s} {e['posting'].strftime('%Y-%m-%d')} {e['amount']:>11.2f} "
                  f"site={e['site']:18s} link={e['link']:8s} {e['description'][:60]}")
        all_new.extend(new)

    if not all_new:
        print("\nNothing new to import.")
        return 0

    print(f"\n=== Total new rows to insert: {len(all_new)} ===")

    if not args.apply:
        print("\nDry-run only. Re-run with --apply to write.")
        return 0

    # Sort new rows by posting date (oldest first), insert before marker
    all_new.sort(key=lambda e: e["posting"])

    # Collect strings
    needed_strings = set(["Actual"])
    for e in all_new:
        needed_strings.add(e["details"])
        needed_strings.add(e["description"])
        needed_strings.add(e["type"])
        if e["site"]:
            needed_strings.add(e["site"])
        needed_strings.add(e["link"])
        if e.get("sublink"):
            needed_strings.add(e["sublink"])
    ss_updated, strings = add_shared_strings(ss_raw, strings, list(needed_strings))
    str_idx = {s: i for i, s in enumerate(strings)}

    # Find marker row, get its number BEFORE we modify the XML
    _, marker_row_num = find_marker_row(sheet_raw, strings)
    print(f"Marker row at row {marker_row_num}; inserting {len(all_new)} new rows above it.")

    # Renumber all rows >= marker_row_num to shift them down by N
    bump = len(all_new)
    pieces = []
    last_end = 0
    for rm in ROW_RE.finditer(sheet_raw):
        rn = int(rm.group(1))
        if rn >= marker_row_num:
            pieces.append(sheet_raw[last_end:rm.start()])
            pieces.append(renumber_row(rm.group(0), rn, rn + bump))
            last_end = rm.end()
    pieces.append(sheet_raw[last_end:])
    sheet_updated = b"".join(pieces)

    # Build new row XML at positions marker_row_num .. marker_row_num + N - 1
    new_xml_pieces = []
    for offset, e in enumerate(all_new):
        new_xml_pieces.append(build_row_xml(marker_row_num + offset, e, str_idx))
    insert_blob = "".join(new_xml_pieces).encode("utf-8")

    # Find new marker position
    _, new_marker_row = find_marker_row(sheet_updated, strings)
    row_pat = re.compile(rb'<row r="' + str(new_marker_row).encode() + rb'"[^>]*>')
    rm = row_pat.search(sheet_updated)
    insert_pos = rm.start()
    sheet_final = sheet_updated[:insert_pos] + insert_blob + sheet_updated[insert_pos:]

    # Update dimension
    dim = re.search(rb'<dimension ref="([A-Z]+\d+):([A-Z]+)(\d+)"', sheet_final)
    if dim:
        old_max = int(dim.group(3))
        sheet_final = sheet_final[:dim.start(3)] + str(old_max + bump).encode() + sheet_final[dim.end(3):]

    save_xlsx({DATA_SHEET_XML: sheet_final, SHARED_STRINGS_XML: ss_updated})
    print(f"\nSaved. Inserted {len(all_new)} new rows at {marker_row_num}–{marker_row_num + bump - 1}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
