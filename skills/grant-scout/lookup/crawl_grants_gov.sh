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

echo "[grants.gov] Search2 — listing posted+forecasted opportunities (paginated)"
# Search2 caps a single call's usable window at whatever `rows` you pass (it does
# NOT auto-page) and silently returns just that page with the true `hitCount`
# alongside it — a fixed rows:1200 with no pagination quietly drops everything
# past the 1200th hit. Verified live 2026-09-11: hitCount was 1571 while a single
# rows:1200 call returned only 1200, permanently starving 371 opportunities
# (~24%) of ever being detail-fetched, including one whose forecast had already
# flipped to a posted synopsis with a changed deadline. Page with startRecordNum
# (documented, confirmed working) until every hit is collected.
PAGE=1200
list_file=$(mktemp)
> "$DEST/_hits.jsonl"
start=0
total=-1
while :; do
    curl -s -X POST "$SEARCH_URL" \
        -H 'Content-Type: application/json' \
        -d "{\"rows\":${PAGE},\"startRecordNum\":${start},\"keyword\":\"\",\"oppNum\":\"\",\"cfda\":\"\",\"agencies\":\"\",\"sortBy\":\"openDate|desc\",\"oppStatuses\":\"posted|forecasted\"}" \
        -o "$list_file"
    page_n=$(python3 - "$list_file" "$DEST/_hits.jsonl" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
data = d.get('data', {}) or {}
hits = data.get('oppHits', []) or []
with open(sys.argv[2], 'a') as out:
    for h in hits:
        out.write(json.dumps({
            "id": h.get('id'),
            "number": h.get('number'),
            "title": h.get('title'),
            "agency": h.get('agencyCode') or h.get('agency'),
            "openDate": h.get('openDate'),
            "closeDate": h.get('closeDate'),
            "oppStatus": h.get('oppStatus'),
        }) + "\n")
print(f"{len(hits)}\t{data.get('hitCount', 0)}")
PY
)
    rc=$?
    [ $rc -ne 0 ] && { echo "[grants.gov] parse failed at startRecordNum=${start}"; exit 1; }
    n=$(echo "$page_n" | cut -f1)
    hitcount=$(echo "$page_n" | cut -f2)
    total="$hitcount"
    echo "[grants.gov] page start=${start} got=${n} of hitCount=${hitcount}"
    [ "${n:-0}" -eq 0 ] && break
    start=$((start + n))
    [ "$start" -ge "$total" ] && break
done
rm -f "$list_file"

python3 - "$DEST/_hits.jsonl" "$DEST/_list.json" <<'PY'
import json, sys
out = []
seen = set()
with open(sys.argv[1]) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        h = json.loads(line)
        key = h.get('id')
        if key in seen:
            continue
        seen.add(key)
        out.append(h)
json.dump(out, open(sys.argv[2], 'w'), indent=2)
print(f"[grants.gov] listed {len(out)} opportunities (deduped across pages)")
PY

# Merge in already-known opportunities that Search2's posted|forecasted view no
# longer surfaces. Verified live 2026-09-11: an opportunity can still answer
# fetchOpportunity with a materially different synopsis (amended responseDate,
# forecast->posted transition) after it drops out of this listing (closed,
# withdrawn, reclassified, or just a transient indexing gap on grants.gov's
# side) — without this, such a record freezes forever at whatever close_date it
# had on the day it fell out, which is exactly the silent-staleness failure mode
# this whole pipeline exists to avoid. One extra fetchOpportunity call per
# already-known id is cheap next to the win of catching that transition or
# confirming the record is genuinely closed now.
python3 - "$RAW" "$DEST/_list.json" <<'PY'
import glob, json, os, sys

raw, list_path = sys.argv[1], sys.argv[2]
current = json.load(open(list_path))
have_numbers = {(h.get('number') or '') for h in current}
added = 0
for fp in glob.glob(os.path.join(raw, '*.json')):
    num = os.path.splitext(os.path.basename(fp))[0]
    if num in have_numbers:
        continue
    try:
        d = json.load(open(fp))
    except Exception:
        continue
    oid = (d.get('data') or {}).get('id')
    if not oid:
        continue
    current.append({"id": oid, "number": num, "title": None, "agency": None,
                     "openDate": None, "closeDate": None, "oppStatus": "recheck-not-in-current-listing"})
    have_numbers.add(num)
    added += 1
json.dump(current, open(list_path, 'w'), indent=2)
print(f"[grants.gov] +{added} already-known opportunities re-queued for a fresh status check "
      f"(no longer in Search2's posted|forecasted listing) -> {len(current)} total to fetch")
PY

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
# ALWAYS re-fetch, even if $out already exists. A forecast opportunity becomes a
# posted one with its own responseDate (grants.gov keeps the SAME opportunityId/
# number across that transition), and posted synopses get amended (deadline
# extensions, version bumps) without a new number either. A skip-if-exists cache
# here silently freezes close_date at whatever it was on first sight — verified
# live 2026-09-11: 100% of the 1924 stored records predated that day's "crawl"
# because every worker call hit this skip, and at least one (HHS-2026-ACF-ACYF-
# CO-0055) had since flipped forecast(estApplicationResponseDate=Aug 17)->posted
# (responseDate=Aug 26) with the cached copy never catching the change. The full
# detail-fetch for ~1900-2400 opportunities completes in well under 2 minutes, so
# there is no real cost to always refreshing.
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

# Dispatch with a bash background-job pattern (xargs has argv length issues for 1000+ ids).
# Feed the loop via process substitution, NOT a pipe: `cmd | while read; do ... & done`
# runs the while loop in a SUBSHELL (the pipe's right-hand side), so every background
# job it forks is a child of that subshell, not of this script. The periodic `wait`
# every 30 iterations (inside the subshell) reaps full batches fine, but the final
# PARTIAL batch (here 1913 % 30 = 23 jobs) has no trailing `wait` before the subshell
# exits when its stdin is exhausted — it orphans those last jobs. The `wait` on the
# line below then runs in THIS shell, which never forked them, so it returns
# immediately, and `rm -f "$WORKER"` deletes the shared worker script while up to ~29
# tail-batch curls are still trying to exec it — observed live 2026-09-11 as three
# "No such file or directory" errors on $WORKER, i.e. up to 29 opportunities near the
# end of every run silently never re-fetched (stale/missing raw file, exactly the
# per-record staleness this crawler was just rewritten to eliminate). Process
# substitution keeps the while loop (and its background jobs) in this shell, so the
# final `wait` actually blocks for every job, tail batch included.
fetched=0
batch=0
while read -r ID NUM; do
    "$WORKER" "$ID" "$NUM" "$RAW" &
    batch=$((batch + 1))
    fetched=$((fetched + 1))
    if [ $((batch % 30)) -eq 0 ]; then
        wait
        echo "[grants.gov] fetched ${fetched}/${TOTAL}"
    fi
done < <(python3 -c "import json;[print(h['id'],(h['number'] or h['id']).replace('/','_')) for h in json.load(open('$LIST'))]")
wait
rm -f "$WORKER"

# Count actual files written
COUNT=$(ls -1 "$RAW" 2>/dev/null | wc -l | tr -d ' ')
echo "$COUNT" > "$DEST/_count"
echo "[grants.gov] done. records: ${COUNT}"
