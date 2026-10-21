"""sbir.gov normalizer — HTML-scraped SBIR/STTR topics (current schema; the JSON
API is disabled) with a legacy-solicitation-schema fallback."""
import glob
import json
import os

from ._common import clean, iso

SOURCE = 'sbir.gov'
DIR = 'sbir.gov'
PRIORITY = 20


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    # Civilian (NSF/HHS) topics carry the "View Official Solicitation" agency document text
    # as official_solicitation_text, written by enrich_sbir_civilian.sh from the shared cache
    # in sbir.gov/solicitations/ (fetch_sbir_solicitations.py).
    #
    # It is folded into topic_text because for most of these topics it is the ONLY real
    # content: sbir.gov publishes no per-topic description for NSF subtopics, so 237 of 268
    # NSF records have an empty description and would otherwise be scored on their title
    # alone. grade_corpus.py reads ('title', 'topic_text', 'agency', 'subagency', 'phase')
    # and never reads solicitation_text, and it stamps confidence='low' below 200 chars of
    # topic_text.
    #
    # The tradeoff is real: one NSF 26-510 document backs all 268 NSF topics, so its text is
    # identical across 268 of 337 records and saturates presence regexes uniformly. Weighting
    # a shared solicitation below the topic's own words belongs in grade_corpus.py, which can
    # tell the two apart; withholding the text here cannot.
    #
    # Set SBIR_FOLD_SOLICITATION=0 to keep it out of topic_text (it then ships as its own
    # capped solicitation_text field instead), SBIR_SOL_CHARS=0 to emit it untruncated.
    # The untruncated text always stays in the raw record at raw_path.
    fold_sol = os.environ.get('SBIR_FOLD_SOLICITATION', '1') == '1'
    sol_chars = int(os.environ.get('SBIR_SOL_CHARS', '20000'))
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        # New HTML-scraped topic schema (website crawl; the JSON API is disabled).
        # Each record is a single awardable topic, marked by 'topic_id'/'topic_number'.
        if 'topic_id' in o or 'topic_number' in o:
            num = (o.get('topic_number') or '').strip() \
                or (f"SBIR-{o.get('topic_id')}" if o.get('topic_id') else None)
            agency_raw = (o.get('agency') or '').strip()
            agency = {'DOW': 'DoD', 'DOD': 'DoD'}.get(agency_raw.upper(), agency_raw or 'SBIR/STTR')
            desc = clean(o.get('description') or '') or clean(o.get('title') or '')
            aurl = o.get('agency_url')
            topic_text = f"{desc}\n\nOfficial solicitation: {aurl}" if aurl else desc
            sol_text = o.get('official_solicitation_text') or ''
            if sol_text and fold_sol:
                topic_text = f"{topic_text}\n\nOfficial solicitation text:\n{clean(sol_text)}"
            topic_text = topic_text[:200000]
            yield {
                'source': SOURCE,
                'source_id': num,
                'solicitation_number': num,
                'title': clean(o.get('title')),
                'agency': agency,
                'subagency': (o.get('suborg') or '').strip() or agency,
                'topic_text': topic_text,
                'url': o.get('url') or o.get('agency_url'),
                'posted_date': iso(o.get('release_date') or o.get('open_date')),
                'close_date': iso(o.get('close_date')),
                'phase': o.get('program') or 'SBIR/STTR',
                'funding_ceiling': None,
                'eligibility': 'SBIR/STTR Small Business',
                # Carried beside topic_text only when it is not already inside it, so the
                # same text never ships twice in one record.
                'solicitation_text': ('' if fold_sol else
                                      (sol_text if sol_chars <= 0 else sol_text[:sol_chars])),
                'solicitation_chars': len(sol_text),
                'solicitation_url': o.get('official_solicitation_source_url') or aurl,
                'raw_path': fp,
            }
            continue
        # Legacy API solicitation schema (kept as a fallback if the API returns):
        # a solicitation can carry multiple sub-topics stashed into topic_text.
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
            'source': SOURCE,
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
