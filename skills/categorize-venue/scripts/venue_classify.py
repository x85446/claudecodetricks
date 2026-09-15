#!/usr/bin/env python3
"""Venue classifier helper for personaldb.

Mechanical half of the venue-classifier skill: lists unclassified merchants
(with sample transaction items for context), validates and applies
classification JSON produced by Claude, and reports coverage. The actual
classification (merchant name -> venue type) is done by Claude in the skill
workflow, knowledge-first with WebSearch fallback.

Usage:
  venue_classify.py list  [--db PATH] [--years 2024,2025,2026] [--limit N]
  venue_classify.py apply [--db PATH] --json FILE [--dry-run] [--source-default claude]
  venue_classify.py stats [--db PATH] [--years 2024,2025,2026]

apply JSON format: [{"id": 123, "venue_type": "coffee shop",
                     "source": "claude", "confidence": 0.95}, ...]
Only rows with venue_type IS NULL are ever written (idempotent).
"""
import argparse, json, os, sqlite3, sys

DEFAULT_DB = "db/personaldb.sqlite"
MAP_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "classification_map.json")


def load_map():
    with open(MAP_PATH) as f:
        m = json.load(f)
    return set(m["venue_types"].keys()), set(m["sources"])


def year_filter(years):
    if not years:
        return "", []
    ys = [int(y) for y in years.split(",")]
    q = f"""AND m.id IN (SELECT DISTINCT merchant_id FROM transactions
             WHERE merchant_id IS NOT NULL AND year IN ({','.join('?'*len(ys))}))"""
    return q, ys


def cmd_list(con, years, limit):
    q, ps = year_filter(years)
    rows = con.execute(f"""
        SELECT m.id, m.name,
               (SELECT COUNT(*) FROM transactions t WHERE t.merchant_id=m.id) AS txns,
               (SELECT GROUP_CONCAT(DISTINCT site) FROM transactions t WHERE t.merchant_id=m.id) AS sites,
               COALESCE((SELECT t.item FROM transactions t WHERE t.merchant_id=m.id LIMIT 1),'') AS sample
        FROM merchants m WHERE m.venue_type IS NULL {q}
        ORDER BY txns DESC LIMIT ?""", ps + [limit]).fetchall()
    for r in rows:
        print(f"{r[0]}\t{r[1]}\t{r[2]} txns\t{r[3] or '-'}\t{(r[4] or '')[:80]}")
    print(f"# {len(rows)} unclassified merchants listed", file=sys.stderr)


def cmd_apply(con, json_path, dry_run, source_default):
    vtypes, sources = load_map()
    with open(json_path) as f:
        entries = json.load(f)
    errors, applied, skipped = [], 0, 0
    for e in entries:
        vid, vt = e.get("id"), e.get("venue_type")
        src = e.get("source", source_default)
        conf = e.get("confidence", 0.9)
        if vt not in vtypes:
            errors.append(f"id {vid}: venue_type '{vt}' not in classification_map.json")
            continue
        if src not in sources:
            errors.append(f"id {vid}: source '{src}' invalid")
            continue
        if not (0.0 <= float(conf) <= 1.0):
            errors.append(f"id {vid}: confidence {conf} out of range")
            continue
        row = con.execute("SELECT venue_type FROM merchants WHERE id=?", (vid,)).fetchone()
        if row is None:
            errors.append(f"id {vid}: merchant does not exist")
            continue
        if row[0] is not None:
            skipped += 1  # already classified — idempotency guard
            continue
        if not dry_run:
            con.execute("""UPDATE merchants SET venue_type=?, venue_type_source=?,
                           venue_type_confidence=? WHERE id=? AND venue_type IS NULL""",
                        (vt, src, float(conf), vid))
        applied += 1
    if errors:
        for err in errors:
            print(f"ERROR: {err}", file=sys.stderr)
    if not dry_run:
        con.commit()
    mode = "DRY-RUN (no writes)" if dry_run else "APPLIED"
    print(f"{mode}: {applied} classified, {skipped} skipped (already set), {len(errors)} errors")
    return 1 if errors else 0


def cmd_stats(con, years):
    q, ps = year_filter(years)
    total = con.execute(f"SELECT COUNT(*) FROM merchants m WHERE 1=1 {q}", ps).fetchone()[0]
    done = con.execute(f"SELECT COUNT(*) FROM merchants m WHERE m.venue_type IS NOT NULL {q}", ps).fetchone()[0]
    print(f"classified {done}/{total} ({'%.1f' % (100.0*done/total) if total else 0}%)")
    for vt, n in con.execute(f"""SELECT m.venue_type, COUNT(*) FROM merchants m
                                 WHERE m.venue_type IS NOT NULL {q}
                                 GROUP BY m.venue_type ORDER BY 2 DESC""", ps):
        print(f"  {vt}: {n}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("cmd", choices=["list", "apply", "stats"])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--years", default=None, help="comma-separated, e.g. 2024,2025,2026")
    ap.add_argument("--limit", type=int, default=100)
    ap.add_argument("--json", dest="json_path")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--source-default", default="claude")
    a = ap.parse_args()
    con = sqlite3.connect(a.db)
    if a.cmd == "list":
        cmd_list(con, a.years, a.limit)
    elif a.cmd == "apply":
        if not a.json_path:
            ap.error("apply requires --json FILE")
        sys.exit(cmd_apply(con, a.json_path, a.dry_run, a.source_default))
    else:
        cmd_stats(con, a.years)


if __name__ == "__main__":
    main()
