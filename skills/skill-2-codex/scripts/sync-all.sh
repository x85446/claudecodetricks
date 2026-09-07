#!/usr/bin/env bash
set -uo pipefail

# ============================================================================
# sync-all.sh — port every Claude Code skill to Codex, safely and repeatedly
# ============================================================================
# Built to run unattended (daily cron) as well as by hand. The whole design
# question is: how do you regenerate automatically without destroying the
# hand-written judgment work that step 4 of the port requires?
#
# Answer: a port stamp per skill, at codex-skills/<name>/.portstamp, holding
#   src=<sha of every source file>   out=<sha of every generated file>
#
# On each run a skill lands in exactly one of four states:
#
#   fresh      no stamp yet                        -> convert
#   unchanged  src sha matches the stamp           -> skip, cost nothing
#   clean      src changed, out sha matches stamp   -> regenerate (no hand-edits
#                                                     to lose)
#   MANUAL     src changed AND out sha differs      -> DO NOT TOUCH. Someone
#              from the stamp                         hand-edited the Codex copy;
#                                                     overwriting would silently
#                                                     delete that work. Report it.
#
# That last state is the entire point. An automated porter that cannot tell
# "generated" from "generated then improved" will eventually eat the improvement.
#
# Usage: sync-all.sh [--install] [--force <skill>] [--only <skill>] [--quiet]
# ============================================================================

REPO="$HOME/workspace/x85446/claudecodetricks"
SRC_ROOT="$REPO/skills"
OUT_ROOT="$REPO/codex-skills"
INSTALL_ROOT="$HOME/.agents/skills"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DO_INSTALL=false; FORCE=""; ONLY=""; QUIET=false; PORT_ALL=false; PRUNE=false
MANIFEST_BUDGET=7600   # Codex's real cap is 8000; hold slack so adding a skill does not immediately overflow
while [[ $# -gt 0 ]]; do
    case "$1" in
        --install) DO_INSTALL=true; shift ;;
        --force)   FORCE="${2:?--force needs a skill name}"; shift 2 ;;
        --only)    ONLY="${2:?--only needs a skill name}"; shift 2 ;;
        --quiet)   QUIET=true; shift ;;
        --all)     PORT_ALL=true; shift ;;
        --prune)   PRUNE=true; shift ;;
        --budget)  MANIFEST_BUDGET="${2:?--budget needs a number}"; shift 2 ;;
        *) echo "unknown arg: $1" >&2; exit 2 ;;
    esac
done
say() { $QUIET || echo "$@"; }

# Skills that are Claude-Code plumbing with no Codex meaning, or that would be
# actively wrong to expose there. Listed explicitly so the exclusion is a
# decision on the record rather than an accident of discovery.
# Skills whose CONTENT is about authoring Claude Code skills. Porting them
# yields a Codex skill that teaches Claude Code's frontmatter and directory
# conventions -- confidently wrong advice inside Codex. They are also the
# source of every false-positive flag in the residual scan, because they
# *document* /loop, AskUserQuestion and disable-model-invocation rather than
# using them.
declare -a SKIP_SKILLS=(skill-2-codex skill-builder skill-maker)

# Metas whose children should be reachable through the meta, not advertised
# individually in the manifest. Codex charges description budget only for
# implicitly-invocable skills, and -- unlike Claude Code -- turning implicit
# invocation off does NOT block delegation, because `$child` explicit
# invocation still resolves. So the family convention that Claude Code can only
# recommend, Codex can actually enforce, for free.
#
# The iterate stack is deliberately absent: it is a peer pipeline (notes ->
# brainstorm -> plan -> execute), not a meta with workers, and each stage is a
# front door the user routes to by natural language.
declare -a FOLD_CHILDREN_OF=(testmaster uxmaster codeconverter categorize importer downloader auditor)

EXT_CONF="$(dirname "$HERE")/external-sources.conf"
ext_source_for() {
    # Path to an externally-owned skill's canonical directory, or empty.
    [[ -f "$EXT_CONF" ]] || return 0
    local want="$1" n p
    while read -r n p; do
        [[ -z "$n" || "$n" == \#* ]] && continue
        [[ "$n" == "$want" ]] || continue
        echo "${p/#\~/$HOME}"; return 0
    done < "$EXT_CONF"
}

sha_of() {
    # Digest of a directory's content, independent of where that directory
    # lives. shasum prints the path alongside the hash, so digesting absolute
    # paths makes the same content hash differently depending on the caller's
    # cwd -- which silently breaks every stamp comparison. Strip to paths
    # relative to the directory itself.
    local dir="$1"
    [[ -d "$dir" ]] || { echo "absent"; return; }
    ( cd "$dir" && find . -type f ! -name '.portstamp' -print0 2>/dev/null \
        | LC_ALL=C sort -z | xargs -0 shasum 2>/dev/null | shasum | cut -d' ' -f1 )
}

# Which skills to port. Default: mirror what is actually installed globally for
# Claude Code (~/.claude/skills), NOT the whole backup repo. The repo is the
# canonical backup for every project, so it holds project-scoped skills that
# were never meant to be globally available -- porting those would hand Codex a
# larger global surface than Claude Code itself has. --all overrides.
CLAUDE_GLOBAL="$HOME/.claude/skills"
if $PORT_ALL || [[ ! -d "$CLAUDE_GLOBAL" ]]; then
    mapfile -t ALL < <(find "$SRC_ROOT" -maxdepth 1 -mindepth 1 -type d -exec basename {} \; | LC_ALL=C sort)
else
    mapfile -t ALL < <(find "$CLAUDE_GLOBAL" -maxdepth 1 -mindepth 1 -type d -exec basename {} \; | LC_ALL=C sort)
fi
ALL_CSV="$(IFS=,; echo "${ALL[*]}")"

declare -a R_CONVERTED=() R_SKIPPED=() R_MANUAL=() R_FAILED=() NO_BACKUP=() EXTERNAL=() BROKEN=()
declare -A STAMP_SRC=()
declare -a FLAGGED=()

for name in "${ALL[@]}"; do
    [[ -n "$ONLY" && "$name" != "$ONLY" ]] && continue
    for s in "${SKIP_SKILLS[@]:-}"; do [[ "$name" == "$s" ]] && continue 2; done
    # Source lookup mirrors the skill's own step-1 order: canonical backup
    # first, then the live global install. A skill can be installed globally
    # with no backup in the repo (izmachine is, today) -- reading only the repo
    # would silently skip it, which is exactly the kind of quiet gap this whole
    # sync exists to prevent.
    src="$SRC_ROOT/$name"
    if [[ ! -f "$src/SKILL.md" ]]; then
        ext="$(ext_source_for "$name")"
        if [[ -n "$ext" && -f "$ext/SKILL.md" ]]; then
            src="$ext"
            EXTERNAL+=("$name")
        elif [[ -f "$CLAUDE_GLOBAL/$name/SKILL.md" ]]; then
            # Last resort. The global install is a COPY of whatever its owning
            # repo last pushed there, so porting from it silently inherits any
            # staleness. Reported loudly rather than treated as normal.
            src="$CLAUDE_GLOBAL/$name"
            NO_BACKUP+=("$name")
        else
            continue
        fi
    fi

    codex_name="${name#-}"          # Codex has no orphan "-" convention
    out="$OUT_ROOT/$codex_name"
    stamp="$out/.portstamp"

    src_sha="$(sha_of "$src")"
    if [[ -f "$stamp" && "$name" != "$FORCE" ]]; then
        old_src="$(sed -n 's/^src=//p' "$stamp")"
        old_out="$(sed -n 's/^out=//p' "$stamp")"
        is_manual="$(sed -n 's/^manual=//p' "$stamp")"
        cur_out="$(sha_of "$out")"
        # Source untouched: nothing to do, at any cost.
        if [[ "$src_sha" == "$old_src" ]]; then
            R_SKIPPED+=("$codex_name"); continue
        fi
        # Source moved, but this port carries a deliberate judgment pass
        # (manual=true) or has been edited since it was stamped. Either way the
        # Codex copy holds work the converter cannot reproduce -- regenerating
        # would delete it silently. Report and leave it alone; --force <skill>
        # is the deliberate override once the work has been re-done or is known
        # to be disposable.
        if [[ "$is_manual" == "true" || "$cur_out" != "$old_out" ]]; then
            R_MANUAL+=("$codex_name"); continue
        fi
    fi

    rm -rf "$out"
    if ! "$HERE/scaffold.sh" "$src" "$out" >/dev/null 2>&1; then
        R_FAILED+=("$codex_name"); continue
    fi
    json="$(python3 "$HERE/convert.py" "$out" --all-skills="$ALL_CSV" \
              --source="$src" --ported="$ALL_CSV" --json 2>&1)" || { R_FAILED+=("$codex_name"); continue; }
    n_flags="$(python3 -c 'import json,sys;print(len(json.loads(sys.argv[1])["flags"]))' "$json" 2>/dev/null || echo 0)"
    [[ "$n_flags" != "0" ]] && FLAGGED+=("$codex_name:$n_flags")

    # The stamp is written in a later pass, AFTER child-folding has finished
    # touching the output. Stamping here would record an out-digest that the
    # folding step immediately invalidates, and every subsequent run would then
    # read the skill as hand-edited and refuse to regenerate it.
    STAMP_SRC["$codex_name"]="$src_sha"
    R_CONVERTED+=("$codex_name")
done

# --- fold children into their meta (manifest budget) ------------------------
declare -a FOLDED=()
for name in "${ALL[@]}"; do
    codex_name="${name#-}"
    out="$OUT_ROOT/$codex_name"
    [[ -d "$out" ]] || continue
    [[ -f "$out/agents/openai.yaml" ]] && continue     # already explicit-only
    for meta in "${FOLD_CHILDREN_OF[@]}"; do
        if [[ "$codex_name" == "$meta"-* ]]; then
            mkdir -p "$out/agents"
            printf 'policy:\n  allow_implicit_invocation: false\n' > "$out/agents/openai.yaml"
            FOLDED+=("$codex_name")
            break
        fi
    done
done

# --- fit the startup manifest ----------------------------------------------
# Codex charges name+description of every implicitly-invocable skill against a
# 2%-of-context / 8,000-char budget, half of Claude Code's. Descriptions written
# for the larger budget carry material that was never routing signal; diet.py
# moves that down a level into the body (loaded on trigger) and never touches a
# trigger phrase. Runs BEFORE stamping so the relocation is part of the
# generated output, not a change that later reads as a hand-edit.
DIET_OUT=""
if [[ -d "$OUT_ROOT" ]]; then
    DIET_OUT="$(python3 "$HERE/diet.py" "$OUT_ROOT" --budget "$MANIFEST_BUDGET" --apply 2>&1 | head -2)"
fi

# --- stamp, now that the output is final ------------------------------------
for codex_name in "${R_MANUAL[@]:-}" "${R_SKIPPED[@]:-}"; do
    [[ -z "$codex_name" ]] && continue
    st="$OUT_ROOT/$codex_name/.portstamp"
    [[ -f "$st" ]] || continue
    grep -q '^manual=true' "$st" || continue
    # keep src= and manual=, refresh out= to absorb this run's diet pass
    { sed -n 's/^\(src=.*\)/\1/p' "$st"; echo "out=$(sha_of "$OUT_ROOT/$codex_name")"; echo "manual=true"; } \
        > "$st.new" && mv "$st.new" "$st"
done
for codex_name in "${!STAMP_SRC[@]}"; do
    out="$OUT_ROOT/$codex_name"
    [[ -d "$out" ]] || continue
    { echo "src=${STAMP_SRC[$codex_name]}"
      echo "out=$(sha_of "$out")"
      echo "manual=false"; } > "$out/.portstamp"
done

if $DO_INSTALL; then
    mkdir -p "$INSTALL_ROOT"
    # Remove installed Codex skills that are no longer in scope -- a skill
    # uninstalled from Claude Code global should not linger in Codex, quietly
    # spending manifest budget nothing maintains any more.
    if $PRUNE; then
        # The generated tree gets pruned too, not just the install -- otherwise
        # a narrowed scope leaves stale build output behind that looks current.
        for d in "$OUT_ROOT"/*/; do
            [[ -d "$d" ]] || continue
            b="$(basename "$d")"; keep=false
            for n in "${ALL[@]}"; do [[ "${n#-}" == "$b" ]] && keep=true && break; done
            $keep || { rm -rf "$d"; say "pruned stale build output: $b"; }
        done
        for d in "$INSTALL_ROOT"/*/; do
            [[ -d "$d" ]] || continue
            b="$(basename "$d")"; keep=false
            for n in "${ALL[@]}"; do [[ "${n#-}" == "$b" ]] && keep=true && break; done
            $keep || { rm -rf "$d"; say "pruned stale install: $b"; }
        done
    fi
    # Install by atomic rename, never by rm-then-copy.
    #
    # `rm -rf dst && cp -R src dst` leaves a window -- the whole duration of the
    # copy -- in which dst EXISTS as a directory but its SKILL.md does not. A
    # harness scanning the skills root during that window does not see "no
    # skill"; it sees a malformed one, and reports
    #   "failed to read file: No such file or directory".
    # Observed live on ~/.agents/skills/i. The window is milliseconds per skill,
    # but this loop runs over 40+ of them and the daily job runs unattended
    # while sessions are open, so it is hit eventually.
    #
    # Staging lives OUTSIDE the scanned root, so a half-copied directory is
    # never visible there at all. The two renames are atomic on the same
    # filesystem: the only transient state a scanner can observe is dst briefly
    # absent, which is a skill that isn't there yet -- harmless -- rather than
    # one that is there and broken.
    STAGE="$HOME/.agents/.skill-staging"
    rm -rf "$STAGE"; mkdir -p "$STAGE"
    trap 'rm -rf "$STAGE"' EXIT
    for n in "${R_CONVERTED[@]:-}" "${R_MANUAL[@]:-}" "${R_SKIPPED[@]:-}"; do
        [[ -z "$n" ]] && continue
        [[ -d "$OUT_ROOT/$n" ]] || continue
        rm -rf "${STAGE:?}/$n" "${STAGE:?}/$n.old"
        cp -R "$OUT_ROOT/$n" "$STAGE/$n" || { R_FAILED+=("$n"); continue; }
        rm -f "$STAGE/$n/.portstamp"
        # Never publish a directory without its SKILL.md -- that is the exact
        # state this whole dance exists to avoid.
        [[ -f "$STAGE/$n/SKILL.md" ]] || { R_FAILED+=("$n"); rm -rf "${STAGE:?}/$n"; continue; }
        [[ -e "$INSTALL_ROOT/$n" ]] && mv "$INSTALL_ROOT/$n" "$STAGE/$n.old"
        mv "$STAGE/$n" "$INSTALL_ROOT/$n"
        rm -rf "${STAGE:?}/$n.old"
    done
    rm -rf "$STAGE"; trap - EXIT

    # Post-install integrity: every installed directory must hold a readable
    # SKILL.md. Cheap, and it is the check that would have caught the race
    # instead of a harness warning doing it for us.
    for d in "$INSTALL_ROOT"/*/; do
        [[ -d "$d" ]] || continue
        [[ -r "$d/SKILL.md" ]] || BROKEN+=("$(basename "$d")")
    done
fi

say "converted:  ${#R_CONVERTED[@]}"
say "unchanged:  ${#R_SKIPPED[@]}"
say "source changed, port protected (NOT overwritten): ${#R_MANUAL[@]} ${R_MANUAL[*]:-}"
say "failed:     ${#R_FAILED[@]} ${R_FAILED[*]:-}"
if [[ ${#EXTERNAL[@]} -gt 0 ]]; then
    say "ported from their owning repo (external-sources.conf): ${EXTERNAL[*]}"
fi
if [[ ${#NO_BACKUP[@]} -gt 0 ]]; then
    say "WARNING — no canonical source, ported from the live global install: ${NO_BACKUP[*]}"
    say "  that install is a copy and may be stale; add an entry to external-sources.conf"
fi
say "folded into their meta (explicit-only, free): ${#FOLDED[@]}"
[[ -n "$DIET_OUT" ]] && say "manifest: $DIET_OUT"
say "needing a judgment pass: ${#FLAGGED[@]} ${FLAGGED[*]:-}"
$DO_INSTALL && say "installed to $INSTALL_ROOT"
if [[ ${#BROKEN[@]} -gt 0 ]]; then
    say "BROKEN INSTALL — directory present with no readable SKILL.md: ${BROKEN[*]}"
fi

# Non-zero only for real breakage. A skill needing a judgment pass, or one
# protected from overwrite, is normal steady-state -- not a failure the cron
# job should alarm on every single night.
[[ ${#R_FAILED[@]} -eq 0 && ${#BROKEN[@]} -eq 0 ]]
