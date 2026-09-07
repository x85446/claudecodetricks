#!/usr/bin/env bash
# crawl_darpa.sh — pull DARPA opportunities via the official RSS feed.
#
# Usage:  crawl_darpa.sh <source-corpus-dir>
# Writes: <dir>/raw/<guid>.json   (one per opportunity)
#         <dir>/raw/_feed.xml     (raw RSS)
#         <dir>/_count            (record count)
#
# DARPA's opportunities page (darpa.mil/work-with-us/opportunities) is JS-rendered
# and doesn't expose a JSON API. The official RSS feed at
# https://www.darpa.mil/rss/opportunities.xml is the canonical machine-readable
# source DARPA publishes. It carries ~10 most-recent items (covers the typical
# 30–60 day open window for most opportunities).
#
# If a user knows about an older DARPA opportunity not in the feed, they can drop
# its URL into grants/inbox/ and grant-scout will score it via WebFetch.

set -u

DEST="${1:?usage: crawl_darpa.sh <source-corpus-dir>}"
RAW="$DEST/raw"
mkdir -p "$RAW"

FEED_URL="https://www.darpa.mil/rss/opportunities.xml"
UA="grant-scout/1.0 (+izuma marketing triage; contact via izumanetworks.com)"

echo "[darpa] fetching RSS feed"
feed="$RAW/_feed.xml"
http=$(curl -sL -A "$UA" -o "$feed" -w "%{http_code}" "$FEED_URL")
if [ "$http" != "200" ]; then
    echo "[darpa] HTTP $http for RSS; aborting"
    head -c 300 "$feed" >&2
    rm -f "$feed"
    echo 0 > "$DEST/_count"
    exit 1
fi

# Parse the feed into per-item JSON files
python3 - "$feed" "$RAW" <<'PY'
import re, json, sys, html, os
from xml.etree import ElementTree as ET
feed_path, raw_dir = sys.argv[1], sys.argv[2]
tree = ET.parse(feed_path)
root = tree.getroot()
ns = {'dc': 'http://purl.org/dc/elements/1.1/'}
items = root.findall('.//item')
n = 0
for it in items:
    title = (it.findtext('title') or '').strip()
    desc_html = (it.findtext('description') or '').strip()
    pub = (it.findtext('pubDate') or '').strip()
    link = (it.findtext('link') or '').strip()
    guid = (it.findtext('guid') or '').strip() or title

    # Extract any embedded "See Program" / "See Topic" / "See <OFFICE>" link from the description
    inner_link = None
    m = re.search(r'href="([^"]+)"', desc_html)
    if m:
        inner_link = m.group(1)
        if inner_link.startswith('/'):
            inner_link = 'https://www.darpa.mil' + inner_link
        elif inner_link.startswith('research/'):
            inner_link = 'https://www.darpa.mil/' + inner_link

    # Strip HTML from description for topic_text
    desc = re.sub(r'<[^>]+>', ' ', desc_html)
    desc = html.unescape(desc)
    desc = re.sub(r'\s+', ' ', desc).strip()

    # Safe filename from guid (guids look like "4907 at https://www.darpa.mil")
    safe = re.sub(r'[^A-Za-z0-9._-]+', '_', guid).strip('_')[:80] or f"item{n}"
    rec = {
        'id': safe,
        'guid': guid,
        'title': title,
        'topic_text': desc,
        'url': inner_link or link,
        'feed_link': link,
        'posted': pub,
        'deadline': None,
        'office': None,
    }
    # Best-effort office detection from inner link path
    if inner_link:
        m = re.search(r'/about/offices/([a-z]+)', inner_link)
        if m: rec['office'] = m.group(1).upper()
        m = re.search(r'/research/programs/([a-z0-9-]+)', inner_link)
        if m: rec['program'] = m.group(1)
    json.dump(rec, open(os.path.join(raw_dir, f"{safe}.json"), 'w'), indent=2)
    n += 1
print(f"[darpa] wrote {n} opportunity records")
PY

COUNT=$(ls -1 "$RAW"/*.json 2>/dev/null | wc -l | tr -d ' ')
echo "$COUNT" > "$DEST/_count"
echo "[darpa] done. records: ${COUNT}"
