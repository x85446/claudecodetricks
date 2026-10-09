---
name: "ii"
description: "Alias for $mailbox send --kind inform. Typing $ii <project> <what to tell them> drops a one-time bug report in another project's inbox. Exists purely as a shorthand."
---


# $ii — alias for $mailbox send --kind inform

**Version:** iterate family 5.13.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not write the report here.

## Usage

Argument: <project> <what to tell them>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$mailbox` — ported.

Invoke `$mailbox` explicitly, passing `send --kind inform $ARGUMENTS` **verbatim**: no interpretation, no preprocessing, no summarizing. `$mailbox` handles everything: project resolution, the report, the inbox file, and the one-line ack.

If explicit `$name` invocation cannot invoke `mailbox` (blocked or missing), read `~/.agents/skills/mailbox/SKILL.md` and follow its `send` verb directly with `--kind inform $ARGUMENTS`. The alias must never behave differently from the real skill.
