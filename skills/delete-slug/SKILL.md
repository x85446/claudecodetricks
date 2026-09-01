---
name: delete-slug
description: Permanently remove a slug file from the repo.
when_to_use: When a slug is genuinely garbage and should not be retried. Idempotent: no-op if already removed. (IT-S9)
audience: operator
---

# delete-slug

`git rm` slug file in any location; commits + pushes. Idempotent: no-op if file already absent. Should be used after AskUserQuestion confirmation.
