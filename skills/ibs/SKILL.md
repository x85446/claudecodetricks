---
name: ibs
description: Alias for /iterate-brainstorm. Typing /ibs <anything> behaves exactly as /iterate-brainstorm <anything> — investigate, present 3 label-locked options with one recommended, discuss, then emit a numbered summary for /ip. Exists purely as a shorthand.
argument-hint: <same arguments as /iterate-brainstorm>
disable-model-invocation: true
version: 5.1.0
---

# /ibs — alias for /iterate-brainstorm

This skill is a pure alias. Do not run the decision procedure from these instructions — run it from `/iterate-brainstorm`'s.

**Do NOT attempt `Skill(iterate-brainstorm)` — it will always fail.** `/iterate-brainstorm` carries `disable-model-invocation: true` by design (the user wants it reachable only when they type it, never auto-fired from natural language), and that flag blocks the Skill tool unconditionally.

Instead: read `~/.claude/skills/iterate-brainstorm/SKILL.md` and follow it directly, with `$ARGUMENTS` as its input, verbatim — no interpretation, no preprocessing, no summarizing. Everything (the silent investigation, the comparison table, label locking, the ★ Recommended marker, summary numbering) is defined there.

**This is the sanctioned path, not a circumvention.** The flag on `/iterate-brainstorm` reserves it for explicit *user* invocation — and this alias is exactly that: it is itself `disable-model-invocation: true`, so the ONLY way these instructions can be in context is that the user personally typed `/ibs`. A user keystroke on `/ibs` IS the explicit user invocation of `/iterate-brainstorm`, same as typing the long form — the alias merely saves characters. Do not refuse, do not ask the user to retype the long form; follow `/iterate-brainstorm`'s SKILL.md now.
