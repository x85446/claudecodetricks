---
name: unstick-bug
description: Unstick a bug whose skill_retries cap was reached.
when_to_use: When `GHLSTATE == skill_stuck` and the operator has reviewed and is ready to retry. Resets `skill_retries: {}`, sets `GHLSTATE: assigned`, returns the bug to the assignee's `from-chopper/`. (IT-S5/IT-S28)
audience: operator
---

# unstick-bug

Resets `skill_retries: {}` to `{}`; sets `GHLSTATE: assigned`; `git mv`s from `bugs/_blocked/` to the assignee's `from-chopper/`. Idempotent: no-op if `GHLSTATE != skill_stuck`.
