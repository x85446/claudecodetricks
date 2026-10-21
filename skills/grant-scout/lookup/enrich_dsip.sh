#!/usr/bin/env bash
# enrich_dsip.sh — deepen DSIP topic records with the published Q&A feed and the
# extracted text of the component-instruction / BAA-preface PDFs, so scoring reads
# real content instead of the topic narrative alone.
#
# Usage:  enrich_dsip.sh <dsip-corpus-dir> [--refresh]
#
# PUBLIC API CONTRACT (verified live 2026-08-13; no auth, browser UA + Referer required):
#   Q&A:   GET https://www.dodsbirsttr.mil/topics/api/public/topics/{topicId}/questions
#          -> HTTP 200, a JSON ARRAY (not an envelope) of
#             {questionId, questionNo, question(HTML), questionSubmittedOn(epoch ms),
#              questionStatus, questionStatusDisplay, answers:[{answerId, answer}]}
#          -> answers[].answer is itself a JSON STRING whose "content" key holds the
#             answer HTML: '{"content":"<p>…</p>","length":86}'. Parsed here.
#          -> A topic with no published Q&A returns [] — an empty qa_text is a normal
#             outcome, not a failure.
#          Rejected during discovery (all HTTP 500): /topics/questions/{id}, /qa/{id},
#          /{id}/qa, /{id}/publishedQuestions, /{id}/details/questions.
#   PDF:   GET https://www.dodsbirsttr.mil/submissions/api/public/download?uploadId={id}
#          -> HTTP 200 application/pdf. (The literal path the Angular bundle builds,
#             /submissions/api/public/download/solicitationDocuments?uploadId=…, 400s;
#             the bare /public/download form is the one that serves bytes.)
#
# Attachment ids come from the raw record itself, no extra lookup:
#   baaInstructions[].uploadId  — component-specific proposal instructions
#   baaPrefaceUploadId          — DoW BAA/CSO preface
#   referenceDocuments[].uploadId — occasional topic reference attachment
#
# PDF DEDUPE: the 93 open topics point at only ~36 distinct instruction PDFs and ~10
# distinct prefaces (a whole component/solicitation shares one). PDFs are cached in
# <dir>/pdf/<uploadId>.pdf + .txt and reused across topics, so a full run is ~46
# downloads, not ~190, and a re-run is zero.
#
# Q&A is NOT cached: an IN_PROGRESS topic gains answers daily, so the feed is re-fetched
# every run (93 cheap requests). --refresh additionally re-downloads and re-extracts the
# PDFs, discarding the on-disk pdf cache entries it touches.
#
# Env:
#   DSIP_QA_SLEEP    = 0.35   polite delay between Q&A requests (s)
#   DSIP_PDF_SLEEP   = 0.5    polite delay between PDF downloads (s)
#   DSIP_MAX_PDF_MB  = 60     skip attachments larger than this
#   DSIP_MAX_DOCS    = 6      cap attachments extracted per topic
#
# Adds to each raw/<topicCode>.json:
#   qa            [{no, status, submitted_utc, question, answers:[…]}]   structured
#   qa_text       flattened "Q1 [status, date]: … / A: …" plain text
#   qa_count      number of published questions returned by the feed
#   instructions_text   concatenated plain text of the attachment PDFs
#   instructions_docs   [{uploadId, fileName, kind, chars}]
#   _enriched_dsip      UTC timestamp of the last successful enrichment
# Writes <dir>/_enrich.json with the run summary.
#
# Requires: curl, pdftotext (poppler).

set -u
DEST="${1:?usage: enrich_dsip.sh <dsip-corpus-dir> [--refresh]}"
REFRESH=0
[ "${2:-}" = "--refresh" ] && REFRESH=1
RAW="$DEST/raw"
[ -d "$RAW" ] || { echo "[dsip-enrich] no raw dir at $RAW"; exit 2; }
command -v pdftotext >/dev/null || { echo "[dsip-enrich] ERROR: pdftotext not on PATH (brew install poppler)"; exit 2; }

exec python3 - "$DEST" "$REFRESH" <<'PY'
import json, os, re, sys, html, glob, time, subprocess, datetime, urllib.parse

DEST    = sys.argv[1]
REFRESH = sys.argv[2] == "1"
RAW     = os.path.join(DEST, "raw")
PDFDIR  = os.path.join(DEST, "pdf")
os.makedirs(PDFDIR, exist_ok=True)

HOST      = "www.dodsbirsttr.mil"
QA_BASE   = f"https://{HOST}/topics/api/public/topics"
DL_BASE   = f"https://{HOST}/submissions/api/public/download"
UA        = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
             "(KHTML, like Gecko) Chrome/126.0 Safari/537.36")
REFER     = "https://www.dodsbirsttr.mil/topics-app/"
QA_SLEEP  = float(os.environ.get("DSIP_QA_SLEEP", "0.35"))
PDF_SLEEP = float(os.environ.get("DSIP_PDF_SLEEP", "0.5"))
MAX_MB    = float(os.environ.get("DSIP_MAX_PDF_MB", "60"))
MAX_DOCS  = int(os.environ.get("DSIP_MAX_DOCS", "6"))
TMP       = os.path.join(DEST, "_enrich.tmp")


def _resolve():
    """Return a curl --resolve token 'host:443:IP', or '' to use system DNS.

    The .mil nameservers (dns*.nipr.mil) intermittently SERVFAIL from some local
    resolvers — crawl_dsip.sh hits the exact same host and already guards against
    this; enrich_dsip.sh talks to the same host right after it in the same
    fetch_all.sh run and was missing this guard entirely, so a resolver hiccup that
    crawl_dsip.sh silently recovered from took down every single Q&A/PDF request
    here (observed live: qa_failed=103/103, all HTTP 0, in the same run crawl_dsip
    logged 'local DNS failed; pinning'). Probe with curl itself, not
    socket.getaddrinfo — curl and python can disagree."""
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
                    print(f"[dsip-enrich] local DNS failed; pinning {HOST} -> {line} (via {rs})")
                    return f"{HOST}:443:{line}"
        except Exception:
            continue
    print("[dsip-enrich] WARNING: could not pre-resolve host; trying system DNS anyway")
    return ""


RESOLVE = _resolve()


def clean(s):
    """HTML -> single-spaced plain text."""
    if not isinstance(s, str):
        return ""
    s = re.sub(r"<(br|/p|/div|/li|/tr)[^>]*>", " \n", s, flags=re.I)
    s = re.sub(r"<[^>]+>", " ", s)
    s = html.unescape(s)
    s = s.replace("\xa0", " ")
    s = re.sub(r"[ \t]+", " ", s)
    s = re.sub(r"\n\s*\n+", "\n", s)
    return s.strip()


def fetch(url, out, accept="application/json"):
    """GET url to file `out` with browser headers + backoff. Returns http code int."""
    code = "0"
    for attempt in range(1, 4):
        args = ["curl", "-sSL", "-A", UA, "-H", f"Accept: {accept}",
                "-H", f"Referer: {REFER}", "--max-time", "90",
                "-o", out, "-w", "%{http_code}"]
        if RESOLVE:
            args += ["--resolve", RESOLVE]
        try:
            cp = subprocess.run(args + [url], capture_output=True, text=True, timeout=120)
            code = re.sub(r"\D", "", (cp.stdout or "")[-3:]) or "0"
        except Exception:
            code = "0"
        if code == "200":
            return 200
        if code == "429" or code.startswith("5") or code == "0":
            back = 3 * attempt
            print(f"[dsip-enrich] HTTP {code} on {url}; backoff {back}s (attempt {attempt})",
                  file=sys.stderr)
            time.sleep(back)
            continue
        break                                   # 4xx other than 429: don't retry
    return int(code) if code.isdigit() else 0


def epoch_utc(v):
    if not isinstance(v, (int, float)):
        return None
    try:
        secs = v / 1000.0 if v > 1e11 else float(v)
        return datetime.datetime.utcfromtimestamp(secs).strftime("%Y-%m-%d")
    except Exception:
        return None


def answer_text(a):
    """answers[].answer is a JSON string carrying the real HTML under "content"."""
    raw = a.get("answer")
    if not isinstance(raw, str):
        return ""
    try:
        parsed = json.loads(raw)
        if isinstance(parsed, dict):
            return clean(parsed.get("content") or "")
    except Exception:
        pass
    return clean(raw)                            # plain HTML fallback


def fetch_qa(topic_id):
    """Return (qa_list, qa_text, http_code). Empty feed is a normal outcome."""
    code = fetch(f"{QA_BASE}/{urllib.parse.quote(str(topic_id))}/questions", TMP)
    if code != 200:
        return None, None, code
    try:
        data = json.load(open(TMP, encoding="utf-8", errors="replace"))
    except Exception:
        return None, None, -1
    if isinstance(data, dict):                   # tolerate an enveloped shape
        data = data.get("data") or data.get("questions") or []
    if not isinstance(data, list):
        return None, None, -1

    qa, lines = [], []
    for q in sorted(data, key=lambda x: x.get("questionNo") or 0):
        qtext = clean(q.get("question"))
        answers = [t for t in (answer_text(a) for a in (q.get("answers") or [])) if t]
        no = q.get("questionNo")
        status = q.get("questionStatusDisplay") or q.get("questionStatus") or ""
        when = epoch_utc(q.get("questionSubmittedOn"))
        qa.append({"no": no, "status": status, "submitted_utc": when,
                   "question": qtext, "answers": answers})
        head = ", ".join(x for x in (status, when) if x)
        lines.append(f"Q{no}" + (f" [{head}]" if head else "") + f": {qtext}")
        for a in answers:
            lines.append(f"A: {a}")
    return qa, "\n".join(lines), 200


def pdf_text(upload_id):
    """Download + extract one attachment, caching both the PDF and its text by
    uploadId (attachments are shared across every topic in a solicitation).
    Returns (text, from_cache, error_or_None)."""
    pdf = os.path.join(PDFDIR, f"{upload_id}.pdf")
    txt = os.path.join(PDFDIR, f"{upload_id}.txt")
    if os.path.exists(txt) and os.path.getsize(txt) > 0 and not REFRESH:
        return open(txt, encoding="utf-8", errors="replace").read(), True, None
    code = fetch(f"{DL_BASE}?uploadId={upload_id}", pdf, accept="application/pdf")
    if code != 200:
        return "", False, f"HTTP {code}"
    size_mb = os.path.getsize(pdf) / 1e6
    if size_mb > MAX_MB:
        os.remove(pdf)
        return "", False, f"skipped ({size_mb:.0f}MB > {MAX_MB}MB)"
    with open(pdf, "rb") as f:
        if f.read(5) != b"%PDF-":
            os.remove(pdf)
            return "", False, "not a PDF"
    try:
        subprocess.run(["pdftotext", "-q", "-enc", "UTF-8", pdf, txt],
                       capture_output=True, timeout=180)
    except Exception as e:
        return "", False, f"pdftotext failed: {e}"
    if not (os.path.exists(txt) and os.path.getsize(txt) > 0):
        return "", False, "pdftotext produced no text"
    body = open(txt, encoding="utf-8", errors="replace").read()
    body = re.sub(r"[ \t]+", " ", body)
    body = re.sub(r"\n\s*\n+", "\n", body).strip()
    with open(txt, "w", encoding="utf-8") as f:
        f.write(body)
    return body, False, None


def attachments_of(o):
    """[(uploadId, fileName, kind)] for the topic, de-duplicated, preface last."""
    out, seen = [], set()
    for b in (o.get("baaInstructions") or []):
        uid = b.get("uploadId")
        if uid and uid not in seen:
            seen.add(uid)
            out.append((uid, b.get("fileName") or f"{uid}.pdf", "component_instructions"))
    for r in (o.get("referenceDocuments") or []):
        uid = r.get("uploadId")
        if uid and uid not in seen:
            seen.add(uid)
            out.append((uid, clean(r.get("referenceTitle")) or f"{uid}.pdf", "reference"))
    uid = o.get("baaPrefaceUploadId")
    if uid and uid not in seen:
        seen.add(uid)
        out.append((uid, o.get("baaPrefaceUploadTitle") or f"{uid}.pdf", "baa_preface"))
    return out[:MAX_DOCS]


files = sorted(glob.glob(os.path.join(RAW, "*.json")))
print(f"[dsip-enrich] {len(files)} raw topics; refresh={REFRESH} "
      f"(Q&A always re-fetched; PDFs cached in {PDFDIR})")

stats = dict(topics=len(files), qa_ok=0, qa_nonempty=0, qa_entries=0, qa_failed=0,
             instr_nonempty=0, pdf_downloaded=0, pdf_cached=0, pdf_failed=0, unreadable=0)
pdf_errors, qa_errors = {}, {}
t0 = time.time()

for i, fp in enumerate(files, 1):
    try:
        o = json.load(open(fp, encoding="utf-8"))
    except Exception:
        stats["unreadable"] += 1
        continue
    if not isinstance(o, dict):
        stats["unreadable"] += 1
        continue
    code = o.get("topicCode") or os.path.basename(fp)[:-5]
    tid = o.get("topicId")

    # --- Q&A ---
    if tid:
        qa, qa_text, http = fetch_qa(tid)
        if qa is None:
            stats["qa_failed"] += 1
            qa_errors[code] = http
        else:
            stats["qa_ok"] += 1
            stats["qa_entries"] += len(qa)
            if qa:
                stats["qa_nonempty"] += 1
            o["qa"] = qa
            o["qa_text"] = qa_text
            o["qa_count"] = len(qa)
        time.sleep(QA_SLEEP)
    else:
        o.setdefault("qa", [])
        o.setdefault("qa_text", "")
        o.setdefault("qa_count", 0)

    # --- attachment PDFs ---
    chunks, docs = [], []
    for uid, fname, kind in attachments_of(o):
        body, cached, err = pdf_text(uid)
        if err:
            stats["pdf_failed"] += 1
            pdf_errors[f"{code}:{uid}"] = err
            continue
        stats["pdf_cached" if cached else "pdf_downloaded"] += 1
        if not cached:
            time.sleep(PDF_SLEEP)
        docs.append({"uploadId": uid, "fileName": fname, "kind": kind, "chars": len(body)})
        chunks.append(f"=== {kind}: {fname} ===\n{body}")
    o["instructions_docs"] = docs
    o["instructions_text"] = "\n\n".join(chunks)
    if o["instructions_text"]:
        stats["instr_nonempty"] += 1

    o["_enriched_dsip"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    tmp_out = fp + ".tmp"
    with open(tmp_out, "w", encoding="utf-8") as f:
        json.dump(o, f, ensure_ascii=False)
    os.replace(tmp_out, fp)                      # atomic: never leave a half-written record

    if i % 10 == 0 or i == len(files):
        print(f"[dsip-enrich] {i}/{len(files)} ({time.time()-t0:.0f}s) "
              f"qa_nonempty={stats['qa_nonempty']} entries={stats['qa_entries']} "
              f"instr={stats['instr_nonempty']} pdf_dl={stats['pdf_downloaded']} "
              f"pdf_cache={stats['pdf_cached']}", flush=True)

if os.path.exists(TMP):
    os.remove(TMP)

stats["pdf_errors"] = pdf_errors
stats["qa_errors"] = qa_errors
stats["enriched_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
stats["elapsed_s"] = round(time.time() - t0, 1)
json.dump(stats, open(os.path.join(DEST, "_enrich.json"), "w"), indent=2)
print(f"[dsip-enrich] done in {stats['elapsed_s']}s: "
      f"qa_ok={stats['qa_ok']} qa_nonempty={stats['qa_nonempty']} "
      f"qa_entries={stats['qa_entries']} qa_failed={stats['qa_failed']} "
      f"instructions={stats['instr_nonempty']}/{stats['topics']} "
      f"pdf_downloaded={stats['pdf_downloaded']} pdf_cached={stats['pdf_cached']} "
      f"pdf_failed={stats['pdf_failed']}")
PY
