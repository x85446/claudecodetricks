---
name: iterate-conductor
description: Works the whole plan queue unattended. When started, sweeps every unarchived iterate plan in this project, drives each to completion via $iterate, clears blockers by escalating to different approaches, and batches whatever it genuinely cannot solve into one shared blocked plan for a human session. Also imports open GitHub/GitLab issues as plans. Controlled with start/stop/pause/resume/run/status/kill; runs on its own cron tick while enabled.
---


<!-- version: bump on EVERY behavioral change (minor for additions, major for schema/contract changes, patch for wording). -->

# $iterate-conductor — keep the whole queue moving

| Skill | Role |
|---|---|
| `$iterate-notes` (`$in`) | **Capture** |
| `$iterate-brainstorm` (`$ibs`) | **Decide** |
| `$iterate-planner` (`$ip`) | **Plan** |
| `$iterate-rules` | **Gate** — when a run may start |
| `$iterate` (`$i`) | **Execute** — one plan |
| `$iterate-conductor` | **Conduct** — every plan, unattended, until the queue is empty |

## Usage

Argument: start | stop | pause | resume | run | status | kill. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$accounts` — ported.
- `$feature-branch` — ported.
- `$i` — ported.
- `$ibs` — ported.
- `$in` — ported.
- `$ip` — ported.
- `$iterate` — ported.
- `$iterate-brainstorm` — ported.
- `$iterate-notes` — ported.
- `$iterate-planner` — ported.
- `$iterate-rules` — ported.

`$iterate` runs *a* plan. The conductor runs *the queue*: it picks the next
plan, lets `$iterate` drive it, handles what comes back, and moves on — sweep
after sweep — until nothing is left that a machine can advance.

**It supervises; it does not re-implement execution.** Every plan is executed by
`$iterate` with its own loop, lock, and rules. The conductor decides *which*
plan runs next and *what happens* when one ends. Never drive steps directly
from here: two things owning execution is how a plan gets worked twice.

## State — `./.claude/iterate/conductor.md`

```markdown
---
enabled: true
paused: false
cron: <job-id>              # the tick this conductor armed; cleared on stop
current: <plan-name>        # plan handed to $iterate, empty between plans
blocked-plan: <plan-name>   # the shared human-needed plan, empty if none open
imported-issues: [12, 47]   # issue numbers already pulled in, never re-imported
sweeps: 14
---

## Sweep log
- [2026-09-07T22:04Z] started owl · 6 steps
- [2026-09-07T23:41Z] owl complete, merged to main
- [2026-09-08T00:02Z] elk blocked (SSH host unreachable) → moved to hare
```

Project-scoped, always. The conductor never reaches outside the project it was
started in.

## Operations

Route on `$1`:

### `start` / `on`
Write `enabled: true`, `paused: false`. Arm a recurring cron firing
`$iterate-conductor` — **every 5 minutes**, not every minute: a tick that finds
a run already in flight does nothing, and `$iterate` owns its own 1-minute
resumption loop. Record the job id in `cron:`. Report what is queued. Then run
one sweep immediately rather than waiting for the first tick.

### `stop` / `off`
**The current plan finishes first.** Write `enabled: false`; the plan in flight
runs to its own terminal state, and no new plan is started after it. Cancel the
cron only once `current:` is empty — cancelling while a plan is mid-flight would
strand it without a supervisor to handle its ending. Report which plan is
draining and that no further plans will start.

### `pause`
Same draining behaviour, but `paused: true` with `enabled:` left true, so
`resume` picks the queue back up exactly where it stopped. The cron stays armed
and its ticks become no-ops while paused.

### `resume`
Clear `paused`. Run a sweep immediately.

### `run`
One sweep, right now, regardless of `enabled:`. Does not arm anything and does
not change enablement — the way to try the conductor without committing to it.

### `status`
Read-only. Print, in this shape:

```
conductor  enabled · sweep 14 · next tick 3m
current    owl — step 4/6, running 22m
queued     elk, fox
blocked    hare — 3 items awaiting a human
imported   12 issues, last sweep pulled 2
```

### `kill`
**The one verb that does not wait.** Halt now, mid-plan: cancel the conductor
cron AND the current plan's own loop, set `enabled: false`, and leave the plan
exactly where it stands (resumable with `$iterate <name>`).

`stop` and `pause` deliberately finish the current plan, so neither can halt a
plan that is misbehaving. `kill` is that lever, under a name that does not
pretend to be graceful. Report the plan left mid-flight and its branch state.

## The sweep

One tick. Do these in order and stop at the first that applies.

1. **Not enabled, or paused, and no `current:`** → exit silently. A no-op tick
   prints nothing.

2. **Launch window closed** → exit silently, logging one line the first time
   only. Read `./.claude/iterate/policy.md` per `$iterate-rules` and honour the
   `launch-schedule`. **Starting the conductor is the standing authorization
   that `require-launch-keyword` asks for** — record `authorized by: conductor
   enablement` in the plan's log rather than skipping the check silently — but
   the *schedule* still shapes when work happens. A conductor that ignored the
   window would make the window meaningless.

3. **`current:` is set and that plan is still executing** → exit silently.
   `$iterate` owns it and has its own resumption loop; a conductor tick here has
   no job.

4. **`current:` is set and that plan reached a terminal state** → handle the
   ending (next section), clear `current:`, continue to 5.

5. **A plan is queued** → pick the next one and hand it to `$iterate <name>`.
   Order: `phase: executing` before `phase: planned` (finish what is started),
   then oldest `Started:` first. Set `current:`, log the start, exit.

6. **Queue empty** → run bug intake (below). If it produced a plan, take it as
   `current:`. Otherwise log `queue empty` once and exit.

## When a plan ends

**Complete** — `$iterate` has already merged and archived it. Log one line.

**Blocked** (`status: blocked-on-operator`, `awaiting-human-gate`, or a 5-cycle
giveup) — try to clear it before accepting it as blocked:

### Clearing a blocker means a different approach, not more retries

`$iterate`'s 5-cycle cap stays. Running the same failing validation fifteen
times is not effort, it is a loop — and an unattended loop is the exact shape
that once ticked a dead plan for thirteen hours. Escalation means changing
*what* is attempted:

| Blocker | Escalation ladder |
|---|---|
| access / credentials | `$accounts` self-heal → retry once → blocked |
| dependency missing | install it → rebuild → retry once → blocked |
| test fails on pre-existing breakage | confirm it predates the plan → skip that check, note it, continue |
| merge conflict | rebase on the default branch → retry → blocked |
| ambiguity in the plan | pick the most reasonable reading, log the decision, continue — never stop to ask |
| needs a human decision, a secret, physical access, or an external party | **blocked immediately, no ladder** — no approach clears it |

**Each plan gets a wall-clock box** (`plan-box:` in conductor.md, default 45
minutes, counted from when the conductor handed it over). When the box expires,
the plan is blocked as-is and the sweep moves on. One stubborn plan must not eat
the night while five easy ones wait behind it.

### Moving blocked work out

Move the blocked items to **the shared blocked plan** — one open at a time,
named from `iterate-run name next` on first use, recorded in `blocked-plan:`,
and reused by every subsequent sweep until a human clears it.

1. Copy the blocked steps, their validations, and their provenance into the
   blocked plan, each tagged with its source: `from <plan>: <blocker>`.
2. Remove them from the source plan. If the source has nothing left unfinished,
   archive it; otherwise leave it queued so its remaining work continues.
3. Log the move in both plans.

**Batch the mergeable blockers into one merge.** If several plans end blocked on
"couldn't merge / couldn't commit / branch not merged", do **not** write one
merge step per plan. Write a single step in the blocked plan listing every
branch, so the human does one merge session that lands everything on the default
branch at once:

```
Na: Merge these branches to main, in order: feature/owl-metrics (clean),
    feature/elk-api (conflicts in src/api/routes.go), feature/fox-docs (clean).
    [skill: $feature-branch]
Nb: All three merged, branches deleted local and remote, main green.
```

That is the whole point of the blocked plan: it turns N interruptions into one
sitting. Any other blocker class that repeats across plans gets the same
treatment — one step covering all instances, never one per plan.

## Bug intake

When the plan queue is empty, import open issues from this repo's forge.

1. Detect the remote: `gh` for GitHub, `glab` for GitLab. Neither available or
   no remote → skip silently, this is not an error.
2. Fetch **all open issues**. Skip any number already in `imported-issues:` —
   an issue is imported once, ever, even if it is still open next sweep.
3. **Cap the import at `max-import:` per sweep (default 10).** A repo with a
   real backlog would otherwise flood the queue on the first run and spend the
   night on issues you never queued. Log how many were left for the next sweep;
   silent truncation would read as "that was all of them".
4. Build **one** plan for the batch via `$iterate-planner`, one step-pair per
   issue, each step citing its number and title, provenance
   `Issue #<n>: <title>`. Add every imported number to `imported-issues:`.
5. Never close, comment on, label, or otherwise write to an issue. Reading is
   the entire interaction — the plan's work speaks for itself when it merges.

## Rules

1. **Never execute steps directly.** `$iterate` executes; the conductor decides
   what runs next and handles endings. Two owners of execution means work done
   twice.
2. **One plan at a time.** Respect `$iterate`'s concurrency lock. The conductor
   gets throughput from never idling, not from parallelism.
3. **Never ask a question.** Unattended by definition. An ambiguity is a
   decision to make and log; a thing needing a human is a blocked item to move.
4. **Never bypass the launch schedule.** Enablement authorizes the *expense*
   (standing in for the launch keyword); the schedule still governs *when*.
5. **A no-op tick prints nothing and changes nothing.** Most ticks find a run in
   flight or an empty queue. Do not manufacture work — no audits, no status
   documents, no re-verifying a plan that is already terminal.
6. **Project-scoped, always.** Never touch plans, branches, or issues outside
   the project the conductor was started in.
7. **Log every state change, once.** Start, ending, block move, import, window
   skip. The sweep log is how a human reconstructs a night they slept through.
8. **`stop` and `pause` drain; only `kill` interrupts.** Never halt a plan
   mid-flight for a graceful verb — that is what leaves a half-built branch.
