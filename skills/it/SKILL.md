---
name: it
description: Alias for /iterate-triage. Typing /it behaves exactly as /iterate-triage — walk up to a stale terminal and get one short answer naming only what is broken and the one act that clears each. Exists purely as a shorthand.
argument-hint: (none — reads the project state)
disable-model-invocation: true
version: 5.5.1
---

# /it — alias for /iterate-triage

This skill is a pure alias. Do not triage or discuss here.

Invoke the Skill tool with skill `iterate-triage`, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (state reading, the broken-only report shape, the fix scripts, the `launch problem N` flow, commits of loose work) is handled by `/iterate-triage` itself.

If the Skill tool cannot invoke `iterate-triage` (e.g. it's blocked or missing), read `~/.claude/skills/iterate-triage/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
