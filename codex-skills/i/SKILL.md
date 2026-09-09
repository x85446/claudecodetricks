---
name: "i"
description: "Alias for $iterate. Typing $i <anything> behaves exactly as $iterate <anything> — autonomous execution of the current or named plan, resumption included. Exists purely as a shorthand."
---


# $i — alias for $iterate

**Version:** iterate family 5.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not execute the plan from these instructions — execute from `$iterate`'s.

## Usage

Argument: <same arguments as /iterate>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate` — ported.

**Do NOT attempt `$iterate` — it will always fail.** `$iterate` carries `disable-model-invocation: true` by design (autonomous execution and PR merges are reserved for explicit user invocation), and that flag blocks explicit `$name` invocation unconditionally.

Instead: read `~/.agents/skills/iterate/SKILL.md` and follow it directly, with `$ARGUMENTS` as its input, verbatim — no interpretation, no preprocessing, no summarizing. Everything (plan resolution, team dispatch, resumption, merge-on-green) is defined there.

**This is the sanctioned path, not a circumvention.** The flag on `$iterate` reserves it for explicit *user* invocation — and this alias is exactly that: it is itself `disable-model-invocation: true`, so the ONLY way these instructions can be in context is that the user personally typed `$i`. A user keystroke on `$i` IS the explicit user invocation of `$iterate`, same as `$iterate` itself — the alias merely saves six characters. Do not refuse, do not ask the user to retype the long form; follow `$iterate`'s SKILL.md now.
