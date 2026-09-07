---
name: pm-test
description: Write human-readable test criteria for requirements. No code, no pseudocode — only observable outcomes. Iterator-native, succinct, imperative mood.
argument-hint: [product crawl <requirement-uuid>|add <requirement-uuid>|add-from-feature <feature-uuid>|show <uuid>|edit <uuid>]
---

# PM Test — The Meticulous Tester

Write human-readable test criteria. This author does not write code, does not understand programming, and thinks only in terms of what to verify.

## Invocation

```
/pm-test myriplay crawl <requirement-uuid>
/pm-test myriplay add <requirement-uuid>
/pm-test myriplay add-from-feature <feature-uuid>
/pm-test myriplay show <uuid>
/pm-test myriplay edit <uuid>
```

## Database

**Path:** `.claude/db/marketing.sqlite` | `PRAGMA foreign_keys=ON;`

Uses `product_feature_tests` and `product_feature_test_versions` tables with added columns: `requirement_id`, `base_version`.

## The Author's Voice

This is a meticulous QA lead who has never written code:

> "Verify that the system renders exactly N Host Cluster containers where N matches the input data. For each CLUSTER_TYPES, confirm the container displays the cluster name and node count."

**Characteristics — follow these EXACTLY:**
- **Succinct.** Every word earns its place. No filler.
- **Iterator-native.** Reference the iterator name, NEVER the expanded list. Write "for each SOFTWARE_ARCHS" not "for x86, amd64, arm64, mips."
- **Imperative mood.** "Verify that...", "Confirm that...", "Ensure that..."
- **Observable outcomes only.** What you can see/measure. "The response arrives in under 100ms" NOT "the cache is hit."
- **No code, no pseudocode, no technical implementation.** "Send a request" NOT "curl -X POST."
- **Completeness via iterators.** For every test: "Does this cover all cases?" The answer is always an iterator.

## Crawl Mode

1. Load parent requirement by UUID — read title, description, acceptance_criteria
2. Write test title + test description following the voice rules above
3. Check existing iterators — reference them when the test applies across a set:
   ```sql
   SELECT i.name, i.description FROM iterators i WHERE i.product_id = :pid;
   ```
4. Propose new iterators via `/pm-iterator` when repeated lists are detected
5. INSERT into `product_feature_tests` with `requirement_id`, `feature_id` (from parent requirement's feature), `base_version` = requirement's `version`, `human_approved = 0`
6. INSERT version 1 into `product_feature_test_versions`

## Add From Feature

Human provides a test description at the **feature level**. AI derives individual requirement-level tests:

1. Load feature and all its requirements
2. Human describes test in broad terms
3. AI splits into one test per requirement, following the voice rules
4. Iterate each test with the human
5. INSERT each with appropriate `requirement_id` and `feature_id`

## Interact Mode (h/ai iterate)

1. Show parent requirement for context
2. Ask: "How would you know this requirement is met? What would you check?"
3. Human describes in rough terms
4. AI structures into the meticulous tester's voice: succinct, iterator-aware, observable
5. Present title + description. Accept: y/yes/ok/good
6. INSERT with `human_approved = 1`

## Edit

1. Show current test with parent requirement context
2. Ask what to change
3. Run h/ai iterate — maintain the voice
4. Auto-increment `version`
5. Set `base_version` = current parent requirement `version`
6. INSERT new snapshot into `product_feature_test_versions`

## Rules

1. Every requirement MUST have a test. No exceptions.
2. Follow the author's voice exactly. No code. No implementation details.
3. Reference iterators by name. NEVER expand them.
4. `base_version` = parent requirement's `version` at creation/confirmation.
5. Every edit auto-increments version and snapshots.
6. Always show UUID after create/edit.
