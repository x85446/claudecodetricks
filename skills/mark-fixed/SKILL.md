---
name: mark-fixed
description: Signal that a fix branch has been pushed to origin. Sets GHLSTATE to awaiting_verify, records fix_branch and fix_commit, moves the bug from from-chopper/ (or human-coding/) to to-chopper/.
when_to_use: After pushing the fix branch and confirming it is visible on the remote. Never call before the branch exists on origin.
---

# mark-fixed

Coder-side signal that a fix is pushed and ready for verification.

## Inputs

- `<BUG-NNNNNN>` (positional)
- `--fix-branch <name>` (default `fix/<BUG-NNNNNN>`)
- `--fix-commit <sha>` (required)
- `--from-dir <name>` — default `from-chopper`; TUI flows pass `human-coding`
- `--as-human` — record actor and tag event accordingly
- `--actor <id>` — `human:email@example.com` when --as-human
- `--target-repo-path <path>` — default `/opt/repos/<repo>`; the repo where the fix branch lives
- `--repo <name>` — required when `--target-repo-path` is omitted (used to compose the default)
- `--dry-run`

## Precondition (§3)

`git ls-remote origin refs/heads/<fix-branch>` must return a non-empty result inside the target repo. If the branch is missing on origin, the skill exits non-zero with `fix branch <fix-branch> not found on origin — push before calling mark-fixed`.

The skill does NOT push to `main`; coders never touch `main` directly. Branch protection on the GitLab side is the actual enforcement boundary (§13).

## Behavior

- Verifies the fix branch is on origin.
- Loads the bug from `<cwd>/<from-dir>/<BUG-NNNNNN>.json`.
- Sets `ghlstate=awaiting_verify`, fills `fix_branch`, `fix_commit`.
- Appends an event `{type: "fixed", by, fix_branch, fix_commit, actor?}`.
- `git mv`s the bug file from from-dir into `to-chopper/`.

## GHLSTATE transition

`in_progress → awaiting_verify`

## Scenarios

S1, S3, S4 (main coder→tester signal); S12 (human-done path); S33, S34.
