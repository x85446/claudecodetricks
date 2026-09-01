---
name: list-dead-letter
description: List slugs in slugs/_dead_letter/.
when_to_use: When the operator wants to inspect slugs that bounced past `slug_max_bounces` and require manual triage. Read-only.
audience: operator
---

# list-dead-letter

Read-only. Walks `slugs/_dead_letter/`. Prints `{slug, filed_by, bounce_count, last_reason}` per line. (IT-S9)
