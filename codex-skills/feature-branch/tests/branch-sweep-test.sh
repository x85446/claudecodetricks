#!/usr/bin/env bash
# Exercises branch-sweep.sh against a fixture repo with a bare origin.
# exit 0 pass, non-zero fail with the failing assertion printed.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
sweep="$here/../scripts/branch-sweep.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

ME="me@example.test"
THEM="colleague@example.test"

g() { git -c user.name=x -c user.email="$ME" "$@"; }
gt() { git -c user.name=y -c user.email="$THEM" "$@"; }
commit_at() { # $1 who(g|gt) $2 file $3 epoch
    echo "$3" > "$2"; git add "$2"
    GIT_AUTHOR_DATE="@$3 +0000" GIT_COMMITTER_DATE="@$3 +0000" "$1" commit -q -m "$2"
}

now="$(date +%s)"
old=$((now - 120*86400))   # 120 days ago
recent=$((now - 2*86400))  # 2 days ago

git init -q --bare -b main "$tmp/origin.git"
git init -q -b main "$tmp/work"
cd "$tmp/work"
git config user.email "$ME"; git config user.name x
git remote add origin "$tmp/origin.git"
commit_at g base.txt "$old"

# merged-mine: branch whose commit is already on main (local + remote)
git checkout -q -b feature/merged-mine
commit_at g merged.txt "$old"
git checkout -q main; git merge -q --ff-only feature/merged-mine

# colleague-merged: remote only, merged
git checkout -q -b feature/colleague-merged
commit_at gt cm.txt "$old"
git checkout -q main; git merge -q --ff-only feature/colleague-merged

# mine-drifted: unmerged, based on old main; main then moves 60 commits
git checkout -q -b feature/mine-drifted
commit_at g drift.txt "$old"
git checkout -q main
for i in $(seq 1 60); do commit_at g "m$i.txt" "$recent"; done

# colleague-fresh and colleague-stale: unmerged, remote only
git checkout -q -b feature/colleague-fresh
commit_at gt cf.txt "$recent"
git checkout -q main
git checkout -q -b feature/colleague-stale
commit_at gt cs.txt "$old"
git checkout -q main

git push -q origin --all
git symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/main
git branch -q -D feature/colleague-merged feature/colleague-fresh feature/colleague-stale
# local main falls behind: one more commit pushed from a second clone
git clone -q "$tmp/origin.git" "$tmp/other"
( cd "$tmp/other" && git config user.email "$ME" && git config user.name x && commit_at g late.txt "$recent" && git push -q origin main )

out="$("$sweep")"
echo "$out"
fail=0
want() { grep -qE "$1" <<<"$out" || { echo "FAIL: expected /$1/"; fail=1; }; }
nowant() { grep -qE "$1" <<<"$out" && { echo "FAIL: unexpected /$1/"; fail=1; } || true; }

want '^deleted	local	feature/merged-mine	merged into main'
want '^deleted	remote	feature/merged-mine	merged into main'
want '^deleted	remote	feature/colleague-merged	merged into main'
want '^kept	local	feature/mine-drifted	mine	behind=6[0-9]	base-age=12[0-9]d	drifted:'
want '^kept	remote	feature/colleague-fresh	colleague	fresh	'"$THEM"
want '^kept	remote	feature/colleague-stale	colleague	stale=1[0-9][0-9]d	'"$THEM"
want '^main	synced	main fast-forwarded 1 commits'
want '^summary	deleted=3	kept=3	drifted=1'
nowant 'colleague-(fresh|stale).*deleted'

git show-ref --verify --quiet refs/heads/feature/merged-mine && { echo "FAIL: local merged-mine survived"; fail=1; }
git ls-remote --exit-code --heads origin feature/colleague-merged >/dev/null 2>&1 && { echo "FAIL: remote colleague-merged survived"; fail=1; }
git ls-remote --exit-code --heads origin feature/colleague-stale >/dev/null 2>&1 || { echo "FAIL: remote colleague-stale deleted"; fail=1; }
[[ "$(git rev-parse main)" == "$(git rev-parse origin/main)" ]] || { echo "FAIL: main not synced"; fail=1; }

# second run is a no-op
out2="$("$sweep")"
grep -q '^summary	deleted=0	kept=3	drifted=1' <<<"$out2" || { echo "FAIL: second run not idempotent"; echo "$out2"; fail=1; }

# local main ahead of origin is reported, never pushed
commit_at g ahead.txt "$recent"
out3="$("$sweep")"
grep -q '^main	ahead	local main is 1 commits ahead' <<<"$out3" || { echo "FAIL: ahead not reported"; echo "$out3"; fail=1; }
[[ "$(git rev-parse main)" != "$(git rev-parse origin/main)" ]] || { echo "FAIL: ahead main was pushed"; fail=1; }

exit $fail
