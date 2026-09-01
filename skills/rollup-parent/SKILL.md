---
name: rollup-parent
description: Close a multi-repo parent bug when all its children are verified/closed.
when_to_use: After any child transitions to verified or closed.
---

# rollup-parent

Reads the parent's `children[]`. If every child is `verified` or `closed`,
calls `transition-state(parent, closed)` and `git mv` to `bugs/closed/`.
When children count == `limits.bug_max_children_per_parent`, emits a
`health-report` warning so operator sees the cap.

## Invocation

```
rollup-parent --parent-id BUG-NNNNNN --repo-root <path>
```
