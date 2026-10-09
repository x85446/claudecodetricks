#!/bin/bash
# Fixtures for skills/mailbox/scripts/mbx.py: registry seeding and resolution, send/dedup,
# agent targeting, atomic claims, reply threading, the hop limit, legacy informs.
# Runs against throwaway repos under a scratch HOME; PASS/FAIL per case, exit 1 on any failure.
set -u
MBX="$(cd "$(dirname "$0")/.." && pwd)/scripts/mbx.py"
FX=$(cd "$(mktemp -d "${TMPDIR:-/tmp}/mbxfx.XXXXXX")" && pwd -P); trap 'rm -rf "$FX"' EXIT
export HOME="$FX/home" MAILBOX_AGENT=claude
mkdir -p "$HOME/workspace/x"
for p in alpha bravo; do git init -q "$HOME/workspace/x/$p"; git -C "$HOME/workspace/x/$p" commit -q --allow-empty -m init; done
A="$HOME/workspace/x/alpha"; B="$HOME/workspace/x/bravo"
pass=0; fail=0
check() { if eval "$2"; then echo "PASS $1"; pass=$((pass+1)); else echo "FAIL $1 :: $3"; fail=$((fail+1)); fi; }
mbx() { local d="$1"; shift; (cd "$d" && python3 "$MBX" "$@"); }

out=$(mbx "$A" inbox list); check "seed registers repos under ~/workspace" '[[ "$out" == *"alpha	$A"* && "$out" == *"bravo	$B"* ]]' "$out"
out=$(mbx "$A" inbox resolve bravp); check "near match resolves with near: tag" '[[ "$out" == "bravo	$B	near:bravp" ]]' "$out"
mbx "$A" inbox resolve nosuch >/dev/null 2>&1; check "unknown inbox exits 3" '[[ $? == 3 ]]' ""

out=$(echo "## Needed
x" | mbx "$A" msg send --to bravo --title "Deploy the thing")
f=$(cut -f3 <<<"$out")
check "send writes into the target inbox" '[[ "$out" == new*bravo* && -f "$f" && "$f" == "$B/.claude/iterate/inbox/"*-alpha-deploy-the-thing.md ]]' "$out"
check "header carries kind, reply wanted, hops 0, status open" 'grep -q "^# Request — Deploy the thing" "$f" && grep -q "^reply: wanted" "$f" && grep -q "^hops: 0" "$f" && grep -q "^status: open" "$f"' "$(cat "$f")"
check "sender keeps a sent copy" '[[ -f "$A/.claude/iterate/sent/$(basename "$f")" ]]' ""
out2=$(echo "## Needed
y" | mbx "$A" msg send --to bravo --title "Deploy the thing")
check "same sender + slug rewrites in place" '[[ "$out2" == updated* && "$(cut -f3 <<<"$out2")" == "$f" && $(ls "$B/.claude/iterate/inbox" | wc -l) -eq 1 ]]' "$out2"

id=$(basename "$f" .md)
out=$(mbx "$B" msg list); check "list shows it trusted" '[[ "$out" == "$id	open	request	alpha	trusted	Deploy the thing" ]]' "$out"
echo "## Needed
z" | mbx "$A" msg send --to bravo --title "Codex only" --agent codex >/dev/null
out=$(mbx "$B" msg list); check "codex-targeted message hidden from claude" '[[ "$out" != *"Codex only"* ]]' "$out"
out=$(MAILBOX_AGENT=codex mbx "$B" msg list); check "codex sees it" '[[ "$out" == *"Codex only"* ]]' "$out"

mbx "$B" msg claim "$id" >/dev/null; out=$(mbx "$B" msg claim "$id" 2>&1); rc=$?
check "second claim exits 4" '[[ $rc == 4 && "$out" == claimed* ]]' "$out"
out=$(mbx "$B" msg list); check "claimed message leaves the open list" '[[ "$out" != *"$id"* ]]' "$out"
mbx "$B" msg status "$id" "done (deployed)" >/dev/null
check "final status releases the claim" '[[ ! -d "$B/.claude/iterate/inbox/.claims/$id" ]] && grep -q "^status: done (deployed)" "$f"' ""

out=$(echo "## Result
deployed" | mbx "$B" msg send --kind reply --in-reply-to "$id" --title "Deployed")
r=$(cut -f3 <<<"$out")
check "reply lands in the sender's inbox, threaded" '[[ "$r" == "$A/.claude/iterate/inbox/"* ]] && grep -q "^in-reply-to: $id" "$r" && grep -q "^thread: $id" "$r" && grep -q "^reply: none" "$r"' "$out"

echo x | mbx "$A" msg send --to bravo --title t --hops 4 >/dev/null 2>&1; check "hops over 3 exits 5" '[[ $? == 5 ]]' ""
echo x | mbx "$A" msg send --to alpha --title t >/dev/null 2>&1; check "send to self exits 6" '[[ $? == 6 ]]' ""

mkdir -p "$FX/stranger"; printf '# Request — hi\n\nFrom: stranger (%s/stranger) · agent claude\nstatus: open\n\n## Needed\nx\n' "$FX" > "$B/.claude/iterate/inbox/20260101T000000Z-stranger-hi.md"
out=$(mbx "$B" msg list); check "unregistered sender flagged" '[[ "$out" == *"stranger	unregistered	hi"* ]]' "$out"
printf '# Inform — old style\n\nFrom: alpha (%s) · plan none · branch main · commit abc\nSent: 2026-01-01T00:00:00Z\nstatus: open\n\n## Problem\np\n' "$A" > "$B/.claude/iterate/inbox/20250101T000000Z-alpha-old-style.md"
out=$(mbx "$B" msg list); check "legacy inform parses as an open inform" '[[ "$out" == *"20250101T000000Z-alpha-old-style	open	inform	alpha	trusted	old style"* ]]' "$out"

echo "$pass passed, $fail failed"; [[ $fail == 0 ]]
