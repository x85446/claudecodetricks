#!/usr/bin/env python3
"""Fetch official solicitation documents for civilian (non-DoD) sbir.gov topics.

Each sbir.gov topic record carries `agency_url` — the "View Official Solicitation"
link. Civilian topics point at nsf.gov, sam.gov or simpler.grants.gov. Fetch each
unique document once, extract its text, and cache it under
`_corpus/sbir.gov/solicitations/`. `apply_sbir_solicitations.py` folds the cached
text back into the raw records.

Usage: fetch_sbir_solicitations.py [corpus_dir]
"""
import glob
import hashlib
import html
import io
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

UA = ('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 '
      '(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36')
MAX_BYTES = 30 * 1024 * 1024
DOD = {'DOW', 'DOD'}
SLEEP = 1.0


def log(msg):
    print(f"[sbir-sol] {msg}", flush=True)


def get(url, accept=None, binary=False, timeout=90):
    req = urllib.request.Request(url, headers={
        'User-Agent': UA,
        'Accept': accept or 'text/html,application/xhtml+xml,*/*',
        'Accept-Language': 'en-US,en;q=0.9',
    })
    with urllib.request.urlopen(req, timeout=timeout) as r:
        size = r.headers.get('Content-Length')
        if size and int(size) > MAX_BYTES:
            raise ValueError(f"too big ({size} bytes)")
        data = r.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES:
        raise ValueError('too big (streamed past 30MB)')
    return data if binary else data.decode('utf-8', 'replace')


CTRL = re.compile('[\x00-\x08\x0b\x0e-\x1f\x7f�]')


def scrub(s):
    """Drop control bytes and decode-failure markers left by binary content."""
    return CTRL.sub('', s.replace('\x0c', '\n'))


def looks_like_text(s, tag=''):
    """Reject blocks that are mostly binary noise rather than prose."""
    if len(s) < 40:
        return False
    ratio = sum(c.isalpha() or c.isspace() for c in s) / len(s)
    if ratio < 0.55:
        log(f"  dropping non-text extraction ({tag}): {ratio:.0%} letters")
        return False
    return True


def strip_html(s):
    s = re.sub(r'(?is)<(script|style|noscript|svg)\b.*?</\1>', ' ', s)
    s = re.sub(r'(?i)<br\s*/?>', '\n', s)
    s = re.sub(r'(?i)</(p|div|li|tr|h[1-6]|section)>', '\n', s)
    s = re.sub(r'(?s)<[^>]+>', ' ', s)
    s = html.unescape(s)
    s = scrub(s)
    s = s.replace(' ', ' ')
    s = re.sub(r'[ \t\r\f\v]+', ' ', s)
    s = re.sub(r' *\n *', '\n', s)
    s = re.sub(r'\n{3,}', '\n\n', s)
    return s.strip()


def pdf_text(data, tag):
    tmp = f"/tmp/_sbir_sol_{os.getpid()}.pdf"
    with open(tmp, 'wb') as fh:
        fh.write(data)
    try:
        out = subprocess.run(['pdftotext', '-layout', '-q', tmp, '-'],
                             capture_output=True, timeout=180)
        txt = out.stdout.decode('utf-8', 'replace')
    except Exception as e:
        log(f"  pdftotext failed on {tag}: {e}")
        txt = ''
    finally:
        os.path.exists(tmp) and os.remove(tmp)
    txt = re.sub(r'[ \t]+', ' ', scrub(txt))
    return re.sub(r'\n{3,}', '\n\n', txt).strip()


def ooxml_text(data, tag):
    """Pull authored text out of a .docx / .pptx / .xlsx container."""
    try:
        zf = zipfile.ZipFile(io.BytesIO(data))
    except Exception as e:
        log(f"  unreadable OOXML ({tag}): {e}")
        return ''
    names = [n for n in zf.namelist()
             if n.endswith('.xml')
             and (n.startswith('word/') or n.startswith('ppt/slides/')
                  or n == 'xl/sharedStrings.xml')
             and not any(k in n for k in ('theme', 'settings', 'styles',
                                          'fontTable', 'webSettings', 'Rels'))]
    chunks = []
    for n in sorted(names):
        try:
            xml = zf.read(n).decode('utf-8', 'replace')
        except Exception:
            continue
        # <w:t>, <a:t> and <t> all carry literal run text.
        runs = re.findall(r'(?s)<(?:\w+:)?t(?:\s[^>]*)?>(.*?)</(?:\w+:)?t>', xml)
        txt = ' '.join(html.unescape(r) for r in runs if r.strip())
        if txt.strip():
            chunks.append(scrub(txt))
    return re.sub(r'[ \t]+', ' ', '\n'.join(chunks)).strip()


def extract(data, name):
    """Route downloaded bytes to the right text extractor by magic bytes."""
    if data[:5] == b'%PDF-':
        return pdf_text(data, name)
    if data[:2] == b'PK':
        return ooxml_text(data, name)
    return strip_html(data.decode('utf-8', 'replace'))


# ---------------------------------------------------------------- nsf.gov

def fetch_nsf(url):
    body = get(url)
    i, j = body.find('<main'), body.rfind('</main>')
    core = body[i:j] if i != -1 and j > i else body
    core = re.sub(r'(?is)<(nav|footer|header)\b.*?</\1>', ' ', core)
    return [(url, strip_html(core))]


# ---------------------------------------------------------------- sam.gov

SAM_API = 'https://sam.gov/api/prod/opps'


def sam_opp(oid):
    return json.loads(get(f"{SAM_API}/v2/opportunities/{oid}",
                          accept='application/hal+json'))


def fetch_sam(url):
    m = re.search(r'/opp/([0-9a-f]{16,})/', url)
    if not m:
        raise ValueError(f"no opportunity id in {url}")
    oid = m.group(1)
    doc = sam_opp(oid)
    # The sbir.gov link often points at a superseded version; follow to current.
    latest = (doc.get('_links') or {}).get('opportunity:publish:latest', {}).get('href')
    if not doc.get('latest') and latest:
        newer = latest.rstrip('/').split('/')[-1]
        if newer != oid:
            log(f"  sam {oid[:8]} superseded -> {newer[:8]}")
            time.sleep(SLEEP)
            try:
                doc, oid = sam_opp(newer), newer
            except Exception as e:
                log(f"  latest fetch failed ({e}); keeping original version")

    d2 = doc.get('data2') or {}
    parts = []
    head = [f"Notice: {d2.get('title', '')}".strip(),
            f"Solicitation number: {d2.get('solicitationNumber', '')}".strip()]
    sol = d2.get('solicitation') or {}
    dl = (sol.get('deadlines') or {}).get('response')
    if dl:
        head.append(f"Response deadline: {dl}")
    parts.append('\n'.join(x for x in head if x.split(': ', 1)[-1]))
    for d in doc.get('description') or []:
        t = strip_html(d.get('body') or '')
        if t:
            parts.append(t)
    out = [(f"https://sam.gov/opp/{oid}/view", '\n\n'.join(parts).strip())]

    # Attachments carry the actual BAA / solicitation document.
    try:
        time.sleep(SLEEP)
        res = json.loads(get(f"{SAM_API}/v3/opportunities/{oid}/resources",
                             accept='application/hal+json'))
    except Exception as e:
        log(f"  sam {oid[:8]} resources failed: {e}")
        return out
    atts = []
    for grp in (res.get('_embedded') or {}).get('opportunityAttachmentList') or []:
        atts.extend(grp.get('attachments') or [])
    for a in atts:
        if a.get('accessStatus') != 'public' or a.get('deletedFlag') == '1':
            continue
        if a.get('type') != 'file' or a.get('fileExists') != '1':
            continue
        if (a.get('size') or 0) > MAX_BYTES:
            log(f"  skip oversized attachment {a.get('name')}")
            continue
        rid, name = a.get('resourceId'), a.get('name') or 'attachment'
        durl = f"{SAM_API}/v3/opportunities/resources/files/{rid}/download"
        try:
            time.sleep(SLEEP)
            data = get(durl, binary=True)
        except Exception as e:
            log(f"  attachment {name} failed: {e}")
            continue
        txt = extract(data, name)
        if looks_like_text(txt, name):
            out.append((f"{durl}  ({name})", f"[Attachment: {name}]\n{txt}"))
            log(f"  attachment {name}: {len(txt)} chars")
    return out


# ------------------------------------------------------ simpler.grants.gov

def fetch_simpler(url):
    body = get(url)
    i = body.find('<main')
    j = body.rfind('</main>')
    core = body[i:j] if i != -1 and j > i else body
    out = [(url, strip_html(core))]
    seen = set()
    for href in re.findall(r'https://files\.simpler\.grants\.gov/[^"\\\s<>]+', body):
        href = href.rstrip('\\"')
        if href in seen:
            continue
        seen.add(href)
        name = urllib.parse.unquote(href.rsplit('/', 1)[-1])
        try:
            time.sleep(SLEEP)
            data = get(href, binary=True)
        except Exception as e:
            log(f"  attachment {name} failed: {e}")
            continue
        txt = extract(data, name)
        if looks_like_text(txt, name):
            out.append((href, f"[Attachment: {name}]\n{txt}"))
            log(f"  attachment {name}: {len(txt)} chars")
    return out


HANDLERS = [
    ('nsf.gov', fetch_nsf),
    ('sam.gov', fetch_sam),
    ('simpler.grants.gov', fetch_simpler),
    ('grants.gov', fetch_simpler),
]


def handler_for(url):
    host = urllib.parse.urlparse(url).netloc.lower()
    for suffix, fn in HANDLERS:
        if host == suffix or host.endswith('.' + suffix):
            return fn
    return None


def slug_for(url):
    p = urllib.parse.urlparse(url)
    tail = re.sub(r'[^a-z0-9]+', '-', (p.path or '').lower()).strip('-')[-60:]
    return f"{p.netloc.replace('.', '_')}-{tail or 'root'}-{hashlib.sha1(url.encode()).hexdigest()[:8]}"


def civilian_urls(raw_dir):
    urls = {}
    for fp in sorted(glob.glob(os.path.join(raw_dir, '*.json'))):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        if (o.get('agency') or '').strip().upper() in DOD:
            continue
        u = (o.get('agency_url') or '').strip()
        if u.startswith('http'):
            urls.setdefault(u, []).append(os.path.basename(fp))
    return urls


def main():
    corpus = sys.argv[1] if len(sys.argv) > 1 else 'output/grants/_corpus'
    raw_dir = os.path.join(corpus, 'sbir.gov', 'raw')
    out_dir = os.path.join(corpus, 'sbir.gov', 'solicitations')
    os.makedirs(out_dir, exist_ok=True)

    urls = civilian_urls(raw_dir)
    log(f"{len(urls)} unique civilian solicitation URLs over "
        f"{sum(len(v) for v in urls.values())} topic records")

    index_path = os.path.join(out_dir, 'index.json')
    index = json.load(open(index_path)) if os.path.exists(index_path) else {}

    for n, (url, files) in enumerate(sorted(urls.items()), 1):
        slug = slug_for(url)
        txt_path = os.path.join(out_dir, slug + '.txt')
        if url in index and os.path.exists(txt_path) and os.path.getsize(txt_path) > 500:
            log(f"[{n}/{len(urls)}] cached {slug} ({index[url]['chars']} chars)")
            continue
        fn = handler_for(url)
        if fn is None:
            log(f"[{n}/{len(urls)}] no handler for {url}")
            continue
        log(f"[{n}/{len(urls)}] fetching {url} ({len(files)} topics)")
        try:
            docs = fn(url)
        except Exception as e:
            log(f"  FAILED: {type(e).__name__}: {e}")
            continue
        blocks = []
        for src, txt in docs:
            if looks_like_text(txt, src):
                blocks.append(f"--- source: {src} ---\n{txt}")
        if not blocks:
            log("  no text extracted")
            continue
        full = '\n\n'.join(blocks)
        with open(txt_path, 'w') as fh:
            fh.write(full)
        index[url] = {'slug': slug, 'chars': len(full), 'docs': len(blocks),
                      'topics': len(files),
                      'fetched': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())}
        with open(index_path, 'w') as fh:
            json.dump(index, fh, indent=1, sort_keys=True)
        log(f"  OK {slug}: {len(full)} chars from {len(blocks)} doc(s)")
        time.sleep(SLEEP)

    log(f"index: {len(index)} solicitations cached -> {index_path}")


if __name__ == '__main__':
    main()
