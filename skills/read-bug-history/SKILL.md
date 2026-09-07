---
name: read-bug-history
description: Return the events[] array for a bug, optionally filtered by type. Read-only. Used to understand prior fix attempts and tester feedback before starting work.
when_to_use: After claim-bug, before writing any code — especially on reassigned bugs where fix_attempts > 0.
---

# read-bug-history

Read-only view of a bug's history.

## Inputs

- `<BUG-NNNNNN>` (positional)
- `--event-type <name>` — filter by `type` field
- `--limit <n>` — default 10; newest-first
- `--from-dir <name>` — default `from-chopper`

## Behavior

Reads `<cwd>/<from-dir>/<BUG-NNNNNN>.json`. Outputs a JSON array of matching events (newest first), respecting `--limit`.

## GHLSTATE transition

Read-only.

## Scenarios

S26 (surfacing prior auto-recovery events), S3/S4 (coder reads tester feedback).
