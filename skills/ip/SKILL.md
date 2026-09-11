---
name: ip
description: Alias for /iterate-planner. Typing /ip <anything> behaves exactly as /iterate-planner <anything> — plan building, adds, teamify/flatify, close/roll, list, status. Exists purely as a shorthand for rapid-fire planning streaks.
argument-hint: <same arguments as /iterate-planner>
disable-model-invocation: true
version: 5.1.0
---

# /ip — alias for /iterate-planner

This skill is a pure alias. Do not plan here.

Invoke the Skill tool with skill `iterate-planner`, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (routing, rapid-fire terse mode, provenance, versions) is handled by `/iterate-planner` itself.

If the Skill tool cannot invoke `iterate-planner` (e.g. it's blocked or missing), read `~/.claude/skills/iterate-planner/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
