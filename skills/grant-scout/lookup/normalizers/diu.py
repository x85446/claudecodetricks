"""diu normalizer — Defense Innovation Unit Commercial Solutions Openings (CSOs)
and Prize Challenge Areas of Interest (AOIs).

Raw records come from crawl_diu.sh, which parses the site's Nuxt SSR
`window.__NUXT__` state payload (see nuxt_devalue.py) rather than scraping
rendered markup — each raw/<number>.json is one full `state.aois[]` entry,
already carrying the complete problem statement / desired-solution narrative
(u_area_of_interest), the full eligibility text, and — when present — FAQs.
Nothing here needs a second "detail" fetch; the SSR payload already is the
detail.
"""
import glob
import json
import os

from ._common import clean, iso

SOURCE = 'diu'
DIR = 'diu'
PRIORITY = 65


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue

        number = o.get('number') or o.get('sys_id')
        title = clean(o.get('u_aoi_title'))
        area = clean(o.get('u_area_of_interest'))
        elig = clean(o.get('u_eligibility_requirements'))
        faqs = clean(o.get('u_solicitation_faqs'))

        topic_parts = []
        if area:
            topic_parts.append(area)
        if elig:
            topic_parts.append('Eligibility:\n' + elig)
        if faqs:
            topic_parts.append('FAQs:\n' + faqs)
        topic_text = '\n\n'.join(topic_parts)

        url = (f'https://www.diu.mil/work-with-us/submit-solution/{number}'
               if number else 'https://www.diu.mil/work-with-us/open-solicitations')

        yield {
            'source': SOURCE,
            'source_id': number,
            'solicitation_number': number,
            'title': title,
            'agency': 'DoD',
            'subagency': o.get('u_intended_contracting_shop') or 'Defense Innovation Unit',
            'topic_text': topic_text,
            'url': url,
            'posted_date': None,
            'close_date': iso(o.get('u_aoi_close_date')),
            'phase': o.get('u_project_type') or 'Commercial Solutions Opening',
            'funding_ceiling': None,
            'eligibility': elig or 'unknown',
            'raw_path': fp,
        }
