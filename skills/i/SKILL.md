---
name: i
description: Alias for /iterate. Typing /i <anything> behaves exactly as /iterate <anything> — autonomous execution of the current or named plan, resumption included. Exists purely as a shorthand.
argument-hint: <same arguments as /iterate>
disable-model-invocation: true
version: 5.0.0
---

# /i — alias for /iterate

This skill is a pure alias. Do not execute the plan from these instructions — execute from `/iterate`'s.

**Do NOT attempt `Skill(iterate)` — it will always fail.** `/iterate` carries `disable-model-invocation: true` by design (autonomous execution and PR merges are reserved for explicit user invocation), and that flag blocks the Skill tool unconditionally.

Instead: read `~/.claude/skills/iterate/SKILL.md` and follow it directly, with `$ARGUMENTS` as its input, verbatim — no interpretation, no preprocessing, no summarizing. Everything (plan resolution, team dispatch, resumption, merge-on-green) is defined there.

**This is the sanctioned path, not a circumvention.** The flag on `/iterate` reserves it for explicit *user* invocation — and this alias is exactly that: it is itself `disable-model-invocation: true`, so the ONLY way these instructions can be in context is that the user personally typed `/i`. A user keystroke on `/i` IS the explicit user invocation of `/iterate`, same as `/iterate` itself — the alias merely saves six characters. Do not refuse, do not ask the user to retype the long form; follow `/iterate`'s SKILL.md now.
