#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# makehelp.sh — complex logic extracted from the root Makefile
# ============================================================================
# The Makefile defines WHAT; this script implements HOW for anything too
# complex for an inline recipe (OS branching, plist templating, multi-step
# launchctl sequences). Currently covers the two always-on launchd daemons
# from the axolotl plan: the iterate-run dashboard server and its keep-alive
# Chrome tab. Every other Makefile target still inlines its own shell — this
# file is not (yet) a full 2-layer migration of the whole Makefile, just the
# delegate for the targets that need one.
# ============================================================================

UID_DOMAIN="gui/$(id -u)"
LOG_DIR="$HOME/Library/Logs/iterate-run"

require_macos() {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        echo "error: $1 is macOS-only (launchd is not portable) — nothing was installed" >&2
        exit 1
    fi
}

# bootstrap_with_retry works around a real, observed race: launchd's gui
# domain briefly rejects a bootstrap with "37: Operation already in
# progress" (surfaced to launchctl as generic "5: Input/output error")
# when another bootstrap/bootout is in flight against the same domain at
# the same instant — confirmed live on this machine via `log show
# --predicate 'process == "launchd"'` while another process's launchctl
# call was mid-flight. It is a transient lock, not a bad plist: retrying
# after the other operation finishes succeeds. Five attempts with a
# short, increasing backoff comfortably covers the contention window
# actually observed (resolved within a couple of seconds).
bootstrap_with_retry() {
    local domain="$1" plist="$2" label="$3" out rc
    for attempt in 1 2 3 4 5; do
        out=$(launchctl bootstrap "$domain" "$plist" 2>&1)
        rc=$?
        if [[ $rc -eq 0 ]]; then
            return 0
        fi
        # Already loaded (e.g. a concurrent install won the race) counts
        # as success — re-check via print rather than trusting rc alone.
        if launchctl print "$domain/$label" >/dev/null 2>&1; then
            return 0
        fi
        sleep "$attempt"
    done
    echo "error: launchctl bootstrap failed after 5 attempts: $out" >&2
    return "$rc"
}

# ---------------------------------------------------------------------------
# iterate-run serve daemon (com.x85446.iterate-run-serve)
# ---------------------------------------------------------------------------

SERVE_LABEL="com.x85446.iterate-run-serve"
SERVE_PLIST="$HOME/Library/LaunchAgents/$SERVE_LABEL.plist"

cmd_serve_install() {
    require_macos "make serve-install"
    local bin="$1" port="$2"
    if [[ ! -x "$bin" ]]; then
        echo "error: $bin not found or not executable — run 'make install' first" >&2
        exit 1
    fi
    mkdir -p "$HOME/Library/LaunchAgents" "$LOG_DIR"

    cat > "$SERVE_PLIST" <<PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>$SERVE_LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$bin</string>
        <string>serve</string>
        <string>--port</string>
        <string>$port</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>$LOG_DIR/serve.log</string>
    <key>StandardErrorPath</key><string>$LOG_DIR/serve.log</string>
</dict>
</plist>
PLISTEOF

    launchctl bootout "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null || true
    bootstrap_with_retry "$UID_DOMAIN" "$SERVE_PLIST" "$SERVE_LABEL"
    launchctl enable "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null || true
    printf 'armed: %s\n  plist: %s\n  binds: localhost:%s\n  logs:  %s/serve.log\n' \
        "$SERVE_LABEL" "$SERVE_PLIST" "$port" "$LOG_DIR"
}

cmd_serve_uninstall() {
    require_macos "make serve-uninstall"
    launchctl bootout "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null || true
    rm -f "$SERVE_PLIST"
    echo "disarmed: $SERVE_LABEL"
}

cmd_serve_status() {
    require_macos "make serve-status"
    echo "plist:   $([[ -f "$SERVE_PLIST" ]] && echo "$SERVE_PLIST" || echo "(not installed)")"
    launchctl print "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null \
        | grep -E '^\s+(state|pid|last exit code) ' || echo "service:  not loaded"
    echo "log:     $LOG_DIR/serve.log"
}

# ---------------------------------------------------------------------------
# Keep-alive Chrome tab on the dashboard (com.x85446.iterate-run-chrome)
# ---------------------------------------------------------------------------

CHROME_LABEL="com.x85446.iterate-run-chrome"
CHROME_PLIST="$HOME/Library/LaunchAgents/$CHROME_LABEL.plist"

cmd_chrome_install() {
    require_macos "make chrome-install"
    local chrome_bin="$1" debug_port="$2" profile_dir="$3" url="$4"
    if [[ ! -x "$chrome_bin" ]]; then
        echo "error: CHROME_BIN=$chrome_bin not found or not executable" >&2
        echo "  override it: make chrome-install CHROME_BIN=/path/to/chromium" >&2
        exit 1
    fi
    mkdir -p "$HOME/Library/LaunchAgents" "$LOG_DIR" "$profile_dir"

    cat > "$CHROME_PLIST" <<PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>$CHROME_LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$chrome_bin</string>
        <string>--user-data-dir=$profile_dir</string>
        <string>--remote-debugging-port=$debug_port</string>
        <string>--no-first-run</string>
        <string>--no-default-browser-check</string>
        <string>$url</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>$LOG_DIR/chrome.log</string>
    <key>StandardErrorPath</key><string>$LOG_DIR/chrome.log</string>
</dict>
</plist>
PLISTEOF

    launchctl bootout "$UID_DOMAIN/$CHROME_LABEL" 2>/dev/null || true
    bootstrap_with_retry "$UID_DOMAIN" "$CHROME_PLIST" "$CHROME_LABEL"
    launchctl enable "$UID_DOMAIN/$CHROME_LABEL" 2>/dev/null || true
    printf 'armed: %s\n  plist:       %s\n  binary:      %s\n  profile:     %s\n  debug port:  %s\n  start url:   %s\n' \
        "$CHROME_LABEL" "$CHROME_PLIST" "$chrome_bin" "$profile_dir" "$debug_port" "$url"
}

cmd_chrome_uninstall() {
    require_macos "make chrome-uninstall"
    launchctl bootout "$UID_DOMAIN/$CHROME_LABEL" 2>/dev/null || true
    rm -f "$CHROME_PLIST"
    echo "disarmed: $CHROME_LABEL"
}

cmd_chrome_status() {
    require_macos "make chrome-status"
    echo "plist:   $([[ -f "$CHROME_PLIST" ]] && echo "$CHROME_PLIST" || echo "(not installed)")"
    launchctl print "$UID_DOMAIN/$CHROME_LABEL" 2>/dev/null \
        | grep -E '^\s+(state|pid|last exit code) ' || echo "service:  not loaded"
    echo "log:     $LOG_DIR/chrome.log"
}


# ---------------------------------------------------------------------------
# run — the headline thing this repo does
# ---------------------------------------------------------------------------

# `make run` serves the iterate dashboard. It binds --port 0 rather than the
# default 8420 on purpose: the always-on launchd agent installed by
# `make serve-install` already holds 8420, and a run target that dies on
# "address already in use" the moment the daemon is working is a bad first
# impression. Port 0 lets the kernel pick, and serve prints the URL.
cmd_run() {
    local bin="$1"; shift
    if [[ ! -x "$bin" ]]; then
        echo "error: $bin not found — run 'make build' first" >&2
        exit 1
    fi
    if [[ $# -gt 0 ]]; then
        exec "$bin" "$@"
    fi
    exec "$bin" serve --port 0
}

# ---------------------------------------------------------------------------
# Dispatcher
# ---------------------------------------------------------------------------

case "${1:-}" in
    run)               shift; cmd_run "$@" ;;
    serve-install)     shift; cmd_serve_install "$@" ;;
    serve-uninstall)   shift; cmd_serve_uninstall "$@" ;;
    serve-status)      shift; cmd_serve_status "$@" ;;
    chrome-install)    shift; cmd_chrome_install "$@" ;;
    chrome-uninstall)  shift; cmd_chrome_uninstall "$@" ;;
    chrome-status)     shift; cmd_chrome_status "$@" ;;
    *)
        echo "Usage: $0 {run|serve-install|serve-uninstall|serve-status|chrome-install|chrome-uninstall|chrome-status}" >&2
        exit 1
        ;;
esac
