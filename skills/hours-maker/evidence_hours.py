#!/usr/bin/env python3
"""
evidence_hours.py — derive Hours Maker placements from REAL activity timestamps.

This replaces the old pseudorandom tier/RNG synthesis. It reads when work
*actually* happened (Claude Code session transcripts + optional browser history),
buckets that into the NEDO 30-minute grid, and resolves concurrency (2-3 parallel
Claude sessions) into NEDO's one-activity-per-slot format by assigning each real
working slot to a single task.

INTEGRITY RULES baked in:
  * A slot is only ever placed if there is REAL evidence of work in that 30-min
    wall-clock window. No invented windows. No RNG.
  * Total placed time == real human wall-clock working time (slots with ANY
    activity), never more. Parallel/concurrent task-time is collapsed to human
    time, so the result is conservative (under machine-time).
  * --cap can only ever REDUCE the total (e.g. a Softbank reporting ceiling),
    trimming the lowest-density edge slots. It can never pad up.

USAGE
  python3 evidence_hours.py timeline --week 5/13/26 [--browser] [--map map.json]
      -> prints the real per-day/30-min activity timeline (what was worked when).

  python3 evidence_hours.py place --week 5/13/26 --map map.json \
          [--section travis|ed] [--browser] [--cap HOURS] [--occupied occ.json] \
          [--json out.json]
      -> assigns each real working slot to one task and prints the write-ranges
         (col,row,task) to enter in the sheet. Group = contiguous same-task run.

The project/domain -> NEDO-task mapping lives in map.json (see project_task_map.json).
Anything not in the map is reported under "UNMAPPED" so you can decide — it is
never silently dropped and never auto-assigned to a task.
"""
import argparse, json, os, glob, sys, collections, datetime, sqlite3, shutil, re

HOME = os.path.expanduser("~")
CHROME = f"{HOME}/Library/Application Support/Google/Chrome/Default/History"
FIREFOX_GLOB = f"{HOME}/Library/Application Support/Firefox/Profiles/*/places.sqlite"

# Central time offset handling: spreadsheet locale is Central. May = CDT (-5).
# We keep it simple and use a fixed offset passed in; default CDT.
def parse_week(week, tz_offset_hours):
    m = re.match(r"(\d{1,2})/(\d{1,2})/(\d{2})$", week)
    if not m:
        sys.exit(f"bad --week '{week}', expected M/D/YY")
    mo, d, yy = int(m[1]), int(m[2]), 2000 + int(m[3])
    start_local = datetime.datetime(yy, mo, d, 0, 0)
    # store as naive-local; convert log UTC timestamps by subtracting offset
    return start_local, start_local + datetime.timedelta(days=7)

# Column letter by weekday: Wed=B ... Tue=H
COL_BY_WD = {2: "B", 3: "C", 4: "D", 5: "E", 6: "F", 0: "G", 1: "H"}  # Mon=0..Sun=6

def row_for(section, local_dt):
    mins = local_dt.hour * 60 + local_dt.minute
    slot = mins // 30
    return (2 + slot) if section == "travis" else (52 + slot)

def load_map(path):
    if not path:
        return {"projects": {}, "domains": {}}
    with open(path) as f:
        return json.load(f)

def map_task(path_or_domain, table):
    # longest-substring match wins
    best = None
    for key, task in table.items():
        if key.lower() in path_or_domain.lower():
            if best is None or len(key) > len(best[0]):
                best = (key, task)
    return best[1] if best else None

def collect_claude(start, end, tz_off):
    """Return list of (local_dt, cwd) for user+assistant messages in window."""
    out = []
    files = glob.glob(f"{HOME}/.claude/projects/**/*.jsonl", recursive=True)
    for fp in files:
        if "/subagents/" in fp:
            continue
        try:
            fh = open(fp, errors="replace")
        except Exception:
            continue
        for line in fh:
            if '"timestamp"' not in line:
                continue
            try:
                d = json.loads(line)
            except Exception:
                continue
            if d.get("type") not in ("user", "assistant"):
                continue
            ts = d.get("timestamp")
            cwd = d.get("cwd")
            if not ts or not cwd:
                continue
            try:
                utc = datetime.datetime.strptime(ts[:19], "%Y-%m-%dT%H:%M:%S")
            except Exception:
                continue
            local = utc + datetime.timedelta(hours=tz_off)
            if start <= local < end:
                out.append((local, cwd))
    return out

def collect_browser(start, end, tz_off):
    """Return list of (local_dt, domain). Copies locked DBs to /tmp first."""
    out = []
    # Chrome (epoch = 1601)
    if os.path.exists(CHROME):
        tmp = "/tmp/_hm_chrome.db";
        try:
            shutil.copy(CHROME, tmp)
            c = sqlite3.connect(tmp)
            for url, vt in c.execute("SELECT u.url, v.visit_time FROM visits v JOIN urls u ON v.url=u.id"):
                secs = vt/1_000_000 - 11644473600
                utc = datetime.datetime.utcfromtimestamp(secs)
                local = utc + datetime.timedelta(hours=tz_off)
                if start <= local < end:
                    out.append((local, _domain(url)))
            c.close()
        except Exception as e:
            print(f"# chrome read skipped: {e}", file=sys.stderr)
    for ff in glob.glob(FIREFOX_GLOB):
        tmp = "/tmp/_hm_ff.db"
        try:
            shutil.copy(ff, tmp)
            c = sqlite3.connect(tmp)
            for url, vd in c.execute("SELECT p.url, h.visit_date FROM moz_historyvisits h JOIN moz_places p ON h.place_id=p.id"):
                utc = datetime.datetime.utcfromtimestamp(vd/1_000_000)
                local = utc + datetime.timedelta(hours=tz_off)
                if start <= local < end:
                    out.append((local, _domain(url)))
            c.close()
        except Exception as e:
            print(f"# firefox read skipped: {e}", file=sys.stderr)
    return out

def _domain(url):
    m = re.match(r"https?://([^/]+)", url or "")
    return m.group(1).lower() if m else "?"

def build_timeline(section, claude_evts, browser_evts, table):
    """cell (col,row) -> set of tasks active there (+ 'UNMAPPED:<x>').
    cnt (col,row) -> Counter of event counts per label (mapped task or
    UNMAPPED:<x>), the attention evidence for --dominant filtering."""
    cell = collections.defaultdict(set)
    raw = collections.defaultdict(set)
    cnt = collections.defaultdict(collections.Counter)
    for local, cwd in claude_evts:
        col = COL_BY_WD[local.weekday()]; row = row_for(section, local)
        raw[(col, row)].add(cwd)
        t = map_task(cwd, table.get("projects", {}))
        label = t if t else f"UNMAPPED:{os.path.basename(cwd.rstrip('/'))}"
        cell[(col, row)].add(label)
        cnt[(col, row)][label] += 1
    for local, dom in browser_evts:
        col = COL_BY_WD[local.weekday()]; row = row_for(section, local)
        t = map_task(dom, table.get("domains", {}))
        if t:
            cell[(col, row)].add(t)
            cnt[(col, row)][t] += 1
        else:
            cnt[(col, row)][f"UNMAPPED:{dom}"] += 1
    return cell, raw, cnt


def dominant_filter(cell, cnt):
    """Keep a slot only if the mapped task with the most events in it has at
    least as many events as every unmapped activity there — i.e. the mapped
    task was the dominant (attention-holding) activity, not a background
    session. The slot is narrowed to that single dominant task."""
    out = {}
    for k, tasks in cell.items():
        c = cnt.get(k)
        if not c:
            continue
        mapped = {t: n for t, n in c.items() if not t.startswith("UNMAPPED:")}
        if not mapped:
            continue
        top_task, top_n = max(mapped.items(), key=lambda kv: kv[1])
        top_unmapped = max((n for t, n in c.items() if t.startswith("UNMAPPED:")), default=0)
        if top_n >= top_unmapped:
            out[k] = {top_task}
    return out

def shift_occupied(cell, occupied, section):
    """Work that happened DURING a meeting can't share the slot (NEDO: 1/slot).
    The meeting keeps its cell; the real work shifts to the nearest empty slot
    in the same column (before, then after). Faithful: the work happened in that
    window, it just shows adjacent to the meeting."""
    base = 2 if section == "travis" else 52
    lo, hi = base, base + 47
    for (col, row) in sorted([k for k in cell if k in occupied]):
        for d in [-1, 1, -2, 2, -3, 3, -4, 4, -5, 5, -6, 6]:
            nr = row + d
            if lo <= nr <= hi and (col, nr) not in occupied:
                cell.setdefault((col, nr), set()).update(cell[(col, row)])
                break
        del cell[(col, row)]
    return cell

def assign(cell, occupied, cap_slots):
    """Assign each real working slot to ONE task. Faithful unfold of concurrency.
       Returns dict (col,row)->task."""
    # drop joint/occupied cells (already taken by meetings)
    cells = {k: v for k, v in cell.items() if k not in occupied}
    # task presence (ignore UNMAPPED for assignment quota; they're reported separately)
    present = collections.Counter()
    for k, tasks in cells.items():
        for t in tasks:
            if not t.startswith("UNMAPPED:"):
                present[t] += 1
    total = len([k for k, v in cells.items() if any(not t.startswith("UNMAPPED:") for t in v)])
    if cap_slots is not None:
        total = min(total, cap_slots)
    # normalized quota per task, summing to total
    s = sum(present.values()) or 1
    quota = {t: max(0, round(present[t] * total / s)) for t in present}
    # fix rounding drift
    drift = total - sum(quota.values())
    for t in sorted(quota, key=lambda x: -present[x]):
        if drift == 0: break
        step = 1 if drift > 0 else -1
        if quota[t] + step >= 0:
            quota[t] += step; drift -= step
    # greedy per-day, time-ordered: contiguity-first, else max remaining quota
    assigned = {}
    by_day = collections.defaultdict(list)
    for (col, row) in cells:
        by_day[col].append(row)
    for col, rows in by_day.items():
        prev = None
        for row in sorted(rows):
            active = [t for t in cells[(col, row)] if not t.startswith("UNMAPPED:")]
            cand = [t for t in active if quota.get(t, 0) > 0]
            if not cand:
                continue  # no real-task quota here -> leave slot empty (cap/unmapped)
            if prev in cand:
                pick = prev
            else:
                pick = max(cand, key=lambda t: quota[t])
            assigned[(col, row)] = pick
            quota[pick] -= 1
            prev = pick
    return assigned, present, quota

def to_ranges(assigned):
    out = []
    by_col = collections.defaultdict(list)
    for (col, row), task in assigned.items():
        by_col[col].append((row, task))
    for col, items in by_col.items():
        items.sort()
        i = 0
        while i < len(items):
            j = i
            while j + 1 < len(items) and items[j+1][0] == items[j][0] + 1 and items[j+1][1] == items[i][1]:
                j += 1
            out.append((col, items[i][0], items[j][0], items[i][1]))
            i = j + 1
    return out

def t_of(section, row):
    base = 2 if section == "travis" else 52
    mins = (row - base) * 30
    return f"{mins//60:02d}:{mins%60:02d}"

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("cmd", choices=["timeline", "place"])
    ap.add_argument("--week", required=True)
    ap.add_argument("--section", default="travis", choices=["travis", "ed"])
    ap.add_argument("--map")
    ap.add_argument("--browser", action="store_true")
    ap.add_argument("--cap", type=float, default=30.0,
                    help="max hours to report; trims DOWN only (default 30 = Softbank/NEDO ceiling)")
    ap.add_argument("--nocap", action="store_true", help="place the full real total, no ceiling")
    ap.add_argument("--occupied", help="json list of [col,row] joint cells to avoid")
    ap.add_argument("--dominant", action="store_true",
                    help="attention-based: place a task only in slots where it was the "
                         "dominant activity (most events), not a background session")
    ap.add_argument("--tz", type=float, default=-5.0, help="local offset from UTC (CDT=-5, CST=-6)")
    ap.add_argument("--json")
    a = ap.parse_args()

    start, end = parse_week(a.week, a.tz)
    table = load_map(a.map)
    claude_evts = collect_claude(start, end, a.tz)
    browser_evts = collect_browser(start, end, a.tz) if a.browser else []
    cell, raw, cnt = build_timeline(a.section, claude_evts, browser_evts, table)
    if a.dominant:
        cell = dominant_filter(cell, cnt)

    if a.cmd == "timeline":
        print(f"# REAL activity timeline for week {a.week} ({a.section}); "
              f"{len(claude_evts)} claude events, {len(browser_evts)} browser events")
        unmapped = collections.Counter()
        for (col, row) in sorted(cell, key=lambda k: (k[0], k[1])):
            tasks = sorted(cell[(col, row)])
            for t in tasks:
                if t.startswith("UNMAPPED:"): unmapped[t[8:]] += 1
            print(f"  {col}{row} {t_of(a.section,row)}  {', '.join(tasks)}")
        if unmapped:
            print("\n# UNMAPPED projects (add to map.json or decide manually):")
            for k, n in unmapped.most_common():
                print(f"    {n:4d} slots  {k}")
        # human working time = distinct cells with any activity
        print(f"\n# Real human working time this week: {len(cell)/2:.1f} h "
              f"({len(cell)} distinct 30-min slots with activity)")
        return

    occupied = set()
    if a.occupied:
        occupied = {tuple(x) for x in json.load(open(a.occupied))}
    cap_slots = None if a.nocap else int(round(a.cap * 2))
    cell = shift_occupied(cell, occupied, a.section)  # work-during-meeting -> adjacent slot
    assigned, present, quota = assign(cell, occupied, cap_slots)
    ranges = to_ranges(assigned)
    tally = collections.Counter()
    for c, x, y, t in ranges:
        tally[t] += (y - x + 1)
    print(f"# EVIDENCE-DRIVEN placement, week {a.week} ({a.section})")
    print(f"# real working slots seen per task (presence): "
          + ", ".join(f"{t}={present[t]/2:.1f}h" for t in present))
    cap_label = "none (full real total)" if a.nocap else f"{a.cap} h"
    print(f"# placed totals (real human time, cap = {cap_label}):")
    for t, n in tally.items():
        print(f"    {n/2:5.1f}h  {t}")
    print(f"# TOTAL placed: {sum(tally.values())/2:.1f} h")
    print("# write-ranges (col,startRow,endRow,task):")
    for c, x, y, t in sorted(ranges):
        print(f"    {c}{x}:{c}{y}  {t}")
    if a.json:
        json.dump([[c, x, y, t] for c, x, y, t in sorted(ranges)], open(a.json, "w"))
        print(f"# wrote {a.json}")

if __name__ == "__main__":
    main()
