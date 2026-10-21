#!/usr/bin/env bash
# enrich_grants_gov.sh — deepen Grants.gov records with the FULL synopsis body and
# extracted ATTACHMENT text (NOFO PDFs, Q&A, .docx/.xlsx), so scoring reads real
# content instead of the title.
#
# Usage:  enrich_grants_gov.sh <corpus-dir>
#
# Two things get fixed here:
#   1. synopsis_text — fetchOpportunity already returns the body, but it lives at
#      `data.synopsis.synopsisDesc` for posted opportunities and `data.forecast.
#      forecastDesc` for forecasted ones. Reading only the former leaves every
#      forecast record with empty topic_text. Both are folded into synopsis_text.
#   2. attachments — `data.synopsisAttachmentFolders[].synopsisAttachments[]`
#      lists documents by id only. The bytes come from
#      https://www.grants.gov/grantsws/rest/opportunity/att/download/<id>
#      (verified live 2026-08-13: content-length matches the record's fileLobSize).
#      No auth, no key, no quota.
#
# The .extract corpus (bulk XML) has no attachment list of its own, so records
# there are mirrored from the Search2 corpus by OpportunityID.
#
# Env:
#   GG_MAX_DOCS = 8       cap attachments fetched per opportunity
#   GG_MAX_DOC_MB = 30    skip files larger than this
#   GG_DELAY = 0.15       polite delay between requests (s, per worker)
#   GG_WORKERS = 8        concurrent downloads
#   GG_FORCE = 0          set 1 to re-enrich records already stamped
#   GG_MAX = 0            stop after N records this run (0 = no cap)
#
# Adds to grants.gov/raw/<num>.json:
#   synopsis_text, attachments:[{id,url,name,kind,mime,bytes,chars,text}],
#   _enriched (UTC), _attach_text_chars
# Adds the same fields to grants.gov.extract/raw/<num>.json (mirrored).
#
# Extracted document text is cached at <corpus>/grants.gov/_attach_text/<id>.txt,
# so a re-run costs no downloads and the mirror pass is free. Downloaded binaries
# are discarded after extraction — only text is kept.
#
# Resumable: already-stamped records are skipped unless GG_FORCE=1.

set -u
DEST="${1:?usage: enrich_grants_gov.sh <corpus-dir>}"
[ -d "$DEST/grants.gov/raw" ] || { echo "[gg-enrich] no raw dir at $DEST/grants.gov/raw"; exit 2; }

python3 - "$DEST" \
    "${GG_MAX_DOCS:-8}" "${GG_MAX_DOC_MB:-30}" "${GG_DELAY:-0.15}" \
    "${GG_WORKERS:-8}" "${GG_FORCE:-0}" "${GG_MAX:-0}" <<'PY'
import sys, os, re, json, html, glob, time, shutil, zipfile, tempfile, datetime, subprocess
import threading
from concurrent.futures import ThreadPoolExecutor

CORPUS  = sys.argv[1]
MAXDOCS = int(sys.argv[2])
MAXMB   = float(sys.argv[3])
DELAY   = float(sys.argv[4])
WORKERS = int(sys.argv[5])
FORCE   = sys.argv[6] == '1'
MAXREC  = int(sys.argv[7])

GG_RAW  = os.path.join(CORPUS, 'grants.gov', 'raw')
EX_RAW  = os.path.join(CORPUS, 'grants.gov.extract', 'raw')
TXT_DIR = os.path.join(CORPUS, 'grants.gov', '_attach_text')
os.makedirs(TXT_DIR, exist_ok=True)

DL = "https://www.grants.gov/grantsws/rest/opportunity/att/download/{}"
MAX_CHARS = 300000          # per document
_print_lock = threading.Lock()


def log(msg):
    with _print_lock:
        print(msg, flush=True)


# ---------- text extraction ----------

def strip_html(s):
    s = re.sub(r'<(script|style)\b.*?</\1>', ' ', s or '', flags=re.S | re.I)
    s = re.sub(r'<br\s*/?>|</p>|</div>|</tr>|</li>', '\n', s, flags=re.I)
    s = re.sub(r'<[^>]+>', ' ', s)
    s = html.unescape(s)
    s = re.sub(r'[ \t\xa0]+', ' ', s)
    return re.sub(r'\n\s*\n+', '\n\n', s).strip()


def _xml_text(blob):
    blob = re.sub(r'</(w:p|a:p|text:p|row)>', '\n', blob)
    blob = re.sub(r'</(si|t|c)>', ' ', blob)
    blob = re.sub(r'<[^>]+>', '', blob)
    return html.unescape(blob)


def extract_pdf(path):
    r = subprocess.run(["pdftotext", "-layout", "-q", path, "-"],
                       capture_output=True, text=True, errors='replace')
    out = r.stdout or ""
    if out.strip():
        return out
    # pdftotext returns nothing for scanned/image-only PDFs; pdfminer sometimes
    # still finds an embedded text layer. No OCR — a scan legitimately has no text.
    try:
        from pdfminer.high_level import extract_text
        return extract_text(path) or ""
    except Exception:
        return ""


def _ooxml(path, members):
    try:
        z = zipfile.ZipFile(path)
    except Exception:
        return ""
    names = z.namelist()
    parts = []
    for pat in members:
        for n in sorted(names):
            if re.fullmatch(pat, n):
                try:
                    parts.append(_xml_text(z.read(n).decode('utf-8', 'replace')))
                except Exception:
                    pass
    return "\n".join(parts)


def extract_docx(path):
    return _ooxml(path, [r'word/document\.xml', r'word/footnotes\.xml'])


def extract_xlsx(path):
    return _ooxml(path, [r'xl/sharedStrings\.xml', r'xl/worksheets/sheet\d+\.xml'])


def extract_pptx(path):
    return _ooxml(path, [r'ppt/slides/slide\d+\.xml', r'ppt/notesSlides/notesSlide\d+\.xml'])


def extract_legacy(path, ext):
    """.doc / .rtf via macOS textutil."""
    if not shutil.which('textutil'):
        return ""
    r = subprocess.run(["textutil", "-convert", "txt", "-stdout", path],
                       capture_output=True, text=True, errors='replace')
    return r.stdout if r.returncode == 0 else ""


def sniff(path, declared_ext, mime):
    """Trust the bytes over the declared type — 84 attachments claim
    application/octet-stream and a handful of .pdf names are really zips."""
    try:
        with open(path, 'rb') as f:
            head = f.read(8)
    except Exception:
        return declared_ext
    if head.startswith(b'%PDF'):
        return 'pdf'
    if head.startswith(b'PK\x03\x04'):
        # OOXML and plain zip share the PK magic — the member list disambiguates.
        try:
            names = zipfile.ZipFile(path).namelist()
        except Exception:
            return 'zip'
        if any(n.startswith('word/') for n in names):
            return 'docx'
        if any(n.startswith('xl/') for n in names):
            return 'xlsx'
        if any(n.startswith('ppt/') for n in names):
            return 'pptx'
        return 'zip'
    if head.startswith(b'{\\rtf'):
        return 'rtf'
    if head.startswith(b'\xd0\xcf\x11\xe0'):
        return 'doc'
    if head[:3] == b'\xff\xd8\xff' or head[:4] == b'\x89PNG':
        return 'image'
    if 'html' in (mime or '').lower() or declared_ext in ('html', 'htm'):
        return 'html'
    return declared_ext


def extract_any(path, ext, mime, depth=0):
    kind = sniff(path, ext, mime)
    if kind == 'pdf':
        return extract_pdf(path), kind
    if kind == 'docx':
        return extract_docx(path), kind
    if kind == 'xlsx':
        return extract_xlsx(path), kind
    if kind == 'pptx':
        return extract_pptx(path), kind
    if kind in ('doc', 'rtf'):
        return extract_legacy(path, kind), kind
    if kind in ('html', 'htm'):
        try:
            return strip_html(open(path, encoding='utf-8', errors='replace').read()), 'html'
        except Exception:
            return "", 'html'
    if kind == 'image':
        return "", 'image'          # no OCR
    if kind == 'zip' and depth < 1:
        # A NOFO delivered as a zip of PDFs — extract the members, cap at 8.
        out = []
        try:
            with tempfile.TemporaryDirectory() as td, zipfile.ZipFile(path) as z:
                for n in [m for m in z.namelist() if not m.endswith('/')][:8]:
                    ie = n.rsplit('.', 1)[-1].lower() if '.' in n else ''
                    if ie in ('jpg', 'jpeg', 'png', 'gif'):
                        continue
                    try:
                        dst = os.path.join(td, os.path.basename(n) or 'x')
                        with z.open(n) as src, open(dst, 'wb') as f:
                            shutil.copyfileobj(src, f)
                    except Exception:
                        continue
                    t, _ = extract_any(dst, ie, '', depth + 1)
                    if t and t.strip():
                        out.append(f"--- {n} ---\n{t.strip()}")
        except Exception:
            pass
        return "\n\n".join(out), 'zip'
    # Unknown: keep it only if it reads as text.
    try:
        blob = open(path, 'rb').read(MAX_CHARS)
        if b'\x00' in blob[:2048]:
            return "", kind or 'binary'
        return blob.decode('utf-8', 'replace'), kind or 'text'
    except Exception:
        return "", kind or 'binary'


def tidy(t):
    if not t:
        return ""
    t = t.replace('\x00', ' ').replace('\f', '\n')
    t = re.sub(r'[ \t\xa0]+', ' ', t)
    t = re.sub(r'\n\s*\n+', '\n\n', t)
    return t.strip()[:MAX_CHARS]


# ---------- download ----------

def curl_file(url, dest, tries=3):
    for a in range(1, tries + 1):
        r = subprocess.run(["curl", "-sSL", "-m", "240", "-o", dest, "-w", "%{http_code}",
                            "-A", "grant-scout/1.0", url], capture_output=True, text=True)
        try:
            status = int((r.stdout or "0").strip() or 0)
        except ValueError:
            status = 0
        if r.returncode == 0 and status == 200 and os.path.exists(dest) and os.path.getsize(dest) > 0:
            return True, status
        if status == 404:
            return False, 404
        time.sleep(1.5 * a)
    return False, status


def doc_text(doc):
    """Extracted text for one attachment, via the on-disk cache. Returns
    (text, kind, note). Cache hit costs no network."""
    aid = doc.get('id')
    if aid is None:
        return "", "", "no-id"
    cache = os.path.join(TXT_DIR, f"{aid}.txt")
    meta  = os.path.join(TXT_DIR, f"{aid}.meta")
    if os.path.exists(cache) and os.path.exists(meta):
        try:
            m = json.load(open(meta))
            return open(cache, encoding='utf-8', errors='replace').read(), m.get('kind', ''), m.get('note', 'cached')
        except Exception:
            pass

    size = doc.get('fileLobSize') or 0
    if size and size > MAXMB * 1024 * 1024:
        note = f"skipped: {size/1e6:.1f}MB > {MAXMB}MB"
        json.dump({"kind": "", "note": note}, open(meta, 'w'))
        open(cache, 'w').write("")
        return "", "", note

    name = doc.get('fileName') or str(aid)
    ext  = name.rsplit('.', 1)[-1].lower() if '.' in name else ''
    with tempfile.TemporaryDirectory() as td:
        tmp = os.path.join(td, 'dl')
        ok, status = curl_file(DL.format(aid), tmp)
        time.sleep(DELAY)
        if not ok:
            return "", "", f"download-failed:{status}"      # not cached — retry next run
        try:
            text, kind = extract_any(tmp, ext, doc.get('mimeType') or '')
        except Exception as e:
            text, kind = "", f"error:{type(e).__name__}"
    text = tidy(text)
    note = "ok" if text else "empty"
    try:
        open(cache, 'w', encoding='utf-8').write(text)
        json.dump({"kind": kind, "note": note, "name": name, "bytes": size}, open(meta, 'w'))
    except Exception:
        pass
    return text, kind, note


# ---------- record pass ----------

def synopsis_text_of(data):
    """Full body. Posted opportunities carry `synopsis.synopsisDesc`; forecasted
    ones carry `forecast.forecastDesc` — 608 records in the corpus are
    forecast-only and normalize to nothing if only the former is read."""
    syn = data.get('synopsis') or {}
    fc  = data.get('forecast') or {}
    parts = []
    for label, blob in (("", syn.get('synopsisDesc')),
                        ("[Forecast]", fc.get('forecastDesc'))):
        t = strip_html(blob or '')
        if t:
            parts.append(f"{label}\n{t}".strip() if label else t)
    for src in (syn, fc):
        for k, tag in (('applicantEligibilityDesc', 'Eligibility'),
                       ('fundingDescLinkUrl', 'Full announcement URL')):
            v = strip_html(str(src.get(k) or ''))
            if v and not any(v in p for p in parts):
                parts.append(f"[{tag}] {v}")
    return "\n\n".join(parts).strip()


def attachments_of(data):
    """Flatten synopsisAttachmentFolders + synopsisDocumentURLs into one list,
    newest folders first, capped at MAXDOCS."""
    out = []
    for fo in (data.get('synopsisAttachmentFolders') or []):
        for doc in (fo.get('synopsisAttachments') or []):
            out.append({"doc": doc, "folder": fo.get('folderType') or fo.get('folderName')})
    out = out[:MAXDOCS]
    urls = []
    for u in (data.get('synopsisDocumentURLs') or []):
        if isinstance(u, dict) and u.get('docUrl'):
            urls.append(u)
    return out, urls


stats = {"recs": 0, "docs_ok": 0, "docs_empty": 0, "docs_fail": 0, "docs_skip": 0,
         "syn": 0, "with_att_text": 0}
stats_lock = threading.Lock()


def process(fp):
    try:
        d = json.load(open(fp))
    except Exception:
        return None
    data = d.get('data')
    if not isinstance(data, dict):
        return None

    d['synopsis_text'] = synopsis_text_of(data)[:200000]

    docs, urls = attachments_of(data)
    atts, total_chars = [], 0
    for item in docs:
        doc = item['doc']
        text, kind, note = doc_text(doc)
        size = doc.get('fileLobSize') or 0
        atts.append({
            "id": doc.get('id'),
            "url": DL.format(doc.get('id')),
            "name": doc.get('fileName'),
            "description": doc.get('fileDescription'),
            "folder": item['folder'],
            "kind": kind,
            "mime": doc.get('mimeType'),
            "bytes": size,
            "chars": len(text),
            "note": note,
            "text": text,
        })
        total_chars += len(text)
        with stats_lock:
            if note.startswith('skipped'):
                stats['docs_skip'] += 1
            elif note.startswith('download-failed') or note.startswith('error'):
                stats['docs_fail'] += 1
            elif text:
                stats['docs_ok'] += 1
            else:
                stats['docs_empty'] += 1

    for u in urls:
        atts.append({
            "id": u.get('id'), "url": u.get('docUrl'),
            "name": u.get('description'), "description": u.get('description'),
            "folder": "synopsisDocumentURL", "kind": "link", "mime": None,
            "bytes": 0, "chars": 0, "note": "external-link", "text": "",
        })

    d['attachments'] = atts
    d['_attach_text_chars'] = total_chars
    d['_enriched'] = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')

    tmp = fp + '.tmp'
    with open(tmp, 'w', encoding='utf-8') as f:
        json.dump(d, f, ensure_ascii=False)
    os.replace(tmp, fp)

    with stats_lock:
        stats['recs'] += 1
        if d['synopsis_text']:
            stats['syn'] += 1
        if total_chars:
            stats['with_att_text'] += 1
        n = stats['recs']
    return n


files = sorted(glob.glob(os.path.join(GG_RAW, '*.json')))
todo = []
for fp in files:
    if not FORCE:
        try:
            if json.load(open(fp)).get('_enriched'):
                continue
        except Exception:
            pass
    todo.append(fp)
if MAXREC:
    todo = todo[:MAXREC]

log(f"[gg-enrich] {len(todo)} of {len(files)} grants.gov records to enrich "
    f"(workers={WORKERS} maxdocs={MAXDOCS} maxmb={MAXMB})")

t0 = time.time()
last = [t0]


def worker(fp):
    n = process(fp)
    if n and (n % 25 == 0 or time.time() - last[0] > 40):
        last[0] = time.time()
        el = time.time() - t0
        rate = n / el if el else 0
        log(f"[gg-enrich] {n}/{len(todo)} recs  {el:.0f}s  {rate:.1f}/s  "
            f"docs ok={stats['docs_ok']} empty={stats['docs_empty']} "
            f"fail={stats['docs_fail']} skip={stats['docs_skip']}")


with ThreadPoolExecutor(max_workers=WORKERS) as ex:
    list(ex.map(worker, todo))

log(f"[gg-enrich] grants.gov done: {stats['recs']} records, synopsis_text on {stats['syn']}, "
    f"attachment text on {stats['with_att_text']}; docs ok={stats['docs_ok']} "
    f"empty={stats['docs_empty']} fail={stats['docs_fail']} skip={stats['docs_skip']} "
    f"in {time.time()-t0:.0f}s")

# ---------- mirror into the bulk-extract corpus ----------
# The XML extract has no attachment list, so it inherits the Search2 corpus's
# by OpportunityID. Text is shared, not copied twice — same strings, one source.
if os.path.isdir(EX_RAW):
    by_id = {}
    for fp in glob.glob(os.path.join(GG_RAW, '*.json')):
        try:
            d = json.load(open(fp))
        except Exception:
            continue
        oid = str(((d.get('data') or {}).get('id')) or '')
        if oid:
            by_id[oid] = (d.get('synopsis_text') or '', d.get('attachments') or [])

    n = mirrored = 0
    for fp in sorted(glob.glob(os.path.join(EX_RAW, '*.json'))):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        oid = str(o.get('OpportunityID') or '')
        syn, atts = by_id.get(oid, ('', []))
        # The extract's own Description is already full text; keep it as the
        # fallback body so the 16 extract-only records are not left blank.
        o['synopsis_text'] = syn or strip_html(o.get('Description') or '')
        o['attachments'] = atts
        o['_attach_text_chars'] = sum(a.get('chars') or 0 for a in atts)
        o['_enriched'] = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
        tmp = fp + '.tmp'
        with open(tmp, 'w', encoding='utf-8') as f:
            json.dump(o, f, ensure_ascii=False)
        os.replace(tmp, fp)
        n += 1
        if atts:
            mirrored += 1
    log(f"[gg-enrich] grants.gov.extract: {n} records updated, {mirrored} carry attachments")
PY
