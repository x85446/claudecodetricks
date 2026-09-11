---
name: in
description: Alias for /iterate-notes. Typing /in <anything> behaves exactly as /iterate-notes <anything> — note capture for the next iterate plan. Exists purely as a shorthand for rapid note-taking.
argument-hint: <same arguments as /iterate-notes>
disable-model-invocation: true
version: 5.1.0
---

# /in — alias for /iterate-notes

This skill is a pure alias. Do not capture or discuss here.

Invoke the Skill tool with skill `iterate-notes`, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (note capture, notes files, decisions, handoff) is handled by `/iterate-notes` itself.

If the Skill tool cannot invoke `iterate-notes` (e.g. it's blocked or missing), read `~/.claude/skills/iterate-notes/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
