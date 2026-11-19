#!/usr/bin/env bash
# skillctl bundle column: a mirrored skill installs with its out-of-folder
# helper, drift in that helper reads as stale, and sync --mirrored repairs it
# while leaving self-owned skills alone. Hermetic: HOME is a temp dir.
set -euo pipefail
here="$(cd "$(dirname "$0")/../.." && pwd)"
t="$(mktemp -d)"; trap 'rm -rf "$t"' EXIT
export HOME="$t"
reg="$t/reg"; mkdir -p "$reg/mine" "$t/owner/skills/hiv" "$t/proj"
cp "$here/skills/skillctl" "$reg/skillctl"
printf -- '---\nname: hiv\ndescription: x\n---\nbody\n' > "$t/owner/skills/hiv/SKILL.md"
printf 'print("v1")\n' > "$t/owner/tool.py"
printf -- '---\nname: mine\ndescription: x\n---\nmine\n' > "$reg/mine/SKILL.md"
printf 'PROJ=~/proj/.claude/skills\n' > "$reg/skillinstall.sh"
printf 'hiv\tproj\t~/owner/skills/hiv\t-\tscripts/tool.py=~/owner/tool.py\nmine\tproj\tself\t-\n' > "$reg/skillmap.tsv"
ctl() { python3 "$reg/skillctl" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

ctl install hiv >/dev/null
cmp -s "$t/owner/tool.py" "$t/proj/.claude/skills/hiv/scripts/tool.py" || fail "bundle file not installed"
[[ -f "$t/proj/.claude/skills/hiv/SKILL.md" ]] || fail "SKILL.md not installed"

out="$(ctl sync --mirrored || true)"
[[ -z "$out" ]] || fail "fresh install reads stale: $out"

printf 'print("v2")\n' > "$t/owner/tool.py"
out="$(ctl sync --mirrored || true)"
[[ "$out" == $'would-install\thiv\tproj' ]] || fail "bundle drift not detected: $out"

ctl sync --apply --mirrored >/dev/null
grep -q v2 "$t/proj/.claude/skills/hiv/scripts/tool.py" || fail "sync did not repair bundle"
[[ ! -e "$t/proj/.claude/skills/mine" ]] || fail "--mirrored installed a self-owned skill"

rm "$t/owner/tool.py"
out="$(ctl status || true)"
[[ "$out" == *$'no-source\t1'* ]] || fail "missing bundle source not reported: $out"
echo ok
