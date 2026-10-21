#!/usr/bin/env python3
"""
health_report.py — builds "the report" (see project CLAUDE.md): one row per
registered grant source with its last EFFECTIVE pull, up/down status, records
on that last pull, and notes.

Ground truth for freshness is per-source raw-file modification times under
<corpus>/<dir>/raw/*.json — NEVER a last_seen/last_crawl_utc column in
_metadata.json. Those columns can be (and have been) bumped by a cache replay —
re-normalizing already-cached raw files with no new fetch at all — which is
exactly how SAM.gov once looked "current" while quietly serving data that was
weeks old. Re-normalizing never touches raw/, so its mtimes only move when a
crawler actually wrote or refreshed a real record.

Status is DOWN when:
  - no crawler is registered for the source yet, or
  - the most recent recorded real crawl attempt for the source (fetch_all.sh's
    own per-source last_run_status in _metadata.json) did not succeed, or
  - the source has never produced a single raw record.
Status is UP otherwise. Staleness (now minus the newest raw-file mtime, in
days) is always computed and shown — prominently in the Notes column whenever
a source is DOWN, per CLAUDE.md's "call out any source that is down and how
stale it is" — even when the corpus itself is not.

A source can be DOWN (its currently-registered crawler is broken) while still
showing a very fresh last-effective-pull timestamp, if some other means (a
newer crawler variant not yet promoted, another operator's run) wrote fresher
raw files in the meantime. That is reported honestly rather than smoothed over
in either direction: the report never claims "unchanged" and never claims data
is old when raw files on disk demonstrably are not.

Usage:
    health_report.py [corpus-dir] [--out FILE] [--stale-days N] [--json]

Default corpus-dir: <project_root(cwd)>/output/grants/_corpus
Default stale-days: 8 (a week of cadence + one day of slack)
"""
import argparse
import datetime
import json
import os
import sys


def load_registry(script_dir):
    with open(os.path.join(script_dir, 'sources.json')) as f:
        return json.load(f)['sources']


def raw_scan(corpus_dir, dir_name, window_secs=3600):
    """Return (newest_mtime_or_None, total_raw_count, count_in_last_pull_window).

    count_in_last_pull_window approximates how many records the last effective
    pull actually returned/touched: raw files whose mtime falls within
    window_secs of the newest mtime (a single crawl run writes its batch back
    to back, typically well under an hour per the project's documented
    per-source timings)."""
    raw = os.path.join(corpus_dir, dir_name, 'raw')
    if not os.path.isdir(raw):
        return None, 0, 0
    mtimes = []
    with os.scandir(raw) as it:
        for entry in it:
            if not entry.name.endswith('.json'):
                continue
            try:
                mtimes.append(entry.stat().st_mtime)
            except OSError:
                continue
    if not mtimes:
        return None, 0, 0
    newest = max(mtimes)
    in_window = sum(1 for m in mtimes if newest - m <= window_secs)
    return newest, len(mtimes), in_window


def fmt_ts(epoch):
    if epoch is None:
        return 'never'
    return datetime.datetime.fromtimestamp(epoch, datetime.timezone.utc).strftime('%Y-%m-%d %H:%M UTC')


def build_rows(corpus_dir, script_dir, stale_days):
    sources = load_registry(script_dir)

    meta_path = os.path.join(corpus_dir, '_metadata.json')
    meta = {}
    if os.path.exists(meta_path):
        try:
            meta = json.load(open(meta_path))
        except Exception:
            meta = {}
    meta_sources = meta.get('sources', {})

    now = datetime.datetime.now(datetime.timezone.utc).timestamp()
    rows = []

    for s in sources:
        short = s.get('short')
        dir_name = s.get('name')
        crawler = s.get('crawler')
        newest_mtime, total_raw, last_pull_count = raw_scan(corpus_dir, dir_name)
        stale = (now - newest_mtime) / 86400.0 if newest_mtime is not None else None

        run = meta_sources.get(short) or {}
        last_run_status = run.get('last_run_status')   # None if never run under the new driver
        last_run_http = run.get('last_run_http')
        last_run_utc = run.get('last_run_utc')

        notes = []
        down = False

        if not crawler:
            down = True
            notes.append('no crawler implemented for this source yet')
        elif not os.path.isdir(os.path.join(corpus_dir, dir_name, 'raw')) or total_raw == 0:
            down = True
            notes.append('no raw records on disk — never successfully pulled')
        elif last_run_status is not None and last_run_status != 'ok':
            down = True
            http_bit = f" (http={last_run_http})" if last_run_http and last_run_http != 'n/a' else ''
            notes.append(f"last crawl attempt failed: status={last_run_status}{http_bit} at {last_run_utc}")
        elif last_run_status is None and stale is not None and stale > stale_days:
            # No recorded attempt under the new per-run tracking, and the raw
            # corpus itself is stale beyond the freshness window — treat as
            # down rather than silently assuming it's fine.
            down = True
            notes.append(f"no recorded crawl attempt and no fresh raw data in over {stale_days} days")

        if down and stale is not None:
            notes.append(f"staleness: {stale:.1f} days")
        elif down and stale is None:
            notes.append('staleness: never pulled')
        elif not down and stale is not None and stale > stale_days:
            # Crawl mechanism is healthy (last real attempt was "ok") but the
            # source itself hasn't produced a new/changed record in a while —
            # surface it without flipping the source to DOWN.
            notes.append(f"no new/changed record observed in {stale:.1f} days (last crawl still succeeded)")

        if s.get('temporarily_unavailable'):
            notes.append('registry flags this source temporarily_unavailable')

        rows.append({
            'source': dir_name,
            'short': short,
            'last_effective_pull': fmt_ts(newest_mtime),
            'status': 'DOWN' if down else 'UP',
            'records_last_pull': last_pull_count if newest_mtime is not None else 0,
            'records_total_cached': total_raw,
            'notes': '; '.join(notes) if notes else '—',
        })
    return rows


def render_markdown(rows):
    lines = [
        '| Source | Last effective pull | Status | Records (last pull) | Notes |',
        '|---|---|---|---|---|',
    ]
    for r in rows:
        lines.append(
            f"| {r['source']} | {r['last_effective_pull']} | {r['status']} | "
            f"{r['records_last_pull']} | {r['notes']} |"
        )
    down = [r for r in rows if r['status'] == 'DOWN']
    lines.append('')
    if down:
        lines.append(f"**{len(down)} of {len(rows)} sources DOWN:** " +
                      ', '.join(r['source'] for r in down))
    else:
        lines.append(f"All {len(rows)} registered sources UP.")
    return '\n'.join(lines) + '\n'


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('corpus_dir', nargs='?', default=None)
    ap.add_argument('--out', default=None, help='also write the markdown report to this path')
    ap.add_argument('--stale-days', type=float, default=8.0)
    ap.add_argument('--json', action='store_true', help='print rows as JSON instead of markdown')
    args = ap.parse_args()

    script_dir = os.path.dirname(os.path.abspath(__file__))
    corpus_dir = args.corpus_dir or os.path.join(os.getcwd(), 'output', 'grants', '_corpus')

    rows = build_rows(corpus_dir, script_dir, args.stale_days)

    if args.json:
        print(json.dumps(rows, indent=2))
        return

    report = render_markdown(rows)
    print(report)
    if args.out:
        with open(args.out, 'w') as f:
            f.write(report)
        print(f"[health_report] wrote {args.out}", file=sys.stderr)


if __name__ == '__main__':
    main()
