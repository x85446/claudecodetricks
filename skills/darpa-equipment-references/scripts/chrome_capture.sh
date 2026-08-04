#!/bin/bash
# Capture a real-browser screenshot of a URL by driving the user's running
# Google Chrome via AppleScript, then snapshotting the window with
# `screencapture -l<window-id>`. Used as a fallback for pages that block
# headless browsers (Cloudflare / CloudFront / etc.).
#
# Usage:
#     chrome_capture.sh <url> <output.png> [max_wait_sec=30]
#
# Exit codes:
#     0 — png written, looks plausible
#     1 — usage error
#     2 — AppleScript / Chrome interaction failed
#     3 — page never settled (timed out on Cloudflare challenge)
#     4 — screencapture produced nothing or an obviously-bad file

set -e

URL="$1"
OUT="$2"
MAX_WAIT="${3:-30}"

if [ -z "$URL" ] || [ -z "$OUT" ]; then
    echo "Usage: chrome_capture.sh <url> <output.png> [max_wait_sec=30]" >&2
    exit 1
fi

mkdir -p "$(dirname "$OUT")"

# ── 1. Minimise every other Chrome window so they don't bleed into the
#      capture region, then open a fresh window pointing at the URL.
osascript >/dev/null 2>&1 <<APPLESCRIPT || true
tell application "Google Chrome"
    repeat with w in windows
        try
            set miniaturized of w to true
        end try
    end repeat
end tell
APPLESCRIPT
sleep 0.4
WID=$(osascript <<APPLESCRIPT
tell application "Google Chrome"
    activate
    set newWin to make new window
    set URL of active tab of newWin to "$URL"
    return id of newWin
end tell
APPLESCRIPT
)

if [ -z "$WID" ]; then
    echo "ERR: failed to open Chrome window" >&2
    exit 2
fi

# ── 2. Size the window to a known region we can capture by coordinates ─
# Chrome's AppleScript window-id is not the same as a CGWindowID, so
# `screencapture -l<id>` fails. Instead we pin the window to a known
# screen rectangle and capture that rectangle with `-R<x,y,w,h>`.
#
# Pin Chrome flush to the top-left so adjacent windows (which usually sit
# on the right/bottom on macOS) don't bleed into the capture. The width
# (1500 px) is wider than typical PDP content so the right column with
# price + quantity isn't right at the edge.
# Get the main display height so the window fills the full screen vertically.
SCREEN_H=$(osascript -e 'tell application "Finder" to get bounds of window of desktop' | awk -F', *' '{print $4}')
SCREEN_W=$(osascript -e 'tell application "Finder" to get bounds of window of desktop' | awk -F', *' '{print $3}')
CAP_X=0
CAP_Y=0
CAP_W=${SCREEN_W:-1500}
[ "$CAP_W" -gt 1800 ] && CAP_W=1800   # cap width so capture file isn't huge
CAP_H=${SCREEN_H:-1300}
osascript >/dev/null <<APPLESCRIPT
tell application "Google Chrome"
    set bounds of (first window whose id is $WID) to {$CAP_X, $CAP_Y, $((CAP_X + CAP_W)), $((CAP_Y + CAP_H))}
end tell
APPLESCRIPT

# ── 3. Poll the tab title until it stops being a CF interstitial ───────
start=$(date +%s)
settled=0
while :; do
    title=$(osascript 2>/dev/null <<APPLESCRIPT
tell application "Google Chrome"
    try
        return title of active tab of (first window whose id is $WID)
    on error
        return ""
    end try
end tell
APPLESCRIPT
)
    case "$title" in
        ""|"Just a moment..."|"Just a moment…"|"Loading..."|*"Verifying"*|*"Checking your browser"*|*"Performing security verification"*)
            : ;;  # still loading / still on challenge
        *)
            settled=1
            break ;;
    esac

    elapsed=$(($(date +%s) - start))
    if [ "$elapsed" -ge "$MAX_WAIT" ]; then
        echo "WARN: timeout (${MAX_WAIT}s) waiting for page to settle. last title: $title" >&2
        break
    fi
    sleep 1
done

# Give the page a moment to render after the challenge clears
if [ "$settled" -eq 1 ]; then
    sleep 2
else
    sleep 1
fi

# ── 4. Bring Chrome to the front + force-hide every other visible app ─
#     Cmd-Opt-H is unreliable on some apps (Outlook ignores it). Setting
#     `visible` on the process via System Events is the hammer.
osascript >/dev/null <<APPLESCRIPT
tell application "Google Chrome"
    set index of (first window whose id is $WID) to 1
    activate
end tell
APPLESCRIPT
sleep 0.4
# Record what we hide so we can restore it after the capture
HIDDEN_APPS=$(osascript <<APPLESCRIPT
tell application "System Events"
    set hidlist to {}
    repeat with p in (every process whose visible is true and name is not "Google Chrome")
        try
            set visible of p to false
            set end of hidlist to name of p
        end try
    end repeat
    return hidlist as string
end tell
APPLESCRIPT
)
sleep 1.2   # let the compositor settle after hiding

# ── 5. Capture by screen rectangle (window is pinned to this region) ───
screencapture -R"${CAP_X},${CAP_Y},${CAP_W},${CAP_H}" -x -t png "$OUT" || true

# ── 6. Close the window we just opened, and restore the user's other
#      Chrome windows from the Dock.
osascript >/dev/null 2>&1 <<APPLESCRIPT || true
tell application "Google Chrome"
    try
        close (first window whose id is $WID)
    end try
    repeat with w in windows
        try
            set miniaturized of w to false
        end try
    end repeat
end tell
APPLESCRIPT

# ── 6b. Un-hide every app we hid in step 4 ─────────────────────────────
if [ -n "$HIDDEN_APPS" ]; then
    IFS=',' read -ra _NAMES <<< "$HIDDEN_APPS"
    for n in "${_NAMES[@]}"; do
        clean=$(echo "$n" | sed 's/^ *//;s/ *$//')
        [ -z "$clean" ] && continue
        osascript >/dev/null 2>&1 <<APPLESCRIPT || true
tell application "System Events"
    try
        set visible of process "$clean" to true
    end try
end tell
APPLESCRIPT
    done
fi

# ── 7. Sanity-check the output ─────────────────────────────────────────
if [ ! -s "$OUT" ]; then
    echo "ERR: screencapture wrote nothing to $OUT" >&2
    exit 4
fi
size=$(stat -f%z "$OUT" 2>/dev/null || echo 0)
if [ "$size" -lt 10000 ]; then
    echo "ERR: $OUT only ${size} bytes — likely empty/blank capture" >&2
    exit 4
fi

echo "OK: wrote $OUT (${size} bytes)"
exit 0
