# The conductor

The conductor (`/iterate-conductor`, alias `/ic`) works a project's queue of
iterate plans while nobody is at the keyboard. `/iterate` runs one plan. The
conductor decides which plan runs next, hands it to `/iterate`, deals with how
it ended, and moves on, until nothing is left that a machine can advance.

It supervises and never executes steps itself. Every step is written, tested
and committed by `/iterate`, under its own rules.

## The pieces

| Piece | What it is |
|---|---|
| A plan | `.claude/iterate/plans/<name>.md`, written by `/ip` |
| Approval | `status: queued` in the plan, written by `/ip stage <name>`. The conductor never starts an unstaged plan. |
| Conductor state | `.claude/iterate/conductor.md`: on/off, paused, current plan, tick state, and the sweep log |
| The tick | What wakes the conductor: the launchd job, a cron the conductor armed, or nothing (Codex) |
| `/iterate` | Executes the chosen plan |
| `/iterate-triage` | Diagnoses a stopped or blocked plan and clears what a machine can |
| Launch policy | `.claude/iterate/policy.md`, written by `/iterate-rules`: the hours any run may start |

## What wakes it

**launchd (the normal case).** A project enrolled with
`iterate-run nightly enroll <dir>` carries `tick-source: launchd` in its
`conductor.md`. The LaunchAgent `com.x85446.iterate-nightly` then starts the
conductor there, headless, whenever the project has work and nothing else is
running. The name says nightly, but it runs at every hour the project's
schedules allow, and a project with no `policy.md` allows every hour.

**A cron it arms itself.** In an interactive Claude session on a project that
is not enrolled, `/ic start` arms a 5-minute cron that fires `/iterate-conductor`
in that session. It lives only as long as the session does.

**Nothing (Codex).** Codex cannot schedule itself, so `$ic start` and `$ic run`
are one sweep each. Codex `$ic` can enroll a project, after which launchd starts
`claude -p` runs there, not Codex ones.

## One cycle under launchd

1. **Every 600 seconds** macOS runs `iterate-run nightly tick`, as
   `~/Library/LaunchAgents/com.x85446.iterate-nightly.plist` instructs.
2. **The tick reads the master switch**, `~/.claude/iterate-run/nightly.json`.
   Off, it logs `disabled` and exits.
3. **It lists the enrolled projects**: every project iterate-run knows whose
   `conductor.md` says `tick-source: launchd`.
4. **For each project it asks, in order**, and the first "no" skips the
   project until the next tick:
   1. Is the conductor enabled and not paused?
   2. Is a plan queued, unblocked, or mid-run?
   3. Is the last run finished? (the project's lock is free)
   4. Is nobody else working here? (no heartbeat stamped in the last 15
      minutes, `ITERATE_LIVE_SECS`, after the last run finished)
   5. Is the current time inside `policy.md`'s launch-schedule and `conductor.md`'s
      conductor-schedule?
5. **It starts a detached runner and exits.** The runner takes the project's
   lock and runs
   `claude -p "/iterate-conductor run" --permission-mode auto --output-format json`
   in the project directory. There is no window. The lock is an OS file lock,
   so a killed run never leaves it held.
6. **The conductor sweeps once** (next section) and hands one plan to
   `/iterate`, marking it `loop-mechanism: external` so `/iterate` arms no
   timer of its own.
7. **`/iterate` works the plan to its ending in that one turn**: checks out
   the plan's branch, then for each step writes the code, runs its validation
   and commits.
8. **The conductor logs the result** in the sweep log and exits. The runner
   records the exit code and finish time and releases the lock.
9. **The next tick** finds the lock free and starts the next run.

## The sweep

Each run is one sweep, and the sweep stops at the first case that applies:

1. **Disabled or paused, nothing current** → exit without a word.
2. **Outside the permitted hours** → exit. Both schedules must allow it. The
   conductor-schedule can only narrow the launch-schedule, never widen it.
3. **The current plan is still executing** → under launchd its run ended short
   of finishing it, so the conductor runs `/iterate-triage fix all <plan>` and
   then `/iterate <plan>` again.
4. **The current plan ended** → handle the ending (below), then continue.
5. **A plan is startable** → pick one and hand it to `/iterate`.
6. **Nothing startable** → import open forge issues as a plan, or wind down.

**Which plan goes next:**

1. `status: unblocked` (cyan): a blocker someone just cleared.
2. `phase: executing`: work already begun.
3. `status: queued` (orange): the oldest staged plan.

Red plans (`blocked-on-operator`, `awaiting-human-gate`) and `status: paused`
plans are skipped.

## How a plan ends

| Ending | What happens |
|---|---|
| All green | `/iterate` opens a PR, merges it to main, deletes the branch and archives the plan. |
| Blocked on one feature | `/iterate` runs the landing test: the green work merges, and only the blocked steps roll to a new plan, parked red. |
| Blocked | The conductor runs `/iterate-triage fix all <plan>` once per blocked state. If that clears it, the plan goes straight back to `/iterate`. If not, it climbs the escalation ladder. |
| Stalled | 45 minutes with no step checked off and no commit (`plan-box:`) counts as blocked. A plan that keeps advancing is never boxed. |
| Needs a secret, physical access or an outside party | Parked red at once, and the conductor takes the next plan in the same sweep. |

The **escalation ladder** tries a different approach each rung, never the same
one again:

- **Access:** substitute a local conformant server, self-heal credentials, retry once.
- **Missing dependency:** install it (free and permissively licensed), rebuild, retry.
- **Merge conflict:** rebase.
- **A design choice inside the repo:** make it and log it.

A wall that only a purchase would clear is never a park. The executor takes the
strongest free path and records the gap.

## Unblocking a red plan

Open a second session in the same project and run `/iterate-triage`. It names
what the plan waits on, clears what it can, and marks the plan
`status: unblocked` (cyan). The conductor takes cyan plans first.

The working tree is shared with a running conductor, so the second session
never switches branches. Environment fixes (credentials, permissions, a
setting) need no branch. Code on the parked plan's branch goes in a
`git worktree`.

## Winding down

The conductor's tick has three states:

| State | Tick | When |
|---|---|---|
| `working` | 5 minutes | Something is running or startable |
| `watching` | 60 minutes | Nothing startable, but something may change by itself (a host returns, an issue is filed) |
| `stood-down` | none | Every remaining blocker needs a human, or watching ran 12 ticks with no change |

Standing down sends one push notification naming what the operator must supply.
Under launchd the OS timer keeps firing, but the tick skips a project with
nothing startable, and staging or unblocking a plan starts it again.

## Controls

| Scope | Command | Effect |
|---|---|---|
| Machine | `iterate-run nightly off` / `on` | Ticks start nothing; a run in progress finishes |
| Machine | `iterate-run nightly uninstall` / `install --every 10m` | Removes or arms the LaunchAgent |
| Machine | `iterate-run nightly tick --dry-run` | Prints what the next tick would do |
| Project | `iterate-run nightly enroll <dir>` / `withdraw <dir>` | Adds or drops the project from the tick |
| Project | `/iterate-rules <rule in plain words>` | Limits the hours any run may start |
| Project | `/ic pause` / `/ic resume` | Pause finishes the current plan and holds the queue; resume picks it back up |
| Project | `/ic stop` | Finishes the current plan, then turns the conductor off |
| Project | `/ic kill` | Halts now, stops the plan's scratch VMs and containers, turns the conductor off |
| Project | `/ic run` | One sweep now |
| Project | `/ic status` | Current plan, queue, blocked plans |
| Plan | `/ip stage <plan>` / `/ip unstage <plan>` | Approves or withdraws a plan for unattended launch |
| Plan | `/iterate pause <plan>` | Parks one plan; only `/iterate resume` unparks it |
| A run | `kill <pid>` | Ends a `claude -p` run; the next tick triages and resumes its plan |

## Watching it

| Question | Where |
|---|---|
| Is a run going now? | `iterate-run nightly status` |
| What each tick decided | `tail -f ~/.claude/log/iterate-nightly/status.txt` |
| The processes | `ps -axo pid,lstart,command \| grep 'claude -p'` |
| What each run did | `## Sweep log` in `.claude/iterate/conductor.md` |
| Every plan's state | `iterate-run status`, run in the project |
| A run's final output | `~/.claude/log/iterate-nightly/<project>/<UTC stamp>.json` |
| A run as it happens | the newest `.jsonl` in `~/.claude/projects/<project slug>/` |
| Dashboard | `iterate-run serve`, then http://localhost:8420 |

In the iTerm2 status bar, ⏰ marks an enrolled project and each plan is one
letter. Orange is queued, green is running, red is waiting on you, and cyan is
unblocked.

## Files

| File | Holds |
|---|---|
| `~/Library/LaunchAgents/com.x85446.iterate-nightly.plist` | The 600-second timer |
| `~/.claude/iterate-run/nightly.json` | The master switch |
| `~/.claude/iterate-run/projects.json` | Every project iterate-run knows |
| `~/.claude/iterate-run/nightly/<project>.lock` | Held while a run is going |
| `~/.claude/iterate-run/nightly/<project>.json` | The last run: launched, finished, exit code |
| `~/.claude/log/iterate-nightly/status.txt` | One line per tick decision and per run |
| `<project>/.claude/iterate/conductor.md` | Conductor state and sweep log |
| `<project>/.claude/iterate/policy.md` | Launch schedule and launch keyword |
| `<project>/.claude/iterate/plans/<name>.md` | Each plan, with its status |

## Running it from Codex

The headless Codex equivalent of one launchd run:

```bash
codex exec -C <project> --approve-for-me --json '$iterate-conductor run'
```

To watch a sweep interactively: `cd <project> && codex '$iterate-conductor run'`.
