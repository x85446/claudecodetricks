---
name: iterate-plan
description: Use when the user wants to formalize a plan into the iterate skill's structured pair-format (1a task / 1b validation) BEFORE autonomous execution begins. Triggers on "/iterate-plan", "plan this for iterate", "give me an iterate plan", "restate the plan for iterate". Output is a structured plan written to ./.claude/iterate/active.md (phase: planned). The user reviews, optionally refines in natural conversation, and types /iterate to kick off execution.
argument-hint: <optional context, e.g. "restate the plan from above" or "plan: 1. do X, 2. validate Y">
disable-model-invocation: true
---

# /iterate-plan — Build the plan, don't execute

Sister skill to `/iterate`. **This skill plans, it does not execute.** It writes a structured plan to `./.claude/iterate/active.md` with `phase: planned`, prints the plan back to the user in paired 1a/1b format, and waits for either natural-language refinements or the user typing `/iterate` to kick off execution.

It does **not** set up `/loop`. It does **not** take the running lock. It does **not** execute anything.

## When to use

- The user is in a conversation where a plan has been discussed and wants it formalized into the iterate state file.
- The user types `/iterate-plan` with brief instructions (often just "restate the plan" or "plan it with these tweaks").
- The user wants a chance to review and refine before committing to autonomous execution.

## Steps

### 1. Identify the plan source

- If `$1` contains substantive instructions (numbered steps, validation criteria): use `$1` as the source.
- If `$1` is brief or refers to recent context (e.g. "restate the plan", "with these changes: …"): read the recent conversation. The plan should already be present in some form — your job is to formalize it into the schema.

### 2. Check the state file

- If `./.claude/iterate/active.md` exists with `phase: executing`: **STOP**. Report "an active iterate task is in flight; let it finish or use `/iterate` to resume it before re-planning." Do not modify the file.
- If `phase: planned`: treat as a **refinement** — preserve `Started`, `CWD`, `Goal` (unless the user is changing it), update the Plan/Constraints sections.
- If state doesn't exist: write fresh.

### 3. Write the plan

Schema for `./.claude/iterate/active.md`:

```markdown
# Iterate Task — <short title>

Started: <UTC timestamp> (planned)
CWD: <pwd>
phase: planned
running: false

## Goal
<one sentence>

## Steps
1. <task>
2. <task>
...

## Validation
1. <how to verify step 1 is done — concrete, runnable assertion>
2. <how to verify step 2 is done>
...

## Constraints
- <rule>

## Decisions log
(empty until execution)

## Status / Log
(empty until execution)
```

Storage uses two parallel numbered lists (Steps + Validation, indexed 1:1). This is the same schema `/iterate` already reads — the only new field is `phase: planned`.

### 4. Present the plan to the user

Print the plan in **paired 1a/1b format** for readability:

```
**Plan ready** — ./.claude/iterate/active.md (phase: planned)

**Goal:** <goal>

1a. <step 1>
1b. <validation 1>

2a. <step 2>
2b. <validation 2>

...

**Constraints:** <list, if any>

Want changes, or type `/iterate` to execute?
```

### 5. Handle refinements

If the user responds with changes in natural conversation (e.g. "change 1a target to ranger and verify ranger has access to .28.x subnet first"), update `active.md` in place and re-print the full plan. Keep `phase: planned`. Don't archive — overwrite.

Each refinement round = one full re-print, so the user always sees the current plan state.

## Rules (hard)

1. **Never execute the plan.** Only `/iterate` does that. Your responsibility ends at writing + presenting.
2. **Never set up `/loop` or take the `running:` lock.** Those happen at `/iterate` execution time.
3. **Pair every step (Na) with a validation (Nb).** If the user didn't specify a validation, infer the most reasonable one (a runnable command + expected output) and note "(validation inferred)" so they can override.
4. **Re-presenting after a refinement is fine** — overwrite `active.md` in place, don't archive.
5. **If `active.md` is `phase: executing`**, never modify — stop and inform the user.
6. **Don't be cute about validations.** Each Nb must be a concrete, observable check (a command + expected output, a file existing, a count, a service responding). "Looks good" is not a validation.
6a. **Interactive testing mandate.** Validations must EXERCISE the system, not just READ the code. If a change touches:
    - a **UI** → the validation must include actually loading the page in a browser and clicking through the changed flow (not "the JSX looks correct").
    - an **API endpoint** → the validation must include a real `curl` / request hitting the running service with realistic input (not "the handler signature looks right").
    - a **CLI command** → the validation must include actually running the command end-to-end (not "the flag parsing looks right").
    - a **migration / script** → the validation must include executing it against a real (test or local) target (not "the SQL parses").
    "Static code review passes" / "tests pass" alone is NOT validation when the change ships behavior. The execution tier (`/iterate`) enforces this, but the plan must SPECIFY the interactive check — don't punt it as a vague "verify the feature works".
7. **NEVER write "STOP", "abort", "give up", "halt", or "manual intervention required" into any step or validation.** The whole point of `/iterate` is autonomous recovery — it exhausts alternatives, it doesn't bail at the first sign of trouble. Validations describe the desired post-condition only. Recovery from a failed validation is `/iterate`'s job at execution time, not yours.
8. **If a step's natural form is "check X, and if missing, halt" — rewrite it as a productive step.** Examples:
   - Bad: "1a. Verify bridge exists. 1b. `incus network list` shows it; STOP if absent."
   - Good: "1a. Ensure IZUMA bridge exists on ranger; create it from the existing template if missing. 1b. `incus network list` shows IZUMA bridge AND ranger can ping 172.29.28.1."
   The plan should describe what we want to be TRUE at the end of the step. How to achieve it (including creation, repair, retries, alternatives) is `/iterate`'s problem.
9. **Pre-flight checks become first-class steps with creation/repair built in.** Don't separate "check" from "fix". The plan assumes everything will be made true; `/iterate` will solve how.
10. **If after rewriting a step still requires the user to act (e.g., physical hardware change, billing decision), say so explicitly in Constraints** — not as a stop-mid-flight instruction. `/iterate` will surface it during execution as a giveup with context, and the user re-invokes after handling it.
11. **NEVER produce a status check, blocker report, or "either/or" decision menu in place of a plan.** When invoked, your ONLY output is: the paired 1a/1b plan + "Want changes, or type `/iterate` to execute?" prompt. If you catch yourself writing "Honest blocker", "To proceed", "Either ... Or ...", "this is your status check", or asking the user to pick between conservative-vs-aggressive paths — **STOP and rewrite as a plan**. Pick the most aggressive path consistent with the user's stated intent and commit to it. The user invoked you to plan; they did NOT ask for a decision menu.
12. **Risk acceptance is standing.** If the user has said (now or earlier in this conversation) "this isn't production", "accept the risk", "do it everywhere", "switching it out is fine", "proceed", or equivalent — treat that as a permanent setting for the planning scope. Do NOT re-prompt for risk acceptance per-step. Do NOT surface risk as a reason to ask the user to choose between two paths. Plan as if the risk is accepted.
13. **Rollback in a plan is never terminal.** If a step needs rollback on failure, the validation MUST also include "then retry, up to N times, until success." Never write "rolled back per X and stops" or any phrasing that lets the iterator stop at the first failure. Recovery is `/iterate`'s job — your plan describes the desired end state, not the give-up condition.
14. **Commit to the full goal, not a one-item pilot — unless the user explicitly asked for a pilot.** If the user said "port everything" or "do the whole sweep," plan to do the whole sweep. Don't downgrade to "let's try one and check in." That downgrade IS the status-check failure mode dressed up as caution.
15. **Scope validations to "caused by this work," not "all global state."** Broad checks like `kubectl get pod -A | grep -v Running | wc -l == 0` catch pre-existing failures and will read as "blocked" when they shouldn't. Prefer scoped checks: "pods in the changed namespaces are Running", "Applications touched by this run are Synced", or "no NEW non-Running pods compared to baseline captured at run start." If the goal genuinely IS cluster-wide health, say so explicitly in the Goal section so the executor knows pre-existing failures are in-scope.

## Example

User in conversation about a polaris/explorer cluster join. They type:

```
/iterate-plan restate the plan
```

You read the recent context, write the plan to `active.md`, and print:

```
**Plan ready** — ./.claude/iterate/active.md (phase: planned)

**Goal:** Join explorer into the polaris incus cluster without losing johnson-warp.

1a. Migrate johnson-warp polaris ← explorer (~2 GB, brief downtime).
1b. `incus list johnson-warp` on polaris returns it RUNNING with expected IP.

2a. Wipe leftover explorer remote/trust on polaris.
2b. `incus remote list` on polaris does not contain "explorer".

3a. On explorer: set core.https_address = explorer.johnson.gravhl.lan:8443.
3b. `incus config get core.https_address` on explorer returns the expected value.

4a. On polaris: `incus cluster add explorer` and capture the join token.
4b. Token output is non-empty and stored in state for step 5.

5a. On explorer: `incus admin init --preseed` with the token + cluster.https_address.
5b. `incus cluster list` on polaris shows polaris + explorer, both ONLINE.

6a. Verify no polaris2/polaris3 ghost members remain.
6b. `incus cluster list` shows exactly 2 members.

Want changes, or type `/iterate` to execute?
```

User responds: "change 1a target to ranger; first verify ranger has access to .28.x subnet."

You update `active.md`: insert a new 1a check for ranger's subnet access, change the old 1a's target. Re-print the full updated plan. Wait for next round or `/iterate`.
