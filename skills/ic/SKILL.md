---
name: ic
description: Alias for /iterate-conductor; controls the binary conductor with the same request and scope.
argument-hint: <same arguments as /iterate-conductor>
disable-model-invocation: true
version: 5.34.0
---

# /ic — conductor alias

Read the sibling `iterate-conductor/SKILL.md` from the installed skill collection and follow it with the user’s complete arguments. This explicit alias carries the same authorization and project scope. It owns no state, timers, policy or scheduler. The conductor skill delegates execution and configuration to `conductor`.

A session claims with `iterate-run plan claim <name>`, which binds the plan to it; the Conductor claims with `--owner conductor --pid <its pid>`. Only a live execution excludes the other executor, and `iterate-run plan owner` says whether a claim is driven. Dependencies gate typed and unnamed launches. A bare iterate selects the sole ready plan from `iterate-run plan graph --json`; several ready plans are listed without a claim. Use `plan heartbeat`, `plan release` and `plan reconcile` for lease lifecycle, preserving uncertain checkpoint evidence.
