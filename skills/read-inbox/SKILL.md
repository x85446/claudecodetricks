---
name: read-inbox
description: List and return the contents of the leaf's from-chopper/ directory. Returns JSON array of {path, bug_id_or_slug, ghlstate, priority, age_seconds}. Skips bugs whose blocked_by[] contains any non-closed/non-verified entry.
when_to_use: First call in every leaf cycle, before doing any other work.
---

# read-inbox

Surface what is in the leaf's `from-chopper/` directory so the cycle script can decide what to do.

## Inputs

- `--cwd <path>` (optional override; defaults to process cwd)
- `--include-blocked` — include items whose blockers are still open (default: skip)

## Behavior

- Operates on `<cwd>/from-chopper/`.
- For each `*.json` file, parses it and reads `id` / `slug`, `ghlstate`, `priority`, file mtime.
- Resolves blockers by reading `bugs/BUG-NNNNNN.json` from the agent-comms repo; an entry is "resolved" when its `ghlstate` is `verified` or `closed`.
- Emits a JSON array on stdout: `[{path, bug_id_or_slug, ghlstate, priority, age_seconds, blocked, blockers_open}]`.

The folder is expected to contain at most one bug at a time (busy-signal, §12).

## GHLSTATE transition

Read-only.

## Scenarios

S1, S11 (busy-signal observation).
