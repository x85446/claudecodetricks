---
name: i
description: Alias for $iterate. Typing $i <anything> behaves exactly as $iterate <anything> — autonomous execution of the current or named plan, resumption included. Exists purely as a shorthand.
---

# $i — alias for $iterate

**Version:** iterate family 5.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not execute the plan from these instructions — execute from `$iterate`'s.

Invoke `$iterate` explicitly, passing `$ARGUMENTS` through **verbatim** — no
interpretation, no preprocessing, no summarizing. Everything (plan resolution, team dispatch, resumption, merge-on-green) is
defined there, and the alias must never produce behavior different from the
real skill.

If `$iterate` does not resolve, it has not been installed under Codex's
skill-discovery path yet. Report that rather than improvising the behavior
from memory; the alias is worthless if it silently diverges from its target.

`$i` is user-invoked in both harnesses: `$iterate` sets
`policy.allow_implicit_invocation: false` in its `agents/openai.yaml` because
autonomous execution and PR merges are reserved for deliberate, explicit invocation. That setting blocks *implicit* selection only — explicit `$iterate`
invocation is exactly what it leaves open, which is why the delegation above
is the normal path and not a workaround.

## Dependencies

- `$iterate` — must also be installed as a Codex skill for this alias to
  resolve. Port it with `$skill-2-codex` if it is missing.
