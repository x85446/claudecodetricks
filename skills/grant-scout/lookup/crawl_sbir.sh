#!/usr/bin/env bash
# crawl_sbir.sh — crawl SBIR.gov open topics (covers DoD + civilian SBIR/STTR).
#
# Usage:  crawl_sbir.sh <source-corpus-dir> [--refresh]
# Writes: <dir>/raw/<topic_number>.json   (one per OPEN topic)
#         <dir>/_count
#         <dir>/_enum.json   (reported-vs-stored diagnostic)
#
# HISTORY / WHY THIS EXISTS (verified live 2026-09-10):
#   The old endpoint `https://api.www.sbir.gov/public/api/solicitations` has
#   returned HTTP 403 for 28+ days and is dead — SBIR.gov's own docs describe
#   it as "undergoing maintenance" with no ETA. Do not resurrect it.
#
#   The *website* (https://www.sbir.gov/topics) is fully up and unauthenticated.
#   Reverse-engineered live in a browser, then confirmed with a plain `curl`:
#     - A bare `curl https://www.sbir.gov/topics` (no headers at all) gets
#       HTTP 403. Add ONLY a browser-like User-Agent and it returns HTTP 200 —
#       Accept/Referer are not required (tested in isolation). This is the same
#       shape of blocker SAM.gov had (one missing header), not a real API
#       removal or a rate limit.
#     - The default `/topics` view (zero query params, zero cookies) is ALREADY
#       filtered to Status=Open — "Showing 1-10 of 337 results" on a clean,
#       cookie-less request matches the page's own "Open (337)" facet count
#       exactly. No filter query string is needed or used.
#     - Pagination is the standard Drupal pager: `?page=N`, 0-indexed, 10
#       results/page. `page=1` returned "Showing 11-20 of 337" with zero
#       overlap against page 0's topic ids.
#     - The listing HTML carries only a truncated teaser per topic (title +
#       release/open/close dates + program/phase tag badges) with a link to
#       `/topics/<numeric-id>`. Full content — agency, suborg, topic number,
#       parent solicitation number, the untruncated description, and the "View
#       Official Solicitation" outbound link — lives on that per-topic detail
#       page, so this crawler still does one GET per open topic.
#
#   This script never opens a browser. The browser was a discovery instrument
#   only (see above); everything here runs as plain curl.
#
# Output schema (matches normalizers/sbir.py's "HTML-topic schema", the same
# shape as the 2026-08-13 cached raw files this replaces):
#   topic_id, topic_number, solicitation_number_parent, title, url, agency,
#   suborg, agency_url, year, tags, program, status, release_date, open_date,
#   close_date, description
#
# Staleness: a topic that has since closed (present in raw/ from a previous
# run but absent from today's open listing) is MOVED to <dir>/raw_closed/
# (never deleted), so raw/ always holds exactly the current open set and
# _count == len(raw/). normalize.py globs only <source>/raw/*.json.
#
# No caching: every open topic's detail page is re-fetched every run (~337
# requests at a polite pace). Content is small and this keeps raw-file mtimes
# honestly reflecting "we checked this today", per the project rule that
# last_seen in the DB is never evidence of freshness — only raw-file mtimes
# are. --refresh is accepted for interface parity with the other crawlers but
# has no effect (there is nothing cached to force through).

set -u

DEST="${1:?usage: crawl_sbir.sh <source-corpus-dir> [--refresh]}"
RAW="$DEST/raw"
mkdir -p "$RAW"

exec python3 - "$DEST" <<'PY'
import html
import json
import os
import re
import shutil
import subprocess
import sys
import time

DEST = sys.argv[1]
RAW = os.path.join(DEST, "raw")
os.makedirs(RAW, exist_ok=True)

BASE = "https://www.sbir.gov/topics"
UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0 Safari/537.36")
LIST_SLEEP = float(os.environ.get("SBIR_LIST_SLEEP", "0.6"))
DETAIL_SLEEP = float(os.environ.get("SBIR_DETAIL_SLEEP", "0.5"))
MAX_PAGES = int(os.environ.get("SBIR_MAX_PAGES", "80"))  # safety: 80*10 = 800 topics
TMP = os.path.join(DEST, "_sbir.tmp")


def fetch(url):
    """GET url with a browser UA + retry/backoff. Returns (code:int, text:str)."""
    code = "0"
    for attempt in range(1, 5):
        args = ["curl", "-sS", "-A", UA, "-H", "Accept: text/html,application/xhtml+xml",
                "-H", "Referer: https://www.sbir.gov/", "--max-time", "45",
                "-o", TMP, "-w", "%{http_code}", url]
        try:
            cp = subprocess.run(args, capture_output=True, text=True, timeout=60)
            code = re.sub(r"\D", "", (cp.stdout or "")[-3:]) or "0"
        except Exception:
            code = "0"
        body = ""
        if os.path.exists(TMP):
            try:
                body = open(TMP, encoding="utf-8", errors="replace").read()
            except Exception:
                body = ""
        if code == "200":
            return 200, body
        if code == "429" or code.startswith("5") or code == "0":
            back = 4 * attempt
            print(f"[sbir] HTTP {code} on {url}; backoff {back}s (attempt {attempt})", file=sys.stderr)
            time.sleep(back)
            continue
        return int(code) if code.isdigit() else 0, body   # 4xx (non-429): give up on this url
    return int(code) if code.isdigit() else 0, body


def safe_name(*candidates):
    for c in candidates:
        s = re.sub(r"[^A-Za-z0-9._-]+", "_", str(c or "")).strip("_")[:80]
        if s:
            return s
    return "unknown"


def text_between(h, start_pat, end_pat, flags=re.S):
    m = re.search(start_pat, h, flags)
    if not m:
        return None
    tail = h[m.end():]
    m2 = re.search(end_pat, tail, flags)
    return tail[:m2.start()] if m2 else tail


def clean(s):
    if not s:
        return ""
    s = re.sub(r"<(br|/p|/div|/li|/tr)[^>]*>", " \n", s, flags=re.I)
    s = re.sub(r"<[^>]+>", " ", s)
    s = html.unescape(s)
    s = s.replace("\xa0", " ")
    s = re.sub(r"[ \t]+", " ", s)
    s = re.sub(r"\n\s*\n+", "\n", s)
    return s.strip()


# --- 1) enumerate open topic ids across the paginated listing ---
print("[sbir] fetching listing page 0")
code, body = fetch(f"{BASE}?page=0")
if code != 200:
    print(f"[sbir] listing page 0 HTTP {code}; aborting")
    with open(os.path.join(DEST, "_count"), "w") as f:
        f.write("0\n")
    sys.exit(1)

m = re.search(r"Showing\s+[\d,]+-[\d,]+\s+of\s+([\d,]+)\s+results", body)
if not m:
    print("[sbir] could not find 'Showing X-Y of Z results' on listing page 0; aborting")
    with open(os.path.join(DEST, "_count"), "w") as f:
        f.write("0\n")
    sys.exit(1)
reported = int(m.group(1).replace(",", ""))
pages = min(MAX_PAGES, (reported + 9) // 10 if reported else 0)
print(f"[sbir] reported open total={reported} ({pages} pages at 10/page)")

# The listing view has no visible stable-sort tiebreaker, and it is a live view over
# topics that can open/close between requests — a single sequential sweep through
# pages 0..pages-1 has been observed to land short of `reported` (some ids shuffle
# out of every page fetched that sweep, not because they don't exist). Sweep
# repeatedly, by unique topic id (never by page position), until either the full
# reported count is reached or two consecutive full sweeps add nothing new. A page
# that comes back empty of topic links is logged and skipped, never treated as "we
# must be done" — that early-stop caused a real undercount (252/337) in testing.
MAX_SWEEPS = int(os.environ.get("SBIR_MAX_SWEEPS", "4"))
topic_ids, seen = [], set()

for pg in re.findall(r'href="/topics/(\d+)"', body):
    if pg not in seen:
        seen.add(pg)
        topic_ids.append(pg)

for sweep in range(MAX_SWEEPS):
    before = len(topic_ids)
    page_range = range(0, pages) if sweep > 0 else range(1, pages)
    for page in page_range:
        time.sleep(LIST_SLEEP)
        code, pbody = fetch(f"{BASE}?page={page}")
        if code != 200:
            print(f"[sbir] sweep {sweep}: listing page {page} HTTP {code}; skipping this page")
            continue
        found = 0
        for tid in re.findall(r'href="/topics/(\d+)"', pbody):
            if tid not in seen:
                seen.add(tid)
                topic_ids.append(tid)
                found += 1
        if found == 0 and sweep == 0:
            print(f"[sbir] sweep {sweep}: listing page {page} had no new topic ids yet")
    gained = len(topic_ids) - before
    print(f"[sbir] sweep {sweep} done: {len(topic_ids)} unique ids so far (reported {reported}, +{gained} this sweep)")
    if len(topic_ids) >= reported:
        break
    if sweep > 0 and gained == 0:
        print(f"[sbir] sweep {sweep}: no new ids gained; stopping sweeps short of reported total")
        break
    if sweep + 1 < MAX_SWEEPS:
        time.sleep(2)  # let any transient ordering/rate-limit condition clear before re-sweeping

print(f"[sbir] enumerated {len(topic_ids)} unique open topic ids (reported {reported})")

# --- 2) fetch + parse each topic's detail page ---
BADGE_RE = re.compile(
    r'class="margin-right-1 padding-x-105 bg-base-dark[^"]*">([^<]+)</p>')

new = failed = 0
expected = set()
for i, tid in enumerate(topic_ids, 1):
    code, h = fetch(f"{BASE}/{tid}")
    if code != 200 or not h:
        print(f"[sbir] detail {tid} HTTP {code}; skipping")
        failed += 1
        time.sleep(DETAIL_SLEEP)
        continue

    title_m = re.search(r"Back to Funding Opportunities Search</a></p>\s*<h2>(.*?)</h2>", h, re.S)
    title = clean(title_m.group(1)) if title_m else None

    agency = suborg = None
    am = re.search(
        r'<h3>Funding Agency</h3>\s*<p class="margin-bottom-0">(.*?)</p>\s*'
        r'<p class="margin-bottom-0">(.*?)</p>', h, re.S)
    if am:
        agency = clean(am.group(1))
        suborg = clean(am.group(2))

    year_m = re.search(r'<strong>Year:</strong>\s*([^<]+)</p>', h)
    topic_number_m = re.search(r'<strong>Topic Number:</strong>\s*([^<]+)</p>', h)
    sol_parent_m = re.search(r'<strong>Solicitation Number:</strong>\s*([^<]+)</p>', h)
    year = clean(year_m.group(1)) if year_m else None
    topic_number = clean(topic_number_m.group(1)) if topic_number_m else None
    sol_parent = clean(sol_parent_m.group(1)) if sol_parent_m else None

    badges = [clean(b) for b in BADGE_RE.findall(h)]
    tags = " ".join(badges) if badges else None
    program = badges[0] if badges else None

    status_m = re.search(r'Solicitation Status:.*?class="[^"]*">([^<]+)</span>', h, re.S)
    status = clean(status_m.group(1)) if status_m else "Open"

    agency_url_m = re.search(r'href="([^"]+)"\s+class="ext usa-button', h)
    agency_url = agency_url_m.group(1) if agency_url_m else None

    def step_date(label):
        sm = re.search(
            r'<strong class="text-secondary">' + re.escape(label) + r'</strong><br>([^<]+)</p>', h)
        return clean(sm.group(1)) if sm else None

    release_date = step_date("Release Date")
    open_date = step_date("Open Date")
    close_date = step_date("Close Date")

    desc_block = text_between(h, r'<div class="pre-form-wrapped usa-prose">', r'</section>')
    description = clean(desc_block) if desc_block else None

    agency_raw = (agency or "").strip()
    name = safe_name(topic_number, f"SBIR-{tid}")
    expected.add(name + ".json")
    rec = {
        "topic_id": tid,
        "topic_number": topic_number,
        "solicitation_number_parent": sol_parent,
        "title": title,
        "url": f"{BASE}/{tid}",
        "agency": agency_raw,
        "suborg": suborg,
        "agency_url": agency_url,
        "year": year,
        "tags": tags,
        "program": program,
        "status": status,
        "release_date": release_date,
        "open_date": open_date,
        "close_date": close_date,
        "description": description,
    }
    with open(os.path.join(RAW, name + ".json"), "w", encoding="utf-8") as f:
        json.dump(rec, f, ensure_ascii=False)
    new += 1

    if i % 25 == 0 or i == len(topic_ids):
        print(f"[sbir] {i}/{len(topic_ids)} detail pages fetched (failed={failed})", flush=True)
    time.sleep(DETAIL_SLEEP)

if os.path.exists(TMP):
    os.remove(TMP)

# --- 3) archive topics that have since closed, so raw/ == the current open set ---
ARCH = os.path.join(DEST, "raw_closed")
archived = 0
for n in os.listdir(RAW):
    if n.endswith(".json") and n not in expected:
        os.makedirs(ARCH, exist_ok=True)
        shutil.move(os.path.join(RAW, n), os.path.join(ARCH, n))
        archived += 1

count = len([n for n in os.listdir(RAW) if n.endswith(".json")])
with open(os.path.join(DEST, "_count"), "w") as f:
    f.write(f"{count}\n")
json.dump({
    "source": "sbir.gov",
    "reported_open": reported,
    "enumerated_ids": len(topic_ids),
    "stored": count,
    "detail_failed": failed,
    "archived_closed": archived,
    "crawled_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
}, open(os.path.join(DEST, "_enum.json"), "w"), indent=2)
print(f"[sbir] done. reported={reported} enumerated={len(topic_ids)} stored={count} "
      f"failed={failed} archived-closed={archived}")
PY
