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
    local name=$1 dest=$2
    mkdir -p "$dest/$name"
    if cp -r "$SKILLHOME/$name/"* "$dest/$name/" 2>/dev/null; then
        ok "$name" "$dest"
    else
        fail "$name" "copy failed"
    fi
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

# codeconverter family (meta + 11 stage children)
CODECONVERTER_SKILLS="codeconverter codeconverter-01-service-profile codeconverter-02-codebase-analysis codeconverter-03-dependency-discovery codeconverter-04-test-baseline codeconverter-05-api-surface codeconverter-06-domain-analysis codeconverter-07-target-codebase codeconverter-08-gap-validation codeconverter-09-dependency-audit codeconverter-10-service-alignment codeconverter-11-migration-plan"

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
        marketing-doc-formatter)
            install_skill marketing-doc-formatter "$IMARKETING"
            ;;
        marketing-template-studio)
            install_skill marketing-template-studio "$IMARKETING"
            ;;
        grant-scout)
            install_skill grant-scout "$IMARKETING"
            ;;
        darpa-equipment-references)
            install_skill darpa-equipment-references "$IMARKETING"
            ;;
        product-discovery)
            install_skill product-discovery "$IMARKETING"
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
        categorizer)
            install_skill categorizer "$PERSONALDB"
            ;;
        auditor)
            install_skill auditor "$PERSONALDB"
            ;;
        importer-fix)
            install_skill importer-fix "$PERSONALDB"
            ;;
        auditor-sourcetable-inspector)
            install_skill auditor-sourcetable-inspector "$PERSONALDB"
            ;;
        categorize-linker)
            install_skill categorize-linker "$PERSONALDB"
            ;;
        importer)
            install_skill importer "$PERSONALDB"
            ;;
        downloader)
            install_skill downloader "$PERSONALDB"
            ;;
        downloader-orderdocs)
            install_skill downloader-orderdocs "$PERSONALDB"
            ;;
        -pdfify)
            install_skill -pdfify "$PERSONALDB"
            ;;
        venue-classifier)
            install_skill venue-classifier "$PERSONALDB"
            ;;
        dev-makefiles)
            install_skill dev-makefiles "$USERGLOBAL"
            ;;
        hours-maker)
            install_skill hours-maker "$FINANCE"
            ;;
        hours-researcher)
            install_skill hours-researcher "$FINANCE"
            ;;
        docs-organizer)
            install_to_all docs-organizer
            ;;
        monitor)
            install_to_all monitor
            ;;
        pptx)
            install_3p_skill pptx "$GRAVHL"
            ;;
        window-schedule)
            install_skill window-schedule "$HOUSES"
            ;;
        iterate-planner)
            install_skill iterate-planner "$USERGLOBAL"
            ;;
        iterate)
            install_skill iterate "$USERGLOBAL"
            ;;
        source2pdf)
            install_skill source2pdf "$USERGLOBAL"
            ;;
        pdf2name)
            install_skill pdf2name "$USERGLOBAL"
            ;;
        safedelete)
            install_skill safedelete "$USERGLOBAL"
            ;;
        fix-chrome-remote-desktop)
            install_skill fix-chrome-remote-desktop "$USERGLOBAL"
            ;;
        skill-2-codex)
            install_skill skill-2-codex "$USERGLOBAL"
            ;;
        codeconverter)
            for s in $CODECONVERTER_SKILLS; do
                install_skill "$s" "$RECODE"
            done
            ;;
        codeconverter-*)
            # Skip during install_all (codeconverter handles the batch).
            # But if called directly, install this one skill.
            if [ -n "$DIRECT_INSTALL" ]; then
                install_skill "$skill" "$RECODE"
            fi
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
