#!/usr/bin/env bash
# tutorial.sh — shared runtime for self-running tutorials.
#
# Source this from a tutorial bucket script. Everything the human does is the
# Enter key: each step shows the real command, pre-filled and editable, and runs
# it on Enter. No copy-paste, no thinking, no setup.
#
# macOS ships bash 3.2, which lacks `read -e -i` (prefilled readline). This file
# re-execs under bash 4+ when it can find one, and degrades to zsh's `vared` or
# a plain confirm prompt when it can't — the tutorial always runs.

# ── Re-exec under a modern bash when the system one is too old ──────────────
if [ -z "${TUT_REEXEC:-}" ] && [ "${BASH_VERSINFO[0]:-0}" -lt 4 ]; then
    for _cand in /opt/homebrew/bin/bash /usr/local/bin/bash /usr/bin/bash; do
        if [ -x "$_cand" ]; then
            export TUT_REEXEC=1
            exec "$_cand" "$0" "$@"
        fi
    done
fi

set -uo pipefail

# ── Capability detection ────────────────────────────────────────────────────
# edit modes: readline (bash4+), vared (zsh), confirm (no prefill possible)
if [ "${BASH_VERSINFO[0]:-0}" -ge 4 ]; then
    TUT_EDIT_MODE=readline
elif command -v zsh >/dev/null 2>&1; then
    TUT_EDIT_MODE=vared
else
    TUT_EDIT_MODE=confirm
fi

TUT_AUTO="${TUT_AUTO:-0}"          # 1 = run everything unattended

# Read from the terminal when there is one, else stdin — so a tutorial piped
# input (CI, a demo recording, `echo q | run.sh`) still works instead of dying
# on "/dev/tty: Device not configured".
if [ -r /dev/tty ] && { : >/dev/tty; } 2>/dev/null; then
    TUT_TTY=/dev/tty
else
    TUT_TTY=/dev/stdin
    [ "$TUT_EDIT_MODE" = "readline" ] && TUT_EDIT_MODE=confirm
fi
TUT_STEP=0
TUT_FAILED=0
TUT_START_TS=$(date +%s)

# ── Colors (disabled when not a TTY or NO_COLOR is set) ─────────────────────
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    C_TITLE=$'\033[1;36m'; C_STEP=$'\033[1;33m'; C_CMD=$'\033[1;32m'
    C_DIM=$'\033[2m'; C_ERR=$'\033[1;31m'; C_OK=$'\033[0;32m'; C_OFF=$'\033[0m'
else
    C_TITLE=""; C_STEP=""; C_CMD=""; C_DIM=""; C_ERR=""; C_OK=""; C_OFF=""
fi

# ── Narration ───────────────────────────────────────────────────────────────

tut_title() {
    printf '\n%s╭─ %s%s\n' "$C_TITLE" "$1" "$C_OFF"
    [ $# -gt 1 ] && printf '%s│  %s%s\n' "$C_DIM" "$2" "$C_OFF"
    printf '%s╰────────────────────────────────────────────%s\n\n' "$C_TITLE" "$C_OFF"
}

tut_section() { printf '\n%s── %s ──%s\n\n' "$C_TITLE" "$1" "$C_OFF"; }

# Explanatory prose. Keep it to one or two lines: the command is the lesson.
tut_say() { printf '%s\n' "$1"; }

# Announce a step. Waits for Enter unless in AUTO mode.
tut_step() {
    TUT_STEP=$((TUT_STEP + 1))
    printf '\n%s[%d] %s%s\n' "$C_STEP" "$TUT_STEP" "$1" "$C_OFF"
    [ $# -gt 1 ] && printf '%s    %s%s\n' "$C_DIM" "$2" "$C_OFF"
}

tut_pause() {
    [ "$TUT_AUTO" = "1" ] && return 0
    printf '%s    (Enter to continue)%s' "$C_DIM" "$C_OFF"
    read -r _ <"$TUT_TTY"
}

# ── The core: show a real command, pre-filled and editable, run it on Enter ──

tut_run() {
    local cmd="$1"
    local line="$cmd"

    if [ "$TUT_AUTO" = "1" ]; then
        printf '  %s$ %s%s\n' "$C_CMD" "$cmd" "$C_OFF"
    else
        case "$TUT_EDIT_MODE" in
            readline)
                # Pre-filled and fully editable, then Enter runs it.
                IFS= read -e -i "$cmd" -r -p "  $ " line <"$TUT_TTY" || line="$cmd"
                ;;
            vared)
                line=$(cmd="$cmd" zsh -c '
                    local buf=$cmd
                    vared -p "  $ " buf </dev/tty >/dev/tty 2>&1
                    print -r -- $buf')
                ;;
            confirm)
                printf '  %s$ %s%s\n' "$C_CMD" "$cmd" "$C_OFF"
                printf '%s    Enter to run, or type a replacement command: %s' "$C_DIM" "$C_OFF"
                IFS= read -r line <"$TUT_TTY"
                ;;
        esac
        [ -z "${line:-}" ] && line="$cmd"
    fi

    eval "$line"
    local rc=$?
    if [ $rc -ne 0 ]; then
        TUT_FAILED=$((TUT_FAILED + 1))
        printf '%s    ↑ exited %d — the tutorial keeps going%s\n' "$C_ERR" "$rc" "$C_OFF"
    fi
    return 0
}

# Same, but the command must not be edited (destructive or order-critical).
tut_run_fixed() {
    printf '  %s$ %s%s\n' "$C_CMD" "$1" "$C_OFF"
    tut_pause
    eval "$1" || {
        TUT_FAILED=$((TUT_FAILED + 1))
        printf '%s    ↑ failed — continuing%s\n' "$C_ERR" "$C_OFF"
    }
    return 0
}

# ── Browser ─────────────────────────────────────────────────────────────────

tut_open() {
    local url="$1"
    printf '  %s→ opening %s%s\n' "$C_CMD" "$url" "$C_OFF"
    tut_pause
    if command -v open >/dev/null 2>&1; then
        open -a "Google Chrome" "$url" 2>/dev/null || open "$url"
    elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "$url" >/dev/null 2>&1
    else
        printf '%s    (no browser opener found — visit it manually)%s\n' "$C_DIM" "$C_OFF"
    fi
}

# ── Bulk shortcut: walk it step by step, or do the whole thing at once ───────
#
# Wrap any multi-step setup (populating a database, seeding fixtures) in
# tut_bulk_offer. The human either walks each step with Enter, or takes the
# shortcut and the whole block runs unattended.
#
#   tut_bulk_offer "Populate the database" bulk_populate
#   bulk_populate() { tut_run "make db-seed"; tut_run "make db-verify"; }

tut_bulk_offer() {
    local label="$1" fn="$2" answer=""
    printf '\n%s%s — this takes several steps.%s\n' "$C_STEP" "$label" "$C_OFF"
    if [ "$TUT_AUTO" != "1" ]; then
        printf '  [Enter] walk through each step   [a] just run it all: '
        IFS= read -r answer <"$TUT_TTY"
    fi
    if [ "${answer:-}" = "a" ] || [ "${answer:-}" = "A" ]; then
        printf '%s  running the whole block…%s\n' "$C_DIM" "$C_OFF"
        local prev="$TUT_AUTO"; TUT_AUTO=1
        "$fn"
        TUT_AUTO="$prev"
    else
        "$fn"
    fi
}

# ── Preconditions ───────────────────────────────────────────────────────────
# Fail loudly and early rather than halfway through a broken lesson.

tut_require() {
    local missing=0 item
    for item in "$@"; do
        if ! command -v "$item" >/dev/null 2>&1; then
            printf '%s  missing required command: %s%s\n' "$C_ERR" "$item" "$C_OFF"
            missing=1
        fi
    done
    [ $missing -eq 1 ] && { printf '%s  install the above, then re-run.%s\n' "$C_ERR" "$C_OFF"; exit 1; }
    return 0
}

# ── Wrap-up ─────────────────────────────────────────────────────────────────

# Exits non-zero when any step failed, so `run.sh --auto` is usable as a build
# check. The tutorial still ran to the end — this only reports the truth.
tut_done() {
    local mins=$(( ($(date +%s) - TUT_START_TS) / 60 ))
    printf '\n%s╭─ done — %d steps in ~%d min%s\n' "$C_OK" "$TUT_STEP" "$mins" "$C_OFF"
    [ "$TUT_FAILED" -gt 0 ] && printf '%s│  %d command(s) exited non-zero%s\n' "$C_ERR" "$TUT_FAILED" "$C_OFF"
    [ $# -gt 0 ] && printf '%s│  next: %s%s\n' "$C_DIM" "$1" "$C_OFF"
    printf '%s╰────────────────────────────────────────────%s\n\n' "$C_OK" "$C_OFF"
    [ "$TUT_FAILED" -gt 0 ] && return 1
    return 0
}
