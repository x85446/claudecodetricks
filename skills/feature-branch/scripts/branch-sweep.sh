#!/usr/bin/env bash
# branch-sweep.sh — return a repo's branches to the starting point.
#
# Deletes every branch, local and remote, whose work is already on the default
# branch; keeps every colleague's unmerged branch; reports what survives and
# how far it has drifted; fast-forwards the local default branch to origin.
#
# Usage: branch-sweep.sh [--dry-run] [--stale-days N] [--drift-commits N] [--drift-days N]
#
# Output: one tab-separated fact per line, machine-first.
#   deleted	local	<branch>	<reason>
#   deleted	remote	<branch>	<reason>
#   kept	local	<branch>	mine|colleague	behind=<n>	base-age=<days>d	<ok|drifted: …>
#   kept	remote	<branch>	colleague	stale=<days>d|fresh	<author>
#   main	<synced|ahead|diverged|detached>	<detail>
#   summary	deleted=<n>	kept=<n>	drifted=<n>
#
# Rules:
#   merged      = ancestor of origin/<default>, or a merged PR/MR exists for it
#   mine        = author email of the branch tip == git config user.email
#   colleague   = anyone else; never deleted unless merged; stale is reported
#   drifted     = behind origin/<default> by >= DRIFT_COMMITS or merge-base older than DRIFT_DAYS
#   default branch, the current branch and protected names are never deleted
#   never pushes anything but branch deletions; never force-moves main
set -euo pipefail

DRY=0; STALE_DAYS=90; DRIFT_COMMITS=50; DRIFT_DAYS=30
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)        DRY=1; shift ;;
        --stale-days)     STALE_DAYS="$2"; shift 2 ;;
        --drift-commits)  DRIFT_COMMITS="$2"; shift 2 ;;
        --drift-days)     DRIFT_DAYS="$2"; shift 2 ;;
        -h|--help)        sed -n '2,25p' "$0"; exit 0 ;;
        *) echo "unknown flag: $1" >&2; exit 2 ;;
    esac
done

git rev-parse --git-dir >/dev/null 2>&1 || { echo "not a git repo" >&2; exit 3; }
git remote get-url origin >/dev/null 2>&1 || { echo "no origin remote" >&2; exit 3; }

git fetch --all --prune --quiet

default="$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|^refs/remotes/origin/||' || true)"
if [[ -z "$default" ]]; then
    for c in main master develop; do
        git show-ref --verify --quiet "refs/remotes/origin/$c" && { default="$c"; break; }
    done
fi
[[ -n "$default" ]] || { echo "cannot determine default branch" >&2; exit 3; }
base="origin/$default"

current="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)"
me="$(git config user.email || true)"
now="$(date +%s)"

url="$(git remote get-url origin)"
host=""
case "$url" in
    *github.com*) command -v gh >/dev/null 2>&1 && host=github ;;
    *gitlab*)     command -v glab >/dev/null 2>&1 && host=gitlab ;;
esac

run() { if [[ $DRY -eq 1 ]]; then echo "dry-run	$*"; else "$@"; fi; }

protected() { case "$1" in "$default"|main|master|develop|HEAD) return 0 ;; esac; return 1; }

merged_reason() {
    # $1 = ref to test. Prints the reason when merged, nothing otherwise.
    local ref="$1" short="${1#origin/}"
    if git merge-base --is-ancestor "$ref" "$base" 2>/dev/null; then
        echo "merged into $default"; return 0
    fi
    case "$host" in
        github)
            local n; n="$(gh pr list --state merged --head "$short" --json number --limit 1 -q '.[].number' 2>/dev/null || true)"
            [[ -n "$n" ]] && { echo "squash-merged via PR #$n"; return 0; } ;;
        gitlab)
            local j; j="$(glab mr list --state merged --source-branch "$short" --output json -L 1 2>/dev/null || true)"
            [[ -n "$j" && "$j" != "[]" ]] && { echo "squash-merged via MR"; return 0; } ;;
    esac
    return 1
}

days_since() { echo $(( (now - $1) / 86400 )); }

deleted=0; kept=0; drifted=0

# --- local branches -------------------------------------------------------
while IFS= read -r b; do
    [[ -z "$b" ]] && continue
    protected "$b" && continue
    [[ "$b" == "$current" ]] && continue
    if reason="$(merged_reason "$b")"; then
        run git branch -D "$b" >/dev/null
        echo "deleted	local	$b	$reason"; deleted=$((deleted+1))
        continue
    fi
    author="$(git log -1 --format=%ae "$b")"
    who=colleague; [[ "$author" == "$me" ]] && who=mine
    behind="$(git rev-list --count "$b..$base")"
    mb="$(git merge-base "$b" "$base" 2>/dev/null || true)"
    age="?"; [[ -n "$mb" ]] && age="$(days_since "$(git log -1 --format=%ct "$mb")")"
    verdict=ok
    if (( behind >= DRIFT_COMMITS )) || { [[ "$age" != "?" ]] && (( age >= DRIFT_DAYS )); }; then
        verdict="drifted: behind $base by $behind commits, base ${age}d old — rebase before reuse"
        drifted=$((drifted+1))
    fi
    echo "kept	local	$b	$who	behind=$behind	base-age=${age}d	$verdict"; kept=$((kept+1))
done < <(git for-each-ref --format='%(refname:short)' refs/heads/)

# --- remote branches ------------------------------------------------------
while IFS= read -r r; do
    [[ -z "$r" ]] && continue
    b="${r#origin/}"
    protected "$b" && continue
    [[ "$b" == "$current" ]] && continue
    if reason="$(merged_reason "$r")"; then
        run git push origin --delete "$b" --quiet
        echo "deleted	remote	$b	$reason"; deleted=$((deleted+1))
        continue
    fi
    author="$(git log -1 --format=%ae "$r")"
    if [[ "$author" == "$me" ]]; then
        # Mine and unmerged: a local copy already reported it, or it is parked work.
        git show-ref --verify --quiet "refs/heads/$b" && continue
        behind="$(git rev-list --count "$r..$base")"
        echo "kept	remote	$b	mine	behind=$behind	no local copy"; kept=$((kept+1))
        continue
    fi
    last="$(git log -1 --format=%ct "$r")"
    sd="$(days_since "$last")"
    state=fresh; (( sd >= STALE_DAYS )) && state="stale=${sd}d"
    echo "kept	remote	$b	colleague	$state	$author"; kept=$((kept+1))
done < <(git for-each-ref --format='%(refname)' refs/remotes/origin/ | sed 's|^refs/remotes/||' | grep -v '^origin/HEAD$')

# --- default branch sync --------------------------------------------------
if git show-ref --verify --quiet "refs/heads/$default"; then
    ahead="$(git rev-list --count "$base..$default")"
    behind="$(git rev-list --count "$default..$base")"
    if (( ahead == 0 && behind == 0 )); then
        echo "main	synced	$default == $base"
    elif (( ahead == 0 )); then
        if [[ "$current" == "$default" ]]; then
            run git merge --ff-only --quiet "$base"
        else
            run git fetch --quiet origin "$default:$default"
        fi
        echo "main	synced	$default fast-forwarded $behind commits to $base"
    elif (( behind == 0 )); then
        echo "main	ahead	local $default is $ahead commits ahead of $base — not pushed, push it or branch it"
    else
        echo "main	diverged	local $default is $ahead ahead and $behind behind $base — not touched, resolve by hand"
    fi
else
    echo "main	absent	no local $default branch"
fi

echo "summary	deleted=$deleted	kept=$kept	drifted=$drifted"
