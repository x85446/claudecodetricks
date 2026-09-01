---
name: health-report
description: Aggregate host health and stale-bug state; write dated report; emit Slack on threshold breach.
when_to_use: Each chopper2 cycle step 9; always; full persist on the hour.
---

# health-report

Aggregates: stale `in_progress` bugs (`> stale.in_progress_warn_hours`),
host silence (`infra/health/<host>.json` age `> 2× cadence`), quarantine
count, Slack token age, backup freshness, `dr.backup_s3 == false` warning.

When `--persist` is set (top of each hour), writes a dated digest to
`agents/chopper2/reports/<YYYY-MM-DD>.json`. Emits Slack via `notify` for
every WARN/ERR item using `manifest_drift`, `slack_token_age_critical`,
`bug_stale_in_progress`, etc.

## Invocation

```
health-report --repo-root <path> [--persist]
```
