---
name: iterate-triage
description: Walk up to a stale terminal and find out what's going on in one short answer. Reads the real state — plans, branch, uncommitted work, blockers — and gives a verdict plus the shortest path back to main. Use when the status line shows a feature branch instead of "main ✔", when a plan looks stuck, or when you've been away and don't remember where you left off.
argument-hint: (none — reads the project state)
version: 1.0.0
---

<!-- version: bump on EVERY behavioral change (minor for additions, major for schema/contract changes, patch for wording). -->

# /iterate-triage — what happened here, and what gets me back to main

You walked up to a terminal that isn't on `main ✔`. This answers why, in a few
lines, and then fixes what it can.

**Reaching triage at all is itself a finding.** `/iterate` is supposed to land
an all-green plan on the default branch by itself — commit, PR, merge, delete
branch, no human. So if the repo is sitting on a feature branch, something
failed to happen. Say what, plainly, in one line. Never present a stuck state as
if it were the normal end of a run.

## Answer in this shape, and keep it short

```
nightjar — blocked, 14/22 steps, branch feature/nightjar-testmaster (6 days)

  ✗ 3 things need you:
      1. delete macos-bake-tahoe, the orphan image, the build VM   (~109 GiB)
      2. stop/start win-runner — your live Windows runner
      3. router creds for the lease-table check

  ✓ committed 19 files of verified work that were sitting loose
  ✗ not merged — 4 failing facets; merging would put a red suite on main

  → start with #1? it closes steps 4, 5, 6, 10.
```

A verdict line, what needs *you* as a numbered list, what triage already did,
and one concrete next move. **No narration of how you found out**, no restating
the plan, no essay. If the whole answer is "everything's fine, you're on main",
that is one line and you stop.

## Decide which case this is

Read, in order: the current branch and whether the tree is dirty; every
`./.claude/iterate/plans/*.md` frontmatter; `./.claude/iterate/current`; and
`./.claude/iterate/conductor.md` if it exists. Then match the first case that
fits.

| State | Verdict and action |
|---|---|
| On default branch, clean, no plans | "clean — nothing outstanding." One line. Stop. |
| Plans exist, all `phase: planned`, on default branch | **Planning session, never kicked off.** Name the plans, say nothing is running, offer to add to one or start it. This is a normal resting state, not a fault. |
| `phase: executing`, `running:` heartbeat fresh (<90s) | **A run is live right now.** Report the step count and leave it alone. Do not touch the branch, the plan, or the tree. |
| `phase: executing`, no fresh heartbeat, all steps done, all validations green, not merged | **The merge never happened.** This is the failure case — finish it (see below). |
| `phase: executing`, `status: blocked-on-operator` / `awaiting-human-gate` | **Blocked on you.** Extract exactly what a human must do, as a numbered list. Walk the first one. |
| `phase: executing`, stopped mid-run, no terminal status | **Died mid-run** (session killed, context ran out, cron lost). Commit anything loose, then offer `/iterate <name>` to resume. |
| Feature branch with no matching plan | **Orphan branch.** Say whose it looks like from the name, whether it has unmerged commits, and offer to merge, rebase or delete. Never delete unasked. |
| `phase: closed` sitting in `plans/` | Should have been archived. Say so, offer to archive. |
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
green, merging is not a decision — it is the obligation `/iterate` already had
and failed to discharge. Run its merge flow: commit, `/feature-branch finish`,
merge the PR, delete the branch, confirm back on the default branch. Then say
which part of the automation dropped it, because that is a bug worth knowing.

**Never merge anything that is not all-green.** Same rule as `/iterate`, and it
does not bend for triage: blocked, partial, or failing means the branch stays.
Say so with the branch named, and give the shortest honest route to green.

Anything else that rewrites history or discards work — force-push, hard reset,
branch delete, rebase onto a moved main — **ask first**, every time.

## Walking a blocker

Take them one at a time, most-unblocking first. For each: state what you need in
one sentence, in the user's terms, not the plan's jargon. Do the machine half
yourself the instant the human half lands — if they grant a deletion, run it and
report the result before moving to the next.

Group blockers of the same kind into one ask. Three instance deletions is one
question about three instances, never three questions. If clearing one unblocks
several steps, say which — that is what makes an ask worth answering.

## Rules

1. **Read state, never recall it.** Days may have passed and the conversation
   may be gone. Every claim comes from a file or a command run now.
2. **Short.** A verdict, a numbered list of what needs the human, what you did,
   one next move. Someone standing at a terminal is not reading paragraphs.
3. **Never execute plan steps.** Triage diagnoses, commits, and finishes an owed
   merge. Running the actual work is `/iterate` — hand off, don't absorb.
4. **Never ask a question you can answer.** "Which plan?" is answerable from
   `current` and the frontmatter.
5. **Say what broke.** Reaching triage means automation failed. Name it in one
   line — a silent fix leaves the same bug to happen next week.
6. **Safe to run mid-plan.** If a run is live, report and stop. Triage must
   never disturb a working plan.
