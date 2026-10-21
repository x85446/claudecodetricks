#!/usr/bin/env bash
# crawl_sam_web.sh — crawl SAM.gov contract opportunities via the PUBLIC WEB APP API.
#
# Usage:  crawl_sam_web.sh <source-corpus-dir> [--detail-filter REGEX] [--max-detail N]
# Writes: <dir>/raw/<noticeId>.json   (shape-compatible with the v2 API records that
#                                      normalize_sam() already understands)
#         <dir>/_count
#
# WHY THIS EXISTS
#   api.sam.gov (opportunities/v2/search) requires $SAM_API_KEY. That key died with
#   HTTP 401 / WSO2 code 900901 and SAM.gov rate-limits key regeneration. The SAM.gov
#   WEB APP talks to a DIFFERENT, UNAUTHENTICATED origin that needs no key at all:
#
#   Search:  GET https://sam.gov/api/prod/sgs/v1/search/
#              ?index=opp&page=<0-based>&size=<n>&mode=search&sort=-modifiedDate
#              &mfe=true&q=&qMode=ALL&is_active=true
#            - `size` up to 1000 honored (53k active opps => ~54 pages).
#            - Returns _embedded.results[] + page.totalElements/totalPages.
#            - descriptions[].content is TRUNCATED (~250 chars) in this index.
#
#   Detail:  GET https://sam.gov/api/prod/opps/v2/opportunities/<noticeId>
#            - Returns description[].body = the FULL notice text, plus data2.naics,
#              data2.solicitation.setAside, data2.archive.date, placeOfPerformance.
#
#   HARD REQUIREMENT on BOTH: header `Accept: application/hal+json`. Any other Accept
#   (including application/json alone) returns HTTP 406 "Acceptable representations:
#   [application/hal+json]". This is the single most common way to think the endpoint
#   is broken when it is not.
#
# Auth: NONE. No API key, no login, no session cookie. Verified with plain curl.
#       A browser User-Agent + Referer is still sent to look like ordinary traffic.
#
# Two phases:
#   1. Index sweep  — every active opportunity, metadata + truncated description.
#   2. Detail fetch — full description body, only for notices matching --detail-filter
#      (matched against the agency path). Detail is ~22 KB/record, so fetching all 53k
#      is ~1.2 GB and hours of requests; the filter keeps it to the relevant subset.
#
# Idempotency: a raw/<id>.json that already carries description_text is skipped in
# phase 2 unless its modifiedDate is newer than the stored copy.

set -u

DEST="${1:?usage: crawl_sam_web.sh <source-corpus-dir> [--detail-filter REGEX] [--max-detail N]}"
shift || true

DETAIL_FILTER="${SAM_DETAIL_FILTER:-DEFENSE|ARMY|NAVY|AIR FORCE|DEFENSE LOGISTICS|NASA|HOMELAND|ENERGY, DEPARTMENT OF}"
MAX_DETAIL="${SAM_MAX_DETAIL:-4000}"
while [ $# -gt 0 ]; do
    case "$1" in
        --detail-filter) DETAIL_FILTER="$2"; shift 2 ;;
        --max-detail)    MAX_DETAIL="$2";    shift 2 ;;
        *) echo "[sam-web] unknown arg: $1" >&2; exit 2 ;;
    esac
done

export SAM_DEST="$DEST" SAM_DETAIL_FILTER="$DETAIL_FILTER" SAM_MAX_DETAIL="$MAX_DETAIL"

exec python3 - <<'PY'
import json, os, re, sys, time, urllib.request, urllib.error

DEST   = os.environ["SAM_DEST"]
FILTER = re.compile(os.environ["SAM_DETAIL_FILTER"], re.I)
MAXDET = int(os.environ["SAM_MAX_DETAIL"])
RAW    = os.path.join(DEST, "raw")
os.makedirs(RAW, exist_ok=True)

ORIGIN = "https://sam.gov"
SEARCH = ORIGIN + "/api/prod/sgs/v1/search/"
DETAIL = ORIGIN + "/api/prod/opps/v2/opportunities/"
UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0 Safari/537.36")
# Accept MUST be hal+json or the API answers 406. See header comment.
HDRS = {"User-Agent": UA, "Accept": "application/hal+json", "Referer": ORIGIN + "/search"}

PAGE_SIZE   = int(os.environ.get("SAM_PAGE_SIZE", "1000"))
PAGE_SLEEP  = float(os.environ.get("SAM_PAGE_SLEEP", "1.0"))
DET_SLEEP   = float(os.environ.get("SAM_DETAIL_SLEEP", "0.15"))


def get(url, tries=4):
    for i in range(tries):
        try:
            req = urllib.request.Request(url, headers=HDRS)
            with urllib.request.urlopen(req, timeout=60) as r:
                return json.loads(r.read().decode("utf-8", "replace"))
        except urllib.error.HTTPError as e:
            if e.code in (429, 500, 502, 503, 504) and i < tries - 1:
                time.sleep(5 * (i + 1)); continue
            print(f"[sam-web] HTTP {e.code} {url[:110]}", file=sys.stderr)
            return None
        except Exception as e:
            if i < tries - 1:
                time.sleep(3 * (i + 1)); continue
            print(f"[sam-web] {type(e).__name__} {url[:110]}", file=sys.stderr)
            return None
    return None


def strip_html(s):
    if not s: return ""
    s = re.sub(r"(?is)<(script|style)[^>]*>.*?</\1>", " ", s)
    s = re.sub(r"(?i)<br\s*/?>|</p>|</div>|</li>", "\n", s)
    s = re.sub(r"<[^>]+>", " ", s)
    for a, b in (("&nbsp;", " "), ("&amp;", "&"), ("&lt;", "<"), ("&gt;", ">"),
                 ("&quot;", '"'), ("&#39;", "'"), ("&rsquo;", "'"), ("&ldquo;", '"'),
                 ("&rdquo;", '"'), ("&mdash;", "-"), ("&ndash;", "-")):
        s = s.replace(a, b)
    return re.sub(r"[ \t]{2,}", " ", re.sub(r"\n{3,}", "\n\n", s)).strip()


def org_path(hier):
    names = [h.get("name") for h in (hier or []) if h.get("name")]
    return ".".join(names)


# ---------- phase 1: index sweep ----------
def sweep():
    recs, page, total, pages = {}, 0, None, None
    while True:
        url = (f"{SEARCH}?index=opp&page={page}&mode=search&sort=-modifiedDate"
               f"&size={PAGE_SIZE}&mfe=true&q=&qMode=ALL&is_active=true")
        j = get(url)
        if not j:
            print(f"[sam-web] index sweep aborted at page {page}", file=sys.stderr); break
        if total is None:
            total = (j.get("page") or {}).get("totalElements")
            pages = (j.get("page") or {}).get("totalPages")
            print(f"[sam-web] active opportunities: {total} across {pages} pages "
                  f"(size={PAGE_SIZE})")
        rows = ((j.get("_embedded") or {}).get("results")) or []
        if not rows:
            break
        for o in rows:
            nid = o.get("_id")
            if not nid: continue
            hier = o.get("organizationHierarchy") or []
            descs = o.get("descriptions") or []
            recs[nid] = {
                "noticeId": nid,
                "solicitationNumber": o.get("solicitationNumber"),
                "title": o.get("title"),
                "fullParentPathName": org_path(hier),
                "type": (o.get("type") or {}).get("value"),
                "baseType": (o.get("type") or {}).get("value"),
                "postedDate": o.get("publishDate"),
                "responseDeadLine": o.get("responseDate"),
                "modifiedDate": o.get("modifiedDate"),
                "active": o.get("isActive"),
                "award": o.get("award") or {},
                "uiLink": f"{ORIGIN}/opp/{nid}/view",
                # truncated preview from the index; phase 2 replaces it with the full body
                "description": strip_html((descs[0] or {}).get("content") if descs else ""),
                "_source": "sam.gov web app (sgs/v1/search)",
            }
        got = len(recs)
        print(f"[sam-web] page {page+1}/{pages} — {got}/{total}", flush=True)
        page += 1
        if pages and page >= pages: break
        time.sleep(PAGE_SLEEP)
    return recs


# ---------- phase 2: detail fetch ----------
def detail(recs):
    targets = [r for r in recs.values() if FILTER.search(r.get("fullParentPathName") or "")]
    targets.sort(key=lambda r: r.get("modifiedDate") or "", reverse=True)
    targets = targets[:MAXDET]
    print(f"[sam-web] detail: {len(targets)} notices match filter (cap {MAXDET})")
    ok = skipped = 0
    for i, r in enumerate(targets, 1):
        nid = r["noticeId"]
        fp = os.path.join(RAW, f"{nid}.json")
        if os.path.exists(fp):
            try:
                prev = json.load(open(fp))
                if prev.get("description_text") and prev.get("modifiedDate") == r.get("modifiedDate"):
                    r.update({k: prev[k] for k in ("description_text", "naicsCode",
                              "typeOfSetAside", "typeOfSetAsideDescription", "archiveDate")
                              if k in prev})
                    skipped += 1
                    continue
            except Exception:
                pass
        j = get(DETAIL + nid)
        if j:
            body = "\n\n".join((d.get("body") or "") for d in (j.get("description") or []))
            r["description_text"] = strip_html(body)
            d2 = j.get("data2") or {}
            naics = d2.get("naics") or []
            if naics and naics[0].get("code"):
                r["naicsCode"] = naics[0]["code"][0]
            sol = d2.get("solicitation") or {}
            if sol.get("setAside"):
                r["typeOfSetAside"] = sol["setAside"]
                r["typeOfSetAsideDescription"] = sol["setAside"]
            arch = d2.get("archive") or {}
            if arch.get("date"):
                r["archiveDate"] = arch["date"]
            ok += 1
        if i % 200 == 0:
            print(f"[sam-web] detail {i}/{len(targets)} (fetched={ok} cached={skipped})", flush=True)
        time.sleep(DET_SLEEP)
    print(f"[sam-web] detail done: fetched={ok} cached={skipped}")


recs = sweep()
if not recs:
    print("[sam-web] no records; leaving existing raw/ untouched", file=sys.stderr)
    sys.exit(1)
detail(recs)

for nid, r in recs.items():
    with open(os.path.join(RAW, f"{nid}.json"), "w") as f:
        json.dump(r, f, ensure_ascii=False)

with open(os.path.join(DEST, "_count"), "w") as f:
    f.write(str(len(recs)))
full = sum(1 for r in recs.values() if r.get("description_text"))
print(f"[sam-web] done. records={len(recs)} with_full_text={full} → {RAW}")
PY
