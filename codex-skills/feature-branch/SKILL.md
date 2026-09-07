---
name: feature-branch
description: "Enforce the Atlassian feature-branch workflow for GitHub and GitLab repos: branch off the default branch, work, open a PR/MR, and delete the local + remote branch once merged. Use when the user says 'start a feature', 'fix a bug', 'work on X', 'open a PR', 'open an MR', 'merge this branch', 'clean up branches', 'prune branches', or runs `$feature-branch`. Also: BEFORE any code edit (Edit/Write) in a git repo, this skill must check the current branch — if on the default branch (main/master/develop), block the edit and create a feature/bugfix branch first."
---


## What This Skill Does

## Usage

Argument: <start|finish|cleanup|status> [feature|bugfix] [slug-or-description]. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

Keeps git working trees clean by enforcing one workflow for everyone:

1. **Start** on the default branch, pull, create `feature/<slug>` or `bugfix/<slug>`.
2. **Work** on that branch only — never on `main`/`master`/`develop`.
3. **Finish** — push, open a GitHub PR or GitLab MR, and (once merged) delete the local + remote branch and switch back to the default branch.
4. **Cleanup** — periodically prune any merged-but-still-present local branches and dead remote-tracking refs.

Bugs use the same lifecycle as features; the only difference is the `bugfix/` prefix.

## When This Skill Fires

- Direct: `$feature-branch <subcommand> [args]`
- Intent triggers (the `description` field above is what makes auto-invoke work):
  - "let's start a new feature for ...", "add a feature that ...", "build the X feature"
  - "fix the bug where ...", "fix this bug", "work on bug Y"
  - "open a PR for this", "create an MR", "ship this", "merge this branch"
  - "clean up branches", "prune branches", "delete old branches"
- **Pre-edit gate**: any Edit/Write in a git repo. Before the edit lands, verify branch — see [Pre-Edit Gate](#pre-edit-gate) below.

## Subcommands

| Subcommand | Args | What it does |
|---|---|---|
| `start` | `[feature\|bugfix] <slug-or-description>` | Switch to default branch, pull, create + checkout new branch. |
| `finish` | none | Push current branch, ensure PR/MR exists, and if merged → delete branch + return to default. |
| `cleanup` | none | Sweep local branches that are merged into the default branch (or squash-merged via a closed PR/MR), delete them locally and remotely, prune stale refs. |
| `status` | none | Show current branch, PR/MR state if any, count of stale local branches. |

If the skill auto-invokes on intent without an explicit subcommand, infer:
- "start/build/add a feature for X" → `start feature X`
- "fix the bug ..." → `start bugfix X`
- "open a PR / MR / merge this" → `finish`
- "clean up / prune branches" → `cleanup`

## Workflow

### Step 0 — Always: detect repo state

Before any subcommand, capture:

```bash
git rev-parse --git-dir              # confirm it's a git repo
git rev-parse --abbrev-ref HEAD      # current branch
git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|^refs/remotes/origin/||'  # default branch
git remote get-url origin            # remote URL → determines gh vs glab
```

If `git rev-parse --git-dir` fails: tell the user "not a git repo" and stop. Do not run `git init` unprompted.

Default-branch fallback: if `origin/HEAD` is not set, check (in order) `main`, `master`, `develop`. If none exist, ask the user.

Remote-type detection from `origin` URL:
- contains `github.com` → use `gh`
- contains `gitlab` (gitlab.com, self-hosted gitlab.example.com) → use `glab`
- anything else → skip PR/MR creation; tell the user "Pushed, but neither `gh` nor `glab` matches the remote. Open the PR/MR manually."

### Step 1 — `start`

1. **Parse args.**
   - If the first arg is literally `feature` or `bugfix`, that's the type. The rest is the description.
   - Otherwise infer type from wording in the description: contains `fix|bug|hotfix|defect|broken` → `bugfix`; else `feature`.
   - Slugify the description: lowercase; replace anything that isn't `[a-z0-9]` with `-`; collapse repeated `-`; trim leading/trailing `-`; truncate to 40 chars. Strip trailing partial word.
   - Final branch name: `<type>/<slug>`.
2. **Verify we're on the default branch.** If not, ask the user:
   - "Currently on `<current>`. Switch to `<default>` first? (Y/n)"
   - On `Y` (default): `git checkout <default>`.
   - On `n`: ask whether to branch off the current branch instead.
3. **Pull.** `git pull --ff-only`. If that fails (diverged), surface the error and stop — do not auto-merge or rebase without asking.
4. **Working tree clean?** If `git status --porcelain` is non-empty:
   - Ask: "Working tree has uncommitted changes. Stash them onto the new branch? (Y/n)"
   - On `Y`: create the branch then commit/leave changes there.
   - On `n`: stop, let the user resolve.
5. **Create + checkout.** `git checkout -b <type>/<slug>`. If branch exists locally, ask before reusing.
6. **Confirm.** Print the new branch name and: "On `<branch>`. Work, commit, then run `$feature-branch finish` when ready."

### Step 2 — `finish`

1. **Refuse on default branch.** If current branch is `main`/`master`/`develop` (or the detected default), say: "Already on the default branch — nothing to finish. Run `$feature-branch start` to begin work." and stop.
2. **Working tree clean?** If uncommitted: prompt to commit (recommend running existing commit hooks/skills if any). Don't auto-commit silently. If user declines, stop.
3. **Push.** `git push -u origin <branch>` (the `-u` is a no-op if upstream already set).
4. **Look for an existing PR/MR.**
   - GitHub: `gh pr view --json state,url,title 2>/dev/null` (queries by current branch) — capture state.
   - GitLab: `glab mr view --output json 2>/dev/null` — capture state.
5. **If none exists, create one.**
   - Title: derive from branch slug (replace `-` with space, capitalize first word, prefix with `[fix]` for bugfix). Or, if there's exactly one commit, use that commit's subject.
   - Body: `git log <default>..HEAD --pretty=format:'- %s'` (bulleted list of commit subjects), then a final line: "🤖 Created via $feature-branch."
   - GitHub: `gh pr create --base <default> --head <branch> --title "..." --body "..."`
   - GitLab: `glab mr create --source-branch <branch> --target-branch <default> --title "..." --description "..." --yes`
   - Capture the URL.
6. **Handle the PR/MR state.**
   - `OPEN`: print URL, exit. Remind user: "Once merged, run `$feature-branch finish` again to clean up, or `$feature-branch cleanup` to sweep all merged branches."
   - `MERGED`: proceed to deletion (next step).
   - `CLOSED` (not merged): ask "PR/MR was closed without merging. Delete the branch anyway? (y/N)". On `N`, exit.
7. **Delete the merged branch.**
   - `git checkout <default> && git pull --ff-only`
   - `git branch -d <branch>` (use `-D` if `-d` refuses because squash-merge made history diverge)
   - `git push origin --delete <branch>` (ignore failure if remote already deleted)
   - Print: "Deleted `<branch>` (local + remote). Now on `<default>`."

### Step 3 — `cleanup`

1. **Refresh refs.** `git fetch --all --prune` — `--prune` deletes remote-tracking refs whose remote branch no longer exists.
2. **Determine the default branch** (Step 0 logic).
3. **Find merged branches.**
   - `git for-each-ref --format='%(refname:short)' refs/heads/` → all locals.
   - Exclude: the default branch, `master`, `develop`, the current branch.
   - For each remaining branch:
     - Fast path: `git merge-base --is-ancestor <branch> origin/<default>` → exit 0 means fully merged.
     - Squash-merge path: if fast path fails, check whether a merged PR/MR exists for the branch:
       - GitHub: `gh pr list --state merged --head <branch> --json number --limit 1 -q '.[].number'` → if non-empty, treat as merged.
       - GitLab: `glab mr list --state merged --source-branch <branch> --output json -L 1` → if non-empty, merged.
4. **Summarize and confirm.**
   - Show the list of candidates with reasons ("merged into main", "squash-merged via PR #123"). If the list is empty, say so and exit.
   - Ask: "Delete these N branches (local + remote where applicable)? (y/N)" Default no.
5. **Delete.** For each:
   - `git branch -d <branch>` then fall back to `-D` if the fast-path check passed for a squash-merge.
   - `git push origin --delete <branch>` only if the remote branch still exists (`git ls-remote --exit-code --heads origin <branch>`). Don't error on already-gone branches.
6. **Report.** "Deleted X branches. Pruned Y stale remote-tracking refs."

### Step 4 — `status`

1. Show current branch + whether it matches the `feature/` or `bugfix/` convention.
2. If on a non-default branch: query PR/MR state (Step 2 step 4). Print URL + state.
3. Count merged-but-present local branches using Step 3's detection (don't delete; just count).
4. If count > 0: suggest `$feature-branch cleanup`.

## Pre-Edit Gate

Before any `Edit`, `Write`, or `NotebookEdit` on a file inside a git repo:

1. Quick check: `cd <file-dir> && git rev-parse --git-dir 2>/dev/null` — if not a repo, **proceed normally** (no gate applies).
2. `git rev-parse --abbrev-ref HEAD` — current branch.
3. Detect default branch (Step 0). If the current branch matches the default branch (or is one of `main`/`master`/`develop`):
   - **Halt the edit.** Tell the user: "You're on `<current>`. The feature-branch skill enforces working on a feature/bugfix branch. Suggested: `$feature-branch start <inferred type> <inferred slug from the request>`. Approve (Y), pick a different name (n), or override and edit on `<current>` (override)?"
   - On `Y`: run `start` with the suggested args, then proceed with the edit.
   - On override: proceed with the edit and remember the override for the rest of the session (don't re-prompt for the same repo).
4. If the current branch is already a feature/bugfix branch: proceed normally, no prompt.

**Override scope.** An override applies to the current session and the current repo only. Don't generalize.

**Don't gate on:**
- Read-only operations (Read, Bash with read-only commands, etc.).
- Edits outside a git repo.
- Edits to files matched by `.gitignore` (use `git check-ignore -q <file>`; exit 0 = ignored, skip the gate).
- Edits when the user has explicitly said "I'm working directly on main this time" earlier in the session.

## Implementation Notes

### Slug generation example

User says: "Let's fix the bug where the login button doesn't respond on mobile Safari."

- Type: `bugfix` (contains "fix" + "bug").
- Description: "login button doesn't respond on mobile Safari".
- Slug: `login-button-doesn-t-respond-on-mobile-safari` → truncate at 40 chars → `login-button-doesn-t-respond-on-mobile-s` → trim partial word → `login-button-doesn-t-respond-on-mobile`.
- Branch: `bugfix/login-button-doesn-t-respond-on-mobile`.

If the slug looks awkward after truncation, ask: "Branch name: `bugfix/login-button-doesn-t-respond-on-mobile`. Use this or pick a shorter one?"

### PR/MR body generation

Aim for useful, not exhaustive:

```
- <commit subject 1>
- <commit subject 2>
- <commit subject 3>

🤖 Created via $feature-branch.
```

If there's only one commit and its subject already describes the change, use that as the PR/MR title and put the commit body (if any) in the description.

### Detecting squash-merge

Squash-merging makes `git merge-base --is-ancestor` return false (the commits literally aren't ancestors of `main` anymore). The reliable signal is the PR/MR state:

- `gh pr list --state merged --head <branch>` returns the merged PR if one exists.
- `glab mr list --state merged --source-branch <branch>` does the same for GitLab.

Use this as the second check in cleanup. Don't try to compare diffs — too brittle.

### Tool availability

- `gh` / `glab` may not be installed. Check with `command -v gh` / `command -v glab`. If missing:
  - For `finish`: push succeeded; tell the user the push went through and they need to open the PR/MR manually. Don't fail the whole command.
  - For `cleanup` (squash-merge detection): fall back to fast-path merged check only. Note in the summary: "Squash-merged branches not detected — install `gh`/`glab` for full sweep."
- Authentication: don't try to `gh auth login` / `glab auth login` — those are interactive. If unauthenticated, surface the error and suggest the user run them manually.

### What NOT to do

- **Never force-push** (`--force`, `--force-with-lease`) unless the user explicitly asks.
- **Never delete an unmerged branch** without confirmation. The `-d` (lowercase) flag is the safety check; only fall back to `-D` when the fast-path check confirmed the merge.
- **Never commit on the user's behalf during `finish`.** If the working tree is dirty, ask. Commit creation is the user's call (or the `git-committer` hook's, if installed).
- **Never auto-rebase or auto-merge.** If `git pull --ff-only` fails, surface and stop.
- **Never run `gh auth` / `glab auth`** commands — they're interactive and credential-touching.
- **Don't gate edits in non-git directories** or on `.gitignored` files. The gate is for tracked source.
- **Don't touch other repos.** Operate only on the repo of the current working directory.

## Common Scenarios

**User asks to add a new feature:**
> "Let's add a dashboard widget for active users."
- Auto-invoke → infer `start feature add dashboard widget for active users` → branch `feature/add-dashboard-widget-for-active-users`.
- Skill creates the branch, then the assistant proceeds with the implementation.

**User asks to fix a bug while on main:**
> "Fix the typo in the README."
- Pre-edit gate fires: on `main`, edit blocked → suggest `bugfix/fix-typo-in-readme`.
- After user confirms, branch is created, then the edit lands on the new branch.

**User finished work and wants to ship:**
> "Open a PR for this." or just `$feature-branch finish`.
- Push, create PR/MR, print URL.
- Next time the user runs `finish` (after merge in browser), it deletes the branch.

**User notices their local repo is cluttered:**
> "Clean up old branches." or `$feature-branch cleanup`.
- Fetch+prune, list merged branches, confirm, delete local + remote.

**One-off edit on main is genuinely intended:**
> "Just fix this on main, it's a one-line typo and I don't want a PR."
- User overrides the gate with "override" → edit proceeds on main, no further prompts for this session in this repo.
