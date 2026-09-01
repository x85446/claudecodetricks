---
name: check-images
description: "Process E*TRADE check and deposit images: stitch front+back into composites, generate thumbnails, import from incoming, and show status. Use when someone says 'stitch checks', 'process check images', 'check image status', or 'import check images'."
argument-hint: [stitch | import | status | clean]
disable-model-invocation: false
---

# Check & Deposit Image Processor

Process E*TRADE check and deposit images — stitch composites, generate thumbnails, import from incoming, and manage the pipeline.

## Invocation

```
/-check-images              # Stitch all pending composites + generate thumbnails
/-check-images stitch       # Same as above
/-check-images import       # Import check/deposit images from data/renamed/ → assets/checks/
/-check-images status       # Show pipeline status
/-check-images clean        # Remove old thumbnails/cache entries and re-stitch
```

## Prerequisites

**Pillow is required.** Before doing any image work, ensure it's installed:

```bash
python3 -c "from PIL import Image; print('Pillow OK')" 2>/dev/null || python3 -m pip install Pillow
```

If `pip` is unavailable, try:
```bash
pip3 install Pillow
```

If neither works, tell the user to install Pillow manually and retry.

## Paths

| Path | Purpose |
|---|---|
| `assets/checks/` | Raw check/deposit images and composites |
| `assets/thumbnails/` | 240x240 WebP thumbnails |
| `db/personaldb.sqlite` | `image_cache` table |
| `scripts/stitch_check_images.py` | Stitching script |
| `scripts/import_check_images.py` | Import/scan script |

## File Naming Conventions

Filenames include the account suffix (last 4 digits, e.g., `1452`) for multi-account support.

| Format | Example | Description |
|---|---|---|
| `YYMMDD_etchk_ACCT_NNN_$amt_front.ext` | `260302_etchk_1452_770_$3,800.00_front.png` | Check front |
| `YYMMDD_etchk_ACCT_NNN_$amt_back.ext` | `260302_etchk_1452_770_$3,800.00_back.png` | Check back |
| `YYMMDD_etbillpaycheck_ACCT_NNN_$amt_front.ext` | `170113_etbillpaycheck_1452_70115_$46.20_front.png` | Bill pay check front |
| `YYMMDD_etdeposit_ACCT_N_$amt_slip.ext` | `090911_etdeposit_1452_5_$3,000.00_slip.png` | Deposit slip |
| `YYMMDD_etdeposit_ACCT_N_$amt_checkN.ext` | `090911_etdeposit_1452_5_$3,000.00_check1.png` | Deposited check |
| `etrade-ACCT_check_NNN_composite.png` | `etrade-1452_check_770_composite.png` | Stitched composite |
| `etrade-ACCT_deposit_YYMMDD_amt_composite.png` | `etrade-1452_deposit_090911_3000.00_composite.png` | Stitched deposit composite |

Old filenames without account suffix default to account `1452`.

## Cache Key Scheme

Cache keys include the bank and account for multi-account/multi-bank support.

| Type | Cache key | Points to |
|---|---|---|
| Check | `etrade-1452:check:770` | `local:checks/etrade-1452_check_770_composite.png` |
| Deposit | `etrade-1452:deposit:090911:3000.00` | `local:checks/etrade-1452_deposit_090911_3000.00_composite.png` |

## Workflows

### Stitch (default)

This is the primary operation. Stitches raw images into composites and generates thumbnails.

```bash
python3 scripts/stitch_check_images.py
```

What it does:
1. Scans `assets/checks/` for raw check/deposit images
2. Groups them by check number or deposit date+amount
3. For checks: stitches front (top) + back (bottom) vertically
4. For deposits: stitches slip (top) + check1 + check2 + ... vertically
5. Saves composite as `*_composite.png` in `assets/checks/`
6. Generates 240x240 WebP thumbnail in `assets/thumbnails/`
7. Updates `image_cache` to point to the composite
8. Removes old back/checkN cache entries (composite replaces them)

To re-stitch everything:
```bash
python3 scripts/stitch_check_images.py --force
```

### Import

Moves check/deposit images from `data/renamed/` into `assets/checks/`, registers in DB, creates symlinks.

```bash
python3 scripts/import_check_images.py data/renamed/
python3 scripts/import_check_images.py --scan
```

Then stitch:
```bash
python3 scripts/stitch_check_images.py
```

Or do it all in one go — run import then stitch:

```bash
python3 scripts/import_check_images.py --scan && python3 scripts/stitch_check_images.py
```

### Status

```bash
python3 scripts/stitch_check_images.py --report
python3 scripts/import_check_images.py --status
```

Show both reports to the user.

### Clean

When thumbnails or cache entries are stale and need rebuilding:

1. Delete old check/deposit thumbnails:
```bash
python3 -c "
from pathlib import Path
t = Path('assets/thumbnails')
deleted = 0
for f in list(t.glob('check_*.webp')) + list(t.glob('deposit_*.webp')):
    f.unlink()
    deleted += 1
print(f'Deleted {deleted} old thumbnails')
"
```

2. Delete old back/checkN cache entries:
```bash
python3 -c "
import sqlite3
conn = sqlite3.connect('db/personaldb.sqlite')
d1 = conn.execute(\"DELETE FROM image_cache WHERE asin LIKE '%:check:%:back'\").rowcount
d2 = conn.execute(\"DELETE FROM image_cache WHERE asin LIKE '%:deposit:%:check%'\").rowcount
conn.commit()
print(f'Deleted {d1 + d2} old cache entries')
"
```

3. Delete old composites:
```bash
python3 -c "
from pathlib import Path
c = Path('assets/checks')
deleted = sum(1 for f in c.glob('*_composite.png') if f.unlink() or True)
print(f'Deleted {deleted} old composites')
"
```

4. Re-stitch:
```bash
python3 scripts/stitch_check_images.py
```

## Full Pipeline (end-to-end)

When the user has just downloaded check/deposit images and wants everything processed:

1. Ensure Pillow is installed (see Prerequisites)
2. If files are in `data/incoming/`, tell user to run `/-renamer` first
3. If files are in `data/renamed/`, run import:
   ```bash
   python3 scripts/import_check_images.py data/renamed/ && python3 scripts/import_check_images.py --scan
   ```
4. Stitch composites + thumbnails:
   ```bash
   python3 scripts/stitch_check_images.py
   ```
5. Show status to confirm

## Notes

- Composites use the **widest** source image as the target width — narrower images are scaled up to match
- Thumbnails are 240x240 WebP, same as product image thumbnails
- The review UI lightbox shows the full composite — no front/back toggle needed
- Deposit composites can be tall (slip + multiple checks) — the lightbox handles scrolling via `max-height: 80vh` with `object-fit: contain`
- `stitch_check_images.py --force` re-stitches even if composites exist (use after re-downloading images)
