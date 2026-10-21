"""grants.gov.extract normalizer — daily bulk Grants.gov DB extract, filtered to
open + Izuma-eligible. Rate-limit-free fallback/superset of the grants.gov
Search2 crawler; covers the civilian-SBIR gap left by the dead api.www.sbir.gov."""
import glob
import json
import os

from ._common import clean, mmddyyyy, ELIG_CODE

SOURCE = 'grants.gov.extract'
DIR = 'grants.gov.extract'
PRIORITY = 60


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        is_forecast = bool(o.get('_is_forecast'))
        elig_codes = o.get('_eligible_applicants') or []
        elig_labels = ', '.join(ELIG_CODE.get(c, c) for c in elig_codes)
        addl = clean(o.get('AdditionalInformationOnEligibility') or '')
        eligibility = ' | '.join(x for x in (elig_labels, addl) if x) or 'unknown'
        oid = o.get('OpportunityID')
        close = (mmddyyyy(o.get('CloseDate'))
                 or mmddyyyy(o.get('EstimatedSynopsisCloseDate'))
                 or mmddyyyy(o.get('ArchiveDate')))
        posted = (mmddyyyy(o.get('PostDate'))
                  or mmddyyyy(o.get('EstimatedSynopsisPostDate')))
        ceiling = o.get('AwardCeiling')
        try:
            ceiling = float(ceiling) if ceiling not in (None, '', '0') else None
        except Exception:
            ceiling = None
        # Prefer the enriched body + extracted attachment text mirrored in from the
        # Search2 corpus (enrich_grants_gov.sh); fall back to the extract's own
        # Description, which is full text already for the extract-only records.
        body = clean(o.get('synopsis_text') or '') or clean(o.get('Description') or '')
        att_txt = ''
        for a in (o.get('attachments') or []):
            tx = (a.get('text') or '').strip()
            if tx:
                att_txt += f"\n\n[Attachment ({a.get('kind')}): {a.get('name')}]\n{tx}"
        topic_text = (body + att_txt).strip()[:200000] or clean(o.get('OpportunityTitle') or '')
        yield {
            'source': SOURCE,
            'source_id': str(oid) if oid else None,
            'solicitation_number': o.get('OpportunityNumber') or (str(oid) if oid else None),
            'title': clean(o.get('OpportunityTitle')),
            'agency': o.get('AgencyCode'),
            'subagency': o.get('AgencyName') or o.get('AgencyCode'),
            'topic_text': topic_text,
            'url': f"https://www.grants.gov/search-results-detail/{oid}" if oid else 'https://www.grants.gov/',
            'posted_date': posted,
            'close_date': close,
            'phase': 'Forecast' if is_forecast else (o.get('FundingInstrumentType') or 'Grant/Assistance'),
            'funding_ceiling': ceiling,
            'eligibility': eligibility,
            'raw_path': fp,
        }
