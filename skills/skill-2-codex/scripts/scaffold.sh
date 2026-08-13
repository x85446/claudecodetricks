#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# scaffold.sh — mechanical half of the Claude-Code -> Codex skill conversion
# ============================================================================
# Handles the deterministic, repeatable part: extract name/description from
# a Claude Code SKILL.md's frontmatter, lay out the Codex directory shape
# (SKILL.md + references/ + scripts/ + assets/), and copy supporting files
# into their new homes. Everything else — rewriting Claude-tool references
# in the body into Codex-native phrasing, deciding on agents/openai.yaml —
# is a judgment call the calling model makes afterward using
# references/codex-format.md. This script only moves bytes and frontmatter
# fields it can extract with total confidence.
#
# Usage: scaffold.sh <source-skill-dir> <output-dir>
# ============================================================================

SRC="${1:?usage: scaffold.sh <source-skill-dir> <output-dir>}"
OUT="${2:?usage: scaffold.sh <source-skill-dir> <output-dir>}"

SRC_SKILL_MD="$SRC/SKILL.md"
[[ -f "$SRC_SKILL_MD" ]] || { echo "error: $SRC_SKILL_MD not found" >&2; exit 1; }

mkdir -p "$OUT"

# Normalize CRLF -> LF before anything else. Some source skill files carry
# CRLF line endings (Windows-edited or copy/pasted); a trailing \r breaks
# every ^...$ -anchored awk/grep match below in ways that are silent and
# confusing (fields quietly extract empty). Codex's own format doesn't
# want CRLF either, so normalizing once here is strictly correct, not a
# workaround.
NORMALIZED="$(mktemp)"
trap 'rm -f "$NORMALIZED"' EXIT
tr -d '\r' < "$SRC_SKILL_MD" > "$NORMALIZED"

# --- Extract frontmatter fields (single-line name:/description: only) -----
name=$(awk -F': *' '/^name:/{print $2; exit}' "$NORMALIZED")
description=$(awk -F': *' '/^description:/{sub(/^description: */,""); print; exit}' "$NORMALIZED")

had_disable_invocation=false
grep -q '^disable-model-invocation: *true' "$NORMALIZED" && had_disable_invocation=true

had_argument_hint=false
argument_hint=""
if grep -q '^argument-hint:' "$NORMALIZED"; then
    had_argument_hint=true
    argument_hint=$(awk -F': *' '/^argument-hint:/{sub(/^argument-hint: */,""); print; exit}' "$NORMALIZED")
fi

# Codex names: lowercase-hyphen, no leading hyphen (Claude Code's "orphan"
# skills lead with "-"; that convention has no meaning in Codex).
name="${name#-}"

# --- Body: everything after the closing '---' of frontmatter --------------
body_start=$(awk '/^---$/{c++; if(c==2){print NR+1; exit}}' "$NORMALIZED")
{
    echo "---"
    echo "name: $name"
    echo "description: $description"
    echo "---"
    echo ""
    tail -n "+$body_start" "$NORMALIZED"
} > "$OUT/SKILL.md"

# --- Supporting .md files -> references/ -----------------------------------
shopt -s nullglob
mds=("$SRC"/*.md)
shopt -u nullglob
ref_files=()
for f in "${mds[@]}"; do
    [[ "$(basename "$f")" == "SKILL.md" ]] && continue
    ref_files+=("$f")
done
if [[ ${#ref_files[@]} -gt 0 ]]; then
    mkdir -p "$OUT/references"
    cp "${ref_files[@]}" "$OUT/references/"
fi

# --- scripts/ and assets/ carry over as-is ---------------------------------
for d in scripts assets; do
    if [[ -d "$SRC/$d" ]]; then
        mkdir -p "$OUT/$d"
        cp -r "$SRC/$d/." "$OUT/$d/"
    fi
done

# --- Report back to the calling model --------------------------------------
echo "scaffolded: $OUT"
echo "name: $name"
echo "description: $description"
echo "had disable-model-invocation:true -> $had_disable_invocation (if true: write agents/openai.yaml with allow_implicit_invocation: false)"
echo "had argument-hint -> $had_argument_hint"
if [[ -n "$argument_hint" ]]; then
    echo "  was: \"$argument_hint\" -- no frontmatter home in Codex; fold into the body's own usage text"
fi
echo "reference files copied: ${#ref_files[@]}"
for f in "${ref_files[@]:-}"; do
    [[ -n "$f" ]] && echo "  - $(basename "$f")"
done
[[ -d "$SRC/scripts" ]] && echo "scripts/ copied"
[[ -d "$SRC/assets" ]] && echo "assets/ copied"
echo ""
echo "NEXT (manual — not done by this script): read $OUT/SKILL.md's body and rewrite Claude-Code-specific mechanism references per references/codex-format.md's mapping table."
