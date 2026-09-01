---
name: slack-intake
description: Pull new messages from allowlisted Slack channels and emit slug drafts.
when_to_use: Each chopper2 cycle, step 6.
---

# slack-intake

Polls every channel in `config.yml::slack.channels.allowlist`. For each new
non-chatter message:
- if the message has the `ignore_reaction`, skip it;
- otherwise extract description / validation / surface via Haiku;
- write a slug draft to `agents/chopper2/to-chopper/SLUG-<sha>.json` with
  `source: slack`, `slack_thread_ts`, `slack_permalink`.

Re-intake is incremental: each slug stores `last_extracted_thread_ts`; only
replies after that ts get re-extracted next cycle.

## Invocation

```
slack-intake --repo-root <path> --token-path <path>
```
