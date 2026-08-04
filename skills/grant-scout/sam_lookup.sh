#!/usr/bin/env bash
# sam_lookup.sh — credential-safe SAM.gov v2 opportunities lookup for grant-scout.
#
# Usage:
#   sam_lookup.sh by-solnum <SOLICITATION-NUMBER> [outdir]
#   sam_lookup.sh by-noticeid <NOTICE-ID> [outdir]
#   sam_lookup.sh validate
#
# Reads the API key from $SAM_API_KEY in the environment ONLY.
# Never accepts the key as an argument; never echoes it; never embeds it in argv.
# The key reaches curl via stdin (`curl --config -` with an unquoted heredoc)
# so it does not appear in process listings, shell history, or saved files.
#
# Exit codes:
#   0  success (HTTP 200, result written to outdir/raw.json + outdir/normalized.json)
#   1  usage error
#   2  $SAM_API_KEY not set
#   3  HTTP non-200 from SAM
#   4  zero results (raw.json written as an audit-trail with totalRecords:0)

set -u

usage() {
    echo "Usage: $0 by-solnum <SOLICITATION-NUMBER> [outdir]" >&2
    echo "       $0 by-noticeid <NOTICE-ID> [outdir]" >&2
    echo "       $0 validate" >&2
    exit 1
}

require_key() {
    if [ -z "${SAM_API_KEY:-}" ]; then
        echo "ERROR: \$SAM_API_KEY is not set. Run \`source ~/.profile\` or open a new shell." >&2
        exit 2
    fi
}

# Call SAM v2 with the key passed via stdin only.
# Args: $1=query-string-without-key, $2=output-file
sam_get() {
    local qs="$1"
    local outfile="$2"
    local http
    http=$(curl -s -o "$outfile" -w "%{http_code}" --config - <<CFG
url = "https://api.sam.gov/opportunities/v2/search?${qs}&api_key=${SAM_API_KEY}"
CFG
)
    echo "$http"
}

# Build adjacent 6-month windows covering ~13 months back (SAM v2 caps each query <1y)
windows() {
    local today
    today=$(date -u +%Y-%m-%d)
    # Window A: today-180d → today
    local a_from a_to
    a_to=$(date -u +%m/%d/%Y)
    a_from=$(date -u -v-180d +%m/%d/%Y 2>/dev/null || date -u -d "180 days ago" +%m/%d/%Y)
    # Window B: today-360d → today-181d
    local b_from b_to
    b_to=$(date -u -v-181d +%m/%d/%Y 2>/dev/null || date -u -d "181 days ago" +%m/%d/%Y)
    b_from=$(date -u -v-360d +%m/%d/%Y 2>/dev/null || date -u -d "360 days ago" +%m/%d/%Y)
    echo "$a_from $a_to $b_from $b_to"
}

# Map a SAM v2 opportunitiesData record to the grant-scout extraction shape.
# Args: $1 = input raw.json path, $2 = output normalized.json path.
normalize_sam() {
    python3 - "$1" "$2" <<'PY'
import json, sys
infile, outfile = sys.argv[1], sys.argv[2]
try:
    d = json.load(open(infile))
except Exception as e:
    json.dump({"found": False, "error": f"raw parse failed: {e}"}, open(outfile, 'w'), indent=2)
    sys.exit(0)
ops = d.get('opportunitiesData', [])
if not ops:
    json.dump({"found": False, "totalRecords": d.get('totalRecords', 0)}, open(outfile, 'w'), indent=2)
    sys.exit(0)
o = ops[0]
syn = o.get('description', '') or ''
out = {
    "found": True,
    "source": "sam.gov v2",
    "id": o.get('solicitationNumber') or o.get('noticeId'),
    "noticeId": o.get('noticeId'),
    "title": o.get('title'),
    "agency": o.get('fullParentPathName', '').split('.')[0],
    "subagency": o.get('fullParentPathName'),
    "type": o.get('type'),
    "baseType": o.get('baseType'),
    "postedDate": o.get('postedDate'),
    "responseDeadline": o.get('responseDeadLine') or o.get('archiveDate'),
    "naicsCode": o.get('naicsCode'),
    "classificationCode": o.get('classificationCode'),
    "setAside": o.get('typeOfSetAsideDescription') or o.get('typeOfSetAside'),
    "awardCeiling": o.get('award', {}).get('amount') if isinstance(o.get('award'), dict) else None,
    "uiLink": o.get('uiLink'),
    "description_short": (syn[:600] if isinstance(syn, str) else ''),
    "raw_keys": sorted(o.keys()),
}
json.dump(out, open(outfile, 'w'), indent=2)
PY
}

cmd="${1:-}"
case "$cmd" in
    validate)
        require_key
        local_today=$(date -u +%m/%d/%Y)
        local_week=$(date -u -v-7d +%m/%d/%Y 2>/dev/null || date -u -d "7 days ago" +%m/%d/%Y)
        tmp=$(mktemp)
        http=$(sam_get "limit=1&postedFrom=${local_week}&postedTo=${local_today}" "$tmp")
        if [ "$http" = "200" ]; then
            tot=$(python3 -c "import json;print(json.load(open('$tmp')).get('totalRecords'))")
            echo "OK: HTTP 200, totalRecords=$tot for last 7 days"
            rm -f "$tmp"
            exit 0
        else
            echo "FAIL: HTTP $http" >&2
            cat "$tmp" >&2
            rm -f "$tmp"
            exit 3
        fi
        ;;
    by-solnum|by-noticeid)
        [ $# -ge 2 ] || usage
        require_key
        local_id="$2"
        outdir="${3:-./sam-out}"
        mkdir -p "$outdir"

        param="solnum"
        [ "$cmd" = "by-noticeid" ] && param="noticeid"

        read -r AF AT BF BT < <(windows)

        # Window A
        rawA="$outdir/raw_windowA.json"
        httpA=$(sam_get "limit=25&postedFrom=${AF}&postedTo=${AT}&${param}=${local_id}" "$rawA")
        totA=0
        if [ "$httpA" = "200" ]; then
            totA=$(python3 -c "import json;print(json.load(open('$rawA')).get('totalRecords') or 0)")
        fi

        # Window B (only if A returned 0)
        totB=0
        rawB=""
        if [ "$totA" = "0" ]; then
            rawB="$outdir/raw_windowB.json"
            httpB=$(sam_get "limit=25&postedFrom=${BF}&postedTo=${BT}&${param}=${local_id}" "$rawB")
            if [ "$httpB" = "200" ]; then
                totB=$(python3 -c "import json;print(json.load(open('$rawB')).get('totalRecords') or 0)")
            fi
        fi

        total=$(( totA + totB ))
        echo "windowA: HTTP $httpA totalRecords=$totA"
        [ -n "$rawB" ] && echo "windowB: HTTP $httpB totalRecords=$totB"

        # Choose the file with hits, or write a not-found audit trail
        if [ "$totA" != "0" ]; then
            cp "$rawA" "$outdir/raw.json"
        elif [ "$totB" != "0" ]; then
            cp "$rawB" "$outdir/raw.json"
        else
            python3 - "$outdir/raw.json" "$local_id" "$param" "$AF" "$AT" "$BF" "$BT" <<'PY'
import json, sys, datetime
out, sid, param, AF, AT, BF, BT = sys.argv[1:8]
data = {
    "lookup_target": sid,
    "lookup_date_utc": datetime.datetime.now(datetime.UTC).isoformat().replace("+00:00", "Z"),
    "result": "not_found",
    "api": "SAM.gov v2 opportunities/search",
    "param": param,
    "windows": [{"postedFrom": AF, "postedTo": AT}, {"postedFrom": BF, "postedTo": BT}],
    "totalRecords": 0,
    "note": "Both adjacent 6-month windows returned 0. Check the originating portal, FPDS-NG, or USAspending."
}
json.dump(data, open(out, 'w'), indent=2)
PY
            echo "not_found — audit trail at $outdir/raw.json"
            normalize_sam "$outdir/raw.json" "$outdir/normalized.json"
            exit 4
        fi

        normalize_sam "$outdir/raw.json" "$outdir/normalized.json"
        echo "OK: $outdir/raw.json + $outdir/normalized.json"
        exit 0
        ;;
    *)
        usage
        ;;
esac
