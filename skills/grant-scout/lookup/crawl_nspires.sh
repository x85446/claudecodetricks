#!/usr/bin/env bash
# crawl_nspires.sh — crawl NASA's NSPIRES solicitation listing (form-driven JSP,
# no documented JSON API — this crawler was DISCOVERED, not documented).
#
# Usage:  crawl_nspires.sh <source-corpus-dir> [--refresh]
# Writes: <dir>/raw/<solicitation_number>.json   (one per OPEN/DUE-SOON/FUTURE solicitation)
#         <dir>/_count
#         <dir>/_enum.json
#
# DISCOVERY (browser network trace, 2026-09-10): the public listing page
# (https://nspires.nasaprs.com/external/solicitations/solicitations.do) renders
# via jQuery DataTables in CLIENT-SIDE mode (serverSide:false) — the *entire*
# corpus (3,467 records as of this writing, every status incl. Closed/Past) is
# fetched in ONE unauthenticated POST and then paginated/filtered in-browser.
# That single endpoint is the whole "API":
#
#   POST https://nspires.nasaprs.com/external/solicitations/solicitationsJSON.do?path=open
#   -> {"aaData": [ {title, solicitation_number, release_date, fiscal_year,
#                    proposal_due, selection_date, status, noi_due,
#                    container_type, announcement_type, sId, statusOrder}, ... ]}
#
# No cookie jar, no JSESSIONID dance, no CSRF token — verified with a bare curl
# POST and no prior GET. `path=open` appears to be inert (the response always
# contains every status bucket; the site's own status facets are computed
# client-side from this one payload). status values observed: "Open",
# "Due in 30 Days", "Future", "Closed", "Past" — the site's left-nav status
# facet counts are exactly a count of each bucket in this payload, which is
# what lets fetch_all/validation compare our stored count against the live site.
#
# Detail page (also unauthenticated GET, no session):
#   GET https://nspires.nasaprs.com/external/solicitations/summary%21init.do?solId=<sId>&path=open
# HTML has a reliable structured block:
#   <span class="curOrgLabel">Number:</span> <span class="curOrgData"><strong>V</strong></span>
# repeated for Number/Directorate/Type/Status. The narrative body sits between
# the plain-text markers "Number:" (first curOrgLabel) and "Contact Us" (start
# of the fixed help-desk boilerplate/modal) — sliced out for topic_text.
#
# "Open" for our purposes = currently biddable = status in {Open, Due in 30
# Days, Future} (Future = announced but not yet accepting proposals — kept for
# parity with DSIP's Open+Pre-Release convention; harmless if always 0).
#
# Idempotency: a solicitation whose raw/<num>.json already exists is not
# re-fetched (detail narrative is stable) unless --refresh, but its search-row
# fields (status/dates) are refreshed in place on every run either way, so a
# cache hit can never serve a stale status or deadline.
#
# Staleness: a solicitation that has since left the open set (closed, or
# dropped from the listing) is MOVED to raw_closed/, never deleted, so raw/
# stays exactly the current open+due-soon+future set and _count == len(raw/).

set -u

DEST="${1:?usage: crawl_nspires.sh <source-corpus-dir> [--refresh]}"
REFRESH=0
[ "${2:-}" = "--refresh" ] && REFRESH=1

exec python3 - "$DEST" "$REFRESH" <<'PY'
import json, os, re, sys, time, subprocess, shutil, urllib.parse

DEST    = sys.argv[1]
REFRESH = sys.argv[2] == "1"
RAW     = os.path.join(DEST, "raw")
os.makedirs(RAW, exist_ok=True)

HOST      = "nspires.nasaprs.com"
LIST_URL  = f"https://{HOST}/external/solicitations/solicitationsJSON.do?path=open"
DETAIL_FMT = f"https://{HOST}/external/solicitations/summary%21init.do?solId={{sid}}&path=open"
UA        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
DETAIL_SLEEP = float(os.environ.get("NSPIRES_DETAIL_SLEEP", "0.6"))
OPEN_STATUSES = {"Open", "Due in 30 Days", "Future"}

TMP = os.path.join(DEST, "_nspires.tmp")

def fetch(url, method="GET"):
    """Fetch url with browser UA + retry/backoff. Returns (code:int, text:str)."""
    for attempt in range(1, 5):
        args = ["curl", "-sS", "-A", UA, "-H", "X-Requested-With: XMLHttpRequest",
                "--max-time", "45", "-o", TMP, "-w", "%{http_code}"]
        if method == "POST":
            args += ["-X", "POST"]
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
            print(f"[nspires] HTTP {code} on {url}; backoff {back}s (attempt {attempt})", file=sys.stderr)
            time.sleep(back)
            continue
        return int(code) if code.isdigit() else 0, body
    return int(code) if code.isdigit() else 0, body

def safe_name(num, sid):
    s = re.sub(r"[^A-Za-z0-9._-]+", "_", str(num or "")).strip("_")[:120]
    return s or re.sub(r"[^A-Za-z0-9._-]+", "_", str(sid))[:120]

def clean_text(html):
    text = re.sub(r"(?is)<(script|style)[^>]*>.*?</\1>", " ", html)
    text = re.sub(r"(?s)<[^>]+>", " ", text)
    import html as htmllib
    text = htmllib.unescape(re.sub(r"\s+", " ", text)).strip()
    return text

def parse_detail(html):
    """Return (kv_dict, topic_text) from a solicitation summary page."""
    kv = {}
    for m in re.finditer(
            r'curOrgLabel">\s*([^<]+?):\s*</span>\s*<span\s+class="curOrgData">\s*(?:<strong>)?\s*([^<]*?)\s*(?:</strong>)?\s*</span>',
            html):
        kv[m.group(1).strip()] = re.sub(r"\s+", " ", m.group(2)).strip()
    text = clean_text(html)
    start = text.find("Number:")
    end = text.find("Contact Us", start) if start >= 0 else -1
    if start >= 0:
        topic_text = text[start:end if end >= 0 else None].strip()
    else:
        topic_text = text[:4000]
    return kv, topic_text[:20000]

# --- 1) fetch the whole listing in one shot (no auth, no pagination) ---
print("[nspires] fetching full solicitation listing (single JSON POST, no auth)")
code, body = fetch(LIST_URL, method="POST")
if code != 200 or not body:
    print(f"[nspires] listing HTTP {code}; aborting")
    with open(os.path.join(DEST, "_count"), "w") as f:
        f.write("0\n")
    sys.exit(1)
try:
    data = json.loads(body)
    all_rows = data.get("aaData") or []
except Exception as e:
    print(f"[nspires] listing did not parse as JSON ({e}); aborting")
    with open(os.path.join(DEST, "_count"), "w") as f:
        f.write("0\n")
    sys.exit(1)

by_status = {}
for r in all_rows:
    by_status[r.get("status")] = by_status.get(r.get("status"), 0) + 1
print(f"[nspires] listing total={len(all_rows)} by_status={by_status}")

rows = [r for r in all_rows if r.get("status") in OPEN_STATUSES]
reported = len(rows)
print(f"[nspires] {reported} solicitations currently Open/Due-in-30-Days/Future "
      f"(this is the count that must match the live site's status facets)")

# --- 2) fetch/refresh detail for each open-ish solicitation ---
new = cached = refreshed = skipped = 0
expected = set()
t0 = time.time()
for i, r in enumerate(rows, 1):
    sid = r.get("sId") or ""
    num = r.get("solicitation_number")
    if not sid:
        skipped += 1
        continue
    name = safe_name(num, sid) + ".json"
    expected.add(name)
    target = os.path.join(RAW, name)

    if os.path.exists(target) and os.path.getsize(target) > 0 and not REFRESH:
        try:
            prev = json.load(open(target))
            if isinstance(prev, dict):
                prev.update(r)  # keep status/dates fresh even on a cache hit
                with open(target, "w", encoding="utf-8") as f:
                    json.dump(prev, f, ensure_ascii=False)
                cached += 1
                if time.time() - t0 > 60:
                    print(f"[nspires] progress {i}/{reported} (cached={cached} new={new})")
                    t0 = time.time()
                continue
        except Exception:
            pass  # unreadable cache — fall through and re-fetch

    sid_q = urllib.parse.quote(sid, safe="")
    dc, dbody = fetch(DETAIL_FMT.format(sid=sid_q))
    if dc == 200 and dbody:
        kv, topic_text = parse_detail(dbody)
        rec = dict(r)
        rec["url"] = f"https://{HOST}/external/solicitations/summary!init.do?solId={sid}&path=open"
        rec["directorate"] = kv.get("Directorate")
        rec["type_text"] = kv.get("Type")
        rec["status_text"] = kv.get("Status")
        rec["topic_text"] = topic_text
        with open(target, "w", encoding="utf-8") as f:
            json.dump(rec, f, ensure_ascii=False)
        if REFRESH:
            refreshed += 1
        else:
            new += 1
    else:
        print(f"[nspires] detail sId={sid} num={num} HTTP {dc}; skipping")
        expected.discard(name)
        skipped += 1

    if time.time() - t0 > 60:
        print(f"[nspires] progress {i}/{reported} (new={new} cached={cached} refreshed={refreshed})")
        t0 = time.time()
    time.sleep(DETAIL_SLEEP)

if os.path.exists(TMP):
    os.remove(TMP)

# --- 3) archive solicitations that have since left the open set ---
ARCH = os.path.join(DEST, "raw_closed")
archived = 0
if os.path.isdir(RAW):
    for n in os.listdir(RAW):
        if n.endswith(".json") and n not in expected:
            os.makedirs(ARCH, exist_ok=True)
            shutil.move(os.path.join(RAW, n), os.path.join(ARCH, n))
            archived += 1

count = len([n for n in os.listdir(RAW) if n.endswith(".json")])
with open(os.path.join(DEST, "_count"), "w") as f:
    f.write(f"{count}\n")
json.dump({
    "source": "nspires", "listing_total": len(all_rows), "by_status": by_status,
    "reported_open_due_future": reported, "stored": count,
    "gap": reported - count, "detail_skipped": skipped, "archived_closed": archived,
    "crawled_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
}, open(os.path.join(DEST, "_enum.json"), "w"), indent=2)
print(f"[nspires] done. reported={reported} stored={count} gap={reported - count} "
      f"new={new} cached-skipped={cached} refreshed={refreshed} detail-skipped={skipped} "
      f"archived-closed={archived}")
PY
