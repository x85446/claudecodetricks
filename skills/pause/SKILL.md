---
name: pause
description: Set config.yml::paused = true; commits + pushes.
when_to_use: When the operator wants to halt all cron-driven work cluster-wide. (IT-S30, AC30)
audience: operator
---

# pause

Flips `config.yml::paused: true`; commits + pushes. All cron ticks on all hosts exit 0 without work. Idempotent: no-op if already paused.
