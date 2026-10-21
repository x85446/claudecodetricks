#!/usr/bin/env bash
# crawl_dsip.sh — crawl the DoD SBIR/STTR Innovation Portal (DSIP) open topics.
#
# Usage:  crawl_dsip.sh <source-corpus-dir> [--refresh]
# Writes: <dir>/raw/<topicCode>.json   (one per OPEN topic — search row merged
#                                        with full detail narrative)
#         <dir>/_count
#
# PUBLIC API CONTRACT (verified 2026-08-13; no auth, no maintenance):
#   Search:  GET /topics/api/public/topics/search?searchParam=<urlencoded JSON>&page=N&size=200
#            - PAGINATION PARAMS ARE page (0-indexed) + size. The rowStart/
#              rowsPerPage names the POST body uses are IGNORED on GET (they
#              silently return a fixed first 10 rows — the bug that made this
#              look like a 10/page cap). `size` is honored and not capped
#              (200 works).
#            - FILTERING: loose query params (topicStatus=…) are ignored, but the
#              app's own `searchParam=<urlencoded JSON>` blob IS honored server-
#              side, and `total` then reports the FILTERED count. We filter on
#              topicReleaseStatus, whose live codes are 591=Open, 592=Pre-Release
#              (that exact spelling: hyphen, capital R). [591,592] is the whole
#              open+upcoming set — 93 topics as of 2026-08-13, i.e. one page, not
#              a 165-page sweep of the 32895-topic corpus.
#              (POST to the same path 500s without a session/XSRF token, and no
#              anonymous token is issued — do NOT use POST here.)
#            - each row carries: topicId (a STRING like
#              "000ea09d...c_86256"), topicCode, topicTitle, topicStatus,
#              topicStartDate/topicEndDate (epoch ms), component, program.
#   Detail:  GET /topics/api/public/topics/{topicId}/details
#            - returns objective, description, phase1/2/3Description, keywords,
#              focusAreas, technologyAreas. NOTE the "/details" suffix: bare
#              /topics/{id} and /topics/details/{id} both 500.
#
# Auth: none. A real browser User-Agent + Referer is required.
#
# DNS: the .mil nameservers (dns*.nipr.mil) intermittently SERVFAIL from some
# resolvers. We resolve once at startup (local first, then 8.8.8.8 / 1.1.1.1)
# and pin the IP via curl --resolve so a flaky local resolver can't stall us.
#
# Idempotency: an open topic whose raw/<code>.json already exists and is
# non-empty is SKIPPED (no detail re-fetch) unless --refresh. Respectful:
# sleeps between requests, exponential backoff on 429/5xx.
#
# Staleness: a topic that has since closed is MOVED to <dir>/raw_closed/ (never
# deleted) so raw/ holds exactly the current open+upcoming set and _count equals
# len(raw/). normalize.py globs only <source>/raw/*.json, so the archive is inert.
# <dir>/_enum.json records reported-vs-stored for the run.

set -u

DEST="${1:?usage: crawl_dsip.sh <source-corpus-dir> [--refresh]}"
REFRESH=0
[ "${2:-}" = "--refresh" ] && REFRESH=1

exec python3 - "$DEST" "$REFRESH" <<'PY'
import json, os, re, sys, time, subprocess, socket, shutil, urllib.parse

DEST    = sys.argv[1]
REFRESH = sys.argv[2] == "1"
RAW     = os.path.join(DEST, "raw")
os.makedirs(RAW, exist_ok=True)

HOST   = "www.dodsbirsttr.mil"
BASE   = f"https://{HOST}/topics/api/public/topics"
UA     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
REFER  = "https://www.dodsbirsttr.mil/topics-app/"
PAGE_SIZE = int(os.environ.get("DSIP_PAGE_SIZE", "200"))  # page/size pagination; size is honored (not capped)
PAGE_SLEEP   = float(os.environ.get("DSIP_PAGE_SLEEP", "0.5"))
DETAIL_SLEEP = float(os.environ.get("DSIP_DETAIL_SLEEP", "0.8"))
MAX_TOPICS   = int(os.environ.get("DSIP_MAX_TOPICS", "40000"))   # safety cap
OPEN = lambda s: bool(s) and ("open" in s.lower() or "pre" in s.lower())  # Open + Pre-Release

# Server-side release-status filter. 591=Open, 592=Pre-Release (verified live).
RELEASE_STATUS = [int(x) for x in
                  os.environ.get("DSIP_RELEASE_STATUS", "591,592").split(",") if x.strip()]
SEARCH_PARAM = urllib.parse.quote(json.dumps({
    "searchText": None, "components": None, "programYear": None,
    "solicitationCycleNames": [], "releaseNumbers": [],
    "topicReleaseStatus": RELEASE_STATUS, "modernizationPriorities": [],
    "sortBy": "finalTopicCode,asc", "technologyAreaIds": [],
    "component": None, "program": None,
}, separators=(",", ":")))

TMP = os.path.join(DEST, "_dsip.tmp")

def _resolve():
    """Return a curl --resolve token 'host:443:IP', or '' to use system DNS.

    Probe with curl itself, not socket.getaddrinfo — curl and python can disagree
    (observed: getaddrinfo + dig both fine while curl returned 'Could not resolve
    host'), and it is curl that has to do the real work here."""
    try:
        cp = subprocess.run(["curl", "-sS", "-o", os.devnull, "-m", "20",
                             "-w", "%{http_code}", f"https://{HOST}/"],
                            capture_output=True, text=True, timeout=30)
        if (cp.stdout or "").strip().isdigit() and (cp.stdout or "").strip() != "000":
            return ""                                   # local resolver works
    except Exception:
        pass
    for rs in ("8.8.8.8", "9.9.9.9", "1.1.1.1"):
        try:
            out = subprocess.run(["dig", f"@{rs}", HOST, "+short"],
                                 capture_output=True, text=True, timeout=15).stdout
            for line in out.splitlines():
                line = line.strip()
                if re.match(r"^\d+\.\d+\.\d+\.\d+$", line):
                    print(f"[dsip] local DNS failed; pinning {HOST} -> {line} (via {rs})")
                    return f"{HOST}:443:{line}"
        except Exception:
            continue
    print("[dsip] WARNING: could not pre-resolve host; trying system DNS anyway")
    return ""

RESOLVE = _resolve()

def fetch(url):
    """GET url with browser headers + retry/backoff. Returns (code:int, text:str)."""
    for attempt in range(1, 5):
        args = ["curl", "-sS", "-A", UA,
                "-H", "Accept: application/json", "-H", f"Referer: {REFER}",
                "--max-time", "45", "-o", TMP, "-w", "%{http_code}"]
        if RESOLVE:
            args += ["--resolve", RESOLVE]
        args += [url]
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
        if code in ("429",) or code.startswith("5") or code == "0":
            back = 4 * attempt
            print(f"[dsip] HTTP {code} on {url}; backoff {back}s (attempt {attempt})", file=sys.stderr)
            time.sleep(back)
            continue
        return int(code), body                          # 4xx (non-429): give up on this url
    return int(code) if code.isdigit() else 0, body

def safe_name(code, tid):
    s = re.sub(r"[^A-Za-z0-9._-]+", "_", str(code or "")).strip("_")[:80]
    return s or re.sub(r"[^A-Za-z0-9._-]+", "_", str(tid))[:80]

def enumerate_topics(search_param):
    """Page the search endpoint. Returns (rows, reported_total) or (None, None)
    if the very first page failed."""
    rows, total, page = [], None, 0
    while page * PAGE_SIZE < MAX_TOPICS:
        qs = f"searchParam={search_param}&" if search_param else "searchText=&"
        code, body = fetch(f"{BASE}/search?{qs}page={page}&size={PAGE_SIZE}")
        if code != 200:
            print(f"[dsip] search page={page} HTTP {code}; stopping.")
            return (None, None) if page == 0 else (rows, total)
        try:
            d = json.loads(body)
        except Exception:
            print(f"[dsip] page={page} non-JSON; stopping.")
            return (None, None) if page == 0 else (rows, total)
        batch = d.get("data") or []
        if total is None:
            total = d.get("total") or 0
            print(f"[dsip] reported total={total} (~{(total + PAGE_SIZE - 1)//PAGE_SIZE or 1} pages)")
        if not batch:
            break
        rows.extend(batch)
        page += 1
        if total and page * PAGE_SIZE >= total:
            break
        time.sleep(PAGE_SLEEP)
    return rows, total

# --- 1) enumerate the open+upcoming set (server-side filter, one small page) ---
print(f"[dsip] searching topicReleaseStatus={RELEASE_STATUS} (591=Open, 592=Pre-Release), "
      f"page/size={PAGE_SIZE}, refresh={int(REFRESH)}")
rows, reported = enumerate_topics(SEARCH_PARAM)
filtered_ok = rows is not None and rows

if not filtered_ok:
    # Server-side filter unavailable (API shape changed / outage) — fall back to
    # the full-corpus sweep with the client-side status filter. Slower, same set.
    print("[dsip] filtered search returned nothing; falling back to full sweep + client-side filter")
    allrows, corpus_total = enumerate_topics("")
    if allrows is None:
        with open(os.path.join(DEST, "_count"), "w") as f:
            f.write("0\n")
        sys.exit(1)
    rows = [r for r in allrows if OPEN(r.get("topicStatus"))]
    reported = len(rows)   # no independent server-side count in this mode
    print(f"[dsip] swept {len(allrows)}/{corpus_total} topics; {len(rows)} Open/Pre-Release")

# The server filter is authoritative, but re-assert client-side so a silently
# ignored filter can never smuggle Closed topics into the corpus.
rows = [r for r in rows if OPEN(r.get("topicStatus"))]
by_status = {}
for r in rows:
    by_status[r.get("topicStatus")] = by_status.get(r.get("topicStatus"), 0) + 1
print(f"[dsip] enumerated {len(rows)} open+upcoming topics {by_status} (reported {reported})")

# --- 2) fetch detail for each ---
new = cached = refreshed = skipped = 0
expected = set()
for r in rows:
    tid   = r.get("topicId")
    code_ = r.get("topicCode") or r.get("finalTopicCode") or tid
    if not tid:
        skipped += 1
        continue
    name = safe_name(code_, tid) + ".json"
    expected.add(name)
    target = os.path.join(RAW, name)
    if os.path.exists(target) and os.path.getsize(target) > 0 and not REFRESH:
        # The narrative detail is expensive and stable, so keep it cached — but the
        # search row is free and NOT stable: topicStatus flips Pre-Release→Open and
        # topicEndDate (which normalize.py turns into close_date) moves when a topic
        # opens. Refresh the row fields in place so a cache hit can't serve a stale
        # status or deadline. Keys added by later enrichment passes are untouched.
        try:
            prev = json.load(open(target))
            if isinstance(prev, dict):
                prev.update(r)
                with open(target, "w", encoding="utf-8") as f:
                    json.dump(prev, f, ensure_ascii=False)
                cached += 1
                continue
        except Exception:
            pass    # unreadable cache — fall through and re-fetch it properly
    dc, dbody = fetch(f"{BASE}/{tid}/details")
    if dc == 200 and dbody:
        try:
            detail = json.loads(dbody)
        except Exception:
            detail = {}
        if isinstance(detail, dict) and not detail.get("errorMessages"):
            merged = {**r, **detail}                 # detail wins; search-row metadata kept
            with open(target, "w", encoding="utf-8") as f:
                json.dump(merged, f, ensure_ascii=False)
            if REFRESH:
                refreshed += 1
            else:
                new += 1
        else:
            print(f"[dsip] detail {tid} returned error payload; skipping")
            expected.discard(name)
            skipped += 1
    else:
        print(f"[dsip] detail {tid} HTTP {dc}; skipping")
        expected.discard(name)
        skipped += 1
    time.sleep(DETAIL_SLEEP)

if os.path.exists(TMP):
    os.remove(TMP)

# --- 3) archive topics that have since closed, so raw/ == the current set ---
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
json.dump({"source": "dsip", "reported_open_upcoming": reported, "stored": count,
           "gap": (reported - count) if isinstance(reported, int) else None,
           "by_status": by_status, "detail_skipped": skipped, "archived_closed": archived,
           "crawled_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())},
          open(os.path.join(DEST, "_enum.json"), "w"), indent=2)
print(f"[dsip] done. reported={reported} stored={count} gap={reported - count if isinstance(reported,int) else '?'} "
      f"new={new} cached-skipped={cached} refreshed={refreshed} detail-skipped={skipped} archived-closed={archived}")
PY
