#!/usr/bin/env bash
# weekly_refresh.sh — grant-scout weekly cadence.
#
# Chains the full self-improving loop:
#   1. refresh_profile.py  — re-learn Izuma capabilities from products/ + submitted/
#   2. fetch_all.sh --refresh-friendly — re-crawl all sources (file cache skips
#      unchanged detail pages; down sources like SBIR.gov/DSIP retry and fold in
#      automatically once their maintenance window ends)
#   3. db_ingest.py        — upsert the fresh corpus (history preserved)
#   4. grade_corpus.py     — re-grade everything still open with deadline >= today+
#
# Run from the project root. Designed to be invoked by the durable weekly schedule
# (Monday AM) or by hand. Idempotent and respectful (crawlers cache-skip + backoff).

set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"   # project root
cd "$ROOT" || exit 1
LK="$ROOT/.claude/skills/grant-scout/lookup"
TODAY="$(date -u +%Y-%m-%d)"

echo "[weekly] $(date -u +%FT%TZ) start in $ROOT"

echo "[weekly] 1/4 refresh_profile"
python3 "$LK/refresh_profile.py" || echo "[weekly] refresh_profile non-zero (continuing)"

echo "[weekly] 2/4 fetch_all (all sources; SBIR.gov + DSIP retry if recovered)"
"$LK/fetch_all.sh" || echo "[weekly] fetch_all non-zero (partial corpus; continuing)"

echo "[weekly] 3/4 db_ingest"
python3 "$LK/db_ingest.py" || { echo "[weekly] db_ingest FAILED"; exit 2; }

echo "[weekly] 4/4 grade_corpus (deadline >= today)"
python3 "$LK/grade_corpus.py" --min-deadline "$TODAY" --report "output/grants/_MASTER_${TODAY}.md"

echo "[weekly] $(date -u +%FT%TZ) done. Report: output/grants/_MASTER_${TODAY}.md"
