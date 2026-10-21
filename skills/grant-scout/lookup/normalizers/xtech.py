"""xtech normalizer — Army xTech prize-competition listing + detail HTML scrape."""
import datetime
import glob
import json
import os

from ._common import clean

SOURCE = 'xtech'
DIR = 'xtech'
PRIORITY = 70


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        # Parse the scraped deadline string into ISO where possible.
        close = None
        d = o.get('deadline')
        if isinstance(d, str) and d:
            for fmt in ('%b %d, %Y', '%B %d, %Y', '%b %d %Y', '%m/%d/%Y', '%m/%d/%y'):
                try:
                    close = datetime.datetime.strptime(d.replace('.', '').strip(), fmt).strftime('%Y-%m-%d')
                    break
                except Exception:
                    continue
        yield {
            'source': SOURCE,
            'source_id': o.get('id'),
            'solicitation_number': f"XTECH-{o.get('id')}" if o.get('id') else None,
            'title': clean(o.get('title')),
            'agency': 'DoD',
            'subagency': 'Army xTech',
            'topic_text': o.get('topic_text') or '',
            'url': o.get('url'),
            'posted_date': None,
            'close_date': close,
            'phase': 'Prize Competition',
            'funding_ceiling': None,
            'eligibility': o.get('status') or 'unknown',
            'raw_path': fp,
        }
