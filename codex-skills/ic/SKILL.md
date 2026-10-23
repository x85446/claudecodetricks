---
name: "ic"
description: "Alias for $iterate-conductor; controls the binary conductor with the same request and scope."
---


# $ic — conductor alias

**Version:** iterate family 5.26.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Read the sibling `iterate-conductor/SKILL.md` from the installed skill collection and follow it with the user’s complete arguments. This explicit alias carries the same authorization and project scope. It owns no state, timers, policy or scheduler. The conductor skill delegates execution and configuration to `iterate-run conductor`.

## Usage

Argument: <same arguments as /iterate-conductor>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate-conductor` — ported.
