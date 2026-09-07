#!/usr/bin/env python3
"""Fetch and cache a Chrome-headless screenshot for each item's link.

Usage:
    cache_screenshots.py --items items.json [--cache <dir>] [--force]
                         [--width 1280] [--height 1600] [--budget-ms 8000]

Writes a PNG per item to:
    <cache>/<sha1-of-canonical-url>.png

…and annotates each item in the JSON with:
    item["screenshot"] = <absolute path to cached PNG>

When the cache file already exists and `--force` is not set, the screenshot
is reused. Items with no link or with a fetch failure are left with
`item["screenshot"] = None`.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

DEFAULT_CACHE = Path(__file__).resolve().parent.parent / "cache" / "screenshots"

CHROME_CANDIDATES = [
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
]


def find_chrome() -> str | None:
    for p in CHROME_CANDIDATES:
        if os.access(p, os.X_OK):
            return p
    for name in ("google-chrome", "chromium", "chromium-browser"):
        path = shutil.which(name)
        if path:
            return path
    return None


def cache_key(url: str) -> str:
    h = hashlib.sha1(url.encode("utf-8")).hexdigest()[:16]
    return h


def fetch_one(chrome: str, url: str, out: Path, width: int, height: int,
              budget_ms: int) -> bool:
    out.parent.mkdir(parents=True, exist_ok=True)
    cmd = [
        chrome,
        "--headless=new",
        "--disable-gpu",
        "--no-sandbox",
        "--hide-scrollbars",
        "--disable-extensions",
        "--disable-software-rasterizer",
        f"--window-size={width},{height}",
        f"--virtual-time-budget={budget_ms}",
        f"--screenshot={out}",
        url,
    ]
    try:
        result = subprocess.run(cmd, capture_output=True, timeout=45)
    except subprocess.TimeoutExpired:
        print(f"  TIMEOUT  {url}", file=sys.stderr)
        return False
    if result.returncode != 0:
        print(f"  ERR rc={result.returncode}  {url}", file=sys.stderr)
        return False
    if not out.exists() or out.stat().st_size < 1024:
        print(f"  EMPTY  {url}", file=sys.stderr)
        if out.exists():
            out.unlink()
        return False
    return True


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--items", type=Path, required=True)
    ap.add_argument("--cache", type=Path, default=DEFAULT_CACHE)
    ap.add_argument("--force", action="store_true",
                    help="Re-fetch screenshots even if a cached file exists.")
    ap.add_argument("--width", type=int, default=1280)
    ap.add_argument("--height", type=int, default=1600)
    ap.add_argument("--budget-ms", type=int, default=8000)
    args = ap.parse_args()

    chrome = find_chrome()
    if not chrome:
        sys.exit("ERROR: Chrome/Chromium not found. Install Google Chrome or pass --skip-screenshots to build_docx.py.")

    items = json.loads(args.items.read_text(encoding="utf-8"))
    args.cache.mkdir(parents=True, exist_ok=True)

    fetched = reused = failed = 0
    for it in items:
        url = it.get("link", "").strip()
        if not url:
            it["screenshot"] = None
            failed += 1
            print(f"  skip (no link)  EQ{it.get('no', '?'):03d}  {it.get('description', '')[:60]}", file=sys.stderr)
            continue
        out = args.cache / f"{cache_key(url)}.png"
        if out.exists() and not args.force:
            it["screenshot"] = str(out)
            reused += 1
            continue
        ok = fetch_one(chrome, url, out, args.width, args.height, args.budget_ms)
        if ok:
            it["screenshot"] = str(out)
            fetched += 1
            print(f"  fetched  EQ{it.get('no', '?'):03d}  {url}", file=sys.stderr)
        else:
            it["screenshot"] = None
            failed += 1

    args.items.write_text(json.dumps(items, indent=2, ensure_ascii=False), encoding="utf-8")
    print(f"\nfetched={fetched} reused={reused} failed={failed} total={len(items)}", file=sys.stderr)
    print(f"items JSON updated → {args.items}", file=sys.stderr)


if __name__ == "__main__":
    main()
