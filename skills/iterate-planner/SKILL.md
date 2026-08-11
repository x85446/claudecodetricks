---
name: iterate-planner
description: The planning half of the iterate stack. Formalizes a task into structured pair-format (1a task / 1b validation) BEFORE autonomous execution, and consults the project oracle (./.claude/data/oracle.md) to bake known checklists, testing requirements, gotchas, deployment rituals, and naming conventions into the plan (a no-op when no oracle exists). Triggers on "/iterate-planner", "plan this for iterate", "give me an iterate plan", "restate the plan", "plan with the oracle", and on managing saved plans ("list plans", "display our plans", "add to <name>", "delete <name>", "from <name> remove <x>"). Also triggers on teamify requests ("team this", "teamify", "make into team stack", "team up the plan", "reorganize into teams") which group the current plan's steps into named teams for parallel execution at /iterate time. Plans are saved, animal-named, and persistent under ./.claude/iterate/plans/. The user reviews, optionally refines in natural conversation, then runs /iterate (or /iterate <name>) to execute.
argument-hint: <optional context, e.g. "restate the plan from above" or "plan: 1. do X, 2. validate Y">
---

# /iterate-planner — Build the plan (oracle-aware), don't execute

The planning skill for the iterate stack:

| Skill | Role |
|---|---|
| `/iterate-planner` | **Plan** — formalize the task into the 1a/1b schema, baking in project lessons from the oracle |
| `/iterate` | **Execute** the plan autonomously until validation passes |

**This skill plans, it does not execute.** It writes a structured, animal-named plan under `./.claude/iterate/plans/<name>.md` with `phase: planned`, prints it back in paired 1a/1b format, and waits for either natural-language refinements or the user typing `/iterate` (or `/iterate <name>`) to kick off execution. It also manages saved plans (list / add-to / delete / remove-from) — see "Named plans" below.

Before writing, it always reads `./.claude/data/oracle.md` (and the global `~/.claude/skills/oracle/known.md`) and folds any in-scope project knowledge into the plan. When no oracle exists, that step is a silent no-op — the skill still runs and produces a plain plan.

It does **not** execute. It does **not** set up `/loop`. It does **not** take the running lock.

## Named plans (save / retrieve / list / manage)

Plans are **saved, named, and persistent**. Each plan is one file:

    ./.claude/iterate/plans/<name>.md

`<name>` is a common animal (dog, cat, fox, owl, elk, wren, mole, hare, lynx, crow, seal, toad, newt, ibis, …) assigned automatically when a plan is first created — one word, lowercase. **Get it by running `iterate-run name next`** from the project's own directory (a real installed binary — see "iterate-run" below), never by picking one yourself: THIS project's plans walk the alphabet on their own sequence (this project's 1st plan is an a-word, 2nd a b-word, …), while the actual word for each letter is drawn from one machine-wide "already used" set at `~/.claude/iterate-run/plan-names.json` — so this project's 1st plan and some other project's 1st plan are both a-words, just never the *same* a-word. Plain "not already present in this project's own `plans/`" used to let two unrelated projects both land on "wren" independently, which silently corrupted both their dashboards (same codename, different runs, merged as one). If `iterate-run` isn't installed, fall back to picking any common animal not already present in this project's `plans/` and tell the user the global registry is unavailable so the name isn't guaranteed unique machine-wide. The plan's `Started:` field records the date it was **drafted** (keep it forever; never reset on refinement). This is planning time, not execution time — leave `Executing:` unset here; `/iterate` writes it once, separately, at the exact moment it actually begins running the plan (see its SKILL.md). The dashboard's "Running for" figure reads `Executing:`, not `Started:`, precisely because a plan can sit drafted for hours or days before anyone runs it.

A single pointer file `./.claude/iterate/current` holds the name of the **current** plan — the one you add to by default. Resolve the current plan like this:

- `current` names an existing plan → that's the current plan.
- `current` missing/stale but exactly one plan exists in `plans/` → that plan is current (write its name into `current`).
- `current` missing and multiple plans exist → there is no current plan until one is named or chosen.

**Legacy migration (do silently on first touch):** if `./.claude/iterate/active.md` exists and `plans/` does not, move it to `plans/<name>.md` (assign a name via `iterate-run name next`, add a `name:` field near the top), write `current`, and delete `active.md`. Create `./.claude/iterate/plans/` if it doesn't exist.

## Teams (optional grouping for parallel execution)

A plan can be **teamed**: its Steps partitioned into named groups so `/iterate` dispatches one subagent per independent team to run concurrently instead of one agent working the whole Steps list serially. Teaming is **opt-in and explicit** — most plans stay flat, and flat plans behave exactly as they always have.

Team count and categories are **discovered per plan, up to roughly 10 teams** — there's no fixed pair, no preset taxonomy (not a "UI vs other" or "code vs everything else" template), and no bias toward the fewest possible groups. A small plan might yield one team; a large, multi-subsystem plan might yield seven or eight. Let the content decide each time.

**The actual test is independent context needs, not topic labels.** Splitting only pays off when it saves work — each team loading only the context it actually needs instead of one agent loading all of it, with no coordination cost lost in exchange. That means:
- Two steps belong in the **same** team if executing them requires the same underlying context — the same files, the same system, the same domain knowledge either way. Merging costs nothing (one agent already has that context loaded) and splitting would make two agents both pay to acquire it, with nothing gained and a real risk they make incompatible decisions about something neither can see the other doing.
- Two steps only belong in **different** teams when their context needs are genuinely independent — different files, different systems, no domain knowledge that has to be shared between them. That's where real parallel efficiency comes from.
- A topic label ("database", "UI") is a decent proxy for this but isn't the test itself. Two topically-similar steps needing genuinely disjoint context can still split; two topically-different steps that both need the same file or system should stay together.

**Trigger phrases** (route to the teamify operation, #5 below): "team this", "teamify", "make into team stack", "team up the plan", "reorganize into teams", "team-stack this" — optionally naming a plan ("teamify <name>").

**Schema** — a plan file with teams has a `teamed: true` field alongside `phase`/`running`/`name`, and a `## Teams` section (placed after `## Constraints`):

```markdown
## Teams
| Team | Steps | Focus | Depends on | Agent | Status |
|---|---|---|---|---|---|
| code | 1,2,4 | Backend service changes | — | backend-expert | pending |
| database | 3,5 | Schema migration + data backfill | — | backend-expert | pending |
| docs | 6 | Update README | code | documentation-expert | pending |
```

- **Team** — short kebab-case name, inferred from the plan's actual content (no fixed vocabulary — could be `code`/`database`/`infra`/`docs`/`frontend`/`tests`/whatever the plan naturally divides into).
- **Steps** — the step numbers from the existing flat `## Steps`/`## Validation` lists that belong to this team. The flat lists themselves are untouched — Teams is purely additive metadata layered on top.
- **Focus** — one line, what makes these steps a coherent unit.
- **Depends on** — other team name(s) that must be `status: done` before this team can start, or `—` for none. Infer from real ordering constraints (e.g. a migration must land before code that reads the new column), not from step number order alone.
- **Agent** — suggested Agent-tool `subagent_type` for `/iterate` to dispatch this team to (`backend-expert`, `frontend-engineer`, `documentation-expert`, `operations-engineer`, `quality-engineer`, `architecture-expert`, or `general-purpose` as the default when nothing fits better).
- **Status** — always written as `pending` by teamify/auto-classify. This is the one cell `/iterate` updates as it runs (`pending` → `in-progress` → `done` / `blocked (<reason>)`) — see below.
- **Unassigned steps** (steps not listed under any team) are executed directly by the `/iterate` coordinator itself, serially, same as an un-teamed plan — never forced into a bad-fit team.

**Table ownership is split, not exclusive:** `/iterate-planner` owns the table's *structure* — which teams exist, their Steps/Focus/Depends on/Agent, adding/renaming/removing teams. It always writes fresh rows with `Status: pending`. `/iterate` owns only the **Status** cell per row once execution starts — it updates that one value as teams progress and never adds/removes a team, never changes Steps/Focus/Depends on/Agent. Neither side ever touches the other's part of the table.

**`iterate-run`** is the real installed CLI binary (`~/go/bin/iterate-run`, built from `claudecodetricks`) that `/iterate` uses at execution time to wrap long-running commands with real heartbeat/progress tracking instead of guessed timers — see `/iterate`'s "Team dispatch" and "Know the baseline, don't guess it" for how it's used. `/iterate-planner` invokes it directly for one thing — `iterate-run name next`, to assign every new plan's codename (see "Named plans" above) — and otherwise references its version (see op 0 above) and can point to `iterate-run status` when a plan mentions wanting live visibility into a run.

### Creating vs. adding — bias hard toward the current plan

Creating a NEW plan must be **explicit**. The DEFAULT for any planning request is to **add to / refine the current plan**. Only create a new plan when:

- there are zero plans yet (the first plan — no name keyword needed), OR
- the user explicitly says "new plan", "create a new plan", "start a new/separate/fresh plan", or otherwise clearly frames the work as a distinct new effort.

If a current plan exists and the user just describes more work, **add it to the current plan** — do not spin up a second plan. When in doubt, add to current.

### Operation router — parse `$1` in this order

0. **version** — `$1` is exactly "version" (or "what version", "iterate version"): run `iterate-run version` and print its output verbatim as the answer — this is a real installed binary (see "iterate-run" below), not something to answer from memory, and it works the same from any project directory since it's on PATH. If the command isn't found, report "iterate-run isn't installed — run `make install` in claudecodetricks" rather than guessing a version. Then **stop** — this is a read-only op, no plan involved.

1. **list** — `$1` asks to see plans ("list plans", "display our plans", "show plans", "what plans do we have", or bare "list"): print every plan in `plans/`, one line each:

   ```
   <name>: started <YYYY-MM-DD> — <goal>        (current)
   ```

   Mark the current plan with `(current)`. Sort by `Started:` ascending. If no plans exist, say so. Then **stop** — this is a read-only op.

2. **delete `<name>`** — "delete `<name>`", "remove plan `<name>`", "drop `<name>`": delete `plans/<name>.md`. If it was the current plan, repoint `current` to the sole remaining plan (if exactly one) else clear it. If the plan is `phase: executing`, refuse and tell the user to let it finish or stop the loop first. Confirm in one line. Then **stop**.

3. **remove-from** — "from `<name>` remove `<thing>`", "remove `<thing>` from `<name>`", "from `<name>` drop step N": open `plans/<name>.md`, remove the matching Step + its paired Validation (renumber the rest 1:1), re-print that plan. Then **stop**.

4. **add-to-named** — `add to "<name>" that <thing>`, "to `<name>` add `<thing>`": open `plans/<name>.md`, append the new Step + an inferred Validation, set `current` = `<name>`, re-print that plan. Then **stop**.

5. **teamify** — `$1` matches a teamify trigger phrase ("team this", "teamify", "make into team stack", "team up the plan", "reorganize into teams", optionally "teamify `<name>`"): resolve the target plan (named, else current), run the Teamify procedure (see "Teamify procedure" below) over its full Steps list, write the `## Teams` table and set `teamed: true`, re-print the plan including the Teams table. Then **stop**. This is the only operation that does a full from-scratch reclustering — never run it implicitly as part of any other op.

6. **new plan** — `$1` contains "new plan" / "create a new plan" / "start a new/separate/fresh plan": create a new plan with a name from `iterate-run name next`, set it current, write and print it (proceed through the Steps below).

7. **default (the common case)** — anything else describing work:
   - a current plan exists → **add to / refine the current plan** (proceed through the Steps below, targeting the current plan file). If the current plan has `teamed: true`, also run the cheap single-step Team classification (see "Auto-classify on add" below) on each newly added step — this is O(1) per step, never a full re-teamify.
   - no plans exist → create the first plan (name from `iterate-run name next`, set current).

For ops 3–7, run the oracle merge (Step 4) on whatever plan you end up writing/refining — including add-to-named and default-add operations, so newly added steps get oracle-aware validations.

### Teamify procedure (op 5 — full reclustering)

1. Read the target plan's Goal + full Steps + Validation lists.
2. For each step, identify what context executing it actually requires — which files, which system/service, which domain knowledge. This is the real unit of analysis, not the step's topic label.
3. Group steps by **shared context need**: any two steps that require the same files/system/domain knowledge go in the same team, regardless of how different they sound topically — splitting them would mean two agents both paying to load context one agent already has, for no gain. Steps only split into different teams when their context needs are genuinely independent (different files, different systems, nothing that has to be known by both) — that's the only case where parallel dispatch actually saves work instead of just adding coordination overhead. Up to roughly 10 teams; no target count, no fixed taxonomy (not "code vs UI vs other," not any preset pair). A 4-step plan might yield one team or four; a 30-step plan might yield six — whatever the actual context boundaries support. Don't default toward fewer teams for its own sake, and don't force extra splits the context doesn't support either.
4. For each group: name it (short kebab-case), write a one-line Focus, list its step numbers, infer real ordering dependencies on other teams (data must exist before code reads it, infra must exist before code deploys to it — not just numeric step order), and suggest an `Agent` (subagent_type) that best matches the Focus.
5. Steps that don't cleanly fit any group stay unassigned (omit from the table) rather than forcing a bad fit.
6. If the plan has no genuine independent context boundaries (e.g. every step touches the same file/system, or it's a strictly sequential dependency chain), don't write a Teams section at all — report "no independent context boundaries found — steps are sequential, staying flat" and leave `teamed` unset. This is a valid, expected outcome, not a failure.
7. Write `## Teams` into the plan file, set `teamed: true` (only when a Teams section was actually written), re-print the full plan.
8. In the presented output, add one line explaining the grouping rationale (what context boundary separates each team) and which teams can run in parallel (those with no `Depends on` entries pointing at an unfinished team).

### Auto-classify on add (part of op 7 — cheap, O(1) per step, never a full reorg)

When the current plan already has `teamed: true` and a new step is appended:

1. Compare the new step's content against each existing team's Focus. If it clearly matches exactly one team, append its number to that team's Steps list in the `## Teams` table — nothing else about the table changes (don't touch Depends on / Agent / other teams).
2. If it doesn't clearly match any existing team, leave it unassigned (simply don't add it to the Teams table) — the coordinator will execute it directly at `/iterate` time. Never force a bad fit, and never trigger a full teamify pass just to place one step.
3. This is a single classification judgment, not a re-analysis of the whole plan — it must stay cheap so rapid-fire `/iterate-planner add ...` calls don't slow down.

## When to use

- The user is in a conversation where a plan has been discussed and wants it formalized into a saved iterate plan.
- The user types `/iterate-planner` with brief instructions (often just "restate the plan" or "plan it with these tweaks").
- The user wants a chance to review and refine before committing to autonomous execution.
- The user wants to manage saved plans (list, add to, delete, remove from).

## Steps

### 1. Read the oracle

1. Check for `./.claude/data/oracle.md` and `~/.claude/skills/oracle/known.md`. If both absent: note "no oracle — planning without project context" and skip to Step 2 with empty oracle data. This is a silent no-op, not a refusal.
2. Parse the oracle index/sections (Post-action checklists, Testing requirements, Gotchas, Deployment rituals, Naming conventions, Architecture notes / buzzword 5W+H entries). Treat any section as optional — only use what's present.

### 2. Identify the plan source

- If `$1` contains substantive instructions (numbered steps, validation criteria): use `$1` as the source.
- If `$1` is brief or refers to recent context (e.g. "restate the plan", "with these changes: …"): read the recent conversation. The plan should already be present in some form — your job is to formalize it.

### 3. Check the target plan file

Resolve the target plan per the operation router (usually the current plan, or a new animal-named file for a new plan):

- If the target plan exists with `phase: executing`: **STOP**. Report "that plan is executing; let it finish or use `/iterate` to resume it before re-planning." Do not modify the file.
- If the target plan exists with `phase: planned`: treat as a **refinement** — preserve `name`, `Started`, `CWD`, `Goal` (unless the user is changing it), update the Plan/Constraints sections AND re-apply the oracle merge.
- If creating a new plan: get a name from `iterate-run name next`, write fresh, set `current` to it.

### 4. Merge oracle into the plan (buzzword-scoped lookup)

Oracle is a **buzzword glossary** (5W+H per registered term), not a categorical rule bin. Merge logic:

1. **Build the buzzword index** from both oracle stores:
   - Project: `./.claude/data/oracle.md` index section
   - Global: `~/.claude/skills/oracle/known.md` index section
2. **Scan the user's plan** (Goal + Steps + the recent conversation context that informed it) for buzzword matches against the index. Match case-insensitive substring. Match plural / verb forms.
3. **For each matched buzzword**, read its 5W+H entry from the right store (project wins on conflict).
4. **Fold the entry's fields into the plan** as follows:

   | Oracle field | Plan placement |
   |---|---|
   | **How** (commands, rituals, procedural steps) | Add as **new Steps + Validations** at the right position in the plan (often appended at end; insert mid-plan if ordering is explicit like "X before Y"). Each How-step gets a paired interactive Nb. If the How includes a **known duration** for an operation this plan performs (e.g., "compiling X normally completes in under 60s"), *also* add it as a Constraint with a `Timing:` prefix on the step that runs it (`Timing: app-build compile normally completes in <60s`) — this becomes the real expectation `/iterate` uses instead of guessing (see `/iterate`'s "Know the baseline, don't guess it"), not just a Step/Validation. |
   | **Where** (paths, repos, related skills) | Add as Constraints with `Context:` prefix — e.g., "Context: GUI link tree lives at `mgmt/web-ui/links.yaml`." |
   | **When** (when to use / when not to) | If "when not to" applies, add as a Constraint. If "when to use" matches the plan's scope, that's the justification for folding the entry in. |
   | **Why** (problem it solves) | Note in the Oracle context audit trail — informs ordering but doesn't add Steps. |
   | **Who** | Note in the Oracle context audit trail. If a step requires a specific operator (e.g., "Only Travis can rotate the OpenBao root token"), surface as a Constraint. |
   | **What** | Note in the Oracle context audit trail; informs interpretation but doesn't add Steps directly. |

5. **Don't fold in buzzwords whose scope clearly doesn't apply.** If the plan mentions "incusmagic" in passing but the work is unrelated to incus operations, log "skipped: incusmagic mentioned but out of scope" in the audit trail and move on.

#### Example

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

#### When oracle has no matching entries

The plan is written without oracle augmentation. Note in the audit trail: "Oracle scanned, 0 matches. Buzzwords in plan: [list]. Use `/oracle add <buzzword>` to register one."

### 5. Write the plan file

Schema, written to `./.claude/iterate/plans/<name>.md`:

```markdown
# Iterate Task — <short title>

name: <animal>
Started: <UTC timestamp> (planned)
CWD: <pwd>
phase: planned
running: false
planner: iterate-planner    # marker so /iterate knows oracle was consulted
teamed: false               # set true only after a teamify pass writes ## Teams

## Goal
<one sentence>

## Steps
1. <task>
2. <task>
...

## Validation
1. <how to verify step 1 — concrete, runnable assertion, including interactive checks where required>
2. <how to verify step 2>
...

## Constraints
- <user constraint>
- <oracle gotcha if applicable>
- Naming: <oracle convention if applicable>
- Context: <oracle architecture note if applicable>
- Timing: <known duration for a specific operation, if the oracle has one>

## Teams
<!-- Only present when teamed: true. See "Teams" section above for schema. Omit entirely on flat plans. -->

## Oracle context applied
<!-- Audit trail: which oracle rules were folded in. Lets the user see what changed. -->
- Post-action: <entry> → added as Step N
- Testing: <entry> → strengthened validation N
- Gotcha: <entry> → added as Constraint
- ...

## Decisions log
(empty until execution)

## Status / Log
(empty until execution)
```

Storage uses two parallel numbered lists (Steps + Validation, indexed 1:1). The `planner:` field tells `/iterate` "oracle was already consulted — don't re-read it" (see iterate's behavior).

### 6. Present the plan

Always lead with a one-line **save confirmation** so the user knows it persisted:
- created a brand-new plan → `plan written to <animal>`
- appended to / refined an existing plan → `plan amended <animal>`

Then print the plan in paired 1a/1b format, AND show which oracle rules were applied:

```
plan written to owl

**Plan ready** — owl — ./.claude/iterate/plans/owl.md (phase: planned, oracle-aware)

**Goal:** <goal>

1a. <step 1>
1b. <validation 1>

2a. <step 2>
2b. <validation 2>

...

**Constraints:** <list, if any>

**Oracle rules applied:**
- Post-action: <entry> → added step <N>
- Testing: <entry> → strengthened validation <N>
- Gotcha: <entry> → added constraint

(Oracle rules NOT applied because out of scope: <N entries skipped>.)

Want changes, or type `/iterate` (or `/iterate <name>`) to execute?
```

If the project had no oracle, omit the "Oracle rules applied" section and add a one-liner: `(No oracle found for this project — planned without project context. Use /oracle remember to start one.)`

If the plan is teamed (`teamed: true`), insert the Teams table between Constraints and Oracle rules applied:

```
**Teams** (run in parallel at /iterate time where independent):
| Team | Steps | Focus | Depends on | Agent | Status |
|---|---|---|---|---|---|
| code | 1,2,4 | Backend service changes | — | backend-expert | pending |
| database | 3,5 | Schema migration + data backfill | — | backend-expert | pending |
| docs | 6 | Update README | code | documentation-expert | pending |
```

### 7. Handle refinements

If the user responds with changes in natural conversation, update the current plan file in place AND re-run the oracle merge (in case the refinement brought new oracle-relevant scope). Re-print the full plan. Keep `phase: planned`. Don't archive — overwrite. Refinements target the **current** plan unless the user names a different one — never spin up a second plan for a refinement.

### 8. Rapid-fire terse mode

The user often queues several `/iterate-planner add <thing>` calls back-to-back without reading each result — dictating a stream of additions and hitting enter repeatedly. Printing the full plan (goal + every paired step + oracle audit + footer prompt) on every single one of those is slow to produce and clutters the conversation by the time they catch up.

Detect this from the conversation itself — no extra state needed: if the **immediately preceding turn** in this conversation was also an `/iterate-planner` add-type invocation (ops 4 or 7) targeting the **same current plan**, treat this as mid-streak and respond with exactly one line instead of the full reprint:

```
+ <animal> step <N> added (team: <name>)
```

or, if unassigned/unteamed:

```
+ <animal> step <N> added
```

Print the **full** plan (with the complete footer, including "Want changes, or type `/iterate` to execute?") on:
- the first invocation of a streak (previous turn wasn't `/iterate-planner`, or targeted a different plan),
- any invocation that isn't a plain add (list, delete, remove-from, add-to-named, teamify, new plan — those already have their own defined output),
- or when the user's message signals they're done queuing ("that's everything", "ok go", "show me the plan", "that's it for now") — even mid-streak.

When genuinely unsure whether the streak has ended, print the full plan — a slightly-too-early full reprint costs nothing; silently staying terse when the user expected to see their plan does.

## Rules (hard)

1. **Never execute the plan.** Only `/iterate` does that. Your responsibility ends at writing + presenting.
2. **Never set up `/loop` or take the `running:` lock.** Those happen at `/iterate` execution time.
2a. **Creating a new plan is explicit-only.** Default every planning request to the current plan; only create a new animal-named plan when the user says "new plan" (or there are zero plans). When unsure, add to current. See "Named plans" above.
3. **Apply oracle rules selectively, not blindly.** Only fold in rules that are plausibly in scope for the user's plan. Log skipped rules in the audit trail so the user can override.
4. **Be transparent about what oracle changed.** The "Oracle rules applied" section is mandatory when the oracle contributed anything. The user must be able to see what got bolted on.
5. **Pair every step (Na) with a validation (Nb).** If the user didn't specify a validation, infer the most reasonable one (a runnable command + expected output) — and if a Testing requirement from oracle applies, use it for the inferred validation. Note "(validation inferred)" so they can override.
6. **Don't be cute about validations — interactive testing mandate.** Each Nb must be a concrete, observable check (a command + expected output, a file existing, a count, a service responding), and it must EXERCISE the system, not just READ the code. If a change touches:
   - a **UI** → the validation must include actually loading the page in a browser and clicking through the changed flow (not "the JSX looks correct").
   - an **API endpoint** → a real `curl` / request hitting the running service with realistic input (not "the handler signature looks right").
   - a **CLI command** → actually running the command end-to-end (not "the flag parsing looks right").
   - a **migration / script** → executing it against a real (test or local) target (not "the SQL parses").
   "Static code review passes" / "tests pass" alone is NOT validation when the change ships behavior. Strengthen validations to require interactive testing where the oracle says so.
7. **Don't write to the oracle from here.** This skill READS the oracle. Only the `/oracle` skill writes to it. If the user shares a new lesson while refining the plan, suggest `/oracle remember <fact>` separately.
8. **No oracle is fine.** Don't refuse to run on projects without an oracle — just plan without the extra context and say so in the footer.
9. **NEVER write "STOP", "abort", "give up", "halt", or "manual intervention required" into any step or validation.** The whole point of `/iterate` is autonomous recovery — it exhausts alternatives, it doesn't bail at the first sign of trouble. Validations describe the desired post-condition only. Recovery from a failed validation is `/iterate`'s job at execution time, not yours.
10. **If a step's natural form is "check X, and if missing, halt" — rewrite it as a productive step that creates/repairs X.** Examples:
    - Bad: "1a. Verify bridge exists. 1b. `incus network list` shows it; STOP if absent."
    - Good: "1a. Ensure IZUMA bridge exists on ranger; create it from the existing template if missing. 1b. `incus network list` shows IZUMA bridge AND ranger can ping 172.29.28.1."
    The plan should describe what we want to be TRUE at the end of the step. If after rewriting a step still requires the user to act (physical hardware change, billing decision), say so in Constraints — not as a stop-mid-flight instruction. `/iterate` surfaces it during execution as a giveup with context, and the user re-invokes after handling it.
11. **NEVER produce a status check, blocker report, or "either/or" decision menu in place of a plan.** When invoked, your ONLY output is: the save line + paired 1a/1b plan + oracle audit trail + "Want changes, or type `/iterate` to execute?" prompt. If you catch yourself writing "Honest blocker", "To proceed", "Either ... Or ...", "this is your status check", or asking the user to pick between conservative-vs-aggressive paths — **STOP and rewrite as a plan**. Pick the most aggressive path consistent with the user's stated intent and commit to it. The user invoked you to plan; they did NOT ask for a decision menu.
12. **Risk acceptance is standing.** If the user has said (now or earlier in this conversation) "this isn't production", "accept the risk", "do it everywhere", "switching it out is fine", "proceed", or equivalent — treat that as a permanent setting for the planning scope. Do NOT re-prompt for risk acceptance per-step. Do NOT surface risk as a reason to ask the user to choose between two paths. Plan as if the risk is accepted.
13. **Rollback in a plan is never terminal.** If a step needs rollback on failure, the validation MUST also include "then retry, up to N times, until success." Never write "rolled back per chart" or "failures roll back and stop." Recovery is `/iterate`'s job — your plan describes the desired end state (every chart migrated, everything green), not the give-up condition.
14. **Commit to the full goal, not a one-item pilot — unless the user explicitly asked for a pilot.** If the user said "port everything" or "do the whole sweep," plan to do the whole sweep. Don't downgrade to "let's try one and check in." That downgrade IS the status-check failure mode dressed up as caution.
15. **Scope validations to "caused by this work," not "all global state."** Broad checks like `kubectl get pod -A | grep -v Running | wc -l == 0` catch pre-existing failures and will read as "blocked" when they shouldn't. Prefer scoped checks: "pods in the changed namespaces are Running", "Applications touched by this run are Synced", or "no NEW non-Running pods compared to baseline captured at run start." If the goal genuinely IS cluster-wide health, say so explicitly in the Goal section so the executor knows pre-existing failures are in-scope.
16. **Teamify only on explicit request.** Never invent the first Teams table unassisted — a plan stays flat unless the user says a teamify trigger phrase. Most plans should stay flat; teaming is for plans with genuinely independent tracks of work.
17. **Auto-classify on add is a single judgment call, never a re-run of teamify.** Once `teamed: true`, slot each newly appended step into the best-fit existing team in one cheap decision, or leave it unassigned. Don't re-cluster the whole plan on every add — that's what makes rapid-fire adds stay fast.
18. **Never invalidate team membership except through teamify or remove-from.** Refining Steps/Validation/Constraints text must not silently drop a step's team assignment. If `remove-from` deletes a step that belonged to a team, remove its number from that team's row too (renumbering the rest) — don't leave a stale reference to a step that no longer exists.
19. **Rapid-fire streaks get a one-line reply, not a full reprint.** See "Rapid-fire terse mode" above — this exists because the user queues several adds in a row without reading each one; a full plan dump on every single one is slow and clutters the conversation. Always show the full plan on the first invocation of a streak and whenever there's genuine doubt about whether the streak ended.
20. **No fixed team count or taxonomy — split on independent context needs, not topic labels.** Two steps stay in the same team whenever they need the same files/system/domain knowledge, even if they sound topically different — splitting those gains nothing and costs two agents paying to load the same context. Two steps only go in different teams when their context needs are genuinely independent — that's the only case parallel dispatch actually saves work. Never default to a binary split ("UI vs other", "code vs everything else") and never bias toward the fewest possible groups just because fewer is simpler to write; also never bias toward the most possible groups if steps genuinely share context. Up to roughly 10 teams, discovered from actual context boundaries in the plan, not chosen from habit.

## Examples

Two full worked examples (a plain restate-the-plan run with oracle merge, and a teamify + rapid-fire-adds streak) live in [examples.md](examples.md) — load that file when you need to see the exact output shape end to end, e.g. building a similar plan-writing skill from this one as a template, or checking an edge case in the operation router / auto-classify / rapid-fire terse-mode logic against a concrete run.
