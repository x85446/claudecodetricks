#!/usr/bin/env bash
# crawl_xtech.sh — crawl Army xTech prize competitions (HTML scrape).
#
# Usage:  crawl_xtech.sh <source-corpus-dir> [--refresh]
# Writes: <dir>/raw/<slug>.json    (one per competition: title, url, status, deadline, text)
#         <dir>/_count
#
# xTech (xtech.army.mil) is a WordPress site whose REST API is locked
# (401 "DRA: Only authenticated users"). So we scrape:
#   1. https://xtech.army.mil/competitions/  -> list of /competition/<slug>/ links
#   2. each /competition/<slug>/             -> detail page; extract title + deadline + body
#
# Idempotency: a competition whose raw/<slug>.json already exists is SKIPPED
# unless --refresh (xTech detail pages change as deadlines update, so the weekly
# cadence should pass --refresh; a same-day re-run skips). Respectful: sleeps
# between page fetches, backoff on 429/5xx.

set -u

DEST="${1:?usage: crawl_xtech.sh <source-corpus-dir> [--refresh]}"
REFRESH=0
[ "${2:-}" = "--refresh" ] && REFRESH=1
RAW="$DEST/raw"
mkdir -p "$RAW"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
LIST="https://xtech.army.mil/competitions/"
NEW=0; CACHED=0; REFRESHED=0

get() { # $1=outfile $2=url  -> echoes http code, retries 429/5xx
    local out="$1" url="$2" attempt=1 http
    while [ "$attempt" -le 4 ]; do
        http=$(curl -sSL "$url" -A "$UA" -H "Accept: text/html" --max-time 45 -o "$out" -w "%{http_code}")
        case "$http" in
            200) echo "$http"; return 0 ;;
            429|5??) local b=$((5*attempt)); echo "[xtech] HTTP $http on $url; backoff ${b}s" >&2; sleep "$b"; attempt=$((attempt+1)) ;;
            *) echo "$http"; return 0 ;;
        esac
    done
    echo "$http"; return 0
}

echo "[xtech] fetching competitions listing (refresh=$REFRESH)"
listing="$DEST/_listing.html"
http=$(get "$listing" "$LIST")
if [ "$http" != "200" ]; then
    echo "[xtech] listing HTTP $http; aborting"; echo 0 > "$DEST/_count"; rm -f "$listing"; exit 1
fi

# Extract unique competition detail URLs (host may be xtech.army.mil or www.xtech.army.mil).
mapfile -t urls < <(grep -oE 'https://(www\.)?xtech\.army\.mil/competition/[a-z0-9-]+' "$listing" | sed 's#www\.##' | sort -u)
echo "[xtech] found ${#urls[@]} competitions"

for url in "${urls[@]}"; do
    slug=$(printf '%s' "$url" | sed -E 's#.*/competition/##; s#/$##')
    target="$RAW/$slug.json"
    if [ -s "$target" ] && [ "$REFRESH" -eq 0 ]; then CACHED=$((CACHED+1)); continue; fi
    page="$RAW/.page.tmp"
    dh=$(get "$page" "$url")
    if [ "$dh" != "200" ] || [ ! -s "$page" ]; then echo "[xtech] $slug HTTP $dh; skipping"; rm -f "$page"; continue; fi
    python3 - "$page" "$url" "$slug" "$target" <<'PY'
import sys, re, json, html, datetime
page, url, slug, target = sys.argv[1:5]
h = open(page, encoding='utf-8', errors='ignore').read()
def meta(prop):
    m = re.search(r'<meta[^>]+(?:property|name)=["\']'+re.escape(prop)+r'["\'][^>]+content=["\']([^"\']+)', h, re.I)
    return html.unescape(m.group(1)).strip() if m else None
title = meta('og:title')
if not title:
    m = re.search(r'<title>(.*?)</title>', h, re.I|re.S); title = html.unescape(m.group(1)).strip() if m else slug
# Plain-text body for keyword/deadline mining
text = re.sub(r'(?is)<(script|style)[^>]*>.*?</\1>', ' ', h)
text = re.sub(r'(?s)<[^>]+>', ' ', text)
text = html.unescape(re.sub(r'\s+', ' ', text)).strip()

# Deadline: find any date-like token, then check for a due/close/deadline/end
# cue word in the ~40 chars BEFORE it, decoupled from strict verb-phrase
# adjacency. The old regex required the cue phrase to sit immediately next to
# the date (e.g. "submission(s) close|due <date>"), which missed real xTech
# copy verified live 2026-09-10 -- "Submissions are due October 19, 2026" (the
# word "are" breaks the old adjacency match). That is the visible half of the
# bug (a genuinely OPEN competition read status=unknown); the more
# consequential half is that 8 of 11 genuinely CLOSED competitions ALSO came
# back deadline=None under the old regex, and normalizers/xtech.py turns a
# null deadline into a null close_date -- a null close_date makes the
# active_opportunities view (`close_date IS NULL OR close_date >= date('now')`)
# treat an already-closed prize competition as perpetually open forever.
DATE_RE = re.compile(
    r'((?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\.?\s+'
    r'\d{1,2}(?:st|nd|rd|th)?,?\s+\d{4}|\d{1,2}/\d{1,2}/\d{2,4})')
deadline = None
for dm in DATE_RE.finditer(text):
    context = text[max(0, dm.start() - 40):dm.start()]
    if re.search(r'\b(due|close[sd]?|closing|deadline|end(?:s|ing)?|through)\b', context, re.I):
        # Strip ordinal suffixes (30th -> 30) so the stored string round-trips
        # cleanly through normalizers/xtech.py's own strptime formats, which
        # don't strip them.
        deadline = re.sub(r'(\d+)(?:st|nd|rd|th)\b', r'\1', dm.group(1))
        break

# Status: prefer the parsed deadline (objective, date-vs-today) over keyword
# sniffing -- xTech copy is inconsistent and often never says "now open" at
# all, so a page whose only signal is a future due-date was falling through
# to 'unknown' rather than 'open'.
today = datetime.date.today()
parsed = None
if deadline:
    cleaned = deadline.replace('.', '').strip()
    for fmt in ('%b %d, %Y', '%B %d, %Y', '%b %d %Y', '%B %d %Y', '%m/%d/%Y', '%m/%d/%y'):
        try:
            parsed = datetime.datetime.strptime(cleaned, fmt).date()
            break
        except Exception:
            continue

if parsed:
    status = 'open' if parsed >= today else 'closed'
elif re.search(r'\b(now open|currently open|open for submission|apply now|accepting)', text, re.I):
    status = 'open'
elif re.search(r'\b(closed|winners announced|competition has ended)\b', text, re.I):
    status = 'closed'
else:
    status = 'unknown'

rec = {
    'id': slug, 'title': title, 'url': url,
    'status': status, 'deadline': deadline,
    'topic_text': text[:4000],
}
json.dump(rec, open(target, 'w'), indent=2, ensure_ascii=False)
print(f"[xtech] {slug}: status={status} deadline={deadline}")
PY
    rm -f "$page"
    if [ -s "$target" ]; then
        if [ "$REFRESH" -eq 1 ]; then REFRESHED=$((REFRESHED+1)); else NEW=$((NEW+1)); fi
    fi
    sleep 2   # respectful
done
rm -f "$listing"

# Archive competitions no longer on the live listing, so raw/ == the current set and
# _count can't silently keep counting a competition the site dropped. Observed live
# 2026-09-10: the listing shrank from 14 to 12 (xtechcounterstrike, xtechoverwatch
# fell off it); without this, those two files would sit forever with a Jun/Aug mtime
# while still counting toward "total" as if freshly checked today — the exact
# silent-staleness failure mode this whole crawl exists to avoid. Never deleted.
ARCH="$DEST/raw_closed"
ARCHIVED=0
declare -A EXPECTED
for url in "${urls[@]}"; do
    slug=$(printf '%s' "$url" | sed -E 's#.*/competition/##; s#/$##')
    EXPECTED["$slug.json"]=1
done
for f in "$RAW"/*.json; do
    [ -e "$f" ] || continue
    n=$(basename "$f")
    if [ -z "${EXPECTED[$n]:-}" ]; then
        mkdir -p "$ARCH"
        mv "$f" "$ARCH/$n"
        ARCHIVED=$((ARCHIVED+1))
        echo "[xtech] $n no longer on live listing; archived to raw_closed/"
    fi
done

COUNT=$(ls -1 "$RAW"/*.json 2>/dev/null | wc -l | tr -d ' ')
echo "$COUNT" > "$DEST/_count"
echo "[xtech] done. new=$NEW cached-skipped=$CACHED refreshed=$REFRESHED archived-closed=$ARCHIVED total=$COUNT"
