---
name: iterate-inform
description: "Alias for /mailbox send --kind inform, kept so /iterate-inform still works: sends another project a one-time bug report about a problem it owns."
argument-hint: <project> <what to tell them — an instruction to the AI>
version: 5.13.0
---
<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# /iterate-inform — alias for /mailbox send --kind inform

`/mailbox` owns every inbox: sending, reading, replying and processing. This
skill exists so `/iterate-inform` and `/ii` keep working, and it adds nothing.

Invoke the Skill tool with skill `mailbox`, passing `send --kind inform $ARGUMENTS`
**verbatim**. If the Skill tool cannot invoke it, read
`~/.claude/skills/mailbox/SKILL.md` and follow its `send` verb directly.

The report's shape (Problem, Evidence, Needed, Done when, Blocks), its
writing rules, and the one-line ack are all defined in `/mailbox`. Never
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
grep -m1 '^version:' ~/.claude/skills/iterate/SKILL.md   # the family version
iterate-run version                                      # the binary
```

**Never quote the `version:` in the skill body you already have in context.**
A session loads a skill body once and keeps it, so after a `skillctl family
iterate set` and reinstall, the copy in context is stale.

If members disagree, say so and name them: drift inside the family is a
defect, not a state, and `skillctl family iterate set X.Y.Z` is the only
correct way to bump.
