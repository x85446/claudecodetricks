#!/usr/bin/env bash
set -uo pipefail

# ============================================================================
# daily-sync.sh — unattended wrapper around sync-all.sh, for launchd/cron
# ============================================================================
# launchd hands a job almost no environment, so everything the run needs is
# established here rather than inherited. Writes a dated log, keeps 14 days,
# and leaves a single-line status file the user (or a status check) can read
# without parsing logs.
# ============================================================================

export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
REPO="$HOME/workspace/x85446/claudecodetricks"
HERE="$REPO/skills/skill-2-codex/scripts"
LOG_DIR="$HOME/.claude/log/codex-sync"
STATUS="$LOG_DIR/status.txt"
STAMP="$(date +%Y-%m-%d)"
LOG="$LOG_DIR/sync-$STAMP.log"

mkdir -p "$LOG_DIR"

{
    echo "=== codex skill sync — $(date '+%Y-%m-%d %H:%M:%S %Z') ==="
    if [[ ! -d "$REPO" ]]; then
        echo "FATAL: repo not found at $REPO"
        exit 1
    fi
    "$HERE/sync-all.sh" --install --prune
    rc=$?
    echo "sync-all exit: $rc"

    # Residual scan: anything the SAFE layer could not port is reported here,
    # never silently converted. This is the line worth reading each morning.
    python3 - <<'PY'
import glob, os, re
ROOT = os.path.expanduser("~/.agents/skills")
FLAGS = {"loop": r'(?<![\w./-])/loop\b', "askuser": r'\bAskUserQuestion\b',
         "agenttool": r'\bAgent tool\b|\bsubagent_type\b|\brun_in_background\b',
         "skilltool": r'\bSkill tool\b', "claudepath": r'~/\.claude/skills'}
n = 0
for f in sorted(glob.glob(f"{ROOT}/*/SKILL.md")) + sorted(glob.glob(f"{ROOT}/*/references/*.md")):
    for i, l in enumerate(open(f).read().split("\n"), 1):
        if "codex-port" in l:
            continue
        for k, p in FLAGS.items():
            if re.search(p, l):
                print(f"  NEEDS JUDGMENT [{k}] {f.replace(ROOT+'/','')}:{i}")
                n += 1
print(f"residual judgment sites: {n}")

# Codex charges manifest budget only for implicitly-invocable skills.
tot = 0
for f in glob.glob(f"{ROOT}/*/SKILL.md"):
    y = os.path.join(os.path.dirname(f), "agents", "openai.yaml")
    if os.path.exists(y) and "allow_implicit_invocation: false" in open(y).read():
        continue
    fm = open(f).read().split("---")[1]
    m = re.search(r'^description: (.+)$', fm, re.M)
    tot += len(os.path.basename(os.path.dirname(f))) + len(m.group(1).strip() if m else "")
print(f"manifest: {tot}/8000 chars" + (f"  OVER BY {tot-8000}" if tot > 8000 else "  ok"))
PY
    # Registry audit. claudecodetricks is the registry of record for every
    # first-party skill on this machine; this reports what is unregistered or
    # has drifted from its live copy. Report-only on purpose -- deciding which
    # side of a drift wins is a content call (a project copy can be an
    # authoritative edit or a stale install), so it is never auto-resolved.
    echo
    echo "--- registry audit ---"
    python3 "$REPO/skills/skills-audit.py" 2>&1 | sed -n '/^first-party:/,$p' | head -40

    echo "=== done $(date '+%H:%M:%S') ==="
} >"$LOG" 2>&1

# One-line status, cheap to read.
{
    date '+last run: %Y-%m-%d %H:%M:%S %Z'
    grep -E '^(converted|unchanged|failed|source changed|residual|manifest|first-party)' "$LOG" | sed 's/^/  /'
} > "$STATUS"

find "$LOG_DIR" -name 'sync-*.log' -mtime +14 -delete 2>/dev/null
exit 0
