---
name: iterate
description: Use when given a multi-step task with validation criteria and asked to execute autonomously until done. The skill does NOT ask the user clarifying questions mid-run; it picks the most reasonable interpretation, executes, validates, loops, solves its own blockers, and only returns control when validation passes or the run is truly stuck. Re-invokable — running `/iterate` again resumes from the saved state file. Triggers on "/iterate", "iterate until done", "keep going until X", "work this until validation passes".
argument-hint: <paragraph describing the work to do AND how to validate success>
disable-model-invocation: true
---

# /iterate — Run a task to completion without interrupting the user

The user invoked this skill because they're tired of being interrupted by clarifying questions. The whole point: **make the call yourself, log it, continue**. Only return control when the validation criteria they provided are all green, or when you've truly exhausted reasonable attempts.

## Named plans & state files (this is how resumption works)

Plans are **saved, animal-named, and persistent**. Each plan is one file:

    ./.claude/iterate/plans/<name>.md        (project-local, relative to cwd)

`<name>` is a random common animal (dog, cat, fox, owl, elk, wren, …). `./.claude/iterate/current` points at the **current** plan. **The plan file** below always means the plan being executed (resolved from `$1`'s name, or `current`, or the sole executing/only plan). Older wording in this doc may say `active.md` — read it as the resolved plan file.

Each plan has a top-level `phase:` field:

- `phase: planned` — written by `/iterate-planner`, waiting for execution to start.
- `phase: executing` — execution is in flight (or has been; check `running:` for whether a run is currently live).

**Legacy migration (do silently on first touch):** if `./.claude/iterate/active.md` exists and `plans/` does not, move it to `plans/<name>.md` (assign an animal name, add a `name:` field), write `current`, delete `active.md`. Create `./.claude/iterate/plans/` if it doesn't exist.

### Entry decisions on `/iterate`

Resolve in this order:

1. **A plan is already `phase: executing`** (scan `plans/`): resume THAT plan from its "Status / Log", honoring the concurrency lock. This takes precedence over everything below — it's what makes the `/loop` re-fires (which pass no `$1`) continue the live run instead of prompting. If several are somehow executing, pick the one named by `current`, else the most-recently-heartbeated.
2. **`$1` names an existing plan** (`$1` exactly matches a `plans/<name>.md`, e.g. `/iterate dog`): set `current` = that plan; if `phase: planned` → transition to `phase: executing`, set up the auto-resume loop, begin; if already executing → resume it.
3. **`$1` is substantive task text** (a paragraph/steps, not a bare existing name): create a **new** animal-named plan with `phase: executing`, set `current`, set up the auto-resume loop, and begin. (This is the direct fresh-task path.)
4. **`$1` empty, exactly one plan exists** with `phase: planned`: transition it to `phase: executing`, set `current`, set up the loop, begin.
5. **`$1` empty, multiple planned plans exist**: ask the user which one via a **number picker** (AskUserQuestion) — one option per plan, labeled `<name>` with description `started <date> — <goal>`. Then execute the chosen plan (transition to executing, set current, loop, begin). This is the ONLY place `/iterate` asks a question, and it only happens on a human-typed no-arg `/iterate` with no executing plan.
6. **Neither `$1` nor any plan exists**: report "no plans yet — supply instructions or run /iterate-planner first" and stop. (Reporting is not the same as asking.)

Create `./.claude/iterate/` and `./.claude/iterate/plans/` if they don't exist.

## Auto-resume via `/loop`

API errors, transient stalls, or hitting context limits will silently end a turn. The skill survives this by piggybacking on `/loop`, which fires `/iterate` on a fixed cadence.

- **On a fresh task or first execution of a `phase: planned` plan** (you just wrote/transitioned the state file): invoke `/loop 1m /iterate` as your *first* action. This schedules a re-invocation every 1 minute. The subsequent firings have no `$1`, so they read the state file and continue.
- **On full success**: invoke `/loop` (no arguments) to cancel the loop. Then archive the state file.
- **On 5-cycle giveup**: invoke `/loop` (no arguments) to cancel. Leave the state file so the user can inspect and re-invoke manually after fixing the blocker. (Without this, the loop would keep firing and hitting the same giveup forever.)
- **When the loop fires and a run is already in progress**: see the lock section below — the second run exits immediately.

The user can manually stop the loop any time with `/loop` (no args) or by pressing Esc during the inter-tick wait.

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

## Steps

### 1. Parse the instructions

From $1, extract:

- **Goal** — one sentence paraphrase of what the user wants.
- **Numbered steps** — their explicit task list (preserve the user's numbering).
- **Validation criteria** — anything they marked as "validate", "success when", "done when", "before moving on". These are the gate; never declare success without checking them.
- **Constraints** — explicit "don't do X", "must use Y", "always Z" rules.

If the user's instructions are vague, **paraphrase the most reasonable interpretation** into the steps section yourself. Do not ask. Log the interpretation in the Status section so the user can see it if they want.

### 1.5. Oracle fallback (for direct /iterate invocations without /iterate-planner)

If you reached this step via a fresh `/iterate <task>` (i.e. `$1` non-empty, no prior planning happened):

- Build the oracle buzzword index by reading the **index sections** of both stores:
  - Project: `./.claude/data/oracle.md`
  - Global: `~/.claude/skills/oracle/known.md`
- Scan your interpreted task (Goal + Steps + the user's `$1`) for buzzword matches against the index. Case-insensitive substring; allow plural / verb forms.
- For each matched buzzword, read its full 5W+H entry from the right store (project wins on conflict).
- Fold the entry into Steps / Validations / Constraints using the same rules as `/iterate-planner`:
  - **How** → new Steps + paired interactive Validations.
  - **Where** → Constraints with `Context:` prefix.
  - **When** (when-not-to-use) → Constraints.
  - **Who** (operator-specific actions) → Constraints.
- Log each folded buzzword in the Decisions log: `oracle: matched "<buzzword>" → added N steps, M constraints`.
- If no buzzwords match: log `oracle: no matches in [list of stores read]`. Don't pad with irrelevant entries.
- If `planner: iterate-planner` is set in the state file (the plan was built oracle-aware already), **skip this entire step** — trust the existing plan.

If neither oracle store exists, skip silently.

### 2. Write the state file

Write `./.claude/iterate/plans/<name>.md` (assign a fresh random animal name for a direct fresh-task run; keep the existing name when transitioning a planned plan) with this schema:

```markdown
# Iterate Task — <short title>

name: <animal>
Started: <UTC timestamp>
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

If transitioning from `phase: planned` (set by `/iterate-planner`): the Steps/Validation/Constraints are already there — just set `phase: executing`, take the lock, set up `/loop`, and start. Do not re-parse from $1.

### 3. Execute the steps

**Contract semantics:** for each numbered pair, the Step (Na) is *one suggested approach*; the Validation (Nb) is *the contract*. Treat Na as a starting hint, not a literal recipe. If Na's specific mechanism fails (tool absent, command syntax changed, auth rejected, host unreachable, etc.), **try other approaches that achieve the same Nb**. Examples:

- Step says "ranger issues token, explorer adds ranger remote" → if `incus remote add` rejects the token, try generating with `--public`, try `incus config trust add` with a pre-signed cert, try resetting trust on either side. Any path that ends with Nb's "incus remote list has a ranger / tls row" is acceptable.
- Step says "use `apt-get install foo`" → if the package name changed or the repo is missing, try `dnf`, try `snap`, try building from source. The contract is "foo binary exists and runs".
- Step says "ssh as travis" → if travis can't auth, try `root`, try a known fallback key, try installing your key via incusmagic. The contract is "I can run commands on the target".

Log every alternative attempt in **Status / Log**. Log the chosen mechanism in **Decisions log**. The user reviews the log to understand what actually happened versus what was suggested.

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
- Invoke `/loop` (no args) to cancel the auto-resume loop.
- Move `./.claude/iterate/plans/<name>.md` to `./.claude/iterate/archive/<UTC-timestamp>-<name>-done.md`. If `current` pointed at this plan, repoint it to the sole remaining plan (if exactly one) else clear it.
- Report a 3-5 line summary: goal, what was done, validation results, time taken.
- **Suggest `/oracle harvest`** to the user — one line at end of report: "If anything in this run is worth remembering for next time, run `/oracle harvest`." Don't auto-invoke; oracle harvesting is opt-in.

**On stuck (5-cycle cap hit AND every other outcome also stuck — genuinely no forward motion possible):**
- Set `running: false`. Update `active.md`: mark which steps completed, which validation checks pass, which fail and why. Save a "Next attempt" hint at the top.
- Invoke `/loop` (no args) to cancel the loop. (Without this, /loop would fire forever and re-hit the same giveup.)
- Stop. Report ONE blocker reason — the specific check that failed 5 times AND why no other outcome could absorb attention — plus what specific operator action would unblock. **Do NOT write a menu of "things the user could do next." Do NOT list "(a) ... (b) ..." options. Do NOT frame remaining work as choices.** One blocker, one ask, done.
- Do **not** archive — leave `active.md` in place so the user can read what happened and re-invoke fresh after fixing the blocker.

Acceptable stuck-report shape:
```
Blocked.
Last green: Outcome 1 steps 1-3, Outcome 2 fully done.
Hard blocker: Outcome 3 step 5b (`bao auth list` returns 403) — failed 5 cycles, last error: "permission denied: kubernetes-kelwin1 backend not registered".
Need: someone with OpenBao root token to run `bao auth enable -path=kubernetes-kelwin1 kubernetes`.
Run `/iterate` again once that's done.
```

UNACCEPTABLE stuck-report shape (this is the cowardly-stop pattern — never write this):
```
You can /iterate again to drive (a) the remaining chart conversions, or address (b) the pre-existing issues separately and re-invoke.
```

**On normal end-of-turn (work not yet complete, no giveup):**
- Set `running: false` (lock released).
- Leave `active.md` and the `/loop` schedule intact. The next loop tick will resume from state.
- Brief status line to the user is OK but not required.

## Rules (hard, non-negotiable)

1. **Never ask the user a clarifying question during execution.** If you need a decision, pick the most reasonable one and log it in the Decisions log. The user can read the log later.
2. **Never stop just because something is uncertain.** Pick a path, try it, log, continue. "Uncertain" is not the same as "stuck".
3. **Never declare success without running every validation check.** Self-validate, every time.
4. **Always update `active.md` before doing anything destructive** (delete, overwrite, force-push, restart service). The state file is the resumption contract; don't violate it.
5. **Never replace `active.md` without archiving first.** Old state goes to `./.claude/iterate/archive/<UTC-timestamp>.md`.
6. **Logging is mandatory.** Every decision and every step outcome must land in the appropriate section. Future-you (next `/iterate` call) reads the log to know what's already done.
7. **Don't loop forever.** 5 cycles per failing validation check, then stop and report. On stop, cancel the `/loop` so it doesn't keep re-running into the same wall.
8. **Respect the user's constraints absolutely.** Constraints listed in the state file override your own judgment.
9. **Set up `/loop 1m /iterate` on first run, cancel it on terminal exit (success or giveup).** This is what makes the skill survive API errors and stalled sessions.
10. **Honor the concurrency lock.** If `running:` is fresh, exit silently — don't double-run.
11. **Na is a hint; Nb is the contract.** If a step's described mechanism doesn't work, find another path that meets the validation. Only after exhausting reasonable alternatives (and hitting the 5-cycle cap per failing validation) do you give up. Never treat the step's wording as a constraint.
12. **Validation requires real execution.** Never declare a check green on the basis of code inspection alone. The validation must actually run/load/curl/click the thing being changed. If the plan's Nb says "tests pass", you ALSO load the page / hit the endpoint / run the command end-to-end before marking it done. Log the added execution in the Decisions log.
13. **Respect the oracle if it exists.** `./.claude/data/oracle.md` contains project-specific lessons. On direct `/iterate` (no prior plan), read it during Step 1.5 and merge its rules. On `phase: planned` with `planner: iterate-planner`, trust that the oracle was already applied.
14. **NEVER produce a status check or "Either/Or" decision menu in any report.** Mid-run, end-of-turn, on giveup, on success — none of these allow:
    - "You can /iterate again to drive (a) X or (b) Y"
    - "(a) I can continue with; (b) needs operator context I don't have"
    - "Stopped because: <list of unfinished things you could just keep doing>"
    - Any framing that asks the user to pick between conservative-vs-aggressive paths
    The user invoked `/iterate` to iterate, not to be polled. Allowed reports:
    - **Success**: 3-5 line summary of what's done.
    - **Hard blocker** (only): "blocked: <specific reason — credential / external service down / 5-cycle giveup with last error>; what's needed: <specific operator action>". One reason. No menu.
    - **End-of-turn (mid-run)**: brief status line is OPTIONAL; the `/loop` resumes it. Don't write a wrap-up that reads like the run is over when it isn't.
15. **Pre-existing failures are NOT blockers — flag and continue.** If validation catches a broken pod / app / state that pre-dates this run (check pod `creationTimestamp`, last-restart age, Application `lastTransitionTime`, prior Decisions log entries), record it once in Status/Log as `pre-existing: <name> broken since <date> — not caused by this work` and TREAT THE CHECK AS GREEN for blocking purposes. Don't count it toward the 5-cycle cap. Don't stop on it. The user can address pre-existing breakage separately; THIS run keeps going.
16. **One blocked outcome does NOT block other outcomes.** Multi-outcome plans (e.g., "do X on cluster A, then B, then C, then retire D") have independently-progressable outcomes. If outcome 1 is stuck at a hard blocker, MOVE TO outcome 2 immediately. Work outcomes in parallel where ordering allows. Only report "stuck" when EVERY outcome is independently stuck on a hard blocker. Acceptable transition: "Outcome 1: blocked on X (operator needed); proceeding to outcome 2." Then keep going.
17. **"Straightforward more work" / "more of the same shape" is NEVER a stop reason.** If the remaining work is "6 more MRs of the same shape as the one that just succeeded," DO THOSE 6 MRS. Don't ask if the user wants you to continue. Don't summarize what's left as if presenting options. Repetitive work is exactly what `/iterate` is FOR — that's why you're called the iterator. The only acceptable stops are: full success, all-outcomes-stuck-on-hard-blocker, or 5-cycle giveup on a specific failing check.

## Example trigger

> "I want you to do the following: 1. connect polaris2 to the polaris/polaris3 cluster. 2. Move all existing polaris2 vms over to polaris. 3. Move all existing polaris3 vms over to polaris. 4. Validate the following before moving on: 1 and 2 have no incus vms or containers. 5. Declare success. /iterate"

What the skill does:
- Parses: 3 action steps + 1 validation check ("polaris2 and polaris3 have no incus vms or containers")
- Writes state file
- Executes step 1 (cluster join), then 2 (move polaris2 vms), then 3 (move polaris3 vms)
- Runs `incus list` on polaris2 and polaris3 to verify both are empty
- If something fails partway (auth error, network blip, VM move stuck): retries, picks alternatives, logs, doesn't ping the user
- If everything passes: archives state, reports "✓ cluster joined, 7 vms migrated, polaris2 & polaris3 empty (verified via incus list). Done."
- If stuck after 5 cycles on validation: stops, leaves state file, reports what's done + what's blocking + last error
