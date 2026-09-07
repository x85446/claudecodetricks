---
name: update-bug
description: Edit a bug file in-place (while it is in the leaf's from-chopper/). Used by the tester to set needs_info on validation failure, or by either role to record intermediate notes. Does NOT move the file unless --ghlstate=needs_info.
when_to_use: After running tests and finding the fix insufficient, or to add investigative notes mid-cycle. Use mark-fixed / mark-verified to signal done.
---

# update-bug

Edit a bug record without changing its location (default), or set `needs_info` to bounce it back to the coder.

## Inputs

- `<BUG-NNNNNN>` (positional, required)
- `--ghlstate <state>` — only `needs_info` accepted from tester path; missing means "no state change"
- `--note <text>` — appended as a `note` event
- `--reason <text>` — populates `ghlstatereason` when `--ghlstate` is supplied
- `--infra-failure` — record an `infra_failure` event without changing GHLSTATE; leaves bug in `from-chopper/` for next-cycle retry
- `--from-dir <name>` — default `from-chopper`; pass `human-coding` for TUI flows
- `--actor <id>` — `human:email@example.com` for TUI; omit for AI
- `--dry-run`

## Behavior

- Loads `<cwd>/<from-dir>/<BUG-NNNNNN>.json`.
- Appends an event entry with `ts`, `by`, optional `actor`, and the supplied note/reason.
- When `--ghlstate=needs_info`, sets `ghlstate`, populates `ghlstatereason`, increments nothing, and `git mv`s the file from the from-dir into `to-chopper/`.
- When `--infra-failure`, only appends the event; ≥2 consecutive `infra_failure` events surface as `notify(infra_failure_persistent)` upstream.

## GHLSTATE transition

`awaiting_verify → needs_info` (tester path only). No-op for `--infra-failure`.

## Scenarios

S3 (needs_info round-trip), S4 (test-failed retry).
