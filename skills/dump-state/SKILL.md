---
name: dump-state
description: Emit a complete JSON snapshot of all bugs, slugs, host-state, identities, cooldowns, and config.
when_to_use: When the operator needs a single document containing the entire factory state for debugging or audit. Read-only. (IT-S32, AC32)
audience: operator
---

# dump-state

Read-only. Walks every bug, slug, `infra/host-state/*.json`, `infra/identities.json`, `agents/chopper2/reports/notify-cooldowns.json`, `config.yml`. Emits one valid JSON object on stdout.
