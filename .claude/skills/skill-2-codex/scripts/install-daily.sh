#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# install-daily.sh — arm (or disarm) the daily Codex skill sync
# ============================================================================
# macOS: a launchd LaunchAgent, not crontab. launchd is the supported mechanism
# here, survives reboot, and -- unlike cron -- runs a missed StartCalendarInterval
# job when the machine wakes instead of silently skipping the night the laptop
# was closed. That difference matters for a job whose whole purpose is "the
# Codex copies are never more than a day stale."
#
# Usage: install-daily.sh [--at HH:MM] [--uninstall] [--status] [--run-now]
# ============================================================================

LABEL="com.x85446.codex-skill-sync"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
SCRIPT="$HOME/workspace/x85446/claudecodetricks/skills/skill-2-codex/scripts/daily-sync.sh"
LOG_DIR="$HOME/.claude/log/codex-sync"
HOUR=3; MIN=15

while [[ $# -gt 0 ]]; do
    case "$1" in
        --at) HOUR="${2%%:*}"; MIN="${2##*:}"; HOUR="${HOUR#0}"; MIN="${MIN#0}"; shift 2 ;;
        --uninstall)
            launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
            rm -f "$PLIST"; echo "disarmed: $LABEL"; exit 0 ;;
        --status)
            echo "plist:   $([[ -f "$PLIST" ]] && echo "$PLIST" || echo "(not installed)")"
            launchctl print "gui/$(id -u)/$LABEL" 2>/dev/null \
                | grep -E '^\s+(state|last exit code|runs) ' || echo "service:  not loaded"
            [[ -f "$LOG_DIR/status.txt" ]] && { echo "--- last run ---"; cat "$LOG_DIR/status.txt"; }
            exit 0 ;;
        --run-now)
            launchctl kickstart -k "gui/$(id -u)/$LABEL"; echo "triggered"; exit 0 ;;
        *) echo "unknown arg: $1" >&2; exit 2 ;;
    esac
done

[[ -x "$SCRIPT" ]] || { echo "error: $SCRIPT not executable" >&2; exit 1; }
mkdir -p "$HOME/Library/LaunchAgents" "$LOG_DIR"

cat > "$PLIST" <<PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>$LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>$SCRIPT</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key><integer>$HOUR</integer>
        <key>Minute</key><integer>$MIN</integer>
    </dict>
    <key>StandardOutPath</key><string>$LOG_DIR/launchd.out</string>
    <key>StandardErrorPath</key><string>$LOG_DIR/launchd.err</string>
    <key>RunAtLoad</key><false/>
    <key>ProcessType</key><string>Background</string>
    <key>LowPriorityIO</key><true/>
</dict>
</plist>
PLISTEOF

launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST"
printf 'armed: %s daily at %02d:%02d\n  plist: %s\n  logs:  %s\n' "$LABEL" "$HOUR" "$MIN" "$PLIST" "$LOG_DIR"
