#!/bin/bash
# crd-fixup.sh — restore CRD host on cypressMini after boot / recovery.
# Idempotent: safe to run any time. Runs the same three fixes the
# fix-chrome-remote-desktop skill applies by hand.
#
# Invoked automatically at boot by /Library/LaunchDaemons/com.travis.crd-keepalive.plist
# Can also be run ad-hoc via `sudo /usr/local/bin/crd-fixup.sh`.

set -u

TRAVIS_UID=503
ENABLE_FLAG=/Library/PrivilegedHelperTools/org.chromium.chromoting.me2me_enabled
HOST_CFG=/Library/PrivilegedHelperTools/org.chromium.chromoting.json
HOST_BIN=/Library/PrivilegedHelperTools/ChromeRemoteDesktopHost.app/Contents/MacOS/remoting_me2me_host
BROKER_PLIST=/Library/LaunchDaemons/org.chromium.chromoting.broker.plist
LOG=/var/log/crd-keepalive.log

ts() { date '+%Y-%m-%d %H:%M:%S'; }
log() { echo "[$(ts)] $*" >> "$LOG"; }

log "==== crd-fixup start ===="

# 1. Wait for the console user to become travis (skip if we're not at boot).
# stat -f %Su /dev/console tells us who owns the console session; before login
# it's root or _windowserver. We wait up to 120s. If it never becomes travis
# (headless / user hasn't logged in yet), we still proceed — the host will run
# without a live Aqua session but at least advertise as online.
for i in $(seq 1 60); do
    CONSOLE_USER=$(stat -f %Su /dev/console 2>/dev/null || echo "?")
    if [ "$CONSOLE_USER" = "travis" ]; then
        log "console user is travis (waited ${i}s)"
        break
    fi
    sleep 2
done

# 2. Restore the enable-flag file if it's been soft-disabled.
if [ -f "${ENABLE_FLAG}.backup" ] && [ ! -f "$ENABLE_FLAG" ]; then
    mv "${ENABLE_FLAG}.backup" "$ENABLE_FLAG"
    log "restored ${ENABLE_FLAG} from .backup"
fi

# 3. Bootstrap the broker LaunchDaemon into system domain. Idempotent — if
# already loaded, this errors and we ignore it.
if ! launchctl print system/org.chromium.chromoting.broker >/dev/null 2>&1; then
    launchctl bootstrap system "$BROKER_PLIST" 2>>"$LOG" && log "broker bootstrapped"
else
    log "broker already in system domain"
fi

# 4. Launch the host as travis if it's not already running. Use nohup so the
# process survives our exit. NOT running it as root — the OAuth tokens in the
# host config are for travis.
if pgrep -x "remoting_me2me_host" >/dev/null 2>&1; then
    log "host already running"
else
    log "launching host as travis"
    # Launch in travis's user bootstrap namespace via launchctl asuser.
    # This reparents the child to travis's launchd session (not our daemon's
    # process group), so it survives independently of this script's exit and
    # doesn't need nohup (which fails under launchd — no controlling tty).
    launchctl asuser "$TRAVIS_UID" sudo -u travis "$HOST_BIN" --host-config="$HOST_CFG" < /dev/null >> "$LOG" 2>&1 &
    disown 2>/dev/null || true
    sleep 3
    if pgrep -x "remoting_me2me_host" >/dev/null 2>&1; then
        log "host up (pid=$(pgrep -f remoting_me2me_host | tr '\n' ' '))"
    else
        log "ERROR: host failed to start; see log above"
    fi
fi

log "==== crd-fixup end ===="
