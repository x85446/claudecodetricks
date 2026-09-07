#!/usr/bin/env python3
"""
file-chase-statements: Auto-file Chase monthly statement PDFs from
incoming-temp/ to processed/. Strict filename pattern; no spreadsheet
action.
"""

import argparse
import datetime
import json
import re
import shutil
import sys
from pathlib import Path

INCOMING_TEMP = Path("/workspace/processing/incoming-temp")
PROCESSED = Path("/workspace/processing/processed")
TRACKING = Path("/workspace/fintool/.claude/skills/fileclerk-data/data/processed_files.json")

PATTERN = re.compile(r"^\d{6} \$[\d,.\-]+ Chase \d{4} (checking|savings)\.pdf$")


def load_tracking():
    if not TRACKING.exists():
        return {"metadata": {}, "files": {}}
    try:
        d = json.loads(TRACKING.read_text())
        d.setdefault("files", {})
        d.setdefault("metadata", {})
        return d
    except Exception:
        return {"metadata": {}, "files": {}}


def save_tracking(data):
    data["metadata"]["last_updated"] = datetime.datetime.utcnow().isoformat()
    TRACKING.write_text(json.dumps(data, indent=2))


def parse_filename(fn):
    """Parse '240131 $260,813.96 Chase 6557 checking.pdf' into structured fields."""
    m = re.match(
        r"^(?P<date>\d{6}) \$(?P<amount>[\d,.\-]+) Chase (?P<last4>\d{4}) (?P<acct>checking|savings)\.pdf$",
        fn,
    )
    if not m:
        return None
    amount = float(m.group("amount").replace(",", ""))
    return {
        "date": m.group("date"),
        "amount": amount,
        "vendor": "Chase",
        "extension": "pdf",
        "type": "monthly statement",
        "last4": m.group("last4"),
        "account_kind": m.group("acct"),
    }


def main():
    ap = argparse.ArgumentParser(description="Auto-file Chase statement PDFs.")
    ap.add_argument("--apply", action="store_true", help="Move files (default is dry-run).")
    args = ap.parse_args()

    if not INCOMING_TEMP.exists():
        print(f"ERROR: source folder missing: {INCOMING_TEMP}", file=sys.stderr)
        sys.exit(2)
    if not PROCESSED.exists():
        print(f"ERROR: destination folder missing: {PROCESSED}", file=sys.stderr)
        sys.exit(2)

    all_files = sorted([p.name for p in INCOMING_TEMP.iterdir() if p.is_file() and not p.name.startswith(".")])
    matches = [fn for fn in all_files if PATTERN.match(fn)]
    non_matches = [fn for fn in all_files if not PATTERN.match(fn)]

    mode = "apply" if args.apply else "dry-run"
    print("=== file-chase-statements ===")
    print(f"Mode: {mode}")
    print(f"incoming-temp/ scanned: {len(all_files)} files")
    print(f"Matched (Chase statement PDF): {len(matches)}")

    if not matches:
        print("Nothing to do.")
        return

    if not args.apply:
        for fn in matches:
            print(f"  would file: {fn}")
        print(f"\n  → would file: {len(matches)}")
        print(f"  → non-matches left for fileclerk-data: {len(non_matches)}")
        print("\nDry-run only. Re-run with --apply to move.")
        return

    tracking = load_tracking()

    filed = 0
    collisions = []
    timestamp = datetime.datetime.utcnow().isoformat()

    for fn in matches:
        src = INCOMING_TEMP / fn
        dst = PROCESSED / fn
        if dst.exists():
            collisions.append(fn)
            print(f"  COLLISION (left in incoming-temp): {fn}")
            continue
        shutil.move(str(src), str(dst))
        tracking["files"][fn] = {
            "status": "filed",
            "parsed": parse_filename(fn) or {},
            "match_info": "Monthly Chase statement PDF — filed as period evidence; no specific row link.",
            "added": timestamp,
        }
        filed += 1
        print(f"  filed: {fn}")

    save_tracking(tracking)

    print()
    print(f"  → filed: {filed}")
    print(f"  → collisions: {len(collisions)}")
    print(f"  → non-matches left for fileclerk-data: {len(non_matches)}")


if __name__ == "__main__":
    main()
