---
name: "in"
description: "Alias for $iterate-notes. Typing $in <anything> behaves exactly as $iterate-notes <anything> — note capture for the next iterate plan. Exists purely as a shorthand for rapid note-taking."
---


# $in — alias for $iterate-notes

**Version:** iterate family 5.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not capture or discuss here.

## Usage

Argument: <same arguments as /iterate-notes>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate-notes` — ported.

Invoke `$iterate-notes` explicitly, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (note capture, notes files, decisions, handoff) is handled by `$iterate-notes` itself.

If explicit `$name` invocation cannot invoke `iterate-notes` (e.g. it's blocked or missing), read `~/.agents/skills/iterate-notes/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
