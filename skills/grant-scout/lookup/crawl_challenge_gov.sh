#!/usr/bin/env bash
# crawl_challenge_gov.sh — crawl active federal challenges/prize competitions
# (HTML scrape of the challenge.gov successor listing on usa.gov).
#
# Usage:  crawl_challenge_gov.sh <source-corpus-dir> [--refresh]
# Writes: <dir>/raw/<slug>.json   (one per challenge: title, tagline, description,
#                                  sponsoring agency, start/end date, challenge type,
#                                  prizes, contact, apply URL, last-updated)
#         <dir>/_count
#
# IMPORTANT — why this hits usa.gov and not challenge.gov:
#   The challenge.gov platform itself was SUNSET on 2026-03-30 (confirmed live:
#   https://www.challenge.gov/ now serves a static "content has moved" notice,
#   zero listings, no API of any kind — the old /api/challenges endpoint is gone
#   for good, not merely relocated). The notice page names the successor for
#   public listings: USA.gov's Innovation section. That trail leads to:
#     https://www.usa.gov/find-active-challenge   (listing: links to every open
#                                                    challenge's detail page)
#     https://www.usa.gov/challenges/<slug>        (detail: full metadata)
#   Both are plain server-rendered Drupal HTML (verified via curl with no JS/
#   browser needed) with no pagination as of this writing (12/12 listed
#   challenges fit on one page). So this crawler treats usa.gov's
#   find-active-challenge listing as the live "challenge.gov" source for
#   grant-scout purposes — it is the only place this data still lives.
#
# Idempotency: a challenge whose raw/<slug>.json already exists is SKIPPED
# unless --refresh (dates/prize amounts can shift over a long-running
# challenge's life, so the weekly cadence should pass --refresh; a same-day
# re-run skips). Respectful: sleeps between page fetches, backoff on 429/5xx.

set -u

DEST="${1:?usage: crawl_challenge_gov.sh <source-corpus-dir> [--refresh]}"
REFRESH=0
[ "${2:-}" = "--refresh" ] && REFRESH=1
RAW="$DEST/raw"
mkdir -p "$RAW"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
LIST="https://www.usa.gov/find-active-challenge"
NEW=0; CACHED=0; REFRESHED=0; FAILED=0

get() { # $1=outfile $2=url  -> echoes http code, retries 429/5xx
    local out="$1" url="$2" attempt=1 http
    while [ "$attempt" -le 4 ]; do
        http=$(curl -sSL "$url" -A "$UA" -H "Accept: text/html" --max-time 45 -o "$out" -w "%{http_code}")
        case "$http" in
            200) echo "$http"; return 0 ;;
            429|5??) local b=$((5*attempt)); echo "[challenge_gov] HTTP $http on $url; backoff ${b}s" >&2; sleep "$b"; attempt=$((attempt+1)) ;;
            *) echo "$http"; return 0 ;;
        esac
    done
    echo "$http"; return 0
}

echo "[challenge_gov] fetching active-challenge listing (refresh=$REFRESH)"
listing="$DEST/_listing.html"
http=$(get "$listing" "$LIST")
if [ "$http" != "200" ]; then
    echo "[challenge_gov] listing HTTP $http; aborting"; echo 0 > "$DEST/_count"; rm -f "$listing"; exit 1
fi

# Extract unique /challenges/<slug> detail links (excludes the /challenges hub
# page itself and sibling pages like /challenge-how-to-participate, which don't
# match the /challenges/<slug> shape).
mapfile -t slugs < <(grep -oE 'href="/challenges/[a-z0-9][a-z0-9-]*"' "$listing" \
    | sed -E 's#href="/challenges/##; s#"$##' | sort -u)
echo "[challenge_gov] found ${#slugs[@]} active challenges on the live listing"

if [ "${#slugs[@]}" -eq 0 ]; then
    echo "[challenge_gov] zero challenges parsed from listing — treating as a failure, not a quiet zero"
    rm -f "$listing"
    echo 0 > "$DEST/_count"
    exit 1
fi

for slug in "${slugs[@]}"; do
    target="$RAW/$slug.json"
    if [ -s "$target" ] && [ "$REFRESH" -eq 0 ]; then CACHED=$((CACHED+1)); continue; fi
    url="https://www.usa.gov/challenges/$slug"
    page="$RAW/.page.tmp"
    dh=$(get "$page" "$url")
    if [ "$dh" != "200" ] || [ ! -s "$page" ]; then
        echo "[challenge_gov] $slug HTTP $dh; skipping"
        rm -f "$page"
        FAILED=$((FAILED+1))
        continue
    fi
    python3 - "$page" "$slug" "$url" "$target" <<'PY'
import sys, re, json, html

page, slug, url, target = sys.argv[1:5]
h = open(page, encoding='utf-8', errors='ignore').read()

def unescape_tags(s):
    if s is None:
        return ''
    s = re.sub(r'(?i)<br\s*/?>', '; ', s)
    s = re.sub(r'(?i)</(p|li|tr|div)>', '. ', s)
    s = re.sub(r'(?i)<li>', ' - ', s)
    s = re.sub(r'<[^>]+>', ' ', s)
    s = html.unescape(s)
    s = re.sub(r'\s+', ' ', s).strip()
    s = re.sub(r'\s*;\s*\.\s*', '; ', s)
    s = re.sub(r'^[\s;.]+|[\s;.]+$', '', s)
    return s

m = re.search(r'(?s)<h1[^>]*>\s*<span>(.*?)</span>', h)
title = unescape_tags(m.group(1)) if m else slug

m = re.search(r'(?s)<p class="usa-intro[^"]*">(.*?)</p>', h)
tagline = unescape_tags(m.group(1)) if m else ''

m = re.search(r'(?s)<div class="usa-prose body-copy">(.*?)<table class="table usa-table">', h)
description = unescape_tags(m.group(1)) if m else ''

m = re.search(r'(?s)<table class="table usa-table">.*?</table>', h)
table_html = m.group(0) if m else ''
rows = re.findall(r'(?s)<tr><th>(.*?)</th><td>(.*?)</td></tr>', table_html)
info = {}
for k, v in rows:
    info[unescape_tags(k).rstrip(':').strip().lower()] = unescape_tags(v)

m = re.search(r'(?s)<a class="usa-button[^"]*"\s+href="([^"]+)">', h)
apply_url = m.group(1) if m else None

m = re.search(r'(?s)<em>(These federal prize competitions.*?)</em>', h)
eligibility_note = unescape_tags(m.group(1)) if m else None

m = re.search(r'(?s)id="last-updated">\s*LAST UPDATED:\s*(.*?)</p>', h)
last_updated = unescape_tags(m.group(1)) if m else None

if not info:
    print(f"[challenge_gov] {slug}: WARNING no Key-information table found (page shape may have changed)", file=sys.stderr)

rec = {
    'id': slug,
    'title': title,
    'tagline': tagline,
    'description': description,
    'sponsoring_agency': info.get('sponsoring agency'),
    'start_date': info.get('start date'),
    'end_date': info.get('end date'),
    'challenge_type': info.get('challenge type'),
    'prizes': info.get('prizes'),
    'contact': info.get('contact'),
    'apply_url': apply_url,
    'usagov_url': url,
    'eligibility_note': eligibility_note,
    'last_updated': last_updated,
}
json.dump(rec, open(target, 'w'), indent=2, ensure_ascii=False)
print(f"[challenge_gov] {slug}: agency={rec['sponsoring_agency']!r} end={rec['end_date']!r}")
PY
    rm -f "$page"
    if [ -s "$target" ]; then
        if [ "$REFRESH" -eq 1 ]; then REFRESHED=$((REFRESHED+1)); else NEW=$((NEW+1)); fi
    fi
    sleep 1   # respectful
done
rm -f "$listing"

COUNT=$(ls -1 "$RAW"/*.json 2>/dev/null | wc -l | tr -d ' ')
echo "$COUNT" > "$DEST/_count"
echo "[challenge_gov] done. new=$NEW cached-skipped=$CACHED refreshed=$REFRESHED failed=$FAILED total=$COUNT"
