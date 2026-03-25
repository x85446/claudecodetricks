---
name: pm
description: Product Manager orchestrator. Use when someone asks to scan code for features, crawl docs for epics, interview about a new feature, add requirements, check product coverage, audit staleness, or publish a product spec.
argument-hint: [product natural-language-request]
---

# PM — Product Manager

Orchestrator skill. Parses intent, dispatches to focused sub-skills, tracks coverage. Never writes content or DB records directly.

## Files Referenced

- **Database:** `.claude/db/marketing.sqlite`
- **Schema:** Managed by `/pm-preflight` — see `skills/pm-preflight/schema.sql`
- **Queries:** Reference queries in `skills/pm-preflight/queries.sql`

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

## Steps

1. **Run preflight.** Invoke `/pm-preflight`. If it fails, STOP.
2. **Parse intent.** Extract the product code (first argument) and detect mode from remaining text:

| Keywords | Mode |
|----------|------|
| scan, crawl, read, discover, learn | Crawl |
| interview, add, new, I need, I want | Interact |
| voice | Voice (activate VoiceMode, then Interact) |
| status, dashboard, coverage | Status → invoke `/pm-status` and stop |
| audit, stale, check | Audit → invoke `/pm-auditor` and stop |
| publish | Publish → invoke `/pm-publish` and stop |

3. **Dispatch pipeline.** Based on mode, run the appropriate pipeline below.
4. **Report results.** Show the summary and coverage gaps (see Output Format).

## Crawl Pipeline

Dispatch in order. Each step feeds the next:

1. Invoke `/pm-epic <product> crawl <sources>` — identify epics from source material
2. For each epic created, invoke `/pm-feature <product> crawl <epic-uuid>`
3. For each feature created, invoke `/pm-requirement <product> crawl <feature-uuid>`
4. For each requirement created, invoke `/pm-test <product> crawl <requirement-uuid>`
5. Invoke `/pm-iterator` if repeated value lists were detected during steps 1-4
6. Invoke `/pm-auditor <product>` — verify dependency chain integrity

All results land with `human_approved = 0`.

## Interact Pipeline

1. Determine what the human wants to add/edit from their natural language
2. Dispatch to the right skill:
   - Epic-level talk → `/pm-epic <product> add`
   - Feature-level talk → `/pm-feature <product> add <epic-uuid>` (ensure epic exists first)
   - Requirement-level talk → `/pm-requirement <product> add <feature-uuid>` (ensure feature exists first)
   - Test descriptions → `/pm-test <product> add <requirement-uuid>`
   - Value lists → `/pm-iterator <product> create ...`
3. If the human describes something at the wrong level (e.g., requirement-level detail when no feature exists), work upward: create the epic first, then the feature, then the requirement.

## Output Format

After every pipeline completes, report:

```
PM Summary: Myriplay
--------------------------------------------
Created:  3 epics, 12 features, 28 requirements, 28 tests
Mode:     crawl (./src/)

Coverage Gaps:
  [!] Feature "WebSocket streaming" has 0 requirements
  [!] Requirement "Compact layout threshold" has no test
  [!] 2 features stale (parent epic updated)

Next Steps:
  - Review unapproved items in ViteTool
  - Run /pm myriplay audit for full staleness report
```

If no gaps exist, say: `Coverage: 100% — all epics have features, all features have requirements, all requirements have tests.`

## Coverage Queries

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

## Edge Cases

- **Ambiguous input:** If you can't determine the mode, ask the user: "Are you looking to scan existing code/docs, or describe something new?"
- **Sub-skill failure:** If a sub-skill fails (e.g., sqlite3 error mid-pipeline), report what succeeded and what failed. Do not retry automatically — surface the error.
- **Partial data:** If the human provides a feature without mentioning an epic, ask which epic it belongs to or offer to create one. Never create orphaned features.
- **Empty crawl results:** If scanning produces zero epics, tell the user and suggest a different source or switching to interact mode.

## Rules

1. **Never write to DB directly.** Delegate to sub-skills.
2. **Never set `human_approved`.** Only ViteTool or explicit human action does that.
3. **Always run preflight first.**
4. **Always report coverage gaps** after completing work.
5. **Work top-down.** Epics before features before requirements before tests.
6. **Every feature must belong to an epic.** If none exists, create one first.
