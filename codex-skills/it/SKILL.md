---
name: "it"
description: "Alias for $iterate-triage. Typing $it behaves exactly as $iterate-triage — walk up to a stale terminal and get one short answer naming only what is broken and the one act that clears each. Exists purely as a shorthand."
---


# $it — alias for $iterate-triage

**Version:** iterate family 5.6.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not triage or discuss here.

## Usage

Argument: (none — reads the project state). `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate-triage` — ported.

Invoke `$iterate-triage` explicitly, passing `$ARGUMENTS` through **verbatim** — no interpretation, no preprocessing, no summarizing. Everything (state reading, the broken-only report shape, the fix scripts, the `launch problem N` flow, commits of loose work) is handled by `$iterate-triage` itself.

If explicit `$name` invocation cannot invoke `iterate-triage` (e.g. it's blocked or missing), read `~/.agents/skills/iterate-triage/SKILL.md` and follow it directly with `$ARGUMENTS` as its input — the alias must never produce behavior different from the real skill.
