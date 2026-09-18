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
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
DURATION_MS=$(echo "$input" | jq -r '.cost.total_duration_ms // 0')

HOST=$(hostname -s)

DIM='\033[2m'; CYAN='\033[36m'; GREEN='\033[32m'; YELLOW='\033[33m'; RED='\033[31m'; MAGENTA='\033[35m'; RESET='\033[0m'

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
  RATE_INFO="⚡ ${RATE_COLOR}${RATE_BAR}${RESET} ${RATE_PCT}% | -${RESET_STR}"
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

BRANCH=""
DIRTY=""
if git rev-parse --git-dir > /dev/null 2>&1; then
  BRANCH=" | 🌿 $(git branch --show-current 2>/dev/null)"
  if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
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
#   yellow = planned   green = executing   red = blocked   cyan = unblocked,
#   ready to re-queue   magenta = paused by /iterate pause   dim = closed but
#   not archived
# Case carries teaming, which colour had no room left to say: UPPERCASE = the
# plan is teamified (`teamed: true`, so /iterate dispatches one subagent per
# team), lowercase = flat (one lane, the launching session's model — a flat
# plan has no Teams table and so no per-team Model column). Two facts, one
# column, no extra width.
ITER=""
if [ -d "$DIR/.claude/iterate/plans" ]; then
  ITER_LETTERS=""
  for pf in "$DIR"/.claude/iterate/plans/*.md; do
    [ -e "$pf" ] || continue
    b=$(basename "$pf" .md)
    # only the frontmatter matters; stop at the closing delimiter
    fm=$(awk 'NR>1 && /^---$/{exit} {print}' "$pf" 2>/dev/null)
    case "$fm" in
      *"teamed: true"*) L=$(printf '%s' "${b:0:1}" | tr '[:lower:]' '[:upper:]') ;;
      *)                L=$(printf '%s' "${b:0:1}" | tr '[:upper:]' '[:lower:]') ;;
    esac
    case "$fm" in
      *"status: unblocked"*)
        # cleared by a human, waiting for the conductor to pick it back up
        ITER_LETTERS="${ITER_LETTERS}${CYAN}${L}${RESET}" ;;
      *"status: blocked-on-operator"*|*"status: awaiting-human-gate"*)
        ITER_LETTERS="${ITER_LETTERS}${RED}${L}${RESET}" ;;
      *"status: paused"*)
        # a human stopped it on purpose; /iterate resume continues it
        ITER_LETTERS="${ITER_LETTERS}${MAGENTA}${L}${RESET}" ;;
      *"phase: executing"*)
        ITER_LETTERS="${ITER_LETTERS}${GREEN}${L}${RESET}" ;;
      *"phase: planned"*)
        ITER_LETTERS="${ITER_LETTERS}${YELLOW}${L}${RESET}" ;;
      *)
        ITER_LETTERS="${ITER_LETTERS}${DIM}${L}${RESET}" ;;
    esac
  done
  [ -n "$ITER_LETTERS" ] && ITER=" | ⚙️ ${ITER_LETTERS}"
fi

if [[ "$HOST" == warden* ]]; then
  HOST_ICON="⛓️"
else
  HOST_ICON="🖥️"
fi

echo -e "${MAGENTA}${HOST_ICON} ${HOST}${RESET} | ${CYAN}[$MODEL]${RESET} | 📁 ${DIR##*/}$BRANCH$DIRTY$ITER"
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
