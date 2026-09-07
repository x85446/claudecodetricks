---
name: show
description: Pretty-print a single bug or slug.
when_to_use: When the operator wants a quick human-readable view of a bug. Wraps `bug-render`. (AC114)
audience: operator
---

# show

Read-only. Calls `bug-render BUG-NNNNNN` (or `bug-render <slug>`); prints title, slug_title (if different), GHLSTATE, and last 5 events.
