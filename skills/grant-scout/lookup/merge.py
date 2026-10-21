#!/usr/bin/env python3
"""
merge.py — merge all per-source normalized.jsonl files into <corpus>/_all.json with cross-source dedupe.

Dedupe rule:
  Two records are "the same opportunity" iff they share a non-empty solicitation_number
  (case-insensitive, whitespace-stripped). On collision, the source with the lower
  PRIORITY (declared on its normalizers/<name>.py module — see normalizers/__init__.py)
  wins identity; every source that reported the same solicitation is still recorded in
  `_cross_refs` so provenance is never lost.

Sources, their corpus directories, and their priority order are all read from the
normalizers/ registry — the same one normalize.py uses — so adding a source never
requires hand-editing PRIORITY or a src_dirs table here.
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import normalizers  # noqa: E402


def norm_key(s):
    if not isinstance(s, str):
        return None
    s = s.strip().upper()
    return s or None


def merge(a, b):
    """Combine record `b` into `a`. `a` is the higher-priority record per PRIORITY."""
    # Preserve cross_refs
    refs = a.setdefault('_cross_refs', [])
    refs.append({
        'source': b.get('source'),
        'source_id': b.get('source_id'),
        'url': b.get('url'),
        'raw_path': b.get('raw_path'),
    })
    # Fill missing fields from b
    for k, v in b.items():
        if k.startswith('_'):
            continue
        if not a.get(k) and v:
            a[k] = v
    # Priority decides which record's identity wins, but not which one has the better body.
    # A DARPA program page is often 3 KB while its SAM.gov twin carries the 100 KB BAA (and
    # just as often the reverse), so the longest topic_text wins regardless of source rank.
    bt = b.get('topic_text') or ''
    if len(bt) > len(a.get('topic_text') or ''):
        a['_topic_text_source'] = b.get('source')
        a['topic_text'] = bt
    return a


def main():
    if len(sys.argv) < 2:
        print("usage: merge.py <corpus-dir>", file=sys.stderr)
        sys.exit(2)
    corpus = sys.argv[1]

    ordered_mods = normalizers.load_all()          # {short: module}, PRIORITY order
    src_dirs = {mod.SOURCE: mod.DIR for mod in ordered_mods.values()}
    priority = [mod.SOURCE for mod in ordered_mods.values()]

    # Load every source's normalized.jsonl
    by_source = {s: [] for s in src_dirs}
    for src, sub in src_dirs.items():
        p = os.path.join(corpus, sub, 'normalized.jsonl')
        if not os.path.exists(p):
            continue
        for line in open(p):
            line = line.strip()
            if not line:
                continue
            try:
                by_source[src].append(json.loads(line))
            except Exception:
                continue

    # Iterate in priority order; the first record to claim a key wins, subsequent merges add cross_refs
    merged = {}
    leftovers = []
    for src in priority:
        for rec in by_source.get(src, []):
            key = norm_key(rec.get('solicitation_number')) or norm_key(rec.get('source_id'))
            if not key:
                leftovers.append(rec)
                continue
            if key in merged:
                merge(merged[key], rec)
            else:
                merged[key] = rec

    out = list(merged.values()) + leftovers
    out_path = os.path.join(corpus, '_all.json')
    json.dump(out, open(out_path, 'w'), indent=2, ensure_ascii=False)

    # Summary
    by_src_count = {}
    for rec in out:
        by_src_count[rec.get('source')] = by_src_count.get(rec.get('source'), 0) + 1
    dup_count = sum(len(r.get('_cross_refs', [])) for r in out)
    print(f"[merge] total unique records: {len(out)}")
    print(f"[merge] cross-source duplicates collapsed: {dup_count}")
    # Report every registered source, even ones with zero records this run, so a
    # source going quiet is visible in the breakdown rather than just disappearing.
    for s in priority:
        print(f"  {by_src_count.get(s, 0):>5}  {s}")
    for s, n in sorted(by_src_count.items(), key=lambda x: -x[1]):
        if s not in priority:
            print(f"  {n:>5}  {s}  (unregistered source)")
    print(f"[merge] wrote {out_path}")


if __name__ == '__main__':
    main()
