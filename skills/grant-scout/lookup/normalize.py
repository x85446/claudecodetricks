#!/usr/bin/env python3
"""
normalize.py — read each source's raw/ records and emit a unified normalized.jsonl.

Usage:
    normalize.py <corpus-dir> [source...]

With no [source...] args, every source auto-discovered from the normalizers/
package (see normalizers/__init__.py) is normalized. Adding a new source is a
matter of dropping a new normalizers/<name>.py module — this driver, merge.py,
and fetch_all.sh's normalize step all pick it up with no further editing.

Unified schema (one JSON object per line):
    {
      "source": "sam.gov" | "darpa.mil" | "dsip" | "grants.gov" | ...,
      "source_id": "<native ID>",
      "solicitation_number": "<canonical>",
      "title": "...",
      "agency": "DoD" | "NSF" | ...,
      "subagency": "DARPA SBPO" | ...,
      "topic_text": "<abstract / synopsis body>",
      "url": "https://...",
      "posted_date": "YYYY-MM-DD",
      "close_date": "YYYY-MM-DD",
      "phase": "SBIR Phase I" | "BAA" | "Contract" | ...,
      "funding_ceiling": null | float,
      "eligibility": "<verbatim>",
      "raw_path": "<absolute path to raw file>"
    }
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import normalizers  # noqa: E402


def main():
    if len(sys.argv) < 2:
        print("usage: normalize.py <corpus-dir> [source...]", file=sys.stderr)
        sys.exit(2)
    corpus = sys.argv[1]
    available = normalizers.discover()
    sources = sys.argv[2:] or sorted(available)

    for src in sources:
        if src not in available:
            print(f"[normalize] unknown source: {src}")
            continue
        mod = normalizers.get(src)
        out_path = os.path.join(corpus, mod.DIR, 'normalized.jsonl')
        os.makedirs(os.path.dirname(out_path), exist_ok=True)
        n = 0
        with open(out_path, 'w') as out:
            for rec in mod.normalize(corpus):
                out.write(json.dumps(rec, ensure_ascii=False) + '\n')
                n += 1
        print(f"[normalize] {src}: {n} records → {out_path}")


if __name__ == '__main__':
    main()
