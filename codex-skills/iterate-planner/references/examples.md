# $iterate-planner — worked examples

Extracted from SKILL.md to keep the main file under the line budget. Load this only when a fuller worked example is actually needed (e.g. building a new plan-writing skill from this one as a template, or debugging an edge case in the operation router / oracle merge / teamify output shape).

## Example

User in conversation about adding a new "metrics" service to mgmt.gravhl.com. Oracle has:

```
## Post-action checklists
- When adding a new service to mgmt.gravhl.com: update the GUI link tree.

## Testing requirements
- All UI changes require interactive browser testing.

## Deployment rituals
- Database migrations run BEFORE the new app version starts.
```

User types:

```
$iterate-planner restate the plan
```

You print:

```
plan written to owl

**Plan ready** — owl — ./.claude/iterate/plans/owl.md (phase: planned, oracle-aware)

**Goal:** Deploy a new "metrics" service behind mgmt.gravhl.com with link-tree integration.

1a. Run the metrics-service schema migration on the prod DB.
1b. `psql -c "\dt metrics.*"` shows the new tables.

2a. Build and deploy the metrics-service container.
2b. `curl https://metrics.gravhl.com/health` returns 200 AND the service appears in the container orchestrator's running list.

3a. Update the GUI link tree at mgmt.gravhl.com to include "metrics" → https://metrics.gravhl.com.
3b. Load mgmt.gravhl.com in a browser, click the metrics link, confirm it routes to the live metrics service AND the page renders without console errors. (Oracle: UI changes require interactive browser testing.)

**Constraints:**
- (none from user)

**Oracle rules applied:**
- Deployment ritual: "Database migrations run BEFORE new app version" → ordered as Step 1.
- Post-action: "Adding a service to mgmt.gravhl.com → update link tree" → added as Step 3.
- Testing: "UI changes require interactive browser testing" → strengthened validation 3b.

Want changes, or type `$iterate` to execute?
```

User responds: "drop step 1; the migration was already done last week."

You delete step 1, renumber, re-run oracle merge (no new scope), re-print (`plan amended owl`). Wait for `$iterate` or next refinement.

## Example — teamify + rapid-fire adds

Plan `owl` (from above) now has 3 steps: 1a migration (already dropped in this thread), 2a build/deploy metrics-service, 3a update GUI link tree. User types:

```
$iterate-planner team this
```

You run the Teamify procedure: step 1 (deploy) and step 3 (link tree) are both "ship the service", but step 3 depends on step 2 existing first and is a distinct UI/docs concern — two teams. Write and print:

```
plan amended owl

**Teams** (run in parallel at $iterate time where independent):
| Team | Steps | Focus | Depends on | Agent | Status |
|---|---|---|---|---|---|
| deploy | 2 | Build and deploy the metrics-service container | — | backend-expert | pending |
| link-tree | 3 | Wire the new service into the mgmt.gravhl.com GUI link tree | deploy | documentation-expert | pending |

Grouped into 2 teams — link-tree depends on deploy finishing first, so they run sequentially, not in parallel, at $iterate time.

Want changes, or type `$iterate` to execute?
```

Later, the user fires off three quick adds without waiting between them:

```
$iterate-planner add: run a smoke test hitting the metrics endpoint after deploy
```
```
$iterate-planner add: update the on-call runbook with the new service's alert thresholds
```
```
$iterate-planner add: notify #platform-eng in slack once it's live
```

First call: previous turn wasn't `$iterate-planner` → full reprint. New step 4 ("smoke test") clearly matches `deploy`'s focus → auto-classified into `deploy`'s Steps (now `2,4`), no other table changes, no full re-teamify.

Second call: previous turn WAS an `$iterate-planner` add targeting `owl` → terse mode. New step 5 ("runbook") matches `link-tree`'s focus (docs/GUI-adjacent) → classified into `link-tree` (now `3,5`). Output is exactly:

```
+ owl step 5 added (team: link-tree)
```

Third call: same streak continues → terse mode again. New step 6 ("slack notify") doesn't clearly fit either team's Focus → left unassigned. Output:

```
+ owl step 6 added
```

The user's next message is "ok show me the plan" — full reprint, showing all 6 steps, both teams, and step 6 sitting unassigned (the `$iterate` coordinator will run it directly).

## Oracle merge — worked example

User plan: "add a new metrics service to mgmt.gravhl.com".

Oracle has an entry for **mgmt.gravhl.com new-service workflow** with:
- How: (1) deploy normally, (2) edit `mgmt/web-ui/links.yaml`, (3) commit + push, (4) load mgmt.gravhl.com in browser and click the new link
- Where: link tree at `~/workspace/gravhl/backend/mgmt/web-ui/links.yaml`
- Why: link tree is hand-maintained; skipping = invisible service

Iterate-planner folds in:

```
Na. Edit ~/workspace/gravhl/backend/mgmt/web-ui/links.yaml — add a "metrics" entry under the appropriate category.
Nb. The file diff shows the new entry; `yq '.links[].name' links.yaml | grep metrics` returns a hit.

Mb. Load https://mgmt.gravhl.com in a browser, click the new "metrics" link, confirm it routes to the metrics service AND the page renders without console errors.
Nb. (interactive — operator's eyes on the live click-through)
```

And in Constraints:
- Context: mgmt link tree is hand-maintained at `~/workspace/gravhl/backend/mgmt/web-ui/links.yaml`. No auto-discovery.

And in the Oracle context audit trail:
- Buzzword matched: "mgmt.gravhl.com" → loaded entry "mgmt.gravhl.com new-service workflow" from global → added 2 Steps, 1 Constraint.
