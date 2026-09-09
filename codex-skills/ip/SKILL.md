---
name: "ip"
description: "Alias for $iterate-planner. Typing $ip <anything> behaves exactly as $iterate-planner <anything> — plan building, adds, teamify/flatify, close/roll, list, status. Exists purely as a shorthand for rapid-fire planning streaks."
---


# $ip — alias for $iterate-planner

**Version:** iterate family 5.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not plan here.

## Usage

Argument: <same arguments as /iterate-planner>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate-planner` — ported.

Invoke `$iterate-planner` explicitly, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (routing, rapid-fire terse mode, provenance, versions) is handled by `$iterate-planner` itself.

If explicit `$name` invocation cannot invoke `iterate-planner` (e.g. it's blocked or missing), read `~/.agents/skills/iterate-planner/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
