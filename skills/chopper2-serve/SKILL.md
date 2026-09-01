---
name: chopper2-serve
description: Launch the local dashboard daemon at 127.0.0.1:7777.
when_to_use: By systemd at host start; do not call from cron cycle.
---

# chopper2-serve

Thin skill wrapper that exec()s the actual dashboard binary at
`target/release/chopper2-serve` (built from `agents/chopper2/dashboard/`).
Skill-side config: `dashboard.bind` from `config.yml`.

## Invocation

```
chopper2-serve --repo-root <path>
```
