#!/usr/bin/env bash
# crawl_darpa.sh — pull DARPA opportunities from the site's own JSON listing endpoint.
#
# Usage:  crawl_darpa.sh <source-corpus-dir>
# Writes: <dir>/raw/<id>.json      (one per opportunity)
#         <dir>/_raw_listing.json  (raw JSON response, for audit)
#         <dir>/_count             (record count)
#
# DARPA's human-facing opportunities page (darpa.mil/work-with-us/opportunities) renders
# a React table (research_opportunities_table.bundle.js) that itself calls
# https://www.darpa.mil/json/opportunity.json — a static, unauthenticated, unpaginated
# JSON export of EVERY currently-open DARPA opportunity: office-wide BAAs, program BAAs,
# SBIR/STTR topics, RFIs, and proposer/industry days. Each row already carries the real
# solicitation number (field_opportunity_number) and a direct link to its authoritative
# posting on sam.gov or dodsbirsttr.mil (field_external_url) — no page-scraping needed to
# find either.
#
# This REPLACES the old opportunities.xml RSS feed as the crawl source. Confirmed
# empirically (2026-09-10): the RSS feed carried only its 10 most-recently-posted items
# against 41 unique opportunities live on this JSON endpoint at the same moment — all 10
# RSS items were a strict subset of the 41. RSS is a recency window; it structurally
# cannot see a standing office-wide BAA that opened months ago and is still open (e.g.
# DARPA-PS-26-115, open since 2026-05-20, close 2027-06-04 — never young enough for RSS's
# top-10, always live on this endpoint). This endpoint has no such window: it is the exact
# feed the DARPA site itself renders as "all current opportunities."
#
# If a user knows about an older/closed DARPA opportunity not on the live listing, they
# can drop its URL into grants/inbox/ and grant-scout will score it via WebFetch.

set -u

DEST="${1:?usage: crawl_darpa.sh <source-corpus-dir>}"
RAW="$DEST/raw"
mkdir -p "$RAW"

LISTING_URL="https://www.darpa.mil/json/opportunity.json"
UA="grant-scout/1.0 (+izuma marketing triage; contact via izumanetworks.com)"

echo "[darpa] fetching full opportunities listing"
listing="$DEST/_raw_listing.json"
http=$(curl -sL -A "$UA" -o "$listing" -w "%{http_code}" "$LISTING_URL")
if [ "$http" != "200" ]; then
    echo "[darpa] HTTP $http for opportunity listing; aborting"
    head -c 300 "$listing" >&2
    rm -f "$listing"
    echo 0 > "$DEST/_count"
    exit 1
fi

python3 - "$listing" "$RAW" <<'PY'
import re, json, sys, html, os
listing_path, raw_dir = sys.argv[1], sys.argv[2]
rows = json.load(open(listing_path))
if not isinstance(rows, list):
    print(f"[darpa] unexpected listing shape: {type(rows).__name__}", file=sys.stderr)
    rows = []

# The endpoint repeats each opportunity once per office/topic taxonomy join; some of the
# repeats blank out the body. Collapse by nid, keeping whichever duplicate has the
# longest body text.
by_nid = {}
for r in rows:
    nid = str(r.get('nid') or '').strip()
    if not nid:
        continue
    body = r.get('field_body_with_summary') or r.get('field_body_with_summary_1') or ''
    prev = by_nid.get(nid)
    if prev is None or len(body) > len((prev.get('field_body_with_summary') or prev.get('field_body_with_summary_1') or '')):
        by_nid[nid] = r

n = 0
live_files = set()
for nid, r in by_nid.items():
    title = (r.get('title') or '').strip()
    desc_html = (r.get('field_body_with_summary') or r.get('field_body_with_summary_1') or '').strip()

    # Body carries the same "blurb + trailing | See X link" shape the old RSS description
    # had; pull out that embedded darpa.mil-relative link when present.
    inner_link = None
    m = re.search(r'href="([^"]+)"', desc_html)
    if m:
        inner_link = m.group(1)
        if inner_link.startswith('/'):
            inner_link = 'https://www.darpa.mil' + inner_link

    desc = re.sub(r'<[^>]+>', ' ', desc_html)
    desc = html.unescape(desc)
    desc = re.sub(r'\s+', ' ', desc).strip()

    # Keep the same guid/filename shape the RSS-based crawler used ("<nid> at
    # https://www.darpa.mil" -> "<nid>_at_https_www.darpa.mil.json") so records for
    # opportunities that persist across a source switch land on the same raw filename
    # instead of duplicating under a new naming scheme.
    guid = f"{nid} at https://www.darpa.mil"
    safe = re.sub(r'[^A-Za-z0-9._-]+', '_', guid).strip('_')[:80] or f"item{n}"

    external_url = (r.get('field_external_url') or '').strip() or None
    sol_number = (r.get('field_opportunity_number') or '').strip() or None
    topics = [t for t in (r.get('field_research_topics') or '').split('|') if t]

    rec = {
        'id': safe,
        'guid': guid,
        'title': title,
        'topic_text': desc,
        # The listing's external_url is always the item's authoritative posting
        # (sam.gov or dodsbirsttr.mil) -- prefer it over a same-site darpa.mil link.
        'url': external_url or inner_link,
        'feed_link': inner_link,
        'posted': r.get('field_open_date') or None,
        'deadline': r.get('field_close_date') or None,
        'office': r.get('field_taxonomy_office') or None,
        'research_topics': topics,
        # The live listing already resolves these; the old RSS-based crawler had to guess
        # them later via enrich_darpa.py page-scraping. Kept alongside the fields
        # normalize.py already reads so nothing downstream has to change to benefit.
        'solicitation_number': sol_number,
        'external_url': external_url,
    }
    json.dump(rec, open(os.path.join(raw_dir, f"{safe}.json"), 'w'), indent=2)
    n += 1
    live_files.add(f"{safe}.json")
print(f"[darpa] wrote {n} opportunity records (from {len(rows)} listing rows, {len(by_nid)} unique)")

# This endpoint is DARPA's current-state listing, not an accumulating feed: an id that
# used to be here and no longer is has closed. Prune its stale raw file so the raw
# directory always mirrors what DARPA is publishing right now, rather than silently
# accumulating closed opportunities forever (the old RSS-window crawler could never do
# this pruning, since it never had visibility into the full live set to prune against).
removed = 0
for fn in os.listdir(raw_dir):
    if fn.endswith('.json') and fn not in live_files:
        os.remove(os.path.join(raw_dir, fn))
        removed += 1
if removed:
    print(f"[darpa] pruned {removed} stale raw record(s) for opportunities no longer listed")
PY

COUNT=$(ls -1 "$RAW"/*.json 2>/dev/null | wc -l | tr -d ' ')
echo "$COUNT" > "$DEST/_count"
echo "[darpa] done. records: ${COUNT}"
