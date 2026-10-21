"""challenge.gov normalizer — federal prize-competition listings.

The challenge.gov platform itself was sunset 2026-03-30 (verified live: the
domain now serves a static "content has moved" notice with zero listings and
no API of any kind). The successor for public listings is usa.gov's Innovation
section (https://www.usa.gov/find-active-challenge + per-challenge detail
pages), which crawl_challenge_gov.sh scrapes. This module normalizes those
raw records; the "challenge.gov" source identity is kept because that is the
registry's stable name for "federal prize competitions", even though the
live pull now happens against usa.gov.
"""
import glob
import json
import os
import re

from ._common import clean

SOURCE = 'challenge.gov'
DIR = 'challenge.gov'
PRIORITY = 80


def _parse_date(s):
    """usa.gov end/start dates look like '9/8/2026 9:30 AM ET',
    '09/25/2025 11:59 ET (Phase 1)', etc. Pull the first M/D/YYYY and convert
    to ISO YYYY-MM-DD; unparseable -> None."""
    if not isinstance(s, str):
        return None
    m = re.search(r'(\d{1,2})/(\d{1,2})/(\d{4})', s)
    if not m:
        return None
    mo, d, y = (int(x) for x in m.groups())
    try:
        return f"{y:04d}-{mo:02d}-{d:02d}"
    except Exception:
        return None


def _max_dollar(s):
    """Prizes text carries one or more $-amounts (cash + follow-on contract
    value etc.); funding_ceiling is the largest one found, as a float."""
    if not isinstance(s, str):
        return None
    amounts = re.findall(r'\$([\d,]+(?:\.\d+)?)', s)
    vals = []
    for a in amounts:
        try:
            vals.append(float(a.replace(',', '')))
        except Exception:
            continue
    return max(vals) if vals else None


def normalize(corpus_dir):
    raw_dir = os.path.join(corpus_dir, DIR, 'raw')
    if not os.path.isdir(raw_dir):
        return
    for fp in glob.glob(os.path.join(raw_dir, '*.json')):
        try:
            o = json.load(open(fp))
        except Exception:
            continue
        parts = [p for p in (o.get('tagline'), o.get('description')) if p]
        topic_text = clean(' '.join(parts))
        apply_url = o.get('apply_url')
        if apply_url:
            topic_text = (topic_text + f"\n\nApply at: {apply_url}").strip()
        slug = o.get('id')
        yield {
            'source': SOURCE,
            'source_id': slug,
            'solicitation_number': f"CHALLENGE-{slug}" if slug else None,
            'title': clean(o.get('title')),
            'agency': clean(o.get('sponsoring_agency')),
            'subagency': None,
            'topic_text': topic_text,
            'url': o.get('usagov_url'),
            'posted_date': None,
            'close_date': _parse_date(o.get('end_date')),
            'phase': 'Prize Competition',
            'funding_ceiling': _max_dollar(o.get('prizes')),
            'eligibility': clean(o.get('eligibility_note')) or 'unknown',
            'raw_path': fp,
        }
