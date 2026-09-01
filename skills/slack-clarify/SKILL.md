---
name: slack-clarify
description: Post exactly one batched clarification reply per bad_slug slug per cycle.
when_to_use: Each cycle for any GHLSTATE=bad_slug in agents/chopper2/to-chopper/.
---

# slack-clarify

Idempotent per slug per cycle: tracks `last_clarify_ts` on the slug. Posts ALL
missing fields in a single message in the original Slack thread. Past
`limits.slack_max_replies_per_slug`, silently skips.

## Invocation

```
slack-clarify --slug-path <path> --repo-root <path> [--dry-run]
```
