---
name: list
description: List bugs filtered by state / repo / assignee.
when_to_use: When the operator wants to see what bugs match a given filter. Read-only.
audience: operator
---

# list

Read-only. Scans `bugs/**/*.json`. Optional `--state`, `--repo`, `--assignee` filters. Prints `{id, title, GHLSTATE, assignee, age}` rows, one per line.
