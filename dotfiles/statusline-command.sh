#!/bin/bash

install_deps() {
  local missing=()
  command -v jq >/dev/null 2>&1 || missing+=(jq)

  if [ ${#missing[@]} -eq 0 ]; then
    echo "✔ all dependencies present"
    return 0
  fi

  echo "Missing: ${missing[*]}"

  case "$(uname -s)" in
    Darwin)
      if ! command -v brew >/dev/null 2>&1; then
        echo "Error: Homebrew not found. Install from https://brew.sh then re-run." >&2
        return 1
      fi
      brew install "${missing[@]}"
      ;;
    Linux)
      if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y "${missing[@]}"
      elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y "${missing[@]}"
      elif command -v yum >/dev/null 2>&1; then
        sudo yum install -y "${missing[@]}"
      elif command -v pacman >/dev/null 2>&1; then
        sudo pacman -S --noconfirm "${missing[@]}"
      elif command -v apk >/dev/null 2>&1; then
        sudo apk add "${missing[@]}"
      else
        echo "Error: no supported package manager found (apt/dnf/yum/pacman/apk)" >&2
        return 1
      fi
      ;;
    *)
      echo "Error: unsupported OS: $(uname -s)" >&2
      return 1
      ;;
  esac
}

if [ "$1" = "--install" ]; then
  install_deps
  exit $?
fi

input=$(cat)

# Rate-limit forensics. The script never computes the weekly percentage — it
# only displays .rate_limits.seven_day.used_percentage as Claude Code reports
# it — so when the number looks wrong there is no way to tell a harness value
# from a display bug without seeing the raw input. Keep a rolling log of just
# the rate-limit block; it is tiny, and it turns "the bar looks wrong" into
# something answerable after the fact. Failures here must never break the
# status line, hence the guards.
{
  RL_LOG="$HOME/.claude/log/statusline-ratelimits.jsonl"
  mkdir -p "$(dirname "$RL_LOG")" 2>/dev/null
  printf '%s\n' "$(printf '%s' "$input" | jq -c --arg ts "$(date -Iseconds)" \
      '{ts:$ts, rl:(.rate_limits // null)}' 2>/dev/null)" >> "$RL_LOG" 2>/dev/null
  # keep the last 500 lines only
  if [ "$(wc -l < "$RL_LOG" 2>/dev/null || echo 0)" -gt 500 ]; then
    tail -n 500 "$RL_LOG" > "$RL_LOG.tmp" 2>/dev/null && mv "$RL_LOG.tmp" "$RL_LOG" 2>/dev/null
  fi
} 2>/dev/null || true

# Persist a session snapshot for the rate-limit watcher (harvested out of band).
# captured_at lets the reader detect a stale file (session closed → no renders).
echo "$input" | jq -c '{
  captured_at: now,
  model: .model.display_name,
  session_id: .session_id,
  context_used_percentage: .context_window.used_percentage,
  rate_limits: .rate_limits
}' > /tmp/sessiondata 2>/dev/null

MODEL=$(echo "$input" | jq -r '.model.display_name' | sed 's/(1M context)/(1M)/')
DIR=$(echo "$input" | jq -r '.workspace.current_dir')
PROJ=$(echo "$input" | jq -r '.workspace.project_dir // empty')
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
DURATION_MS=$(echo "$input" | jq -r '.cost.total_duration_ms // 0')

HOST=$(hostname -s)

BOLD='\033[1m'; DIM='\033[2m'; CYAN='\033[36m'; GREEN='\033[32m'; YELLOW='\033[33m'; RED='\033[31m'; MAGENTA='\033[35m'; RESET='\033[0m'
# Colour tier, per skills/uxmaster-cli/color.md: COLORTERM is the real
# truecolor signal (most truecolor terminals still say TERM=xterm-256color);
# TERM *-256color / *-direct opt in to 256. Everything above ANSI-16 needs a
# 16-colour fallback that still carries the meaning.
case "${COLORTERM:-}" in
  truecolor|24bit) ITER_TIER=256 ;;
  *) case "${TERM:-}" in *-256color|*-direct) ITER_TIER=256 ;; *) ITER_TIER=16 ;; esac ;;
esac
if [ "$ITER_TIER" = 256 ]; then
  ORANGE='\033[38;5;208m'   # queued: approved for the nightly tick
  DKGREEN='\033[38;5;28m'   # executing but stalled: no heartbeat in the live window
else
  ORANGE='\033[93m'         # bright yellow — still "more than planned" next to yellow
  DKGREEN='\033[32m'        # plain green — still "less than live" next to bold green
fi

# Pick bar color based on context usage
if [ "$PCT" -ge 50 ]; then BAR_COLOR="$RED"
elif [ "$PCT" -ge 25 ]; then BAR_COLOR="$YELLOW"
else BAR_COLOR="$GREEN"; fi

FILLED=$((PCT / 10)); EMPTY=$((10 - FILLED))
printf -v FILL "%${FILLED}s"; printf -v PAD "%${EMPTY}s"
BAR="${FILL// /█}${PAD// /░}"

MINS=$((DURATION_MS / 60000)); SECS=$(((DURATION_MS % 60000) / 1000))

# Back-calculate session start time from elapsed duration
NOW_S=$(date +%s)
START_S=$(( NOW_S - (DURATION_MS / 1000) ))
START_TIME=$(date -d "@$START_S" '+%H:%M' 2>/dev/null || date -r "$START_S" '+%H:%M' 2>/dev/null)

# Rate limit session info (5-hour window)
RATE_PCT=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty' | cut -d. -f1)
RATE_RESETS=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // empty')
RATE_INFO=""
if [ -n "$RATE_PCT" ] && [ -n "$RATE_RESETS" ]; then
  REMAINING_S=$(( RATE_RESETS - NOW_S ))
  REMAINING_M=$(( REMAINING_S / 60 ))
  if [ "$REMAINING_M" -lt 0 ]; then REMAINING_M=0; fi
  REMAINING_H=$(( REMAINING_M / 60 ))
  REMAINING_MM=$(( REMAINING_M % 60 ))
  if [ "$REMAINING_H" -gt 0 ]; then
    RESET_STR="${REMAINING_H}h${REMAINING_MM}m"
  else
    RESET_STR="${REMAINING_M}m"
  fi
  # Color based on usage
  if [ "$RATE_PCT" -ge 40 ]; then RATE_COLOR="$RED"
  elif [ "$RATE_PCT" -ge 20 ]; then RATE_COLOR="$YELLOW"
  else RATE_COLOR="$GREEN"; fi
  # Build session bar same style as context bar
  RATE_FILLED=$((RATE_PCT / 10)); RATE_EMPTY=$((10 - RATE_FILLED))
  printf -v RATE_FILL "%${RATE_FILLED}s"; printf -v RATE_PAD "%${RATE_EMPTY}s"
  RATE_BAR="${RATE_FILL// /█}${RATE_PAD// /░}"
  RATE_INFO="🕔 ${RATE_COLOR}${RATE_BAR}${RESET} ${RATE_PCT}% | -${RESET_STR}"
fi

# Weekly rate limit info (7-day window)
WEEK_PCT=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty' | cut -d. -f1)
WEEK_RESETS=$(echo "$input" | jq -r '.rate_limits.seven_day.resets_at // empty')
WEEK_INFO=""
if [ -n "$WEEK_PCT" ] && [ -n "$WEEK_RESETS" ]; then
  WEEK_REMAINING_S=$(( WEEK_RESETS - NOW_S ))
  WEEK_REMAINING_M=$(( WEEK_REMAINING_S / 60 ))
  if [ "$WEEK_REMAINING_M" -lt 0 ]; then WEEK_REMAINING_M=0; fi
  WEEK_REMAINING_D=$(( WEEK_REMAINING_M / 1440 ))
  WEEK_REMAINING_H=$(( (WEEK_REMAINING_M % 1440) / 60 ))
  if [ "$WEEK_REMAINING_D" -gt 0 ]; then
    WEEK_RESET_STR="${WEEK_REMAINING_D}d${WEEK_REMAINING_H}h"
  else
    WEEK_RESET_STR="${WEEK_REMAINING_H}h"
  fi
  # Pace-based color. Window is 7 days = 10080 min; elapsed = total - remaining.
  #   green  : cumulative use <= 1/7 per elapsed day (on pace or better)
  #   red    : cumulative use >= the red curve below, or 100% at any time
  #   yellow : between
  # The red curve is a hand-set schedule of cumulative %-used by elapsed day,
  # interpolated linearly between the points (a 13h reading gets a threshold
  # between day 0 and day 1, not day 1's):
  #   0d 0 · 1d 28 · 2d 48 · 3d 58 · 4d 72 · 5d 80 · 6d 95 · 7d 100
  # Integer form: everything is scaled by 1440 (minutes per day) so the
  # comparison is WEEK_PCT*1440 >= p[k]*1440 + (p[k+1]-p[k])*minutes_into_day.
  WEEK_ELAPSED_M=$(( 10080 - WEEK_REMAINING_M ))
  if [ "$WEEK_ELAPSED_M" -lt 1 ]; then WEEK_ELAPSED_M=1; fi
  if [ "$WEEK_ELAPSED_M" -gt 10080 ]; then WEEK_ELAPSED_M=10080; fi
  WEEK_RED_CURVE=(0 28 48 58 72 80 95 100)
  WEEK_DAY=$(( WEEK_ELAPSED_M / 1440 )); WEEK_INTO=$(( WEEK_ELAPSED_M % 1440 ))
  if [ "$WEEK_DAY" -ge 7 ]; then WEEK_DAY=6; WEEK_INTO=1440; fi
  WEEK_P0=${WEEK_RED_CURVE[$WEEK_DAY]}; WEEK_P1=${WEEK_RED_CURVE[$((WEEK_DAY + 1))]}
  WEEK_RED_X1440=$(( WEEK_P0 * 1440 + (WEEK_P1 - WEEK_P0) * WEEK_INTO ))
  # Pace answers "will I run out early?" Once the cap is actually hit that
  # question is moot: 100% is red no matter when it landed.
  if [ "$WEEK_PCT" -ge 100 ]; then WEEK_COLOR="$RED"
  elif [ $(( WEEK_PCT * 10080 )) -le $(( 100 * WEEK_ELAPSED_M )) ]; then WEEK_COLOR="$GREEN"
  elif [ $(( WEEK_PCT * 1440 )) -ge "$WEEK_RED_X1440" ]; then WEEK_COLOR="$RED"
  else WEEK_COLOR="$YELLOW"; fi
  # Build week bar same style as context bar
  WEEK_FILLED=$((WEEK_PCT / 10)); WEEK_EMPTY=$((10 - WEEK_FILLED))
  printf -v WEEK_FILL "%${WEEK_FILLED}s"; printf -v WEEK_PAD "%${WEEK_EMPTY}s"
  WEEK_BAR="${WEEK_FILL// /█}${WEEK_PAD// /░}"
  WEEK_INFO="📅 ${WEEK_COLOR}${WEEK_BAR}${RESET} ${WEEK_PCT}% | -${WEEK_RESET_STR}"
fi

# -C "$DIR" everywhere: the status line is spawned with a cwd of its own and
# must report the session's repository, not the spawner's.
BRANCH=""
DIRTY=""
if git -C "$DIR" rev-parse --git-dir > /dev/null 2>&1; then
  BRANCH=" | 🌿 $(git -C "$DIR" branch --show-current 2>/dev/null)"
  if [ -n "$(git -C "$DIR" status --porcelain 2>/dev/null)" ]; then
    DIRTY=" ${RED}●${RESET}"
  else
    DIRTY=" ${GREEN}✔${RESET}"
  fi
else
  BRANCH=" | 🍂"
fi

# --- iterate plans: one letter per plan, coloured by state -------------------
# The branch name was doing this job badly: "not on main" only ever meant
# "something is unfinished", never which plan or what state. Plan codenames walk
# the alphabet per project (1st plan an a-word, 2nd a b-word, ...), so the first
# letter is unique WITHIN a project by construction — L M N reads unambiguously
# as three plans.
#   yellow = planned   orange = queued (approved for the nightly tick)
#   bold green = executing, live   dark green = executing, stalled
#   red = blocked   cyan = unblocked, ready to re-queue   magenta = paused by
#   /iterate pause   dim = closed but not archived
# The trailing dim lowercase letter is always the NEXT plan's letter (from
# iterate-run's registry), so the segment renders even with zero plans -- an
# empty segment used to be indistinguishable from a broken one.
# Liveness is a colour, not just a weight: bold green means a hook fired in
# this project inside the live window, so something is really driving the run
# right now; dark green means the plan file still says executing but nothing
# has touched a tool in a while. That gap is not hypothetical -- a killed or
# crashed run never gets to write its ending, so "executing" persists forever
# and green used to mean only "a file says so". Bold-vs-plain was the only
# signal before, and color.md forbids a distinction that survives only if
# bold means bright. iterate-run stamps the heartbeat on every hook;
# ITERATE_LIVE_SECS overrides the 900s window.
# ⏰ after the letters means this project is enrolled in the nightly launchd
# tick (`iterate-run nightly enroll`); a dim `off` after it means the global
# switch is off. Emoji ignore ANSI, so a word carries what colour cannot.
# 📥N after that counts open informs: bug reports another project's session
# dropped in <root>/.claude/iterate/inbox with /iterate-inform, still reading
# `status: open` above their first `## ` heading. `/ip plan the inbox` plans them.
# Case carries teaming, which colour had no room left to say: UPPERCASE = the
# plan is teamified (`teamed: true`, so /iterate dispatches one subagent per
# team), lowercase = flat (one lane, the launching session's model — a flat
# plan has no Teams table and so no per-team Model column). Two facts, one
# column, no extra width.
#
# The segment is anchored to the PROJECT ROOT, never to current_dir. Plans
# live at <project>/.claude/iterate/plans, but current_dir is wherever the
# session last cd'd to: a single `cd src` in an unrelated turn made the whole
# segment vanish, so the line read "this project has no plans" when what had
# actually happened was "you are standing one directory too deep". The walk
# stops at the repository root -- plans sit beside .git, never above it, so a
# stray ~/.claude/iterate/plans can never leak into an unrelated project.
# The anchor is .claude/iterate itself (plans/, archive/ OR inbox/): a project
# whose last plan was archived has no plans/ directory at all, and requiring
# that exact path made the segment vanish precisely when it should have read
# "nothing queued" (confirmed live, claudecodetricks after eclectus archived).
# A project whose only iterate state is an inform another project sent it has
# neither, and must still show the 📥.
iter_root_for() {
  local base="$1" d
  [ -n "$base" ] && [ -d "$base" ] || return 1
  d="$base"
  while :; do
    { [ -d "$d/.claude/iterate/plans" ] || [ -d "$d/.claude/iterate/archive" ] || [ -d "$d/.claude/iterate/inbox" ]; } && { printf '%s' "$d"; return 0; }
    [ -e "$d/.git" ] && return 1
    case "$d" in /|.|"") return 1 ;; esac
    d=$(dirname "$d")
  done
}
ITER_ROOT=$(iter_root_for "$DIR") || ITER_ROOT=$(iter_root_for "$PROJ") || ITER_ROOT=""

ITER=""
if [ -n "$ITER_ROOT" ]; then
  ITER_LETTERS=""
  # Liveness: mtime of this project's heartbeat, stamped by iterate-run's
  # PreToolUse/PostToolUse hook. Missing file reads the same as stale.
  ITER_EXEC="$DKGREEN"
  ITER_HB_LIVE=0
  ITER_LIVE=0
  ITER_HB="$HOME/.claude/iterate-run/heartbeats/${ITER_ROOT//\//-}"
  if [ -f "$ITER_HB" ]; then
    ITER_HB_M=$(stat -f %m "$ITER_HB" 2>/dev/null || stat -c %Y "$ITER_HB" 2>/dev/null)
    if [ -n "$ITER_HB_M" ] && [ $(( $(date +%s) - ITER_HB_M )) -le "${ITERATE_LIVE_SECS:-900}" ]; then
      ITER_EXEC="${BOLD}${GREEN}"
      ITER_HB_LIVE=1
    fi
  fi
  for pf in "$ITER_ROOT"/.claude/iterate/plans/*.md; do
    [ -e "$pf" ] || continue
    b=$(basename "$pf" .md)
    # Only the key block at the top matters, and it must stop at the first
    # `## ` heading. Plans do not fence their keys with `---`, so stopping
    # only at that delimiter read the whole file and matched every state
    # string a changelog entry quotes about itself -- lines like
    # "PLAN PARKED -- `status: blocked-on-operator`" are normal in a plan's
    # history and would colour a finished plan by what it once was. Where a
    # plan happens to contain a `---` rule that bound still applies, so a
    # future fenced format keeps working.
    fm=$(awk '/^## /{exit} NR>1 && /^---$/{exit} {print}' "$pf" 2>/dev/null)
    case "$fm" in
      *"teamed: true"*) L=$(printf '%s' "${b:0:1}" | tr '[:lower:]' '[:upper:]') ;;
      *)                L=$(printf '%s' "${b:0:1}" | tr '[:upper:]' '[:lower:]') ;;
    esac
    case "$fm" in
      *"status: unblocked"*)
        # cleared by a human, waiting for the conductor to pick it back up
        ITER_LETTERS="${ITER_LETTERS}${CYAN}${L}${RESET}" ;;
      *"status: blocked"*|*"status: awaiting-human-gate"*)
        # ANY blocked-* reading is red. Naming the variants individually left
        # blocked-on-quota falling through to the `phase: executing` arm
        # below, so a plan parked on a quota wall showed as a live run.
        # (`status: unblocked` is matched above and does not contain
        # "status: blocked", so it is unaffected by the wider pattern.)
        ITER_LETTERS="${ITER_LETTERS}${RED}${L}${RESET}" ;;
      *"status: paused"*)
        # a human stopped it on purpose; /iterate resume continues it
        ITER_LETTERS="${ITER_LETTERS}${MAGENTA}${L}${RESET}" ;;
      *"status: queued"*)
        # approved as-is for the nightly tick (/ip stage); still planned
        ITER_LETTERS="${ITER_LETTERS}${ORANGE}${L}${RESET}" ;;
      *"phase: executing"*)
        [ "$ITER_HB_LIVE" = 1 ] && ITER_LIVE=1
        ITER_LETTERS="${ITER_LETTERS}${ITER_EXEC}${L}${RESET}" ;;
      *"phase: planned"*)
        ITER_LETTERS="${ITER_LETTERS}${YELLOW}${L}${RESET}" ;;
      *)
        ITER_LETTERS="${ITER_LETTERS}${DIM}${L}${RESET}" ;;
    esac
  done
  # The next plan's letter, dim, always last: iterate-run keeps each
  # project's position in its own a-z walk under project_next_idx, keyed by
  # the project's absolute path; a project it has never named is at a.
  ITER_NEXT=$(jq -r --arg k "$ITER_ROOT" '.project_next_idx[$k] // 0' \
    "$HOME/.claude/iterate-run/plan-names.json" 2>/dev/null)
  case "$ITER_NEXT" in ''|*[!0-9]*) ITER_NEXT=0 ;; esac
  ITER_NEXT_L=$(printf '%s' "abcdefghijklmnopqrstuvwxyz" | cut -c$((ITER_NEXT % 26 + 1)))
  ITER_LETTERS="${ITER_LETTERS}${DIM}${ITER_NEXT_L}${RESET}"
  # Enrolled in the nightly launchd tick? conductor.md says so per project;
  # the global switch lives in iterate-run's store.
  ITER_TIMER=""
  if grep -q '^tick-source: *launchd' "$ITER_ROOT/.claude/iterate/conductor.md" 2>/dev/null; then
    ITER_TIMER=" ⏰"
    if jq -e '.enabled == false' "$HOME/.claude/iterate-run/nightly.json" >/dev/null 2>&1; then
      ITER_TIMER="${ITER_TIMER} ${DIM}off${RESET}"
    fi
  fi
  ITER_INBOX=""
  ITER_OPEN=0
  for inf in "$ITER_ROOT"/.claude/iterate/inbox/*.md; do
    [ -e "$inf" ] || continue
    awk '/^## /{exit} {print}' "$inf" 2>/dev/null | grep -q '^status: *open' && ITER_OPEN=$((ITER_OPEN + 1))
  done
  [ "$ITER_OPEN" -gt 0 ] && ITER_INBOX=" 📥${ITER_OPEN}"
  # The icon lights up while a run is live: ⚡ replaces ⚙️. Emoji ignore
  # ANSI colour, so brightness has to come from the glyph itself.
  ITER_ICON="⚙️"
  [ "$ITER_LIVE" = 1 ] && ITER_ICON="⚡"
  ITER=" | ${ITER_ICON} ${ITER_LETTERS}${ITER_TIMER}${ITER_INBOX}"
fi

if [[ "$HOST" == warden* ]]; then
  HOST_ICON="⛓️"
else
  HOST_ICON="🖥️"
fi

echo -e "📁 ${DIR##*/} | ${MAGENTA}${HOST_ICON} ${HOST}${RESET} | ${CYAN}[$MODEL]${RESET}$BRANCH$DIRTY$ITER"
COST_FMT=$(printf '$%.2f' "$COST")
# Pick cost color based on session cost as a percentage of a $1.00 reference
COST_INT=$(awk -v c="$COST" 'BEGIN { printf "%.0f", c * 100 }')
if [ "$COST_INT" -ge 60 ]; then COST_COLOR="$RED"
elif [ "$COST_INT" -ge 40 ]; then COST_COLOR="$YELLOW"
else COST_COLOR="$GREEN"; fi
echo -e "📦 ${BAR_COLOR}${BAR}${RESET} ${PCT}% | ${COST_COLOR}${COST_FMT}${RESET} | ⏱️ ${MINS}m ${SECS}s"
ROW3="$RATE_INFO"
if [ -n "$WEEK_INFO" ]; then
  if [ -n "$ROW3" ]; then
    ROW3="${ROW3} | ${WEEK_INFO}"
  else
    ROW3="$WEEK_INFO"
  fi
fi
if [ -n "$ROW3" ]; then
  echo -e "${ROW3}"
fi
