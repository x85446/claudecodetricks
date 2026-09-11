---
name: in
description: Alias for $iterate-notes. Typing $in <anything> behaves exactly as $iterate-notes <anything> — note capture for the next iterate plan. Exists purely as a shorthand for rapid note-taking.
---

# $in — alias for $iterate-notes

**Version:** iterate family 5.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


This skill is a pure alias. Do not capture or discuss here.

Invoke `$iterate-notes` explicitly, passing `$ARGUMENTS` through **verbatim** — no
interpretation, no preprocessing, no summarizing. Everything (note capture, notes files, decisions, handoff) is
defined there, and the alias must never produce behavior different from the
real skill.

If `$iterate-notes` does not resolve, it has not been installed under Codex's
skill-discovery path yet. Report that rather than improvising the behavior
from memory; the alias is worthless if it silently diverges from its target.

## Dependencies

- `$iterate-notes` — must also be installed as a Codex skill for this alias to
  resolve. Port it with `$skill-2-codex` if it is missing.
