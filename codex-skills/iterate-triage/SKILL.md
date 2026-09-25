---
name: "iterate-triage"
description: "Use when the status line shows a feature branch instead of \"main ✔\", when a plan looks stuck, or when you've been away and don't remember where you left off."
---


<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# $iterate-triage — what happened here, and what gets me back to main

**Version:** iterate family 5.6.0

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

Walk up to a stale terminal and find out what's going on in one short answer. Reads the real state — plans, branch, uncommitted work, blockers — and reports only what is broken, with the one act that clears each.

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


You walked up to a terminal that isn't on `main ✔`. This answers why, in a few
lines, and then fixes what it can.

## Usage

Argument: (none — reads the project state). `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$feature-branch` — ported.
- `$iterate` — ported.
- `$iterate-conductor` — ported.

**Reaching triage at all is itself a finding.** `$iterate` is supposed to land
an all-green plan on the default branch by itself — commit, PR, merge, delete
branch, no human. So if the repo is sitting on a feature branch, something
failed to happen. Say what, plainly, in one line. Never present a stuck state as
if it were the normal end of a run.

## Answer in this shape — the broken, and how each gets fixed. Nothing else.

```
complete: 27 of 28
broken: 1 of 28
Detail:
  problem 1: step 28 is a human gate. The moment the rules publish, the
    router moves 26 scans into 38 documents — 29 routed downstream, 9 held
    in .review for missing pages — and nothing has approved that split.
    The gate reads docs/scanner-decisions.md; two of its rows are
    contestable (the intake types that existed in no signature file).
  problem 2: 83 files of finished work sat uncommitted for 14 hours. A run
    that stops at a human gate never reaches $iterate's merge-time commit,
    so nothing protected the work.
Fix:
  problem 1: /tmp/wombat-p1.sh publishes the rules and runs the sweep.
    Check the two contestable rows in docs/scanner-decisions.md, then
    `pbcopy < /tmp/wombat-p1.sh` and paste into any terminal.
  problem 2: done — committed as 3f2a9c1 on feature/wombat-router-filer-reject.
```

Two count lines, then one entry per problem in two lists that share their
numbers. Nothing before, between, or after.

- **`complete` / `broken` count the plan's steps.** `broken` is every step not
  done — blocked, failed, gated. With no plan in play, count what is in front
  of you (branches, PRs) and say so in the same line.
- **A problem is anything standing between here and `main ✔`**: a blocked
  step, a gate, or an automation defect triage found — work sitting loose, a
  merge that never happened, a dead loop still recorded in a plan file. A step
  that is merely not started is not a problem.
- **`Detail` says what is broken, ≤150 words, in the user's terms.** What it
  is, why it stopped here, what it holds up. No restating the plan, no
  narration of how you found out, no reassurance about what still holds.
- **`Fix` says how it gets cleared with the least the human can possibly do,
  ≤50 words.** Triage plans the whole route and does every machine step
  *before* writing the line, so what is left for the human is exactly one act:
  - **Run one command.** When the fix is commands the human must issue — a
    `gh pr merge`, a deletion, a credential paste — write them into
    `/tmp/<plan>-p<N>.sh`, make it executable, and give the one line that
    puts it on the clipboard: `pbcopy < /tmp/<plan>-p<N>.sh`. The rest of the
    50 words say what the script does. One script per problem; never a list
    of commands to type by hand.
  - **Say `launch problem N`.** When the human must do something no script
    can — sign in, create an account, click through a console, decide a row —
    say how far you get on your own (`I get us to the consent screen; you sign
    in`) and stop. Their `launch problem N` is the go: do all of it, hand back
    the single thing only they can do, and finish the rest the moment it
    lands.
  - **`done — <what>`.** A problem triage already fixed under "What triage does
    without asking" gets one line here (`done — committed 83 files as
    3f2a9c1`) and one line in `Detail`. It is still reported: a silent fix
    hides the bug that caused it.
- **Nothing that is good goes in the answer.** No ✓ lines, no "standing
  constraints still hold", no recap of what the plan achieved, no offer, no
  "want me to?". The person asked what is broken; every other word is in the
  way. If nothing is broken, the whole answer is the two count lines and one
  clause saying where things stand — `merged, on main` · `planned, never
  started` · `running now, heartbeat 20s, leaving it alone` — then stop.
- **Most-unblocking first.** If clearing one problem closes several steps,
  that is the first thing its `Detail` says.

## Decide which case this is

Read, in order: the current branch and whether the tree is dirty; every
`./.claude/iterate/plans/*.md` frontmatter; `./.claude/iterate/current`; and
`./.claude/iterate/conductor.md` if it exists. Then match the first case that
fits.

| State | Verdict and action |
|---|---|
| On default branch, clean, no plans | `complete: 0 of 0 · broken: 0 of 0 — clean, nothing outstanding.` Stop. |
| Plans exist, all `phase: planned`, on default branch | **Planning session, never kicked off.** `broken: 0` — name the plans; `$iterate <name>` starts one. A normal resting state, not a fault. |
| `phase: executing`, `running:` heartbeat fresh (<90s) | **A run is live right now.** Report the step count and leave it alone. Do not touch the branch, the plan, or the tree. |
| `phase: executing`, no fresh heartbeat, all steps done, all validations green, not merged | **The merge never happened.** This is the failure case — finish it (see below). |
| `phase: executing`, `status: blocked-on-operator` / `awaiting-human-gate` | **Blocked on you.** Each blocker is a problem; plan its `Fix` down to one human act before you answer. |
| `phase: executing`, stopped mid-run, no terminal status | **Died mid-run** (session killed, context ran out, cron lost). Commit anything loose; the `Fix` is `$iterate <name>` — one command. |
| Feature branch with no matching plan | **Orphan branch.** Say whose it looks like from the name and what it carries that main lacks. `Fix` is one script to land it and one to drop it; the human runs the one they mean. Never delete unasked. |
| `status: paused` (magenta) | Not broken — a human stopped it with `$iterate pause`. `broken: 0`; say the step it stopped at and that `$iterate resume <name>` continues. Commit loose work as always. |
| `status: unblocked` (cyan) | Already cleared, waiting its turn. Say which plan; the conductor takes it next. If the conductor is off, `Fix` is `$iterate-conductor start` — one command. |
| Several plans all-green but unmerged | Batch them: one merge session landing every branch, in dependency order, rather than N separate ones. |
| `phase: closed` sitting in `plans/` | Should have been archived. Archive it now (nothing is lost, it is the step `$iterate` owed) and report `done — archived`. |
| Not a git repo | Report plan state only; every branch line is a silent no-op. |

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
anyway and say so — an honest commit of broken work beats losing it.

**Finish an interrupted merge.** When every step is done and every validation is
green, merging is not a decision — it is the obligation `$iterate` already had
and failed to discharge. Run its merge flow: commit, `$feature-branch finish`,
merge the PR, delete the branch, confirm back on the default branch. Then say
which part of the automation dropped it, because that is a bug worth knowing.

**Never merge anything that is not all-green.** Same rule as `$iterate`, and it
does not bend for triage: blocked, partial, or failing means the branch stays.
Say so with the branch named, and give the shortest honest route to green.

Anything else that rewrites history or discards work — force-push, hard reset,
branch delete, rebase onto a moved main — **ask first**, every time.

## Walking a blocker

`launch problem N` is the go for a problem whose `Fix` said so. Do everything
up to the human's one act — open the console, create the client, fill every
field a script can fill, stage the deletion — then say the one thing they must
do, in one sentence, in their terms. The instant it lands, do the rest: verify,
record, mark. Then the next problem, most-unblocking first.

A script a `Fix` line points at is real before the line is written: it exists
at the path named, `bash -n` passes, its read-only parts were run by you, and a
person can run it blind — it prints what it did and stops at the first failure
with the exact next thing to do. A `Fix` naming a script that does not exist
yet is a promise, not a fix.

Group blockers of the same kind into one problem. Three instance deletions is
one script about three instances, never three problems. If clearing one
unblocks several steps, say which — that is what makes it worth doing first.

### When it's cleared, mark it cyan

Set `status: unblocked` in the plan's frontmatter and append what changed to its
log. The status line turns that plan's letter cyan, and `$iterate-conductor`
picks cyan plans up **before** anything never started.

Only mark it when the blocker is actually gone — verify it, don't take "should
be fine" for an answer. A plan wrongly marked cyan goes straight back into the
queue to fail on the same wall.

**Then check that something is still ticking.** A conductor whose whole queue
was red stands its own cron down (`stood-down:` in `conductor.md`) — cyan is
exactly the event it was waiting for, and it has no tick left to notice it with.
Re-arm it with `$iterate-conductor start` and say you did. Otherwise the plan
you just cleared sits cyan in front of a conductor that will never look again,
which is the same stall you were called in to fix, wearing a friendlier colour.

### Unblocking while the conductor is running something else

This is the normal case, not an edge case: the conductor works K while you sit
in a second session clearing J. **The concurrency lock is per-plan**, so two
sessions on two different plans is not a conflict.

**But the working tree is shared, and the conductor has K's branch checked out.**

- **Never check out J's branch.** That pulls files out from under a live run
  mid-step and corrupts K.
- **Environment blockers need no branch at all** — credentials, permissions,
  deleting a VM, restarting a runner, answering a question. Fix the world,
  record it, mark cyan. This covers almost everything that actually blocks.
- **If J genuinely needs code changes**, use a worktree:
  `git worktree add ../<repo>-J <J's branch>`, work and commit there, then
  `git worktree remove`. Say you're doing it and why — a stray worktree left
  behind is confusing later.

Before touching anything, check whether the plan you're about to edit is the one
currently running (`running:` heartbeat under 90 seconds). If it is, report and
stop — that plan is not yours to triage right now.

## Rules

1. **Read state, never recall it.** Days may have passed and the conversation
   may be gone. Every claim comes from a file or a command run now.
2. **The format is the answer.** Counts, `Detail`, `Fix` — nothing before,
   between, or after, and nothing about what went right. Someone standing at
   a terminal is not reading paragraphs, least of all reassuring ones.
3. **Never execute plan steps.** Triage diagnoses, commits, and finishes an owed
   merge. Running the actual work is `$iterate` — hand off, don't absorb.
4. **Never ask a question you can answer.** "Which plan?" is answerable from
   `current` and the frontmatter.
5. **Say what broke.** Reaching triage means automation failed. Name it in one
   line — a silent fix leaves the same bug to happen next week.
6. **Safe to run mid-plan.** If a run is live, report and stop. Triage must
   never disturb a working plan.
7. **Every `Fix` ends in one human act.** One command already on a script, or
   `launch problem N`. If it takes two, triage has not finished planning.

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

The path is the Claude-side file on purpose: `skillctl` stamps the family
number there, and the Codex ports are generated from it without the field.

**Never quote a `version:` from the skill body you already have in context.**
A session loads a skill body once and keeps it, so after a bump and reinstall
the copy in context is stale — and reporting its number is precisely the
memory recall this rule already forbids for the binary. Confirmed live: a
session answered `iterate family 5.4.0` ten minutes after 5.5.0 was installed
and verified on disk.

If members disagree, say so and name them: drift inside the family is a
defect, not a state, and `skillctl family iterate set X.Y.Z` is the only
correct way to bump.
