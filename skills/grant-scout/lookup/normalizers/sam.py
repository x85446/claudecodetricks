"""sam.gov normalizer — SAM.gov v2 opportunity search records."""
import glob
import json
import os

from ._common import clean, iso

SOURCE = 'sam.gov'
DIR = 'sam.gov'
PRIORITY = 40


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        path_segs = (o.get('fullParentPathName') or '').split('.')
        # Prefer enriched full text (fetched description body + extracted attachment
        # text from enrich_sam.sh). Fall back to any inline description, then title.
        dt = clean(o.get('description_text') or '')
        if not dt:
            d0 = o.get('description')
            if isinstance(d0, str) and not d0.startswith('http'):
                dt = clean(d0)
        att_txt = ''
        for a in (o.get('attachments') or []):
            tx = (a.get('text') or '').strip()
            if tx:
                att_txt += f"\n\n[Attachment ({a.get('kind')}): {a.get('url')}]\n{tx}"
        topic_text = (dt + att_txt).strip()[:200000] or clean(o.get('title') or '')
        yield {
            'source': SOURCE,
            'source_id': o.get('noticeId'),
            'solicitation_number': o.get('solicitationNumber'),
            'title': clean(o.get('title')),
            'agency': path_segs[0] if path_segs else None,
            'subagency': o.get('fullParentPathName'),
            'topic_text': topic_text,
            'url': o.get('uiLink'),
            'posted_date': iso(o.get('postedDate')),
            'close_date': iso(o.get('responseDeadLine') or o.get('archiveDate')),
            'phase': o.get('type') or o.get('baseType'),
            'funding_ceiling': (o.get('award') or {}).get('amount') if isinstance(o.get('award'), dict) else None,
            'eligibility': clean(o.get('typeOfSetAsideDescription') or o.get('typeOfSetAside') or ''),
            'raw_path': fp,
        }
