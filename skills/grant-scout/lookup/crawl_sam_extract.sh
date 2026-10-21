#!/usr/bin/env bash
# crawl_sam_extract.sh — SAM.gov contract opportunities via the PUBLIC BULK CSV EXTRACT.
#
# Usage:  crawl_sam_extract.sh <source-corpus-dir> [--keep-csv]
# Writes: <dir>/extract/ContractOpportunitiesFullCSV.csv   (the raw download)
#         <dir>/raw/<NoticeId>.json                        (shape normalize_sam() reads)
#         <dir>/_count
#
# WHY THIS EXISTS — replaces crawl_sam.sh
#   crawl_sam.sh calls api.sam.gov/opportunities/v2/search, which requires $SAM_API_KEY.
#   That key died (HTTP 401, WSO2 code 900901) and SAM.gov rate-limits regeneration.
#   This path needs NO KEY, NO LOGIN, NO SESSION — and is strictly better data:
#
#     metric                  old API crawl        this extract
#     records                 13,912               ~85,000
#     full description text   461 (3.3%)           ~55,400 (65%)
#     requests               ~14,000               1
#     wall clock              ~40 min              ~7 s
#
#   The old crawler stored a *noticedesc URL string* in topic_text for most records and
#   needed a second authenticated call per notice to resolve it. The CSV carries the
#   full Description inline, so one download yields what used to take 14k API calls.
#
# ENDPOINT
#   GET https://sam.gov/api/prod/fileextractservices/v1/api/download/
#       Contract%20Opportunities/datagov/ContractOpportunitiesFullCSV.csv?privacy=Public
#   -> HTTP 303 to a presigned falextracts.s3.amazonaws.com URL (valid 2700s).
#      Follow redirects (-L). A HEAD on the presigned URL 403s because the signature
#      covers GET only — that is expected, not a failure. Use a ranged GET to probe.
#   Refreshed daily by GSA. ~235 MB.
#
#   Sibling public extracts live under the same path prefix; browse them at
#   https://sam.gov/data-services/Contract%20Opportunities/datagov?privacy=Public
#
# NOTE ON THE OTHER NO-KEY ROUTE (see crawl_sam_web.sh): the web app's own JSON API at
#   /api/prod/sgs/v1/search/  (Accept: application/hal+json REQUIRED, else HTTP 406)
#   also needs no key, but Elasticsearch caps deep paging at 10,000 results
#   (page*size > 10000 -> HTTP 400) and no date/type filter param is honored, so it
#   cannot enumerate the full set. It remains useful for targeted per-notice lookups via
#   /api/prod/opps/v2/opportunities/<noticeId>, which returns the full description body.

set -u

DEST="${1:?usage: crawl_sam_extract.sh <source-corpus-dir> [--keep-csv]}"
KEEP_CSV=0
[ "${2:-}" = "--keep-csv" ] && KEEP_CSV=1

EXTRACT_DIR="$DEST/extract"
CSV="$EXTRACT_DIR/ContractOpportunitiesFullCSV.csv"
mkdir -p "$EXTRACT_DIR" "$DEST/raw"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
URL="https://sam.gov/api/prod/fileextractservices/v1/api/download/Contract%20Opportunities/datagov/ContractOpportunitiesFullCSV.csv?privacy=Public"

if [ "${SAM_SKIP_DOWNLOAD:-0}" != "1" ]; then
    echo "[sam-extract] downloading ContractOpportunitiesFullCSV.csv (~235 MB)"
    http=$(curl -sSL -A "$UA" "$URL" -o "$CSV" -w '%{http_code}')
    if [ "$http" != "200" ]; then
        echo "[sam-extract] HTTP $http downloading extract; aborting" >&2
        exit 1
    fi
    echo "[sam-extract] downloaded $(wc -c < "$CSV" | tr -d ' ') bytes"
else
    echo "[sam-extract] SAM_SKIP_DOWNLOAD=1 — using existing $CSV"
fi

export SAM_CSV="$CSV" SAM_RAW="$DEST/raw" SAM_DEST="$DEST"

python3 - <<'PY'
import csv, json, os, re, sys

CSV = os.environ["SAM_CSV"]; RAW = os.environ["SAM_RAW"]; DEST = os.environ["SAM_DEST"]
csv.field_size_limit(sys.maxsize)
os.makedirs(RAW, exist_ok=True)


def strip_html(s):
    if not s:
        return ""
    s = re.sub(r"(?is)<(script|style)[^>]*>.*?</\1>", " ", s)
    s = re.sub(r"(?i)<br\s*/?>|</p>|</div>|</li>|</tr>", "\n", s)
    s = re.sub(r"<[^>]+>", " ", s)
    for a, b in (("&nbsp;", " "), ("&amp;", "&"), ("&lt;", "<"), ("&gt;", ">"),
                 ("&quot;", '"'), ("&#39;", "'"), ("&rsquo;", "'"), ("&lsquo;", "'"),
                 ("&ldquo;", '"'), ("&rdquo;", '"'), ("&mdash;", "-"), ("&ndash;", "-")):
        s = s.replace(a, b)
    return re.sub(r"[ \t]{2,}", " ", re.sub(r"\n{3,}", "\n\n", s)).strip()


def g(row, *names):
    for n in names:
        v = row.get(n)
        if v not in (None, ""):
            return v.strip()
    return None


n = written = with_text = 0
with open(CSV, newline="", encoding="utf-8", errors="replace") as fh:
    for row in csv.DictReader(fh):
        n += 1
        nid = g(row, "NoticeId")
        if not nid:
            continue
        # Agency path, dot-joined, matching the v2 API's fullParentPathName convention
        path = ".".join([p for p in (g(row, "Department/Ind.Agency"),
                                     g(row, "Sub-Tier"),
                                     g(row, "Office")) if p])
        desc = g(row, "Description") or ""
        desc_text = "" if desc.startswith("http") else strip_html(desc)
        if desc_text:
            with_text += 1
        award_amt = g(row, "Award$")
        rec = {
            "noticeId": nid,
            "solicitationNumber": g(row, "Sol#"),
            "title": g(row, "Title"),
            "fullParentPathName": path or None,
            "type": g(row, "Type"),
            "baseType": g(row, "BaseType"),
            "postedDate": g(row, "PostedDate"),
            "responseDeadLine": g(row, "ResponseDeadLine"),
            "archiveDate": g(row, "ArchiveDate"),
            "archiveType": g(row, "ArchiveType"),
            "typeOfSetAside": g(row, "SetASideCode"),
            "typeOfSetAsideDescription": g(row, "SetASide"),
            "naicsCode": g(row, "NaicsCode"),
            "classificationCode": g(row, "ClassificationCode"),
            "active": g(row, "Active"),
            "award": ({"amount": award_amt, "number": g(row, "AwardNumber"),
                       "date": g(row, "AwardDate"), "awardee": g(row, "Awardee")}
                      if award_amt else {}),
            "uiLink": g(row, "Link") or f"https://sam.gov/opp/{nid}/view",
            "additionalInfoLink": g(row, "AdditionalInfoLink"),
            "placeOfPerformance": {
                "city": {"name": g(row, "PopCity")},
                "state": {"code": g(row, "PopState")},
                "zip": g(row, "PopZip"),
                "country": {"code": g(row, "PopCountry")},
            },
            "pointOfContact": [p for p in [
                ({"fullName": g(row, "PrimaryContactFullname"),
                  "email": g(row, "PrimaryContactEmail"),
                  "phone": g(row, "PrimaryContactPhone"), "type": "primary"}
                 if g(row, "PrimaryContactEmail", "PrimaryContactFullname") else None),
                ({"fullName": g(row, "SecondaryContactFullname"),
                  "email": g(row, "SecondaryContactEmail"),
                  "phone": g(row, "SecondaryContactPhone"), "type": "secondary"}
                 if g(row, "SecondaryContactEmail", "SecondaryContactFullname") else None),
            ] if p],
            "description_text": desc_text,
            "_source": "sam.gov public bulk extract (ContractOpportunitiesFullCSV)",
        }
        with open(os.path.join(RAW, f"{nid}.json"), "w") as f:
            json.dump(rec, f, ensure_ascii=False)
        written += 1
        if written % 10000 == 0:
            print(f"[sam-extract] wrote {written}/{n}", flush=True)

with open(os.path.join(DEST, "_count"), "w") as f:
    f.write(str(written))
print(f"[sam-extract] done. rows={n} written={written} with_full_text={with_text} -> {RAW}")
PY

if [ "$KEEP_CSV" != "1" ] && [ "${SAM_SKIP_DOWNLOAD:-0}" != "1" ]; then
    echo "[sam-extract] keeping CSV at $CSV (pass nothing to delete; --keep-csv is default-on for audit)"
fi
