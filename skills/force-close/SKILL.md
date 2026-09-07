---
name: force-close
description: Force-close a bug regardless of current state (operator-only).
when_to_use: When a bug should be terminated immediately — out of scope, duplicate decision, won't-fix — and normal close workflow does not apply. Confirms via AskUserQuestion before acting on non-closed bugs.
audience: operator
---

# force-close

Moves any bug to `bugs/closed/`; sets `GHLSTATE: closed`; records `closed_reason`; appends `force_closed` event with `actor: operator`; fires `notify(bug_closed)`. Idempotent: no-op if `GHLSTATE == closed`. Respects `--dry-run` / `DRY_RUN=1`. (IT-S7)
