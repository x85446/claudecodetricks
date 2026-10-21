#!/usr/bin/env bash
# crawl_grants_extract.sh — pull the Grants.gov DAILY BULK XML EXTRACT.
#
# Usage:  crawl_grants_extract.sh <source-corpus-dir>
# Writes: <dir>/raw/<OpportunityNumber>.json   (one file per OPEN, for-profit-
#                                                eligible opportunity — flat dict
#                                                of the extract's native fields)
#         <dir>/_count
#
# WHY THIS EXISTS:
#   A rate-limit-free, single-file fallback to crawl_grants_gov.sh (Search2 +
#   per-opp fetchOpportunity). The extract is one ~78MB zip (~320MB XML) holding
#   EVERY opportunity Grants.gov has ever posted — including all civilian-agency
#   SBIR/STTR (NASA, NSF, DOE, HHS, USDA, EPA...). When Search2 is slow or the
#   SBIR.gov API is down, this still gets the full civilian picture in one GET.
#
# PUBLIC CONTRACT (no auth):
#   Daily zip:  https://prod-grants-gov-chatbot.s3.amazonaws.com/extracts/GrantsDBExtract<YYYYMMDD>v2.zip
#   Refreshed ~08:40 GMT daily. We try today, then walk back up to 5 days if the
#   current day's file isn't published yet.
#
# FILTERING (the extract is the WHOLE DB, historical included — we must narrow):
#   1. OPEN only: OpportunitySynopsisDetail with CloseDate >= today (or empty
#      CloseDate but ArchiveDate >= today). Forecasts kept when their estimated
#      close/archive is future — flagged phase "Forecast".
#   2. IZUMA-ELIGIBLE only: EligibleApplicants intersects {23 small business,
#      22 other for-profit, 25 other, 99 unrestricted}, OR eligibility is empty/
#      unspecified. Drops state/tribal/nonprofit/university-only grants that
#      would auto-F on eligibility anyway. Keeps the corpus to things we can bid.

set -u

DEST="${1:?usage: crawl_grants_extract.sh <source-corpus-dir>}"
RAW="$DEST/raw"
mkdir -p "$RAW"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) grant-scout/1.0"
BUCKET="https://prod-grants-gov-chatbot.s3.amazonaws.com/extracts"

WORK="$DEST/_work"
mkdir -p "$WORK"
ZIP="$WORK/extract.zip"

# --- locate + download the most recent published extract (today, then back 5 days) ---
FOUND=""
for back in 0 1 2 3 4 5; do
    d=$(date -u -v-"${back}"d +%Y%m%d 2>/dev/null || date -u -d "-${back} day" +%Y%m%d)
    url="${BUCKET}/GrantsDBExtract${d}v2.zip"
    code=$(curl -sIL -A "$UA" -m 30 -o /dev/null -w "%{http_code}" "$url")
    if [ "$code" = "200" ]; then
        echo "[grants_extract] found ${d} extract; downloading (~78MB)"
        if curl -sL -A "$UA" -m 300 -o "$ZIP" "$url" && [ -s "$ZIP" ]; then
            FOUND="$d"; break
        fi
        echo "[grants_extract] download of ${d} failed; trying prior day"
    fi
done

if [ -z "$FOUND" ]; then
    echo "[grants_extract] no extract found in the last 6 days; aborting (leaving prior corpus intact)"
    rm -rf "$WORK"
    # don't clobber _count if a prior good crawl exists
    [ -f "$DEST/_count" ] || echo "0" > "$DEST/_count"
    exit 1
fi

echo "[grants_extract] unzipping"
if ! unzip -o -q "$ZIP" -d "$WORK"; then
    echo "[grants_extract] unzip failed; aborting"
    rm -rf "$WORK"; exit 1
fi
XML=$(ls "$WORK"/*.xml 2>/dev/null | head -1)
if [ -z "$XML" ]; then
    echo "[grants_extract] no XML inside zip; aborting"; rm -rf "$WORK"; exit 1
fi

echo "[grants_extract] streaming + filtering (open + Izuma-eligible)"
python3 - "$XML" "$RAW" "$FOUND" <<'PY'
import sys, os, re, json
import xml.etree.ElementTree as ET
import datetime

xml, raw, extract_date = sys.argv[1], sys.argv[2], sys.argv[3]
today = datetime.datetime.now(datetime.timezone.utc).date()

# for-profit / small-business / unrestricted eligibility codes (Grants.gov codes)
ELIGIBLE = {'22', '23', '25', '99'}

def local(t): return t.split('}')[-1]

def mmddyyyy(s):
    """Grants.gov extract dates are MMDDYYYY strings. -> (date, isostr) or (None,None)."""
    if not s: return None, None
    s = s.strip()
    if not re.fullmatch(r'\d{8}', s): return None, None
    try:
        dt = datetime.datetime.strptime(s, '%m%d%Y').date()
        return dt, dt.strftime('%Y-%m-%d')
    except Exception:
        return None, None

def safe(name):
    s = re.sub(r'[^A-Za-z0-9._-]+', '_', str(name or '')).strip('_')[:90]
    return s

written = scanned = open_kept = elig_dropped = closed_dropped = 0
synopsis_open = forecast_open = dup_number = 0
seen = set()
expected = set()

ctx = ET.iterparse(xml, events=('end',))
for ev, el in ctx:
    lt = local(el.tag)
    if lt not in ('OpportunitySynopsisDetail_1_0', 'OpportunityForecastDetail_1_0'):
        continue
    scanned += 1
    is_forecast = (lt == 'OpportunityForecastDetail_1_0')

    # Collect fields; EligibleApplicants can repeat.
    rec = {}
    elig = []
    for c in el:
        tag = local(c.tag)
        val = (c.text or '').strip()
        if tag == 'EligibleApplicants':
            if val:
                elig.append(val)
        elif tag in rec:
            # rare repeat: keep first, ignore dup
            pass
        else:
            rec[tag] = val
    el.clear()

    # --- OPEN filter ---
    if is_forecast:
        cd, cd_iso = mmddyyyy(rec.get('EstimatedSynopsisCloseDate'))
        ad, ad_iso = mmddyyyy(rec.get('ArchiveDate'))
        # keep forecast if estimated close is future OR unknown-but-archive-future
        horizon = cd or ad
        if horizon and horizon < today:
            closed_dropped += 1
            continue
    else:
        cd, cd_iso = mmddyyyy(rec.get('CloseDate'))
        ad, ad_iso = mmddyyyy(rec.get('ArchiveDate'))
        # open = close date today-or-later; if no close date, fall back to archive date
        horizon = cd or ad
        if horizon is None:
            # no usable date — skip (almost always historical junk)
            closed_dropped += 1
            continue
        if horizon < today:
            closed_dropped += 1
            continue

    # date-wise open, before the scope (eligibility) filter — this is the honest
    # "reported open+upcoming" denominator for this source
    if is_forecast:
        forecast_open += 1
    else:
        synopsis_open += 1

    # --- ELIGIBILITY filter ---
    if elig and not (set(elig) & ELIGIBLE):
        elig_dropped += 1
        continue  # explicitly restricted to classes Izuma isn't

    open_kept += 1

    num = rec.get('OpportunityNumber') or rec.get('OpportunityID')
    key = safe(num)
    if not key:
        continue
    if key in seen:
        dup_number += 1     # same OpportunityNumber posted twice — one file wins
        continue
    seen.add(key)
    expected.add(key + '.json')

    rec['_eligible_applicants'] = elig
    rec['_is_forecast'] = is_forecast
    rec['_extract_date'] = extract_date
    with open(os.path.join(raw, key + '.json'), 'w', encoding='utf-8') as f:
        json.dump(rec, f, ensure_ascii=False)
    written += 1

# --- archive opportunities that have since closed, so raw/ == the current set ---
import shutil, time
arch = os.path.join(os.path.dirname(raw), 'raw_closed')
archived = 0
for n in os.listdir(raw):
    if n.endswith('.json') and n not in expected:
        os.makedirs(arch, exist_ok=True)
        shutil.move(os.path.join(raw, n), os.path.join(arch, n))
        archived += 1

count = len([n for n in os.listdir(raw) if n.endswith('.json')])
print(f"[grants_extract] scanned={scanned} open(date-wise)={synopsis_open + forecast_open} "
      f"(synopsis={synopsis_open} forecast={forecast_open}) closed_dropped={closed_dropped} "
      f"elig_dropped={elig_dropped} dup_number={dup_number} written={written} "
      f"stored={count} archived_closed={archived}")
with open(os.path.join(os.path.dirname(raw), '_count'), 'w') as f:
    f.write(f"{count}\n")
json.dump({"source": "grants.gov.extract",
           "extract_date": extract_date,
           # the extract is the whole historical DB, so its "reported" open+upcoming
           # total is what survives the date filter, before the deliberate
           # for-profit-eligibility scope filter
           "reported_open_upcoming": synopsis_open + forecast_open,
           "reported_synopsis_open": synopsis_open,
           "reported_forecast_open": forecast_open,
           "stored": count,
           "gap": (synopsis_open + forecast_open) - count,
           "gap_reason": (f"{elig_dropped} dropped by the documented for-profit/small-business "
                          f"eligibility scope filter, {dup_number} duplicate OpportunityNumber "
                          f"collisions collapsed to one file"),
           "eligibility_dropped": elig_dropped,
           "duplicate_numbers": dup_number,
           "archived_closed": archived,
           "crawled_utc": time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())},
          open(os.path.join(os.path.dirname(raw), '_enum.json'), 'w'), indent=2)
PY

rm -rf "$WORK"
echo "[grants_extract] done. records: $(cat "$DEST/_count" 2>/dev/null)"
