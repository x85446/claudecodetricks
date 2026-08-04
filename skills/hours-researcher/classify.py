#!/usr/bin/env python3
"""
classify.py — build a granular, TRUTHFUL activity base from real evidence.

Reads when activity actually happened (Claude Code session transcripts + browser
history WITH page titles + URLs) and classifies each event via a user-owned map
into: class (work|personal), company, nedo, category. Buckets into hour blocks and
emits a month.md truth base with per-day summaries + EVIDENCE so every claim is
checkable.

Classification order for a browser event:
  1. drop-list  -> discarded entirely (noise; counted only as "dropped N").
  2. search engines -> extract the query (?q=) and classify the QUERY text via
     keyword rules. Label shows the actual search ("search: kubernetes longhorn").
  3. domain map (host substring). For title-bearing hosts (chatgpt, docs.google,
     meet.google, sharepoint, github) the page TITLE is shown as detail.
  4. keyword rules applied to (title + url-path) — "see what I was researching".
  5. else UNMAPPED, reported with a sample title/url so it can be classified.
Claude events match on the full cwd path; the LABEL/UNMAPPED shows a workspace-
relative path (so "projects"/"tmp" are disambiguated, not bare basenames).

INTEGRITY: pure analysis of real timestamps; nothing synthesized/placed; UNMAPPED
never guessed; daily totals use one dominant activity per 30-min slot.

USAGE
  python3 classify.py --month 2026-06 [--end 2026-06-15] \
          --map classification_map.json --browser --tz -5 --out /tmp/june.md
"""
import argparse, json, os, glob, sys, collections, datetime, sqlite3, shutil, re, urllib.parse

HOME = os.path.expanduser("~")
CHROME = f"{HOME}/Library/Application Support/Google/Chrome/Default/History"
FIREFOX_GLOB = f"{HOME}/Library/Application Support/Firefox/Profiles/*/places.sqlite"
SEARCH_HOSTS = {"www.google.com":"q","www.bing.com":"q","duckduckgo.com":"q",
                "search.brave.com":"q","www.youtube.com":"search_query"}
TITLE_HOSTS = ("chatgpt.com","chat.openai.com","docs.google.com","meet.google.com",
               "sharepoint.com","github.com","drive.google.com")

def host_of(url):
    m = re.match(r"https?://([^/]+)", url or ""); return m.group(1).lower() if m else "?"
def path_of(url):
    try: p = urllib.parse.urlparse(url); return (p.path + " " + p.query).replace("+"," ")
    except Exception: return ""
def search_query(url):
    h = host_of(url)
    if h not in SEARCH_HOSTS: return None
    qs = urllib.parse.parse_qs(urllib.parse.urlparse(url).query)
    v = qs.get(SEARCH_HOSTS[h])
    return v[0] if v else None
def rel_cwd(cwd):
    for p in ("/Users/travis/workspace/","/home/travis/workspace/"):
        if cwd.startswith(p): return cwd[len(p):]
    if cwd.startswith(HOME+"/"): return "~/"+cwd[len(HOME)+1:]
    return cwd

def collect_claude(start, end, tz):
    out = []
    for fp in glob.glob(f"{HOME}/.claude/projects/**/*.jsonl", recursive=True):
        if "/subagents/" in fp: continue
        try: fh = open(fp, errors="replace")
        except Exception: continue
        for line in fh:
            if '"timestamp"' not in line: continue
            try: d = json.loads(line)
            except Exception: continue
            if d.get("type") not in ("user","assistant"): continue
            ts, cwd = d.get("timestamp"), d.get("cwd")
            if not ts or not cwd: continue
            try: u = datetime.datetime.strptime(ts[:19], "%Y-%m-%dT%H:%M:%S")
            except Exception: continue
            loc = u + datetime.timedelta(hours=tz)
            if start <= loc < end: out.append((loc,"claude",cwd,""))
    return out

def collect_browser(start, end, tz):
    out = []
    if os.path.exists(CHROME):
        try:
            shutil.copy(CHROME, "/tmp/_hr_chrome.db"); c = sqlite3.connect("/tmp/_hr_chrome.db")
            for url,title,vt in c.execute("SELECT u.url,u.title,v.visit_time FROM visits v JOIN urls u ON v.url=u.id"):
                u = datetime.datetime.utcfromtimestamp(vt/1_000_000 - 11644473600)
                loc = u + datetime.timedelta(hours=tz)
                if start <= loc < end: out.append((loc,"browser",url,title or ""))
            c.close()
        except Exception as e: print(f"# chrome skipped: {e}", file=sys.stderr)
    for ff in glob.glob(FIREFOX_GLOB):
        try:
            shutil.copy(ff, "/tmp/_hr_ff.db"); c = sqlite3.connect("/tmp/_hr_ff.db")
            for url,title,vd in c.execute("SELECT p.url,p.title,h.visit_date FROM moz_historyvisits h JOIN moz_places p ON h.place_id=p.id"):
                u = datetime.datetime.utcfromtimestamp(vd/1_000_000)
                loc = u + datetime.timedelta(hours=tz)
                if start <= loc < end: out.append((loc,"browser",url,title or ""))
            c.close()
        except Exception as e: print(f"# firefox skipped: {e}", file=sys.stderr)
        break
    return out

def match_sub(key, tbl):
    best = None
    for k, info in tbl.items():
        if k.lower() in key.lower() and (best is None or len(k) > len(best[0])):
            best = (k, info)
    return best[1] if best else None
def match_kw(text, rules):
    for r in rules:
        if re.search(r["match"], text or "", re.I):
            return {k:v for k,v in r.items() if k != "match"}
    return None

def classify(source, key, detail, table):
    """Return (info|None|'DROP', label)."""
    if source == "claude":
        rp = rel_cwd(key)
        return match_sub(key, table["projects"]), f"claude:{rp}"
    host = host_of(key)
    if any(d.lower() in host for d in table.get("drop", [])):
        return "DROP", None
    q = search_query(key)
    if q is not None:
        info = match_kw(q, table.get("keywords", []))
        return info, f"search: {q[:60]}"
    info = match_sub(host, table["domains"])
    if info:
        lbl = f"browser:{host}"
        if any(t in host for t in TITLE_HOSTS) and detail:
            lbl += f" «{detail[:50]}»"
        return info, lbl
    info = match_kw((detail or "") + " " + path_of(key), table.get("keywords", []))
    if info:
        return info, f"browser:{host} «{(detail or path_of(key))[:50]}»"
    return None, f"browser:{host}" + (f" «{detail[:50]}»" if detail else "")

def tag_of(info):
    # tag = (class, MASTER, company, nedo, sub-category)
    if info.get("class") == "personal":
        return ("personal", info.get("master","Personal·Misc"), None, False, info.get("category","personal"))
    return ("work", info.get("master","Izuma·Business"), info.get("company","?"),
            bool(info.get("nedo",False)), info.get("category","work"))
def tag_label(t):
    cls,master,comp,nedo,cat = t
    if cls=="personal": return f"{master} — {cat}"
    return f"{master} | {comp} — {cat}"

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--month", required=True); ap.add_argument("--end")
    ap.add_argument("--map", required=True); ap.add_argument("--browser", action="store_true")
    ap.add_argument("--tz", type=float, default=-5.0); ap.add_argument("--out", default="/tmp/month.md")
    a = ap.parse_args()
    y,m = map(int, a.month.split("-"))
    start = datetime.datetime(y,m,1); end = datetime.datetime(y+(m//12),(m%12)+1,1)
    if a.end:
        ey,em,ed = map(int,a.end.split("-")); end = min(end, datetime.datetime(ey,em,ed))
    table = json.load(open(a.map))
    evts = collect_claude(start,end,a.tz) + (collect_browser(start,end,a.tz) if a.browser else [])

    slot = collections.defaultdict(list)            # (date,slotidx) -> [(tag, label)]
    unmapped = collections.defaultdict(collections.Counter)  # host/key -> Counter(sample labels)
    dropped = 0
    for loc, source, key, detail in evts:
        info, label = classify(source, key, detail, table)
        sidx = loc.hour*2 + (1 if loc.minute>=30 else 0)
        if info == "DROP": dropped += 1; continue
        if info is None:
            base = label.split(" «")[0]
            unmapped[base][label] += 1
            slot[(loc.date(),sidx)].append((("unmapped","UNMAPPED",None,False,"unmapped"), label))
            continue
        slot[(loc.date(),sidx)].append((tag_of(info), label))

    days = sorted(set(d for (d,s) in slot))
    L = [f"# {start.strftime('%B %Y')} — Activity Research (granular truth base)",
         f"# Source: {sum(1 for e in evts if e[1]=='claude')} claude + {sum(1 for e in evts if e[1]=='browser')} browser events; "
         f"window {start.date()}..{end.date()} (tz {a.tz}); dropped {dropped} noise events",
         "# INTEGRITY: real timestamps only; UNMAPPED reported below; nothing synthesized.\n"]
    for d in days:
        dom = {}
        for s in range(48):
            cnt = collections.Counter(t for (t,l) in slot.get((d,s),[]) if t[0]!="unmapped")
            if cnt: dom[s] = cnt.most_common(1)[0][0]
        active = len([s for s in range(48) if slot.get((d,s))]) / 2
        nedo = len([s for s,t in dom.items() if t[0]=="work" and t[3]]) / 2
        work = len([s for s,t in dom.items() if t[0]=="work"]) / 2
        pers = len([s for s,t in dom.items() if t[0]=="personal"]) / 2
        masters = collections.Counter(t[1] for t in dom.values())   # by MASTER, dominant per slot
        mstr = ", ".join(f"{mm} {n/2}" for mm,n in masters.most_common())
        L.append(f"## {d.strftime('%B %-d %Y')}  [active {active}h | NEDO {nedo}h | work {work}h | personal {pers}h]")
        L.append(f"   by master: {mstr}")
        for hour in range(24):
            evs = slot.get((d,hour*2),[]) + slot.get((d,hour*2+1),[])
            if not evs: continue
            L.append(f"{hour}:00-{hour+1}:00")
            bytag = collections.defaultdict(collections.Counter)
            for (t,l) in evs: bytag[t][l] += 1
            order = lambda t:(0 if (t[0]=="work" and t[3]) else 1 if t[0]=="work" else 2 if t[0]=="personal" else 3, str(t))
            for t in sorted(bytag, key=order):
                ev = ", ".join(f"{k} x{n}" for k,n in bytag[t].most_common(4))
                L.append(f" - {'UNMAPPED' if t[0]=='unmapped' else tag_label(t)}  ({ev})")
        L.append("")
    if unmapped:
        L.append("## UNMAPPED — decide & add to classification_map.json (never auto-counted)")
        tot = lambda h: sum(unmapped[h].values())
        for h in sorted(unmapped, key=lambda x:-tot(x)):
            samples = "; ".join(s.split('«')[1].rstrip('»') for s,_ in unmapped[h].most_common(3) if '«' in s)
            L.append(f"   {tot(h):5d}  {h}" + (f"   e.g. {samples[:90]}" if samples else ""))
    open(a.out,"w").write("\n".join(L))
    print(f"wrote {a.out}: {len(days)} days, {len(evts)} events, {dropped} dropped, {len(unmapped)} unmapped hosts")

if __name__ == "__main__":
    main()
