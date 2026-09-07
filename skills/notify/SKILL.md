---
name: notify
description: Post a typed Slack notification, applying cooldown deduplication and failure persistence.
when_to_use: Any time a trunk skill needs to emit a Slack notification.
---

# notify

Single chokepoint for every Slack post. Resolves subscribers from
`config.yml::notifications` (`_global` + `per_type` + `per_repo`), applies
the rate-limit window, persists cooldown state to
`agents/chopper2/reports/notify-cooldowns.json`, and writes failures to
`agents/chopper2/reports/notify-failures-<date>.json` for retry next cycle.

## Notification types

`new_bug_in_who_codes`, `bug_stale_who_codes`, `bug_stale_in_progress`,
`p0_bug_filed`, `bug_verified`, `bug_closed`, `slug_dead_letter`,
`wrong_repo_reroute`, `auto_recovery_triggered`, `infra_failure_persistent`,
`manifest_drift`, `merge_conflict`, `external_bug_filed`, `escalation_needed`,
`skill_error`, `skill_stuck`, `crash_recovered`, `non_allowlist_mention`,
`slack_token_age_critical`.

## Invocation

```
notify --type <notif_type> --context <json> [--repo <r>] --repo-root <path>
```
