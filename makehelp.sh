#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# makehelp.sh — complex logic extracted from the root Makefile
# ============================================================================
# The Makefile defines WHAT; this script implements HOW for anything too
# complex for an inline recipe (OS branching, plist templating, multi-step
# launchctl sequences), scoped binary installation.
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
        if out=$(launchctl bootstrap "$domain" "$plist" 2>&1); then rc=0; else rc=$?; fi
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
    if [[ "$port" != "8420" ]]; then
        echo "error: the shared dashboard uses port 8420; no alternate application server is installed" >&2
        return 1
    fi
    local listener managed_pid
    listener="$(lsof -nP -t -iTCP:8420 -sTCP:LISTEN 2>/dev/null || true)"
    managed_pid="$(launchctl print "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null | awk '/^[[:space:]]*pid = / {print $3}' || true)"
    if [[ -n "$listener" && "$listener" != "$managed_pid" ]]; then
        echo "error: port 8420 belongs to another process ($listener); stop that service yourself before make serve-install" >&2
        return 1
    fi
    local expected health prior_plist
    expected="ok $("$bin" version)"
    health="$(curl --noproxy '*' --silent --fail --max-time 2 http://localhost:8420/healthz 2>/dev/null || true)"
    if [[ -n "$managed_pid" && "$health" == "$expected" ]]; then
        echo "iterate-run dashboard ready: http://localhost:8420/ (managed build already current)"
        return 0
    fi
    mkdir -p "$HOME/Library/LaunchAgents" "$LOG_DIR"
    prior_plist="$SERVE_PLIST.previous"
    if [[ -f "$SERVE_PLIST" ]]; then cp -p "$SERVE_PLIST" "$prior_plist"; fi

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
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key><string>$HOME/go/bin:$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
    <key>StandardOutPath</key><string>$LOG_DIR/serve.log</string>
    <key>StandardErrorPath</key><string>$LOG_DIR/serve.log</string>
</dict>
</plist>
PLISTEOF

    launchctl bootout "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null || true
    # Wait for the old registration to disappear before publishing its successor.
    for attempt in {1..20}; do
        if ! launchctl print "$UID_DOMAIN/$SERVE_LABEL" >/dev/null 2>&1; then break; fi
        sleep 0.1
    done
    if ! bootstrap_with_retry "$UID_DOMAIN" "$SERVE_PLIST" "$SERVE_LABEL"; then
        if [[ -f "$prior_plist" ]]; then
            cp -p "$prior_plist" "$SERVE_PLIST"
            bootstrap_with_retry "$UID_DOMAIN" "$SERVE_PLIST" "$SERVE_LABEL" || true
        fi
        return 1
    fi
    launchctl enable "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null || true
    local ready=false
    for attempt in {1..30}; do
        health="$(curl --noproxy '*' --silent --fail --max-time 1 http://localhost:8420/healthz 2>/dev/null || true)"
        if [[ "$health" == "$expected" ]]; then ready=true; break; fi
        if ! launchctl print "$UID_DOMAIN/$SERVE_LABEL" >/dev/null 2>&1; then
            bootstrap_with_retry "$UID_DOMAIN" "$SERVE_PLIST" "$SERVE_LABEL" || true
        fi
        sleep 0.5
    done
    if [[ "$ready" != true ]]; then
        echo "error: managed dashboard did not report the intended build; inspect $LOG_DIR/serve.log" >&2
        if [[ -f "$bin.previous" && -f "$prior_plist" ]]; then
            launchctl bootout "$UID_DOMAIN/$SERVE_LABEL" 2>/dev/null || true
            local rollback
            rollback="$(mktemp "$(dirname "$bin")/.iterate-rollback.XXXXXX")"
            cp -p "$bin.previous" "$rollback"
            mv -f "$rollback" "$bin"
            cp -p "$prior_plist" "$SERVE_PLIST"
            bootstrap_with_retry "$UID_DOMAIN" "$SERVE_PLIST" "$SERVE_LABEL" || true
            echo "Restored the preserved binary and service definition." >&2
        fi
        return 1
    fi
    printf 'armed: %s\n  plist: %s\n  binds: localhost:%s\n  logs:  %s/serve.log\n' \
        "$SERVE_LABEL" "$SERVE_PLIST" "$port" "$LOG_DIR"
}

# Atomic replacement keeps a running worker's executable inode intact.
cmd_install_iterate_run() {
    local source="$1" destination="$2" temporary
    mkdir -p "$destination"
    temporary="$(mktemp "$destination/.iterate-run.XXXXXX")"
    if ! cp "$source" "$temporary" || ! chmod 755 "$temporary"; then rm -f "$temporary"; return 1; fi
    if command -v codesign >/dev/null 2>&1; then
        if ! codesign --force --sign - "$temporary"; then rm -f "$temporary"; return 1; fi
    fi
    if [[ -f "$destination/iterate-run" ]] && cmp -s "$temporary" "$destination/iterate-run"; then
        rm -f "$temporary"
        "$destination/iterate-run" version
        return 0
    fi
    if [[ -f "$destination/iterate-run" ]]; then cp -p "$destination/iterate-run" "$destination/iterate-run.previous"; fi
    mv -f "$temporary" "$destination/iterate-run"
    "$destination/iterate-run" version
}

# `make install` replaces the binary the managed dashboard runs. Restart the
# dashboard on the new build when its agent is installed; install nothing when
# it is not.
cmd_serve_refresh() {
    local bin="$1" port="$2"
    [[ "$(uname -s)" == "Darwin" && -f "$SERVE_PLIST" ]] || return 0
    cmd_serve_install "$bin" "$port"
}

cmd_install_iterate_skills() {
    local names=() name
    while IFS= read -r name; do names+=("$name"); done < <(awk -F '\t' '$4 == "iterate" {print $1}' skills/skillmap.tsv)
    skills/skillctl family iterate
    skills/skillctl install "${names[@]}"
    for name in "${names[@]}"; do
        skills/skill-2-codex/scripts/sync-all.sh --only "$name" --install --quiet
    done
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

# `make run` reuses the shared dashboard or starts it on its fixed port.
cmd_run() {
    local bin="$1"; shift
    if [[ ! -x "$bin" ]]; then
        echo "error: $bin not found — run 'make build' first" >&2
        exit 1
    fi
    if [[ $# -gt 0 ]]; then
        exec "$bin" "$@"
    fi
    local health expected
    expected="ok $("$bin" version)"
    if health="$(curl --noproxy '*' --fail --silent --max-time 2 http://localhost:8420/healthz 2>/dev/null)"; then
        if [[ "$health" != "ok iterate-run "* ]]; then
            echo "error: port 8420 serves another application; no second server was started" >&2
            return 1
        fi
        if [[ "$health" != "$expected" ]]; then
            echo "Existing dashboard build: ${health#ok }" >&2
            echo "Run make serve-install to deploy this build to the managed service." >&2
        fi
        echo "iterate-run dashboard ready: http://localhost:8420/ (existing service)"
        return 0
    fi
    exec "$bin" serve --port 8420
}

# `make preview` serves this build beside the managed dashboard. Plans,
# archives, hook history and the run record are read without changing them.
cmd_preview() {
    local bin="$1" port="$2"
    if [[ ! -x "$bin" ]]; then
        echo "error: $bin not found — run 'make build' first" >&2
        exit 1
    fi
    if [[ "$port" == "8420" ]]; then
        echo "error: 8420 is the managed dashboard's port; pick another: make preview PREVIEW_PORT=8490" >&2
        exit 1
    fi
    if lsof -nP -t -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
        echo "error: port $port is in use; pick another: make preview PREVIEW_PORT=<port>" >&2
        exit 1
    fi
    echo "preview: http://localhost:$port/  (Ctrl-C stops it)"
    exec "$bin" serve --port "$port"
}

# `make report-json` reads the plan report model from a running dashboard.
cmd_report_json() {
    local url="$1" project="${2:-$PWD}" plan="$3" archive="$4" history="$5" query
    if [[ -z "$plan" && -z "$archive" ]]; then
        echo "error: name the execution: make report-json PLAN=<name> or ARCHIVE=<archive file> [PROJECT=<dir>]" >&2
        exit 2
    fi
    query="project=$(python3 -c 'import sys,urllib.parse;print(urllib.parse.quote(sys.argv[1],safe=""))' "$project")"
    [[ -n "$plan" ]] && query="$query&plan=$plan"
    [[ -n "$archive" ]] && query="$query&archive=$archive"
    [[ -n "$history" ]] && query="$query&history=1"
    if ! curl --noproxy '*' --silent --show-error --fail-with-body --max-time 120 "$url/api/report?$query"; then
        echo >&2
        echo "error: $url did not serve /api/report — start a current build with 'make preview' and pass REPORT_URL=http://localhost:8490" >&2
        exit 1
    fi
    echo
}

# ---------------------------------------------------------------------------
# Dispatcher
# ---------------------------------------------------------------------------

case "${1:-}" in
    run)               shift; cmd_run "$@" ;;
    preview)           shift; cmd_preview "$@" ;;
    report-json)       shift; cmd_report_json "$@" ;;
    install-iterate-run) shift; cmd_install_iterate_run "$@" ;;
    serve-install)     shift; cmd_serve_install "$@" ;;
    serve-refresh)     shift; cmd_serve_refresh "$@" ;;
    install-iterate-skills) cmd_install_iterate_skills ;;
    serve-uninstall)   shift; cmd_serve_uninstall "$@" ;;
    serve-status)      shift; cmd_serve_status "$@" ;;
    chrome-install)    shift; cmd_chrome_install "$@" ;;
    chrome-uninstall)  shift; cmd_chrome_uninstall "$@" ;;
    chrome-status)     shift; cmd_chrome_status "$@" ;;
    *)
        exit 1
        ;;
esac
