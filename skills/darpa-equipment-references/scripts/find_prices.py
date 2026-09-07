#!/usr/bin/env python3
"""OCR each cached product-page screenshot and flag items whose page does
not surface a clear price for the actual product.

Usage:
    find_prices.py --items items.json [--report report.json] [--tolerance 0.30]

For each item with a `screenshot` path:
  * Runs Tesseract OCR on the image.
  * Extracts `$\\d+(\\.\\d{1,2})?` tokens (plus € / £).
  * Compares those tokens to the spreadsheet `cost_per_item` (treated as
    ground truth). A token within ±`tolerance` of the expected cost
    counts as confirmation that the *real* product price is rendered.

Classification:
    ok       — at least one detected price within ±tolerance of expected
    suspect  — prices were detected but none match the expected cost
               (e.g. Amazon surfacing alternative-item prices instead of
                the listing itself)
    missing  — no currency tokens detected at all

The price-vs-cost match is more robust than buy-button keyword detection
because Tesseract is unreliable at reading button text on colored
backgrounds, but it reliably reads large dark-on-light price strings.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path


MONEY_RE = re.compile(
    r"(?:US\$|USD\s*|EUR\s*|€|£|¥|\$)\s*([\d,]+(?:\.\d{1,2})?)",
    re.IGNORECASE,
)


def ocr(img: Path, timeout: int = 30) -> str:
    """Return tesseract's text output for the image, ignoring stderr."""
    try:
        r = subprocess.run(
            ["tesseract", str(img), "-", "-l", "eng", "--psm", "6"],
            capture_output=True, timeout=timeout,
        )
    except FileNotFoundError:
        sys.exit("ERROR: tesseract not installed. brew install tesseract")
    except subprocess.TimeoutExpired:
        return ""
    return r.stdout.decode("utf-8", errors="replace") if r.returncode == 0 else ""


def extract_prices(text: str) -> tuple[list[str], list[float]]:
    """Return parallel (formatted, numeric) lists of unique prices found."""
    raw = MONEY_RE.findall(text)
    fmt: list[str] = []
    vals: list[float] = []
    seen_vals: set[float] = set()
    for p in raw:
        p = p.strip()
        if not p:
            continue
        try:
            v = float(p.replace(",", ""))
        except ValueError:
            continue
        if v < 1 or v in seen_vals:
            continue
        seen_vals.add(v)
        fmt.append(f"${p}")
        vals.append(v)
    return fmt, vals


def classify(prices_fmt: list[str], prices_val: list[float],
             expected_cost: float, tolerance: float) -> tuple[str, float | None]:
    """Return (status, best_match_price). Status is 'ok', 'suspect', or 'missing'."""
    if not prices_val:
        return "missing", None
    if not expected_cost or expected_cost <= 0:
        # No ground truth to compare against; if any price was found, treat as ok.
        return "ok", prices_val[0]
    best = min(prices_val, key=lambda v: abs(v - expected_cost) / expected_cost)
    delta = abs(best - expected_cost) / expected_cost
    if delta <= tolerance:
        return "ok", best
    return "suspect", best


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--items", type=Path, required=True)
    ap.add_argument("--report", type=Path, default=None,
                    help="Optional path to write the price-status report as JSON.")
    ap.add_argument("--tolerance", type=float, default=0.30,
                    help="Allowed fractional delta between detected price and "
                         "expected cost_per_item (default 0.30 = ±30%%).")
    args = ap.parse_args()

    items = json.loads(args.items.read_text(encoding="utf-8"))

    report = []
    for it in items:
        path = it.get("screenshot")
        expected = float(it.get("cost_per_item") or 0)
        if not path or not Path(path).exists():
            it["price_status"] = "no_screenshot"
            it["prices_found"] = []
            it["price_best_match"] = None
            report.append({"no": it.get("no"), "status": "no_screenshot",
                           "title": it.get("description", "")})
            continue
        text = ocr(Path(path))
        prices_fmt, prices_val = extract_prices(text)
        status, best = classify(prices_fmt, prices_val, expected, args.tolerance)
        it["prices_found"] = prices_fmt
        it["price_best_match"] = best
        it["price_status"] = status
        report.append({
            "no": it.get("no"),
            "title": it.get("description", ""),
            "vendor": it.get("vendor", ""),
            "link": it.get("link", ""),
            "expected_cost": expected,
            "prices_found": prices_fmt,
            "best_match": best,
            "status": status,
        })

    args.items.write_text(json.dumps(items, indent=2, ensure_ascii=False), encoding="utf-8")
    if args.report:
        args.report.write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")

    counts = {"ok": 0, "suspect": 0, "missing": 0, "no_screenshot": 0}
    for r in report:
        counts[r["status"]] = counts.get(r["status"], 0) + 1
    print(f"\nPrice detection: ok={counts['ok']}  suspect={counts['suspect']}  "
          f"missing={counts['missing']}  no_screenshot={counts['no_screenshot']}",
          file=sys.stderr)
    for r in report:
        if r["status"] in ("missing", "suspect"):
            tag = "MISSING" if r["status"] == "missing" else "SUSPECT"
            prices = ", ".join(r.get("prices_found", [])) or "—"
            best = r.get("best_match")
            best_str = f"${best:g}" if best is not None else "—"
            print(f"  {tag}  EQ{r['no']:03d}  expected=${r.get('expected_cost', 0):g}  "
                  f"prices_in_image={prices}  best={best_str}\n"
                  f"          {r['title'][:70]}\n"
                  f"          {r.get('link', '')[:100]}", file=sys.stderr)
    print(f"\nitems JSON updated → {args.items}", file=sys.stderr)


if __name__ == "__main__":
    main()
