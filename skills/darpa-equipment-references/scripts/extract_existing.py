#!/usr/bin/env python3
"""Extract existing EQ entries from an Equipment List .docx so that
explanations and product names can be preserved across edits.

Output JSON: a list of records keyed in order, each with:
    {"eq": "EQ001", "title": "...", "product": "...", "vendor": "...",
     "cost_qty": "$1,114 × 1 = $1,114", "explanation": "...", "link": "..."}

Usage:
    extract_existing.py <docx-path> [--out <json>]
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


def extract(docx_path: Path) -> list[dict]:
    try:
        from docx import Document
    except ImportError:
        sys.exit("ERROR: python-docx not installed. pip install python-docx")

    doc = Document(docx_path)
    # Flatten paragraph text, collapsing the run-internal newlines that
    # python-docx surfaces with `\n` inside a single paragraph.
    blob = "\n".join(p.text for p in doc.paragraphs)
    # Normalize odd whitespace and zero-width chars
    blob = blob.replace(" ", " ").replace("​", "")

    # Each entry starts with EQ### followed by " - " or " – " (en dash) then title.
    pattern = re.compile(
        r"^EQ(\d{3})\s*[-–]\s*(.+?)$",
        re.MULTILINE,
    )

    entries: list[dict] = []
    matches = list(pattern.finditer(blob))
    for i, m in enumerate(matches):
        start = m.end()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(blob)
        body = blob[start:end]

        def field(label: str) -> str:
            mm = re.search(
                rf"(?im)^\s*{re.escape(label)}\s*:?\s*(.*?)(?=\n\s*(?:Product|Vendor|Cost\s*&\s*Quantity|Explanation|Link)\s*:|\Z)",
                body,
                re.DOTALL,
            )
            if not mm:
                return ""
            return re.sub(r"\s+", " ", mm.group(1)).strip()

        entries.append({
            "eq": f"EQ{m.group(1)}",
            "title": m.group(2).strip(),
            "product": field("Product"),
            "vendor": field("Vendor"),
            "cost_qty": field("Cost & Quantity"),
            "explanation": field("Explanation"),
            "link": field("Link"),
        })

    return entries


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("docx", type=Path)
    ap.add_argument("--out", type=Path, default=None)
    args = ap.parse_args()

    if not args.docx.exists():
        sys.exit(f"ERROR: docx not found: {args.docx}")

    entries = extract(args.docx)
    payload = json.dumps(entries, indent=2, ensure_ascii=False)
    if args.out:
        args.out.write_text(payload, encoding="utf-8")
        print(f"Wrote {len(entries)} existing entries → {args.out}")
    else:
        print(payload)


if __name__ == "__main__":
    main()
