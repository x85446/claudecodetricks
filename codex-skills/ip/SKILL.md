---
name: ip
description: Alias for $iterate-planner. Typing $ip <anything> behaves exactly as $iterate-planner <anything> — plan building, adds, teamify/flatify, close/roll, list, status. Exists purely as a shorthand for rapid-fire planning streaks.
---

# $ip — alias for $iterate-planner

**Version:** iterate family 5.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not plan here.

Invoke `$iterate-planner` explicitly, passing `$ARGUMENTS` through **verbatim** — no
interpretation, no preprocessing, no summarizing. Everything (routing, rapid-fire terse mode, provenance, versions) is
defined there, and the alias must never produce behavior different from the
real skill.

If `$iterate-planner` does not resolve, it has not been installed under Codex's
skill-discovery path yet. Report that rather than improvising the behavior
from memory; the alias is worthless if it silently diverges from its target.

## Dependencies

- `$iterate-planner` — must also be installed as a Codex skill for this alias to
  resolve. Port it with `$skill-2-codex` if it is missing.
