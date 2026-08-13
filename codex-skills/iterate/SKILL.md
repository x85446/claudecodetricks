---
name: iterate
description: Use when given a multi-step task with validation criteria and asked to execute autonomously until done. The skill does NOT ask the user clarifying questions mid-run; it picks the most reasonable interpretation, executes, validates, loops, solves its own blockers, and only returns control when validation passes or the run is truly stuck. When the plan is teamed (see $iterate-planner's teamify), dispatches one subagent per independent team to run concurrently instead of working the Steps list serially. Re-invokable — running `$iterate` again resumes from the saved state file. Triggers on "$iterate", "iterate until done", "keep going until X", "work this until validation passes".
---


# $iterate — Run a task to completion without interrupting the user

The user invoked this skill because they're tired of being interrupted by clarifying questions. The whole point: **make the call yourself, log it, continue**. Only return control when the validation criteria they provided are all green, or when you've truly exhausted reasonable attempts.

## Named plans & state files (this is how resumption works)

Plans are **saved, animal-named, and persistent**. Each plan is one file:

    ./.claude/iterate/plans/<name>.md        (project-local, relative to cwd)

`<name>` is a common animal (dog, cat, fox, owl, elk, wren, …), assigned via `iterate-run name next` — see $iterate-planner's "Named plans" for why (this project's own alphabetical sequence, a/b/c/…, drawing from one machine-wide "already used" set so two unrelated projects never land on the same codename). If `iterate-run` isn't installed, fall back to any common animal not already present in this project's own `plans/` and note that the global registry was unavailable. `./.claude/iterate/current` points at the **current** plan. **The plan file** below always means the plan being executed (resolved from `$1`'s name, or `current`, or the sole executing/only plan). Older wording in this doc may say `active.md` — read it as the resolved plan file.

Each plan has a top-level `phase:` field:

- `phase: planned` — written by `$iterate-planner`, waiting for execution to start.
- `phase: executing` — execution is in flight (or has been; check `running:` for whether a run is currently live).

Two separate timestamps, don't conflate them: `Started:` is when the plan was **drafted** (set once by `$iterate-planner` or on a direct fresh-task run, kept forever). `Executing:` is when execution **actually began** (set once, by `$iterate`, at the exact moment `phase` first flips to `executing` — see the transition points below — and never touched again, including on resume). A plan drafted well ahead of when it's actually run needs both: `Started:` answers "when was this planned", `Executing:` answers "how long has this really been running" (what the dashboard's "Running for" box uses).

**Legacy migration:** see $iterate-planner's "Named plans" section for the one-time `active.md` → `plans/<name>.md` migration procedure — identical here, do it silently on first touch rather than maintaining two copies of the same steps.

### Entry decisions on `$iterate`

Resolve in this order:

0. **`$1` is exactly "version"** (or "what version", "iterate version"): run `iterate-run version` and print its output verbatim — real installed binary, not a memory recall, works from any directory. If not found, report "iterate-run isn't installed — run `make install` in claudecodetricks." Then **stop**, no plan involved.
1. **A plan is already `phase: executing`** (scan `plans/`): resume THAT plan from its "Status / Log", honoring the concurrency lock. This takes precedence over everything below — it's what makes the Cron job's re-fires (which pass no `$1`) continue the live run instead of prompting. If several are somehow executing, pick the one named by `current`, else the most-recently-heartbeated.
2. **`$1` names an existing plan** (`$1` exactly matches a `plans/<name>.md`, e.g. `$iterate dog`): set `current` = that plan; if `phase: planned` → transition to `phase: executing`, **set `Executing: <UTC timestamp now>`**, set up the auto-resume loop, begin; if already executing → resume it (leave `Executing:` untouched).
3. **`$1` is substantive task text** (a paragraph/steps, not a bare existing name): create a **new** plan, named via `iterate-run name next`, with `phase: executing`, **`Executing:` set to the same UTC timestamp as `Started:`**, set `current`, set up the auto-resume loop, and begin. (This is the direct fresh-task path.)
4. **`$1` empty, exactly one plan exists** with `phase: planned`: transition it to `phase: executing`, **set `Executing: <UTC timestamp now>`**, set `current`, set up the loop, begin.
5. **`$1` empty, multiple planned plans exist**: ask the user which one via a **number picker** (AskUserQuestion) — one option per plan, labeled `<name>` with description `started <date> — <goal>`. Then execute the chosen plan (transition to executing, **set `Executing:`**, set current, loop, begin). This is the ONLY place `$iterate` asks a question, and it only happens on a human-typed no-arg `$iterate` with no executing plan.
6. **Neither `$1` nor any plan exists**: report "no plans yet — supply instructions or run $iterate-planner first" and stop. (Reporting is not the same as asking.)

Create `./.claude/iterate/` and `./.claude/iterate/plans/` if they don't exist.

## Auto-resume via Cron

<!-- codex-port: was Claude Code's `/loop` (ScheduleWakeup-based re-firing). Codex's confirmed close analog is CronCreate/CronList/CronDelete — recurring or one-shot 5-field cron jobs, session-only by default or durable to .codex/scheduled_tasks.json. One real gap: recurring Cron jobs auto-expire after 7 days, which /loop never did — see the caveat below. Verify exact tool call shape against current Codex docs before relying on this section as-written. -->

API errors, transient stalls, or hitting context limits will silently end a turn. The skill survives this by piggybacking on a Cron job, which fires `$iterate` on a fixed cadence.

- **On a fresh task or first execution of a `phase: planned` plan — flat or teamed** (you just wrote/transitioned the state file): call `CronCreate` for a recurring job that runs `$iterate` at most every 1 minute, as your *first* action. **1 minute is the maximum interval, for flat plans and teamed plans alike — never stretch it wider.** The subsequent firings have no `$1`, so they read the state file and continue. **Record the Cron job id in the state file** (a new `cron_job_id:` field alongside `running:`) — unlike `/loop`, which was a single implicit per-session toggle, Cron jobs are addressed by id and you need it later to cancel the right one with `CronDelete`.
- **Recurring Cron jobs auto-expire after 7 days — `/loop` had no such limit.** If a plan is still `phase: executing` when its Cron job is close to that window (track via the job's created time, or just re-arm defensively every few days on a plan that's clearly still alive), call `CronCreate` again for a fresh job and update `cron_job_id:`. A plan silently going quiet after ~a week with no error is the symptom of this being missed.
- **On a teamed plan, treat the automatic background-completion notification (if Codex provides one for this run's subagents — unconfirmed, see "Team dispatch" below) as a bonus, not a replacement for tight polling.** A scheduled Cron trigger isn't always able to react to a notification the instant it arrives the way an interactive session can — a 1-minute poll is what actually guarantees no long idle gap regardless of whether a notification exists or landed cleanly. A heartbeat tick that finds nothing new costs nothing; a missed notification with a wide poll interval costs real wall-clock time doing nothing. When in doubt, poll tighter, not looser.
- **On full success**: call `CronDelete` on the recorded `cron_job_id:` to cancel the recurring job. Then archive the state file.
- **On 5-cycle giveup**: call `CronDelete` on `cron_job_id:`. Leave the state file so the user can inspect and re-invoke manually after fixing the blocker. (Without this, the job would keep firing and hitting the same giveup forever.)
- **When the Cron job fires and a run is already in progress**: see the lock section below — the second run exits immediately.

The user can manually stop the resumption cycle any time by asking to cancel it (call `CronDelete` on `cron_job_id:`) — Codex's equivalent of Claude Code's Esc-during-wait interrupt, if one exists, is unconfirmed; don't assume it and rely on the explicit cancel path instead.

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

On every exit path (success, giveup, normal end-of-turn): set `running: false` and write before returning control.

## Team dispatch (parallel execution on teamed plans)

Most plans are flat — no `## Teams` section — and execute exactly as described below in "Execute the steps": one agent, one step at a time, in order. **Nothing in this section changes that path.** Team dispatch only activates when the plan file has `teamed: true` and a non-empty `## Teams` table (written by `$iterate-planner`'s teamify operation — see its SKILL.md for the table schema: Team / Steps / Focus / Depends on / Agent).

<!-- codex-port: this whole section was Claude Code's Agent tool (named tool_use blocks, run_in_background: true, automatic background-completion notifications). Codex's confirmed capability is a native subagent system, but the public interface is natural-language spawn instructions + /agent CLI commands, not a fixed tool-call schema — rewritten below accordingly. Whether a parent gets pushed a notification on completion, or must poll, is NOT confirmed; this section assumes polling is required and treats any notification as a bonus. Verify against https://developers.openai.com/codex/subagents and adjust before trusting this on a long unattended run. -->

When a plan IS teamed, on each `$iterate` entry (fresh dispatch, an automatic background-completion notification if one arrives, or a Cron resumption tick):

1. **Compute readiness.** A team is *ready* if every team named in its `Depends on` column has `Status: done` in its own row. `$iterate-planner` writes every row with `Status: pending` at teamify/auto-classify time — **that Status cell is the one thing `$iterate` is allowed to update** in the Teams table (`pending` → `in-progress` → `done` / `blocked (<reason>)`); never touch Team/Steps/Focus/Depends on/Agent, and never add or remove a row — that's `$iterate-planner`'s side of the table. Teams with no unmet dependencies and no prior dispatch are ready immediately.
2. **Unassigned steps** (step numbers not listed under any team) belong to the coordinator — that's you, the top-level `$iterate` turn. Execute them yourself, serially, following the normal "Execute the steps" procedure, at whatever point their step numbers fall relative to team dependencies (before dispatching teams that depend on them, interleaved otherwise).
3. **Spawn every currently-ready, not-yet-dispatched team as its own subagent, all in the same turn** — one plain-language spawn instruction per team ("spawn an agent to handle team `<name>`: ...", issued for every ready team together so they run concurrently), not sequentially one after another. Don't block this turn waiting on them to finish. Set that row's `Status` to `in-progress` immediately on dispatch, so a status report never has to say just "still going" — it can say *which* teams are running.
4. **Each team's spawn instruction must be fully self-contained** (a fresh subagent has no memory of this conversation or this plan file beyond what you put in the instruction). Include, verbatim:
   - **Its identity, as its own labeled statement — not something to infer from a file path.** State it plainly: "You are team: `<team-name>` (from this plan's Teams table). Your log file MUST be exactly `./.claude/iterate/plans/<name>.teams/<team-name>.log.md` — do not rename yourself, even if you'd naturally describe your own work differently." Confirmed live: a dispatched team named its own log file after its own description of the work instead of the Teams-table name (`app-macos` instead of `gui`) — the identity had only ever been implicit, embedded inside a path string it was told to write to, never stated as its own fact.
   - The plan's Goal.
   - This team's Steps + Validations (the exact Na/Nb pairs it owns — nothing from other teams).
   - The plan's global Constraints — **including any known baseline duration for a specific operation** that `$iterate-planner` folded in from the oracle (e.g., "compiling X normally completes in under 60s") — see "Know the baseline, don't guess it" below. If a Constraint gives a real number, that's the team's expectation, not something to estimate.
   - **The team writes ONLY to its own scoped log file (named above), never to the main plan file.** This is what makes concurrent dispatch safe: N subagents never touch the same file, so there's no write race for the coordinator to worry about.
   - **Mandatory progress checkins, real content, real tooling — not a guess.** Run any operation likely to take more than ~1 minute through `iterate-run` instead of invoking it bare: `iterate-run run --plan <plan> --team <team> --unit <step-id> -- <command...>`. It wraps the command, tees its output, ticks a heartbeat every 10s, and — critically — makes the *wake-up* decision itself: it stays silent on its own stdout for routine ticks, and only prints when something is actually worth reacting to (`ALERT stalled ...` after 6 quiet ticks / 60s of genuine inactivity, `RESUMED ...`, `DONE ... exit=0`, `FAILED ... exit=<code>`). Relay whatever `iterate-run` reports into your own outward checkin log at least once a minute — that's real observability, not a paraphrase. **If you are writing a new script/tool as part of this step** (not just invoking an existing one), have it emit `##ITERATE-PROGRESS## {"done":N,"total":M,"message":"..."}` lines as it works (a line per item or per batch) — `iterate-run` parses that exactly, instead of guessing at arbitrary output; a silent loop over thousands of items is exactly the black-box case this exists to prevent. If a Constraint carries a known duration for this exact operation (see "Know the baseline" below), that's real evidence — trust it over `iterate-run`'s own generic stall window. The team must also log a line every time it finishes a step, and `iterate-run status` (run from any directory, no plan file parsing needed) is always available as an independent cross-check of current state — yours or any other team's.
   - **Structured per-validation reporting, not just a final summary.** As each numbered Validation (Nb) is actually assessed — not reconstructed retroactively at the end — append a line to this team's own log: `##ITERATE-VALIDATION## {"step":N,"status":"met|partial|not-met","note":"one line, specific"}`. `met` = the validation's own wording is satisfied in full. `partial` = some of it is proven and the rest is either infeasible as written or genuinely not yet attempted — say which, specifically, in the note (e.g. "file push/pull and SSH login both proven; utmctl exec cannot return output on this host, no invocation satisfies that clause"). `not-met` = attempted and failed, or not yet attempted at all. This is what lets anything reading the plan — a dashboard, a status check, the next agent — see real per-step state instead of one blanket team-level status; a team can be `in-progress` overall while several of its own validations are already `met`.
   - A condensed restatement of `$iterate`'s own non-negotiable execution rules (see "Rules" below) — never ask a question, Nb is the contract and Na is a hint, pick the most reasonable default and log it, validation must exercise the system not just read code, 5-cycle cap per failing check, pre-existing breakage isn't a blocker, "more of the same shape" is never a stop reason.
   - The instruction to end its log file with exactly one terminal line when finished: `TEAM DONE: <one-line summary>` or `TEAM BLOCKED: <specific reason + what's needed>`.
   - Tell the subagent its own identity is `<plan-name>-<team-name>` (e.g. `owl-database`) and to state that label in any response — this is how you'll recognize which team a reply or completion belongs to, and how you address it later (via `/agent` or a direct message to that thread) if you need to check on it before re-dispatching (see step 6).
5. **Log the dispatch** in the main plan file's Status/Log: `dispatched teams: <names> (background, in progress)`. Update heartbeat, end the turn.
6. **Merge as soon as you know, not just on a poll.** Two ways you find out a team is done:
   - **Primary, if available: an automatic background-completion notification.** Whether a dispatched subagent's finish pushes a notification to the parent is unconfirmed for Codex (see the codex-port flag above) — if one does arrive, don't wait for the next Cron tick to notice; merge it right then, same steps as below.
   - **Fallback, and the one to actually rely on: the next Cron tick (every 1 minute)**, for the rare case a notification was missed (session restart, harness hiccup) — or the normal case if there's no notification at all. On every tick, check every outstanding team's (dispatched, not yet merged) scoped log file against three tiers, not two — silence up to a large threshold is not "nothing to do":
     - **Fresh** (a write within the last ~2 minutes, matching the team's own mandated ≤1-minute checkin cadence plus a small buffer — or, if the last line was a "starting: X, expect ~Nm" announcement backed by a real Constraint number, still within that N) → on track. Leave it, don't act.
     - **Overdue** (past the Fresh window, no terminal line) → **don't wait passively — ping it.** Send a lightweight status-check message by its dispatch name (`<plan-name>-<team-name>`) asking it to report progress. This is cheap (a heartbeat costs nothing) and catches a problem — or confirms a long operation is legitimately still running — well before it becomes a big silent gap. **A known baseline makes this sharper, not just faster:** if the last log line says "starting: run compile, expect ~60s" (a real Constraint-backed number) and it's now been 5 minutes, that's a strong, specific signal something's actually wrong — ping immediately, don't wait for a generic timer. Without a known baseline, ~10 minutes of total silence is the outer bound before pinging. If it responds, or the log gets a fresh write, treat it as fine and don't escalate further this tick.
     - **Stale** (a ping was already sent in a prior tick, and several minutes have passed with still no response and no fresh log write) → now, and only now, treat it as dead. Side-effecting work (writes, migrations, deploys) can't safely run twice — a second agent doing the same work could double-apply a change the first one already made, or corrupt a resource under concurrent access — so this tier is strictly downstream of Overdue, never a standalone timer of its own. Log "team `<name>` unresponsive after ping, treating as dead, re-dispatching" and dispatch a fresh one.
     - `TEAM DONE` / `TEAM BLOCKED` present → **merge**: copy the team's log content into the main plan file under a `### Team: <name>` heading in Decisions log / Status-Log, check off that team's step numbers in the main `## Steps` checklist, and set that row's `Status` cell to `done` or `blocked (<reason>)` — the only cell you touch. This merge is the ONLY thing that writes team content into the shared plan file, and only the coordinator does it — never a team subagent.
7. **Newly-ready teams** (all their dependencies just flipped to `Status: done`) get dispatched immediately after merging — right then, on the same notification or tick, not on the next cycle.
8. **Full-plan validation** (the "Validate" step below) only runs once every team AND every unassigned step is `Status: done`. Some teams done, others still in flight is a normal **end-of-turn, not complete** state — log it and let the Cron job continue; this is not a status-check menu, it's the existing "normal end-of-turn" allowance applied per-team.
9. **One blocked team does not block independent teams** — exactly like the existing "one blocked outcome does NOT block other outcomes" rule, just scoped to teams instead of goal-outcomes. Only report a hard stop when EVERY team (and every unassigned step) is either done or blocked, with at least one blocked — aggregate the blockers from each `TEAM BLOCKED` line into the single stuck-report (see "Report and either complete or stop" below).
10. **Any status report on a teamed plan (mid-run, when the user checks in, or an optional line at dispatch/merge time) must be structured, not "still going."** For every team currently `in-progress`, report: last log update (age), which steps are done vs. the one in flight, and a one-line gist of what's happening right now (pull it straight from the team's latest progress line — don't paraphrase into vagueness). **Explicitly flag any team in the Overdue tier or beyond** (see "Team dispatch" step 6) as `checking in` rather than silently folding it into the same bucket as a team that's actively logging on schedule — the whole point of the tiered staleness check is that quiet-but-under-some-large-threshold is not the same as "nothing to report." This is what makes team dispatch transparent and accountable instead of a silent black box between merges. Example shape:
    ```
    data — updated 1m ago, steps 3-5 done, step 6 in progress (FFVI link-group sweep)
    ui — updated 6m ago (checking in), steps 1-3 done, last seen: redeploying to re-verify a fix
    ```
    This is a report of real state pulled from the logs, not a status-check decision menu — it doesn't ask the user anything, it just tells them what's true right now. Producing it costs nothing extra: you already read these logs to check for terminal lines.

### Know the baseline, don't guess it

A fresh subagent has no memory of how long anything in this project normally takes — a compile that's run 5000 times and always finishes under 60 seconds looks, to a subagent seeing it for the first time, exactly like an unknown quantity that could reasonably take 10 minutes. That gap is what makes generic timers weak: they're either too loose for something normally fast (real problems hide inside the slack) or too tight for something normally slow (false alarms).

The fix is real data, not a better guess: if `./.claude/data/oracle.md` (or the global oracle) has a known duration for a specific operation — a build, a migration, a deploy step — `$iterate-planner`'s oracle merge (see its SKILL.md) folds it into that step's Constraints as a concrete number, e.g. `Context: the app-build compile normally completes in under 60s (even faster on a warm cache).` That Constraint flows into the team's prompt verbatim (item 4 above), so the team's pre-announcement line uses the real number instead of inventing one, and the coordinator's Overdue check compares against it directly instead of falling back to the generic window.

**When a team hits a real, unexplained deviation from a known baseline** (the compile that always takes under a minute is still running at 10x that with no error), that's a strong signal to actively investigate right then — check the process, look for a hang, don't just keep waiting — not something to shrug off because it's still short of some generic ceiling. It's also exactly the kind of fact worth an `/oracle harvest` at the end of the run, win or lose: either "confirmed: still under 60s" (reinforcing the baseline) or "found: now regularly takes N minutes because of X" (updating it) are both worth capturing so the next run starts smarter than this one did.

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
  - Global: `~/.claude/skills/oracle/known.md`
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

### 2. Write the state file

Write `./.claude/iterate/plans/<name>.md` (name from `iterate-run name next` for a direct fresh-task run; keep the existing name when transitioning a planned plan) with this schema:

```markdown
# Iterate Task — <short title>

name: <animal>
Started: <UTC timestamp>
Executing: <UTC timestamp>     # same instant as Started: on this direct fresh-task path — set once, never touch again
CWD: <pwd at first invocation>
phase: executing
running: <UTC timestamp>       # heartbeat — update at every step boundary

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

## Decisions log
(append-only. Each entry: timestamp + decision made + why.)

## Status / Log
(append-only. Each entry: timestamp + step + outcome / error / next attempt.)
```

If resuming, do not overwrite — append to Decisions log and Status / Log. Update the `running:` heartbeat as you work.

If transitioning from `phase: planned` (set by `$iterate-planner`): the Steps/Validation/Constraints are already there — just set `phase: executing`, **add an `Executing: <UTC timestamp now>` line** (this is the real execution-start marker the dashboard's "Running for" box reads — do NOT touch `Started:`, which stays as the original drafting time), take the lock, set up the Cron job (`CronCreate`, and record its id as `cron_job_id:`), and start. Do not re-parse from $1.

### 3. Execute the steps

**If the plan is teamed** (`teamed: true`, non-empty `## Teams`), go to "Team dispatch" above first — it handles dispatching ready teams and merging finished ones. Everything below in this section is what the coordinator uses for the plan's unassigned steps (if any), and is exactly what runs for the common case of a flat, un-teamed plan.

**Contract semantics:** for each numbered pair, the Step (Na) is *one suggested approach*; the Validation (Nb) is *the contract*. Treat Na as a starting hint, not a literal recipe. If Na's specific mechanism fails (tool absent, command syntax changed, auth rejected, host unreachable, etc.), **try other approaches that achieve the same Nb**. Examples:

- Step says "ranger issues token, explorer adds ranger remote" → if `incus remote add` rejects the token, try generating with `--public`, try `incus config trust add` with a pre-signed cert, try resetting trust on either side. Any path that ends with Nb's "incus remote list has a ranger / tls row" is acceptable.
- Step says "use `apt-get install foo`" → if the package name changed or the repo is missing, try `dnf`, try `snap`, try building from source. The contract is "foo binary exists and runs".
- Step says "ssh as travis" → if travis can't auth, try `root`, try a known fallback key, try installing your key via incusmagic. The contract is "I can run commands on the target".

Log every alternative attempt in **Status / Log**. Log the chosen mechanism in **Decisions log**. The user reviews the log to understand what actually happened versus what was suggested.

**Same streaming discipline as team dispatch applies here.** For any single operation likely to run more than ~1 minute (a build, a migration, a long test run), run it through `iterate-run run --plan <plan> --unit <step-id> -- <command...>` (no `--team`, since this is the coordinator's own unassigned step) instead of a blind blocking call, and log its `ALERT`/`DONE`/`FAILED` output into Status/Log as it happens rather than one entry after it finally returns — see "Know the baseline, don't guess it" under Team dispatch above, which applies to flat plans too: if a Constraint carries a known duration for the exact operation running, use that as the real expectation, and treat a genuine unexplained deviation from it as worth investigating immediately, not something to wait out.

Walk through `Steps` in order. For each step:

1. Figure out *how* to do it from current context. Read files, run commands, ssh wherever needed.
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
5. Mark the step done in the checklist when complete.

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
- Set `running: false` in `active.md`.
- **Add a `Finished: <UTC timestamp now>` line** (same `date -u +%Y-%m-%dT%H:%M:%SZ` format as `Executing:`) right before archiving — this is the real "done at" instant the dashboard's "Ran for" figure reads once archived. Without it, that figure falls back to the latest CONFIRMED activity span (hook/registry data), which can simply not exist for a project with neither wired up — confirmed live: a flat plan showed "Running for 0s" despite a correct `Executing:`, because there was no activity data to compute a span against at all. Set once, never touched again.
- Call `CronDelete` on `cron_job_id:` to cancel the auto-resume job.
- Move `./.claude/iterate/plans/<name>.md` to `./.claude/iterate/archive/<UTC-timestamp>-<name>-done.md`. If `current` pointed at this plan, repoint it to the sole remaining plan (if exactly one) else clear it. If the plan was teamed, also move `./.claude/iterate/plans/<name>.teams/` to `./.claude/iterate/archive/<UTC-timestamp>-<name>-done.teams/` (the per-team logs are already merged into the archived plan file — this just keeps the raw team logs around for audit, don't leave the working `.teams/` dir behind).
- Report a 3-5 line summary: goal, what was done, validation results, time taken. On a teamed plan, name which teams ran (and, if any ran concurrently, say so — that's the payoff of teaming).
- **Suggest `$oracle harvest`** to the user — one line at end of report: "If anything in this run is worth remembering for next time, run `$oracle harvest`." Don't auto-invoke; oracle harvesting is opt-in. <!-- codex-port: requires the oracle skill also ported via /skill-2-codex -->

**On stuck (5-cycle cap hit AND every other outcome also stuck — genuinely no forward motion possible), OR every validation is met except one clause that only a human can clear** (billing, an external approval, physical/credential access no agent has — a wall, not a failing check):
- Set `running: false`. Update `active.md`: mark which steps completed, which validation checks pass, which fail and why.
- Write the "Next attempt" hint in the ONE standardized shape the dashboard tool actually parses — this was previously freeform prose per-run, which the dashboard couldn't recognize at all (it just kept reading as plain `executing` no matter how done the plan actually was):
  - At the very top of the file, above the frontmatter, a blockquote banner: `> **Next attempt (one operator action):** <what's blocking, one or two sentences> <the exact command(s) to run once it's cleared>`.
  - In the frontmatter, `status: blocked-on-operator: <one-line reason>` — that exact `blocked-on-operator` prefix is the literal string the dashboard matches on; don't paraphrase it into "waiting on user" or similar. This is IN ADDITION to `phase:`, not a replacement — leave `phase: executing` as-is.
- Call `CronDelete` on `cron_job_id:` to cancel the job. (Without this, it would fire forever and re-hit the same giveup.)
- Stop. Report ONE blocker reason — the specific check that failed 5 times (or the specific operator-only clause) AND why no other outcome could absorb attention — plus what specific operator action would unblock. **Do NOT write a menu of "things the user could do next." Do NOT list "(a) ... (b) ..." options. Do NOT frame remaining work as choices.** One blocker, one ask, done. On a teamed plan where multiple teams are blocked, aggregate: report the done/blocked status of every team in one line each, then the single most-actionable next operator step (usually whichever blocker, once fixed, unblocks the most dependent teams).
- Do **not** archive — leave `active.md` in place so the user can read what happened and re-invoke fresh after fixing the blocker.

Acceptable stuck-report shape:
```
Blocked.
Last green: Outcome 1 steps 1-3, Outcome 2 fully done.
Hard blocker: Outcome 3 step 5b (`bao auth list` returns 403) — failed 5 cycles, last error: "permission denied: kubernetes-kelwin1 backend not registered".
Need: someone with OpenBao root token to run `bao auth enable -path=kubernetes-kelwin1 kubernetes`.
Run `$iterate` again once that's done.
```

UNACCEPTABLE stuck-report shape (this is the cowardly-stop pattern — never write this):
```
You can $iterate again to drive (a) the remaining chart conversions, or address (b) the pre-existing issues separately and re-invoke.
```

**On normal end-of-turn (work not yet complete, no giveup):**
- Set `running: false` (lock released).
- Leave `active.md` and the Cron job intact. The next tick will resume from state.
- Brief status line to the user is OK but not required.

## Rules (hard, non-negotiable)

1. **Never ask the user a clarifying question during execution.** If you need a decision, pick the most reasonable one and log it in the Decisions log. The user can read the log later.
2. **Never stop just because something is uncertain.** Pick a path, try it, log, continue. "Uncertain" is not the same as "stuck".
3. **Never declare success without running every validation check.** Self-validate, every time.
4. **Always update `active.md` before doing anything destructive** (delete, overwrite, force-push, restart service). The state file is the resumption contract; don't violate it.
5. **Never replace `active.md` without archiving first.** Old state goes to `./.claude/iterate/archive/<UTC-timestamp>.md`.
6. **Logging is mandatory.** Every decision and every step outcome must land in the appropriate section. Future-you (next `$iterate` call) reads the log to know what's already done.
7. **Don't loop forever.** 5 cycles per failing validation check, then stop and report. On stop, cancel the Cron job (`CronDelete` on `cron_job_id:`) so it doesn't keep re-running into the same wall.
8. **Respect the user's constraints absolutely.** Constraints listed in the state file override your own judgment.
9. **Set up a resumption loop on first run, cancel it on terminal exit (success or giveup). 1 minute is the maximum interval — flat or teamed, no exceptions.** Team-completion notifications can accelerate a merge when they land cleanly, but never widen the poll interval on the assumption they will — a stale-but-cheap heartbeat beats a wide gap of real inactivity. This is what makes the skill survive API errors, stalled sessions, and missed notifications alike.
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
    - **End-of-turn (mid-run)**: brief status line is OPTIONAL; the Cron job resumes it. Don't write a wrap-up that reads like the run is over when it isn't.
15. **Pre-existing failures are NOT blockers — flag and continue.** If validation catches a broken pod / app / state that pre-dates this run (check pod `creationTimestamp`, last-restart age, Application `lastTransitionTime`, prior Decisions log entries), record it once in Status/Log as `pre-existing: <name> broken since <date> — not caused by this work` and TREAT THE CHECK AS GREEN for blocking purposes. Don't count it toward the 5-cycle cap. Don't stop on it. The user can address pre-existing breakage separately; THIS run keeps going.
16. **One blocked outcome does NOT block other outcomes.** Multi-outcome plans (e.g., "do X on cluster A, then B, then C, then retire D") have independently-progressable outcomes. If outcome 1 is stuck at a hard blocker, MOVE TO outcome 2 immediately. Work outcomes in parallel where ordering allows. Only report "stuck" when EVERY outcome is independently stuck on a hard blocker. Acceptable transition: "Outcome 1: blocked on X (operator needed); proceeding to outcome 2." Then keep going.
17. **"Straightforward more work" / "more of the same shape" is NEVER a stop reason.** If the remaining work is "6 more MRs of the same shape as the one that just succeeded," DO THOSE 6 MRS. Don't ask if the user wants you to continue. Don't summarize what's left as if presenting options. Repetitive work is exactly what `$iterate` is FOR — that's why you're called the iterator. The only acceptable stops are: full success, all-outcomes-stuck-on-hard-blocker, or 5-cycle giveup on a specific failing check.
18. **Un-teamed plans are unaffected by any of this.** Team dispatch only activates when `teamed: true` and `## Teams` is non-empty. A flat plan runs exactly as it always has — single agent, serial steps, no scoped log files, no dispatch logic. Don't go looking for teams to dispatch when there's no Teams table.
19. **A team subagent never writes to the main plan file.** It writes only to its own `./.claude/iterate/plans/<name>.teams/<team>.log.md`. Only the coordinator (the top-level `$iterate` turn) merges team logs into the shared plan file, and only after seeing a `TEAM DONE`/`TEAM BLOCKED` terminal line. This is the entire safety mechanism against concurrent-write races between teams — don't shortcut it by having a team edit the plan file directly, even "just this once."
20. **Never dispatch a team whose dependencies aren't `status: done`.** Check the Teams table's `Depends on` column before every dispatch, every tick. A team becomes eligible the moment its dependencies clear — dispatch it that same tick, don't wait for an extra loop cycle.
21. **A `TEAM BLOCKED` team is a per-team giveup, not a whole-plan giveup.** Apply rule 16 (one blocked outcome doesn't block others) at the team level: keep dispatching and progressing every team that isn't itself blocked. Only reach the whole-plan "stuck" report (rule 14/16's stuck path) when every team is done-or-blocked and at least one is blocked.

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
- Transitions `owl` to `phase: executing`, sets `Executing: <now>`, takes the lock, calls `CronCreate` for a 1-minute recurring `$iterate` job and records its id as `cron_job_id:` (1 minute max, teamed or not — notifications help when they land but never widen the poll interval).
- Team dispatch: `deploy` has no unmet dependencies → ready. `link-tree` depends on `deploy`, which isn't done yet → not ready.
- Dispatches one Agent named `owl-deploy` (steps 2,4 + Goal + Constraints + its scoped log path `./.claude/iterate/plans/owl.teams/deploy.log.md` + the mandatory-checkin instruction, background). Sets `deploy` row `Status: in-progress`. Logs "dispatched teams: deploy (background, in progress)". Ends the turn.
- Mid-flight, if the user checks in: reads `deploy.log.md`'s latest checkin line (not just waiting for a terminal line) and reports it plainly — e.g. "deploy — updated 4m ago, container built, running smoke test now." No terminal line yet, so nothing to merge; this is just reading the log, not a poll tick.
- `owl-deploy` finishes → **automatic completion notification** arrives (no need to wait for the next loop tick). `deploy.log.md` ends with `TEAM DONE: metrics-service deployed and smoke-tested, curl https://metrics.gravhl.com/health returns 200`. Merges that into `owl.md`'s Status/Log under `### Team: deploy`, checks off steps 2 and 4, sets the `deploy` row `Status: done`.
- Same turn: `link-tree`'s dependency (`deploy`) just cleared → now ready. Dispatches `owl-link-tree` immediately (steps 3,5 + same Goal/Constraints + its own scoped log path), sets `Status: in-progress`. Ends the turn.
- `owl-link-tree` finishes → notification arrives, `link-tree.log.md` ends with `TEAM DONE: link tree updated, verified live in browser`. Merges it, checks off steps 3 and 5, sets `link-tree` `Status: done`.
- All teams done, no unassigned steps → runs full-plan Validation once more across everything. All green.
- Archives `owl.md` and `owl.teams/`, invokes `/loop` (no args) to cancel, reports: "✓ owl done — 2 teams (deploy, link-tree ran sequentially due to dependency), 4 steps, all validations green."
