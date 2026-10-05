#!/usr/bin/env bash
# contract-test.sh — the iterate family's contract-expansion rules, in source
# and as installed.
#
#   contract-1  /iterate rule 33 exists and answers extend-or-withdraw with extend
#   contract-2  /iterate's On-stuck operator-only list names no decision, and the
#               landing test's Fails paragraph makes clauses 3 and 4 work
#   contract-3  the conductor ladder has a parity row that ends in continue, and
#               no "needs a human decision" row
#   contract-4  triage has the Executor-defect row with `fix problem N`
#   contract-5  every family member carries the family version and its
#               installed copy matches the source
#
#   bash skills/iterate/tests/contract-test.sh     # from the repo
#
# Exits 0 when all five pass, 1 otherwise.
#
# TESTMASTER: id=contract-1 tier=? parallel=yes
# TESTMASTER: id=contract-2 tier=? parallel=yes
# TESTMASTER: id=contract-3 tier=? parallel=yes
# TESTMASTER: id=contract-4 tier=? parallel=yes
# TESTMASTER: id=contract-5 tier=? parallel=yes

set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(git -C "$HERE" rev-parse --show-toplevel 2>/dev/null)" || { echo "contract-test: run it from the claudecodetricks repo" >&2; exit 1; }
SK="$ROOT/skills"
INSTALLED="${ITERATE_INSTALLED:-$HOME/.claude/skills}"
fails=0
pass() { echo "$1: pass"; }
fail() { echo "$1: FAIL — $2"; fails=$((fails + 1)); }

# The paragraph of $2 that starts with $1 (a line prefix), as one line.
para() { awk -v p="$1" 'index($0, p) == 1 { on = 1 } on && /^$/ { exit } on { printf "%s ", $0 }' "$2"; }

# contract-1
r33=$(grep -E '^33\. \*\*' "$SK/iterate/SKILL.md")
if [ -n "$r33" ] && printf '%s' "$r33" | grep -q 'contract expansion' \
   && printf '%s' "$r33" | grep -q 'answered \*\*extend\*\*' \
   && printf '%s' "$r33" | grep -q 'tern'; then
    pass contract-1
else
    fail contract-1 "iterate rule 33 missing, or does not answer extend-or-withdraw with extend and cite tern"
fi

# contract-2
stuck=$(para '**On stuck' "$SK/iterate/SKILL.md")
fails2=$(para '**Fails' "$SK/iterate/SKILL.md")
if printf '%s' "$stuck" | grep -q 'a secret no agent has, physical access, an external party' \
   && ! printf '%s' "$stuck" | grep -qi 'decision' \
   && printf '%s' "$fails2" | grep -q 'clause 3 or clause 4' \
   && printf '%s' "$fails2" | grep -q 'is work'; then
    pass contract-2
else
    fail contract-2 "On-stuck list not closed to secret/physical/external, names a decision, or Fails does not make clauses 3-4 work"
fi

# contract-3
ladder=$(awk '/^\| Blocker \| Escalation ladder \|/{on=1} on && /^$/{exit} on' "$SK/iterate-conductor/SKILL.md")
if printf '%s\n' "$ladder" | grep -i 'parity' | grep -q 'continue' \
   && ! grep -q 'needs a human decision' "$SK/iterate-conductor/SKILL.md"; then
    pass contract-3
else
    fail contract-3 "no parity row ending in continue, or a 'needs a human decision' row remains"
fi

# contract-4
rows=$(grep -c 'Executor defect' "$SK/iterate-triage/SKILL.md")
row=$(grep 'Executor defect' "$SK/iterate-triage/SKILL.md")
if [ "$rows" -eq 1 ] && printf '%s' "$row" | grep -q 'fix problem N' && printf '%s' "$row" | grep -q 'unblocked'; then
    pass contract-4
else
    fail contract-4 "Executor-defect rows: $rows (want 1, with fix problem N and unblocked)"
fi

# contract-5
fam=$("$SK/skillctl" family iterate 2>&1); famrc=$?
ver=$(sed -n 's/^version: //p' "$SK/iterate/SKILL.md" | head -1)
why=""
[ "$famrc" -eq 0 ] || why="skillctl family iterate exit $famrc"
printf '%s' "$fam" | grep -q "version	$ver" || why="${why:+$why; }family line does not read $ver"
members=$(awk -F'\t' '$NF == "iterate" || $4 == "iterate" { print $1 }' "$SK/skillmap.tsv")
[ -n "$members" ] || members="i ibs ic in ip it iterate iterate-brainstorm iterate-conductor iterate-notes iterate-planner iterate-rules iterate-triage"
for m in $members; do
    grep -q "^version: $ver\$" "$SK/$m/SKILL.md" || why="${why:+$why; }$m source not $ver"
    diff -rq "$SK/$m" "$INSTALLED/$m" >/dev/null 2>&1 || why="${why:+$why; }$m installed copy differs"
done
if [ -z "$why" ]; then pass contract-5; else fail contract-5 "$why"; fi

[ "$fails" -eq 0 ]
