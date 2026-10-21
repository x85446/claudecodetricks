# Conductor

Conductor processes queued plans through configured AI launch sequences. The `iterate-run` binary owns scheduling, allowance checks, exclusive project ownership, execution and progress capture. The web dashboard, CLI and conductor skills control the same configuration.

Open **http://localhost:8420/**. All projects leads to each project's **Plans**, **Automations** and **Activity** tabs. All, Draft, Queued, Running, Blocked and Archived filters stay inside Plans. All includes every state, with blocked plans first. Draft means awaiting staging; Queued means approved or resumable work waiting for admission; Running identifies a live owner: **Running · Conductor** means automated execution, and **Running · CLI** means a manual iterate session. The AI tool name is separate from execution ownership. Blocked includes operator gates and paused work, with the explanation under **Why blocked**. Queue counts link to the exact project queue; plan and archive links preserve full goals, requirements, timelines and evidence. Existing `/plan?project=…&name=…` and `/archive?project=…&file=…` bookmarks open inside the project workspace. An archive describes recorded history, not a current process.

An explicit `running: false` releases CLI execution ownership; recent tool activity does not reclaim it. A fresh plan run timestamp still protects active CLI work. When an enabled automation admits an unblocked plan, its no-progress budget starts at that admission; time spent waiting for a blocker does not consume that budget. Explicit pauses, stops, schedules and allowance gates still apply.

A blocked plan does not stop the whole project queue. Conductor skips that plan and can admit another eligible queued plan. Dependencies written only in plan prose are not automatic cross-plan launch gates; keep dependent work unstaged or explicitly blocked until its prerequisite clears. Dependencies between teams inside a plan govern checkpoint selection.

The All projects table sorts by its column headers. **Configure** or **Edit** opens project settings, where **Tags** accepts comma-separated labels. Tags use lowercase and combine duplicates. Select tag buttons above the table to match any selected tag; **Clear filters** restores all projects. Sorting and tag selections persist in the URL.

## Set up a project

1. Choose **Add project** and select its existing folder. Registration starts disabled and leaves repository files alone. Each worktree has its own canonical identity, even when names match.
2. Open **Settings → AI Subscriptions**. Choose or add the CLI/account you use. Choose its AI tool and name, then use **Detect** to resolve the installed program on the Conductor host. **Verify installation** inspects capabilities without submitting model work. **Use another path** accepts a specific executable path. The optional plan/tier and defaulted power band stay visible; Advanced holds concurrency, usage freshness, literal argument rows, collector override and estimated reset. Diagnostics shows account/pool identities. Account binding and fresh provider telemetry establish actual allowance; a suggested template or estimated reset does not establish readiness.
3. Add a **Named Launch Sequence** under that subscription. Its name combines the subscription prefix and your suffix, such as `Codex · build`. Select model, reasoning and permission preset, or enter literal CLI arguments. **Effective command** shows the compiled launch. Presets such as YOLO remain editable; unsupported or conflicting transport arguments produce a validation error.
4. Create rules under **Settings → Named Automations**, then choose the saved automation in a project's **Automations** tab and select **Add to project**. Each attachment starts disabled. Enable it when ready, use **Automation enabled** on the dashboard to enable the project, and start Conductor to permit admissions. A staged plan is still subject to account availability, schedule, budget and ownership gates.

Use `/ip stage <plan>` in Claude or `$ip stage <plan>` in Codex to approve a saved plan for unattended execution. Editing its approved requirements removes staging. Release and release-candidate plans require a direct human iterate invocation.

The subscription summary card and editor share a **Subscription enabled** switch. Disabled summaries remain muted and editable, with a visible re-enable control. Editing project settings preserves activation; the dashboard owns the project switch. Disabling prevents future assignments and lets existing work finish its checkpoint.

Use the ⓘ buttons for field explanations. Enter opens focused help; Escape or an outside click dismisses it. **Refresh usage / bind account** acts on the saved subscription and can bind its authenticated account; it never enables a subscription. Codex collects usage through app-server. Automatic Claude collection requires CodexBar and the default Claude executable. A custom Claude path requires a collector override. Each argument row holds one literal value, including spaces or repeated flags.

## Choose an automation

| Mode | Behavior |
|---|---|
| Keep it working | Try launch sequences in your order whenever work is eligible. |
| Run on a schedule | Try the same ordered choices inside a named time window. |
| Follow subscription overdrive | Use participating sequences only while their subscriptions have active overdrive. |

Named Automations are shared definitions. **Used by** lists attached projects. Editing rules affects future admissions for every attachment; active assignments retain their saved inputs. **Duplicate** creates independent rules. A project's enabled state and priority are local; lower priority numbers run first. Queues, progress, occurrence consumption and history remain separate for each project. **Remove from project** detaches rules without deleting their definition or history. Delete accepts only unused definitions.

Each normal row has a **maximum band**: Green only, Green + Yellow, or Green + Yellow + Red. A Yellow row also accepts Green. The first eligible row wins. Repeated sequences allow choices such as Claude Green, Codex Green, Claude Yellow, Codex Red. Every row still respects hard quota, fresh telemetry and exclusive project ownership.

Normal automations honor their selected maximum bands. The advanced **Honor subscription overdrive** switch permits subscription overdrive to relax soft bands for that automation. **Only during subscription overdrive** also requires honor and waits outside active overdrive. Follow subscription overdrive sets both and uses an unordered participant list; overlapping eligible subscriptions prefer the soonest trusted reset, with fair tie handling.

One assignment runs per project. Conductor reevaluates choices between bounded task/validation checkpoints, so another AI cannot start the same project while an assignment owns it. Changing an automation affects subsequent admission; an active assignment retains its launch snapshot.

## Time windows and closing behavior

Named Time Windows use selected weekdays, start/end, timezone and repeat interval. Recurrence has no expiry date. An overnight window belongs to its starting day: Monday 22:00–06:00 includes Tuesday morning. The editable **Weekdays** and **Weekends** defaults select **Full day**, displayed as 00:00–23:59 and covering every instant through the final minute. With Full day off, end time is exclusive; equal start/end covers a full day. Times use 24-hour HH:MM. Recurrence uses a deterministic phase without an anchor-date field; existing stored phases remain effective. A project-wide time window further restricts its automations; Anytime adds no restriction.

Scheduled automations have two independent choices:

| Setting | Options |
|---|---|
| During the window | **Keep processing queued plans** or **Run one plan** |
| When the window ends | **Finish current checkpoint**, **Finish current plan**, or **Stop immediately** |

Finish current checkpoint is the default. Finish current plan continues only the plan already claimed in that occurrence, while budgets, quota, enablement, project limits and ownership still permit it. Run one plan keeps its claim across retries and restarts; parking does not free a second slot. Stop immediately interrupts the owned process tree and can leave unfinished edits. It records resumable state rather than success.

## Named Power Bands and overdrive

Settings contains AI Subscriptions, Named Power Bands, Named Time Windows and Named Automations. **Universal power band** is editable and is the default. Each subscription selects exactly one named definition. Editing a shared definition affects its assigned subscriptions; their usage, resets and overdrive remain independent unless they share an actual allowance pool. Referenced definitions require reassignment before deletion.

Bars increase from **0% used** to **100% used**. The black marker shows current use; ordinary bands use dark green, orange and red. Numeric remaining capacity and independent weekly/short-window reset clocks provide context. Missing or unsupported telemetry stays explicit.

Each subscription owns any number of enabled overdrive windows. A window repeats every week on selected days/time, or starts a number of hours before each trusted reset, and ends at its target, scheduled boundary or reset. Its target is **weekly percentage used**. A 60% target permits soft-band bypass while use is below 60%; 100% permits consumption until hard exhaustion. Bright green covers exactly the active used target; any ordinary green allowance beyond that remains dark green.

For a saved weekly window, changing **Start relative to** to **Hours before trusted reset** previews an equivalent offset from fresh, account-bound reset evidence. The preview preserves that allowance interval’s exact start/end and window identity. Multiple-day, multiweek or independently ending windows stay weekly when one offset cannot represent them. Future local start times follow the trusted reset and hour offset, including daylight-saving or provider reset changes.

Overdrive does not choose a project. A participating project automation supplies eligible queued work. It never bypasses hard provider limits, freshness, project schedules, disabled state, staging or locks. At target/end/reset, overdrive-only automations wait; ordinary honoring automations return to normal bands. Targets do not reserve a balance or guarantee an exact percentage at completion.

For example, a Codex subscription can have Saturday 00:00 → 100% used, Monday 00:00 → 100% used with an explicit Tuesday boundary, and Thursday 16:00 → 60% used. A Filemaster Follow subscription overdrive automation simply participates through a Codex launch sequence. Actual reset evidence governs reset-ended windows; Conductor does not redeem reset tokens.

## Try a configuration

**Try the power bands** uses only the draft policy, elapsed time and percentage used. Weekly elapsed time and usage are independent, as are five-hour elapsed time and usage. Six green/red points at hours 0–5 define the hourly curve; interpolation supplies fractional-hour thresholds. Existing fixed thresholds form constant curves. Real admission uses the allowance window's actual start time.

At exact zero, the black marker stays at zero and the current-sample label is green. Threshold regions retain their configured widths, including zero-width green. A sample of 2h 33m and 51% used describes two independent values. This tester has no subscription, overdrive or eligibility controls.

Subscription-window and automation testers call the same evaluator as admission. A disabled subscription’s sample chart labels its hypothetical enabled state; the decision row retains the saved disabled state. Samples do not save settings or launch work. At or above an applicable overdrive target, **✓ Overdrive achieved** retains bright green through the target while the marker shows actual sample use. Moving below the target recomputes active overdrive. This display does not grant launch permission; 100% use still denies work. Keep it working needs no day/hour unless a project-wide time window applies. Scheduled testing supplies time in the window's timezone. Overdrive testing offers only available enabled windows across participating subscriptions, uses one common sample instant and lets you vary weekly use.

Each tester has one results table. Only the selected eligible row receives a light-green highlight and a single Would launch label; other rows explain why they wait. Disabled or deleted windows cannot become available merely by selecting a sample.

Settings editors use draft dialogs. Save validates and commits; Cancel, Close and Escape discard the draft. Duplicate and Delete use link styling, while Edit and Add use buttons. Dialogs preserve background scroll and return focus to their origin. Browser Back/Forward and the Settings return control preserve project context.

## CLI and skills

`/iterate-conductor` or `/ic` in Claude, and `$iterate-conductor` or `$ic` in Codex, translate requests into binary commands. For example: “Keep Filemaster working with Claude Green, then Codex Yellow,” or “Run one plan on weeknights and finish the current checkpoint when the window closes.” Repeating a named request reuses its stable definitions. A preview request does not enable or dispatch it.

```sh
iterate-run conductor status --json
iterate-run conductor project list --json
iterate-run conductor explain --project PROJECT_ID --json
iterate-run conductor config show --json
iterate-run conductor sequence preview SEQUENCE_ID --subscription SUBSCRIPTION_ID --json
iterate-run conductor overdrive-conversion --subscription SUBSCRIPTION_ID --id WINDOW_ID --json
iterate-run conductor run list --project PROJECT_ID --json
iterate-run conductor event list --run RUN_ID --json
```

Writes require the current configuration revision:

```sh
iterate-run conductor config validate --file draft.json --json
iterate-run conductor config apply --file draft.json --revision REVISION --dry-run --json
iterate-run conductor config apply --file draft.json --revision REVISION --json
iterate-run conductor start --project PROJECT_ID --revision REVISION --json
```

A revision conflict preserves the concurrent edit. Read the current configuration, merge the intended fields and validate again. Full configuration applies commit related definitions atomically. `--store DIR` or `ITERATE_CONDUCTOR_DIR` selects an isolated store; the default is `~/.claude/iterate/conductor`. Skills never edit that store directly or run a second scheduler.

**Start/resume** enable admission and launch a binary worker. **Pause/stop** finish the active checkpoint and prevent subsequent admission. **Kill** interrupts only owned work. **Tick** attempts one admission; status/explain/simulate and dry-run do not launch. Project-scoped controls do not silently enable a stopped global conductor.

JSON commands return one envelope with revision and data or a structured error. Exit codes are 0 for success/read-only waiting, 1 for operation failure, 2 for invalid usage, 3 for revision conflict and 4 for gated or unavailable dispatch. `iterate-run conductor --help` lists resources and flags.

## Live progress and recovery

Project views and plan timelines share launch visibility: the effective sequence, usage age, admission reason and next opportunity. An enabled schedule supplies its next opening within a fourteen-day preview horizon; an unknown quota recovery, missing queue/telemetry condition or stopped conductor remains explicit. Active assignments show their saved launch sequence rather than a later configuration edit.

Plan details put project navigation above the plan identity, goal and progress summary. Conductor activity follows in a separate panel. The collapsible event log scrolls independently and follows incoming events while positioned at the bottom; scrolling back preserves your place. Usage observations expand separately.

The binary drains child stdout and stderr, stores protected logs, and normalizes activity into durable events. The portal polls those events and state; closing or disconnecting the browser does not stop execution. Reconnection resumes from a cursor. Execution-token totals describe the run, while account allowance comes from authenticated provider telemetry.

Each assignment contains one task/validation pair or a typed diagnosis, intake or delivery request. Only a matching validated result with concrete evidence advances its checkpoint. Exit zero alone is insufficient. Recovery checks process identity and durable ownership before resuming; ambiguous delivery remains visible for reconciliation. Paused, parked, exhausted and unavailable states include reasons rather than claiming completion.

`iterate-run conductor migrate --legacy --json` previews registered project settings. `--apply` commits the import with protected backups. Migration preserves existing definitions and flags, reports settings needing review and leaves ambiguous ownership paused. The dashboard and conductor share the single server on port 8420. The managed installer uses macOS launchd. Process supervision uses Unix ownership primitives; unsupported platforms fail closed.
