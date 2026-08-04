#!/usr/bin/env bash
# crawl_grants_gov.sh — bulk crawl Grants.gov posted+forecasted opportunities.
#
# Usage:  crawl_grants_gov.sh <source-corpus-dir>
# Writes: <dir>/raw/<oppNumber>.json   (one file per opportunity; opportunity detail JSON via fetchOpportunity)
#         <dir>/_count                 (record count)
# Uses the public Search2 + fetchOpportunity APIs (no auth).

set -u

DEST="${1:?usage: crawl_grants_gov.sh <source-corpus-dir>}"
RAW="$DEST/raw"
mkdir -p "$RAW"

SEARCH_URL="https://api.grants.gov/v1/api/search2"
FETCH_URL="https://api.grants.gov/v1/api/fetchOpportunity"

echo "[grants.gov] Search2 — listing posted+forecasted opportunities"
list_file=$(mktemp)
curl -s -X POST "$SEARCH_URL" \
    -H 'Content-Type: application/json' \
    -d '{"rows":1200,"keyword":"","oppNum":"","cfda":"","agencies":"","sortBy":"openDate|desc","oppStatuses":"posted|forecasted"}' \
    -o "$list_file"

# Extract list of (id, oppNumber, title) from the response
python3 - "$list_file" "$DEST/_list.json" <<'PY' || { echo "[grants.gov] parse failed"; exit 1; }
import json, sys
d = json.load(open(sys.argv[1]))
hits = d.get('data', {}).get('oppHits', []) or []
out = []
for h in hits:
    out.append({
        "id": h.get('id'),
        "number": h.get('number'),
        "title": h.get('title'),
        "agency": h.get('agencyCode') or h.get('agency'),
        "openDate": h.get('openDate'),
        "closeDate": h.get('closeDate'),
        "oppStatus": h.get('oppStatus'),
    })
json.dump(out, open(sys.argv[2], 'w'), indent=2)
print(f"[grants.gov] listed {len(out)} opportunities")
PY
rm -f "$list_file"

# Fetch detail for each id with parallel jobs, 30 concurrent at a time, then wait.
LIST="$DEST/_list.json"
TOTAL=$(python3 -c "import json;print(len(json.load(open('$LIST'))))")
echo "[grants.gov] fetching detail for ${TOTAL} opportunities (this takes a while)"

# Write a worker script
WORKER=$(mktemp)
cat > "$WORKER" <<'WSH'
#!/usr/bin/env bash
id="$1"
num="$2"
raw="$3"
out="$raw/${num}.json"
# Skip if file already exists and is non-empty
[ -s "$out" ] && exit 0
curl -s -X POST "https://api.grants.gov/v1/api/fetchOpportunity" \
    -H 'Content-Type: application/json' \
    -d "{\"opportunityId\":${id}}" \
    -o "$out.tmp"
if [ -s "$out.tmp" ] && grep -q '"data"' "$out.tmp"; then
    mv "$out.tmp" "$out"
else
    rm -f "$out.tmp"
fi
WSH
chmod +x "$WORKER"

# Dispatch with a bash background-job pattern (xargs has argv length issues for 1000+ ids)
fetched=0
batch=0
python3 -c "import json;[print(h['id'],(h['number'] or h['id']).replace('/','_')) for h in json.load(open('$LIST'))]" | \
while read -r ID NUM; do
    "$WORKER" "$ID" "$NUM" "$RAW" &
    batch=$((batch + 1))
    fetched=$((fetched + 1))
    if [ $((batch % 30)) -eq 0 ]; then
        wait
        echo "[grants.gov] fetched ${fetched}/${TOTAL}"
    fi
done
wait
rm -f "$WORKER"

# Count actual files written
COUNT=$(ls -1 "$RAW" 2>/dev/null | wc -l | tr -d ' ')
echo "$COUNT" > "$DEST/_count"
echo "[grants.gov] done. records: ${COUNT}"
