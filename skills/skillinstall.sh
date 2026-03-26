#!/usr/bin/env bash
# Install skills from backup to their deploy locations.
# Usage: ./skillinstall.sh [skill-name]
#   No args = install all. Pass a name to install one.

set -e

SKILLHOME=~/workspace/x85446/claudecodetricks/skills

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
    local name=$1 dest=$2
    mkdir -p "$dest/$name"
    if cp -r "$SKILLHOME/$name/"* "$dest/$name/" 2>/dev/null; then
        ok "$name" "$dest"
    else
        fail "$name" "copy failed"
    fi
}

# PM skills list
PM_SKILLS="pm pm-epic pm-feature pm-requirement pm-test pm-iterator pm-auditor pm-preflight pm-publish pm-status pm-webtool"

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
    local skill=$1
    case "$skill" in
        competitive-intel)
            install_skill competitive-intel "$IMARKETING"
            ;;
        feature-tracker)
            install_skill feature-tracker "$IMARKETING"
            install_skill feature-tracker "$IMYRIPLAY"
            ;;
        pm)
            install_pm_skills "$IMARKETING"
            install_pm_skills "$IMYRIPLAY"
            ;;
        pm-*)
            # Skip during install_all (pm handles the batch).
            # But if called directly, install this one skill.
            if [ -n "$DIRECT_INSTALL" ]; then
                install_skill "$skill" "$IMARKETING"
                install_skill "$skill" "$IMYRIPLAY"
            fi
            ;;
        tax-organizer)
            install_skill tax-organizer "$TAXES"
            ;;
        tax-doc-combiner)
            install_skill tax-doc-combiner "$TAXES"
            ;;
        categorize)
            install_skill categorize "$CCTRICKS"
            ;;
        *)
            skip "$skill"
            ;;
    esac
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
