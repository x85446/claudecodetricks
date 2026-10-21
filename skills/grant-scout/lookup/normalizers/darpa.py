"""darpa.mil normalizer — DARPA RSS opportunities enriched with program-page body text."""
import glob
import json
import os
import re

from ._common import clean, iso

SOURCE = 'darpa.mil'
DIR = 'darpa.mil'
PRIORITY = 30


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        # The RSS blurb is one sentence ending in "| See Topic/Program/Event"; enrich_darpa.py
        # attaches the program page's real body as full_text. Lead with the blurb (it is a
        # good summary) and follow with the body.
        blurb = clean(re.sub(r'\|\s*See\s+\w+\s*$', '', o.get('topic_text') or '')).strip()
        body = (o.get('full_text') or '').strip()
        if body and blurb and blurb[:60] not in body:
            topic_text = blurb + '\n\n' + body
        else:
            topic_text = body or blurb
        # enrich_darpa.py resolves the number the opportunity is actually submitted under
        # (SAM.gov / DSIP), which is what merge.py dedupes on. Without it the RSS id is used,
        # and the record stays standalone rather than collapsing into its twin.
        kind = o.get('_kind') or 'solicitation'
        sol = o.get('solicitation_number') or o.get('id')
        if re.match(r'^DPA\d{2}', str(sol or '')):
            phase, elig = 'SBIR/STTR', 'SBIR/STTR Small Business (DSIP submission)'
        elif kind == 'event':
            phase, elig = 'Event', 'Open attendance (registration required)'
        elif kind == 'rfi':
            phase, elig = 'RFI', 'Open to all responders'
        else:
            phase, elig = 'BAA/Program Announcement', 'Open (see solicitation)'
        rec = {
            'source': SOURCE,
            'source_id': o.get('id'),
            'solicitation_number': sol,
            'title': o.get('title'),
            'agency': 'DoD',
            'subagency': o.get('office') or 'DARPA',
            'topic_text': topic_text,
            'url': o.get('url'),
            'posted_date': iso(o.get('posted')),
            'close_date': iso(o.get('deadline')),
            'phase': phase,
            'funding_ceiling': None,
            'eligibility': elig,
            'raw_path': fp,
            '_kind': kind,
        }
        if o.get('_twin'):
            rec['_twin'] = o['_twin']
        if o.get('_opportunities'):
            rec['_opportunities'] = o['_opportunities']
        yield rec
