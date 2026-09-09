---
name: iterate-conductor
description: Works the whole plan queue unattended. When started, sweeps every unarchived iterate plan in this project, drives each to completion via /iterate, clears blockers by escalating to different approaches, and parks whatever it genuinely cannot solve as a blocked plan you unblock from a second session while it keeps working the rest. When nothing is left that a machine can advance it notifies you once and stands its own tick down instead of ticking at a wall. Also imports open GitHub/GitLab issues as plans. Controlled with start/stop/pause/resume/run/status/kill/schedule; runs on its own cron tick while enabled.
argument-hint: start | stop | pause | resume | run | status | kill | schedule <rule>
disable-model-invocation: true
version: 5.1.0
---

<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# /iterate-conductor — keep the whole queue moving

| Skill | Role |
|---|---|
| `/iterate-notes` (`/in`) | **Capture** |
| `/iterate-brainstorm` (`/ibs`) | **Decide** |
| `/iterate-planner` (`/ip`) | **Plan** |
| `/iterate-rules` | **Gate** — when a run may start |
| `/iterate` (`/i`) | **Execute** — one plan |
| `/iterate-conductor` | **Conduct** — every plan, unattended, until the queue is empty |

`/iterate` runs *a* plan. The conductor runs *the queue*: it picks the next
plan, lets `/iterate` drive it, handles what comes back, and moves on — sweep
after sweep — until nothing is left that a machine can advance.

**It supervises; it does not re-implement execution.** Every plan is executed by
`/iterate` with its own loop, lock, and rules. The conductor decides *which*
plan runs next and *what happens* when one ends. Never drive steps directly
from here: two things owning execution is how a plan gets worked twice.

## State — `./.claude/iterate/conductor.md`

```markdown
---
enabled: true
paused: false
cron: <job-id>              # the tick this conductor armed; cleared on stop
conductor-schedule:         # optional, same grammar as launch-schedule;
  - allow daily 22:00-06:00 #   intersected with it, so it can only narrow
current: <plan-name>        # plan handed to /iterate, empty between plans
tick: working               # working (5m) · watching (60m) · stood-down (none)
watch-ticks: 0              # consecutive watching ticks with nothing changed
watch-bound: 12             # watching ticks allowed before standing down
stood-down:                 # <ISO> <reason> once the queue needs a human
notified:                   # <ISO> the one push already sent; never repeated
last-cycle: 12             # sweep number of the last full pass; red plans get
                            #   one cheap blocker re-test per completed cycle
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
`/iterate-conductor` — **every 5 minutes**, not every minute: a tick that finds
a run already in flight does nothing, and `/iterate` owns its own 1-minute
resumption loop. Record the job id in `cron:`. Reset the wind-down ladder: `tick: working`,
`watch-ticks: 0`, clear `stood-down:` and `notified:`. Report what is queued.
Then run one sweep immediately rather than waiting for the first tick.

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
Clear `paused`, reset the ladder (`tick: working`, `watch-ticks: 0`, clear
`stood-down:` and `notified:`), and re-arm the 5-minute cron if standing down
had cancelled it. Run a sweep immediately.

### `run`
One sweep, right now, regardless of `enabled:`. Does not arm anything and does
not change enablement — the way to try the conductor without committing to it.

### `schedule` / `rules`
Delegate to `/iterate-rules`, passing the rest of the argument verbatim
(`/ic schedule weeknights only` → `/iterate-rules weeknights only`).

**The conductor does not own the launch schedule and must never write
`policy.md`.** That file gates *every* launch, not just the conductor's — a
human typing `/iterate owl` at 3pm is gated by it too. If it moved here,
`/iterate` would have to read the supervisor's config to know whether it may
run, which is backwards and breaks outright on a project where the conductor
was never installed. Policy says what is *allowed*; the conductor says what is
*running*. This verb exists so you can type the thought where you have it,
not to move the ownership.

To narrow *only* the conductor's own sweeping without touching what a human may
launch, use `conductor-schedule:` below — that one is genuinely ours.

### `unblock <plan>`
Mark a red plan cyan: set `status: unblocked`, append what changed to its log.
Use when you cleared the blocker outside a triage session. `/iterate-triage`
does this for you when it resolves one. **If the conductor is stood down, this
is what it was waiting for — re-arm it** (`start`'s ladder reset plus a fresh
5-minute cron) and sweep immediately, rather than leaving a cyan plan sitting
in front of a conductor with no tick.

### Degraded mode — enabled with no trigger

A harness that cannot schedule itself (Codex today) leaves the conductor in a
real but partial state: `enabled: true` and it will sweep, but **only when
something invokes it.** Nothing ticks on its own, so `start` is effectively one
sweep plus an intention.

Say that plainly rather than letting `enabled` imply autonomy:

```
conductor  enabled · NO TRIGGER — sweeps only when invoked
           create one: ChatGPT → Scheduled → "$iterate-conductor" every 5m,
           or an OS cron running `codex exec`
```

The same gap applies one level down: `$iterate` cannot arm its own resumption
either, so a plan the conductor starts has no recovery if that turn stalls. It
is still correct to start — the plan file loses nothing and a human can resume
it — but **say so at dispatch**, once, rather than letting a stalled run look
like a running one. Never claim a tick was armed when none was.

### `status`
Read-only. Print, in this shape:

```
conductor  enabled · sweep 14 · next tick 3m   (or "NO TRIGGER" — see above)
current    owl — step 4/6, running 22m
queued     elk (unblocked, next), fox
blocked    hare — needs: router creds
imported   12 issues, last sweep pulled 2
```

Wound down, the first line says so and the ask moves to the front — a stood-down
conductor is not a stopped one, and the difference is a thing you have to do:

```
conductor  stood down 13:24Z — the queue needs you · sweep 14 · no tick armed
current    —
queued     11, all gated behind stoat
blocked    stoat — needs: Fastmail JMAP API token, then `/ic resume`
imported   0 issues
```

### `kill`
**The one verb that does not wait.** Halt now, mid-plan: cancel the conductor
cron AND the current plan's own loop, set `enabled: false`, and leave the plan
exactly where it stands (resumable with `/iterate <name>`).

`stop` and `pause` deliberately finish the current plan, so neither can halt a
plan that is misbehaving. `kill` is that lever, under a name that does not
pretend to be graceful. Report the plan left mid-flight and its branch state.

## The sweep

One tick. Do these in order and stop at the first that applies.

1. **Not enabled, or paused, and no `current:`** → exit silently. A no-op tick
   prints nothing.

   **`tick: stood-down` and a tick fired anyway** → the cron outlived its
   cancel. Cancel it again, verify it is dead, log one line, exit. Standing down
   cancels the tick; a tick arriving afterwards means the cancel didn't take,
   which is the one thing that can quietly restore the behaviour it was there
   to end.

2. **Outside the permitted hours** → exit silently, logging one line the first
   time only. Two schedules apply and a sweep needs **both**:

   - `launch-schedule:` in `./.claude/iterate/policy.md`, owned by
     `/iterate-rules` — governs every launch in this project, human or
     conductor.
   - `conductor-schedule:` in `conductor.md` (optional) — same grammar, but
     scoped to the conductor's own sweeping.

   Requiring both makes the conductor schedule **narrowing by construction**:
   it can carve the conductor's hours down inside the launch window but can
   never widen past it, with no validation needed to enforce that. This is what
   expresses "I can launch by hand whenever I like, but the conductor only
   works overnight" — a thing the launch schedule alone cannot say, because it
   applies to both.

   **Starting the conductor is the standing authorization that
   `require-launch-keyword` asks for** — record `authorized by: conductor
   enablement` in the plan's log rather than skipping the check silently — but
   the schedules still shape when work happens. A conductor that ignored them
   would make them meaningless.

3. **`current:` is set and that plan is still executing** → exit silently.
   `/iterate` owns it and has its own resumption loop; a conductor tick here has
   no job.

4. **`current:` is set and that plan reached a terminal state** → handle the
   ending (next section), clear `current:`, continue to 5.

5. **A plan is queued** → pick the next one and hand it to `/iterate <name>`.
   Order, highest first:

   1. `status: unblocked` (cyan) — someone just cleared its path; honour that
      before starting anything new.
   2. `phase: executing` with no terminal status — finish what is already begun.
   3. `phase: planned` — oldest `Started:` first.

   Skip `status: blocked-on-operator` / `awaiting-human-gate` (red) entirely,
   except for the one cheap re-test per completed cycle described below. Set
   `current:`, clear any `status: unblocked` you just picked up, log the start,
   exit.

6. **Nothing startable** → run bug intake (below). If it produced a plan, take
   it as `current:`, stay in `working`, exit. If it produced nothing, this sweep
   is **dry** — resolve the tick state in "Wind-down" below. Do not exit still
   armed at 5 minutes.

## A compaction is not a plan ending

Long sweeps compact. When the context is summarized mid-sweep, re-read
`conductor.md` and the current plan file before deciding anything — the sweep
log records which plans are done and which is in flight, and that record is
accurate where your post-compact recollection is not.

**Never treat a compaction as a plan reaching a terminal state.** A plan is done
when its file says so, never because control came back to you looking like a
boundary. Mistaking one for the other archives unfinished work and moves on.

## When a plan ends

**Complete** — `/iterate` has already merged and archived it. Log one line.

**Blocked** (`status: blocked-on-operator`, `awaiting-human-gate`, or a 5-cycle
giveup) — try to clear it before accepting it as blocked:

### Clearing a blocker means a different approach, not more retries

`/iterate`'s 5-cycle cap stays. Running the same failing validation fifteen
times is not effort, it is a loop — and an unattended loop is the exact shape
that once ticked a dead plan for thirteen hours. Escalation means changing
*what* is attempted:

| Blocker | Escalation ladder |
|---|---|
| access / credentials | substitution test (is a local conformant implementation enough? see `/iterate`) → `/accounts` self-heal → retry once → blocked |
| dependency missing | install it → rebuild → retry once → blocked |
| test fails on pre-existing breakage | confirm it predates the plan → skip that check, note it, continue |
| merge conflict | rebase on the default branch → retry → blocked |
| ambiguity in the plan | pick the most reasonable reading, log the decision, continue — never stop to ask |
| needs a human decision, a secret, physical access, or an external party | **blocked immediately, no ladder** — no approach clears it |

**Each plan gets a wall-clock box** (`plan-box:` in conductor.md, default 45
minutes, counted from when the conductor handed it over). When the box expires,
the plan is blocked as-is and the sweep moves on. One stubborn plan must not eat
the night while five easy ones wait behind it.

### Blocked means "park it and move on"

**The plan stays exactly where it is.** Set `status: blocked-on-operator` (or
`awaiting-human-gate`) in its frontmatter with a one-line reason, leave it in
`plans/` on its own branch, clear `current:`, and go to the next plan. Nothing
is copied, nothing is archived, no separate blocked plan is built.

Earlier this skill assembled blocked work into one shared plan and archived the
sources. That was more machinery for a worse result: it split a plan's steps
across two files, cost the plan its own branch and history, and gave you one
undifferentiated pile instead of a per-plan state you can see at a glance. A
blocked plan is just a plan that is waiting.

**What you see instead.** The status line renders one letter per plan, coloured
by state — `⚙️ J K L` with J red and K green means J is waiting on you while K
is being worked right now. That is the whole handoff.

### Unblocking happens in a second session, concurrently

The conductor keeps working K. You open another session in the same directory
and run `/iterate-triage` to find out what J is waiting on and clear it. This is
supported and expected — the concurrency lock is **per-plan**, so a second
session touching a different plan is not a conflict.

**One physical constraint: the working tree is shared.** The conductor has K's
branch checked out. A second session must not check out J's branch — that yanks
files out from under a live run mid-step.

- **Environment blockers** — credentials, permissions, deleting a VM, restarting
  a runner, answering a question — need no branch at all. Fix the world, record
  the resolution in J's plan file, mark it cyan. This is the overwhelming
  majority.
- **Blockers that genuinely need code on J's branch** — use a git worktree:
  `git worktree add ../<repo>-J <J's branch>`, work there, commit, remove it.
  Never switch the shared tree's branch while the conductor is running.

### `status: unblocked` — cyan, ready to re-queue

When a blocker is cleared, the resolving session sets `status: unblocked` and
appends what changed to the plan's log. The status line turns that letter cyan:
*was blocked, is not any more, waiting for its turn.*

The conductor picks up cyan plans **first** — ahead of never-started ones. They
were already in flight and someone has just spent effort clearing their path;
leaving them behind a fresh plan wastes that.

**Red plans also get one cheap re-test per full pass through the queue.** Not
every sweep — once per complete cycle. Some blockers clear themselves (a host
comes back, a lease renews) and would otherwise sit red until noticed; but
re-testing a permission that still is not granted, every single sweep, is the
loop shape that burned thirteen hours. Only re-test blockers that are cheap and
objectively checkable — does the host answer, does the file exist, does the
command return zero. A blocker whose resolution is a human judgement is never
auto-retested; it waits for cyan.

## Wind-down — the conductor has a terminal state too

The top of this file promises to run *until nothing is left that a machine can
advance*. That is a **stopping condition**, and it has to live on the tick, not
only in the prose. A conductor whose queue has gone dry has finished; leaving
its cron armed turns a finished night into a machine ticking at a wall.

Confirmed live (symmail, 2026-09-09): stage 1 parked at 13:22Z needing a
Fastmail API token — correctly, no ladder clears a secret — and the eleven
plans behind it were gated on it merging. Every piece of state was right. The
conductor then ticked every five minutes into a queue that could not move, and
the first thing the operator learned about the token was the hour of no-op
scrollback they woke up to.

Three tick states. A sweep only ever moves **down** the ladder; a human touch is
what moves it back up.

| State | Tick | Entered when |
|---|---|---|
| `working` | 5m | Something is in flight or startable. The normal state. |
| `watching` | 60m | Nothing startable, but something could still become true **without a human deciding to make it true**: a host that may come back, a lease that renews, an empty queue on a repo that files issues. |
| `stood-down` | none | Every remaining blocker needs a human, or `watching` spent its bound with nothing changing. `cron:` cancelled. |

A **dry sweep** — step 5 found nothing to start and step 6's intake produced
nothing — resolves to exactly one of these:

1. **Every remaining blocker needs a human** (a secret, a decision, physical
   access, an external party), or there are no plans left at all and no forge to
   import from → **stand down.** Nothing but the operator changes this, and the
   operator is asleep.

   The discriminator is *not* "can I check it cheaply" — plenty of human
   blockers are trivially checkable, a token file among them. It is **can this
   become true without a human deciding to make it true.** A token file appears
   because a person went and made one, and when they do they are at a keyboard
   where `/ic resume` is one line. Polling for someone's decision is not
   watching, it is waiting with extra steps.
2. **Something can still change on its own** — a host that may come back, a
   lease that renews, an external job that may finish, or an empty queue on a
   repo that files issues → **watch.** Set `tick: watching`, re-arm at 60
   minutes, and spend each tick on exactly one thing: the cheap re-test, or the
   issue intake. Increment `watch-ticks:`; at `watch-bound:` (default 12, about
   half a day) with nothing changed, stand down. A watch that never converges is
   the same defect at a slower clock.
3. **Something became startable** → back to `working` at 5 minutes,
   `watch-ticks: 0`, and take it.

### Standing down

- **Notify, once.** `PushNotification` with the one thing the operator must
  supply and the one command that restarts the queue. The blocked plan's
  `Next attempt` banner already says both — quote it, don't re-derive it. This
  does not breach "never ask a question": it asks nothing and blocks on nothing.
  The run is over, and a finished night that never reached the one person who
  can restart it wasn't unattended, it was unreported. Stamp `notified:` and
  never send twice for the same blocker.
- **Cancel the tick you armed, and verify.** `CronDelete <cron:>`, read the
  result, confirm it is gone, clear `cron:`. `/iterate` learned this one the
  expensive way — a cron cancelled through the wrong path went on firing at a
  dead plan for thirteen hours — and the conductor arms the same kind of job.
- **Leave `enabled: true`** and write `stood-down: <ISO> <reason>`. The
  conductor did not stop, it *finished*: `status` says so, and `start` /
  `resume` / `unblock` pick it straight back up.
- **Park state, not work.** Plans stay exactly where they are, on their
  branches, red. Standing down changes nothing about them.

### Silence was never the fix

Rule 5 says a no-op tick prints nothing, and that is still true — but it could
never have solved this. **The harness renders the scheduled invocation itself.**
A tick that says nothing still lands in the transcript as one more *Running
scheduled task*, and an hour of those reads exactly like an hour of work not
happening. The only quiet tick is the one that doesn't fire.

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
4. Build **one** plan for the batch via `/iterate-planner`, one step-pair per
   issue, each step citing its number and title, provenance
   `Issue #<n>: <title>`. Add every imported number to `imported-issues:`.
5. Never close, comment on, label, or otherwise write to an issue. Reading is
   the entire interaction — the plan's work speaks for itself when it merges.

## Rules

1. **Never execute steps directly.** `/iterate` executes; the conductor decides
   what runs next and handles endings. Two owners of execution means work done
   twice.
2. **One plan at a time.** Respect `/iterate`'s concurrency lock. The conductor
   gets throughput from never idling, not from parallelism.
3. **Never ask a question — but do say when you're done.** Unattended by
   definition: an ambiguity is a decision to make and log, and a thing needing a
   human is a blocked item to move past. The single message you owe the operator
   is the one push when the whole queue has gone dry (see "Wind-down"), and it
   is a statement, not a prompt.
4. **Never bypass either schedule, and never write `policy.md`.** Enablement
   authorizes the *expense* (standing in for the launch keyword); the schedules
   still govern *when*. `/iterate-rules` owns `policy.md` — the conductor reads
   it and delegates edits, because a supervisor that rewrites the project's
   launch policy has escaped the thing meant to contain it.
5. **A no-op tick prints nothing and changes nothing.** Most ticks find a run in
   flight or an empty queue. Do not manufacture work — no audits, no status
   documents, no re-verifying a plan that is already terminal.
   Silence is not stand-down, though: the operator sees the tick fire whether
   or not you print. A queue that cannot move needs the tick gone, not quiet.
6. **Project-scoped, always.** Never touch plans, branches, or issues outside
   the project the conductor was started in.
7. **Log every state change, once.** Start, ending, block move, import, window
   skip. The sweep log is how a human reconstructs a night they slept through.
8. **`stop` and `pause` drain; only `kill` interrupts.** Never halt a plan
   mid-flight for a graceful verb — that is what leaves a half-built branch.
9. **Finish; never idle.** "Nothing left that a machine can advance" is a
   terminal state for the conductor itself: notify once, cancel your own tick,
   stand down. A supervisor that outlives its queue is the same defect as a
   loop that outlives its plan, one level up.

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
