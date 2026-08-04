#!/usr/bin/env python3
"""
merge.py — merge all per-source normalized.jsonl files into <corpus>/_all.json with cross-source dedupe.

Dedupe rule:
  Two records are "the same opportunity" iff they share a non-empty solicitation_number
  (case-insensitive, whitespace-stripped). On collision, sources are preferred in this order:
    1. dsip          (authoritative SBIR topic text)
    2. darpa.mil     (authoritative DARPA topic text)
    3. sam.gov       (authoritative for contract opportunities)
    4. grants.gov    (assistance listings)
  All raw_path/source/source_id values are recorded in `_cross_refs` so we don't lose provenance.
"""
import json, sys, os, glob

PRIORITY = ['sbir.gov', 'darpa.mil', 'sam.gov', 'grants.gov']


def norm_key(s):
    if not isinstance(s, str): return None
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
        if k.startswith('_'): continue
        if not a.get(k) and v:
            a[k] = v
    return a


def main():
    if len(sys.argv) < 2:
        print("usage: merge.py <corpus-dir>", file=sys.stderr)
        sys.exit(2)
    corpus = sys.argv[1]

    src_dirs = {
        'sam.gov': 'sam.gov',
        'darpa.mil': 'darpa.mil',
        'sbir.gov': 'sbir.gov',
        'grants.gov': 'grants.gov',
    }
    # Load every source's normalized.jsonl
    by_source = {s: [] for s in src_dirs}
    for src, sub in src_dirs.items():
        p = os.path.join(corpus, sub, 'normalized.jsonl')
        if not os.path.exists(p): continue
        for line in open(p):
            line = line.strip()
            if not line: continue
            try:
                by_source[src].append(json.loads(line))
            except Exception:
                continue

    # Iterate in priority order; the first record to claim a key wins, subsequent merges add cross_refs
    merged = {}
    leftovers = []
    for src in PRIORITY:
        for rec in by_source.get(src, []):
            key = norm_key(rec.get('solicitation_number')) or norm_key(rec.get('source_id'))
            if not key:
                leftovers.append(rec)
                continue
            if key in merged:
                merge(merged[key], rec)
            else:
                merged[key] = rec
    # Any sources not in PRIORITY order (shouldn't happen)
    for src, recs in by_source.items():
        if src in PRIORITY: continue
        for rec in recs:
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
    for s, n in sorted(by_src_count.items(), key=lambda x: -x[1]):
        print(f"  {n:>5}  {s}")
    print(f"[merge] wrote {out_path}")


if __name__ == '__main__':
    main()
