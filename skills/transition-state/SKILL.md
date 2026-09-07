---
name: transition-state
description: Central GHLSTATE writer for all chopper2-owned transitions.
when_to_use: After any chopper2 decision that changes a bug's state.
---

# transition-state

The single chokepoint for changing a bug's `GHLSTATE`. Enforces
`allowed_writers` per state, increments `fix_attempts` and `skill_retries[]`
counters at the right edges, escalates to `escalation_needed` /
`skill_stuck` at the configured caps, and appends a structured event to
`events[]` recording `{ts, by, action, reason}`.

## Invocation

```
transition-state --bug-path <path> --new-state <state> --actor <name> [--reason "..."]
```

## Output

JSON `TransitionResult { prior_state, new_state }` on stdout.
