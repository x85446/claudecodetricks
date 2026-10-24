---
name: iterate-triage
description: Walk up to a stale terminal and find out what's going on in one short answer. Reads the real state — plans, branch, uncommitted work, blockers — and reports only what is broken, then asks which fixes to queue. Use when the status line shows a feature branch instead of "main ✔", when a plan looks stuck, or when you've been away and don't remember where you left off.
argument-hint: "[fix all [<plan>]]"
version: 5.27.0
---

<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# /iterate-triage — what happened here, and what gets me back to main

You walked up to a terminal that isn't on `main ✔`. This answers why, in a few
lines, and then fixes what it can.

**Reaching triage at all is itself a finding.** `/iterate` is supposed to land
an all-green plan on the default branch by itself — commit, PR, merge, delete
branch, no human. So if the repo is sitting on a feature branch, something
failed to happen. Its problem line names what. Never present a stuck state as
if it were the normal end of a run.

## Answer in this shape — counts, then one line per open problem. Nothing else.

```
fixed: 2/9
broken still: 7/9
I can still fix 3/7
you must fix 4/7

1. I can fix the stood-down conductor by starting it for izcros.
2. I can fix the fairywren park by recording extend in its Decisions log.
3. I can fix orphan branch feature/old-sweep by landing or dropping it.
4. You can fix the stale manifests by running make build manifests.
5. You can fix the unsigned bundles by running make build sign.
6. You can fix the Gmail token by signing in at the consent screen.
7. You can fix the stopped release by typing /iterate ibex.
```

Four count lines, a blank line, then one numbered line per open problem.
Nothing before, between, or after, until the questions.

- **The counts are problems.** A problem is anything standing between here
  and `main ✔`: a blocked step, a gate, an inbox message waiting on this
  project, or an automation defect — work sitting loose, a merge that never
  happened, a dead loop still recorded in a plan file. A step that is merely
  not started is not a problem. Group blockers of one kind into one problem:
  three instance deletions is one problem about three instances.
- **`fixed`** counts the problems triage already cleared under "What triage
  does without asking"; **`broken still`** counts the rest. The last two lines
  split `broken still` by who clears each one and add up to it.
- **One line per open problem:** `I can fix <problem> by <how>.` or
  `You can fix <problem> by <how>.` The problem in a few words, the `by`
  clause ten words or fewer. `I can fix` lines come first, then `You can fix`,
  most-unblocking first within each.
- **`I can fix`** whenever triage can carry the whole route in this session
  and only consent is missing. A choice only the user can make — land or drop
  a branch, which of two rows wins — is still `I can fix`: triage asks the
  choice and carries it out.
- **`You can fix`** only when the route needs the user's hands: a command the
  permission layer refused in this session (try it first; never assume a
  refusal), the user's credentials, a machine triage cannot reach, a
  human-only act (sign in, click through a console), or a typed slash command
  (`/iterate <name>` for a release). If triage could have run it, it is
  `I can fix`.
- **Nothing that is good goes in the answer.** No detail paragraphs, no ✓
  lines, no recap, no offer. A fixed problem shows only in the count; its
  cause goes in the commit message or plan log it touched. If `broken still`
  is 0, the whole answer is the first two lines and one clause saying where
  things stand — `merged, on main` · `planned, never started` · `running now,
  heartbeat 20s, leaving it alone` — then stop.

## Ask, then fix

Plan every route before the list is shown, so each line is a promise triage
can keep. Then ask about each problem, in list order, with `AskUserQuestion`:
up to four questions per call, header `Fix N`, options `y` and `n`.

- `I can fix`: `N. queue this fix?`
- `You can fix`: `N. queue this fix in script?`
- A choice asks the choice itself — its options (`land`, `drop`) plus `n`.

Every question keeps its `Type something.` row, where the user answers
anything that is not on the list. **Never give an option a `preview`:** a
single-select question with one renders side by side, and that layout has no
`Type something.` row. Put the route in the option's `description` instead.

Without `AskUserQuestion`, print `queue this fix? y/n` (or `queue this fix in
script? y/n`) under each line and end the turn; read the reply's answers in
list order (`y n y y n y y`). `fix all [<plan>]` answers `y` to every question
for that plan's problems, or every problem, and asks only the choices.

Once every answer is in, without another word:

1. **Run each queued `I can fix`**, in list order, and verify its end state
   from the filesystem. A failing fix stops at its first error; the others
   still run.
2. **Write every queued `You can fix` into one script**, `<repo>-triage.sh` in
   the scratchpad directory (`/tmp` without one): `set -euo pipefail`, each fix
   echoing `fix N:` before its commands, and a human-only act as
   `open <url|file>` plus one echoed instruction. Nothing in it reads stdin — a
   `!` command has no terminal. Run `bash -n` on it, run its read-only parts,
   then put one line on the clipboard: `printf 'bash %s' "$script" | pbcopy`.
3. **Answer** with one line per failed fix — `N failed at <where>: <error>` —
   then, when the script exists, exactly:

   ```
   your queued fixes are now on the clipboard.  run them with "! <paste>"
   ```

   A queued slash command cannot ride in a script; each one follows as
   `then type <command>`.

When the script's output lands in the conversation, verify each fix it
claimed, mark cleared plans cyan, and finish whatever waited on the human act.
Report only what failed.

**Verify before you report, always.** A fix that says done and did not is
worse than one that failed loudly. Read back what each fix claimed: the branch
is gone, the file is non-empty, the tree is clean, the commit is reachable from
somewhere that is not about to be deleted.

## Decide which case this is

Read, in order: the current branch and whether the tree is dirty; every
`./.claude/iterate/plans/*.md` frontmatter; `./.claude/iterate/current`;
`conductor status --json` and project-scoped explain output; and every
`./.claude/iterate/inbox/*.md` key block. Then match the first case that fits.

| State | Verdict and action |
|---|---|
| On default branch, clean, no plans, no unworked or operator-waiting messages | `fixed: 0/0`, `broken still: 0/0`, `clean, nothing outstanding`. Stop. |
| Plans exist, all `phase: planned`, on default branch | **Planning session, never kicked off.** Nothing broken — the clause names the plans; `/iterate <name>` starts one. A normal resting state, not a fault. |
| binary reports a live execution owner | **A run is live right now.** Say so in the clause and leave it alone. Do not touch the branch, the plan, or the tree. |
| `phase: executing`, no fresh heartbeat, all steps done, all validations green, not merged | **The merge never happened.** This is the failure case — finish it (see below); it counts as fixed. |
| `phase: executing`, `status: blocked-on-operator` / `awaiting-human-gate` | **Blocked on you.** Each blocker is a problem; plan its route down to one question before you answer. |
| `status: blocked-on-operator` on a purchase, subscription, token or licence key the plan's `Cost:` line did **not** approve (an approved one missing its credential is an ordinary access blocker — handle it as one) | **Planning defect, not a blocker on you.** The executor should never have parked here (`/iterate` rule 29). Never "buy X": `I can fix` it by re-planning the free path through `/iterate-planner` and marking the plan `unblocked` with the reason. |
| `status: blocked-on-operator` on notarization, a notary credential or an Apple agreement, in a plan **without** `mode: release` | **Planning defect, not a blocker on you.** Only a release plan notarizes (`/iterate` rule 34). `I can fix` it: mark the release half `[RELEASE → /iterate-release]` and set `status: unblocked`, and the conductor lands the rest. |
| `mode: release`, `phase: executing`, stopped mid-run | **A release stopped part way.** Its line says which of tag-pushed and published happened. `You can fix` it by typing `/iterate <name>` — the conductor never resumes a release. |
| `status: blocked-on-operator` whose reason is a design decision or contract expansion the executor could have made (extend vs withdraw, which approach, what a flag means) | **Executor defect, not a blocker on you.** The executor should never have parked here (`/iterate` rule 33). `I can fix` it: record the decision as extend in the plan's Decisions log and set `status: unblocked`. |
| `phase: executing`, stopped mid-run, no terminal status | **Died mid-run** (session killed, context ran out, cron lost). Commit anything loose; `You can fix` it by typing `/iterate <name>`. |
| Feature branch with no matching plan | **Orphan branch.** Its line says whose it looks like from the name. `I can fix` it by landing or dropping it — a choice; triage runs the one picked. Never delete unasked. |
| `status: paused` (magenta) | Not a problem — a human stopped it with `/iterate pause`, and `/iterate resume <name>` continues. Commit loose work as always. |
| `status: unblocked` (cyan) | Already cleared, waiting its turn; the conductor takes it next unless the graph shows it waiting (next row). If the conductor is off, `I can fix` it by starting it: `conductor start --project <id>`. |
| A plan `iterate-run plan graph` shows waiting | Not a problem: it waits on an unfinished predecessor and starts when that lands. The plan at the root of the hold is the problem, reported once in its own line as `<root> holds <n> plans`, routed by that root's own row in this table. A plan the graph shows blocked (an unknown name, a cycle, itself on `depends:`) is a problem whose route is fixing its `depends:` line with `/ip <name> depends on <x>`. |
| Several plans all-green but unmerged | Batch them: one merge session landing every branch, in dependency order, rather than N separate ones. |
| `phase: closed` sitting in `plans/` | Should have been archived. Archive it now (nothing is lost, it is the step `/iterate` owed); it counts as fixed. |
| Not a git repo | Report plan state only; every branch line is a silent no-op. |

**An unworked or operator-waiting message is a problem in every case above.**
An `inbox/*.md` whose key block reads `status: open` (unread), `status: read (…)`
(read, not yet worked) or `status: waiting (operator: …)` is another project
waiting on this one: its session sent a request or bug report here with
`/mailbox`. A `status: held (…)` line reads as `waiting (operator: …)`. Each
message is one problem, and its line names the sender and the ask, read from
the file.
- **Unworked** (`open` or `read`): `I can fix` it by drafting its plan with
  `/ip plan inbox <id>`. That is planning only. The plan is born unstaged, so
  `/ip stage <name>` or `/iterate <name>` is still the human's call.
- **Waiting on the operator**: the act its `waiting (operator: …)` reason
  names — a decision there is a choice triage asks.

A message in `waiting (reply: …)` waits on another project's reply and is not
a problem here. While any other is present, never report the project as clean.

## What triage does without asking

**Commit loose work on a feature branch. Always, immediately, no confirmation.**
Uncommitted work is the one state with no upside — a commit on a feature branch
is not a merge, carries no risk, and is what protects the work. The auto-commit
hook does *not* cover this: it writes to `refs/snapshots/` and never moves HEAD,
so `git log` shows nothing and normal git cannot see it. Confirmed live: 19
files of verified work sat loose for six days while snapshots existed the whole
time.

Commit with a real message describing what the work *did*, not "wip". Run the
project's fast checks first if they exist and take seconds; if they fail, commit
anyway and say so in the commit message — an honest commit of broken work beats
losing it.

**Finish an interrupted merge.** When every step is done and every validation is
green, merging is not a decision — it is the obligation `/iterate` already had
and failed to discharge. Run its merge flow: commit, `/feature-branch finish`,
merge the PR, delete the branch, confirm back on the default branch. Then
record which part of the automation dropped it in the plan's log, because that
is a bug worth knowing.

**Never merge anything that is not all-green.** Same rule as `/iterate`, and it
does not bend for triage: blocked, partial, or failing means the branch stays,
and the problem line gives the shortest honest route to green.

Anything else that rewrites history or discards work — force-push, hard reset,
branch delete, rebase onto a moved main — **ask first**, every time. The ask is
that problem's question, not a script in the user's lap: plan the route, and
run it on their `y`. A human who has to execute a fix triage could have
executed has been made to do the work twice — once deciding, once typing.

## Bounded conductor diagnosis

A binary supervisor diagnosis supplies assignment identity, scope and a result destination. Inspect only its named blocker and return evidence using that result contract. Do not resume whole-plan execution, create a timer, write runtime/lease state or clear an operator pause. The supervisor owns retries, parking and subsequent admission. Existing consent does not authorize destructive history rewrites, unavailable credentials or external messages beyond the assignment.

## Scripts

The clipboard script is real before the clipboard line is written: it exists
at the path, `bash -n` passes, its read-only parts were run by you, and a
person can run it blind — it prints what it did and stops at the first failure
with the exact next thing to do. A clipboard line pointing at a script that
does not exist yet is a promise, not a fix. One script per triage run; never a
list of commands to type by hand.

### When it's cleared, mark it cyan

Set `status: unblocked` in the plan's frontmatter and append what changed to its
log. The status line turns that plan's letter cyan, and `/iterate-conductor`
picks cyan plans up **before** anything never started.

Only mark it when the blocker is actually gone — verify it, don't take "should
be fine" for an answer. A plan wrongly marked cyan goes straight back into the
queue to fail on the same wall.

**Check binary status after unblocking.** Read `conductor explain --project ID --json`; a waiting reason it reports is a problem line of its own, except `plan_waiting`: a plan held behind an unfinished predecessor is counted once, at its root. Existing enabled rules reconsider staged/unblocked work through the binary worker. Resume a paused project only when the user requests it — a `y` to that problem's question is the request — using its current revision; never arm a skill cron or silently override an operator stop.

### Unblocking while the conductor is running something else

This is the normal case, not an edge case: the conductor works K while you sit
in a second session clearing J. **The conductor owns one project lease across plans and subscriptions.** Inspect binary status before editing. Independent read-only diagnosis is safe; code changes require a separate worktree and explicit ownership, without bypassing the active project lease.

**But the working tree is shared, and the conductor has K's branch checked out.**

- **Never check out J's branch.** That pulls files out from under a live run
  mid-step and corrupts K.
- **Environment blockers need no branch at all** — credentials, permissions,
  deleting a VM, restarting a runner, answering a question. Fix the world,
  record it, mark cyan. This covers almost everything that actually blocks.
- **If J genuinely needs code changes**, use a worktree:
  `git worktree add ../<repo>-J <J's branch>`, work and commit there, then
  `git worktree remove`. Record it in J's log — a stray worktree left behind
  is confusing later.

Before touching anything, check whether the plan you're about to edit is the one
currently held by a live binary execution owner. If it is, report and
stop — that plan is not yours to triage right now.

## Rules

1. **Read state, never recall it.** Days may have passed and the conversation
   may be gone. Every claim comes from a file or a command run now.
2. **The format is the answer.** Counts, one line per open problem, the
   questions, then failures and the clipboard line — nothing before, between,
   or after, and nothing about what went right. Someone standing at a
   terminal is not reading paragraphs, least of all reassuring ones.
3. **Never execute plan steps.** Triage diagnoses, commits, and finishes an owed
   merge. Running the actual work is `/iterate` — hand off, don't absorb.
4. **Never ask a question you can answer.** "Which plan?" is answerable from
   `current` and the frontmatter. The only questions are the per-problem
   ones.
5. **Say what broke.** Reaching triage means automation failed. An open
   problem's line names it; a fixed one's commit message or plan log does — a
   silent fix leaves the same bug to happen next week.
6. **Safe to run mid-plan.** If a run is live, report and stop. Triage must
   never disturb a working plan.
7. **`I can fix` is the default.** `You can fix` only when the user's hands
   are required. A fix triage could have run, put in the user's script, is
   triage offloading its own job.

## `version`

`version` (or "what version") on **any** iterate skill reports the same thing —
the family version, because the stack is versioned as one unit:

```
iterate family 5.0.0
iterate-run iterate-v3.3 (commit 4dd09ec5, built 2026-08-27_17:02:20)
```

**Both lines come from a real read, never from memory — the family line
included.** Run these two, from any directory:

```bash
grep -m1 '^version:' ~/.claude/skills/iterate/SKILL.md   # the family version
iterate-run version                                      # the binary
```

**Never quote the `version:` in the skill body you already have in context.**
A session loads a skill body once and keeps it, so after a `skillctl family
iterate set` and reinstall, the copy in context is stale — and reporting its
number is precisely the memory recall this rule already forbids for the
binary. Confirmed live: a session answered `iterate family 5.4.0` ten minutes
after 5.5.0 was installed and verified on disk.

If members disagree, say so and name them: drift inside the family is a
defect, not a state, and `skillctl family iterate set X.Y.Z` is the only
correct way to bump.

## Execution ownership

Use `iterate-run plan reconcile <name> --json` to reconcile dead execution leases. Preserve checkpoint evidence and alive orphaned children. Never delete lease files or clear ownership fields by hand. A planned, queued, staged or idle executing plan has no permanent executor; only a live execution owns the project. Read readiness and execution order from `iterate-run plan graph --json`.
