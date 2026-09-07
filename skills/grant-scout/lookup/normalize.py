#!/usr/bin/env python3
"""
normalize.py — read each source's raw/ records and emit a unified normalized.jsonl.

Usage:
    normalize.py <corpus-dir> [source...]

Unified schema (one JSON object per line):
    {
      "source": "sam.gov" | "darpa.mil" | "dsip" | "grants.gov",
      "source_id": "<native ID>",
      "solicitation_number": "<canonical>",
      "title": "...",
      "agency": "DoD" | "NSF" | ...,
      "subagency": "DARPA SBPO" | ...,
      "topic_text": "<abstract / synopsis body>",
      "url": "https://...",
      "posted_date": "YYYY-MM-DD",
      "close_date": "YYYY-MM-DD",
      "phase": "SBIR Phase I" | "BAA" | "Contract" | ...,
      "funding_ceiling": null | float,
      "eligibility": "<verbatim>",
      "raw_path": "<absolute path to raw file>"
    }
"""
import json, sys, os, re, html, glob


def clean(s):
    if not isinstance(s, str): return s
    s = re.sub(r'<[^>]+>', ' ', s)
    s = html.unescape(s)
    s = re.sub(r'\s+', ' ', s).strip()
    return s


def iso(d):
    if not d: return None
    if not isinstance(d, str): return None
    return d[:10]


def normalize_sam(corpus_dir):
    raw_dir = os.path.join(corpus_dir, 'sam.gov', 'raw')
    if not os.path.isdir(raw_dir): return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        path_segs = (o.get('fullParentPathName') or '').split('.')
        yield {
            'source': 'sam.gov',
            'source_id': o.get('noticeId'),
            'solicitation_number': o.get('solicitationNumber'),
            'title': clean(o.get('title')),
            'agency': path_segs[0] if path_segs else None,
            'subagency': o.get('fullParentPathName'),
            'topic_text': clean(o.get('description')) or '',
            'url': o.get('uiLink'),
            'posted_date': iso(o.get('postedDate')),
            'close_date': iso(o.get('responseDeadLine') or o.get('archiveDate')),
            'phase': o.get('type') or o.get('baseType'),
            'funding_ceiling': (o.get('award') or {}).get('amount') if isinstance(o.get('award'), dict) else None,
            'eligibility': clean(o.get('typeOfSetAsideDescription') or o.get('typeOfSetAside') or ''),
            'raw_path': fp,
        }


def normalize_darpa(corpus_dir):
    raw_dir = os.path.join(corpus_dir, 'darpa.mil', 'raw')
    if not os.path.isdir(raw_dir): return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        yield {
            'source': 'darpa.mil',
            'source_id': o.get('id'),
            'solicitation_number': o.get('id'),  # DARPA URL slug doubles as solicitation #
            'title': o.get('title'),
            'agency': 'DoD',
            'subagency': o.get('office') or 'DARPA',
            'topic_text': o.get('topic_text') or '',
            'url': o.get('url'),
            'posted_date': iso(o.get('posted')),
            'close_date': iso(o.get('deadline')),
            'phase': 'SBIR/BAA',
            'funding_ceiling': None,
            'eligibility': 'SBIR Small Business (DSIP submission)',
            'raw_path': fp,
        }


def normalize_sbir(corpus_dir):
    raw_dir = os.path.join(corpus_dir, 'sbir.gov', 'raw')
    if not os.path.isdir(raw_dir): return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        # SBIR.gov solicitation can carry multiple sub-topics; we keep the top-level
        # solicitation record here and stash topics as a list inside topic_text.
        topics = o.get('solicitation_topics') or []
        topic_lines = []
        for t in topics:
            tnum = t.get('topic_number') or ''
            ttitle = t.get('topic_title') or ''
            tdesc = (t.get('topic_description') or '')[:600]
            topic_lines.append(f"[{tnum}] {ttitle} — {tdesc}")
        topic_blob = '\n'.join(topic_lines)
        agency = (o.get('agency') or '').strip()
        branch = (o.get('branch') or '').strip()
        yield {
            'source': 'sbir.gov',
            'source_id': str(o.get('solicitation_number') or '').strip() or None,
            'solicitation_number': str(o.get('solicitation_number') or '').strip() or None,
            'title': clean(o.get('solicitation_title')),
            'agency': agency or 'DoD',
            'subagency': branch or agency,
            'topic_text': topic_blob or clean(o.get('solicitation_title') or ''),
            'url': o.get('solicitation_agency_url'),
            'posted_date': iso(o.get('release_date') or o.get('open_date')),
            'close_date': iso(o.get('close_date') or o.get('application_due_date')),
            'phase': o.get('phase') or o.get('program') or 'SBIR/STTR',
            'funding_ceiling': None,
            'eligibility': 'SBIR/STTR Small Business',
            'raw_path': fp,
        }


def normalize_grants_gov(corpus_dir):
    raw_dir = os.path.join(corpus_dir, 'grants.gov', 'raw')
    if not os.path.isdir(raw_dir): return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            d = json.load(open(fp))
        except Exception:
            continue
        # fetchOpportunity returns {"data": {...}}
        data = d.get('data') if isinstance(d, dict) else None
        if not isinstance(data, dict): continue
        syn = data.get('synopsis', {}) or {}
        yield {
            'source': 'grants.gov',
            'source_id': str(data.get('id')) if data.get('id') is not None else None,
            'solicitation_number': data.get('opportunityNumber'),
            'title': clean(data.get('opportunityTitle')),
            'agency': data.get('owningAgencyCode'),
            'subagency': data.get('owningAgencyName') or data.get('owningAgencyCode'),
            'topic_text': clean(syn.get('synopsisDesc') or ''),
            'url': f"https://www.grants.gov/search-results-detail/{data.get('id')}" if data.get('id') else None,
            'posted_date': iso(syn.get('postingDate')),
            'close_date': iso(syn.get('responseDate')),
            'phase': data.get('opportunityCategory'),
            'funding_ceiling': syn.get('awardCeiling'),
            'eligibility': clean(syn.get('applicantEligibilityDesc') or ''),
            'raw_path': fp,
        }


def main():
    if len(sys.argv) < 2:
        print("usage: normalize.py <corpus-dir> [source...]", file=sys.stderr)
        sys.exit(2)
    corpus = sys.argv[1]
    sources = sys.argv[2:] or ['sam', 'darpa', 'dsip', 'grants_gov']

    fn_for = {
        'sam': normalize_sam,
        'darpa': normalize_darpa,
        'sbir': normalize_sbir,
        'grants_gov': normalize_grants_gov,
    }
    src_dir = {
        'sam': 'sam.gov',
        'darpa': 'darpa.mil',
        'sbir': 'sbir.gov',
        'grants_gov': 'grants.gov',
    }
    for src in sources:
        if src not in fn_for:
            print(f"[normalize] unknown source: {src}")
            continue
        out_path = os.path.join(corpus, src_dir[src], 'normalized.jsonl')
        n = 0
        os.makedirs(os.path.dirname(out_path), exist_ok=True)
        with open(out_path, 'w') as out:
            for rec in fn_for[src](corpus):
                out.write(json.dumps(rec, ensure_ascii=False) + '\n')
                n += 1
        print(f"[normalize] {src}: {n} records → {out_path}")


if __name__ == '__main__':
    main()
