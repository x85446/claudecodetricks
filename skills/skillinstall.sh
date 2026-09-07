#!/usr/bin/env bash
# Install skills from backup to their deploy locations.
# Usage: ./skillinstall.sh [skill-name]
#   No args = install all. Pass a name to install one.

set -e

SKILLHOME=~/workspace/x85446/claudecodetricks/skills
THIRDPARTY=~/workspace/x85446/claudecodetricks/3rd-party-Skills

# ── Colors ──────────────────────────────────────────────────────
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

# ── Deploy targets ──────────────────────────────────────────────
IMARKETING=~/workspace/izuma/marketing/.claude/skills
IMYRIPLAY=~/workspace/izuma/myriplay/.claude/skills
TAXES="$HOME/Library/CloudStorage/GoogleDrive-travis.mccollum@gmail.com/My Drive/TRAVIS_Taxes/.claude/skills"
CCTRICKS=~/workspace/x85446/claudecodetricks/temp/.claude/skills
WARDEN=~/workspace/x85446/warden/.claude/skills
FINANCE=~/workspace/x85446/financeSheets/.claude/skills
PERSONALDB=~/workspace/x85446/financeSheets/personaldb/.claude/skills
GRAVHL=~/workspace/gravhl/backend/.claude/skills
GRAVHL_CLOUD=~/workspace/gravhl/backend/cloud-setup/.claude/skills
GRAVHL_API_HEALTH=~/workspace/gravhl/backend/mgmt/api-health-dashboard/.claude/skills
MKTOOL=~/workspace/x85446/marketing-tool/.claude/skills
HOUSES=~/workspace/x85446/houses/.claude/skills
RECODE=~/workspace/izuma/RECODE/.claude/skills
USERGLOBAL="$HOME/.claude/skills"   # user-global skills, available in every session

# ── All targets (for global skills) ────────────────────────────
# Add new repos here — any skill using install_to_all picks them up automatically.
ALL_TARGETS=(
    "$IMARKETING"
    "$IMYRIPLAY"
    "$TAXES"
    "$CCTRICKS"
    "$WARDEN"
    "$FINANCE"
    "$PERSONALDB"
    "$GRAVHL"
    "$GRAVHL_CLOUD"
    "$GRAVHL_API_HEALTH"
    "$MKTOOL"
)

installed=0
skipped=0

# ── Functions ───────────────────────────────────────────────────

ok() {
    echo -e "  ${GREEN}✔${RESET}  ${BOLD}$1${RESET}  ${DIM}→${RESET}  ${CYAN}$2/${RESET}"
    installed=$((installed + 1))
}

fail() {
    echo -e "  ${YELLOW}✘${RESET}  ${BOLD}$1${RESET}  ${YELLOW}— $2${RESET}"
}

skip() {
    echo -e "  ${DIM}⊘  $1 — no deploy target, skipped${RESET}"
    skipped=$((skipped + 1))
}

header() {
    echo -e "\n${BOLD}${BLUE}⚡ Skill Installer${RESET}\n"
    echo -e "${BOLD}$1${RESET}\n"
}

summary() {
    echo ""
    echo -e "${GREEN}${BOLD}✔ ${installed} installed${RESET}  ${DIM}|${RESET}  ${DIM}${skipped} skipped${RESET}"
    echo ""
}

install_skill() {
    # Publish SKILL.md atomically.
    #
    # The obvious `mkdir -p dest/name && cp -r src/* dest/name/` is racy in two
    # ways, and a harness scanning the skills root mid-install reports both as
    # "Skipped loading N skill(s) due to invalid SKILL.md files":
    #   - for a NEW skill, mkdir publishes an empty directory before SKILL.md
    #     lands in it, so the scan sees a directory with no SKILL.md;
    #   - for an EXISTING one, cp truncates SKILL.md in place before rewriting
    #     it, so the scan can read a partial file.
    # Both windows are milliseconds, and both are hit eventually because the
    # installer runs while sessions are open.
    #
    # Staging sits beside the destination root, not inside it, so nothing
    # half-written is ever visible to a scan. SKILL.md is renamed into place
    # LAST -- rename is atomic, so the file is only ever old-and-valid or
    # new-and-valid, never in between.
    #
    # Copy stays ADDITIVE on purpose: a skill writes runtime state next to
    # itself (oracle/known.md, accounts/known.md, incus/routing.md) that has no
    # counterpart in the backup. A full replace would delete the user's data.
    local name=$1 dest=$2
    local stage="$dest/../.skillinstall-staging.$$"

    rm -rf "$stage"; mkdir -p "$stage/$name" || { fail "$name" "staging failed"; return; }
    if ! cp -r "$SKILLHOME/$name/"* "$stage/$name/" 2>/dev/null; then
        rm -rf "$stage"; fail "$name" "copy failed"; return
    fi
    if [[ ! -f "$stage/$name/SKILL.md" ]]; then
        rm -rf "$stage"; fail "$name" "no SKILL.md in source"; return
    fi

    if [[ -d "$dest/$name" ]]; then
        # Existing install: everything except SKILL.md first, then SKILL.md by
        # atomic rename, so the published file never appears truncated.
        mv "$stage/$name/SKILL.md" "$stage/$name.SKILL.md"
        cp -r "$stage/$name/." "$dest/$name/" 2>/dev/null
        mv "$stage/$name.SKILL.md" "$dest/$name/SKILL.md"
    else
        mkdir -p "$dest"
        mv "$stage/$name" "$dest/$name"
    fi
    rm -rf "$stage"
    ok "$name" "$dest"
}

install_to_all() {
    local name=$1
    for dest in "${ALL_TARGETS[@]}"; do
        install_skill "$name" "$dest"
    done
}

install_3p_skill() {
    local name=$1 dest=$2
    mkdir -p "$dest/$name"
    if cp -r "$THIRDPARTY/$name/"* "$dest/$name/" 2>/dev/null; then
        ok "$name" "$dest"
    else
        fail "$name" "copy failed"
    fi
}

# PM skills list
PM_SKILLS="pm pm-epic pm-feature pm-requirement pm-test pm-iterator pm-auditor pm-preflight pm-publish pm-status pm-webtool"

# TESTMASTER family (meta + 4 children)
TESTMASTER_SKILLS="testmaster testmaster-adopt testmaster-derive testmaster-catalog testmaster-maintain testmaster-prune testmaster-run testmaster-report"

# UXMASTER family (meta + 7 children)
UXMASTER_SKILLS="uxmaster uxmaster-analysis uxmaster-macos uxmaster-linux uxmaster-windows uxmaster-web uxmaster-cli uxmaster-implement"

# codeconverter family (meta + 18 stage children + 1 shared helper)
CODECONVERTER_SKILLS="codeconverter codeconverter-00-guidance codeconverter-00-source-provenance codeconverter-01-service-profile codeconverter-02-codebase-analysis codeconverter-03-dependency-discovery codeconverter-04-test-baseline codeconverter-05-api-surface codeconverter-05a-endpoint-consumers codeconverter-05b-outbound-dependencies codeconverter-05c-datastore-peers codeconverter-06-domain-analysis codeconverter-07-target-codebase codeconverter-08-gap-validation codeconverter-09-dependency-audit codeconverter-10-service-alignment codeconverter-10a-pilot-slice codeconverter-11-migration-plan codeconverter-12-migration-qa codeconverter-verify"

WEBTOOL_SRC=~/workspace/x85446/claudecodetricks/webtool

install_webtool() {
    local project_root=$1
    local dest="$project_root/.claude/webtool"
    mkdir -p "$dest/static"
    if cp "$WEBTOOL_SRC/serve.py" "$dest/" 2>/dev/null && \
       cp "$WEBTOOL_SRC/requirements.txt" "$dest/" 2>/dev/null && \
       cp "$WEBTOOL_SRC/package.json" "$dest/" 2>/dev/null && \
       cp "$WEBTOOL_SRC/vite.config.js" "$dest/" 2>/dev/null && \
       cp "$WEBTOOL_SRC/static/"* "$dest/static/" 2>/dev/null; then
        ok "webtool" "$dest"
    else
        fail "webtool" "copy failed"
    fi
}

install_pm_skills() {
    local dest=$1
    # dest is like ~/workspace/izuma/myriplay/.claude/skills
    # project root is two dirs up from .claude/skills
    local project_root=$(dirname "$(dirname "$dest")")
    for s in $PM_SKILLS; do
        install_skill "$s" "$dest"
    done
    install_webtool "$project_root"
}

do_install() {
    # Deploy targets come from skillmap.tsv, not from a case statement here.
    #
    # This used to be ~250 lines of `name) install_skill name "$TARGET" ;;` —
    # a second copy of knowledge that also lived in the now-removed Rust TUI's
    # skill-mappings.toml, in
    # per-entry .origin files, and in external-sources.conf. Four copies is
    # four chances to disagree, and they did: 21 globally-installed skills had
    # no case entry at all and were silently un-updatable for months.
    #
    # skillctl owns the install now. This stays as the human-facing CLI.
    local skill=$1
    local out rc
    out="$(python3 "$SKILLHOME/skillctl" install "$skill" 2>&1)"; rc=$?
    if [[ -z "$out" ]]; then
        skip "$skill" "no deploy target"
        return 0
    fi
    while IFS=$'\t' read -r verb name target rest; do
        [[ -z "$verb" ]] && continue
        case "$verb" in
            installed) ok "$name" "$target" ;;
            skip)      skip "$name" "${target:-backup-only}" ;;
            FAIL)      fail "$name" "${rest:-$target}" ;;
            *)         fail "$skill" "$verb $name $target $rest" ;;
        esac
    done <<< "$out"
    return $rc
}

install_all() {
    for dir in "$SKILLHOME"/*/; do
        local name=$(basename "$dir")
        [ "$name" = "db" ] && continue
        do_install "$name"
    done
}

# ── Main ────────────────────────────────────────────────────────

if [ -n "$1" ]; then
    header "Installing: $1"
    DIRECT_INSTALL=1 do_install "$1"
else
    header "Installing all skills:"
    install_all
fi

summary
