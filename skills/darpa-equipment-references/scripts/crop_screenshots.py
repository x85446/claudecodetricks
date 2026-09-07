#!/usr/bin/env python3
"""Content-aware crop pass on the cached product-page screenshots.

Goal: keep the product hero (image + name + price + description), trim away
the "Related items" / reviews / footer that follows, and remove blank side
gutters. Operates on the cache only — does not re-fetch.

Usage:
    crop_screenshots.py --items items.json [--cache <dir>] [--force]

For each item with a `screenshot` PNG in the cache, writes a cropped
sibling file `<basename>_crop.png` and rewrites `item["screenshot"]` to
point at it. Items without a cached screenshot are left untouched.

Heuristic:
    1. Walk down the rows from top to bottom. Each row gets a "density"
       score = fraction of pixels that aren't near-white.
    2. Ignore the top 30% of the page (this is the hero — we always keep it).
    3. After the 30% mark, find the first sustained low-density gap
       (default: 25+ consecutive rows with density < 2%). That's the bottom
       boundary of the main-content area.
    4. If no such gap exists, fall back to 55% of image height.
    5. Trim left/right whitespace by column-density the same way.
    6. Crop to [left..right, 0..cutoff] and save.

Tunables: --gap-rows, --row-threshold, --col-threshold, --max-top-fraction,
--min-top-fraction.
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

DEFAULT_CACHE = Path(__file__).resolve().parent.parent / "cache" / "screenshots"


def find_cutoff(density, *, min_frac: float, max_frac: float,
                threshold: float, gap_rows: int) -> int:
    """Return the row index to crop at. Searches for the first sustained
    low-density gap inside the [min_frac, max_frac] band; if none, falls
    back to max_frac. The hard cap at max_frac prevents the search from
    drifting deep into the page on sites with no whitespace gaps
    (e.g. Amazon's tightly-packed PDP layouts)."""
    n = len(density)
    start = int(min_frac * n)
    cap = int(max_frac * n)
    end = min(n - gap_rows, cap)
    i = start
    while i < end:
        window = density[i:i + gap_rows]
        if all(d < threshold for d in window):
            return i
        i += 1
    return cap


def trim_columns(col_density, threshold: float, pad: int, width: int) -> tuple[int, int]:
    nonempty = [i for i, d in enumerate(col_density) if d > threshold]
    if not nonempty:
        return 0, width
    left = max(0, nonempty[0] - pad)
    right = min(width, nonempty[-1] + pad)
    return left, right


def crop_one(src: Path, dst: Path, *, gap_rows: int, row_threshold: float,
             col_threshold: float, min_top: float, max_top: float) -> tuple[int, int, int, int]:
    from PIL import Image
    import numpy as np

    img = Image.open(src).convert("RGB")
    arr = np.array(img)
    h, w, _ = arr.shape

    # "Dark" = any pixel where R+G+B < 720 (i.e. not pure white). This catches
    # text, images, anything with colour. Pure-white background pixels sum to
    # 765, the threshold 720 gives us ~6% leeway for JPEG-ish off-white.
    dark = arr.sum(axis=2) < 720
    row_density = dark.sum(axis=1) / w
    col_density = dark.sum(axis=0) / h

    cutoff = find_cutoff(
        row_density.tolist(),
        min_frac=min_top,
        max_frac=max_top,
        threshold=row_threshold,
        gap_rows=gap_rows,
    )

    # Trim trailing whitespace from the cropped region (in case the cutoff
    # lands in a long gap, we'd ship a lot of dead pixels).
    while cutoff > int(min_top * h) and row_density[cutoff - 1] < row_threshold:
        cutoff -= 1

    left, right = trim_columns(col_density.tolist(), col_threshold, pad=12, width=w)

    cropped = img.crop((left, 0, right, cutoff))
    dst.parent.mkdir(parents=True, exist_ok=True)
    cropped.save(dst, format="PNG", optimize=True)
    return left, right, 0, cutoff


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--items", type=Path, required=True)
    ap.add_argument("--cache", type=Path, default=DEFAULT_CACHE)
    ap.add_argument("--force", action="store_true",
                    help="Re-crop even if the *_crop.png file already exists.")
    ap.add_argument("--gap-rows", type=int, default=25,
                    help="Number of consecutive low-density rows that mark "
                         "the end of the hero section.")
    ap.add_argument("--row-threshold", type=float, default=0.02,
                    help="Row is 'empty' if dark-pixel fraction < this.")
    ap.add_argument("--col-threshold", type=float, default=0.005,
                    help="Column is 'empty' if dark-pixel fraction < this.")
    ap.add_argument("--min-top-fraction", type=float, default=0.30,
                    help="Search for the cutoff starting at this fraction of image height.")
    ap.add_argument("--max-top-fraction", type=float, default=0.55,
                    help="Fall back to this fraction of image height when no gap is found.")
    args = ap.parse_args()

    try:
        import PIL  # noqa: F401
        import numpy  # noqa: F401
    except ImportError as e:
        sys.exit(f"ERROR: {e}. Install with: pip install Pillow numpy")

    items = json.loads(args.items.read_text(encoding="utf-8"))

    cropped = reused = skipped = 0
    for it in items:
        path = it.get("screenshot")
        if not path:
            skipped += 1
            continue
        src = Path(path)
        # If items.json already points at a *_crop.png from a prior run, walk
        # back to the original so we always crop from the full screenshot.
        if src.stem.endswith("_crop"):
            original = src.with_name(src.stem[: -len("_crop")] + src.suffix)
            if original.exists():
                src = original
        if not src.exists():
            skipped += 1
            continue
        dst = src.with_name(src.stem + "_crop.png")
        if dst.exists() and not args.force:
            it["screenshot"] = str(dst)
            reused += 1
            continue
        try:
            l, r, t, b = crop_one(
                src, dst,
                gap_rows=args.gap_rows,
                row_threshold=args.row_threshold,
                col_threshold=args.col_threshold,
                min_top=args.min_top_fraction,
                max_top=args.max_top_fraction,
            )
            it["screenshot"] = str(dst)
            cropped += 1
            print(f"  EQ{it.get('no', '?'):03d}  cropped to [{l}..{r}] × [0..{b}]  → {dst.name}",
                  file=sys.stderr)
        except Exception as e:
            print(f"  EQ{it.get('no', '?'):03d}  FAILED  {e}", file=sys.stderr)
            skipped += 1

    args.items.write_text(json.dumps(items, indent=2, ensure_ascii=False), encoding="utf-8")
    print(f"\ncropped={cropped} reused={reused} skipped={skipped} total={len(items)}",
          file=sys.stderr)
    print(f"items JSON updated → {args.items}", file=sys.stderr)


if __name__ == "__main__":
    main()
