# /bump — Gravhl Mobile Release Automation

Bundle changes, bump version, generate changelog, commit, and push to `dev` branch for Gravhl Mobile releases. Bryan merges `dev` -> `main` via GitLab MR.

## Overview

The `/bump` skill runs a 5-phase release workflow combining the web project's `/bunch` (WIP bundling) and `/bump` (clean release) patterns into a single command. It handles everything from scanning uncommitted changes through pushing to the `dev` branch.

**Key difference from web:** The web project has separate `/bunch` (bundle WIP) and `/bump` (clean release) skills. Mobile combines both into `/bump` since the workflow is simpler: work on `dev`, push to `dev`, merge to `main` on GitLab.

## Prerequisites

Before running `/bump`:
- Must be on `dev` branch (will prompt to switch if on `main`)
- `package.json` exists with a valid `version` field
- `CHANGELOG.md` exists
- `src/lib/version.ts` exists
- `src/lib/changelog.ts` exists
- `src/lib/changelog-types.ts` exists

## Trigger

```
/bump
```

User-invocable. Can also be triggered by keywords: bump, version, release, changelog.

---

## Phase 1: Scan & Analyze

**Goal:** Understand what's changed and prepare for bundling.

### Steps

1. **Check current branch**:
   ```bash
   git branch --show-current
   ```
   - If on `main`: prompt to switch to `dev` (`git checkout dev`)
   - If on `dev`: proceed
   - If on another branch: ask user if they want to continue or switch to `dev`

2. **Scan for uncommitted changes**:
   ```bash
   git status --porcelain
   git diff --stat
   git diff --cached --stat
   ```

3. **Scan for committed-but-unpushed changes**:
   ```bash
   git log origin/dev..HEAD --oneline 2>/dev/null
   ```
   (If `origin/dev` doesn't exist yet, all local commits are new)

4. **Check for merge conflicts**:
   ```bash
   git diff --name-only --diff-filter=U
   ```
   If conflicts found, **stop immediately** and tell the user.

5. **Auto-flag suspicious files** (exclude from staging):
   - `.env*` (environment secrets)
   - `*.pem`, `*.key`, `*.p12` (certificates)
   - `credentials*`, `*secret*`, `*token*` (credentials)
   - Files > 5MB

6. **Read current version** from `package.json`

7. **Show summary**:
   ```
   Branch: dev
   Uncommitted: 12 modified, 3 untracked, 1 deleted
   Unpushed commits: 4
   Current version: 0.1.9
   Flagged (excluded): .env.local

   Continue? (y/n)
   ```

8. **Get user confirmation** before proceeding.

---

## Phase 2: Version & Changelog

**Goal:** Bump version and generate changelog entry.

### Steps

1. **Suggest version bump**:
   - If changes contain new screens or major features: suggest **minor** bump
   - Otherwise: suggest **patch** bump
   ```
   Current version: 0.1.9
   Suggested: 0.1.10 (patch)
   Options: [patch] [minor] [custom] [skip]
   ```
   - **Skip**: Bundle and push without version bump (commit message: `bunch: {summary}`)

2. **Analyze all changes** (uncommitted + unpushed commits):
   ```bash
   git diff --stat HEAD
   git log --oneline origin/dev..HEAD 2>/dev/null
   ```
   Read diffs to understand what changed.

3. **Categorize changes** into Keep a Changelog categories:
   - **Added**: New features, screens, components, skills
   - **Changed**: Modifications to existing behavior, refactors, redesigns
   - **Fixed**: Bug fixes, crash fixes, performance fixes
   - **Removed**: Deleted features, deprecated code removal

4. **Generate CHANGELOG.md entry**:
   ```markdown
   ## [X.Y.Z] - YYYY-MM-DD

   ### Added
   - **Feature Name** - Description of what was added

   ### Changed
   - **Component Name** - Description of what changed

   ### Fixed
   - **Bug Name** - Description of what was fixed

   ### Technical Details
   - **Files Created**: N
   - **Files Modified**: N
   - **Stats**: +X / -Y lines
   - **Focus**: Brief focus description
   - **Platforms**: Both (iOS + Android) | iOS | Android
   - **Audit Health**: N/100
   ```

5. **Generate `src/lib/changelog.ts` entry** as a `ChangelogEntry` object matching the TypeScript interface

6. **Show both previews** to user for review/editing

7. **Get user confirmation** or allow edits before proceeding

### Tips for Good Changelogs

- Bold the feature/component name as a label prefix
- Focus on user-visible impact, not implementation details
- Group related commits into single entries
- Include audit health score (read from latest audit report if available)
- Count files created/modified using `git diff --stat`

---

## Phase 3: File Updates

**Goal:** Update version and changelog files atomically.

### Files Modified (in order)

1. **`package.json`** — Update `"version"` field (skip if no version bump)
2. **`CHANGELOG.md`** — Prepend new entry after the header (before previous entries)
3. **`src/lib/changelog.ts`** — Insert new `ChangelogEntry` object at top of `CHANGELOG` array
4. **`src/lib/version.ts`** — Update `APP_VERSION`, `BUILD_DATE`, and `VERSION_SCOPE` (skip if no version bump)

### Validation

After updates, verify:
- `package.json` is valid JSON
- `src/lib/changelog.ts` has no TypeScript syntax errors
- `CHANGELOG.md` renders correctly (eyeball the markdown structure)

### Recovery

If something goes wrong during file updates:
- Git working tree still has the pre-update state
- `git checkout -- <file>` to restore any individual file
- All changes are local until Phase 4 commits them

---

## Phase 3.5: Lint Gate (REQUIRED before commit)

**Goal:** Catch lint errors before they fail CI.

### Steps

1. **Run ESLint across the full project**:
   ```bash
   npm run lint
   ```
   - Warnings are acceptable — the CI pipeline uses `--max-warnings` implicitly via the lint script
   - **Errors (exit code 1) block the release** — fix them before proceeding

2. **If errors are found**:
   - Fix each error (prefix unused vars with `_`, add missing props, etc.)
   - Re-run lint to confirm zero errors
   - Stage the fixed files — they'll be included in the release commit in Phase 4

3. **If clean**: proceed to Phase 4

> **Why this exists:** CI runs `npm run lint` and fails the job on any error. A lint error discovered post-push means a broken pipeline and a follow-up fix commit. Always run lint here first.

---

## Phase 4: Commit & Tag

**Goal:** Stage everything, commit with changelog, and tag.

### Steps

1. **Stage all non-flagged files**:
   ```bash
   git add <file1> <file2> ...
   ```
   Use explicit file paths, NOT `git add -A` or `git add .` (to avoid staging flagged files).

2. **Also stage version files** (if modified):
   ```bash
   git add package.json CHANGELOG.md src/lib/version.ts src/lib/changelog.ts
   ```

3. **Create commit**:
   - If version bumped: `chore(release): vX.Y.Z`
   - If no version bump: `bunch: {short summary of changes}`
   - Always add `Co-Authored-By: Claude <noreply@anthropic.com>`

4. **Create git tag** (only if version was bumped):
   ```bash
   git tag vX.Y.Z
   ```

5. **Show commit details** to user for verification

6. **Print native version reminder** (only if version was bumped):
   ```
   REMINDER: Native versions not auto-updated. Before store submission, manually update:
   - Android: android/app/build.gradle (versionCode + versionName)
   - iOS: ios/GravhlMobile/Info.plist (CFBundleShortVersionString + CFBundleVersion)
   See docs/planning/release-guide.md for details.
   ```

---

## Phase 5: Push to Dev

**Goal:** Push the commit and tag to the `dev` branch on GitLab.

### Steps

1. **Verify branch is `dev`**:
   ```bash
   git branch --show-current
   ```
   If not on `dev`, **stop** — something went wrong.

2. **NEVER push to `main`** — this is an absolute rule. If somehow on `main`, abort.

3. **Show push preview**:
   ```
   Push Preview

   Branch: dev
   Remote: https://gitlab.com/gravhl/gravhl-mobile-rn.git
   Commit: a1b2c3d — chore(release): v0.1.10
   Tag: v0.1.10

   Push to GitLab? (y/n)
   ```

4. **Get user confirmation.**

5. **Push**:
   ```bash
   git push origin dev
   git push origin vX.Y.Z    # Push the tag (only if version was bumped)
   ```
   If `dev` has no upstream yet:
   ```bash
   git push -u origin dev
   ```

6. **On success**:
   ```
   Pushed successfully!

   Branch: dev
   Commit: a1b2c3d
   Version: 0.1.10
   Tag: v0.1.10

   Next step: Create a merge request on GitLab to merge dev -> main
   GitLab: https://gitlab.com/gravhl/gravhl-mobile-rn/-/merge_requests/new?source_branch=dev
   ```

7. **On failure**:
   - Show the error
   - Commit stays local — tell user: "Retry with: `git push origin dev`"
   - Do NOT revert the commit

---

## Rollback

### Before Committing (Phase 3 went wrong)
```bash
git checkout -- package.json CHANGELOG.md src/lib/version.ts src/lib/changelog.ts
```

### After Committing, Before Pushing (Phase 4 completed)
```bash
git tag -d vX.Y.Z          # Delete the tag
git reset --soft HEAD~1     # Undo the commit, keep changes staged
git reset HEAD .            # Unstage changes
git checkout -- .           # Discard all changes
```

### After Pushing (Phase 5 completed)
```bash
git revert HEAD             # Create a revert commit
git push origin dev         # Push the revert
```
Or if you need to remove the tag from remote:
```bash
git push origin --delete vX.Y.Z
```

---

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| No changes (clean working directory, no unpushed commits) | Exit: "Nothing to bump — working directory is clean." |
| Only suspicious/flagged files | Warn: "All files are flagged as suspicious. Nothing to commit." |
| Merge conflicts detected | Stop immediately: "Merge conflicts found in: {files}. Resolve conflicts first." |
| On `main` branch | Prompt to switch to `dev`. Never push to `main`. |
| `origin/dev` doesn't exist yet | Use `git push -u origin dev` to create it. |
| Version file updates fail | Rollback with `git restore`, show error, suggest manual fix. |
| Commit fails (pre-commit hook) | Unstage files, restore version files, show error. |
| Push fails (auth, network) | Keep commit local, show retry command. |

---

## Tool Usage Priority

| Tool | Purpose |
|------|---------|
| **Bash** | Git commands (log, tag, commit, diff, describe, push) |
| **Read** | Read current file contents (package.json, CHANGELOG.md, etc.) |
| **Edit** | Update existing files with new content |
| **Write** | Only if Edit can't handle the change (e.g., prepending to CHANGELOG.md) |
| **Grep** | Search for patterns in git log output |
| **AskUserQuestion** | Confirmations at each phase gate |

---

## Example Run

```
User: /bump

Phase 1: Scan & Analyze
  Branch: dev
  Uncommitted: 8 modified, 2 untracked
  Unpushed commits: 3
  Current version: 0.1.9
  [Continue? y/n]

Phase 2: Version & Changelog
  Suggested bump: patch (0.1.9 -> 0.1.10)
  [Confirm? patch / minor / skip]
  Generated entry with 3 Added, 2 Fixed, 1 Changed
  [Preview shown, confirm or edit]

Phase 3: File Updates
  Updated package.json: 0.1.9 -> 0.1.10
  Updated CHANGELOG.md: prepended new entry
  Updated src/lib/changelog.ts: inserted new ChangelogEntry
  Updated src/lib/version.ts: APP_VERSION = '0.1.10'

Phase 4: Commit & Tag
  Staged 14 files (10 changed + 4 version files)
  Committed: chore(release): v0.1.10
  Tagged: v0.1.10
  REMINDER: Update native versions before store submission

Phase 5: Push to Dev
  Branch: dev -> origin/dev
  Commit: a1b2c3d
  Tag: v0.1.10
  Pushed successfully!
  Next: Create MR on GitLab to merge dev -> main
```
