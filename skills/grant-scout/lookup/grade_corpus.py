#!/usr/bin/env python3
"""
grade_corpus.py — apply the grant-scout 5-axis rubric across the SQLite corpus.

This is the SCALABLE grader: a principled keyword/heuristic implementation of the
SKILL.md rubric so 10k+ opportunities can be triaged in one pass. It is NOT a
replacement for an LLM deep-read of the finalists — it ranks the corpus so a human
(or a follow-up grant-scout deep grade) only reads the A/B tier.

Scores `needs_grading` rows (active + not yet scored at the current profile version),
or every active row with --rescore-all. Writes current `scores` and ALWAYS appends to
`score_history`. Then emits a ranked master report.

Two text tiers
--------------
A record's own words (title + its own description + extracted attachment text + its
published Q&A) are the PRIMARY signal. Solicitation-wide text — one NSF/DoD PDF shared
by up to 268 topics — is a SECONDARY signal, scored at reduced weight, because a
presence regex over shared boilerplate fires identically on every record that shares it
and carries no information about the individual topic. Secondary text arrives either
folded into topic_text behind a marker (normalize_sbir with SBIR_FOLD_SOLICITATION=1) or
as its own column (solicitation_text / instructions_text); both are handled.

Secondary text still counts for CONFIDENCE: a record carrying 20k+ chars of attached
solicitation is not a `confidence=low` record just because sbir.gov published no
per-topic blurb for it.

Usage:
  grade_corpus.py [--db output/grants/grants.db] [--min-deadline 2026-07-01]
                  [--report output/grants/_MASTER_<date>.md] [--limit N]
                  [--rescore-all] [--exclude-undated]
                  [--watchlist .claude/skills/grant-scout/watchlist.md]
                  [--corpus output/grants/_corpus]

Axis weights (must match SKILL.md Step 4): A1 .35  A2 .20  A3 .25  A4 .15  A5 .05(bonus)
"""
import os, sys, re, json, sqlite3, datetime

# ---- Axis 1: Roadmap alignment -- product theme -> base score (take best match) ----
ROADMAP = [
    (95, 'Exonet (DDIL mesh)', r'ddil|disconnected|contested|encrypted mesh|wireguard|\bcrdt\b|multipoint|partition.?toleran|tactical (edge )?network|resilient comm|mesh network'),
    (92, 'Myriplane/VirtualCluster', r'multi-?tenan|federat|tenant isolation|control plane|\bvcluster\b|kubernetes|k8s|container orchestrat|namespace isolation'),
    (88, 'Metalworks (bare-metal/HRoT)', r'bare.?metal|\bsmarc\b|computer.?on.?module|\bsom\b|com.?express|uefi|secure boot|\btpm\b|attestation|remote provisioning|root of trust'),
    (80, 'Device Management/FOTA', r'lwm2m|\bfota\b|firmware (over|update)|device management|fleet (management|ops)|\biot\b (device|fleet)|post.?quantum|\bpki\b migration'),
    (70, 'cross-cutting platform', r'\bpki\b|identity|observability|sd-?wan|edge (comput|orchestrat|server)|zero.?trust|\boidc\b|\bopa\b|\brbac\b|gossip|service mesh'),
]
ROADMAP_ADJ = r'\bedge\b|container|secure|distributed|encrypt|network|cloud|software-defined|virtuali'
# A roadmap theme found ONLY in shared solicitation boilerplate is evidence about the
# program, not about this topic. Cap it at the "adjacent" tier rather than letting one
# shared PDF hand 268 sibling records a 95.
SECONDARY_A1_CAP = 50

# ---- Axis 2: Customer segment -- agency token -> weight ----
SEGMENTS = [
    (100, r'\barmy\b|\bnavy\b|air force|space force|marine|socom|\bdiu\b|darpa|\bdod\b|defense|\bdtra\b|\bdisa\b|\bdla\b|sbir.*(af|army|navy|dod)|missile defense'),
    (95,  r'\bnsa\b|\bnga\b|\bnro\b|\bodni\b|intelligence|in-?q-?tel|\bic\b community'),
    (80,  r'\bdhs\b|cisa|\bdoe\b|\bnih\b|\bnsf\b|\bnasa\b|critical infrastructure|grid|homeland'),
    (70,  r'arpa-?e|manufactur|smart.?grid|utility|industrial|\bot\b security|\bics\b'),
    (20,  r'education|\bk-?12\b|stem outreach|museum|undergraduate|teacher'),
]
# Agencies with Izuma submission history (curated, from refresh_profile) -> small boost.
SEG_BOOST = r'\barmy\b|\bnavy\b|darpa|\bnec\b'

# ---- Axis 3: Skillset ----
STRONG = r'distributed system|kubernetes|orchestrat|cryptograph|\bpki\b|wireguard|\bmls\b|\bnoise\b|\bcrdt\b|gossip|disconnected|partition|lwm2m|multi-?tenan|\bopa\b|\boidc\b|\brbac\b|operator|\bcrd\b|webhook|mesh|edge comput|firmware|attestation|\btpm\b'
WEAK = r'\basic\b|\bfpga\b|\brtl\b|silicon|semiconductor|chip fab|foundry|wafer|model training|train(ing)? (a |large )?(language )?model|\bllm\b training|gpu cluster|wet.?lab|biotech|genom|materials science|mechanical|machining|robot(ic)? control|propulsion|hypersonic airframe'
# Strong hits seen only in shared boilerplate contribute at 1/4 rate, capped.
SECONDARY_A3_CAP = 8

# ---- Axis 5: Personality ----
PERS_PLUS = r'dual-?use|open.?source|open standard|small business|sbir|sttr|commercial(ization)?|software-only'
PERS_MINUS = r'proprietary (deliverable|lock)|foreign ownership prohibited|itar.*restrict|sole government rights'

# ---- Eligibility pre-filter -> auto F (SKILL.md Step 2.5) ----
ELIG_EXCLUDE = r'\b(states?|territor|tribal|local government|municipal|law enforcement|fire department|\bems\b|school district|k-?12|public housing|hospital)s?\b.*only|nonprofit organizations? only|institutions? of higher education only'
ELIG_OK = r'sbir|sttr|small business|for-?profit|all eligible|any (entity|organization)'

GRADE_BANDS = [(95,'A+'),(90,'A'),(85,'A-'),(80,'B+'),(75,'B'),(70,'B-'),
               (65,'C+'),(60,'C'),(55,'C-'),(45,'D'),(0,'F')]

# Markers normalize.py writes when it folds solicitation-wide text into topic_text.
# Everything from the FIRST marker onward is secondary. "Published Q&A:" is deliberately
# absent — Q&A is topic-specific and stays primary.
SECONDARY_MARKERS = ('\nOfficial solicitation text:\n', '\nProposal instructions:\n')

# Regex input caps. topic_text runs to 200k chars; scanning all of it on both tiers for
# every one of 15k records is the whole runtime.
PRIMARY_CAP = 200_000
SECONDARY_CAP = 60_000

# A record with this much attached solicitation text is not `confidence=low`.
ATTACHED_CONF_CHARS = 20_000
PRIMARY_CONF_CHARS = 200


# ---------------------------------------------------------------- watch-list ----
# watchlist.md is the operator's file and decides which entries EXIST; the match
# semantics for each known entry live here because WL-002 is an AND/NOT composition
# that a comma-separated signals line cannot express.
WATCH_RULES = {
    'WL-001': dict(
        require=[r'content disarm|\bcdr\b|cross.?domain video|video sanitis|video sanitiz|'
                 r'media guard|transcode guard|pixel reconstruction|render.?and.?recapture|'
                 r'rasteriz|steganography removal|covert.?channel removal|'
                 r'video filter for cross.?domain|(verified|trusted) media transform'],
        forbid=[],
    ),
    'WL-002': dict(
        require=[r'cross.?domain|\bcds\b|security domain|data guard|multi.?level security',
                 r'\bdevelop|\bresearch|\bnovel\b|prototype|proof of concept|phase i\b|'
                 r'white paper|\bbaa\b'],
        # An already-accredited / baseline-listed guard requirement is a structural pass,
        # not a WL-002 match.
        forbid=[r'accredited|ncdsmo baseline|raise the bar|on the baseline list|'
                r'reference.?verifiable required'],
    ),
}
WATCH_A1_BOOST = 8


def load_watchlist(path):
    """Return [(id, title)] for every WL-00N entry in the operator's watch-list."""
    if not path or not os.path.exists(path):
        return []
    out = []
    for line in open(path):
        m = re.match(r'^##\s+(WL-\d+)\s*[—-]\s*(.+?)\s*$', line)
        if m:
            out.append((m.group(1), m.group(2)))
    return out


def watch_match(entry_id, primary):
    rule = WATCH_RULES.get(entry_id)
    if not rule:
        return False
    for pat in rule['require']:
        if not re.search(pat, primary, re.I):
            return False
    for pat in rule['forbid']:
        if re.search(pat, primary, re.I):
            return False
    return True


# ---------------------------------------------------------------- scoring ----
def grade_of(pct):
    for thr, g in GRADE_BANDS:
        if pct >= thr:
            return g
    return 'F'


def best(table, text):
    for score, *rest in table:
        pat = rest[-1]
        if re.search(pat, text, re.I):
            return score, (rest[0] if len(rest) > 1 else None)
    return None


def split_tiers(topic_text):
    """Split topic_text into (own words, shared solicitation boilerplate)."""
    t = topic_text or ''
    cut = len(t)
    for marker in SECONDARY_MARKERS:
        i = t.find(marker)
        if i != -1:
            cut = min(cut, i)
    return t[:cut], t[cut:]


def score_one(o, watchlist=()):
    own_topic, folded_sol = split_tiers(o.get('topic_text'))
    primary = ' '.join(str(o.get(k) or '') for k in
                       ('title', 'agency', 'subagency', 'phase'))
    primary = (primary + ' ' + own_topic)[:PRIMARY_CAP]
    secondary = ' '.join(x for x in (folded_sol,
                                     o.get('solicitation_text') or '',
                                     o.get('instructions_text') or '') if x)[:SECONDARY_CAP]

    # Eligibility pre-filter. Primary only — a shared BAA that mentions "nonprofit
    # organizations only" somewhere in 250 KB must not auto-F every topic under it.
    elig = (o.get('eligibility') or '') + ' ' + primary
    if re.search(ELIG_EXCLUDE, elig, re.I) and not re.search(ELIG_OK, elig, re.I):
        return dict(grade='F', pct=0.0, a1=0, a2=0, a3=0, a4=0, a5=0,
                    confidence='med', watch=[], why='Killed by eligibility, not topic.')

    # A1 roadmap — own words first; shared boilerplate only as a capped fallback.
    r = best(ROADMAP, primary)
    if r:
        a1, theme = r[0], r[1]
    elif re.search(ROADMAP_ADJ, primary, re.I):
        a1, theme = 45, 'adjacent'
    else:
        rs = best(ROADMAP, secondary) if secondary else None
        if rs:
            a1, theme = min(rs[0], SECONDARY_A1_CAP), f'{rs[1]} (shared-doc only)'
        elif secondary and re.search(ROADMAP_ADJ, secondary, re.I):
            a1, theme = 35, 'adjacent (shared-doc only)'
        else:
            a1, theme = 22, 'off-roadmap'

    # A2 segment
    seg = best(SEGMENTS, primary)
    a2 = seg[0] if seg else 40
    if re.search(SEG_BOOST, primary, re.I):
        a2 = min(100, a2 + 5)

    # A3 skillset
    strong = len(re.findall(STRONG, primary, re.I))
    weak = len(re.findall(WEAK, primary, re.I))
    strong2 = len(re.findall(STRONG, secondary, re.I)) if secondary else 0
    a3 = 35 + min(strong, 8) * 8                                  # 35..99
    a3 += min(strong2 // 4, SECONDARY_A3_CAP)                     # shared doc, 1/4 rate
    a3 = min(99, a3)
    if weak:
        a3 = max(8, a3 - weak * 20)

    # A4 execution
    if re.search(r'sbir|sttr', primary, re.I):
        a4 = 90
    elif re.search(r'\bbaa\b|broad agency|\bota\b|other transaction|\bcso\b|commercial solutions', primary, re.I):
        a4 = 75
    else:
        a4 = 70
    fc = o.get('funding_ceiling')
    try:
        if fc and float(fc) > 10_000_000:
            a4 = min(a4, 45)
    except (TypeError, ValueError):
        pass

    # A5 personality (bonus/penalty, -20..+10)
    a5 = 0
    if re.search(PERS_PLUS, primary, re.I):
        a5 += 5
    if re.search(r'dual-?use', primary, re.I) and re.search(r'open.?source|open standard', primary, re.I):
        a5 += 5
    if re.search(PERS_MINUS, primary, re.I):
        a5 -= 10
    a5 = max(-20, min(10, a5))

    # Operator watch-list: matched on the record's own words, never on shared
    # boilerplate (a marker in a shared BAA would flag all its siblings).
    watch = [wid for wid, _ in watchlist if watch_match(wid, primary)]
    if watch:
        a1 = min(100, a1 + WATCH_A1_BOOST)

    pct = 0.35*a1 + 0.20*a2 + 0.25*a3 + 0.15*a4 + 0.05*a5
    pct = max(0.0, min(100.0, pct))

    # Confidence. Own words carry it; failing that, a substantial attached solicitation
    # is still real evidence and downgrades the record to 'med', not 'low'.
    attached = (len(secondary)
                + int(o.get('solicitation_chars') or 0)
                + int(o.get('instructions_chars') or 0))
    if len(own_topic) >= PRIMARY_CONF_CHARS:
        conf = 'high'
    elif attached >= ATTACHED_CONF_CHARS:
        conf = 'med'
    else:
        conf = 'low'

    why = f"roadmap={theme}; seg={a2}; skill_strong={strong}(+{strong2} shared)/weak={weak}"
    if watch:
        why = 'WATCH:' + ','.join(watch) + '; ' + why
    return dict(grade=grade_of(pct), pct=round(pct, 1),
                a1=a1, a2=a2, a3=a3, a4=a4, a5=a5, confidence=conf,
                watch=watch, why=why)


# ---------------------------------------------------------------- report ----
def reconciliation(corpus_dir, con, min_deadline, scored, run_stamp, seen_since=None):
    """Account for every record: normalized -> merged -> ingested -> active -> scored."""
    rows = []
    norm_total = 0
    src_dirs = ['sam.gov', 'grants.gov', 'grants.gov.extract', 'sbir.gov',
                'dsip', 'darpa.mil', 'xtech']
    for sub in src_dirs:
        p = os.path.join(corpus_dir, sub, 'normalized.jsonl')
        n = 0
        if os.path.exists(p):
            with open(p) as f:
                for _ in f:
                    n += 1
        norm_total += n
        rows.append((sub, n))
    all_json = os.path.join(corpus_dir, '_all.json')
    merged = cross = 0
    if os.path.exists(all_json):
        recs = json.load(open(all_json))
        merged = len(recs)
        cross = sum(len(r.get('_cross_refs') or []) for r in recs)
    n_opp = con.execute("SELECT count(*) FROM opportunities").fetchone()[0]
    n_active = con.execute("SELECT count(*) FROM active_opportunities").fetchone()[0]
    n_run = con.execute("SELECT count(*) FROM scores WHERE graded_at=?", (run_stamp,)).fetchone()[0]
    n_seen = n_seen_active = None
    if seen_since:
        n_seen = con.execute("SELECT count(*) FROM opportunities WHERE last_seen >= ?",
                             (seen_since,)).fetchone()[0]
        n_seen_active = con.execute("SELECT count(*) FROM active_opportunities WHERE last_seen >= ?",
                                    (seen_since,)).fetchone()[0]
    return dict(per_source=rows, norm_total=norm_total, merged=merged, cross=cross,
                opportunities=n_opp, active=n_active, scored=scored, scored_this_run=n_run,
                seen_this_run=n_seen, active_this_run=n_seen_active)


def write_report(path, con, pv, min_deadline, run_stamp, recon, watchlist, include_undated):
    date_pred = ("(o.close_date IS NULL OR o.close_date='' OR o.close_date >= ?)"
                 if include_undated else "o.close_date >= ?")
    top = con.execute(f"""SELECT o.solicitation_number,o.title,o.source,o.agency,o.close_date,
                                 s.grade,s.pct,s.confidence,o.url,o.id
                          FROM scores s JOIN opportunities o ON o.id=s.opportunity_id
                          WHERE {date_pred} AND s.graded_at=?
                          ORDER BY s.pct DESC""", (min_deadline, run_stamp)).fetchall()
    dist, conf_dist = {}, {}
    for r in top:
        dist[r['grade']] = dist.get(r['grade'], 0) + 1
        conf_dist[r['confidence']] = conf_dist.get(r['confidence'], 0) + 1

    # Watch-list hits are recorded in score_history.why-free form, so recompute the flag
    # set from the run's watch table written by main().
    watch_rows = con.execute(f"""SELECT o.solicitation_number,o.title,o.source,o.agency,
                                        o.close_date,s.grade,s.pct,s.confidence,o.url,w.entry
                                 FROM watch_flags w
                                 JOIN opportunities o ON o.id=w.opportunity_id
                                 JOIN scores s ON s.opportunity_id=o.id
                                 WHERE w.run_stamp=?
                                 ORDER BY w.entry, s.pct DESC""", (run_stamp,)).fetchall()

    os.makedirs(os.path.dirname(path) or '.', exist_ok=True)
    with open(path, 'w') as f:
        f.write(f"# grant-scout master ranking — {datetime.date.today().isoformat()}\n\n")
        f.write(f"Profile version: v{pv}. Scored {recon['scored']} open/upcoming opportunities "
                f"(deadline ≥ {min_deadline}"
                f"{', undated/rolling included' if include_undated else ''}) across all sources. "
                f"Heuristic rubric pass (SKILL.md 5-axis, eligibility pre-filter per Step 2.5) — "
                f"treat A/B tier as the deep-read shortlist, not a final grade.\n\n")
        f.write("**Grade distribution:** " + ', '.join(f"{g}={dist[g]}" for g in
                ['A+','A','A-','B+','B','B-','C+','C','C-','D','F'] if g in dist) + "\n\n")
        f.write("**Confidence distribution:** " + ', '.join(
            f"{c}={conf_dist[c]}" for c in ['high','med','low'] if c in conf_dist) + "\n\n")

        # ---- watch-list ----
        f.write("## Operator watch-list hits\n\n")
        if not watch_rows:
            f.write(f"No records matched the operator watch-list "
                    f"({', '.join(w for w, _ in watchlist)}) this run.\n\n")
        else:
            f.write("| WATCH | Grade | % | Deadline | Source | Title | ID |\n|---|---|---|---|---|---|---|\n")
            for r in watch_rows:
                title = (r['title'] or '')[:70].replace('|', '/')
                f.write(f"| WATCH: {r['entry']} | {r['grade']} | {r['pct']} | "
                        f"{r['close_date'] or 'rolling'} | {r['source']} | {title} | "
                        f"{r['solicitation_number'] or ''} |\n")
            f.write("\n")

        # ---- top ----
        f.write("## Top 60 (pursue / deep-read first)\n\n")
        f.write("| Grade | % | Conf | Deadline | Source | Agency | Title | ID |\n"
                "|---|---|---|---|---|---|---|---|\n")
        watch_ids = {r['solicitation_number'] for r in watch_rows}
        for r in top[:60]:
            title = (r['title'] or '')[:70].replace('|', '/')
            if r['solicitation_number'] in watch_ids:
                title = 'WATCH: ' + title
            f.write(f"| {r['grade']} | {r['pct']} | {r['confidence']} | "
                    f"{r['close_date'] or 'rolling'} | {r['source']} | "
                    f"{(r['agency'] or '')[:18]} | {title} | {r['solicitation_number'] or ''} |\n")

        # ---- reconciliation ----
        f.write("\n## Reconciliation — every record accounted for\n\n")
        f.write("| Stage | Count |\n|---|---|\n")
        for sub, n in recon['per_source']:
            f.write(f"| normalized: {sub} | {n} |\n")
        f.write(f"| **normalized total** | **{recon['norm_total']}** |\n")
        f.write(f"| merged unique (`_all.json`) | {recon['merged']} |\n")
        f.write(f"| cross-source duplicates collapsed | {recon['cross']} |\n")
        f.write(f"| merged + collapsed (= normalized total) | "
                f"{recon['merged'] + recon['cross']} |\n")
        f.write(f"| DB `opportunities` (all runs, cumulative) | {recon['opportunities']} |\n")
        f.write(f"| DB `active_opportunities` (open or undated, cumulative) | {recon['active']} |\n")
        if recon.get('seen_this_run') is not None:
            f.write(f"| DB rows seen in THIS run (= merged unique) | {recon['seen_this_run']} |\n")
            f.write(f"| … of those, still open or undated | {recon['active_this_run']} |\n")
            f.write(f"| … closed before today (not scored) | "
                    f"{recon['seen_this_run'] - recon['active_this_run']} |\n")
        f.write(f"| scored this run | {recon['scored_this_run']} |\n")
        f.write(f"| ranked in this report (deadline filter applied) | {len(top)} |\n")
        assert_line = ("OK" if recon['merged'] + recon['cross'] == recon['norm_total']
                       else "MISMATCH")
        f.write(f"\nMerge reconciliation: **{assert_line}** "
                f"({recon['merged']} unique + {recon['cross']} collapsed "
                f"= {recon['merged'] + recon['cross']} normalized).\n")
    return top, dist, conf_dist, watch_rows


WATCH_TABLE = """
CREATE TABLE IF NOT EXISTS watch_flags (
    opportunity_id INTEGER REFERENCES opportunities(id),
    entry          TEXT,
    run_stamp      TEXT,
    PRIMARY KEY (opportunity_id, entry, run_stamp)
);
"""


def main():
    args = sys.argv[1:]
    db = 'output/grants/grants.db'
    corpus = 'output/grants/_corpus'
    watchlist_path = '.claude/skills/grant-scout/watchlist.md'
    min_deadline = '2026-07-01'
    report = None
    limit = None
    rescore_all = False
    include_undated = True
    # `opportunities` is cumulative across runs, so a record from an April crawl with a
    # 2027 close date still counts as "active" long after it left the corpus. Scoring the
    # CURRENT corpus means filtering on the last_seen stamp this run's db_ingest wrote.
    seen_since = None
    i = 0
    while i < len(args):
        if args[i] == '--db': db = args[i+1]; i += 2
        elif args[i] == '--corpus': corpus = args[i+1]; i += 2
        elif args[i] == '--watchlist': watchlist_path = args[i+1]; i += 2
        elif args[i] == '--min-deadline': min_deadline = args[i+1]; i += 2
        elif args[i] == '--report': report = args[i+1]; i += 2
        elif args[i] == '--limit': limit = int(args[i+1]); i += 2
        elif args[i] == '--rescore-all': rescore_all = True; i += 1
        elif args[i] == '--seen-since': seen_since = args[i+1]; i += 2
        elif args[i] == '--exclude-undated': include_undated = False; i += 1
        else: i += 1
    if report is None:
        report = f"output/grants/_MASTER_{datetime.date.today().isoformat()}.md"

    watchlist = load_watchlist(watchlist_path)
    if not watchlist:
        print(f"[grade] WARNING: no watch-list entries loaded from {watchlist_path}",
              file=sys.stderr)
    else:
        print(f"[grade] watch-list: {', '.join(w for w, _ in watchlist)}")

    con = sqlite3.connect(db)
    con.row_factory = sqlite3.Row
    con.executescript(WATCH_TABLE)
    pv = con.execute("SELECT COALESCE(MAX(version),0) FROM profile_versions").fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')

    view = 'active_opportunities' if rescore_all else 'needs_grading'
    date_pred = ("(close_date IS NULL OR close_date='' OR close_date >= ?)"
                 if include_undated else "close_date IS NOT NULL AND close_date >= ?")
    params = [min_deadline]
    seen_pred = ''
    if seen_since:
        seen_pred = ' AND last_seen >= ?'
        params.append(seen_since)
    rows = con.execute(f"SELECT * FROM {view} WHERE {date_pred}{seen_pred} "
                       f"ORDER BY close_date", params).fetchall()
    if limit:
        rows = rows[:limit]
    print(f"[grade] scoring {len(rows)} opportunities from {view} "
          f"(deadline>={min_deadline}, undated={'in' if include_undated else 'out'}, profile v{pv})")

    graded = 0
    watch_hits = {}
    total = len(rows)
    for o in rows:
        o = dict(o)
        s = score_one(o, watchlist)
        con.execute("""INSERT INTO scores(opportunity_id,grade,pct,a1,a2,a3,a4,a5,confidence,profile_version,graded_at)
                       VALUES(?,?,?,?,?,?,?,?,?,?,?)
                       ON CONFLICT(opportunity_id) DO UPDATE SET
                         grade=excluded.grade,pct=excluded.pct,a1=excluded.a1,a2=excluded.a2,
                         a3=excluded.a3,a4=excluded.a4,a5=excluded.a5,confidence=excluded.confidence,
                         profile_version=excluded.profile_version,graded_at=excluded.graded_at""",
                    (o['id'], s['grade'], s['pct'], s['a1'], s['a2'], s['a3'], s['a4'], s['a5'],
                     s['confidence'], pv, now))
        con.execute("""INSERT INTO score_history(opportunity_id,grade,pct,a1,a2,a3,a4,a5,confidence,profile_version,graded_at)
                       VALUES(?,?,?,?,?,?,?,?,?,?,?)""",
                    (o['id'], s['grade'], s['pct'], s['a1'], s['a2'], s['a3'], s['a4'], s['a5'],
                     s['confidence'], pv, now))
        for w in s['watch']:
            con.execute("INSERT OR IGNORE INTO watch_flags(opportunity_id,entry,run_stamp) VALUES(?,?,?)",
                        (o['id'], w, now))
            watch_hits[w] = watch_hits.get(w, 0) + 1
        graded += 1
        if graded % 1000 == 0 or graded == total:
            print('##ITERATE-PROGRESS## ' + json.dumps(
                {"done": graded, "total": total, "message": "grading corpus"}), flush=True)
    con.commit()

    recon = reconciliation(corpus, con, min_deadline, graded, now, seen_since)
    top, dist, conf_dist, watch_rows = write_report(
        report, con, pv, min_deadline, now, recon, watchlist, include_undated)
    con.commit()
    con.close()
    print(f"[grade] graded={graded}  distribution={dist}")
    print(f"[grade] confidence={conf_dist}")
    print(f"[grade] watch-list hits={watch_hits or 'none'}")
    print(f"[grade] master report → {report}")


if __name__ == '__main__':
    main()
