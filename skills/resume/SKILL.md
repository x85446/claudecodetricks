---
name: resume
description: Set config.yml::paused = false; commits + pushes.
when_to_use: When the operator wants to resume cron-driven work after a pause. Reverses `pause`. Idempotent.
audience: operator
---

# resume

Flips `config.yml::paused: false`; commits + pushes. Idempotent: no-op if already not paused.
