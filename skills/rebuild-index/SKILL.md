---
name: rebuild-index
description: Rebuild bugs/_index/by-status/ and by-assignee/ from the on-disk bug tree.
when_to_use: When the index is suspected stale or corrupted. Idempotent; preserves state.json::next_id.
audience: operator
---

# rebuild-index

Walks `bugs/**/*.json`; regenerates `_index/by-status/<state>.json` and `_index/by-assignee/<agent>.json` from scan; preserves `state.json::next_id`. Commits + pushes if anything changed; no-op otherwise.
