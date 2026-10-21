#!/usr/bin/env bash
# enrich_sam.sh — deepen SAM.gov records with the FETCHED description body and
# extracted ATTACHMENT text (PDF / .docx / Q&A), so scoring reads real content
# instead of the title. SAM stores its description as an unfetched `noticedesc`
# URL and its documents as `resourceLinks` — this follows both.
#
# STATUS AS OF 2026-09-10: NOT PART OF THE AUTOMATED PIPELINE, AND CURRENTLY
# NON-FUNCTIONAL. This script hard-requires $SAM_API_KEY (see the check below)
# and calls the authenticated api.sam.gov v2 opportunities endpoints. That is
# the SAME key/API path that crawl_sam.sh (now deleted) used, and which
# sources.json documents as dead: SAM.gov rejects the current key with
# HTTP 401 / 900901 "Invalid Credentials", and key regeneration is itself
# rate-limited by SAM.gov. So even a manual, key-in-hand invocation of this
# script will fail today, not just an unattended one. sources.json's "sam"
# entry has NO "enrich" field, so fetch_all.sh never invokes this — it cannot
# silently break the registry-driven crawl.
# The replacement source, crawl_sam_extract.sh (the public bulk CSV extract,
# no key, no auth), already ships inline `description_text` for ~65% of
# records straight from the extract itself — see crawl_sam_extract.sh's
# header for the field-coverage comparison. This script is kept, unregistered,
# only in case a future valid $SAM_API_KEY makes the resourceLinks
# attachment-text path (PDFs/docx SAM doesn't put in the extract) worth
# re-enabling by hand; do not wire it into sources.json's "enrich" field
# without first confirming the key actually authenticates.
#
# Usage:  enrich_sam.sh <sam-corpus-dir>
# Env:
#   SAM_API_KEY (required)         auth for noticedesc + resource downloads
#   SAM_ENRICH_SCOPE = kwtitle     which open records to deep-fetch:
#       none               nothing by scope — only SAM_ENRICH_SOLNUMS forced ones
#       kwtitle  (default) open + eligibility-OK + roadmap keyword in title (~266)
#       eligible           open + eligibility-OK               (large — watch quota)
#       all                every open record                   (very large)
#   SAM_ENRICH_SOLNUMS = ""        comma-separated solicitationNumbers to ALWAYS
#                                  enrich (force), bypassing scope/eligibility/kw
#                                  filters — use to guarantee already-ranked jobs
#                                  get full docs regardless of title.
#   SAM_ENRICH_FORCE = 0           set 1 to re-enrich records already done
#   SAM_ENRICH_MAX = 0             stop after N records this run (0 = no cap) —
#                                  the quota-budget knob for batched widening
#   SAM_MAX_DOCS = 8               cap attachments fetched per opportunity
#   SAM_MAX_DOC_MB = 30            skip files larger than this
#   SAM_DELAY = 0.3               polite delay between requests (s)
#
# Quota safety: SAM returns HTTP 429 (OVER_RATE_LIMIT) with a 200-shaped body once
# the daily key quota is spent. This script checks the HTTP status on every call and
# ABORTS the run rather than stamping records `_enriched` with an error page as their
# description — so a later resume re-tries them instead of skipping them as done.
#
# Adds to each raw/<id>.json:  description_text, attachments:[{url,name,kind,chars,text}],
#                              _enriched (UTC), _enrich_scope
# Resumable: already-enriched records are skipped unless SAM_ENRICH_FORCE=1.

set -u
DEST="${1:?usage: enrich_sam.sh <sam-corpus-dir>}"
RAW="$DEST/raw"
[ -d "$RAW" ] || { echo "[sam-enrich] no raw dir at $RAW"; exit 2; }
[ -n "${SAM_API_KEY:-}" ] || { echo "[sam-enrich] ERROR: \$SAM_API_KEY not set"; exit 2; }

SCOPE="${SAM_ENRICH_SCOPE:-kwtitle}"
FORCE="${SAM_ENRICH_FORCE:-0}"
MAXDOCS="${SAM_MAX_DOCS:-8}"
MAXMB="${SAM_MAX_DOC_MB:-30}"
DELAY="${SAM_DELAY:-0.3}"
MAXREC="${SAM_ENRICH_MAX:-0}"

SAM_API_KEY="$SAM_API_KEY" SAM_ENRICH_SOLNUMS="${SAM_ENRICH_SOLNUMS:-}" \
python3 - "$RAW" "$SCOPE" "$FORCE" "$MAXDOCS" "$MAXMB" "$DELAY" "$MAXREC" <<'PY'
import sys, os, re, json, html, glob, subprocess, tempfile, datetime, zipfile

RAW, SCOPE, FORCE, MAXDOCS, MAXMB, DELAY, MAXREC = (
    sys.argv[1], sys.argv[2], sys.argv[3] == '1',
    int(sys.argv[4]), float(sys.argv[5]), float(sys.argv[6]), int(sys.argv[7]))
KEY = os.environ['SAM_API_KEY']
SOLNUMS = {s.strip().upper() for s in os.environ.get('SAM_ENRICH_SOLNUMS', '').split(',') if s.strip()}
import time

EXCL = re.compile(r'(SDVOSB|Service-Disabled|Women|WOSB|EDWOSB|8\(a\)|8a\b|HUBZone|Indian|Tribal)', re.I)
KW = re.compile(r'\b(cross[- ]?domain|CDS|post[- ]?quantum|PQC|cryptograph|crypto|encrypt|PKI|zero trust|'
    r'tactical|expeditionary|DDIL|disconnect|degraded|mesh|SD-?WAN|overlay|kubernetes|container|'
    r'orchestrat|edge comput|edge network|command and control|\bC2\b|C5ISR|JADC2|resilient|'
    r'network|comms|communication|IoT|firmware|FOTA|TPM|secure boot|root of trust|provision|'
    r'bare metal|software factory|devsecops|data guard|content disarm|sanitiz|diode|multi-?domain|'
    r'cyber|software|data exchange|interoperab|situational awareness|mission)\b', re.I)

today = datetime.date.today()

class QuotaExhausted(Exception):
    """SAM daily key quota spent — abort without stamping records as enriched."""

# SAM signals quota exhaustion as HTTP 429, and sometimes as a 200/403 whose body is
# an OVER_RATE_LIMIT error blob. Both must abort: writing that blob into
# description_text and stamping `_enriched` would permanently poison the record,
# because the resume path skips anything already stamped.
RATE_BODY = re.compile(r'OVER_RATE_LIMIT|exceeded your rate limit|API rate limit exceeded', re.I)

def _rate_check(status, body):
    if status == 429 or (body and RATE_BODY.search(body[:2000])):
        raise QuotaExhausted(f"HTTP {status} rate-limited by SAM")

def curl_text(url, tries=3):
    """Fetch text, separating the HTTP status from the body (status appended by -w)."""
    for a in range(1, tries + 1):
        r = subprocess.run(["curl", "-sSL", "-m", "60", "-w", "\n%{http_code}", url],
                           capture_output=True, text=True)
        out = r.stdout or ""
        status, body = 0, out
        if r.returncode == 0 and "\n" in out:
            body, _, tail = out.rpartition("\n")
            try:
                status = int(tail.strip())
            except ValueError:
                body, status = out, 0
        _rate_check(status, body)
        if r.returncode == 0 and status == 200 and body.strip():
            return body
        if status in (401, 403):          # bad/blocked key — retrying cannot help
            raise QuotaExhausted(f"HTTP {status} — key rejected by SAM")
        if status == 404:                 # genuinely absent noticedesc, not an error
            return None
        time.sleep(1.2 * a)
    return None

def curl_file(url, dest, tries=3):
    """Download to dest; returns (ok, status). Raises QuotaExhausted on rate limit."""
    for a in range(1, tries + 1):
        r = subprocess.run(["curl", "-sSL", "-m", "180", "-o", dest, "-w", "%{http_code}", url],
                           capture_output=True, text=True)
        try:
            status = int((r.stdout or "0").strip() or 0)
        except ValueError:
            status = 0
        # A rate-limited download lands the JSON error blob in dest, not a document.
        peek = ""
        if os.path.exists(dest) and 0 < os.path.getsize(dest) < 4096:
            try:
                peek = open(dest, encoding='utf-8', errors='replace').read()
            except Exception:
                peek = ""
        _rate_check(status, peek)
        if r.returncode == 0 and status == 200 and os.path.exists(dest) and os.path.getsize(dest) > 0:
            return True, status
        if status in (401, 403):
            raise QuotaExhausted(f"HTTP {status} — key rejected by SAM")
        if status == 404:
            return False, 404
        time.sleep(1.2 * a)
    return False, status

def strip_html(s):
    s = re.sub(r'<(script|style)\b.*?</\1>', ' ', s or '', flags=re.S | re.I)
    s = re.sub(r'<[^>]+>', ' ', s)
    s = html.unescape(s)
    return re.sub(r'[ \t]+', ' ', re.sub(r'\n\s*\n+', '\n\n', s)).strip()

def extract_pdf(path):
    r = subprocess.run(["pdftotext", "-layout", path, "-"], capture_output=True, text=True)
    return r.stdout if r.returncode == 0 else ""

def extract_docx(path):
    try:
        z = zipfile.ZipFile(path)
        xml = z.read('word/document.xml').decode('utf-8', 'replace')
        xml = re.sub(r'</w:p>', '\n', xml)
        xml = re.sub(r'<[^>]+>', '', xml)
        return html.unescape(xml)
    except Exception:
        return ""

def key_url(u):
    return u + ("&" if "?" in u else "?") + "api_key=" + KEY

# --- pick the scope set ---
files = glob.glob(os.path.join(RAW, '*.json'))
todo = []
for fp in files:
    try:
        o = json.load(open(fp))
    except Exception:
        continue
    # Forced records (already-ranked jobs) bypass every scope filter, including
    # open/eligibility/keyword and the already-enriched skip.
    forced = bool(SOLNUMS) and (o.get('solicitationNumber') or '').strip().upper() in SOLNUMS
    if not forced:
        # scope=none selects nothing on its own — the forced list is the whole run.
        # Without this the loop falls through every remaining filter (they are all
        # scope-gated) and silently enriches the entire open corpus.
        if SCOPE == 'none':
            continue
        rd = o.get('responseDeadLine')
        if rd:
            try:
                if datetime.date.fromisoformat(rd[:10]) < today:
                    continue
            except Exception:
                pass
        setaside = o.get('typeOfSetAsideDescription') or o.get('typeOfSetAside') or ''
        if SCOPE in ('kwtitle', 'eligible') and EXCL.search(setaside):
            continue
        if SCOPE == 'kwtitle' and not KW.search(o.get('title') or ''):
            continue
        if not FORCE and o.get('_enriched'):
            continue
    todo.append((fp, o))

# SAM_ENRICH_MAX is the quota budget: trim the selection up front so the run cannot
# spend more than the operator allowed. Forced records are enriched first so a capped
# (or quota-aborted) batch always covers the already-ranked jobs before anything else.
todo.sort(key=lambda t: (t[1].get('solicitationNumber') or '').strip().upper() not in SOLNUMS)
if MAXREC > 0 and len(todo) > MAXREC:
    print(f"[sam-enrich] budget cap SAM_ENRICH_MAX={MAXREC}: {len(todo)} selected → enriching first {MAXREC}")
    todo = todo[:MAXREC]

print(f"[sam-enrich] scope={SCOPE} force={FORCE} → {len(todo)} records to enrich")

# Dry run: report the size of the selection (and its attachment-fetch cost) and stop
# before spending a single API call. Use this to budget a scope against the daily quota.
if os.environ.get('SAM_ENRICH_DRYRUN') == '1':
    links = sum(len((o.get('resourceLinks') or [])[:MAXDOCS]) for _, o in todo)
    print(f"[sam-enrich] DRY RUN — would enrich {len(todo)} records, "
          f"fetch {links} attachments, ≈{len(todo) + links} API calls. No calls made.")
    for _, o in todo[:20]:
        print(f"[sam-enrich]   {o.get('solicitationNumber')} | {(o.get('title') or '')[:70]}")
    sys.exit(0)

done = docs_ok = docs_skip = 0
for i, (fp, o) in enumerate(todo, 1):
    nid = o.get('noticeId')
    # 1) description body
    desc_text = ""
    du = o.get('description')
    if isinstance(du, str) and du.startswith('http'):
        body = curl_text(key_url(du))
        if body:
            try:
                desc_text = strip_html(json.loads(body).get('description') or '')
            except Exception:
                desc_text = strip_html(body)
    elif isinstance(du, str):
        desc_text = strip_html(du)
    o['description_text'] = desc_text[:40000]

    # 2) attachments
    atts = []
    for u in (o.get('resourceLinks') or [])[:MAXDOCS]:
        with tempfile.NamedTemporaryFile(delete=False) as tf:
            tmp = tf.name
        try:
            # curl_file returns (ok, status) — a tuple is always truthy, so testing it
            # directly never detects a failure and lets a 404's error body fall through
            # to extraction as if it were the document.
            ok, status = curl_file(key_url(u), tmp)
            if not ok:
                # Record the failure with its HTTP status so the download audit can
                # categorize it (genuine 404 vs transient) instead of it vanishing.
                atts.append({'url': u, 'kind': f'failed-http-{status}', 'chars': 0, 'text': ''})
                docs_skip += 1
                continue
            sz = os.path.getsize(tmp)
            if sz > MAXMB * 1024 * 1024:
                atts.append({'url': u, 'kind': 'skipped-large', 'chars': 0, 'text': ''})
                docs_skip += 1
                continue
            magic = subprocess.run(["file", "-b", tmp], capture_output=True, text=True).stdout.lower()
            if 'pdf' in magic:
                kind, text = 'pdf', extract_pdf(tmp)
            elif 'microsoft word 2007' in magic or 'openxml' in magic or 'zip' in magic:
                kind, text = 'docx', extract_docx(tmp)
            elif 'html' in magic or 'ascii' in magic or 'unicode text' in magic or 'utf-8' in magic:
                kind, text = 'text', strip_html(open(tmp, encoding='utf-8', errors='replace').read())
            else:
                kind, text = 'unextracted', ''
            text = re.sub(r'[ \t]+', ' ', re.sub(r'\n\s*\n+', '\n\n', text or '')).strip()
            atts.append({'url': u, 'kind': kind, 'chars': len(text), 'text': text[:60000]})
            if text:
                docs_ok += 1
            else:
                docs_skip += 1
        finally:
            try:
                os.unlink(tmp)
            except Exception:
                pass
        time.sleep(DELAY)

    o['attachments'] = atts
    o['_enriched'] = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    o['_enrich_scope'] = SCOPE
    json.dump(o, open(fp, 'w'), ensure_ascii=False, indent=2)
    done += 1
    if i % 20 == 0 or i == len(todo):
        print(f"[sam-enrich] {i}/{len(todo)} records | docs ok={docs_ok} skipped={docs_skip}")
    time.sleep(DELAY)

print(f"[sam-enrich] done. enriched={done} records, doc-texts={docs_ok}, doc-skips={docs_skip}")
PY
echo "[sam-enrich] complete."
