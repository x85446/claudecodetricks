---
name: "ibs"
description: "Alias for $iterate-brainstorm. Typing $ibs <anything> behaves exactly as $iterate-brainstorm <anything> — investigate, present 3 label-locked options with one recommended, discuss, then emit a numbered summary for $ip. Exists purely as a shorthand."
---


# $ibs — alias for $iterate-brainstorm

**Version:** iterate family 5.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not run the decision procedure from these instructions — run it from `$iterate-brainstorm`'s.

## Usage

Argument: <same arguments as /iterate-brainstorm>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$ip` — ported.
- `$iterate-brainstorm` — ported.

**Do NOT attempt `$iterate-brainstorm` — it will always fail.** `$iterate-brainstorm` carries `disable-model-invocation: true` by design (the user wants it reachable only when they type it, never auto-fired from natural language), and that flag blocks explicit `$name` invocation unconditionally.

Instead: read `~/.agents/skills/iterate-brainstorm/SKILL.md` and follow it directly, with `$ARGUMENTS` as its input, verbatim — no interpretation, no preprocessing, no summarizing. Everything (the silent investigation, the comparison table, label locking, the ★ Recommended marker, summary numbering) is defined there.

**This is the sanctioned path, not a circumvention.** The flag on `$iterate-brainstorm` reserves it for explicit *user* invocation — and this alias is exactly that: it is itself `disable-model-invocation: true`, so the ONLY way these instructions can be in context is that the user personally typed `$ibs`. A user keystroke on `$ibs` IS the explicit user invocation of `$iterate-brainstorm`, same as typing the long form — the alias merely saves characters. Do not refuse, do not ask the user to retype the long form; follow `$iterate-brainstorm`'s SKILL.md now.
