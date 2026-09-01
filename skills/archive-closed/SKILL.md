---
name: archive-closed
description: git-rm bugs older than ttl.prune_closed_days from bugs/closed/.
when_to_use: Each chopper2 cycle, step 8.
---

# archive-closed

Walks `bugs/closed/`. For each bug, reads the most recent `closed_at`-bearing
event from `events[]`. If `now - closed_at > ttl.prune_closed_days`, runs
`git rm <path>`. History remains in `git log`. Idempotent — empty walk on a
fresh tree, exits 0.

## Invocation

```
archive-closed --repo-root <path> [--dry-run]
```
