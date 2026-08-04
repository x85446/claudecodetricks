#!/usr/bin/env python3
"""Detect bot-blocked headless screenshots and re-capture them by driving
the user's real Google Chrome via AppleScript + screencapture.

Workflow:
    1. Read items.json (parse_input.py + cache_screenshots.py output).
    2. For each item, OCR its cached PNG and check for known anti-bot
       strings ("Performing security verification", "Just a moment",
       "Verifying you are human", "Request blocked", CloudFront 403, etc).
    3. For each blocked item, invoke `chrome_capture.sh <url> <original-png>`
       which opens a real Chrome window, waits for the Cloudflare challenge
       to pass, and screencaptures the window region.
    4. Re-write the items.json with `cloudflare_blocked` annotations.

After this step, run `crop_screenshots.py --force` (so the cropped
versions get refreshed from the new originals) and `find_prices.py`.

Usage:
    recapture_blocked.py --items items.json
                         [--script chrome_capture.sh]
                         [--max-wait 30]
                         [--dry-run]
"""
from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

# Phrases that indicate the cached screenshot is an interstitial / block
# page rather than the real product page.
BLOCK_PATTERNS = [
    r"performing\s+security\s+verification",
    r"verifying\s+you\s+(?:are\s+(?:not\s+)?(?:a\s+)?(?:bot|human))",
    r"verifying\s+that\s+you\s+are",
    r"just\s+a\s+moment",
    r"checking\s+your\s+browser",
    r"checking\s+if\s+the\s+site\s+connection\s+is\s+secure",
    r"please\s+enable\s+(?:cookies\s+and\s+)?javascript",
    r"cloudflare\s+ray\s+id",
    r"generated\s+by\s+cloudfront",
    r"the\s+request\s+could\s+not\s+be\s+satisfied",
    r"access\s+denied",
    r"403\s+error",
    r"403\s+forbidden",
    r"attention\s+required.*cloudflare",
    r"why\s+have\s+i\s+been\s+blocked",
]
BLOCK_RE = re.compile("|".join(BLOCK_PATTERNS), re.IGNORECASE)


def looks_blocked(png_path: Path) -> tuple[bool, str]:
    """Return (is_blocked, matched_phrase). False if OCR fails — we err on
    the side of treating opaque output as legitimate to avoid recapturing
    things unnecessarily."""
    try:
        r = subprocess.run(
            ["tesseract", str(png_path), "-", "-l", "eng", "--psm", "6"],
            capture_output=True, timeout=20,
        )
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False, ""
    if r.returncode != 0:
        return False, ""
    text = r.stdout.decode("utf-8", errors="replace")
    m = BLOCK_RE.search(text)
    if m:
        return True, m.group(0)
    return False, ""


def original_png_for(screenshot_path: str) -> Path:
    """items.json's `screenshot` often points at `<sha1>_crop.png`. Walk
    back to the original `<sha1>.png` — that's what chrome_capture.sh
    overwrites."""
    p = Path(screenshot_path)
    if p.stem.endswith("_crop"):
        return p.with_name(p.stem[: -len("_crop")] + p.suffix)
    return p


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--items", type=Path, required=True)
    ap.add_argument("--script", type=Path,
                    default=Path(__file__).resolve().parent / "chrome_capture.sh",
                    help="Path to chrome_capture.sh (default: alongside this script).")
    ap.add_argument("--max-wait", type=int, default=30,
                    help="Seconds to wait for Cloudflare challenge to clear.")
    ap.add_argument("--dry-run", action="store_true",
                    help="Detect blocked pages and report — don't recapture.")
    args = ap.parse_args()

    if not args.script.exists():
        sys.exit(f"ERROR: {args.script} not found")
    if not shutil.which("tesseract"):
        sys.exit("ERROR: tesseract not installed. brew install tesseract")
    if not shutil.which("osascript") or not shutil.which("screencapture"):
        sys.exit("ERROR: macOS osascript / screencapture not available — this fallback is macOS-only")

    items = json.loads(args.items.read_text(encoding="utf-8"))

    # Phase 1 — detect
    blocked: list[tuple[dict, Path]] = []
    for it in items:
        screenshot = it.get("screenshot")
        if not screenshot:
            it["cloudflare_blocked"] = False
            continue
        original = original_png_for(screenshot)
        if not original.exists():
            it["cloudflare_blocked"] = False
            continue
        is_blocked, phrase = looks_blocked(original)
        it["cloudflare_blocked"] = is_blocked
        if is_blocked:
            blocked.append((it, original))
            print(f"  BLOCKED  EQ{it.get('no', '?'):03d}  matched={phrase!r}\n"
                  f"           {it.get('description', '')[:70]}\n"
                  f"           {it.get('link', '')[:100]}", file=sys.stderr)

    print(f"\n{len(blocked)} of {len(items)} screenshots flagged as bot-blocked.",
          file=sys.stderr)

    if args.dry_run or not blocked:
        args.items.write_text(json.dumps(items, indent=2, ensure_ascii=False), encoding="utf-8")
        print("(dry-run / nothing to do — items JSON updated with cloudflare_blocked flags)",
              file=sys.stderr)
        return

    # Phase 2 — recapture each blocked item via real Chrome
    print(f"\nRecapturing {len(blocked)} blocked items via real Chrome. "
          f"Don't move Chrome windows during capture.", file=sys.stderr)
    recaptured = 0
    for it, original in blocked:
        url = it.get("link", "")
        if not url:
            continue
        print(f"\n→ EQ{it['no']:03d}  {url}", file=sys.stderr)
        try:
            r = subprocess.run(
                [str(args.script), url, str(original), str(args.max_wait)],
                check=False, timeout=args.max_wait + 30,
            )
        except subprocess.TimeoutExpired:
            print(f"   timeout running chrome_capture.sh", file=sys.stderr)
            continue
        if r.returncode == 0:
            # Re-check: did the new capture succeed in dodging the block?
            still_blocked, _ = looks_blocked(original)
            if still_blocked:
                print(f"   WARN: page still looks blocked after recapture. "
                      f"Maybe the wait was too short — try a longer --max-wait.",
                      file=sys.stderr)
            else:
                it["cloudflare_blocked"] = False
                recaptured += 1
                print(f"   ✓ recaptured", file=sys.stderr)
        else:
            print(f"   chrome_capture.sh exited {r.returncode}", file=sys.stderr)

    args.items.write_text(json.dumps(items, indent=2, ensure_ascii=False), encoding="utf-8")
    print(f"\nRecaptured {recaptured}/{len(blocked)} items.", file=sys.stderr)
    print(f"items JSON updated → {args.items}", file=sys.stderr)
    print("\nNext: run crop_screenshots.py --force, then find_prices.py.", file=sys.stderr)


if __name__ == "__main__":
    main()
