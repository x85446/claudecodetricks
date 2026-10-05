---
name: "ii"
description: "Alias for $iterate-inform. Typing $ii <project> <what to tell them> behaves exactly as $iterate-inform — a one-time bug report dropped in another project's iterate inbox. Exists purely as a shorthand."
---


# $ii — alias for $iterate-inform

**Version:** iterate family 5.12.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not write the report here.

## Usage

Argument: <project> <what to tell them>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate-inform` — ported.

Invoke `$iterate-inform` explicitly, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (project resolution, the report, the inbox file, the one-line ack) is handled by `$iterate-inform` itself.

If explicit `$name` invocation cannot invoke `iterate-inform` (e.g. it's blocked or missing), read `~/.agents/skills/iterate-inform/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
