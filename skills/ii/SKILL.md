---
name: ii
description: Alias for /mailbox send --kind inform. Typing /ii <project> <what to tell them> drops a one-time bug report in another project's inbox. Exists purely as a shorthand.
argument-hint: <project> <what to tell them>
disable-model-invocation: true
version: 5.13.0
---

# /ii — alias for /mailbox send --kind inform

This skill is a pure alias. Do not write the report here.

Invoke the Skill tool with skill `mailbox`, passing `send --kind inform $ARGUMENTS` **verbatim**: no interpretation, no preprocessing, no summarizing. `/mailbox` handles everything: project resolution, the report, the inbox file, and the one-line ack.

If the Skill tool cannot invoke `mailbox` (blocked or missing), read `~/.claude/skills/mailbox/SKILL.md` and follow its `send` verb directly with `--kind inform $ARGUMENTS`. The alias must never behave differently from the real skill.
