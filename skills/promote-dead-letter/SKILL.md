---
name: promote-dead-letter
description: Promote a dead-letter slug to a full bug.
when_to_use: When a dead-letter slug is genuine and should be accepted into the bug pipeline despite earlier classifier rejection. Calls `issue-next-id`; routes to leaf. (IT-S9, AC9)
audience: operator
---

# promote-dead-letter

Calls `issue-next-id` to allocate `BUG-NNNNNN`; creates `bugs/BUG-N.json` with `accepted_by: operator` (AC9); removes slug; routes to the leaf indicated by the slug (`repo`); commits + pushes. Idempotent: no-op if a bug already exists with this slug.
