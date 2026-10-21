#!/usr/bin/env python3
"""
enrich_darpa.py — upgrade darpa.mil RSS-stub records to full solicitation text.

The darpa.mil opportunities RSS feed gives one sentence per item. Each item links to a
program page (or event page) that carries the real program description and, for actual
solicitations, an "Opportunities" block naming the solicitation number(s) the proposal
is submitted under (DARPA-SN-26-129, HR001126S0010, ...). Those numbers are the join key
that lets merge.py collapse the DARPA record into its SAM.gov / DSIP twin.

For each raw record this writes back:
    _kind            'event' | 'solicitation' | 'rfi'
    full_text        extracted page body
    _opportunities   [{number, published, deadline, label}]
    _twin            {source, solicitation_number, title}  (resolved from local corpora)
    _fetch           {url, status, len, at}

Usage:
    enrich_darpa.py <corpus-dir> [--no-fetch]
"""
import json, os, re, sys, glob, html, time, datetime
import urllib.request, urllib.error

UA = ('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 '
      '(KHTML, like Gecko) Chrome/126.0 Safari/537.36')

# Solicitation-number shapes DARPA uses across SAM.gov / DSIP.
SOL_RE = re.compile(
    r'\b(HR\d{6}[A-Z]{1,2}\d{4}|DARPA-[A-Z]{2}-\d{2}-\d{2,3}|DPA\d{2}[A-Z]Z\d{2}-[A-Z]{2}\d{3})\b')

# RSS blurbs for non-solicitations end in "| See Event"; these words in the title are the
# other tell (a proposers day / pitch day / info session is an event, not something to bid).
EVENT_TITLE_RE = re.compile(
    r'proposers?\s*day|pitch\s*day|spark\s*tank|information\s*session|industry\s*day|webinar',
    re.I)


# A few RSS items link to a JavaScript-rendered or login-gated portal that yields no text.
# Redirect those to the darpa.mil page for the solicitation that actually governs them.
URL_OVERRIDE = {
    # "ERIS: <topic>" shopping notices point at the DARPAConnect marketplace (login-gated);
    # submissions are made under the standing ERIS open call, DARPA-PS-25-05.
    'https://www.darpaconnect.us/eris':
        'https://www.darpa.mil/work-with-us/opportunities/darpa-ps-25-50',
}

# Below this, an extraction is assumed to have hit a JS shell rather than real content.
THIN_TEXT = 200


def fetch(url, timeout=45):
    req = urllib.request.Request(url, headers={'User-Agent': UA, 'Accept': 'text/html,*/*'})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.getcode(), r.read().decode('utf-8', 'replace')
    except urllib.error.HTTPError as e:
        return e.code, ''
    except Exception as e:
        return 0, ''


def page_text(h):
    """HTML -> readable body text, minus the .mil chrome."""
    b = re.sub(r'(?is)<(script|style|noscript|svg)[^>]*>.*?</\1>', ' ', h)
    b = re.sub(r'(?is)<(header|footer|nav|form)[^>]*>.*?</\1>', ' ', b)
    b = re.sub(r'(?is)<br\s*/?>|</(p|div|li|h[1-6]|tr)>', '\n', b)
    t = re.sub(r'(?s)<[^>]+>', ' ', b)
    t = html.unescape(t)
    t = re.sub(r'[ \t\xa0]+', ' ', t)
    t = re.sub(r'\n[ \t]*', '\n', t)
    t = re.sub(r'\n{3,}', '\n\n', t).strip()
    # Drop the government banner + breadcrumb that prefixes every darpa.mil page.
    m = re.search(r'(?s)^.*?\bBreadcrumb\b\s*\n(?:\s*\S[^\n]*\n){0,4}?', t)
    if m and m.end() < len(t) - 200:
        t = t[m.end():].strip()
    t = re.sub(r'(?is)\s*(Skip to main content|Official websites use \.mil.*?official, secure websites\.)\s*',
               ' ', t).strip()
    return t


def parse_opportunities(text):
    """The program page's Opportunities block: solicitation number + published/deadline."""
    m = re.search(r'(?is)\bOpportunities\b(.{0,1200}?)(?:\n\s*(?:Office|Resources|Contact|Program manager|Related)\b|$)',
                  text)
    seg = m.group(1) if m else ''
    out, seen = [], set()
    for mm in SOL_RE.finditer(seg):
        num = mm.group(1)
        if num in seen:
            continue
        seen.add(num)
        tail = seg[mm.end():mm.end() + 160]
        pub = re.search(r'Published:\s*([A-Z][a-z]{2}\.?\s+\d{1,2},\s*\d{4})', tail)
        dl = re.search(r'Deadline:\s*([A-Z][a-z]{2}\.?\s+\d{1,2},\s*\d{4})', tail)
        label = re.search(r'\b(Proposers\s*Day|Solicitation|Special Notice|RFI)\b', tail, re.I)
        out.append({
            'number': num,
            'published': iso_date(pub.group(1)) if pub else None,
            'deadline': iso_date(dl.group(1)) if dl else None,
            'label': label.group(1) if label else None,
        })
    # Fall back to any solicitation number anywhere on the page.
    if not out:
        for mm in SOL_RE.finditer(text):
            if mm.group(1) not in seen:
                seen.add(mm.group(1))
                out.append({'number': mm.group(1), 'published': None, 'deadline': None, 'label': None})
    return out


def iso_date(s):
    s = s.replace('.', '').strip()
    for fmt in ('%b %d, %Y', '%B %d, %Y'):
        try:
            return datetime.datetime.strptime(s, fmt).strftime('%Y-%m-%d')
        except Exception:
            continue
    return None


def classify(rec, text):
    title = rec.get('title') or ''
    blurb = rec.get('topic_text') or ''
    url = rec.get('url') or ''
    if EVENT_TITLE_RE.search(title) or '/events/' in url or blurb.rstrip().endswith('| See Event'):
        return 'event'
    if re.match(r'\s*RFI\b', title, re.I) or 'Request for Information' in title:
        return 'rfi'
    return 'solicitation'


# ---- local twin resolution -------------------------------------------------

def load_norm(corpus, src):
    p = os.path.join(corpus, src, 'normalized.jsonl')
    recs = []
    if os.path.exists(p):
        for line in open(p):
            line = line.strip()
            if line:
                try:
                    recs.append(json.loads(line))
                except Exception:
                    pass
    return recs


TITLE_PREFIX_RE = re.compile(
    r'^\s*(SBIR XL|SBIR|STTR|RFI|RFP|Proposers?\s*Day|Information Session|Industry Day)\s*:\s*', re.I)
DARPA_SOL_RE = re.compile(r'^(DARPA-|HR0011|DPA\d{2})', re.I)
STOP = {'THE', 'AND', 'FOR', 'WITH', 'FROM', 'INTO', 'THAT', 'THIS', 'PROGRAM', 'TOPIC',
        'REQUEST', 'INFORMATION', 'NOTICE', 'ANNOUNCEMENT', 'SOLICITATION', 'DARPA'}


def _toks(s):
    return {w for w in re.findall(r'[A-Z0-9]{4,}', (s or '').upper()) if w not in STOP}


def _is_darpa(src, o):
    """A sam.gov / grants.gov candidate is only a plausible DARPA twin if it *is* DARPA."""
    if src in ('dsip', 'sbir.gov'):
        return True
    sn = o.get('solicitation_number') or ''
    sub = (o.get('subagency') or '').upper()
    return bool(DARPA_SOL_RE.match(sn)) or 'DEFENSE ADVANCED RESEARCH' in sub or 'DARPA' in sub


def _title_match(corpus_recs, title):
    """Strict title match. Loose substring matching pulled in unrelated notices (an Air
    Force carpet-replacement RFQ matched "SHINE"), so a length floor, a token-overlap
    floor, and an is-it-really-DARPA guard all apply."""
    core = TITLE_PREFIX_RE.sub('', title or '').strip()
    core = re.sub(r'\s*\([^)]*\)\s*$', '', core).strip()
    if len(core) < 10:
        return None
    cu = core.upper()
    ct = _toks(core)
    if not ct:
        return None
    best = None
    for src in ('dsip', 'sam.gov', 'sbir.gov', 'grants.gov.extract'):
        for o in corpus_recs.get(src, []):
            if not _is_darpa(src, o):
                continue
            t = (o.get('title') or '').upper()
            ot = _toks(t)
            if not ot:
                continue
            overlap = len(ct & ot) / len(ct)
            contained = cu in t or t.split(' - ')[0].strip() == cu
            if contained or overlap >= 0.7:
                score = (1 if contained else 0, overlap, len(o.get('topic_text') or ''))
                if best is None or score > best[0]:
                    best = (score, src, o)
    if not best:
        return None
    _, src, o = best
    return {'source': src, 'solicitation_number': o.get('solicitation_number'),
            'title': o.get('title'), 'match': 'title',
            'text_len': len(o.get('topic_text') or '')}


def find_twin(corpus_recs, numbers, title):
    """Prefer a strict title match — the RSS title names the actual effort, whereas a
    program/office page's Opportunities block also lists sibling and office-wide notices.
    Fall back to those advertised solicitation numbers."""
    tm = _title_match(corpus_recs, title)
    if tm:
        return tm
    by_num = {}
    for src in ('dsip', 'sbir.gov', 'sam.gov', 'grants.gov.extract'):
        for o in corpus_recs.get(src, []):
            sn = (o.get('solicitation_number') or '').strip().upper()
            if sn:
                by_num.setdefault(sn, (src, o))
    for n in numbers:
        hit = by_num.get(n.upper())
        if hit:
            src, o = hit
            return {'source': src, 'solicitation_number': o.get('solicitation_number'),
                    'title': o.get('title'), 'match': 'number',
                    'text_len': len(o.get('topic_text') or '')}
    return None


def main():
    corpus = sys.argv[1]
    do_fetch = '--no-fetch' not in sys.argv
    raw_dir = os.path.join(corpus, 'darpa.mil', 'raw')
    corpus_recs = {s: load_norm(corpus, s) for s in ('dsip', 'sam.gov', 'sbir.gov', 'grants.gov.extract')}
    print(f"[enrich] local corpora: " + ', '.join(f'{s}={len(v)}' for s, v in corpus_recs.items()))

    files = sorted(glob.glob(os.path.join(raw_dir, '*.json')))
    stats = {'event': 0, 'solicitation': 0, 'rfi': 0, 'fetched': 0, 'twin': 0, 'fulltext': 0}
    for fp in files:
        rec = json.load(open(fp))
        # Every field below is derived; drop the previous pass's values so a match that
        # no longer holds cannot survive as a stale twin.
        for k in ('_kind', '_twin', '_opportunities', 'solicitation_number'):
            rec.pop(k, None)
        url = rec.get('url') or ''
        fetch_url = URL_OVERRIDE.get(url, url)
        text = rec.get('full_text') or ''
        # Refetch when the cached extraction is too thin to be real content — that is the
        # signature of a JS shell, and the override table may now route around it.
        if do_fetch and fetch_url and len(text) < THIN_TEXT:
            code, h = fetch(fetch_url)
            stats['fetched'] += 1
            text = page_text(h) if code == 200 and h else text
            rec['_fetch'] = {'url': fetch_url, 'status': code, 'len': len(text),
                             'at': datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')}
            if fetch_url != url:
                rec['_fetch']['overrode'] = url
            time.sleep(0.6)
        if text:
            rec['full_text'] = text
            stats['fulltext'] += 1
        opps = parse_opportunities(text) if text else []
        if opps:
            rec['_opportunities'] = opps
        kind = classify(rec, text)
        rec['_kind'] = kind
        stats[kind] += 1
        numbers = [o['number'] for o in opps]
        twin = find_twin(corpus_recs, numbers, rec.get('title'))
        if twin:
            rec['_twin'] = twin
            stats['twin'] += 1
        # The number the record should dedupe on: prefer the twin's, else the first
        # solicitation number the page advertises.
        canon = (twin or {}).get('solicitation_number') or (numbers[0] if numbers else None)
        if canon:
            rec['solicitation_number'] = canon
        # Deadline from the Opportunities block (RSS never carries one).
        if not rec.get('deadline'):
            dls = [o['deadline'] for o in opps if o.get('deadline')]
            if dls:
                rec['deadline'] = max(dls)
        json.dump(rec, open(fp, 'w'), indent=2, ensure_ascii=False)
        print(f"  {os.path.basename(fp):<38} {kind:<12} txt={len(text):<6} "
              f"sol={canon or '-':<18} twin={(twin or {}).get('source', '-')}")
    print(f"[enrich] {stats}")


if __name__ == '__main__':
    main()
