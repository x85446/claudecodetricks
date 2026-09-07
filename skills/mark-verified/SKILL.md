---
name: mark-verified
description: Signal that the fix has passed all validation. Rebases fix branch onto main and fast-forward merges it, then deletes the branch from origin and local. Sets GHLSTATE to verified, records merge_commit, moves bug to to-chopper/.
when_to_use: After all validation passes (including private crossbugs). Never call if any test failed — use update-bug with needs_info instead.
---

# mark-verified

Tester-side signal that a fix has been validated and merged.

## Inputs

- `<BUG-NNNNNN>` (positional)
- `--from-dir <name>` — default `from-chopper`; TUI flows pass `human-testing`
- `--as-human` — record actor and tag event accordingly
- `--actor <id>` — `human:email@example.com` when --as-human
- `--target-repo-path <path>` — default `/opt/repos/<repo>`
- `--repo <name>` — required when `--target-repo-path` is omitted
- `--fix-branch <name>` — default `fix/<BUG-NNNNNN>`
- `--target-branch <name>` — default `main`
- `--dry-run`

## Behavior (§3)

In the target repo:

1. `git fetch origin`
2. `git checkout <fix-branch>`
3. `git rebase origin/<target-branch>` — on conflict, the skill aborts with an error pointing to `update-bug --ghlstate needs_info` (§7).
4. `git checkout <target-branch> && git merge --ff-only <fix-branch>`
5. `git push origin <target-branch>`
6. `git push origin --delete <fix-branch>`
7. `git branch -d <fix-branch>`

Then in the agent-comms repo: sets `ghlstate=verified`, fills `merge_commit`, appends a `verified` event, and `git mv`s the bug from from-dir into `to-chopper/`.

## GHLSTATE transition

`awaiting_verify → verified`

## Scenarios

S1, S3 (success); S33 (ff-merge attribution).
