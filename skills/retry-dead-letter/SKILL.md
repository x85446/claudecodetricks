---
name: retry-dead-letter
description: Reset bounce_count and re-emit a dead-letter slug to chopper2.
when_to_use: When a dead-letter slug should be given another chance through the chopper2 pipeline.
audience: operator
---

# retry-dead-letter

Resets `bounce_count: 0`; `git mv`s slug from `slugs/_dead_letter/` to `agents/chopper2/to-chopper/`; appends `retried` event. Idempotent: no-op if slug is not in `_dead_letter/`. (IT-S9)
