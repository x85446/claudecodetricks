#!/bin/bash
# Byte-level fixtures for the statusline's ⚙️ segment: colour per plan state (orange queued,
# dark-green stalled, bold-green live, red/yellow unchanged), the dim next-plan letter, the ⏰
# enrollment icon, and the .claude/iterate root walk. Runs the real script against throwaway
# projects under a scratch HOME and prints PASS/FAIL per case; exit 1 on any failure.
# Case ids match .claude/testmaster/catalog.json (req-flamingo-statusline, sl-1..sl-17).
set -u; export LC_ALL=C
SCRIPT="$(cd "$(dirname "$0")" && pwd)/statusline-command.sh"
FX=$(mktemp -d "${TMPDIR:-/tmp}/slfx.XXXXXX"); export HOME="$FX/home"; mkdir -p "$HOME/.claude/iterate-run/heartbeats"
pass=0; fail=0
render() { # render <dir> [ENV=VAL ...] -> first line via cat -v
  local dir="$1"; shift
  env "$@" bash "$SCRIPT" <<<"{\"workspace\":{\"current_dir\":\"$dir\"},\"model\":{\"display_name\":\"m\"}}" 2>/dev/null | head -1 | cat -v
}
seg() { sed -n 's/.*M-bM-^ZM-^Y\(.*\)/GEAR\1/p; s/.*M-bM-^ZM-!\(.*\)/BOLT\1/p' <<<"$1"; }
check() { local id="$1" cond="$2" got="$3"; if eval "$cond"; then echo "PASS $id"; pass=$((pass+1)); else echo "FAIL $id :: $got"; fail=$((fail+1)); fi; }
mkplan() { mkdir -p "$1/.claude/iterate/plans"; printf '# x\n\nname: %s\nphase: %s\n%s\n\n## Goal\ng\n' "$2" "$3" "$4" > "$1/.claude/iterate/plans/$2.md"; }
T256="TERM=xterm-256color COLORTERM="; T16="TERM=xterm COLORTERM="

# --- step 6: root detection ---
A="$FX/arch"; mkdir -p "$A/.claude/iterate/archive" "$A/src"; : > "$A/.claude/iterate/archive/old.md"
out=$(render "$A/src" $T256); s=$(seg "$out")
check "6-archive-only renders from subdir" '[[ "$s" == GEAR* ]]' "$out"
G="$FX/gitonly"; mkdir -p "$G/.git"
out=$(render "$G" $T256); s=$(seg "$out")
check "sl-11 .git without .claude/iterate renders no segment" '[[ -z "$s" ]]' "$out"

# --- step 7: orange queued ---
Q="$FX/queued"; mkplan "$Q" quokka planned "status: queued"
out=$(render "$Q" $T256); check "sl-1 queued is 38;5;208 on 256" '[[ "$out" == *"^[[38;5;208mq^[[0m"* ]]' "$out"
out=$(render "$Q" $T16);  check "sl-3 queued falls back to 93 on ANSI-16" '[[ "$out" == *"^[[93mq^[[0m"* ]]' "$out"
out=$(render "$Q" TERM=xterm COLORTERM=truecolor); check "7 COLORTERM truecolor selects 256" '[[ "$out" == *"^[[38;5;208m"* ]]' "$out"
B="$FX/blockedq"; mkplan "$B" bison planned "status: blocked-on-operator: token
status: queued"
out=$(render "$B" $T256); check "sl-10 blocked beats queued (red)" '[[ "$out" == *"^[[31mb^[[0m"* ]]' "$out"
P="$FX/plain"; mkplan "$P" yak planned ""
out=$(render "$P" $T256); check "sl-2 planned still yellow" '[[ "$out" == *"^[[33my^[[0m"* ]]' "$out"

# --- step 8: next letter ---
cat > "$HOME/.claude/iterate-run/plan-names.json" <<J
{"project_next_idx":{"$P":6},"seeded":true,"used":{}}
J
out=$(render "$P" $T256); check "sl-6 idx 6 renders dim g after the letters" '[[ "$out" == *"^[[33my^[[0m^[[2mg^[[0m"* ]]' "$out"
out=$(render "$Q" $T256); check "sl-5 absent project renders dim a" '[[ "$out" == *"q^[[0m^[[2ma^[[0m"* ]]' "$out"
E="$FX/empty"; mkdir -p "$E/.claude/iterate/plans"
out=$(render "$E" $T256); s=$(seg "$out"); check "sl-4 zero plans still renders GEAR + dim a" '[[ "$s" == *"^[[2ma^[[0m"* ]]' "$out"
check "sl-6b placeholder never uppercase" '[[ "$out" != *"^[[2mA"* ]]' "$out"

# --- step 9: timer icon ---
N="$FX/enrolled"; mkplan "$N" newt planned ""; printf -- '---\nenabled: true\ntick-source: launchd\n---\n' > "$N/.claude/iterate/conductor.md"
out=$(render "$N" $T256); check "sl-7 enrolled + no switch file ends with alarm clock" '[[ "$out" == *"M-bM-^OM-0"* && "$out" != *"off"* ]]' "$out"
echo '{"enabled":true}' > "$HOME/.claude/iterate-run/nightly.json"
out=$(render "$N" $T256); check "sl-7b enrolled + on ends with alarm clock" '[[ "$out" == *"M-bM-^OM-0"* && "$out" != *"off"* ]]' "$out"
echo '{"enabled":false}' > "$HOME/.claude/iterate-run/nightly.json"
out=$(render "$N" $T256); check "sl-8 switch off renders alarm clock + dim off" '[[ "$out" == *"M-bM-^OM-0 ^[[2moff^[[0m"* ]]' "$out"
printf -- '---\nenabled: true\n---\n' > "$N/.claude/iterate/conductor.md"
out=$(render "$N" $T256); check "sl-9 withdrawn renders no icon" '[[ "$out" != *"M-bM-^OM-0"* ]]' "$out"

# --- step 10: live vs stalled ---
X="$FX/exec"; mkplan "$X" emu executing ""; HB="$HOME/.claude/iterate-run/heartbeats/${X//\//-}"
touch "$HB"; out=$(render "$X" $T256 ITERATE_LIVE_SECS=900); s=$(seg "$out")
check "sl-12 fresh heartbeat: bold green + bolt" '[[ "$s" == BOLT* && "$out" == *"^[[1m^[[32me^[[0m"* ]]' "$out"
touch -t "$(date -v-901S +%Y%m%d%H%M.%S)" "$HB"; out=$(render "$X" $T256 ITERATE_LIVE_SECS=900); s=$(seg "$out")
check "sl-13 stale heartbeat: dark green 38;5;28 + gear" '[[ "$s" == GEAR* && "$out" == *"^[[38;5;28me^[[0m"* ]]' "$out"
rm -f "$HB"; out=$(render "$X" $T256 ITERATE_LIVE_SECS=900); s=$(seg "$out")
check "sl-14 no heartbeat: dark green + gear" '[[ "$s" == GEAR* && "$out" == *"^[[38;5;28me^[[0m"* ]]' "$out"
out=$(render "$X" $T16 ITERATE_LIVE_SECS=900); check "sl-15 ANSI-16 stalled falls back to plain 32" '[[ "$out" == *"^[[32me^[[0m"* && "$out" != *"38;5;28"* ]]' "$out"
touch -t "$(date -v-899S +%Y%m%d%H%M.%S)" "$HB"; out=$(render "$X" $T256 ITERATE_LIVE_SECS=900); s=$(seg "$out")
check "sl-16 inside the window (899s): still live" '[[ "$s" == BOLT* ]]' "$out"
touch "$HB"; out=$(render "$X" $T256 ITERATE_LIVE_SECS=900); s=$(seg "$out")
check "sl-17 refreshed heartbeat restores bold green + bolt" '[[ "$s" == BOLT* && "$out" == *"^[[1m^[[32me"* ]]' "$out"
XB="$FX/execblocked"; mkplan "$XB" wolf executing "status: blocked-on-quota"
out=$(render "$XB" $T256); check "10 executing + blocked-on-quota is red" '[[ "$out" == *"^[[31mw^[[0m"* ]]' "$out"
echo "== $pass passed, $fail failed (fixtures under $FX)"
[ $fail -eq 0 ]
