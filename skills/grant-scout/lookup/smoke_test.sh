#!/usr/bin/env bash
# TESTMASTER: id=grant-scout-smoke tier=standard parallel=no
# smoke_test.sh — fast contract check for the grant-scout acquisition pipeline.
#
# Usage:  smoke_test.sh          (or: make smoke)
#
# This is NOT a full crawl. It answers one question in ~20 seconds: "is every
# source still downloadable, keylessly, headlessly, and does the pipeline still
# wire together?" Run it before trusting a corpus, and after any crawler edit.
#
# WHY IT EXISTS
#   The addax run found eight real defects that all shared one shape: something
#   was silently wrong while the pipeline reported success. A dead SAM key, a
#   403 that sat for 28 days, a pagination cap dropping 24% of listings, a date
#   regex that nulled close_date so closed items looked perpetually open, an
#   ingest map covering 6 of 10 sources. None of them threw. Each check below
#   exists because one of them got through.
#
# Exit 0 only if every check passes. Any failure exits 1 and names the source.

set -u
cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." || exit 1
LOOKUP=".claude/skills/grant-scout/lookup"
REG="$LOOKUP/sources.json"
UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

PASS=0; FAIL=0
ok()   { printf '  \033[32mPASS\033[0m  %s\n' "$1"; PASS=$((PASS+1)); }
bad()  { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; FAIL=$((FAIL+1)); }

echo "grant-scout smoke test"
echo

# ---- 1. every registered source has an executable crawler ----------------
echo "[1] registry -> crawler wiring"
while IFS=$'\t' read -r short name crawler; do
    [ -z "${short:-}" ] && continue
    if [ -z "$crawler" ] || [ "$crawler" = "None" ] || [ "$crawler" = "null" ]; then
        bad "$name — no crawler registered"
    elif [ ! -x "$LOOKUP/$crawler" ]; then
        bad "$name — $crawler missing or not executable"
    else
        ok "$name -> $crawler"
    fi
done < <(python3 -c "
import json
for s in json.load(open('$REG'))['sources']:
    print('\t'.join([str(s.get('short') or ''), s['name'], str(s.get('crawler') or '')]))
")
echo

# ---- 2. no crawler the driver invokes depends on an API key --------------
# Comments naming the dead key are fine; a live reference is not.
echo "[2] keyless guarantee"
KEYHITS=0
while IFS=$'\t' read -r crawler; do
    [ -z "${crawler:-}" ] || [ "$crawler" = "None" ] && continue
    f="$LOOKUP/$crawler"
    [ -f "$f" ] || continue
    if grep -vE '^\s*#' "$f" 2>/dev/null | grep -q 'SAM_API_KEY'; then
        bad "$crawler references \$SAM_API_KEY in live code"
        KEYHITS=$((KEYHITS+1))
    fi
done < <(python3 -c "
import json
for s in json.load(open('$REG'))['sources']:
    print(s.get('crawler') or '')
")
[ "$KEYHITS" -eq 0 ] && ok "no invoked crawler requires an API key"
echo

# ---- 3. normalizer plugin discovery covers every source ------------------
echo "[3] normalizer discovery"
python3 - <<'PY'
import json, sys, os
sys.path.insert(0, os.path.join('.claude', 'skills', 'grant-scout', 'lookup'))
try:
    import normalizers
except Exception as e:
    print(f"  \033[31mFAIL\033[0m  normalizers package will not import: {e}")
    sys.exit(1)
found = set(normalizers.discover())
reg = json.load(open('.claude/skills/grant-scout/lookup/sources.json'))
want = {s.get('short') for s in reg['sources'] if s.get('short')}
missing = want - found
extra = found - want
if missing:
    print(f"  \033[31mFAIL\033[0m  registered sources with no normalizer: {sorted(missing)}")
    sys.exit(1)
print(f"  \033[32mPASS\033[0m  {len(found)} normalizers discovered, covering all {len(want)} registered sources")
if extra:
    print(f"         (note: normalizers with no registry entry: {sorted(extra)})")
PY
[ $? -eq 0 ] && PASS=$((PASS+1)) || FAIL=$((FAIL+1))
echo

# ---- 4. live endpoint reachability, one cheap request per source ---------
# This is the check that would have caught SBIR.gov's 403 on day one instead
# of day 28.
echo "[4] live endpoint reachability"
# The .mil nameservers (dns*.nipr.mil) intermittently SERVFAIL from some
# resolvers, so a plain curl against dodsbirsttr.mil fails with "Could not
# resolve host" while dig answers fine. crawl_dsip.sh already handles this by
# resolving once and pinning the IP via --resolve; the probe must do the same or
# it reports a false failure on a source that actually works.
_resolve() { # host -> first A record, trying local resolver then public ones
    local h="$1" ip
    for server in "" "@8.8.8.8" "@1.1.1.1"; do
        ip=$(dig +short +time=3 +tries=1 $server "$h" A 2>/dev/null \
             | grep -Eo '^[0-9]+(\.[0-9]+){3}$' | head -1)
        [ -n "$ip" ] && { echo "$ip"; return 0; }
    done
    return 1
}

probe() { # name, url, [method]
    local n="$1" u="$2" m="${3:-GET}"
    local host code pin=()
    host=$(printf '%s' "$u" | sed -E 's#^https?://([^/]+).*#\1#')
    code=$(curl -sS -o /dev/null -w '%{http_code}' -A "$UA" \
           -H 'Accept: application/hal+json, application/json, text/html' \
           --max-time 25 ${m:+-X "$m"} "$u" 2>/dev/null)
    # HTTP 000 with a resolvable host is a resolver problem, not an outage.
    if [ "$code" = "000" ]; then
        local ip
        if ip=$(_resolve "$host"); then
            pin=(--resolve "${host}:443:${ip}" --resolve "${host}:80:${ip}")
            code=$(curl -sS -o /dev/null -w '%{http_code}' -A "$UA" \
                   -H 'Accept: application/hal+json, application/json, text/html' \
                   --max-time 25 ${m:+-X "$m"} "${pin[@]}" "$u" 2>/dev/null)
            case "$code" in
                2*|3*) ok "$n — HTTP $code (resolver flaky; reached via pinned $ip)"; return ;;
            esac
        fi
    fi
    case "$code" in
        2*|3*) ok  "$n — HTTP $code" ;;
        000)   bad "$n — unreachable (HTTP 000, DNS pin also failed)" ;;
        *)     bad "$n — HTTP $code" ;;
    esac
}
probe "sam.gov (bulk extract)" "https://sam.gov/api/prod/fileextractservices/v1/api/download/Contract%20Opportunities/datagov/ContractOpportunitiesFullCSV.csv?privacy=Public"
probe "grants.gov"             "https://api.grants.gov/v1/api/search2" "POST"
probe "darpa.mil"              "https://www.darpa.mil/json/opportunity.json"
probe "sbir.gov"               "https://www.sbir.gov/topics"
probe "dsip"                   "https://www.dodsbirsttr.mil/topics-app/"
probe "xtech"                  "https://xtech.army.mil/competitions/"
probe "challenge.gov (usa.gov)" "https://www.usa.gov/find-active-challenge"
probe "nspires"                "https://nspires.nasaprs.com/external/solicitations/solicitations.do"
probe "diu"                    "https://www.diu.mil/work-with-us/open-solicitations"
echo

# ---- 5. the report renders and covers every source ----------------------
echo "[5] health report"
ROWS=$(python3 "$LOOKUP/health_report.py" 2>/dev/null | grep -cE '^\| [a-z]')
WANT=$(python3 -c "import json;print(len(json.load(open('$REG'))['sources']))")
if [ "$ROWS" -eq "$WANT" ]; then
    ok "health_report.py renders all $WANT sources"
else
    bad "health_report.py rendered $ROWS rows, expected $WANT"
fi
echo

echo "----------------------------------------"
if [ "$FAIL" -eq 0 ]; then
    printf '\033[32m%s passed, 0 failed\033[0m\n' "$PASS"
    exit 0
fi
printf '\033[31m%s passed, %s FAILED\033[0m\n' "$PASS" "$FAIL"
exit 1
