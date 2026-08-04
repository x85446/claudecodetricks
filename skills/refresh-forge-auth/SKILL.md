---
name: refresh-forge-auth
description: Use when glab or gh authentication is broken/expired/missing, when a glab/gh command fails with "unauthorized" or "authentication required", or when someone asks to refresh, re-auth, or login to GitLab/GitHub CLI.
argument-hint: [gitlab|github|both] [--force]
allowed-tools: Bash, Read
---

## What This Skill Does

Refreshes local `glab` and/or `gh` CLI authentication using long-lived super-tokens stored in `~/.ssh/`. Tokens are stored locally; cypressMini holds the canonical copies as a fallback.

## When To Run

- User says: "refresh glab/gh auth", "re-login to gitlab/github CLI", "fix glab unauthorized"
- During a task, a `glab` or `gh` command fails with `401`, `unauthorized`, `authentication required`, or `Token has been revoked`
- A `glab api ...` returns `{"message":"401 Unauthorized"}`
- Auto-invocable: yes (skill is idempotent — re-auths only if status check fails, unless `--force`)

## Arguments

- `$1` — `gitlab` | `github` | `both` (default: `both`)
- `$2` — `--force` to re-auth even if `auth status` currently passes

## Token Sources

| Forge | Primary (local) | Fallback (cypressMini) |
|---|---|---|
| GitLab | `~/.ssh/gitlab-super-token` | `ssh cypressMini "cat ~/.ssh/gitlab-super-token"` |
| GitHub | `~/.ssh/github_personal_access_token_superuser` | `ssh cypressMini "cat ~/.ssh/github_personal_access_token_superuser"` |

Both token files are 1 line, no whitespace. Never echo their contents to the user, to logs, or to disk in any other location.

## Steps

### Step 1 — Parse arguments

- `target=${1:-both}` → one of `gitlab`, `github`, `both`
- `force=$([[ "$*" == *--force* ]] && echo yes || echo no)`

### Step 2 — For each target forge, run the refresh subroutine

#### GitLab subroutine

1. **CLI present?** `command -v glab >/dev/null` — if not, report `glab not installed; install with: brew install glab` (macOS) or download from https://gitlab.com/gitlab-org/cli/-/releases (linux). Skip to next forge.
2. **Already authed?** If `force=no`, run `glab auth status 2>&1 | grep -q "Logged in"` — if it passes, print `glab: already authenticated` and skip.
3. **Resolve token:**
   - If `[[ -r ~/.ssh/gitlab-super-token ]]`, set `TOKEN_SRC=local`
   - Else `ssh -o BatchMode=yes -o ConnectTimeout=5 cypressMini "test -r ~/.ssh/gitlab-super-token"` — if it succeeds, set `TOKEN_SRC=cypressMini`
   - Else report `gitlab: token file not found locally or on cypressMini` and skip
4. **Authenticate** — DO NOT capture token in a shell variable that the model sees. Pipe directly:
   - Local: `glab auth login --hostname gitlab.com --git-protocol ssh --stdin < ~/.ssh/gitlab-super-token`
   - Remote: `ssh cypressMini "cat ~/.ssh/gitlab-super-token" | glab auth login --hostname gitlab.com --git-protocol ssh --stdin`
5. **Verify:** `glab auth status` — must show `Logged in to gitlab.com`. Print `gitlab: refreshed via $TOKEN_SRC` on success.

#### GitHub subroutine

1. **CLI present?** `command -v gh >/dev/null` — if not, attempt install:
   - `apt-get` available + sudoers: `sudo apt-get install -y gh` (after adding the GitHub CLI apt repo if missing)
   - Otherwise report `gh not installed; install with: see https://github.com/cli/cli#installation` and skip
2. **Already authed?** If `force=no`, run `gh auth status 2>&1 | grep -q "Logged in to github.com"` — if it passes, print `gh: already authenticated` and skip.
3. **Resolve token** (same pattern as glab, paths swapped to `~/.ssh/github_personal_access_token_superuser`).
4. **Authenticate:**
   - Local: `gh auth login --hostname github.com --git-protocol ssh --with-token < ~/.ssh/github_personal_access_token_superuser`
   - Remote: `ssh cypressMini "cat ~/.ssh/github_personal_access_token_superuser" | gh auth login --hostname github.com --git-protocol ssh --with-token`
5. **Verify:** `gh auth status` — must show `Logged in to github.com`. Print `github: refreshed via $TOKEN_SRC` on success.

### Step 3 — Summary

Print one line per forge handled:

```
gitlab: <refreshed via local | already authenticated | skipped: <reason> | failed: <reason>>
github: <refreshed via cypressMini | already authenticated | skipped: <reason> | failed: <reason>>
```

Exit nonzero if any target failed.

## Auto-Invocation Detection

When a tool call output contains any of these substrings, this skill is a candidate:
- `glab: error: ... 401`
- `Unauthorized` from a `glab` or `gh` command
- `Token has been revoked`
- `authentication required`
- `gh: To get started ... gh auth login`
- `You are not logged in`

If detected, invoke once with default args before retrying the failing command. Do not loop — if the refresh succeeds and the original command still fails, treat it as a non-auth issue.

## Guardrails

- **Never echo token contents.** No `cat ~/.ssh/...` to the chat. No `echo $TOKEN`. Only pipe directly into the CLI's stdin.
- **Token file permissions:** if `stat -c %a ~/.ssh/gitlab-super-token` is wider than `600`, warn the user (do not modify).
- **No retry storms.** One auth attempt per forge per invocation. If it fails, surface the error and stop.
- **No silent re-auth.** Always print one summary line per forge so the user can see what happened.
- **SSH fallback timeout:** `ssh -o BatchMode=yes -o ConnectTimeout=5` — never hang waiting for a password prompt.
- **Force flag is opt-in.** Default behavior is idempotent: skip if `auth status` already passes.

## Notes

- The canonical GitLab refresh pattern was discovered by reading cypressMini's `~/.zshrc.d/40-utils.sh` `glabauth()` function: `glab auth login --hostname gitlab.com --git-protocol ssh --stdin < ~/.ssh/gitlab-super-token`. This skill mirrors that exactly, plus a local-first lookup and a github equivalent.
- Both super-tokens are PATs with broad scopes (`api`, `read_registry`, `write_registry`, `read_repository`, `write_repository` for GitLab). Treat them with the same care as a root SSH key.
- This skill does NOT rotate the tokens themselves — only refreshes CLI auth using the existing tokens. To rotate, do it manually in each forge's UI and update both the local file and cypressMini's copy.
