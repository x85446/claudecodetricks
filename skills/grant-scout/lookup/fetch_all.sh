#!/usr/bin/env bash
# fetch_all.sh — driver. Runs every adapter, then normalizes + merges into _all.json.
#
# Usage:
#   fetch_all.sh                 # all sources
#   fetch_all.sh sam             # one source
#   fetch_all.sh sam grants_gov  # subset
#
# Output: $PROJECT_ROOT/output/grants/_corpus/
#   <source>/raw/<id>.{json,html}
#   <source>/normalized.jsonl        — one record per line, unified schema
#   _all.json                        — merged + deduped across sources
#   _metadata.json                   — last-crawl timestamps + counts + errors per source

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Project root = the project we're invoked from (cwd), so the corpus lands under that project.
PROJECT_ROOT="$(pwd)"
CORPUS="${PROJECT_ROOT}/output/grants/_corpus"
mkdir -p "$CORPUS"

ALL_SOURCES=(sam darpa sbir grants_gov)
SOURCES=("$@")
[ ${#SOURCES[@]} -eq 0 ] && SOURCES=("${ALL_SOURCES[@]}")

# Map short crawler-name → long corpus-dir name (must match normalize.py/merge.py)
declare -A SRC_DIR=(
    [sam]=sam.gov
    [darpa]=darpa.mil
    [sbir]=sbir.gov
    [grants_gov]=grants.gov
)

START_TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "[fetch_all] start ${START_TS} — sources: ${SOURCES[*]}"
echo "[fetch_all] corpus: ${CORPUS}"

declare -A STATUS
declare -A COUNT

for src in "${SOURCES[@]}"; do
    script="${SCRIPT_DIR}/crawl_${src}.sh"
    dir="${SRC_DIR[$src]:-$src}"
    if [ ! -x "$script" ]; then
        echo "[fetch_all] SKIP ${src}: ${script} not executable"
        STATUS[$src]="skipped"
        continue
    fi
    echo "----- ${src} (→ ${dir}) -----"
    if "$script" "$CORPUS/$dir"; then
        STATUS[$src]="ok"
        if [ -f "$CORPUS/$dir/_count" ]; then
            COUNT[$src]=$(cat "$CORPUS/$dir/_count")
        else
            COUNT[$src]=$(ls "$CORPUS/$dir/raw/" 2>/dev/null | wc -l | tr -d ' ')
        fi
    else
        STATUS[$src]="error"
    fi
done

echo "----- normalize -----"
python3 "${SCRIPT_DIR}/normalize.py" "$CORPUS" "${SOURCES[@]}"

echo "----- merge -----"
python3 "${SCRIPT_DIR}/merge.py" "$CORPUS"

END_TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
python3 - "$CORPUS" "$START_TS" "$END_TS" "${SOURCES[@]}" <<PY
import json, sys, os
corpus, start, end = sys.argv[1], sys.argv[2], sys.argv[3]
sources = sys.argv[4:]
SRC_DIR = {"sam":"sam.gov","darpa":"darpa.mil","sbir":"sbir.gov","grants_gov":"grants.gov"}
meta_path = os.path.join(corpus, "_metadata.json")
meta = {}
if os.path.exists(meta_path):
    try: meta = json.load(open(meta_path))
    except Exception: meta = {}
meta["last_run_start"] = start
meta["last_run_end"] = end
src_meta = meta.setdefault("sources", {})
for s in sources:
    d = SRC_DIR.get(s, s)
    cnt_path = os.path.join(corpus, d, "_count")
    n = None
    if os.path.exists(cnt_path):
        try: n = int(open(cnt_path).read().strip())
        except Exception: pass
    if n is None:
        raw_dir = os.path.join(corpus, d, "raw")
        if os.path.isdir(raw_dir):
            n = len(os.listdir(raw_dir))
    src_meta[s] = {"last_crawl_utc": end, "count": n, "dir": d}
# total in _all.json
all_path = os.path.join(corpus, "_all.json")
total = None
if os.path.exists(all_path):
    try: total = len(json.load(open(all_path)))
    except Exception: pass
meta["all_count"] = total
json.dump(meta, open(meta_path, "w"), indent=2)
print(f"[fetch_all] _metadata.json written; total in _all.json = {total}")
PY

echo "[fetch_all] done ${END_TS}"
