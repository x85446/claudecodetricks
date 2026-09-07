---
name: reopen
description: Reopen a closed bug; restores prior GHLSTATE from event history.
when_to_use: When a closed bug regresses or was closed in error. Reads `prior_state` from the most recent close-type event and restores the bug to that state and folder. (IT-S6)
audience: operator
---

# reopen

`git mv`s the bug from `bugs/closed/` to its prior folder; restores `GHLSTATE` from history; appends `reopened` event with `by: operator, reason`. Idempotent: no-op if already at the prior state.
