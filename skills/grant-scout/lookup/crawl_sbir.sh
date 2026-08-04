#!/usr/bin/env bash
# crawl_sbir.sh — bulk crawl SBIR.gov open solicitations (covers DoD + civilian SBIR/STTR).
#
# Usage:  crawl_sbir.sh <source-corpus-dir>
# Writes: <dir>/raw/<solicitation_number>.json
#         <dir>/_count
#
# Endpoint: GET https://api.www.sbir.gov/public/api/solicitations
# Auth: none. Rate-limit: undocumented; observed 429 under aggressive polling.
#       This script paginates start+=50 with a 6-second sleep between requests.
#
# Note: SBIR.gov is the canonical SBIR/STTR data source — it carries every DoD
# SBIR topic that would otherwise require DSIP CAC login, plus all civilian
# agency SBIR/STTR (HHS, NASA, NSF, DOE, USDA, EPA, DOC, ED, DOT, DHS).

set -u

DEST="${1:?usage: crawl_sbir.sh <source-corpus-dir>}"
RAW="$DEST/raw"
mkdir -p "$RAW"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) grant-scout/1.0"
BASE="https://api.www.sbir.gov/public/api/solicitations"
ROWS=50
START=0
TOTAL_WRITTEN=0
MAX_PAGES=40  # safety: 40 * 50 = 2000 records max per crawl

while [ "$START" -lt $((MAX_PAGES * ROWS)) ]; do
    page="$DEST/_page_$(printf '%05d' $START).json"
    attempt=1
    while [ "$attempt" -le 3 ]; do
        http=$(curl -sL -A "$UA" \
            -H "Accept: application/json" \
            -H "Referer: https://www.sbir.gov/" \
            -o "$page" \
            -w "%{http_code}" \
            "${BASE}?open=1&rows=${ROWS}&start=${START}")
        if [ "$http" = "200" ]; then
            break
        elif [ "$http" = "429" ]; then
            backoff=$((10 * attempt))
            echo "[sbir] 429 at start=${START} attempt=${attempt}; sleeping ${backoff}s"
            sleep "$backoff"
            attempt=$((attempt + 1))
        else
            echo "[sbir] HTTP $http at start=${START}; aborting"
            head -c 300 "$page" >&2
            break 2
        fi
    done

    if [ "$http" != "200" ]; then
        echo "[sbir] giving up at start=${START} after 429-retries"
        break
    fi

    # Split the response (a JSON array) into per-solicitation files
    written=$(python3 - "$page" "$RAW" <<'PY'
import json, sys, os, re
page, raw = sys.argv[1], sys.argv[2]
try:
    d = json.load(open(page))
except Exception as e:
    print(0); sys.exit(0)
if not isinstance(d, list):
    print(0); sys.exit(0)
n = 0
for s in d:
    sid = s.get('solicitation_number') or s.get('solicitation_title') or ''
    safe = re.sub(r'[^A-Za-z0-9._-]+', '_', str(sid)).strip('_')[:80]
    if not safe: continue
    json.dump(s, open(os.path.join(raw, f"{safe}.json"), 'w'), indent=2)
    n += 1
print(n)
PY
)
    if [ "$written" = "0" ]; then
        echo "[sbir] empty page at start=${START}; done"
        rm -f "$page"
        break
    fi
    TOTAL_WRITTEN=$((TOTAL_WRITTEN + written))
    echo "[sbir] start=${START} written=${written} cumulative=${TOTAL_WRITTEN}"
    rm -f "$page"

    START=$((START + ROWS))
    sleep 6  # polite — observed 429 at lower intervals
done

echo "$TOTAL_WRITTEN" > "$DEST/_count"
echo "[sbir] done. records: ${TOTAL_WRITTEN}"
