"""nspires normalizer — NASA NSPIRES solicitation listing (form-driven JSP
site scraped via its underlying DataTables JSON feed + per-solicitation detail
page; see crawl_nspires.sh for the discovery notes)."""
import glob
import json
import os

from ._common import clean

SOURCE = 'nspires'
DIR = 'nspires'
PRIORITY = 60


def _date_or_none(v):
    if not isinstance(v, str):
        return None
    v = v.strip()
    if not v or v == '--':
        return None
    return v[:10]


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        num = o.get('solicitation_number')
        sid = o.get('sId')
        close = _date_or_none(o.get('proposal_due')) or _date_or_none(o.get('noi_due'))
        posted = _date_or_none(o.get('release_date'))
        phase = o.get('type_text') or o.get('announcement_type') or 'NASA Solicitation'
        url = o.get('url') or (
            f"https://nspires.nasaprs.com/external/solicitations/summary!init.do?solId={sid}&path=open"
            if sid else 'https://nspires.nasaprs.com/external/solicitations/solicitations.do'
        )
        yield {
            'source': SOURCE,
            'source_id': str(sid or num or ''),
            'solicitation_number': num,
            'title': clean(o.get('title')),
            'agency': 'NASA',
            'subagency': o.get('directorate') or 'NASA',
            'topic_text': clean(o.get('topic_text')) or clean(o.get('title')) or '',
            'url': url,
            'posted_date': posted,
            'close_date': close,
            'phase': phase,
            'funding_ceiling': None,
            'eligibility': None,
            'raw_path': fp,
        }
