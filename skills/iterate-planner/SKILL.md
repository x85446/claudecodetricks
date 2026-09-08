---
name: iterate-planner
description: The planning half of the iterate stack. Formalizes a task into paired 1a-task / 1b-validation format BEFORE autonomous execution, consulting the project oracle to bake in known checklists, gotchas, and deployment rituals. Names each plan's feature branch but never creates it — planning stays on your current branch, so the status line reads main until execution starts; plans are teamed by default and end with three standing finishers (Makefile, TESTMASTER, product-docs). Plans are saved and animal-named under ./.claude/iterate/plans/. Never executes — the user runs /iterate for that.
when_to_use: Triggers on "/iterate-planner" or its alias "/ip", "plan this for iterate", "give me an iterate plan", "restate the plan", "plan with the oracle". Plan management: "status" (git + plans snapshot), "publish" / "show plan" (re-render read-only), "list plans", "add to <name>", "delete <name>", "from <name> remove <x>", "close <name>" (archive unfinished, branch left unmerged), "roll <name>" (carry unfinished steps to a new plan, same branch), "turn these notes into a plan" / "notes-to-plan". Teaming: "team this", "teamify", "team up the plan", "reorganize into teams"; reverse with "flat", "flatify", "un-team", "remove teams". Also recognizes the FFIV macro (Find, Fix, Iterate, Verify) for quality sweeps over a named scope, and the "skip" modifier ("skip", "skip finishers", "skip the pre-baked steps", "skip tests/docs/makefile") which suppresses the standing end-of-plan finishers for that plan.
argument-hint: <optional context, e.g. "restate the plan from above", "plan: 1. do X, 2. validate Y", or "flat" / "flatify" to un-team the current plan>
version: 5.0.0
---
<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

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

A plan can be **teamed**: its Steps partitioned into named groups so `/iterate` dispatches one subagent per independent team to run concurrently instead of one agent working the whole Steps list serially. **Teaming is the default** — every newly-created plan gets a Teamify pass automatically (see "Write the plan file" below) unless the request explicitly asks to stay flat ("flat", "flatify", "keep it flat", "no teams", "don't team this"), or the plan genuinely has no independent context boundaries to split on (a normal, expected Teamify outcome — see "Teamify procedure" below). An already-teamed plan can be converted back with `/iterate-planner flat` / `flatify` (procedure in [procedures.md](procedures.md)); an already-flat plan can still be teamed on request with the existing teamify trigger phrases.

Team count and categories are **discovered per plan, up to roughly 10 teams** — there's no fixed pair, no preset taxonomy (not a "UI vs other" or "code vs everything else" template), and no bias toward the fewest possible groups. A small plan might yield one team; a large, multi-subsystem plan might yield seven or eight. Let the content decide each time.

**The actual test is independent context needs, not topic labels.** Splitting only pays off when it saves work — each team loading only the context it actually needs instead of one agent loading all of it, with no coordination cost lost in exchange. That means:
- Two steps belong in the **same** team if executing them requires the same underlying context — the same files, the same system, the same domain knowledge either way. Merging costs nothing (one agent already has that context loaded) and splitting would make two agents both pay to acquire it, with nothing gained and a real risk they make incompatible decisions about something neither can see the other doing.
- Two steps only belong in **different** teams when their context needs are genuinely independent — different files, different systems, no domain knowledge that has to be shared between them. That's where real parallel efficiency comes from.
- A topic label ("database", "UI") is a decent proxy for this but isn't the test itself. Two topically-similar steps needing genuinely disjoint context can still split; two topically-different steps that both need the same file or system should stay together.

**Trigger phrases** (route to the teamify operation, #5 below): "team this", "teamify", "make into team stack", "team up the plan", "reorganize into teams", "team-stack this" — optionally naming a plan ("teamify <name>").

**Flat trigger phrases** (route to the flatify operation, #6 below — the reverse; procedure in procedures.md): "flat", "flatify", "make flat", "convert to flat", "un-team", "remove teams", "flatten the plan", "go back to flat" — optionally naming a plan ("flatify <name>"). Also recognized inline during plan creation/refinement ("plan this, keep it flat", "no teams") to skip the now-default auto-teamify pass for that request.

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

## One plan = one feature branch

Every plan in a git repo lives on its own feature branch, managed through the **`feature-branch` skill** (`[skill: /feature-branch]`) — never by hand-rolled git commands. The lifecycle:

- **At new-plan creation** (here, in the planner): **do NOT create or check out the branch.** Compute the name it will have and record it as `branch: feature/<name>-<slug>` with `branch-created: false`. Planning stays on whatever branch the user is already on — normally the default one.

  Planning is not editing the repo. The plan file lives under `./.claude/iterate/`, which is project metadata (commonly gitignored outright), so writing it on the default branch dirties nothing tracked. Creating a branch at plan time bought nothing and cost the single clearest signal the user has: **a status line reading `main ✔` means no iterate work is outstanding.** Switching branches the moment planning began destroyed that, leaving "not on main" to mean either real unfinished work or merely that someone once opened a planning session — and the two are indistinguishable at a glance.

  **feature-branch's pre-edit gate does not apply to the plan file.** That gate protects tracked source from landing on a default branch; `./.claude/iterate/plans/<name>.md` is neither tracked source nor the plan's work product. Write it where you stand.
- **During execution**: `/iterate` checks out the plan's `branch:` at execution start and all work happens there — see its SKILL.md.
- **At all-green completion**: `/iterate` runs the merge flow (push → PR → merge to the default branch → delete the branch) automatically. **A plan that did not finish all-green NEVER merges** — not on blocked, not on close, not on roll-forward — unless the user explicitly orders a merge.
- **Not a git repo** (no `.git` — e.g. a Google Drive project): skip all of this silently; write the plan with no `branch:` field and note "not a git repo — no feature branch" in the audit trail. Everything else works unchanged.
- **Refinements never touch the branch.** Adding/removing steps edits the plan file on whatever branch is checked out; only creation makes a branch, only `/iterate` merges one.

Two plan-management operations exist specifically for the not-all-green endings (ops 7 and 8 below): **close** (archive as-is, merge withheld) and **roll** (carry unfinished steps to a new plan that inherits the SAME branch). Both must state plainly that the merge has not happened.

### Creating vs. adding — bias hard toward the current plan

Creating a NEW plan must be **explicit**. The DEFAULT for any planning request is to **add to / refine the current plan**. Only create a new plan when:

- there are zero plans yet (the first plan — no name keyword needed), OR
- the user explicitly says "new plan", "create a new plan", "start a new/separate/fresh plan", or otherwise clearly frames the work as a distinct new effort.

If a current plan exists and the user just describes more work, **add it to the current plan** — do not spin up a second plan. When in doubt, add to current.

### Operation router — parse `$1` in this order

0. **version** — `$1` is exactly "version" (or "what version", "iterate version"): run `iterate-run version` and print its output verbatim as the answer — this is a real installed binary (see "iterate-run" below), not something to answer from memory, and it works the same from any project directory since it's on PATH. If the command isn't found, report "iterate-run isn't installed — run `make install` in claudecodetricks" rather than guessing a version. Then **stop** — this is a read-only op, no plan involved.

0.5. **status** — `$1` is exactly "status" (or "show status", "plan status", "git status" in an iterate context): run the Status report procedure (see [procedures.md](procedures.md)) and print its fixed-format output. Then **stop** — read-only, touches nothing.

0.7. **publish / show plan** — `$1` is "publish", "show plan", "show the plan", "display the plan", optionally naming a plan ("publish `<name>`", "show plan `<name>`"): resolve the target plan (named, else current) and render it in the FULL presentation format from Step 7 — the `**Plan ready**` header line (name, path, phase, oracle-aware, teamed/flat with the one-line rationale), Branch line, Goal, every `N-prompted:` + Na/Nb pair, Constraints, Teams table if teamed, Access preflight, Oracle rules applied — exactly as if the plan had just been written, except with NO save-confirmation line (nothing was written) and no trailing refinement prompt needed beyond the standard `Want changes, or type /iterate to execute?`. Read-only: touch nothing, modify nothing, re-run nothing (no oracle/preflight re-scan — render what's in the file). This is the formal version of the mid-streak "show me the plan" escape: it always ends a rapid-fire terse streak with a full render. Then **stop**.

1. **list** — `$1` asks to see plans ("list plans", "display our plans", "show plans", "what plans do we have", or bare "list"): print every plan in `plans/`, one line each:

   ```
   <name>: started <YYYY-MM-DD> — <goal>        (current)
   ```

   Mark the current plan with `(current)`. Sort by `Started:` ascending. If no plans exist, say so. Then **stop** — this is a read-only op.

2. **delete `<name>`** — "delete `<name>`", "remove plan `<name>`", "drop `<name>`": delete `plans/<name>.md`. If it was the current plan, repoint `current` to the sole remaining plan (if exactly one) else clear it. If the plan is `phase: executing`, refuse and tell the user to let it finish or stop the loop first. If the plan had a `branch:` with commits on it, say so — the branch is left in place, unmerged (sweep it later with `/feature-branch cleanup` or merge it explicitly). Confirm in one line. Then **stop**.

3. **remove-from** — "from `<name>` remove `<thing>`", "remove `<thing>` from `<name>`", "from `<name>` drop step N": open `plans/<name>.md`, remove the matching Step + its paired Validation + its Provenance line (renumber all three lists 1:1), re-print that plan. Then **stop**.

4. **add-to-named** — `add to "<name>" that <thing>`, "to `<name>` add `<thing>`": open `plans/<name>.md`, append the new Step + an inferred Validation, set `current` = `<name>`, re-print that plan. Then **stop**.

5. **teamify** — `$1` matches a teamify trigger phrase ("team this", "teamify", "make into team stack", "team up the plan", "reorganize into teams", optionally "teamify `<name>`"): resolve the target plan (named, else current), run the Teamify procedure (see "Teamify procedure" below) over its full Steps list, write the `## Teams` table and set `teamed: true`, re-print the plan including the Teams table. Then **stop**. This is the only operation that does a full from-scratch reclustering — never run it implicitly as part of any other op. (Since teaming is now the default on new plans, this op mainly matters for teaming a plan that was created flat — by explicit request or because the original Teamify pass found no boundaries to split on.)

6. **flatify** — `$1` matches a flat trigger phrase ("flat", "flatify", "make flat", "convert to flat", "un-team", "remove teams", "flatten the plan", "go back to flat", optionally "flatify `<name>`"): resolve the target plan (named, else current), run the Flatify procedure (see [procedures.md](procedures.md)), re-print the plan without the Teams table. Then **stop**. The reverse of op 5 — always available as an escape hatch now that teaming is the default.

7. **close** — "close `<name>`", "close the plan", "archive `<name>` as is", "wrap it up unfinished", "close it out": archive the plan even though not everything finished. Run the Close procedure (see [procedures.md](procedures.md)). Then **stop**. The defining property: **the feature branch is NOT merged** — the close report must say so explicitly.

8. **roll** — "roll `<name>`", "roll the uncompleted steps to a new plan", "carry the unfinished work forward", "roll it over": create a NEW plan holding only the source plan's unfinished steps, **inheriting the same feature branch**, and archive the source. Run the Roll-forward procedure (see [procedures.md](procedures.md)). Then **stop**. Same defining property as close: no merge happened, and the report says so.

8.5. **notes-to-plan** — `$1` matches "notes-to-plan `<topic>`", "turn these notes into a plan", "turn the latest notes into a plan", "plan from notes": resolve the notes file (`./.claude/iterate/notes/<topic>.md`; unnamed → the `notes/current` pointer, else the most-recently-modified `status: open` notes file — see `/iterate-notes`). This is a NEW-plan creation (proceed exactly as op 9 — name from `iterate-run name next`, feature branch, oracle merge, access preflight, auto-teamify) with the notes file as the plan source (Step 2): each `## Notes` line becomes candidate step material, each `## Decisions` line binds as a Constraint or shapes a step (decisions are settled — don't re-litigate them), `## Open questions` surface as `Inferred:` decisions the planner makes and logs (never as questions back to the user), and `## Research appendix` is mined for context, not steps. Provenance for notes-derived steps cites the source: `Notes <topic>: <the synthesized ask>`. After writing the plan, set the notes file's `status: consumed (plan: <name>)` — it stays in `notes/` as the record of where the plan came from.

9. **new plan** — `$1` contains "new plan" / "create a new plan" / "start a new/separate/fresh plan": create a new plan with a name from `iterate-run name next`, set it current, write and print it (proceed through the Steps below — this includes the now-default auto-Teamify pass unless `$1` also carries a flat trigger phrase, see Step 6 "Write the plan file", and the feature-branch creation per "One plan = one feature branch" above).

9.5. **`skip` is a modifier, never an op.** If `$1` contains a skip trigger, strip it from the request text, set the suppression per Step 5.8, and route the *remainder* through the ops above (a bare `skip` with nothing left routes to op 10). Never treat `skip` as the plan's subject matter.

10. **default (the common case)** — anything else describing work:
   - a current plan exists → **add to / refine the current plan** (proceed through the Steps below, targeting the current plan file). If the current plan has `teamed: true`, also run the cheap single-step Team classification (see "Auto-classify on add" below) on each newly added step — this is O(1) per step, never a full re-teamify. If the current plan is flat, it stays flat on ordinary refinement (adding to a flat plan never auto-teamifies mid-stream — that would re-cluster on every add; use the explicit teamify trigger if the plan has grown enough to warrant it).
   - no plans exist → create the first plan (name from `iterate-run name next`, set current) — same auto-Teamify-by-default and feature-branch-creation path as op 9.

For ops 3–6 and 9–10, run the oracle merge (Step 4) AND the access preflight scan (Step 5) on whatever plan you end up writing/refining — including add-to-named and default-add operations, so newly added steps get oracle-aware validations and any newly-introduced access dependency gets a verification step. (Ops 7 and 8 move existing content without re-planning it — no oracle/preflight re-run there.)

### FFIV macro — recognized anywhere in `$1` or an added step

**FFIV = Find, Fix, Iterate, Verify** — a quality sweep hunting mistakes AND UX/UI enhancement opportunities over a named scope (unscoped = the whole user-facing surface). When "FFIV" appears in the request or an added step (any case), expand it into the four paired steps defined in [procedures.md](procedures.md), which also holds the standing finding rules. Find is tagged `[skill: /uxmaster]` for UI scopes; FFIV never displaces the standing finishers.

### On-demand procedures — status, flatify, close, roll (see procedures.md)

The full procedures for **status** (op 0.5), **flatify** (op 6), **close** (op 7), and **roll-forward** (op 8) live in [procedures.md](procedures.md). When one of those ops fires, read that file and follow its matching section exactly — the router summaries above are routing hints, not the procedure. Everything else in this file (Teamify, auto-classify, the plan-write steps) stays inline because it runs on ordinary plan creation.

### Teamify procedure (op 5 — full reclustering)

1. Read the target plan's Goal + full Steps + Validation lists.
2. For each step, identify what context executing it actually requires — which files, which system/service, which domain knowledge. This is the real unit of analysis, not the step's topic label.
3. Group steps by **shared context need**: any two steps that require the same files/system/domain knowledge go in the same team, regardless of how different they sound topically — splitting them would mean two agents both paying to load context one agent already has, for no gain. Steps only split into different teams when their context needs are genuinely independent (different files, different systems, nothing that has to be known by both) — that's the only case where parallel dispatch actually saves work instead of just adding coordination overhead. Up to roughly 10 teams; no target count, no fixed taxonomy (not "code vs UI vs other," not any preset pair). A 4-step plan might yield one team or four; a 30-step plan might yield six — whatever the actual context boundaries support. Don't default toward fewer teams for its own sake, and don't force extra splits the context doesn't support either.
4. For each group: name it (short kebab-case), write a one-line Focus, list its step numbers, infer real ordering dependencies on other teams (data must exist before code reads it, infra must exist before code deploys to it — not just numeric step order), and suggest an `Agent` (subagent_type) that best matches the Focus. If this group's steps reference an access dependency (SSH host, API key, gated URL — see "Access preflight scan" above) not shared by other teams, give it its own access-verification step as its first step, ahead of its other steps — don't rely on a global check, since teams may run against different machines/credentials entirely.
5. Steps that don't cleanly fit any group stay unassigned (omit from the table) rather than forcing a bad fit.
6. If the plan has no genuine independent context boundaries (e.g. every step touches the same file/system, or it's a strictly sequential dependency chain), don't write a Teams section at all — report "no independent context boundaries found — steps are sequential, staying flat" and leave `teamed` unset. This is a valid, expected outcome, not a failure.
7. Write `## Teams` into the plan file, set `teamed: true` (only when a Teams section was actually written), re-print the full plan.
8. In the presented output, add one line explaining the grouping rationale (what context boundary separates each team) and which teams can run in parallel (those with no `Depends on` entries pointing at an unfinished team).

This procedure runs two ways: explicitly via op 5 (teamify) on an existing plan, and **automatically** on every new plan (op 9, the first-plan path of op 10, and the fresh-Teamify pass of a roll-forward per op 8) as part of "Write the plan file" (Step 6) — teaming by default means this pass always runs on creation, not just on request.

### Auto-classify on add (part of op 10 — cheap, O(1) per step, never a full reorg)

When the current plan already has `teamed: true` and a new step is appended:

1. Compare the new step's content against each existing team's Focus. If it clearly matches exactly one team, append its number to that team's Steps list in the `## Teams` table — nothing else about the table changes (don't touch Depends on / Agent / other teams). If the new step introduces an access dependency (SSH host, API key, gated URL) that team's existing steps don't already have a verification step for, also insert one `[skill: /accounts]` step at the front of that team's steps — this is still a single, cheap classification (one dependency, one step), not a re-teamify.
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

A full worked oracle-merge example (buzzword match → folded Steps, Constraints, and audit-trail lines) is in [examples.md](examples.md).

#### When oracle has no matching entries

The plan is written without oracle augmentation. Note in the audit trail: "Oracle scanned, 0 matches. Buzzwords in plan: [list]. Use `/oracle add <buzzword>` to register one."

### 5. Access preflight scan (mandatory, every plan, every refinement)

**Why this exists:** a plan that references a remote machine, an SSH host, an API/access key, or a gated URL can look like it's making progress while actually just waiting on something nobody ever confirmed was reachable. (Real case: a plan drove work on an incus VM over `ssh cypressLinux` and sat "in progress" for a long stretch — the actual problem was nobody had ever confirmed the plan could check on that VM's build status from that host at all. It wasn't blocked, it was guessing.) This step exists so that check happens at planning time, as the plan's own first step, instead of being discovered mid-run.

This is **not opt-in** — run it on every plan write and every refinement, same as the oracle merge, over the full Goal + Steps + Validation + Constraints (including anything the oracle merge in Step 4 just added).

1. **Scan for access dependencies** — anything later steps need reachable that isn't already local/guaranteed: SSH hosts / remote machine names (`ssh <host>`, "on <host>", VM/container targets reached via a remote), API keys / access keys / tokens / credentials / secrets, gated URLs (dashboards, APIs requiring login), cloud accounts (AWS/GCP/Cloudflare/etc.), git remotes or GitHub/GitLab orgs+repos not already known-accessible, database connection targets.
2. **For each dependency found, identify the SPECIFIC capability later steps actually need** — not bare reachability. "Can ssh to cypressLinux" is not the requirement; "can ssh to cypressLinux AND run `incus` there AND read a target VM's build status" is. Bare-connectivity checks miss exactly the failure mode this step exists to catch.
3. **Dedupe** — one verification step per distinct target+capability, even if several later steps reference it.
4. **Write one verification Step+Validation pair per dependency, tagged `[skill: /accounts]`** (the `accounts` skill already owns SSH/credential/cloud-account access diagnosis and repair — don't reinvent it here):
   - Na: `Verify <capability> on <target>. [skill: /accounts]`
   - Nb: a real, runnable, END-TO-END probe of the exact operation later steps depend on — reuse the oracle's known-good command for that target if one exists (Step 4 may have just surfaced it), otherwise the most direct real command that exercises the actual capability (not `ssh host true` — the real status/read command later steps will actually run).
5. **Insert these steps at the very front of whichever scope depends on them** — ahead of every other step in that scope, since nothing that needs the access should run before it's confirmed:
   - Flat plan, or a dependency referenced by unassigned/global steps → front of the main `## Steps`/`## Validation` lists; renumber everything else after.
   - Teamed plan, dependency referenced only within one team's steps → that team's own first step (see "Teamify procedure" and "Auto-classify on add" below) — different teams may run against different machines/credentials, so a global check would be both wrong-scoped and wasted work for teams that don't need it.
6. **Add one `Access:`-prefixed Constraint per dependency** (same pattern as the existing `Timing:` prefix): `Access: <target> — <capability needed>, verified via step <N>`. This is the structured marker `/iterate` uses to recognize an access-check step and treat its failure differently from an ordinary failing validation (immediate `/accounts` self-heal attempt, immediate operator-wall report if that fails — no 5-cycle wait on something that simply doesn't exist yet).
7. **If nothing was found**, write nothing and note it in the audit trail (Step 6 below) — most plans touching only local files have no access dependencies. This is a normal, expected outcome, not a shortcut taken.

### 5.5. Human-gate detection (mandatory scan, same spirit as the access preflight)

Scan the plan for a **terminal human gate**: a step whose completion inherently requires human decisions or approval — a decision session, "review and approve", "Q&A with the owner", "final sign-off", recording product choices no agent may make. (Distinct from an operator *blocker* like a missing credential — a gate is designed-in, known at planning time.)

- **Found, and it's at the end** (nothing but reporting depends on it): mark it — `human-gate: <step N>` in the frontmatter, and `(human-gate)` suffix on the step line. **Scope every OTHER validation to be agent-completable**: no validation outside the gate step may quantify over the gate's outputs (the civet failure: V8 required zero unchecked boxes across ALL stage MANIFESTs, including the human-gated one — guaranteeing an unreachable green; it should have excluded stage-12's box, which the gate step itself owns). At execution time `/iterate` treats reaching the gate with everything else green as **success-with-handoff**: loop cancelled, user actually asked via a prompt (not just a markdown file), branch unmerged until the gate clears — see its Step 5.
- **Found mid-plan**: don't leave it inline where it would stall autonomous execution — restructure: move it to the end if ordering allows; otherwise split the plan at the gate (steps after it become a follow-on plan, noted in the footer) so each autonomous stretch ends at a gate rather than parking on one.
- **Rule 9's ban on STOP-steps is unchanged** — a human-gate is not a "halt if X" check; it's a real deliverable (prepare the agenda/materials, hold the session, record decisions) whose *completion* needs a human. Prepare-side work stays agent-owned; only the decision itself gates.
- Not found → nothing to write; most plans have no gate.

### 5.7. Skill tagging pass (mandatory, every step, at creation time)

The available-skills list is already in context (descriptions are always loaded) — this pass costs a lookup, not a load. For EVERY step, at the moment it's written:

1. Match the step's actual work against the available skills. If one applies, append the tag to the step line: `[skill: /dev-makefiles]` — or `[skill: /auditor → /importer-fix]` for read-then-write pairs. The tag is a binding instruction to `/iterate`, not a suggestion.
2. If no skill fits, tag `[skill: none — <two-word reason>]` (e.g. `[skill: none — plain edit]`). Explicit, so the user can see the gap rather than wonder.
3. Match on the work's NATURE, not its noun: a step that builds/compiles → `/dev-makefiles`; touches SSH/creds/remotes → `/accounts`; branch/PR work → `/feature-branch`; a domain skill's subject → that skill. When a step's wording is ambiguous, read the candidate skill's description once and decide — don't tag speculatively.
4. Applies equally to steps added later (op 4/10 adds, auto-classify time) — one cheap judgment per new step, same as team classification.

This exists because plans reliably bypassed installed skills: executors built software with ad-hoc shell instead of the Makefile skill, because nothing at planning time forced the lookup. The tag converts an always-loaded description into an explicit instruction at the moment of action.

### 5.9. Test-case detection (mandatory scan, every plan)

A prompt that states behavior — a trigger, a condition, an expected effect ("when hitting play, if mute-devices is selected, mute the device; on stop, unmute") — is a testable requirement, and the moment to capture it is while the user's own words are still in front of you.

1. **Scan the request + every step** for behavioral statements: something the user does, a state that gates it, an effect that must follow. Pure refactors, research, and config edits usually have none — that's a normal outcome.
2. **For each one found, invoke `/testmaster-derive`** with the requirement in the user's own words. It returns the implied cases — including the negative case, each distinct path to the same end state, and the restore-state/interrupted cases the prose left out — and records them in `./.claude/testmaster/catalog.json`.
3. **Fold the result into the plan**: list the derived case ids in the step's Validation (`Nb: ... and cases mute-1..mute-4 pass`), and add any `AMBIGUOUS` case as a `Decision:` Constraint resolving it (planner picks the most reasonable reading and logs it — never a question back to the user, per rule 11).
4. **The standing test finisher then has something concrete to verify** — see Step 5.8, whose validation requires these specific cases to exist and pass, not just "the suite is green".
5. Nothing found → note `test-case scan: no behavioral requirements` in the audit trail and move on.

### 5.8. Standing finisher tasks (mandatory, every plan — tests, then docs)

Every plan ends with the same three finisher steps, appended automatically at creation — the user does not ask for them each time:

0. **Makefile pass** — `Na: Maintain the project Makefile against this plan's work — add/update targets for anything this plan made buildable, runnable, or testable; remove targets whose subject this plan deleted; keep help output current. [skill: /dev-makefiles]` / `Nb: every repeatable dev task this plan introduced is reachable via a make target (run each new/changed target once, real invocation); no target references removed code; \`make help\` (or equivalent) lists them accurately.`
1. **Test pass** — `Na: Run the project test suite via TESTMASTER — fast+standard tiers only, all green; maintain coverage first for behavior this plan added/changed. [skill: /testmaster]` / `Nb: /testmaster run reports 0 failures across fast+standard; every feature this plan added or changed has a registered, executed test (new tests show runs ≥ 1 in ./.claude/testmaster/registry.json), and every case derived in Step 5.9 is present and passing — `/testmaster-catalog status` shows none of this plan's requirements unverified or drifted.` The slow tier is NEVER part of this step — it belongs to the nightly schedule.
2. **Docs pass** — `Na: Sync the end-user product documentation to the final state of this plan's work — add new features' operating instructions, update changed behavior, delete removed features' docs. [skill: /product-docs]` / `Nb: /product-docs reports docs synced (or "already true"); no doc section describes behavior absent from the final tree; new user-visible features each have an operating section.`

Placement: **Makefile → tests → docs** (targets must exist before TESTMASTER runs them; docs describe what survived the tests), and all three are the **last agent steps** — only a `human-gate` (Step 5.5) may follow; refinement adds always insert before them, and they keep their end position through every renumbering. All stay team-unassigned (coordinator-run after all teams merge — they sweep the whole plan's work) with provenance `Standing rule: end-of-plan finisher (tests then docs).` A project with genuinely no end-user product may drop the docs finisher (audit-trail note: `docs finisher skipped: no end-user product`).

#### Project launch gate — surface it when printing a plan

If `./.claude/iterate/policy.md` exists and sets `require-launch-keyword: <word>`, the plan's closing line must say how to launch it: `launch with /iterate <name> <word>` rather than a bare `/iterate <name>`. The planner does not enforce the gate — `/iterate` does — but handing someone a plan whose documented launch command is the one that will be refused is a paper cut this avoids. No policy file means print the bare form as usual.

#### The `skip` modifier — the user's explicit override

**`skip` anywhere in the request suppresses the finishers.** It is a modifier, not an operation: it parses in **any position** (`skip`, `/ip skip`, `plan the migration, skip`, `/ip skip the finishers and plan X`) and combines with every op that writes a plan. A bare `/ip skip` is not ambiguous — the request itself comes from conversation context per op 10, and `skip` only says what not to append.

Scope, narrowest match wins:

| Request contains | Suppressed |
|---|---|
| `skip makefile` / `skip the makefile pass` | the Makefile finisher |
| `skip tests` / `skip testmaster` / `skip the test pass` | the test finisher |
| `skip docs` / `skip product-docs` | the docs finisher |
| `skip` / `skip finishers` / `skip the canned steps` / `skip the pre-baked steps` | **all three** |

Two behaviours, depending on what the target plan already has:

1. **Creating or adding** — do not append the suppressed finishers at all.
2. **The target plan already carries them** — remove the suppressed finisher steps, their paired Validations, and their Provenance lines, renumbering all three lists 1:1 exactly as op 3 does. `skip` said late means the same thing as `skip` said early.

**Always record it.** Add one audit-trail line per suppressed finisher: `finisher skipped by explicit request: <makefile|tests|docs>`. A plan missing its finishers must be readable as a deliberate choice, not as a plan built by an older planner or one where the rule silently failed. The suppression applies to **that plan only** — it is not a standing setting, and the next plan gets all three back unless it also says `skip`.

**Do not argue the point.** `skip` is the user telling you the pre-baked steps are not wanted this time. Suppress them, log the audit line, and carry on — no warning about untested work, no offer to add them back, no asking whether they are sure.

### 6. Write the plan file

**On a brand-new plan** (op 9, or the first-plan path of op 10) — after Steps 1-5 above have produced the full Goal/Steps/Validation/Constraints — do two things before writing:

1. **Name the feature branch, do not create it** (git repos only — see "One plan = one feature branch" above): compute `feature/<plan-name>-<short-goal-slug>` and record it in the frontmatter as `branch:` plus `branch-created: false`. Do not run `/feature-branch start`, do not check anything out — `/iterate` creates the branch at execution start. Not a git repo → skip, no `branch:` field, one audit-trail note. (Roll-forward plans inherit their source's branch, which by then usually exists — carry `branch-created:` forward unchanged.)
2. Run the **Teamify procedure** (see below) automatically, unless `$1` also carried a flat trigger phrase (see "Flat trigger phrases" above), in which case skip straight to writing flat. This is what makes teaming the default: every new plan gets a real attempt at clustering, and either ends up with a `## Teams` table or a legitimate "no independent context boundaries — staying flat" outcome (both are normal, neither needs the user to ask).

**Refining an existing plan triggers neither** — no new branch (the plan already has one, or deliberately has none), and no auto-teamify: a flat plan being added to stays flat (use the explicit teamify trigger if it's grown enough to warrant teams); a teamed plan being added to uses the existing cheap Auto-classify-on-add, never a full re-cluster.

Schema, written to `./.claude/iterate/plans/<name>.md`:

```markdown
# Iterate Task — <short title>

name: <animal>
Started: <UTC timestamp> (planned)
CWD: <pwd>
phase: planned
running: false
planner: iterate-planner    # marker so /iterate knows oracle was consulted
planner-version: <version>  # this skill's own frontmatter `version:` at write time — update on every refinement this skill makes
teamed: false               # set true only after a teamify pass writes ## Teams
branch: feature/<name>-<slug>  # the plan's feature branch (omit when not a git repo); created via /feature-branch at plan creation, merged+deleted by /iterate only on all-green
human-gate: <step N>           # only when Step 5.5 found a terminal human-decision step; /iterate ends in success-with-handoff there, not "blocked"

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
- Access: <target> — <capability needed>, verified via step <N> (one per access dependency found)

## Teams
<!-- Only present when teamed: true. See "Teams" section above for schema. Omit entirely on flat plans. -->

## Provenance
<!-- One line per step, numbered 1:1 with Steps, written AT THE MOMENT the step is created: WHY does
     this step exist? Three source kinds, each naming its origin specifically:
       15. You asked for a redundant transcript_<serial>.txt beside each recording, even though it's redundant.
       13. You reported a bug: text in the description field can't be copied.
       17. Oracle: "app-build compile" entry — its How mandates the smoke-run after any build change.
       18. Inferred: validation for step 3 (you gave no explicit check; planner chose `make test`).
     SYNTHESIZE, don't quote: the user often rambles through context, tangents, and self-corrections
     before landing on the ask — boil each ask down to its simplest one-sentence form, keeping only
     the distinctive words that anchor it to their message (a filename, a feature name, "even though
     it's redundant"). The test: reading the line, the user instantly recognizes "yes, that's what I
     meant" — without re-reading their own paragraph. One ask can spawn several steps (same provenance
     line on each); one step can merge several asks (join them: "You asked for X and, separately, Y").
     Renumber with Steps on remove-from. Never backfill from memory later; if the origin is genuinely
     unknown (legacy plan), write "unrecorded (pre-provenance plan)". -->

## Changelog draft
<!-- Empty at planning time. /iterate appends one line per real change at step check-off / team merge,
     then distills into CHANGELOG.md + RELEASES.md at completion — see /iterate's "Changelog" section. -->

## Access preflight
<!-- Audit trail from Step 5. One line per dependency found + which step verifies it, or "No external access dependencies detected." -->

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

### 7. Present the plan

Always lead with a one-line **save confirmation** so the user knows it persisted:
- created a brand-new plan → `plan written to <animal>`
- appended to / refined an existing plan → `plan amended <animal>`

Then print the plan in paired 1a/1b format, AND show which oracle rules were applied:

```
plan written to owl

**Plan ready** — owl — ./.claude/iterate/plans/owl.md (phase: planned, oracle-aware)
**Branch:** `feature/owl-<slug>` (created + checked out — merges to <default> automatically when the plan finishes all green)

**Goal:** <goal>

1-prompted: You asked for <paraphrase of the user's ask that created this step>.
1a. <step 1>
1b. <validation 1>

2-prompted: You reported a bug: <paraphrase>.
2a. <step 2>
2b. <validation 2>

...

(The `N-prompted:` line prints the step's Provenance entry — the user's own ask in their own key words, or `Oracle: <entry>` / `Inferred: <what the planner chose and why>` for steps the user didn't directly request. This is how the user audits, at a glance, that each step traces to something they actually said — and spots steps they never asked for.)

**Constraints:** <list, if any>

**Access preflight:**
- <target> → verified via step <N> (<capability>) [skill: /accounts]
- ...

(No external access dependencies detected.) <!-- use this line instead when the scan found nothing -->

**testmaster:**
```
  -current total test cases: <x>
  -plan adds: <n>
  -new total: <z>
  -plan could effect: <y> (will FFIV)
```
(Numbers come from `/testmaster-catalog impact <plan>` — never estimated here. `plan could effect` = EXISTING cases whose `covers` intersect this plan's files, i.e. what this plan puts into drift. **Nonzero always renders `(will FFIV)` and the plan must carry the sweep step** — there is no "will not FFIV" branch. Zero renders `(nothing to FFIV)`; an empty or unadopted catalog renders `unknown (no covers recorded — run /testmaster-adopt)`, never a bare `0`. Rendering contract, the FFIV step template, and the prediction-vs-derived rule: [procedures.md](procedures.md#the-testmaster-block).)

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

### 8. Handle refinements

If the user responds with changes in natural conversation, update the current plan file in place AND re-run the oracle merge (in case the refinement brought new oracle-relevant scope) AND the access preflight scan (in case the refinement introduced a new remote host, key, or gated URL). Re-print the full plan. Keep `phase: planned`. Don't archive — overwrite. Refinements target the **current** plan unless the user names a different one — never spin up a second plan for a refinement.

### 9. Rapid-fire terse mode

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
16. **Teaming is the default on every new plan; flat is the opt-out, not the other way around.** Run the Teamify procedure automatically when a brand-new plan is written (op 9, or the first-plan path of op 10), unless the request explicitly asked to stay flat. A plan legitimately ending up flat because Teamify found no independent context boundaries is still a normal outcome — don't force teams onto a plan that has none. **Refining an existing plan never auto-teamifies** — that stays manual (the teamify trigger phrases) so rapid-fire adds to a flat plan don't get re-clustered out from under the user.
17. **Auto-classify on add is a single judgment call, never a re-run of teamify.** Once `teamed: true`, slot each newly appended step into the best-fit existing team in one cheap decision, or leave it unassigned. Don't re-cluster the whole plan on every add — that's what makes rapid-fire adds stay fast.
18. **Never invalidate team membership except through teamify, flatify, or remove-from.** Refining Steps/Validation/Constraints text must not silently drop a step's team assignment. If `remove-from` deletes a step that belonged to a team, remove its number from that team's row too (renumbering the rest) — don't leave a stale reference to a step that no longer exists. Flatify is the one operation that's SUPPOSED to drop every step's team assignment at once — that's its entire job, not a bug.
19. **Rapid-fire streaks get a one-line reply, not a full reprint.** See "Rapid-fire terse mode" above — this exists because the user queues several adds in a row without reading each one; a full plan dump on every single one is slow and clutters the conversation. Always show the full plan on the first invocation of a streak and whenever there's genuine doubt about whether the streak ended.
20. **No fixed team count or taxonomy — split on independent context needs, not topic labels.** Two steps stay in the same team whenever they need the same files/system/domain knowledge, even if they sound topically different — splitting those gains nothing and costs two agents paying to load the same context. Two steps only go in different teams when their context needs are genuinely independent — that's the only case parallel dispatch actually saves work. Never default to a binary split ("UI vs other", "code vs everything else") and never bias toward the fewest possible groups just because fewer is simpler to write; also never bias toward the most possible groups if steps genuinely share context. Up to roughly 10 teams, discovered from actual context boundaries in the plan, not chosen from habit.
21. **Every plan gets an access preflight pass, every time — not opt-in, not something the user has to ask for.** Scan for SSH hosts, remote machines, API/access keys, credentials, and gated URLs; write a verification step, tagged `[skill: /accounts]`, at the very front of whichever scope (global, or a specific team) actually depends on it — before any step that needs that access. The check must exercise the SPECIFIC capability later steps depend on (e.g. "can read the remote build's status"), not bare reachability — a plan that assumes access works and finds out mid-run has already wasted the time this step exists to save. See "Access preflight scan" above.
22. **Flatify never refuses on an executing plan without saying why, and never refuses on an already-flat one either.** Same phase:executing guard as delete (removing Teams out from under a live dispatch would orphan tracked status) — refuse and say so. On an already-flat plan, `flatify` is a no-op, not an error: report "already flat" and stop. Never touch the underlying `## Steps`/`## Validation` content — flatify only ever removes the `## Teams` section and flips `teamed: false`.
23. **One plan = one feature branch, managed only through `/feature-branch`.** Every new plan in a git repo gets `feature/<name>-<slug>` created at plan-creation time and recorded as `branch:` in the frontmatter — before the plan file is written, so nothing lands on main. Roll-forward plans are the one exception: they inherit their source plan's branch verbatim and never create a new one. Not a git repo → skip silently, no `branch:` field.
24. **Close and roll-forward NEVER merge, and ALWAYS say the merge hasn't happened.** The ⚠-line naming the unmerged branch is mandatory in both reports — the user must never discover later that archived work silently isn't on main. The only paths that merge a plan's branch are `/iterate`'s all-green completion and an explicit user order ("merge it", "close it and merge") — nothing implicit, ever.
25. **A plan ending in human decisions gets a `human-gate` marker, and no validation outside the gate step may depend on the gate's outputs.** Detect it at planning time (Step 5.5) — never let "requires a human session" reach execution disguised as an ordinary validation, where it can only ever read as an unresolvable red check. The gate's prepare-work (agendas, evidence, briefing material) stays agent-owned; only the decision itself gates.
26. **Every step records its provenance at creation time — the user's ask, SYNTHESIZED to its simplest form.** One `## Provenance` line per step, written the moment the step is written (never reconstructed later): the user's ask boiled down to one plain sentence — distill a rambling multi-paragraph description to the essential ask, preserving only the few distinctive words that make it recognizably theirs — or `Oracle: <entry>` for oracle-merged steps, or `Inferred: <what + why>` for planner-invented steps (access preflight probes, inferred validations). Presentation prints it as `N-prompted:` above each Na/Nb pair. This is the user's audit line: every step must trace to something — and a step whose provenance you can't state is a step you shouldn't be adding. Renumbers with Steps on remove-from; carried verbatim on roll-forward.
27. **Every step carries a skill tag — `[skill: /x]` or an explicit `[skill: none — why]`.** Written at step creation from the always-loaded skills list (Step 5.7), never left implicit. A prose-only step that silently implies tool usage is the failure mode this kills: the executor must see, inline, which installed skill governs the work. Tags renumber/carry exactly like provenance lines.

27.5. **Every plan carries the three standing finishers — dev-makefiles, then TESTMASTER, then product-docs — as its last agent steps.** Appended automatically at creation (Step 5.8), never waiting for the user to ask: a `/dev-makefiles` maintenance pass (targets for everything the plan made buildable/runnable/testable; dead targets removed), a fast+standard `/testmaster` pass, then a `/product-docs` sync. Refinement adds insert before them; only a human-gate follows them; the slow test tier never runs mid-plan. The docs finisher may be dropped for projects with no end-user product, and **any or all three are suppressed when the request carries the `skip` modifier** (Step 5.8) — `skip` parses in any position, scopes to `makefile`/`tests`/`docs` or all three, removes them from a plan that already has them, and requires one `finisher skipped by explicit request: <which>` audit line each. Absent `skip`, all three are automatic and never wait for the user to ask.
27.6. **Every plan is scanned for testable requirements, and the user's own words become the test cases.** Step 5.9 runs on every plan: behavioral statements go to `/testmaster-derive`, whose derived cases (negative, every-path, restore-state, interrupted) land in the plan's validations and in the catalog. A stated behavior that ships with no case for it is the gap this closes — and an `AMBIGUOUS` case is resolved by the planner as a logged `Decision:` constraint, never bounced back as a question.
27.7. **A nonzero `could effect` always FFIVs.** If `/testmaster-catalog impact` says this plan can put existing cases into drift, the plan carries the step that sweeps them (Step 7's testmaster block) — never merely reports the number. The sweep targets the *derived* drifted set after execution, not the predicted count. An `unknown` count (unadopted project, no `covers` recorded) is treated as nonzero: it means the blast radius is unmeasured, not empty, and the plan gets a `/testmaster-adopt` step before the sweep.
## Examples

Two full worked examples (a plain restate-the-plan run with oracle merge, and a teamify + rapid-fire-adds streak) live in [examples.md](examples.md) — load that file when you need to see the exact output shape end to end, e.g. building a similar plan-writing skill from this one as a template, or checking an edge case in the operation router / auto-classify / rapid-fire terse-mode logic against a concrete run.

## `version`

`version` (or "what version") on **any** iterate skill reports the same thing —
the family version, because the stack is versioned as one unit:

```
iterate family 5.0.0
iterate-run iterate-v3.3 (commit 4dd09ec5, built 2026-08-27_17:02:20)
```

Run `iterate-run version` for the second line — a real installed binary, never a
recalled string. If members disagree, say so and name them: drift inside the
family is a defect, not a state, and `skillctl family iterate set X.Y.Z` is the
only correct way to bump.
