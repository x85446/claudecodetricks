#!/usr/bin/env bash
# crawl_sam.sh — bulk crawl SAM.gov v2 currently-open opportunities.
#
# Usage:  crawl_sam.sh <source-corpus-dir>
# Writes: <dir>/raw/<noticeId>.json   (one file per opportunity)
#         <dir>/_count                (record count)
#
# Credentials: $SAM_API_KEY env var ONLY. Never argv, never URL on cli.

set -u

DEST="${1:?usage: crawl_sam.sh <source-corpus-dir>}"
RAW="$DEST/raw"
mkdir -p "$RAW"

if [ -z "${SAM_API_KEY:-}" ]; then
    echo "[sam] ERROR: \$SAM_API_KEY is not set; skipping SAM crawl"
    echo 0 > "$DEST/_count"
    exit 2
fi

# Currently-open = posted in last 12 months (SAM v2 caps a single query at <1y, so use 11 months)
TODAY=$(date -u +%m/%d/%Y)
FROM=$(date -u -v-330d +%m/%d/%Y 2>/dev/null || date -u -d "330 days ago" +%m/%d/%Y)

echo "[sam] window: ${FROM} → ${TODAY}"

PAGE_SIZE=1000
OFFSET=0
TOTAL=-1
FETCHED=0

while :; do
    page_file=$(mktemp)
    http=$(curl -s -o "$page_file" -w "%{http_code}" --config - <<CFG
url = "https://api.sam.gov/opportunities/v2/search?limit=${PAGE_SIZE}&offset=${OFFSET}&postedFrom=${FROM}&postedTo=${TODAY}&api_key=${SAM_API_KEY}"
CFG
)
    if [ "$http" != "200" ]; then
        echo "[sam] HTTP $http at offset ${OFFSET}; aborting"
        head -c 300 "$page_file" >&2
        rm -f "$page_file"
        break
    fi

    if [ "$TOTAL" = "-1" ]; then
        TOTAL=$(python3 -c "import json; print(json.load(open('$page_file')).get('totalRecords') or 0)")
        echo "[sam] totalRecords reported: ${TOTAL}"
    fi

    # Split records into per-noticeId files, filter to currently open (responseDeadLine in future)
    written=$(python3 - "$page_file" "$RAW" <<'PY'
import json, sys, os, datetime
page, raw = sys.argv[1], sys.argv[2]
d = json.load(open(page))
today = datetime.date.today()
n = 0
for o in d.get('opportunitiesData', []) or []:
    # Active filter
    if o.get('active') not in (True, 'Yes', 'yes', 1): pass  # keep — SAM 'active' is unreliable
    rd = o.get('responseDeadLine')
    if rd:
        try:
            close = datetime.date.fromisoformat(rd[:10])
            if close < today: continue
        except Exception: pass
    nid = o.get('noticeId')
    if not nid: continue
    json.dump(o, open(os.path.join(raw, f"{nid}.json"), 'w'), indent=2)
    n += 1
print(n)
PY
)
    rm -f "$page_file"
    FETCHED=$((FETCHED + written))
    echo "[sam] offset=${OFFSET} written=${written} cumulative=${FETCHED}"

    OFFSET=$((OFFSET + PAGE_SIZE))
    [ "$OFFSET" -ge "$TOTAL" ] && break
    sleep 1
done

echo "$FETCHED" > "$DEST/_count"
echo "[sam] done. currently-open records: ${FETCHED}"
