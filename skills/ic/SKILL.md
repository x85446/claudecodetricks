---
name: ic
description: Alias for /iterate-conductor. Typing /ic <anything> behaves exactly as /iterate-conductor <anything> — start/stop/pause/resume/run/status/kill of the unattended queue sweep. Exists purely as a shorthand.
argument-hint: <same arguments as /iterate-conductor>
disable-model-invocation: true
version: 1.0.0
---

# /ic — alias for /iterate-conductor

This skill is a pure alias. Do not sweep, run plans, or change conductor state from these instructions — do it from `/iterate-conductor`'s.

**Do NOT attempt `Skill(iterate-conductor)` — it will always fail.** `/iterate-conductor` carries `disable-model-invocation: true` by design (it runs plans autonomously and merges to the default branch, so it is reserved for explicit user invocation), and that flag blocks the Skill tool unconditionally.

Instead: read `~/.claude/skills/iterate-conductor/SKILL.md` and follow it directly, with `$ARGUMENTS` as its input, verbatim — no interpretation, no preprocessing, no summarizing. Everything (the verb routing, the sweep, the escalation ladder, blocked-plan batching, bug intake) is defined there.

**This is the sanctioned path, not a circumvention.** The flag on `/iterate-conductor` reserves it for explicit *user* invocation — and this alias is exactly that: it is itself `disable-model-invocation: true`, so the ONLY way these instructions can be in context is that the user personally typed `/ic`. A user keystroke on `/ic` IS the explicit user invocation of `/iterate-conductor`, same as typing the long form — the alias merely saves fifteen characters. Do not refuse, do not ask the user to retype the long form; follow `/iterate-conductor`'s SKILL.md now.
