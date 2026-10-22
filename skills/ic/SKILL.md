---
name: ic
description: Alias for /iterate-conductor; controls the binary conductor with the same request and scope.
argument-hint: <same arguments as /iterate-conductor>
disable-model-invocation: true
version: 5.23.0
---

# /ic — conductor alias

Read the sibling `iterate-conductor/SKILL.md` from the installed skill collection and follow it with the user’s complete arguments. This explicit alias carries the same authorization and project scope. It owns no state, timers, policy or scheduler. The conductor skill delegates execution and configuration to `iterate-run conductor`.
