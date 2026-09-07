---
name: reject-bug
description: Signal that the bug does not belong to this leaf's repo. Provide an optional suggested_repo so chopper2 can reroute rather than close.
when_to_use: When the coder or tester determines the bug is for a different repo and can name the correct one. Do NOT use to fail validation — use update-bug.
---

# reject-bug

Bounce a bug back to chopper2 so it can be re-routed.

## Inputs

- `<BUG-NNNNNN>` (positional)
- `--reason <text>` (required)
- `--suggested-repo <name>` (optional)
- `--from-dir <name>` — default `from-chopper`
- `--actor <id>` (optional)
- `--dry-run`

## Behavior

- Loads `<cwd>/<from-dir>/<BUG-NNNNNN>.json`.
- Sets `ghlstate=rejected_by_leaf`, `ghlstatereason=<reason>`.
- If `--suggested-repo` is supplied, writes `suggested_repo` field and includes it in the event.
- Appends a `rejected_by_leaf` event.
- `git mv`s the bug file from the from-dir to `to-chopper/`.

## GHLSTATE transition

`in_progress / awaiting_verify → rejected_by_leaf`

Chopper2 picks up; after `limits.routing_max_reroutes` exceeded → `escalation_needed`.

## Scenarios

S7 (wrong-repo reroute path).
