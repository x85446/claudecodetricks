#!/usr/bin/env python3
"""
db_ingest.py — load the merged corpus into the grant-scout SQLite DB.

Usage:
    db_ingest.py [--db output/grants/grants.db] [--corpus output/grants/_corpus]
                 [--sources .claude/skills/grant-scout/lookup/sources.json]

What it does:
  * Creates schema if absent (opportunities, scores, score_history,
    profile_versions, sources) + the active_opportunities / needs_grading views.
  * Upserts every record in <corpus>/_all.json into `opportunities`, keyed by a
    deterministic `uid` = solicitation_number (upper/trimmed) or "<source>:<source_id>".
    first_seen is set once on insert; last_seen + mutable fields refresh each run.
  * Mirrors the source registry (sources.json) + crawl metadata (_metadata.json)
    into the `sources` table.

Idempotent: re-running does NOT duplicate opportunities (uid UNIQUE) and NEVER
touches `score_history` (append-only; only the scoring step writes it).
"""
import json, os, sys, sqlite3, datetime, hashlib

SCHEMA = """
CREATE TABLE IF NOT EXISTS opportunities (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    uid                 TEXT UNIQUE NOT NULL,
    solicitation_number TEXT,
    source              TEXT,
    title               TEXT,
    agency              TEXT,
    subagency           TEXT,
    topic_text          TEXT,
    url                 TEXT,
    posted_date         TEXT,
    close_date          TEXT,
    phase               TEXT,
    eligibility         TEXT,
    funding_ceiling     REAL,
    first_seen          TEXT,
    last_seen           TEXT,
    raw_path            TEXT,
    -- Enrichment side-channels. These carry solicitation-wide text (one PDF shared by
    -- many topics) that normalize.py deliberately keeps OUT of topic_text. The scorer
    -- reads them as a separate, lower-weight signal; the *_chars counts are what lift a
    -- record off `confidence=low` when its own description is thin.
    solicitation_text   TEXT,
    solicitation_chars  INTEGER,
    instructions_text   TEXT,
    instructions_chars  INTEGER,
    qa_text             TEXT,
    qa_count            INTEGER
);
CREATE TABLE IF NOT EXISTS scores (
    opportunity_id  INTEGER PRIMARY KEY REFERENCES opportunities(id),
    grade           TEXT,
    pct             REAL,
    a1 REAL, a2 REAL, a3 REAL, a4 REAL, a5 REAL,
    confidence      TEXT,
    profile_version INTEGER,
    graded_at       TEXT
);
CREATE TABLE IF NOT EXISTS score_history (
    hist_id         INTEGER PRIMARY KEY AUTOINCREMENT,
    opportunity_id  INTEGER REFERENCES opportunities(id),
    grade           TEXT,
    pct             REAL,
    a1 REAL, a2 REAL, a3 REAL, a4 REAL, a5 REAL,
    confidence      TEXT,
    profile_version INTEGER,
    graded_at       TEXT
);
CREATE TABLE IF NOT EXISTS profile_versions (
    version       INTEGER PRIMARY KEY,
    captured_at   TEXT,
    snapshot_path TEXT,
    source_hash   TEXT,
    notes         TEXT
);
CREATE TABLE IF NOT EXISTS sources (
    name           TEXT PRIMARY KEY,
    base_url       TEXT,
    automatable    TEXT,
    reachable      TEXT,
    last_http      INTEGER,
    crawler        TEXT,
    last_crawl_utc TEXT,
    count          INTEGER,
    notes          TEXT
);
-- Still-open (or undated) opportunities only.
CREATE VIEW IF NOT EXISTS active_opportunities AS
    SELECT * FROM opportunities
    WHERE close_date IS NULL OR close_date = '' OR close_date >= date('now');
-- Active opportunities not yet scored at the current (max) profile version.
CREATE VIEW IF NOT EXISTS needs_grading AS
    SELECT o.* FROM active_opportunities o
    LEFT JOIN scores s ON s.opportunity_id = o.id
    WHERE s.opportunity_id IS NULL
       OR s.profile_version IS NULL
       OR s.profile_version < (SELECT COALESCE(MAX(version), 0) FROM profile_versions);
"""


# Columns added after the first schema shipped. CREATE TABLE IF NOT EXISTS is a no-op on
# an existing DB, so new columns have to be added explicitly.
ADD_COLUMNS = [
    ('opportunities', 'solicitation_text', 'TEXT'),
    ('opportunities', 'solicitation_chars', 'INTEGER'),
    ('opportunities', 'instructions_text', 'TEXT'),
    ('opportunities', 'instructions_chars', 'INTEGER'),
    ('opportunities', 'qa_text', 'TEXT'),
    ('opportunities', 'qa_count', 'INTEGER'),
]


def migrate(con):
    for table, col, decl in ADD_COLUMNS:
        have = {r[1] for r in con.execute(f"PRAGMA table_info({table})")}
        if col not in have:
            con.execute(f"ALTER TABLE {table} ADD COLUMN {col} {decl}")


def utcnow():
    return datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')


def scalar(v):
    """Coerce a value to a SQLite-bindable scalar (some source fields are dict/list)."""
    if v is None or isinstance(v, (str, int, float)):
        return v
    if isinstance(v, (dict, list)):
        return json.dumps(v, ensure_ascii=False)
    return str(v)


def uid_for(rec):
    sn = rec.get('solicitation_number')
    if isinstance(sn, str) and sn.strip():
        return sn.strip().upper()
    return f"{rec.get('source')}:{rec.get('source_id')}"


def ingest_opportunities(con, all_json):
    recs = json.load(open(all_json))
    now = utcnow()
    ins = upd = 0
    for r in recs:
        uid = uid_for(r)
        if not uid or uid.endswith(':None'):
            # No stable identity — skip rather than create junk rows.
            continue
        row = con.execute("SELECT id FROM opportunities WHERE uid=?", (uid,)).fetchone()
        fields = {k: scalar(v) for k, v in dict(
            solicitation_number=r.get('solicitation_number'),
            source=r.get('source'), title=r.get('title'),
            agency=r.get('agency'), subagency=r.get('subagency'),
            topic_text=r.get('topic_text'), url=r.get('url'),
            posted_date=r.get('posted_date'), close_date=r.get('close_date'),
            phase=r.get('phase'), eligibility=r.get('eligibility'),
            funding_ceiling=r.get('funding_ceiling'), raw_path=r.get('raw_path'),
            # Enrichment side-channels (present only on the sources that emit them).
            solicitation_text=r.get('solicitation_text'),
            solicitation_chars=r.get('solicitation_chars'),
            instructions_text=r.get('instructions_text'),
            instructions_chars=r.get('instructions_chars'),
            qa_text=r.get('qa_text'), qa_count=r.get('qa_count'),
        ).items()}
        if row:
            sets = ', '.join(f"{k}=?" for k in fields) + ", last_seen=?"
            con.execute(f"UPDATE opportunities SET {sets} WHERE uid=?",
                        (*fields.values(), now, uid))
            upd += 1
        else:
            cols = ', '.join(['uid'] + list(fields) + ['first_seen', 'last_seen'])
            qs = ', '.join('?' * (len(fields) + 3))
            con.execute(f"INSERT INTO opportunities ({cols}) VALUES ({qs})",
                        (uid, *fields.values(), now, now))
            ins += 1
    return ins, upd, len(recs)


def sync_sources(con, sources_json, metadata_json):
    reg = {}
    short_to_name = {}
    if os.path.exists(sources_json):
        for s in json.load(open(sources_json)).get('sources', []):
            reg[s['name']] = s
            if s.get('short'):
                short_to_name[s['short']] = s['name']
    meta = {}
    if os.path.exists(metadata_json):
        m = json.load(open(metadata_json))
        # map crawler-short-name -> {last_crawl_utc, count}. Built from sources.json
        # itself (short -> name), NOT a hand-maintained literal: a hardcoded subset
        # here previously covered only 6 of 10 registered sources (missing
        # grants_extract->"grants.gov.extract" and challenge_gov->"challenge.gov",
        # whose short/name spellings differ), so those two sources' rows in the
        # `sources` table silently never got last_crawl_utc/count populated even
        # after a fully successful crawl. Falling back to the short name itself
        # only matters if sources.json is missing entirely.
        smeta = m.get('sources', {})
        for short, info in smeta.items():
            meta[short_to_name.get(short, short)] = info
    for name, s in reg.items():
        mi = meta.get(name, {})
        con.execute("""INSERT INTO sources(name,base_url,automatable,reachable,last_http,crawler,last_crawl_utc,count,notes)
                       VALUES(?,?,?,?,?,?,?,?,?)
                       ON CONFLICT(name) DO UPDATE SET
                         base_url=excluded.base_url, automatable=excluded.automatable,
                         reachable=excluded.reachable, last_http=excluded.last_http,
                         crawler=excluded.crawler, last_crawl_utc=excluded.last_crawl_utc,
                         count=excluded.count, notes=excluded.notes""",
                    (name, s.get('base_url'), s.get('automatable'), s.get('reachable'),
                     s.get('last_http'), s.get('crawler'),
                     mi.get('last_crawl_utc'), mi.get('count'), s.get('notes')))


def main():
    args = sys.argv[1:]
    db = 'output/grants/grants.db'
    corpus = 'output/grants/_corpus'
    sources_json = '.claude/skills/grant-scout/lookup/sources.json'
    i = 0
    while i < len(args):
        if args[i] == '--db': db = args[i+1]; i += 2
        elif args[i] == '--corpus': corpus = args[i+1]; i += 2
        elif args[i] == '--sources': sources_json = args[i+1]; i += 2
        else: i += 1

    os.makedirs(os.path.dirname(db), exist_ok=True)
    con = sqlite3.connect(db)
    con.executescript(SCHEMA)
    migrate(con)

    all_json = os.path.join(corpus, '_all.json')
    if not os.path.exists(all_json):
        print(f"[db] no {all_json}; nothing to ingest", file=sys.stderr)
        con.commit(); con.close(); sys.exit(1)

    ins, upd, total = ingest_opportunities(con, all_json)
    sync_sources(con, sources_json, os.path.join(corpus, '_metadata.json'))
    con.commit()

    n_opp = con.execute("SELECT count(*) FROM opportunities").fetchone()[0]
    n_active = con.execute("SELECT count(*) FROM active_opportunities").fetchone()[0]
    n_hist = con.execute("SELECT count(*) FROM score_history").fetchone()[0]
    con.close()
    print(f"[db] ingested _all.json: {total} records → +{ins} new, {upd} updated")
    print(f"[db] opportunities={n_opp}  active={n_active}  score_history rows={n_hist} (untouched)")
    print(f"[db] {db}")


if __name__ == '__main__':
    main()
