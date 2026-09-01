---
name: image-fetcher
description: Fetch, cache, and thumbnail product images for Amazon, Lowe's, and other retailers. Handles the full pipeline from finding product URLs to generating local WebP thumbnails.
argument-hint: [site] [--stage N] [--report]
---

# Image Fetcher Skill

Fetch product images for transaction line items and generate local thumbnails for the review UI.

## Invocation

```
/-image-fetcher                     # Show pipeline status for all sites
/-image-fetcher amazon              # Run Amazon image pipeline
/-image-fetcher lowes               # Run Lowe's image pipeline (all stages)
/-image-fetcher lowes --stage 1     # Stage 1 only: Google search for product URLs
/-image-fetcher lowes --stage 2     # Stage 2 only: scrape pages for image URLs
/-image-fetcher lowes --stage 3     # Stage 3 only: generate thumbnails
/-image-fetcher thumbnails          # Generate thumbnails for all sites
/-image-fetcher report              # Full status report
```

## Architecture

### Image Cache

All image URLs are stored in `image_cache` table in `db/personaldb.sqlite`:

| Column | Type | Description |
|--------|------|-------------|
| `asin` | TEXT PRIMARY KEY | Cache key: ASIN for Amazon, `lowes:{item_number}` for Lowe's |
| `image_url` | TEXT | CDN URL, empty string (not yet found), or `DEAD_CONFIRMED` |
| `fetched_at` | TEXT | Timestamp |

### Thumbnail System

- **Directory**: `assets/thumbnails/`
- **Format**: 240x240 WebP (2x retina for 120px display)
- **Naming**: `{cache_key}.webp` with `:` replaced by `_` (e.g., `lowes_12345.webp`)
- **Serving**: `/thumb/{key}.webp` endpoint, falls back to 302 redirect to source URL

### Frontend Integration

The review UI uses `image_key` field per candidate (not just ASIN):
- Amazon: `image_key = asin` (e.g., `B08BFMT9KL`)
- Lowe's: `image_key = lowes:{item_number}` (e.g., `lowes:894289`)
- Frontend calls `getImageUrl(c.image_key)` → `/thumb/{key}.webp`

## Pipeline by Site

### Amazon

**Makefile**: `make amazon-images INTERVAL=6 RETRY=all MAXFAILS=24`

**Script**: `scripts/fetch_images.py`

Stages (all in one script):
1. **CDN direct** — tries `m.media-amazon.com/images/P/{ASIN}` patterns (fast, no bot risk)
2. **Page scraping** — rotates through 10 downloaders (urllib, brew-curl, sys-curl, wget, requests, httpx, curl_cffi×4) with randomized browser profiles
3. **Fallback domains** — tries `.sg`, `.co.uk`, `.ca`, `.de`, `.co.jp`, `.in`, `.com.mx`, `.com.au`, `.co.za`

**Special values**:
- `DEAD_CONFIRMED` — product verified gone everywhere, never retried (except `--retry dead`)
- Empty string — not yet found, will be retried

**Manual caching**: Insert directly into `image_cache` table:
```sql
INSERT OR REPLACE INTO image_cache (asin, image_url) VALUES ('B0BM4ZVK91', 'https://...');
```

### Lowe's

**Makefile**: `make lowes-images STAGE=1 LIMIT=20 DELAY=5`

**Script**: `scripts/fetch_product_images.py lowes`

**Three stages** (can be run independently):

#### Stage 1: Google Search → Product URLs

- Uses `googlesearch-python` library
- Searches `site:lowes.com/pd "{model}" {description}`
- For generic models (PVC, WOOD, LUMBER): `site:lowes.com/pd item {item_number} {description}`
- Stores result in `src_lowes.product_url`
- **This works well** — ~97% hit rate

#### Stage 2: Page Scrape → Image URLs

- **CHALLENGE**: Lowe's has aggressive bot detection
- Server-side scraping (all methods) gets 403
- Browser `fetch()` from lowes.com gets 403 after ~50 requests
- **What works**: Real browser navigation to each page, then DOM extraction
- Regex: `https://mobileimages.lowes.com/productimages/[uuid]/[id].(jpg|png)`
- Stores in `image_cache` as `lowes:{item_number}`

**Browser automation approach** (claude-in-chrome):
1. Navigate to lowes.com product page
2. Extract: `document.querySelectorAll('img').filter(s => s.includes('productimages'))`
3. First `productimages` URL is the main product image
4. Rate limit: 10+ seconds between pages to avoid detection
5. IP rotation helps but session-based detection persists

**CORS endpoint** for browser-to-server saves:
- `POST /api/cache-images` with body `{"images": {"lowes:12345": "https://..."}}`
- Server needs CORS headers (Access-Control-Allow-Origin: *)
- Mixed content blocks HTTPS→HTTP, so crawler must run from HTTP page or same-origin

#### Stage 3: Thumbnails

- Same as Amazon: download image, resize to 240×240, save as WebP
- `make thumbnails` handles all sites at once

### Home Depot

**Source data**: `src_homedepot.sku` — SKU numbers from PDF order details
**Cache key**: `hd:{sku}`

Home Depot uses UUIDs in their CDN URLs (`images.thdstatic.com/productImages/{uuid}/`), so SKUs can't be used directly. Same approach as Lowe's:

#### Stage 1: Google Search → Product URLs
- Search: `site:homedepot.com/p "{sku}" {description}`
- Store in `src_homedepot.product_url`
- HD product pages: `homedepot.com/p/{product-slug}/{sku}`

#### Stage 2: Scrape for Image URLs
- HD likely has similar bot detection to Lowe's
- Try `fetch()` from browser first; fall back to page navigation
- Image pattern: `https://images.thdstatic.com/productImages/{uuid}/{filename}`
- Store in `image_cache` as `hd:{sku}`

#### Stage 3: Thumbnails
- Same as other sites

### eBay

**Source data**: `src_ebay.image_url` — direct `i.ebayimg.com` URLs from XLSX exports
**Cache key**: `ebay:{item_id}`

eBay is the easiest — image URLs come directly from the purchase history export. Auto-cached during import. For older items without image URLs, scrape `ebay.com/itm/{item_id}` (no bot detection). Many old listings return 404 (items removed after ~6 months) — mark as `DEAD_CONFIRMED`.

### Adding New Retailers

Add entry to `SITES` dict in `scripts/fetch_product_images.py`:

```python
SITES = {
    "newsite": {
        "table": "src_newsite",           # source table name
        "id_col": "item_number",          # unique product ID column
        "model_col": "model_number",      # model number column
        "desc_col": "description",        # description column
        "url_col": "product_url",         # column to store product page URL
        "cache_prefix": "newsite",        # prefix for image_cache keys
        "search_domain": "newsite.com",   # domain for Google search
        "search_template": 'site:newsite.com "{model}" {desc_short}',
        "search_template_no_model": 'site:newsite.com {item_id} {desc_short}',
        "generic_models": {"GENERIC"},    # models to skip in search
        "image_patterns": [               # regex patterns for image extraction
            r'https://images\.newsite\.com/products/[^"]+\.(jpg|png)',
        ],
    },
}
```

## Key Files

| File | Purpose |
|------|---------|
| `scripts/fetch_images.py` | Amazon image fetcher (standalone, battle-tested) |
| `scripts/fetch_product_images.py` | Generic multi-site pipeline (Google search → scrape → thumbnail) |
| `scripts/thumbnails.py` | Batch thumbnail generator for all sites |
| `scripts/db_helpers.py` | Image cache DB layer, in-memory cache, `get_candidates()` with image_key |
| `scripts/review_handler.py` | `/thumb/`, `/image`, `/api/cache-images` endpoints |
| `scripts/server.py` | Route dispatch including CORS preflight |
| `assets/thumbnails/` | Generated WebP thumbnails |
| `temp/scrapes/` | Saved HTML pages for debugging |
| `temp/failed_asins.txt` | Amazon fail log |
| `temp/failed_product_images.txt` | Generic fail log |

## Current Status

Check with: `make amazon-images MODE=report` and `make lowes-images MODE=report`

## Known Issues

1. **Lowe's Stage 2 rate limiting** — `fetch()` from browser gets 403 after ~50 requests. Need real page navigation with 10s+ delays, or IP rotation between batches.
2. **Browser CORS** — HTTPS lowes.com cannot POST to HTTP localhost. Use the `/api/cache-images` endpoint from same-origin (localhost tab) or save to localStorage and pull manually.
3. **Thumbnail generation requires Pillow** — `pip install Pillow` on the host machine.
4. **`image_cache.asin` column name** — legacy name from Amazon-only days. Holds any cache key (ASIN, `lowes:12345`, etc.). Would benefit from rename to `cache_key` in a future migration.
