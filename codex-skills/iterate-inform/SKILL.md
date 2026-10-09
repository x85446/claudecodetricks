---
name: "iterate-inform"
description: "Alias for $mailbox send --kind inform, kept so $iterate-inform still works: sends another project a one-time bug report about a problem it owns."
---

<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# $iterate-inform — alias for $mailbox send --kind inform

**Version:** iterate family 5.13.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


`$mailbox` owns every inbox: sending, reading, replying and processing. This
skill exists so `$iterate-inform` and `$ii` keep working, and it adds nothing.

## Usage

Argument: <project> <what to tell them — an instruction to the AI>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$ii` — ported.
- `$mailbox` — ported.

Invoke `$mailbox` explicitly, passing `send --kind inform $ARGUMENTS`
**verbatim**. If explicit `$name` invocation cannot invoke it, read
`~/.agents/skills/mailbox/SKILL.md` and follow its `send` verb directly.

The report's shape (Problem, Evidence, Needed, Done when, Blocks), its
writing rules, and the one-line ack are all defined in `$mailbox`. Never
write a report here.

## `version`

`version` (or "what version") on **any** iterate skill reports the same thing:
the family version, because the stack is versioned as one unit:

```
iterate family 5.0.0
iterate-run iterate-v3.3 (commit 4dd09ec5, built 2026-08-27_17:02:20)
```

**Both lines come from a real read, never from memory, the family line
included.** Run these two, from any directory:

```bash
grep -m1 '^version:' ~/.agents/skills/iterate/SKILL.md   # the family version
iterate-run version                                      # the binary
```

**Never quote the `version:` in the skill body you already have in context.**
A session loads a skill body once and keeps it, so after a `skillctl family
iterate set` and reinstall, the copy in context is stale.

If members disagree, say so and name them: drift inside the family is a
defect, not a state, and `skillctl family iterate set X.Y.Z` is the only
correct way to bump.
