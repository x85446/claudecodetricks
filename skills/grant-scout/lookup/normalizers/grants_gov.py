"""grants.gov normalizer — Search2 + fetchOpportunity detail records (civilian
agency assistance incl. SBIR/STTR)."""
import glob
import json
import os

from ._common import clean, iso, num

SOURCE = 'grants.gov'
DIR = 'grants.gov'
PRIORITY = 50


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            d = json.load(open(fp))
        except Exception:
            continue
        # fetchOpportunity returns {"data": {...}}
        data = d.get('data') if isinstance(d, dict) else None
        if not isinstance(data, dict):
            continue
        # A record carries EITHER a posted `synopsis` or a forecasted `forecast`;
        # 608 of 1718 are forecast-only, so reading synopsis alone blanks them out.
        syn = data.get('synopsis', {}) or {}
        fc = data.get('forecast', {}) or {}
        # Prefer enriched full text + extracted attachment text (enrich_grants_gov.sh).
        body = clean(d.get('synopsis_text') or '')
        if not body:
            body = clean(syn.get('synopsisDesc') or fc.get('forecastDesc') or '')
        att_txt = ''
        for a in (d.get('attachments') or []):
            tx = (a.get('text') or '').strip()
            if tx:
                att_txt += f"\n\n[Attachment ({a.get('kind')}): {a.get('name')}]\n{tx}"
        topic_text = (body + att_txt).strip()[:200000] or clean(data.get('opportunityTitle') or '')
        # opportunityCategory is {"category":"D","description":"Discretionary"} —
        # phase is a string field, so take the description.
        cat = data.get('opportunityCategory')
        if isinstance(cat, dict):
            cat = cat.get('description') or cat.get('category')
        yield {
            'source': SOURCE,
            'source_id': str(data.get('id')) if data.get('id') is not None else None,
            'solicitation_number': data.get('opportunityNumber'),
            'title': clean(data.get('opportunityTitle')),
            'agency': data.get('owningAgencyCode'),
            'subagency': data.get('owningAgencyName') or data.get('owningAgencyCode'),
            'topic_text': topic_text,
            'url': f"https://www.grants.gov/search-results-detail/{data.get('id')}" if data.get('id') else None,
            # The bare dates read 'Aug 07, 2026 12:00:00 AM' — the trailing time
            # defeats iso() and leaves every record undated. The parallel *Str
            # fields are '2026-08-07-00-00-00', whose ISO head iso() takes.
            'posted_date': iso(syn.get('postingDateStr') or fc.get('postingDateStr')
                               or syn.get('postingDate') or fc.get('postingDate')),
            'close_date': iso(syn.get('responseDateStr') or fc.get('estApplicationResponseDateStr')
                              or syn.get('responseDate') or fc.get('estApplicationResponseDate')),
            'phase': 'Forecast' if (fc and not syn) else cat,
            'funding_ceiling': num(syn.get('awardCeiling') or fc.get('awardCeiling')),
            'eligibility': clean(syn.get('applicantEligibilityDesc')
                                 or fc.get('applicantEligibilityDesc') or ''),
            'raw_path': fp,
        }
