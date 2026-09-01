---
name: reassign-bug
description: Reassign a bug to a different leaf agent (operator-only).
when_to_use: When a bug is stuck on the wrong assignee or in escalation_needed and the operator has decided which leaf should own the next attempt. Confirms via AskUserQuestion before moving in-progress bugs.
audience: operator
---

# reassign-bug

Reassigns `BUG-NNNNNN` to a different `<repo>/<role>` leaf agent. Confirms target is in the role allowlist; performs `git mv` from the current location to the target's `from-chopper/` inbox; sets `assignee`, `GHLSTATE: assigned`; appends a `reassigned` event with `by: operator`. Optional `--clear-fix-attempts` resets the attempt counter for `escalation_needed` bugs (T25). Idempotent: if the bug is already at the target, the skill is a no-op. Respects `--dry-run` and `DRY_RUN=1`.
