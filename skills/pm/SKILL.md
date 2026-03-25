---
name: pm
description: Product Manager orchestrator. Parses natural language intent, dispatches to sub-skills (epic, feature, requirement, test, iterator, auditor), and tracks coverage. Use for any product management workflow.
argument-hint: [product natural-language-request]
---

# PM — Product Manager

Orchestrator skill. Parses intent, dispatches to focused sub-skills, tracks coverage. Never writes content or DB records directly.

## Invocation

Natural language after the product code:

```
/pm myriplay scan this codebase and write the epics and features
/pm myriplay crawl ./src/ ./docs/api.md https://docs.example.com
/pm myriplay interview me about a new feature
/pm myriplay I need a new feature. When X happens we want Y
/pm myriplay voice new feature
/pm myriplay status
/pm myriplay audit
```

## Preflight

Before ANY work, invoke `/pm-preflight`. If it fails, STOP.

## Mode Detection

Parse the user's natural language to determine mode:

| Keywords | Mode | Action |
|----------|------|--------|
| scan, crawl, read, discover, learn | Crawl | AI reads sources, populates DB |
| interview, add, new, I need, I want | Interact | Interview human via sub-skills |
| voice | Voice | Activate VoiceMode, then Interact |
| status, dashboard, coverage | Status | Invoke `/pm-status` |
| audit, stale, check | Audit | Invoke `/pm-auditor` |
| publish | Publish | Invoke `/pm-publish` |

## Crawl Pipeline

Dispatch in order. Each step feeds the next:

1. `/pm-epic` — identify epics from source material
2. `/pm-feature` — break each epic into features
3. `/pm-requirement` — derive requirements from each feature
4. `/pm-test` — write test criteria for each requirement
5. `/pm-iterator` — detect and create iterators for repeated value lists
6. `/pm-auditor` — verify dependency chain integrity

All results: `human_approved = 0`. Report summary when done.

## Interact Pipeline

Determine what the human wants to add/edit and dispatch to the right skill:

- Epic-level talk → `/pm-epic`
- Feature-level talk → `/pm-feature` (ensure epic exists first)
- Requirement-level talk → `/pm-requirement` (ensure feature exists first)
- Test descriptions → `/pm-test`
- Value lists → `/pm-iterator`

If the human describes something at the wrong level (e.g., gives requirement-level detail when no feature exists), work upward: create the epic and feature first, then the requirement.

## Coverage Check

After any pipeline completes, query and report:

```sql
-- Epics without features
SELECT e.id, e.name FROM epics e
LEFT JOIN product_features pf ON pf.epic_id = e.id
WHERE e.product_id = :pid AND pf.id IS NULL;

-- Features without requirements
SELECT pf.id, pf.short_desc FROM product_features pf
LEFT JOIN requirements r ON r.feature_id = pf.id
WHERE pf.product_id = :pid AND r.id IS NULL;

-- Requirements without tests
SELECT r.id, r.title FROM requirements r
JOIN product_features pf ON pf.id = r.feature_id
LEFT JOIN product_feature_tests t ON t.requirement_id = r.id
WHERE pf.product_id = :pid AND t.id IS NULL;
```

Surface gaps to the user after every run.

## Rules

1. **Never write to DB directly.** Delegate to sub-skills.
2. **Never set `human_approved`.** Only ViteTool or explicit human action does that.
3. **Always run preflight first.**
4. **Always report coverage gaps** after completing work.
5. **Work top-down.** Epics before features before requirements before tests.
6. **Every feature must belong to an epic.** If none exists, create one first.
