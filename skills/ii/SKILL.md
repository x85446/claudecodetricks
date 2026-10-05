---
name: ii
description: Alias for /iterate-inform. Typing /ii <project> <what to tell them> behaves exactly as /iterate-inform — a one-time bug report dropped in another project's iterate inbox. Exists purely as a shorthand.
argument-hint: <project> <what to tell them>
disable-model-invocation: true
version: 5.12.0
---

# /ii — alias for /iterate-inform

This skill is a pure alias. Do not write the report here.

Invoke the Skill tool with skill `iterate-inform`, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (project resolution, the report, the inbox file, the one-line ack) is handled by `/iterate-inform` itself.

If the Skill tool cannot invoke `iterate-inform` (e.g. it's blocked or missing), read `~/.claude/skills/iterate-inform/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
