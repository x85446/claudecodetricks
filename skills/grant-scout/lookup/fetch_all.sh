#!/usr/bin/env bash
# fetch_all.sh — driver. Runs every registered adapter, then normalizes + merges
# into _all.json.
#
# Usage:
#   fetch_all.sh                 # every source in sources.json
#   fetch_all.sh sam             # one source (registry 'short' name)
#   fetch_all.sh sam grants_gov  # subset
#
# Output: $PROJECT_ROOT/output/grants/_corpus/
#   <source>/raw/<id>.{json,html}
#   <source>/normalized.jsonl        — one record per line, unified schema
#   _all.json                        — merged + deduped across sources
#   _metadata.json                   — last-run timestamps + per-source outcome/count/http
#
# The source list, its corpus directory, and its crawler script are all read from
# sources.json — nothing here is hard-coded per source. A source with no crawler
# registered yet (challenge_gov, nspires, diu as of this writing) is driven the
# moment its crawl_<short>.sh lands; until then it is reported NO-CRAWLER, not
# silently skipped.
#
# A registry entry may also carry:
#   "crawler_args": [...]   extra argv appended after <dest> on every invocation
#                            (e.g. xtech's ["--refresh"], forcing a full refresh
#                            every run so its raw-file mtimes can't go stale).
#   "enrich": "<script>"    a required second-pass script run right after a
#                            successful crawl, same calling convention
#                            (<script> <dest>) (e.g. dsip's enrich_dsip.sh, which
#                            pulls the Q&A feed + attachment PDFs). Its failure
#                            fails the source.
#
# Exit code: non-zero if ANY requested source ends this run with zero stored
# records (no crawler, crawler failed, or crawler ran but returned nothing) or if
# normalize/merge themselves fail. A source that cannot be fetched must fail loud,
# never look merely "unchanged".

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Project root = the project we're invoked from (cwd), so the corpus lands under that project.
PROJECT_ROOT="$(pwd)"
CORPUS="${PROJECT_ROOT}/output/grants/_corpus"
mkdir -p "$CORPUS"

REGISTRY="${SOURCES_REGISTRY:-${SCRIPT_DIR}/sources.json}"
if [ ! -f "$REGISTRY" ]; then
    echo "[fetch_all] FATAL: registry not found: $REGISTRY" >&2
    exit 2
fi

# ---- load the registry into bash arrays/maps (short<FS>dir<FS>crawler<FS>enrich<FS>args) ----
# Field separator is \x01, NOT a tab. Bash's `read` treats every whitespace
# character in IFS (tab included, no matter what you set IFS to) as collapsible —
# consecutive tab delimiters merge into one and silently shift later fields left.
# That is invisible with trailing-only empty fields (the original 3-column form)
# but corrupts any row with an empty field in the MIDDLE (e.g. enrich="" followed
# by a non-empty crawler_args) — observed live: xtech's empty `enrich` collapsed
# away and its `--refresh` arg was read into $enrich instead of $args. \x01 is not
# whitespace, so `read` neither collapses it nor trims it.
REGISTRY_TSV="$(mktemp)"
trap 'rm -f "$REGISTRY_TSV"' EXIT

python3 - "$REGISTRY" > "$REGISTRY_TSV" <<'PY'
import json, sys
FS = "\x01"
reg = json.load(open(sys.argv[1]))
for s in reg["sources"]:
    short = s.get("short") or ""
    name = s.get("name") or ""
    crawler = s.get("crawler") or ""
    enrich = s.get("enrich") or ""
    args = " ".join(s.get("crawler_args") or [])
    if not short or not name:
        continue
    print(FS.join([short, name, crawler, enrich, args]))
PY

ALL_SOURCES=()
declare -A SRC_DIR
declare -A SRC_CRAWLER
declare -A SRC_ENRICH
declare -A SRC_ARGS
while IFS=$'\x01' read -r short name crawler enrich args; do
    [ -z "$short" ] && continue
    ALL_SOURCES+=("$short")
    SRC_DIR["$short"]="$name"
    SRC_CRAWLER["$short"]="$crawler"
    SRC_ENRICH["$short"]="$enrich"
    SRC_ARGS["$short"]="$args"
done < "$REGISTRY_TSV"

if [ ${#ALL_SOURCES[@]} -eq 0 ]; then
    echo "[fetch_all] FATAL: registry parsed but named zero sources" >&2
    exit 2
fi

SOURCES=("$@")
[ ${#SOURCES[@]} -eq 0 ] && SOURCES=("${ALL_SOURCES[@]}")

START_TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "[fetch_all] start ${START_TS} — sources: ${SOURCES[*]}"
echo "[fetch_all] corpus: ${CORPUS}"

declare -A STATUS      # ok | error | no-crawler | crawler-missing | unknown-source
declare -A COUNT       # records stored in raw/ after this run
declare -A HTTP        # best-effort HTTP/outcome code for this run

# Best-effort HTTP-status extraction from a crawler's own stdout/stderr. Crawlers
# are not required to emit a machine-readable status; when one doesn't, we fall
# back to an honest derived label rather than fabricating a code.
http_code_from_log() {
    local log="$1" rc="$2" n="$3"
    local code
    code=$(grep -Eoi 'http[^0-9]{0,12}[1-5][0-9]{2}\b' "$log" 2>/dev/null \
           | grep -Eo '[1-5][0-9]{2}' | tail -1)
    if [ -n "$code" ]; then
        echo "$code"
    elif [ "$rc" -eq 0 ] && [ "${n:-0}" -gt 0 ] 2>/dev/null; then
        echo "200(assumed)"
    elif [ "$rc" -ne 0 ]; then
        echo "ERR(exit=$rc)"
    else
        echo "200(0-records)"
    fi
}

for src in "${SOURCES[@]}"; do
    dir="${SRC_DIR[$src]:-}"
    if [ -z "$dir" ]; then
        echo "----- ${src} -----"
        echo "[fetch_all] ${src}: UNKNOWN — not in sources.json registry"
        STATUS[$src]="unknown-source"
        COUNT[$src]=0
        HTTP[$src]="n/a"
        continue
    fi

    dest="$CORPUS/$dir"
    mkdir -p "$dest"
    crawler_name="${SRC_CRAWLER[$src]:-}"
    echo "----- ${src} (→ ${dir}) -----"

    if [ -z "$crawler_name" ]; then
        echo "[fetch_all] ${src}: NO CRAWLER REGISTERED in sources.json"
        STATUS[$src]="no-crawler"
        COUNT[$src]=0
        HTTP[$src]="n/a"
        continue
    fi

    script="${SCRIPT_DIR}/${crawler_name}"
    if [ ! -x "$script" ]; then
        echo "[fetch_all] ${src}: crawler '${crawler_name}' not found or not executable at ${script}"
        STATUS[$src]="crawler-missing"
        COUNT[$src]=0
        HTTP[$src]="n/a"
        continue
    fi

    log="$dest/_fetch_stdout.log"
    # shellcheck disable=SC2086 — SRC_ARGS is a small, registry-controlled flag list (e.g. "--refresh")
    "$script" "$dest" ${SRC_ARGS[$src]:-} > "$log" 2>&1
    rc=$?
    cat "$log"

    # Optional second-pass enrichment (e.g. DSIP's Q&A + attachment fetch). Runs only after a
    # successful crawl, and its failure fails the source: a source whose enrichment silently
    # never runs is exactly the "quietly serving old data" failure mode this driver exists to
    # prevent (DSIP's qa_count/qa_text would otherwise never populate from fetch_all at all).
    enrich_name="${SRC_ENRICH[$src]:-}"
    if [ "$rc" -eq 0 ] && [ -n "$enrich_name" ]; then
        enrich_script="${SCRIPT_DIR}/${enrich_name}"
        if [ -x "$enrich_script" ]; then
            elog="$dest/_enrich_stdout.log"
            "$enrich_script" "$dest" > "$elog" 2>&1
            erc=$?
            cat "$elog"
            if [ "$erc" -ne 0 ]; then
                echo "[fetch_all] ${src}: enrich '${enrich_name}' exited ${erc}"
                rc=$erc
            fi
        else
            echo "[fetch_all] ${src}: enrich '${enrich_name}' not found or not executable at ${enrich_script}"
            rc=1
        fi
    fi

    if [ -f "$dest/_count" ]; then
        n=$(tr -d ' \n' < "$dest/_count" 2>/dev/null)
    else
        n=$(ls "$dest/raw/" 2>/dev/null | wc -l | tr -d ' ')
    fi
    n="${n:-0}"
    case "$n" in ''|*[!0-9]*) n=0 ;; esac

    HTTP[$src]="$(http_code_from_log "$log" "$rc" "$n")"
    COUNT[$src]="$n"
    if [ "$rc" -eq 0 ] && [ "$n" -gt 0 ]; then
        STATUS[$src]="ok"
    elif [ "$rc" -eq 0 ]; then
        STATUS[$src]="empty"
        echo "[fetch_all] ${src}: crawler exited 0 but stored 0 records — treating as a failure"
    else
        STATUS[$src]="error"
        echo "[fetch_all] ${src}: crawler exited ${rc}"
    fi
done

echo "----- normalize -----"
python3 "${SCRIPT_DIR}/normalize.py" "$CORPUS" "${SOURCES[@]}"
NORM_RC=$?

echo "----- merge -----"
python3 "${SCRIPT_DIR}/merge.py" "$CORPUS"
MERGE_RC=$?

END_TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# ---- write per-source outcome/count/http into _metadata.json ----
META_TSV="$(mktemp)"
for src in "${SOURCES[@]}"; do
    printf '%s\t%s\t%s\t%s\t%s\n' "$src" "${SRC_DIR[$src]:-}" "${STATUS[$src]:-unknown}" \
        "${COUNT[$src]:-0}" "${HTTP[$src]:-n/a}" >> "$META_TSV"
done

python3 - "$CORPUS" "$START_TS" "$END_TS" "$META_TSV" <<'PY'
import json, os, sys

corpus, start, end, meta_tsv = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]

meta_path = os.path.join(corpus, "_metadata.json")
meta = {}
if os.path.exists(meta_path):
    try:
        meta = json.load(open(meta_path))
    except Exception:
        meta = {}

meta["last_run_start"] = start
meta["last_run_end"] = end
src_meta = meta.setdefault("sources", {})

rows = []
with open(meta_tsv) as f:
    for line in f:
        parts = line.rstrip("\n").split("\t")
        if len(parts) != 5:
            continue
        rows.append(parts)

for short, d, status, count, http in rows:
    try:
        n = int(count)
    except Exception:
        n = 0
    entry = src_meta.setdefault(short, {})
    entry["dir"] = d
    entry["last_run_utc"] = end
    entry["last_run_status"] = status
    entry["last_run_count"] = n
    entry["last_run_http"] = http
    # Back-compat: db_ingest.py's sync_sources() reads a plain "count" key.
    entry["count"] = n
    # last_crawl_utc is retained for backward compatibility with other consumers
    # (db_ingest.py, grade_corpus.py) but is NEVER what the health report (Step 15)
    # uses for freshness — a re-normalize of cached raw files would refresh this
    # timestamp with no new fetch at all. The health report reads raw/ file mtimes.
    if status == "ok":
        entry["last_crawl_utc"] = end

all_path = os.path.join(corpus, "_all.json")
total = None
if os.path.exists(all_path):
    try:
        total = len(json.load(open(all_path)))
    except Exception:
        pass
meta["all_count"] = total
json.dump(meta, open(meta_path, "w"), indent=2)
print(f"[fetch_all] _metadata.json written; total in _all.json = {total}")
PY
rm -f "$META_TSV"

echo "===== fetch_all summary ====="
FAIL=0
for src in "${SOURCES[@]}"; do
    st="${STATUS[$src]:-unknown}"
    n="${COUNT[$src]:-0}"
    http="${HTTP[$src]:-n/a}"
    printf '  %-16s status=%-16s records=%-8s http=%s\n' "$src" "$st" "$n" "$http"
    if [ "$st" != "ok" ]; then
        FAIL=1
    fi
done
if [ "$NORM_RC" -ne 0 ]; then
    echo "[fetch_all] normalize.py exited ${NORM_RC}"
    FAIL=1
fi
if [ "$MERGE_RC" -ne 0 ]; then
    echo "[fetch_all] merge.py exited ${MERGE_RC}"
    FAIL=1
fi

echo "[fetch_all] done ${END_TS}"
exit $FAIL
