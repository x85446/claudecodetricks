#!/usr/bin/env bash
# crawl_diu.sh — crawl Defense Innovation Unit (DIU) open solicitations.
#
# Usage:  crawl_diu.sh <source-corpus-dir> [--refresh]
# Writes: <dir>/raw/<number>.json   (one per open AOI/solicitation: full record)
#         <dir>/_count
#
# DIU's open-solicitations page (https://www.diu.mil/work-with-us/open-solicitations)
# is a Nuxt 2 SPA with no documented JSON API. But Nuxt SSR pages embed their full
# page state directly in the served HTML as:
#
#   <script>window.__NUXT__=(function(a,b,c,...){ ... return {...} }(args...))</script>
#
# — a `@nuxt/devalue`-style payload (deduped via bare-identifier back-references,
# plus a couple of pre-`return` mutation statements for the one shared/circular
# container). It is NOT valid JSON, but it IS a small deterministic grammar, so a
# plain `curl` of the page plus nuxt_devalue.py (pure stdlib, no browser, no JS
# runtime) extracts the exact same data a browser would render. Verified
# 2026-09-10: the parsed `state.aois` list matches the live page's displayed
# solicitation count and titles exactly.
#
# The open solicitations live at parsed_payload.state.aois — each entry already
# carries the full narrative (u_area_of_interest), full eligibility text, and
# (when present) FAQs: no separate per-item detail fetch is needed, unlike DSIP.
#
# Idempotency: this is a single cheap page fetch (not a paginated crawl), so we
# always refresh the full raw/ set from the current payload every run — there is
# no meaningful caching to do, and doing so guarantees a closed AOI disappears
# from raw/ on the very next run rather than lingering as stale data. --refresh
# is accepted for interface consistency with the other crawlers but is a no-op.

set -u

DEST="${1:?usage: crawl_diu.sh <source-corpus-dir> [--refresh]}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAW="$DEST/raw"
mkdir -p "$RAW"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
URL="https://www.diu.mil/work-with-us/open-solicitations"
PAGE="$DEST/_page.html"

echo "[diu] fetching ${URL}"
attempt=1
http=""
while [ "$attempt" -le 4 ]; do
    http=$(curl -sSL "$URL" -A "$UA" -H "Accept: text/html" --max-time 45 -o "$PAGE" -w "%{http_code}")
    case "$http" in
        200) break ;;
        429|5??)
            b=$((5*attempt))
            echo "[diu] HTTP $http on $URL; backoff ${b}s (attempt $attempt)"
            sleep "$b"
            attempt=$((attempt+1))
            ;;
        *) break ;;
    esac
done

if [ "$http" != "200" ] || [ ! -s "$PAGE" ]; then
    echo "[diu] listing HTTP ${http:-?}; aborting"
    echo 0 > "$DEST/_count"
    rm -f "$PAGE"
    exit 1
fi

python3 - "$PAGE" "$RAW" "$DEST" <<PY
import json
import os
import sys

sys.path.insert(0, "${SCRIPT_DIR}")
from nuxt_devalue import extract_nuxt_expr, parse_nuxt_devalue, DevalueParseError

page, raw_dir, dest = sys.argv[1], sys.argv[2], sys.argv[3]
html = open(page, encoding="utf-8", errors="ignore").read()

expr = extract_nuxt_expr(html)
if expr is None:
    print("[diu] no window.__NUXT__ payload found on the page; aborting")
    with open(os.path.join(dest, "_count"), "w") as f:
        f.write("0\n")
    sys.exit(1)

try:
    data = parse_nuxt_devalue(expr)
except DevalueParseError as e:
    print(f"[diu] failed to parse __NUXT__ payload: {e}")
    with open(os.path.join(dest, "_count"), "w") as f:
        f.write("0\n")
    sys.exit(1)

aois = (((data or {}).get("state") or {}).get("aois")) or []
if not isinstance(aois, list):
    print(f"[diu] state.aois was not a list ({type(aois).__name__}); aborting")
    with open(os.path.join(dest, "_count"), "w") as f:
        f.write("0\n")
    sys.exit(1)

print(f"[diu] parsed {len(aois)} open solicitation(s) from __NUXT__ state.aois")

# Always refresh: clear stale raw/*.json first so a closed AOI (removed from the
# live payload) doesn't linger and get reported as still-open.
for n in os.listdir(raw_dir):
    if n.endswith(".json"):
        os.remove(os.path.join(raw_dir, n))

written = 0
expected = set()
for a in aois:
    if not isinstance(a, dict):
        continue
    number = a.get("number") or a.get("sys_id")
    if not number:
        print(f"[diu] AOI with no number/sys_id ({a.get('u_aoi_title')!r}); skipping")
        continue
    safe = "".join(c if (c.isalnum() or c in "._-") else "_" for c in str(number))
    name = f"{safe}.json"
    expected.add(name)
    with open(os.path.join(raw_dir, name), "w", encoding="utf-8") as f:
        json.dump(a, f, ensure_ascii=False)
    written += 1
    print(f"[diu] {number}: {a.get('u_aoi_title')!r} closes {a.get('u_aoi_close_date')}")

count = len([n for n in os.listdir(raw_dir) if n.endswith(".json")])
with open(os.path.join(dest, "_count"), "w") as f:
    f.write(f"{count}\n")

print(f"[diu] done. reported={len(aois)} written={written} stored={count}")
PY
rc=$?

rm -f "$PAGE"
exit $rc
