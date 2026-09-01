---
name: claim-bug
description: Transition a bug from assigned/reassigned to in_progress. Edits the bug file in-place in from-chopper/. Does not move the file.
when_to_use: Immediately after read-inbox returns a bug and the agent decides to work on it. Must be called before any code changes are made.
---

# claim-bug

Mark a bug as actively being worked on by this leaf.

## Inputs

- `<BUG-NNNNNN>` (positional)
- `--from-dir <name>` — default `from-chopper`
- `--actor <id>` (optional)
- `--dry-run`

## Behavior

- Loads `<cwd>/<from-dir>/<BUG-NNNNNN>.json`.
- Sets `ghlstate=in_progress`, `current_state.since=<now>`.
- Appends a `claimed` event.
- Surfaces `related_bugs` summaries on stdout (so the agent does not have to scan).
- Does NOT git mv the file.

## GHLSTATE transition

`assigned / reassigned / awaiting_verify (re-claim) → in_progress`

Idempotent: re-claiming an already-`in_progress` bug appends an event and re-stamps `since` but does not error.

## Scenarios

S1, S3 (re-claim after needs_info).
