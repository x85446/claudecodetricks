---
name: ibs
description: Alias for $iterate-brainstorm. Typing $ibs <anything> behaves exactly as $iterate-brainstorm <anything> — investigate, present 3 label-locked options with one recommended, discuss, then emit a numbered summary for $ip. Exists purely as a shorthand.
---

# $ibs — alias for $iterate-brainstorm

**Version:** iterate family 5.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not run the decision procedure from these instructions — run it from `$iterate-brainstorm`'s.

Invoke `$iterate-brainstorm` explicitly, passing `$ARGUMENTS` through **verbatim** — no
interpretation, no preprocessing, no summarizing. Everything (the silent investigation, the comparison table, label locking, the ★ Recommended marker, summary numbering) is
defined there, and the alias must never produce behavior different from the
real skill.

If `$iterate-brainstorm` does not resolve, it has not been installed under Codex's
skill-discovery path yet. Report that rather than improvising the behavior
from memory; the alias is worthless if it silently diverges from its target.

`$ibs` is user-invoked in both harnesses: `$iterate-brainstorm` sets
`policy.allow_implicit_invocation: false` in its `agents/openai.yaml` because
a decision session is something the user opens deliberately, never something natural language trips into. That setting blocks *implicit* selection only — explicit `$iterate-brainstorm`
invocation is exactly what it leaves open, which is why the delegation above
is the normal path and not a workaround.

## Dependencies

- `$iterate-brainstorm` — must also be installed as a Codex skill for this alias to
  resolve. Port it with `$skill-2-codex` if it is missing.
