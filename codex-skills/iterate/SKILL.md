---
name: iterate
description: Use when given a multi-step task with validation criteria and asked to execute autonomously until done. The skill does NOT ask the user clarifying questions mid-run; it picks the most reasonable interpretation, executes, validates, loops, solves its own blockers, and only returns control when validation passes or the run is truly stuck. When the plan is teamed (see $iterate-planner's teamify), dispatches one subagent per independent team to run concurrently instead of working the Steps list serially. Runs on the plan's own feature branch (via the feature-branch skill) and, on all-green completion, automatically opens the PR, merges to the default branch, and deletes the branch; any other ending leaves the branch unmerged and says so. Re-invokable — running `$iterate` again resumes from the saved state file. Triggers on "$iterate", "iterate until done", "keep going until X", "work this until validation passes".
---

<!-- version: shared across the family; see the **Version:** line above. -->

# $iterate — Run a task to completion without interrupting the user

**Version:** iterate family 5.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


The user invoked this skill because they're tired of being interrupted by clarifying questions. The whole point: **make the call yourself, log it, continue**. Only return control when the validation criteria they provided are all green, or when you've truly exhausted reasonable attempts.

## Usage

The argument is a paragraph describing the work to do AND how to validate success — or a saved plan's animal name to execute/resume that plan, or nothing at all to resume the executing plan (or pick among planned ones). `$1` is the first word of that argument; `$ARGUMENTS` is the whole thing.

<!-- codex-port: Claude Code's `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose and never left to the token alone. -->

## Dependencies

This skill invokes other skills with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate-planner` — ported alongside this one.
- `$feature-branch` — **not yet ported** — run `$skill-2-codex` on it.
- `$accounts` — **not yet ported** — run `$skill-2-codex` on it.
- `$oracle` — **not yet ported** — run `$skill-2-codex` on it.
- `$dev-makefiles` — **not yet ported** — run `$skill-2-codex` on it.

Project state stays at `./.claude/iterate/` — **not** `./.agents/`. That path is read and written by the `iterate-run` binary (dashboard, plan-name registry, run wrapper), which is harness-agnostic; moving it would split state between harnesses and blind the dashboard.


## Named plans & state files (this is how resumption works)

Plans are **saved, animal-named, and persistent**. Each plan is one file:

    ./.claude/iterate/plans/<name>.md        (project-local, relative to cwd)

`<name>` is a common animal (dog, cat, fox, owl, elk, wren, …), assigned via `iterate-run name next` — see $iterate-planner's "Named plans" for why (this project's own alphabetical sequence, a/b/c/…, drawing from one machine-wide "already used" set so two unrelated projects never land on the same codename). If `iterate-run` isn't installed, fall back to any common animal not already present in this project's own `plans/` and note that the global registry was unavailable. `./.claude/iterate/current` points at the **current** plan. **The plan file** below always means the plan being executed (resolved from `$1`'s name, or `current`, or the sole executing/only plan).

Each plan has a top-level `phase:` field:

- `phase: planned` — written by `$iterate-planner`, waiting for execution to start.
- `phase: executing` — execution is in flight (or has been; check `running:` for whether a run is currently live).

Two separate timestamps, don't conflate them: `Started:` is when the plan was **drafted** (set once by `$iterate-planner` or on a direct fresh-task run, kept forever). `Executing:` is when execution **actually began** (set once, by `$iterate`, at the exact moment `phase` first flips to `executing` — see the transition points below — and never touched again, including on resume). A plan drafted well ahead of when it's actually run needs both: `Started:` answers "when was this planned", `Executing:` answers "how long has this really been running" (what the dashboard's "Running for" box uses).

**Legacy migration:** see $iterate-planner's "Named plans" section for the one-time `active.md` → `plans/<name>.md` migration procedure — identical here, do it silently on first touch rather than maintaining two copies of the same steps.

### Entry decisions on `$iterate`

Resolve in this order:

0. **`$1` is exactly "version"** (or "what version", "iterate version"): run `iterate-run version` and print its output verbatim — real installed binary, not a memory recall, works from any directory. If not found, report "iterate-run isn't installed — run `make install` in claudecodetricks." Then **stop**, no plan involved.
0.5. **Re-entry guard — a terminal plan does not resume on a no-arg tick.** If `$1` is empty and the resolved executing plan has `status: blocked-on-operator` (or `status: awaiting-human-gate`) already set, AND nothing material has changed since it was written (no new user message addressing the blocker, no change to the gate's input files): this tick has no job. **First check whether the loop is somehow still armed** (`loop-mechanism:` non-empty in the plan file) — it shouldn't be, the terminal path cancels it, but if it is (a prior run died before canceling, or canceled the wrong mechanism), cancel it NOW with verification, log one line `re-entry guard: killed leftover <mechanism>`, and exit. Otherwise **exit immediately and silently** — no status re-verification, no environment audits, no "handoff document" polishing, no re-asserting the wall. Confirmed live (civet): 13+ hours of once-a-minute ticks against an already-blocked plan, each manufacturing self-generated audit work because nothing told a resuming tick that "already terminal, nothing changed" means *stop*, not *find something to do*. A user-typed `$iterate <name>` (non-empty `$1`) bypasses this guard — an explicit human invocation IS a material change, so re-check the blocker for real then.
1. **A plan is already `phase: executing`** (scan `plans/`): resume THAT plan from its "Status / Log", honoring the concurrency lock. This takes precedence over everything below — it's what makes the cron re-fires (which pass no argument) continue the live run instead of prompting. If several are somehow executing, pick the one named by `current`, else the most-recently-heartbeated.
1.5. **Project launch gate (only if this project set one).** Read `./.claude/iterate/policy.md` if it exists (see "Project policy" below). If it sets `require-launch-keyword: <word>` and that word is **not** present anywhere in the argument, do NOT launch: print the policy's stated reason plus `re-run as \`$iterate <plan> <word>\`` and **stop**. Nothing is transitioned, no lock taken, no cron armed.

   **This rule sits below rule 1 on purpose, and that placement is the whole design.** Rule 1 (resume an already-executing plan) has already returned by the time you get here, so a cron resumption tick — which passes no argument and therefore could never carry the keyword — is structurally incapable of tripping this gate. A gate that blocked resumption would kill every run on its first tick, one minute after a correctly-authorized launch. Only the fresh-launch paths (rules 2–5) are gated.

   The keyword parses in **any position** — `$iterate owl permission`, `$iterate permission owl`, `$iterate permission` — and is stripped from the argument before the remaining rules read it, so it is never mistaken for a plan name or for task text. Most projects have no policy file; absent one, this rule is a silent no-op.

1.6. **Launch schedule (only if this project set one).** If `policy.md` sets `launch-schedule:` (or the `launch-window:` shorthand), get the real local date and time — run `date +'%F %H:%M %a'`, never estimate it from context — and evaluate. Outside the permitted set, refuse a fresh launch: print the policy's reason, the rule that decided it, the current time, and when the next window opens; then **stop**.

   Evaluation order: any matching `deny` refuses; else any matching `allow` permits; else, if allow rules exist and none match, refuse; no schedule at all permits. Deny beats allow, and any allow rule makes the schedule default-deny — so editing policy can only narrow when runs happen, never accidentally widen it. `$iterate-rules` owns the full grammar and writes this file; read it there rather than inferring the syntax.

   **The window wraps midnight, and that is the normal case.** `22:00-06:00` means 22:00 through 05:59 the next morning. Compare by wrapping when `start > end`, never with a plain `start <= now <= end` — a naive comparison refuses 02:00 as "before 22:00", which is exactly backwards for the overnight block these windows exist to describe.

   **A day label matches the day the window STARTED, not the current instant.** `allow mon-fri 22:00-06:00` includes Saturday 02:00, because that window opened Friday 22:00 — which is what "weeknights" means to a person. Matching the instant would silently truncate every such window at midnight.

   **The launch keyword overrides the schedule.** If `require-launch-keyword` is set and the user supplied it, launch regardless of the window — and say so in one line: `outside the launch window (<rule>); proceeding on explicit "<word>"`. Never refuse a launch the user explicitly authorized.

   This is not a loophole, it is what the two gates actually mean. Both exist to answer one question — *is the machine free to spend?* The schedule is a **guess** at that, from hours when the user is usually away. The keyword is the **answer**, from the person who knows. When the guess and the answer disagree, the answer wins; refusing someone who just told you they are leaving for the afternoon is the gate mistaking its proxy for its purpose.

   The schedule still gates hard when no keyword was required or supplied — that is the case where nobody has said anything and the clock is the only evidence there is. A project that genuinely wants the hours enforced even against an explicit keyword sets `launch-window-strict: true`, and then the refusal stands.

   Same placement logic as 1.5: this is below rule 1, so a run legitimately started at 23:00 keeps resuming at 07:00. Gating resumption on the window would kill every overnight run at dawn, which is the one thing an overnight window is for.

2. **`$1` names an existing plan** (`$1` exactly matches a `plans/<name>.md`, e.g. `$iterate dog`): set `current` = that plan; if `phase: planned` → transition to `phase: executing`, **set `Executing: <UTC timestamp now>`**, set up the auto-resume loop, begin; if already executing → resume it (leave `Executing:` untouched).
3. **`$1` is substantive task text** (a paragraph/steps, not a bare existing name): create a **new** plan, named via `iterate-run name next`, with `phase: executing`, **`Executing:` set to the same UTC timestamp as `Started:`**, set `current`, set up the auto-resume loop, and begin. (This is the direct fresh-task path.)
4. **`$1` empty, exactly one plan exists** with `phase: planned`: transition it to `phase: executing`, **set `Executing: <UTC timestamp now>`**, set `current`, set up the loop, begin.
5. **`$1` empty, multiple planned plans exist**: ask the user which one, presenting a plainly numbered list — one line per plan, `<n>. <name> — started <date> — <goal>` — and wait for them to pick a number. <!-- codex-port: no confirmed structured-picker equivalent in Codex; rendered as a numbered prose list; verify manually --> Then execute the chosen plan (transition to executing, **set `Executing:`**, set current, loop, begin). This is the ONLY place `$iterate` asks a question, and it only happens on a human-typed no-arg `$iterate` with no executing plan.
6. **Neither `$1` nor any plan exists**: report "no plans yet — supply instructions or run $iterate-planner first" and stop. (Reporting is not the same as asking.)

Create `./.claude/iterate/` and `./.claude/iterate/plans/` if they don't exist.

### Project policy (`./.claude/iterate/policy.md`)

Project-scoped knowledge for the iterate stack — things true of THIS project that no plan should have to restate. Optional; most projects have none, and its absence is never a warning.

```markdown
---
require-launch-keyword: permission
launch-window: 22:00-06:00
---

# Iterate policy — <project>

## Why the launch gate exists

<Free text. The refusal in entry rule 1.5 quotes this verbatim, so write it as
the sentence you want to read when you are told no.>
```

Keys `$iterate` acts on today:

| Key | Effect |
|---|---|
| `require-launch-keyword: <word>` | a fresh launch must carry `<word>` anywhere in its argument |
| `launch-window: HH:MM-HH:MM` | shorthand for a single `allow daily <window>`; wraps midnight when start > end |
| `launch-schedule:` | list of `<allow\|deny> [days] [HH:MM-HH:MM] [dates]` rules — the full grammar, owned by `$iterate-rules` |
| `launch-window-strict: true` | the schedule refuses even when the launch keyword was given; off by default, because the keyword is the user stating the very thing the schedule is guessing at |

Both exist for projects where a run is expensive enough that starting one casually is the mistake. Set either, both, or neither.

**Adding a key is a skill change, not a config change.** A key this table does not list is silently ignored, so writing an invented key into `policy.md` produces a file that looks like a rule and enforces nothing.

Rules:

- **The gate is per-project, never global.** It lives in the project's own tree and applies only there. Never infer a gate for a project that has no policy file, and never carry one project's gate to another.
- **State the reason, don't invent one.** The refusal quotes the file. If the file gives no reason, say only that this project requires the keyword.
- **Refusing is not asking.** Print the refusal and stop — no picker, no "shall I proceed anyway?", no offer to bypass. The keyword IS the authorization; without it there is nothing to decide.
- **Read the clock, never estimate it.** Any time comparison runs `date +%H:%M` for real. Context timestamps go stale within a session and a stale clock silently inverts the gate.
- **An explicit keyword beats the clock.** The schedule is a proxy for "the user is away"; the keyword is the user saying it directly. Report the override, never refuse over it, unless `launch-window-strict: true`.
- **One keyword authorizes one launch.** It is consumed by the launch it appears in, not remembered. This is deliberately unlike standing risk acceptance: the gate exists precisely because each run is expensive, so each run is authorized on its own.

## Auto-resume — Codex cannot arm this itself

<!-- codex-port: Claude Code arms its own resumption with /loop or CronCreate.
     Codex has NO in-session scheduling tool -- Cron*/ScheduleWakeup are Claude
     Code's, and an open feature request for Codex (openai/codex#25466). An
     earlier port instructed Codex to call CronCreate and it answered "this
     session doesn't expose them". Rewritten to ask the user instead. -->

API errors, transient stalls, or a killed session will silently end a turn. (A context *compaction* is different — the turn continues; see "Surviving an auto-compact" below.) Under Claude Code the skill survives this by arming its own recurring trigger. **Codex has no in-session equivalent** — there is no tool this skill can call to schedule its own re-firing. Do not attempt `CronCreate`, `CronList`, `CronDelete` or `ScheduleWakeup`; they are not available and inventing a call wastes a turn and misleads the user.

So the resumption trigger is **the user's to create, once, outside the session**, and this skill's job is to say so clearly:

- **On a fresh task or first execution of a `phase: planned` plan**, print exactly one line naming what is needed and continue working: `auto-resume not armed — Codex cannot schedule itself. To survive a stalled turn, create a Codex Automation for "$iterate" (ChatGPT desktop/web → Scheduled) or an OS cron job running \`codex exec\`, firing at most every minute.` Record `loop-mechanism: user-managed` in the plan frontmatter so a later turn knows nothing is armed and does not try to cancel a job it never created.
- **Without a trigger, a stalled turn simply stops.** That is the honest state. The plan file holds everything needed, so a human typing `$iterate <name>` resumes exactly where it left off — nothing is lost, it just waits for a person instead of a clock.
- **On every terminal exit**, there is nothing to cancel. Note `loop-mechanism:` was user-managed and say whether the user's Automation, if any, should be paused now that the plan is finished — they own it, so they must be told it is still firing.
- **When the trigger fires and a run is already in progress**: see the lock section below — the second run exits immediately.

If a future Codex release ships in-session scheduling, this section is the one to rewrite; check `references/codex-format.md` first.

## Feature branch (one plan = one branch, merge only on all-green)

Every plan in a git repo runs on its own feature branch — `branch: feature/<name>-<slug>` in the plan's frontmatter, normally created by `$iterate-planner` at plan-creation time. All branch operations go through the **`feature-branch` skill** (`[skill: $feature-branch]`), never hand-rolled git.

**At execution start** (any transition to `phase: executing`, and on every resume entry):
- Plan has `branch:` → ensure it's checked out (`git rev-parse --abbrev-ref HEAD`; if not on it, check it out) before any step runs. All work — coordinator steps AND dispatched teams, which share the same working tree — happens on this branch. Teams never switch branches.
- Plan has no `branch:` but this IS a git repo (direct `$iterate <task>` fresh runs, pre-branch-era plans): **a plan has exactly one branch for its whole life, so derive the name once and never again.**

  1. **Look before creating.** `git branch --list 'feature/<plan-name>-*'`. If any branch already exists for this plan, **adopt it** — write it into `branch:` and check it out. Never create a second one because you cannot remember the first.
  2. **Write `branch:` to the plan file BEFORE creating the branch**, not after. The name must be on disk before anything can interrupt — a compaction, an API error, a killed session — because the only way a second branch gets created is the name being derivable but not recorded.
  3. Then `$feature-branch start feature <plan-name>-<slug>`. If the default branch has uncommitted work, let `$feature-branch start` carry it over.

  **Never re-derive the slug.** It comes from the goal, and the goal reads differently from inside step 2 than from inside step 9 — that is how one plan ends up with `feature/pigeon-cloud-setup-bug-monitor`, `feature/pigeon-portal-service-control` and `feature/pigeon-desktop-template-adapter`, three branches and three MRs for work that was always one unit. Confirmed live. If `branch:` is set, it is the answer; there is no second opinion to seek.
- Not a git repo → no branch anything; log `not a git repo — no feature branch` once and proceed. Every branch-related instruction in this file is a silent no-op for such plans.

**On all-green completion — the merge flow** (runs in Step 5's success path, BEFORE archiving):
1. **Commit the work yourself — the hook will not have.** The auto-commit hook snapshots to `refs/snapshots/` and never moves HEAD, so nothing it does is on the branch: any uncommitted work is still uncommitted. Stage and commit it with a real message describing what the plan actually did (the reasoning belongs in history, not only in the run log). `snapshots.py list` in the project shows anything a dead session left behind.
2. `$feature-branch finish` — pushes the branch and opens the PR/MR.
3. **Merge it.** An all-green plan is standing approval to land: `gh pr merge --squash --delete-branch` (or `glab mr merge` + branch deletion on GitLab). Then confirm you're back on the default branch, pulled, with the feature branch gone local + remote.
4. Report the merge in the success summary: PR URL, merge commit, branch deleted.
5. **If the merge itself fails** (conflicts with a moved main, required CI checks, protected-branch rules): the PLAN is still complete — don't un-archive, don't count it as a failed validation, don't retry-loop the merge. Report success WITH a plainly-flagged exception: "⚠ plan complete but NOT merged — PR <url> open, blocked by <reason>; branch `<branch>` preserved." Resolving that is the user's call.

## Changelog (drafted while working, published once at the end)

Every plan produces two changelog entries in the project root — the git-worthy distillation of the run (plan files themselves stay local):

- **`CHANGELOG.md`** — technical, [Keep a Changelog](https://keepachangelog.com) convention: prepend `## [<plan-name>] - YYYY-MM-DD` with `### Added` / `### Changed` / `### Fixed` / `### Removed` / `### Security` subsections, one terse line per change, PR/issue refs allowed. Developer audience.
- **`RELEASES.md`** — product-level, marketing style: prepend `## YYYY-MM-DD — <plain-English title>` with **New** / **Improved** / **Fixed** labels. Written for someone who uses the product and has never seen the code: benefit-first ("You can now…"), no jargon, no file paths, no superlatives. Refactors, tests, tooling, and internal plumbing are omitted entirely — if nothing user-visible shipped, write the one honest line "Internal improvements only" rather than dressing plumbing up as features.

**Drafting (during execution — coordinator only, never teams):** every time you check off a step or merge a team's `TEAM DONE`, append one line to the plan file's `## Changelog draft` section while context is hot: `- [added|changed|fixed|removed|security|internal] <what changed, product-level phrasing> (step N)`. One line per real change — skip steps that produced no change (pure verification, research recorded elsewhere). This is cheap, crash-safe raw material, not published prose.

**Publishing (once, in Step 5's success path, BEFORE `$feature-branch finish`) — a real final sweep, not a copy-paste of the draft.** The draft is a mid-flight journal: a feature that took three attempts, got reworked, or was partially rolled back has three-plus lines describing states that no longer exist. Two mandatory passes over it:

1. **Consolidate — describe what LANDED, not the journey.** Group draft lines by the actual feature/fix they belong to (not by step number — one feature often spans several steps and re-attempts). Each group becomes ONE line describing the final, as-merged state. Lines describing superseded intermediate states, reverted work, or fix-of-my-own-earlier-line are folded away entirely — the reader gets the net change, never the churn. (Rewrites that ship nothing net = no line.)
2. **Validate — every published claim must be true of the final tree.** For each consolidated line, confirm it's backed by a green validation (or directly observable in the final state: file exists, endpoint responds, behavior demonstrated). A draft line whose feature was later cut, disabled, or left broken does NOT publish — drafts record intent at check-off time; only the sweep confirms survival. When in doubt, re-check the thing itself, not the draft.

Then format: drop `internal`-tagged lines from `RELEASES.md` (they stay in `CHANGELOG.md` under Changed), create either file with a standard header if missing, prepend, never rewrite old entries. The git-committer hook (or a manual commit) lands them on the plan's branch so they merge with the work in the same PR — that's the point: the changelog IS the reviewable summary of the branch, so a wrong claim in it is a review-poisoning bug, same severity as a false validation.

**On not-all-green endings** (close, roll — see `$iterate-planner`'s ops): the draft section is the honest partial record — close publishes it flagged as partial; roll carries it forward to the successor plan (same branch, so the eventual merge publishes the whole accumulated story). Blocked/stuck plans just keep their draft in the plan file until resumed.

**Never merge an incomplete plan.** Blocked, stuck, closed-by-order, rolled-forward — in every not-all-green ending the branch stays unmerged, and **every such report must say so explicitly** (the ⚠-line naming the branch and the open/unopened PR). The user may then: fix the gaps interactively and `$iterate` again (normal resume — merge happens when it finally goes all-green); order "close the plan" or "roll it to a new plan" (route those to `$iterate-planner`'s close/roll ops — roll keeps the SAME branch); or explicitly order a merge anyway ("merge it", "merge what we have") — an explicit order is the one thing that overrides the all-green requirement: run the merge flow above and continue with whatever else they asked.

## Concurrency lock (don't double-run)

The state file has a `running` field:

```
running: <UTC ISO timestamp>   # set when an iterate run begins
running: false                  # set when the run ends cleanly
```

Heartbeat: update the `running:` timestamp at every step boundary and at every validation check. This is your "I am alive" signal.

On entry, **before doing anything else**:

1. Read `running:` from state.
2. If `running:` is a timestamp newer than 90 seconds ago → another iterate run is in progress. **Exit immediately and silently** (no log line, no status report). The other run will continue.
3. If `running:` is a timestamp older than 90 seconds → previous run died (API error, killed session, etc.). Treat as stale; take the lock, log "resumed after stale lock from <timestamp>", continue.
4. If `running: false` → take the lock (set timestamp), continue.

On every exit path (success, giveup, blocked-on-operator, human-gate): set `running: false` and write before returning control. Under Codex those four are the only exits — there is no voluntary mid-run stop.

## Team dispatch (parallel execution on teamed plans)

Most plans are flat — no `## Teams` section — and execute exactly as described below in "Execute the steps": one agent, one step at a time, in order. **Nothing in this section changes that path.** Team dispatch only activates when the plan file has `teamed: true` and a non-empty `## Teams` table (written by `$iterate-planner`'s teamify operation — see its SKILL.md for the table schema: Team / Steps / Focus / Depends on / Agent).

When a plan IS teamed, on each `$iterate` entry (a fresh dispatch, the return of a parallel dispatch, or a cron resumption tick):

1. **Compute readiness.** A team is *ready* if every team named in its `Depends on` column has `Status: done` in its own row. `$iterate-planner` writes every row with `Status: pending` at teamify/auto-classify time — **that Status cell is the one thing `$iterate` is allowed to update** in the Teams table (`pending` → `in-progress` → `done` / `blocked (<reason>)`); never touch Team/Steps/Focus/Depends on/Agent, and never add or remove a row — that's `$iterate-planner`'s side of the table. Teams with no unmet dependencies and no prior dispatch are ready immediately.
2. **Unassigned steps** (step numbers not listed under any team) belong to the coordinator — that's you, the top-level `$iterate` turn. Execute them yourself, serially, following the normal "Execute the steps" procedure, at whatever point their step numbers fall relative to team dependencies (before dispatching teams that depend on them, interleaved otherwise).
3. **Dispatch every currently-ready, not-yet-dispatched team as parallel subagents, in one instruction** — state plainly that you are spawning one agent per ready team to work concurrently, name each one, and give each its full self-contained brief (step 4). Set that row's `Status` to `in-progress` and write the plan file **before** dispatching, so the record survives even if this turn dies mid-flight. Codex runs them in parallel, waits until all requested results are available, and returns a consolidated response — you do not poll and you do not need a completion notification. <!-- codex-port: subagent orchestration rewritten in plain language; Codex exposes no fixed spawn/wait tool schema to skill authors — verify against https://learn.chatgpt.com/docs/agent-configuration/subagents -->
4. **Each team's Agent prompt must be fully self-contained** (a fresh subagent has no memory of this conversation or this plan file beyond what you put in the prompt). Include, verbatim:
   - **Its identity, as its own labeled statement — not something to infer from a file path.** State it plainly: "You are team: `<team-name>` (from this plan's Teams table). Your log file MUST be exactly `./.claude/iterate/plans/<name>.teams/<team-name>.log.md` — do not rename yourself, even if you'd naturally describe your own work differently." Confirmed live: a dispatched team named its own log file after its own description of the work instead of the Teams-table name (`app-macos` instead of `gui`) — the identity had only ever been implicit, embedded inside a path string it was told to write to, never stated as its own fact.
   - The plan's Goal.
   - This team's Steps + Validations (the exact Na/Nb pairs it owns — nothing from other teams), **including each step's `[skill: /x]` tag, plus this instruction verbatim: "A step's `[skill: /x]` tag is binding — invoke that skill for that step's work; do not substitute ad-hoc shell for work a tagged skill governs. `[skill: none]` steps are yours to do directly."**
   - The plan's global Constraints — **including any known baseline duration for a specific operation** that `$iterate-planner` folded in from the oracle (e.g., "compiling X normally completes in under 60s") — see "Know the baseline, don't guess it" below. If a Constraint gives a real number, that's the team's expectation, not something to estimate.
   - **The plan's feature branch, stated plainly:** "All work happens on the already-checked-out branch `<branch>` — never switch branches, never create one, never merge or push. Branch lifecycle belongs to the coordinator." (Teams share the coordinator's working tree; a team switching branches mid-run would yank every other team's files out from under it.)
   - **If this team's Steps depend on a remote/access-gated resource** — an SSH host, API key, database, or gated URL, per an `Access:` Constraint or a `[skill: $accounts]`-tagged step — that step must be this team's own first action, run for real before anything else: the exact capability its later steps need, not bare connectivity. A team that starts polling/waiting on a remote resource without first confirming it can observe that resource's actual state is not making progress, it's guessing — see "Access verification" above for the full failure-handling protocol (self-heal via `$accounts`, then an immediate operator-wall report if that can't fix it, never a silent wait-and-hope).
   - **The team writes ONLY to its own scoped log file (named above), never to the main plan file.** This is what makes concurrent dispatch safe: N subagents never touch the same file, so there's no write race for the coordinator to worry about.
   - **Mandatory progress checkins, real content, real tooling — not a guess.** Run any operation likely to take more than ~1 minute through `iterate-run` instead of invoking it bare: `iterate-run run --plan <plan> --team <team> --unit <step-id> -- <command...>`. It wraps the command, tees its output, ticks a heartbeat every 10s, and — critically — makes the *wake-up* decision itself: it stays silent on its own stdout for routine ticks, and only prints when something is actually worth reacting to (`ALERT stalled ...` after 6 quiet ticks / 60s of genuine inactivity, `RESUMED ...`, `DONE ... exit=0`, `FAILED ... exit=<code>`). Relay whatever `iterate-run` reports into your own outward checkin log at least once a minute — that's real observability, not a paraphrase. **If you are writing a new script/tool as part of this step** (not just invoking an existing one), have it emit `##ITERATE-PROGRESS## {"done":N,"total":M,"message":"..."}` lines as it works (a line per item or per batch) — `iterate-run` parses that exactly, instead of guessing at arbitrary output; a silent loop over thousands of items is exactly the black-box case this exists to prevent. If a Constraint carries a known duration for this exact operation (see "Know the baseline" below), that's real evidence — trust it over `iterate-run`'s own generic stall window. The team must also log a line every time it finishes a step, and `iterate-run status` (run from any directory, no plan file parsing needed) is always available as an independent cross-check of current state — yours or any other team's.
   - **Structured per-validation reporting, not just a final summary.** As each numbered Validation (Nb) is actually assessed — not reconstructed retroactively at the end — append a line to this team's own log: `##ITERATE-VALIDATION## {"step":N,"status":"met|partial|not-met","note":"one line, specific"}`. `met` = the validation's own wording is satisfied in full. `partial` = some of it is proven and the rest is either infeasible as written or genuinely not yet attempted — say which, specifically, in the note (e.g. "file push/pull and SSH login both proven; utmctl exec cannot return output on this host, no invocation satisfies that clause"). `not-met` = attempted and failed, or not yet attempted at all. This is what lets anything reading the plan — a dashboard, a status check, the next agent — see real per-step state instead of one blanket team-level status; a team can be `in-progress` overall while several of its own validations are already `met`.
   - A condensed restatement of `$iterate`'s own non-negotiable execution rules (see "Rules" below) — never ask a question, Nb is the contract and Na is a hint, pick the most reasonable default and log it, validation must exercise the system not just read code, 5-cycle cap per failing check, pre-existing breakage isn't a blocker, "more of the same shape" is never a stop reason.
   - The instruction to end its log file with exactly one terminal line when finished: `TEAM DONE: <one-line summary>` or `TEAM BLOCKED: <specific reason + what's needed>`.
   - Name each dispatched agent `<plan-name>-<team-name>` (e.g. `owl-database`) — this is how you tell whose result is whose in the consolidated response, and how you address one directly via `/agent` if you need to steer or stop it mid-run.
5. **Log the dispatch** in the main plan file's Status/Log: `dispatched teams: <names> (parallel)`. Update the heartbeat and write the file before the dispatch returns.
6. **Merge when the consolidated result returns.** Codex waits for every dispatched agent and hands back their results together, so there is no notification to catch and no poll interval to tune. For each returned team, read its scoped log file, confirm it ends with a terminal line, and **merge**: copy the team's log content into the main plan file under a `### Team: <name>` heading in Decisions log / Status-Log, check off that team's step numbers in the main `## Steps` checklist, append one `## Changelog draft` line per real change the team shipped (from its log — see "Changelog" above), and set that row's `Status` cell to `done` or `blocked (<reason>)` — the only cell you touch in the Teams table. This merge is the ONLY thing that writes team content into the shared plan file, and only the coordinator does it — never a team subagent.
   - **A team whose log has no `TEAM DONE`/`TEAM BLOCKED` line** despite its agent returning did not finish cleanly. Treat it as blocked, record `blocked (returned without a terminal line)`, and carry that into the stuck-report rather than assuming success from a bare return.
   - **Crash recovery — the one case a cron tick still matters.** If a tick finds rows marked `in-progress` but this turn never dispatched them, the previous turn died between dispatch and merge. Check each such team's scoped log file: a terminal line means merge it normally; a log with recent content but no terminal line means the work was cut off — re-dispatch that team with an explicit note of what its log already shows, so it resumes rather than redoing side-effecting work; an empty or missing log means it never really started, so dispatch it fresh. **Side-effecting work (writes, migrations, deploys) can't safely run twice** — always read the log before re-dispatching, never re-dispatch blind.
7. **Newly-ready teams** (all their dependencies just flipped to `Status: done`) get dispatched immediately after merging — right then, in the same turn, not on the next cycle.
8. **Full-plan validation** (the "Validate" step below) only runs once every team AND every unassigned step is `Status: done`. Some teams done, others still in flight is a normal **end-of-turn, not complete** state — log it and let the cron continue; this is not a status-check menu, it's the existing "normal end-of-turn" allowance applied per-team.
9. **One blocked team does not block independent teams** — exactly like the existing "one blocked outcome does NOT block other outcomes" rule, just scoped to teams instead of goal-outcomes. Only report a hard stop when EVERY team (and every unassigned step) is either done or blocked, with at least one blocked — aggregate the blockers from each `TEAM BLOCKED` line into the single stuck-report (see "Report and either complete or stop" below).
10. **Any status report on a teamed plan (mid-run, when the user checks in, or an optional line at dispatch/merge time) must be structured, not "still going."** For every team currently `in-progress`, report: last log update (age), which steps are done vs. the one in flight, and a one-line gist of what's happening right now (pull it straight from the team's latest progress line — don't paraphrase into vagueness). **Explicitly flag any team whose log has gone quiet past its own mandated ≤1-minute checkin cadence** (or past a Constraint-backed baseline, if its last line announced one) as `quiet since <age>` rather than folding it into the same bucket as a team that's actively logging on schedule — quiet-but-not-yet-returned is not the same as "nothing to report." Use `/agent` to address that team directly if you need a live answer before its result lands. This is what makes team dispatch transparent and accountable instead of a silent black box between merges. Example shape:
    ```
    data — updated 1m ago, steps 3-5 done, step 6 in progress (FFVI link-group sweep)
    ui — updated 6m ago (checking in), steps 1-3 done, last seen: redeploying to re-verify a fix
    ```
    This is a report of real state pulled from the logs, not a status-check decision menu — it doesn't ask the user anything, it just tells them what's true right now. Producing it costs nothing extra: you already read these logs to check for terminal lines.

### Know the baseline, don't guess it

A fresh subagent has no memory of how long anything in this project normally takes — a compile that's run 5000 times and always finishes under 60 seconds looks, to a subagent seeing it for the first time, exactly like an unknown quantity that could reasonably take 10 minutes. That gap is what makes generic timers weak: they're either too loose for something normally fast (real problems hide inside the slack) or too tight for something normally slow (false alarms).

The fix is real data, not a better guess: if `./.claude/data/oracle.md` (or the global oracle) has a known duration for a specific operation — a build, a migration, a deploy step — `$iterate-planner`'s oracle merge (see its SKILL.md) folds it into that step's Constraints as a concrete number, e.g. `Context: the app-build compile normally completes in under 60s (even faster on a warm cache).` That Constraint flows into the team's prompt verbatim (item 4 above), so the team's pre-announcement line uses the real number instead of inventing one, and the coordinator's quiet-team check compares against it directly instead of falling back to a generic window.

**When a team hits a real, unexplained deviation from a known baseline** (the compile that always takes under a minute is still running at 10x that with no error), that's a strong signal to actively investigate right then — check the process, look for a hang, don't just keep waiting — not something to shrug off because it's still short of some generic ceiling. It's also exactly the kind of fact worth an `$oracle harvest` at the end of the run, win or lose: either "confirmed: still under 60s" (reinforcing the baseline) or "found: now regularly takes N minutes because of X" (updating it) are both worth capturing so the next run starts smarter than this one did.

### Access verification — never wait on what you can't observe

A step or team can look busy while accomplishing nothing if it starts waiting on a remote resource — an SSH host, a VM build, a deploy pipeline — without ever confirming it can actually observe that resource's real state. This is a distinct failure mode from an ordinary blocked validation: nothing fails loudly, because there was never a real check running in the first place, just a hope that one would materialize. (Confirmed real case: a run driving an incus VM build over `ssh cypressLinux` sat "in progress" for a long stretch; the actual problem was it had never confirmed it could read that VM's build status from that host at all.)

**The rule:** before entering any wait-and-poll loop against a remote target (SSH host, API, deploy status, VM/container build), run ONE real probe of the exact capability you're about to depend on — not bare reachability, the actual operation (not `ssh host true`, but `ssh host '<the real status-check command>'` returning real, sensible output). A plan built by `$iterate-planner` already has this as an explicit `[skill: $accounts]`-tagged step at the front of whichever scope needs it (see its "Access preflight scan"); a direct `$iterate <task>` run inserts it itself in Step 1.6 above. Either way, only after that probe succeeds does the wait loop begin.

**On probe failure — iterate with the owner, don't just report and stop:**
1. Try self-heal via the `accounts` skill first (`[skill: $accounts]`) — it already handles the common self-service cases (KeePassXC locked and emptying ssh-agent, no identities loaded, a wrong Host alias, an expired-but-refreshable token). This IS the "alternative approach" rule 11 already allows for any step whose Na mechanism fails — try it before treating this as a hard wall.
2. If `$accounts` resolves it, log the fix in the Decisions log and proceed — it was never a human-only wall, just a fixable state.
3. If `$accounts` reports it genuinely needs the user — a brand-new credential/API key to generate, a new SSH key to authorize on the remote side, an account that doesn't exist yet, a physical/MFA action — this is exactly the existing "wall only a human can clear" stuck condition (see Step 5's stuck-report shape below). Report it **immediately**, do not burn the 5-cycle cap first (there is nothing to retry — the access genuinely doesn't exist yet), and phrase the ask as one crisp, actionable request naming exactly what's needed (the credential type, where it goes, or the exact command the user needs to run). The user re-invokes `$iterate` the instant it's supplied, which resumes from state exactly like any other blocker — this is the "iterate with the owner to get access" loop, not a dead stop.
4. **Never proceed past a failed probe to start the wait loop anyway, hoping it resolves.** A step or team whose access probe hasn't passed does not get to move on to its next step — same treatment as any other unmet Nb. Independent steps/teams that don't need this particular access proceed normally per rule 16 (one blocked outcome doesn't block others).

## Steps

### 1. Parse the instructions

From $1, extract:

- **Goal** — one sentence paraphrase of what the user wants.
- **Numbered steps** — their explicit task list (preserve the user's numbering).
- **Validation criteria** — anything they marked as "validate", "success when", "done when", "before moving on". These are the gate; never declare success without checking them.
- **Constraints** — explicit "don't do X", "must use Y", "always Z" rules.

If the user's instructions are vague, **paraphrase the most reasonable interpretation** into the steps section yourself. Do not ask. Log the interpretation in the Status section so the user can see it if they want.

### 1.5. Oracle fallback (for direct $iterate invocations without $iterate-planner)

If you reached this step via a fresh `$iterate <task>` (i.e. `$1` non-empty, no prior planning happened):

- Build the oracle buzzword index by reading the **index sections** of both stores:
  - Project: `./.claude/data/oracle.md`
  - Global: `~/.agents/skills/oracle/known.md` (Codex's user-level skill directory; `$oracle` must be ported separately for this to exist)
- Scan your interpreted task (Goal + Steps + the user's `$1`) for buzzword matches against the index. Case-insensitive substring; allow plural / verb forms.
- For each matched buzzword, read its full 5W+H entry from the right store (project wins on conflict).
- Fold the entry into Steps / Validations / Constraints using the same rules as `$iterate-planner`:
  - **How** → new Steps + paired interactive Validations.
  - **Where** → Constraints with `Context:` prefix.
  - **When** (when-not-to-use) → Constraints.
  - **Who** (operator-specific actions) → Constraints.
- Log each folded buzzword in the Decisions log: `oracle: matched "<buzzword>" → added N steps, M constraints`.
- If no buzzwords match: log `oracle: no matches in [list of stores read]`. Don't pad with irrelevant entries.
- If `planner: iterate-planner` is set in the state file (the plan was built oracle-aware already), **skip this entire step** — trust the existing plan.

If neither oracle store exists, skip silently.

### 1.6. Access preflight (fallback for direct invocations)

If `planner: iterate-planner` is set in the state file, trust that the plan's access-check steps (tagged `[skill: $accounts]`, front of whichever scope needs them) were already inserted at planning time — skip re-scanning, same as the oracle fallback rule above.

Otherwise (a fresh `$iterate <task>` with no prior planning, OR a plan that references an SSH host / remote machine / API key / credential / gated URL in its Steps or Constraints with no matching `[skill: $accounts]` step already covering it): run the same scan `$iterate-planner` runs (see its SKILL.md, "Access preflight scan") over the interpreted Goal + Steps + Constraints, and insert the resulting verification step(s) at the very front of the Steps list — or, on a teamed plan, the front of whichever team's own Steps actually reference that dependency — before writing the state file in Step 2. Add the matching `Access:` Constraint per dependency found. If nothing was found, note `access: no external dependencies detected` in the Decisions log and move on — most direct-task runs touch only local files.

### 2. Write the state file

Write `./.claude/iterate/plans/<name>.md` (name from `iterate-run name next` for a direct fresh-task run; keep the existing name when transitioning a planned plan) with this schema:

```markdown
# Iterate Task — <short title>

name: <animal>
Started: <UTC timestamp>
Executing: <UTC timestamp>     # same instant as Started: on this direct fresh-task path — set once, never touch again
CWD: <pwd at first invocation>
phase: executing
executor-version: <version>    # this skill's own frontmatter `version:` — stamped when phase first flips to executing; on resume by a DIFFERENT version, leave it and add a Status/Log line "resumed by executor <version>"
running: <UTC timestamp>       # heartbeat — update at every step boundary
branch: feature/<name>-<slug>  # the plan's feature branch (omit when not a git repo) — see "Feature branch" above
loop-mechanism: user-managed   # Codex cannot arm its own resumption; the user owns any Automation/OS cron
human-gate: <step N>           # only when the plan marks a terminal human-decision step (written by $iterate-planner) — see Step 5's human-gate path

## Goal
<one sentence>

## Steps
- [ ] 1. <step>
- [ ] 2. <step>
...

## Validation
- [ ] check 1: <criterion>     # paired 1:1 with Steps by index
- [ ] check 2: <criterion>
...

## Constraints
- <rule>
- <rule>

## Changelog draft
(append-only, coordinator-only. One line per real change at step check-off / team merge:
`- [added|changed|fixed|removed|security|internal] <product-level phrasing> (step N)`.
Distilled into CHANGELOG.md + RELEASES.md once, at the success path — see "Changelog" above.)

## Decisions log
(append-only. Each entry: timestamp + decision made + why.)

## Status / Log
(append-only. Each entry: timestamp + step + outcome / error / next attempt.)
```

If resuming, do not overwrite — append to Decisions log and Status / Log. Update the `running:` heartbeat as you work.

If transitioning from `phase: planned` (set by `$iterate-planner`): the Steps/Validation/Constraints are already there — just set `phase: executing`, **add `Executing: <UTC timestamp now>` and `executor-version: <this skill's version>` lines** (this is the real execution-start marker the dashboard's "Running for" box reads — do NOT touch `Started:`, which stays as the original drafting time), take the lock, arm the cron, and start. Do not re-parse from $1.

### 3. Execute the steps

**If the plan is teamed** (`teamed: true`, non-empty `## Teams`), go to "Team dispatch" above first — it handles dispatching ready teams and merging finished ones. Everything below in this section is what the coordinator uses for the plan's unassigned steps (if any), and is exactly what runs for the common case of a flat, un-teamed plan.

**Contract semantics:** for each numbered pair, the Step (Na) is *one suggested approach*; the Validation (Nb) is *the contract*. Treat Na as a starting hint, not a literal recipe. If Na's specific mechanism fails (tool absent, command syntax changed, auth rejected, host unreachable, etc.), **try other approaches that achieve the same Nb**. Examples:

- Step says "ranger issues token, explorer adds ranger remote" → if `incus remote add` rejects the token, try generating with `--public`, try `incus config trust add` with a pre-signed cert, try resetting trust on either side. Any path that ends with Nb's "incus remote list has a ranger / tls row" is acceptable.
- Step says "use `apt-get install foo`" → if the package name changed or the repo is missing, try `dnf`, try `snap`, try building from source. The contract is "foo binary exists and runs".
- Step says "ssh as travis" → if travis can't auth, try `root`, try a known fallback key, try installing your key via incusmagic. The contract is "I can run commands on the target".

Log every alternative attempt in **Status / Log**. Log the chosen mechanism in **Decisions log**. The user reviews the log to understand what actually happened versus what was suggested.

**Same streaming discipline as team dispatch applies here.** For any single operation likely to run more than ~1 minute (a build, a migration, a long test run), run it through `iterate-run run --plan <plan> --unit <step-id> -- <command...>` (no `--team`, since this is the coordinator's own unassigned step) instead of a blind blocking call, and log its `ALERT`/`DONE`/`FAILED` output into Status/Log as it happens rather than one entry after it finally returns — see "Know the baseline, don't guess it" under Team dispatch above, which applies to flat plans too: if a Constraint carries a known duration for the exact operation running, use that as the real expectation, and treat a genuine unexplained deviation from it as worth investigating immediately, not something to wait out.

Walk through `Steps` in order. For each step:

1. Figure out *how* to do it from current context. Read files, run commands, ssh wherever needed. **If the step carries a `[skill: /x]` tag, invoke that skill — the tag is binding, not advisory** (Na's mechanism may flex per rule 11, but "which tool governs this work" doesn't). On a direct fresh-task run whose steps have no tags (no planner pass ran), do the cheap version yourself: before executing a step, check the always-loaded skills list for an obvious governing skill (builds → `$dev-makefiles`, access → `$accounts`) and use it.
2. **If a decision is required that the user didn't pre-specify, pick the most reasonable interpretation.** Examples of safe defaults:
   - Path doesn't exist → create it (`mkdir -p`).
   - Command failed with permissions → try `sudo`.
   - Multiple matching files → pick the most-recent-modified or the alphabetically-first; log which.
   - Hostname has multiple IPs → first non-link-local.
   - Tool not installed → try to install via the platform's package manager (`apt-get install -y`, `brew install`, etc.), or fall back to an obvious alternative tool.
   - Service is down → restart it; if still down, log and move on; come back during validation.
3. Log the decision in the **Decisions log** section: `<timestamp> chose <X> for step <N> because <reason>`.
4. On error/blocker: try at least two alternative approaches before logging as "blocked". Common patterns:
   - Network call fails → retry with backoff (3 times, 1s/3s/9s).
   - Command not found on remote → try with full path / alternative names.
   - Resource locked → wait 5s, retry.
   - File parse error → try alternative encoding / line-ending.
5. Mark the step done in the checklist when complete, and append its one-line entry to `## Changelog draft` (see "Changelog" above) if it changed anything real.

### 4. Validate

**On a teamed plan**, don't run this step until every team is `Status: done` (or `blocked`, aggregated per rule 9 in "Team dispatch") and every unassigned step is complete — each team already ran its own owned validations under this same interactive-testing mandate before writing `TEAM DONE`, so this step is the final full-plan sweep across everything once all teams have reported in, not a duplicate of team-internal checks.

After all steps are done (or appear done), execute every validation check from `Validation`. Each check must be a concrete, runnable assertion (a command + expected output, a file's existence, a count, etc.).

**Interactive testing mandate — non-negotiable.** Validation means EXERCISING the system, not READING the code:

- UI changes → actually load the page in a browser, click through the changed flow, watch for console errors.
- API changes → actually `curl` the running endpoint with realistic input and check the response.
- CLI changes → actually invoke the command end-to-end and check stdout / exit code / side effects.
- Migrations / scripts → actually run them against a real target (local or test environment).
- Service additions / deploys → actually hit the live URL and confirm behavior, not just "deploy returned 0".

"Static code inspection passes" / "tests pass" / "the diff looks right" alone is NEVER sufficient. If the plan's Nb doesn't include a real execution step, ADD one before declaring the check green. Log the addition in the Decisions log.

If any check fails:
- **Don't ask the user.** Diagnose, re-attempt the affected step, re-run all validation.
- Cap: 5 cycles of (re-attempt + re-validate) on the *same* failing check. If still failing on cycle 5, log "blocked: <check> failed after 5 cycles, last error: <error>" and stop (see step 5).

### 5. Report and either complete or stop

**On full success (every validation check green):**
- Set `running: false` in the plan file.
- **Publish the changelogs** (see "Changelog" above): distill `## Changelog draft` into `CHANGELOG.md` + `RELEASES.md`, commit them on the plan's branch — they ride the PR.
- **Run the merge flow** (see "Feature branch" above): `$feature-branch finish` → merge the PR → branch deleted, back on the default branch. All-green is the merge trigger; no separate approval needed. A failed merge does NOT un-succeed the plan — flag it in the summary (`⚠ complete but NOT merged — <reason>, branch preserved`) and continue archiving.
- **Add a `Finished: <UTC timestamp now>` line** (same `date -u +%Y-%m-%dT%H:%M:%SZ` format as `Executing:`) right before archiving — this is the real "done at" instant the dashboard's "Ran for" figure reads once archived. Without it, that figure falls back to the latest CONFIRMED activity span (hook/registry data), which can simply not exist for a project with neither wired up — confirmed live: a flat plan showed "Running for 0s" despite a correct `Executing:`, because there was no activity data to compute a span against at all. Set once, never touched again.
- Cancel the auto-resume loop — the exact mechanism recorded in `loop-mechanism:`, per "Auto-resume" — nothing was armed by this skill, so clear the field and tell the user if their Automation is still firing.
- Move `./.claude/iterate/plans/<name>.md` to `./.claude/iterate/archive/<UTC-timestamp>-<name>-done.md`. If `current` pointed at this plan, repoint it to the sole remaining plan (if exactly one) else clear it. If the plan was teamed, also move `./.claude/iterate/plans/<name>.teams/` to `./.claude/iterate/archive/<UTC-timestamp>-<name>-done.teams/` (the per-team logs are already merged into the archived plan file — this just keeps the raw team logs around for audit, don't leave the working `.teams/` dir behind).
- Report a 3-5 line summary: goal, what was done, validation results, time taken, **and the merge result** (`merged to <default> via PR <url>, branch deleted` — or the ⚠ not-merged flag with reason). On a teamed plan, name which teams ran (and, if any ran concurrently, say so — that's the payoff of teaming).
- **Suggest `$oracle harvest`** to the user — one line at end of report: "If anything in this run is worth remembering for next time, run `$oracle harvest`." Don't auto-invoke; oracle harvesting is opt-in.

**On human-gate reached (every agent-completable validation green; only the plan's marked `human-gate` step remains):** this is the DESIGNED ending of a plan that ends in a human decision session — it is success-with-handoff, not "blocked", and it must not fall through to the stuck path below.
- Set `running: false`. Set `status: awaiting-human-gate: <one-line what's needed>` in the frontmatter (leave `phase: executing`). Do NOT archive; do NOT merge the feature branch (⚠-line in the report as always).
- **Cancel the auto-resume loop, verified** (see "Auto-resume" — exact mechanism from `loop-mechanism:`, confirm the cancel result, clear the field). The loop's job is finished; only the human can advance the plan now, and ticks against a waiting gate produce nothing but manufactured work.
- **Then ASK the user directly in the conversation, not only in a markdown file.** This is the second sanctioned exception to "never ask" (alongside entry rule 5's plan picker), and it's principled: autonomy is over by definition at a human gate, so interrupting is the job. Ask the gate's actual question(s) — or, when the gate is a decision *session* too big for one prompt (like a 21-item agenda), ask the meta-question: point at the prepared agenda file and offer "start the session now / I'll come back later". If the user answers now, work the decisions conversationally, record them into the gate's artifacts, then re-run full Validation → normal success path (including the merge flow). If they defer (or the question times out unanswered), leave the awaiting state — the plan resumes when they type `$iterate` again.
- Report shape: 2-3 lines of what's done + `awaiting human gate: <what>` + where the prepared material lives + the ⚠ branch-not-merged line.

**On stuck (5-cycle cap hit AND every other outcome also stuck — genuinely no forward motion possible), OR every validation is met except one clause that only a human can clear** (billing, an external approval, physical/credential access no agent has — a wall, not a failing check — and the plan did NOT mark it as a `human-gate`, else use the human-gate path above):
- Set `running: false`. Update the plan file: mark which steps completed, which validation checks pass, which fail and why.
- Write the "Next attempt" hint in the ONE standardized shape the dashboard tool actually parses — this was previously freeform prose per-run, which the dashboard couldn't recognize at all (it just kept reading as plain `executing` no matter how done the plan actually was):
  - At the very top of the file, above the frontmatter, a blockquote banner: `> **Next attempt (one operator action):** <what's blocking, one or two sentences> <the exact command(s) to run once it's cleared>`.
  - In the frontmatter, `status: blocked-on-operator: <one-line reason>` — that exact `blocked-on-operator` prefix is the literal string the dashboard matches on; don't paraphrase it into "waiting on user" or similar. This is IN ADDITION to `phase:`, not a replacement — leave `phase: executing` as-is.
- Cancel the loop — clear `loop-mechanism:` — nothing was armed to cancel (see "Auto-resume"). Without this, the job fires forever and re-hits the same giveup — and an unverified cancel is exactly as bad as no cancel, just quieter.
- Stop. Report ONE blocker reason — the specific check that failed 5 times (or the specific operator-only clause) AND why no other outcome could absorb attention — plus what specific operator action would unblock. **Do NOT write a menu of "things the user could do next." Do NOT list "(a) ... (b) ..." options. Do NOT frame remaining work as choices.** One blocker, one ask, done. On a teamed plan where multiple teams are blocked, aggregate: report the done/blocked status of every team in one line each, then the single most-actionable next operator step (usually whichever blocker, once fixed, unblocks the most dependent teams).
- Do **not** archive — leave the plan file in place so the user can read what happened and re-invoke fresh after fixing the blocker.

- **The merge-status line is mandatory in every stuck report** (git-repo plans): `⚠ feature branch <branch> NOT merged — <PR open at <url> | no PR opened>`. The merge only happens on all-green completion or an explicit user order (see "Feature branch" above) — the user must never have to wonder whether blocked work landed on main. It didn't.

Acceptable stuck-report shape:
```
Blocked.
Last green: Outcome 1 steps 1-3, Outcome 2 fully done.
Hard blocker: Outcome 3 step 5b (`bao auth list` returns 403) — failed 5 cycles, last error: "permission denied: kubernetes-kelwin1 backend not registered".
Need: someone with OpenBao root token to run `bao auth enable -path=kubernetes-kelwin1 kubernetes`.
⚠ feature branch `feature/owl-bao-auth` NOT merged — no PR opened. Merges automatically when the plan finishes all green.
Run `$iterate` again once that's done.
```

UNACCEPTABLE stuck-report shape (this is the cowardly-stop pattern — never write this):
```
You can $iterate again to drive (a) the remaining chart conversions, or address (b) the pre-existing issues separately and re-invoke.
```

**There is no "normal end-of-turn" under Codex. Keep working.**

<!-- codex-port: Claude Code may stop mid-run because a cron tick resumes it.
     Codex has no such tick, so a voluntary pause here strands the plan until a
     human retypes the command. The allowance is removed, not translated. -->

Under Claude Code this skill may pause mid-run because a scheduled tick will
wake it. **Nothing wakes it here.** A voluntary stop is therefore not a pause —
it is the end of the run, with the plan half-done and no one coming.

So: work until one of the genuine terminal states is reached — all-green
success, 5-cycle giveup, blocked-on-operator, or human-gate. Do not stop because
the run feels long, because a step finished cleanly, because a milestone looks
like a good place to report, or because a summary would read well right now.
None of those are endings; under Codex each one abandons the plan.

If the turn ends anyway — context exhausted, an API error, the session killed —
that is survivable but not automatic: `running:` goes stale, the plan file holds
every step, decision and validation, and the next `$iterate <name>` a human
types resumes exactly where it stopped. Nothing is lost. It simply waits for a
person instead of a clock, which is the one real cost of having no scheduler.

## Surviving an auto-compact (this happens on essentially every plan)

Long runs fill the context window and the harness compacts: the conversation is
replaced by a summary and **the turn continues**. This is not an interruption to
recover from — it is the normal middle of a long plan, and it happens on
essentially every real run. Treat it as routine.

What compaction costs you is **memory, not state**. The plan file is untouched:
phase, the Steps checklist with its checkmarks, Validations, Constraints,
Decisions log, Status/Log and the `running:` heartbeat all survive on disk
exactly as they were. Your recollection of them does not.

So, the moment you notice the context was compacted — a summary in place of the
history, or simply that you cannot recall the last few steps in detail:

1. **Re-read the plan file before doing anything else.** Not to "check", but
   because it is now your only accurate account of where the run is. Never
   reconstruct progress from the summary; a summary is lossy exactly where step
   numbers and validation outcomes live.
2. **Re-read this skill if the rules are no longer in context.** A compacted run
   that has forgotten its own contract is how a plan gets a cheerful wrap-up at
   step 7 of 22.
3. **Resume, do not restart.** A checked-off step is done — do not redo it,
   re-verify it, or "confirm" it. Side-effecting work redone after a compact is
   the expensive failure here: a second deploy, a duplicate migration, a
   re-deleted resource.
4. **Keep going.** Compaction is not a terminal state. It is not a milestone, a
   natural pause, or a good moment to report. Pick up at the first unchecked
   step and continue to a real ending.
5. **Do not write a summary of what you have done so far.** The instinct after
   a compact is to re-establish context out loud for the user. Resist it — the
   plan file already holds that record, and a mid-run wrap-up is how a
   half-finished plan gets mistaken for a finished one.

**If the plan file and your memory disagree, the file wins, always.** It is
written at every step boundary and every validation; your post-compact
recollection is a summary of a summary.

## Destructive work the plan already authorized

**A step that says "delete X" IS the authorization to delete X.** The plan was
written and accepted; re-asking at execution time is asking the same question
twice and stalling a run for days between them.

Escalate only when the plan does NOT cover it — a resource the plan never names,
production, someone else's data, or anything genuinely irreversible outside this
project. Size is not a reason: 109 GiB of dev artifacts the plan lists by name
is not more sensitive than 1 GiB of them.

**Precedent within the same plan settles it.** If a step of the same class
already ran — step 7 deleted three instances cleanly — then steps 4, 5 and 6 are
that same class and do not get a fresh gate. Confirmed live: a plan sat eight
days on exactly that boundary, having already proven both the capability (step 1
verified administrative delete) and the permission (step 7 ran).

**When the operation is fiddly, write a script and run it.** Multi-step
destructive work with verify-then-delete ordering is safer as a script than as
a sequence of remembered commands: the ordering is encoded, it survives a
compaction, it can be dry-run, and a failed verification skips that delete
instead of aborting everything. Put it in the project's `scripts/`, make it
re-runnable, and gate every delete on its own check passing. That is normal
engineering, not a workaround.

**What is never a blocker:** your own caution, a large number, a resource you
created but no longer remember creating, or an operation you have already
performed once in this plan. Blocked means *a human must supply something you
cannot* — a credential, a physical action, a decision only they can make.

## Rules (hard, non-negotiable)

1. **Never ask the user a clarifying question during execution.** If you need a decision, pick the most reasonable one and log it in the Decisions log. The user can read the log later. Exactly two sanctioned exceptions: the entry rule 5 plan picker, and the **human-gate handoff** (Step 5) — when the only thing left is the plan's marked `human-gate` step, autonomy is over by definition and asking IS the job.
2. **Never stop just because something is uncertain.** Pick a path, try it, log, continue. "Uncertain" is not the same as "stuck".
3. **Never declare success without running every validation check.** Self-validate, every time.
4. **Always update the plan file before doing anything destructive** (delete, overwrite, force-push, restart service). The state file is the resumption contract; don't violate it.
5. **Never replace a plan file without archiving first.** Old state goes to `./.claude/iterate/archive/<UTC-timestamp>.md`.
6. **Logging is mandatory.** Every decision and every step outcome must land in the appropriate section. Future-you (next `$iterate` call) reads the log to know what's already done.
7. **Don't loop forever.** 5 cycles per failing validation check, then stop and report. On stop, tell the user to pause their Automation if they created one.
8. **Respect the user's constraints absolutely.** Constraints listed in the state file override your own judgment.
9. **Set up a resumption loop on first run; on terminal exit (success, giveup, blocked-on-operator, human-gate) cancel the EXACT mechanism you armed and verify the cancel took. 1 minute is the maximum interval — flat or teamed, no exceptions.** Record `loop-mechanism: user-managed` — this skill arms nothing, so there is nothing to cancel, but a user-created Automation keeps firing until they pause it and must be told so. Under Codex a stalled turn simply waits for a human; the plan file loses nothing.
10. **Honor the concurrency lock.** If `running:` is fresh, exit silently — don't double-run.
11. **Na is a hint; Nb is the contract.** If a step's described mechanism doesn't work, find another path that meets the validation. Only after exhausting reasonable alternatives (and hitting the 5-cycle cap per failing validation) do you give up. Never treat the step's wording as a constraint.
12. **Validation requires real execution.** Never declare a check green on the basis of code inspection alone. The validation must actually run/load/curl/click the thing being changed. If the plan's Nb says "tests pass", you ALSO load the page / hit the endpoint / run the command end-to-end before marking it done. Log the added execution in the Decisions log.
13. **Respect the oracle if it exists.** `./.claude/data/oracle.md` contains project-specific lessons. On direct `$iterate` (no prior plan), read it during Step 1.5 and merge its rules. On `phase: planned` with `planner: iterate-planner`, trust that the oracle was already applied.
14. **NEVER produce a status check or "Either/Or" decision menu in any report.** Mid-run, end-of-turn, on giveup, on success — none of these allow:
    - "You can $iterate again to drive (a) X or (b) Y"
    - "(a) I can continue with; (b) needs operator context I don't have"
    - "Stopped because: <list of unfinished things you could just keep doing>"
    - Any framing that asks the user to pick between conservative-vs-aggressive paths
    The user invoked `$iterate` to iterate, not to be polled. Allowed reports:
    - **Success**: 3-5 line summary of what's done.
    - **Hard blocker** (only): "blocked: <specific reason — credential / external service down / 5-cycle giveup with last error>; what's needed: <specific operator action>". One reason. No menu.
    - **Mid-run**: do not write a wrap-up at all. Under Codex there is no tick to resume you, so a mid-run summary is how a half-finished plan gets mistaken for a finished one. Keep working instead.
15. **Pre-existing failures are NOT blockers — flag and continue.** If validation catches a broken pod / app / state that pre-dates this run (check pod `creationTimestamp`, last-restart age, Application `lastTransitionTime`, prior Decisions log entries), record it once in Status/Log as `pre-existing: <name> broken since <date> — not caused by this work` and TREAT THE CHECK AS GREEN for blocking purposes. Don't count it toward the 5-cycle cap. Don't stop on it. The user can address pre-existing breakage separately; THIS run keeps going.
16. **One blocked outcome does NOT block other outcomes.** Multi-outcome plans (e.g., "do X on cluster A, then B, then C, then retire D") have independently-progressable outcomes. If outcome 1 is stuck at a hard blocker, MOVE TO outcome 2 immediately. Work outcomes in parallel where ordering allows. Only report "stuck" when EVERY outcome is independently stuck on a hard blocker. Acceptable transition: "Outcome 1: blocked on X (operator needed); proceeding to outcome 2." Then keep going.
17. **"Straightforward more work" / "more of the same shape" is NEVER a stop reason.** If the remaining work is "6 more MRs of the same shape as the one that just succeeded," DO THOSE 6 MRS. Don't ask if the user wants you to continue. Don't summarize what's left as if presenting options. Repetitive work is exactly what `$iterate` is FOR — that's why you're called the iterator. The only acceptable stops are: full success, all-outcomes-stuck-on-hard-blocker, or 5-cycle giveup on a specific failing check.
18. **Un-teamed plans are unaffected by any of this.** Team dispatch only activates when `teamed: true` and `## Teams` is non-empty. A flat plan runs exactly as it always has — single agent, serial steps, no scoped log files, no dispatch logic. Don't go looking for teams to dispatch when there's no Teams table.
19. **A team subagent never writes to the main plan file.** It writes only to its own `./.claude/iterate/plans/<name>.teams/<team>.log.md`. Only the coordinator (the top-level `$iterate` turn) merges team logs into the shared plan file, and only after seeing a `TEAM DONE`/`TEAM BLOCKED` terminal line. This is the entire safety mechanism against concurrent-write races between teams — don't shortcut it by having a team edit the plan file directly, even "just this once."
20. **Never dispatch a team whose dependencies aren't `status: done`.** Check the Teams table's `Depends on` column before every dispatch, every tick. A team becomes eligible the moment its dependencies clear — dispatch it that same tick, don't wait for an extra loop cycle.
21. **A `TEAM BLOCKED` team is a per-team giveup, not a whole-plan giveup.** Apply rule 16 (one blocked outcome doesn't block others) at the team level: keep dispatching and progressing every team that isn't itself blocked. Only reach the whole-plan "stuck" report (rule 14/16's stuck path) when every team is done-or-blocked and at least one is blocked.
22. **Never start a wait-and-poll loop against a remote target without first confirming, via one real probe of the actual capability needed, that you can observe its state.** See "Access verification" above. A step/team that's "still running" against a remote host, VM, or API with no verified way to check on it is not progress — treat the missing probe itself as the blocker, and run it before doing anything else in that scope. On probe failure: try `$accounts` self-heal first, then report an operator-wall blocker immediately (no 5-cycle wait) if that can't fix it — never proceed to the wait loop hoping it resolves.
23. **All-green is the ONLY automatic merge trigger; every not-all-green report names the unmerged branch.** On full success, run the merge flow (`$feature-branch finish` → merge PR → delete branch) as part of Step 5 — no separate approval needed, that's what all-green means. On ANY other ending (blocked, stuck, user-ordered close, roll-forward), the branch stays unmerged and the report carries the ⚠ not-merged line — the user must never have to guess whether unfinished work landed on main. The one override is an explicit user order to merge ("merge it", "merge what we have and close") — obey it, then do whatever else they asked. Branch operations always go through `$feature-branch`, never hand-rolled git; and a failed merge on an otherwise-complete plan is flagged, not retried into the 5-cycle loop — merge conflicts against a moved main are the user's call, not a validation failure.
24. **Teams never touch the branch.** Dispatched team subagents work on the coordinator's already-checked-out plan branch — no switching, no creating, no pushing, no merging. All branch lifecycle belongs to the coordinator (and `$iterate-planner` at creation time). A team that needs "a different branch" doesn't — that's a sign the step belongs to a different plan.
25. **An MR or merge problem is never a reason to stop, and never a question.** A failed push, a rejected merge, a protected branch, a pipeline that will not go green, a branch that should not have existed — none of these are plan failures and none of them earn an interruption. Log it, flag it in the report with the branch named, and carry on with the rest of the plan. **Do not ask "may I merge these?" mid-run**: the all-green rule already answers it — all-green merges automatically, anything else does not merge and says so. Asking turns a clear contract into a decision the user has to make at a keyboard they may not be sitting at, which is the exact interruption this skill exists to avoid.
26. **Never escalate something the plan already authorized.** A step naming a resource to delete, restart or replace is the approval — executing it is the job, not a decision to bring back. If an earlier step of the same class already ran in this plan, that settles any doubt about the later ones. Reserve blocked-on-operator for what a human must actually supply: a credential, a physical action, a judgement only they can make. Your own caution is not a blocker, and neither is a big number.
25. **Changelog: draft at every check-off, publish exactly once — through the final sweep.** One draft line per real change, appended by the coordinator at step check-off / team merge — never by teams, never as polished prose. Publishing always runs the two-pass sweep (consolidate multi-attempt/reworked lines into one as-landed line each; validate every claim against the final tree — unvalidated claims don't publish). `CHANGELOG.md`/`RELEASES.md` are written only in the success path (or a flagged-partial entry on close, same sweep applied) — never incrementally mid-run, and old entries are never rewritten. `RELEASES.md` carries only user-visible changes in product language; internal work stays in `CHANGELOG.md`.
26. **A step's `[skill: /x]` tag is binding, for coordinator and teams alike.** Rule 11's "Na is a hint" covers the mechanism *within* the governing tool, never a license to bypass the tool: a build step tagged `$dev-makefiles` gets its target added via that skill, not an ad-hoc shell script that happens to compile. Tags travel into every team prompt with the binding instruction. Untagged plans (direct fresh-task runs) get the cheap check: scan the always-loaded skills list before each step for an obvious governing skill.

## Example trigger

> "I want you to do the following: 1. connect polaris2 to the polaris/polaris3 cluster. 2. Move all existing polaris2 vms over to polaris. 3. Move all existing polaris3 vms over to polaris. 4. Validate the following before moving on: 1 and 2 have no incus vms or containers. 5. Declare success. $iterate"

What the skill does:
- Parses: 3 action steps + 1 validation check ("polaris2 and polaris3 have no incus vms or containers")
- Writes state file
- Executes step 1 (cluster join), then 2 (move polaris2 vms), then 3 (move polaris3 vms)
- Runs `incus list` on polaris2 and polaris3 to verify both are empty
- If something fails partway (auth error, network blip, VM move stuck): retries, picks alternatives, logs, doesn't ping the user
- If everything passes: archives state, reports "✓ cluster joined, 7 vms migrated, polaris2 & polaris3 empty (verified via incus list). Done."
- If stuck after 5 cycles on validation: stops, leaves state file, reports what's done + what's blocking + last error

## Example trigger — teamed plan

Plan `owl` is `phase: planned`, `teamed: true`, with Teams: `deploy` (steps 2,4; no deps; agent backend-expert) and `link-tree` (steps 3,5; depends on `deploy`; agent documentation-expert). User types `$iterate owl`.

What the skill does:
- Transitions `owl` to `phase: executing`, sets `Executing: <now>`, takes the lock, arms a 1-minute recurring cron firing `$iterate` (1 minute max, teamed or not) and records its job id in `loop-mechanism:`.
- Team dispatch: `deploy` has no unmet dependencies → ready. `link-tree` depends on `deploy`, which isn't done yet → not ready.
- Sets `deploy` row `Status: in-progress`, logs "dispatched teams: deploy (parallel)", writes the plan file, then spawns one agent named `owl-deploy` (steps 2,4 + Goal + Constraints + its scoped log path `./.claude/iterate/plans/owl.teams/deploy.log.md` + the mandatory-checkin instruction).
- Mid-flight, if the user checks in: reads `deploy.log.md`'s latest checkin line (not just waiting for a terminal line) and reports it plainly — e.g. "deploy — updated 4m ago, container built, running smoke test now." No terminal line yet, so nothing to merge; this is just reading the log, not a poll tick.
- `owl-deploy` returns in the consolidated response. `deploy.log.md` ends with `TEAM DONE: metrics-service deployed and smoke-tested, curl https://metrics.gravhl.com/health returns 200`. Merges that into `owl.md`'s Status/Log under `### Team: deploy`, checks off steps 2 and 4, sets the `deploy` row `Status: done`.
- Same turn: `link-tree`'s dependency (`deploy`) just cleared → now ready. Dispatches `owl-link-tree` immediately (steps 3,5 + same Goal/Constraints + its own scoped log path), sets `Status: in-progress`. Ends the turn.
- `owl-link-tree` returns, `link-tree.log.md` ends with `TEAM DONE: link tree updated, verified live in browser`. Merges it, checks off steps 3 and 5, sets `link-tree` `Status: done`.
- All teams done, no unassigned steps → runs full-plan Validation once more across everything. All green.
- Merge flow: `$feature-branch finish` pushes `feature/owl-metrics-service` and opens the PR, then `gh pr merge --squash --delete-branch` lands it on main and removes the branch local + remote.
- Archives `owl.md` and `owl.teams/`, confirms nothing was armed (Codex resumption is user-managed), reports: "✓ owl done — 2 teams (deploy, link-tree ran sequentially due to dependency), 4 steps, all validations green. Merged to main via PR #12, `feature/owl-metrics-service` deleted."
