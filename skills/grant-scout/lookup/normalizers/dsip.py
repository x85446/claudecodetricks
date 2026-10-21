"""dsip normalizer — DoD SBIR/STTR Innovation Portal topics (highest Izuma fit;
authoritative DoD topic text)."""
import glob
import json
import os

from ._common import clean, epoch_or_iso

SOURCE = 'dsip'
DIR = 'dsip'
PRIORITY = 10


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    # enrich_dsip.sh stores the full component-instruction / BAA-preface PDF text on
    # every topic. That text is solicitation-wide boilerplate — one PDF is shared by
    # up to 12 topics — and it is ~250 KB per record, so it is NOT folded into
    # topic_text by default: grade_corpus.py scores topic_text with presence regexes,
    # and identical boilerplate on all 93 topics saturates the skillset/roadmap
    # signals uniformly, which is noise, not content. It ships as its own capped
    # field instead; the untruncated text stays in the raw record at raw_path.
    # Set DSIP_FOLD_INSTRUCTIONS=1 to fold it into topic_text anyway,
    # DSIP_INSTR_CHARS=0 to emit it untruncated.
    fold_instr = os.environ.get('DSIP_FOLD_INSTRUCTIONS', '0') == '1'
    instr_chars = int(os.environ.get('DSIP_INSTR_CHARS', '20000'))
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        # DSIP detail responses sometimes wrap the topic under a key.
        if isinstance(o, dict) and 'topic' in o and isinstance(o['topic'], dict):
            o = o['topic']
        code = o.get('topicCode') or o.get('finalTopicCode') or o.get('topicId') or o.get('id')
        # Assemble topic text from the descriptive fields DSIP exposes.
        parts = []
        for k in ('objective', 'description', 'phase1Description', 'phase2Description',
                  'phase3Description', 'focusAreas', 'technologyAreas', 'keywords', 'itar'):
            v = o.get(k)
            if isinstance(v, list):
                v = ' '.join(str(x) for x in v)
            if v:
                parts.append(clean(str(v)))
        ph = o.get('phaseHierarchy')
        if isinstance(ph, str) and ph:
            parts.append(clean(ph))
        topic_text = ' '.join(parts) or clean(o.get('topicTitle') or '')
        # Published Q&A is topic-specific and often the only place a requirement is
        # made concrete (TRL, interfaces, what the government will and won't accept),
        # so it always joins topic_text.
        qa_text = clean(o.get('qa_text') or '')
        if qa_text:
            topic_text = f"{topic_text}\n\nPublished Q&A:\n{qa_text}"
        instructions_text = o.get('instructions_text') or ''
        if instructions_text and fold_instr:
            topic_text = f"{topic_text}\n\nProposal instructions:\n{clean(instructions_text)}"
        topic_text = topic_text[:200000]
        comp = o.get('component') or o.get('topicComponent')
        close = (epoch_or_iso(o.get('topicEndDate'))
                 or epoch_or_iso(o.get('proposalWindowEndDate'))
                 or epoch_or_iso(o.get('closeDate'))
                 or epoch_or_iso(o.get('topicClosedDate')))
        posted = (epoch_or_iso(o.get('topicStartDate'))
                  or epoch_or_iso(o.get('topicPreReleaseStartDate'))
                  or epoch_or_iso(o.get('openDate')))
        yield {
            'source': SOURCE,
            'source_id': str(o.get('topicId') or code or ''),
            'solicitation_number': str(code or '') or None,
            'title': clean(o.get('topicTitle') or o.get('title')),
            'agency': 'DoD',
            'subagency': comp or 'DoD SBIR/STTR',
            'topic_text': topic_text,
            'url': f"https://www.dodsbirsttr.mil/topics-app/?topicCode={code}" if code else 'https://www.dodsbirsttr.mil/topics-app/',
            'posted_date': posted,
            'close_date': close,
            'phase': o.get('program') or 'SBIR/STTR',
            'funding_ceiling': None,
            'eligibility': 'SBIR/STTR Small Business (DSIP)',
            'qa_text': qa_text,
            'qa_count': o.get('qa_count') or 0,
            'instructions_text': (instructions_text if instr_chars <= 0
                                  else instructions_text[:instr_chars]),
            'instructions_chars': len(instructions_text),
            'instructions_docs': o.get('instructions_docs') or [],
            'raw_path': fp,
        }
