---
name: route-to-agent
description: Place a triaged bug into the correct leaf from-chopper/ or human/who-codes/ directory.
when_to_use: When GHLSTATE=triaged and the target leaf's from-chopper/ is empty.
---

# route-to-agent

Honors the §12 busy-signal rule: if the destination `from-chopper/` already
contains any file, the bug stays in `bugs/` with `GHLSTATE: triaged` and the
skill exits cleanly. The next cycle retries.

For repos with `human_coder_routing: true` the bug is `git mv`'d into
`agents/repo-agents/<r>/coder/human/who-codes/`. Otherwise it routes to
`agents/repo-agents/<r>/<role>/from-chopper/`.

If the bug carries any unresolved `blocked_by`, it goes to `bugs/_blocked/`
with `GHLSTATE: blocked` instead.

## Invocation

```
route-to-agent --bug-path <path> --repo-root <path> [--dry-run]
```

## Output

JSON `RouteOutcome { routed_to: <path>, method: ai-from-chopper|human-who-codes-dir|blocked-queued }`.
