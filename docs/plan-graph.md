# Plan dependency graph

A project's iterate plans form a graph. Each plan names the plans it builds on in its key block, and the conductor starts a plan only when every one of them has landed. Plans with nothing unfinished behind them run in any order.

## Declaring predecessors

The key block is the lines above a plan's first `## ` heading. One line there names the predecessors:

```
milestone: M1-06
depends: M1-04, M1-05
```

- A reference is a plan name (`caiman`) or a milestone id (`M1-04`) that some plan carries on a `milestone:` line. A plan can carry several `milestone:` lines when it covers several milestones, and any of its ids resolves to it.
- `depends: none` declares that the plan builds only on what main already holds.
- A plan with no `depends:` line has no predecessors.
- Dependencies are local to one project. Work another project owes goes in the plan's Goal and `## Blocks`.

`/ip` writes the line on every plan it creates. `/ip <name> depends on <x>` and `/ip drop dependency <x> from <name>` edit it, and an edit keeps the plan staged: staging approves the work, and `depends:` only orders it.

## When a predecessor counts as landed

The graph reads live plans from `.claude/iterate/plans/` and finished ones from `.claude/iterate/archive/`, taking each archive's outcome from its filename suffix:

| Predecessor | Edge |
|---|---|
| archived `<ts>-<name>-done.md` | satisfied |
| archived `<ts>-<name>-landed-rolled-to-<new>.md` (its green work merged, a parked feature rolled on) | satisfied |
| archived `<ts>-<name>-rolled-to-<new>.md` | follows the roll to `<new>` |
| archived `<ts>-<name>-closed.md` | holds, reason `closed unfinished` |
| live with `phase: complete` or `done` | satisfied |
| any other live plan: planned, queued, executing, blocked, paused, unblocked | holds |

The timestamp prefix can take any of the forms `/iterate` has written (`20261013T014252Z-`, `2026-10-10T060037Z-`, `2026-10-09T05-24-17Z-`).

## States

Every live plan has one graph status, computed when the graph is read and never written into the plan:

- **ready** — every predecessor has landed.
- **waiting** — at least one predecessor is unfinished. `waiting_on` lists them; `held_by` names the plan at the root of the hold, the one whose landing would start the chain moving.
- **blocked** — the plan's own `depends:` line is in error: it names an unknown plan or milestone, the plan itself, a milestone two plans carry, or a cycle (`cycle alpha → bravo → alpha`).

A predecessor that another of the plan's predecessors already depends on is an *implied* edge. It still gates admission. Drawings and wait reasons leave it out, so a plan whose prose lists every earlier phase reads as waiting on the one directly before it.

## How the conductor uses it

- A waiting plan is denied as `plan_waiting`, with the reason (`waiting on dace (unblocked)`), whether it would start or resume. A queued, `unblocked` or `executing` plan with no live run waits alike; a plan with a live run is never interrupted.
- A blocked plan is denied as `plan_blocked` with its error.
- Within a rank (unblocked, then executing, then queued), plans are ordered by graph depth, then milestone id, then `Started:`.
- When every staged plan waits behind a predecessor that only a human can move (blocked, parked, closed or unstaged), the watch stands down with a notice naming that root: `Queued plans require operator attention: caiman holds 7 plans`.
- The nightly decision reports `nothing startable (<n> waiting)`, and the dashboard stops waking workers for a queue whose every plan waits or is blocked once its watch has stood down.
- A typed `/iterate <name>` on a waiting plan proceeds after one notice line and logs `started ahead of depends: <x>`.

## Reading the graph

### Terminal

```bash
iterate-run plan graph                       # tree: each plan once, under its first predecessor
iterate-run plan graph --format mermaid      # Mermaid flowchart to paste into Markdown
iterate-run plan graph --json                # nodes, edges, state, waiting_on, held_by
iterate-run plan graph --check               # one line per graph error; exit 1 when any
```

`-C, --project <dir>` names another project. Flags work in any position. The tree marks each plan `✔` done, `●` ready, `◌` waiting or `✖` blocked, and the marks fall back to `+ * ~ x` outside a UTF-8 locale. A plan with several predecessors prints once, under its first, followed by `also after <names>`. The summary line goes to stderr.

### Dashboard

Each project in the dashboard on http://localhost:8420 has a **Graph** tab beside Plans, Automations and Activity:
- Plans are drawn left to right by depth, colored by status.
- Dashed arrows still wait, and solid arrows are satisfied.
- Selecting a plan opens its plan page.
- A wide graph scrolls sideways inside its panel, and the summary line says so.
- **Held plans** lists each waiting plan with its reason.

In the Plans tab, a waiting plan's badge reads **Waiting**, and its row names what it waits on.

`GET /api/graph?project=<id or path>` serves the same document as `iterate-run plan graph --json`, for a configured or registered project only.

## Retrofitting existing plans

`iterate-run plan depends backfill` derives a `depends:` line for every plan that has none. It merges two sources:
- The plan's `docs/milestones.md` row: the internal ids on each of its milestones' `**Depends on:**` lines, with items marked `(external)` dropped and the plan's own ids removed.
- Its `Depends on:` constraints naming `(from plan X)`, `(plan X)`, `(from milestone M…)` or `nothing`.

```bash
iterate-run plan depends backfill            # dry run: propose, keep, skip, unresolved
iterate-run plan depends backfill --apply    # write the proposed lines
```

`--apply` inserts each line after the plan's `milestone:` lines, or after its last key line, by atomic replace. Every other byte stays as it was, staging included. A plan that already has a `depends:` line is kept as written. A plan with a live run (a fresh heartbeat, `running: true`, or a conductor assignment) is skipped. Prose that names no plan is listed as `unresolved <plan>: <prose>` for a reader to settle.

`/ip retrofit` runs the whole procedure in one project:
1. A dry run.
2. A check of each proposal against its source.
3. The apply.
4. A reading of every unresolved plan.
5. `iterate-run plan graph --check`.
6. The `/ip stage …` command that would let the graph order the plans not yet staged.

The retrofit never stages anything.

## Execution sequence

`iterate-run plan graph --json` supplies each live node's `position` (1-based),
`label` (A–Z), `release` group and `waiting_reason`. The project's `next` names
the first ready, unpaused plan, or remains absent when no plan is ready.
Animal codenames identify plans; labels identify their execution positions.
Status transitions preserve positions and release groups.

`iterate-run plan order show --json` displays the sequence. Set the complete
live sequence with `iterate-run plan order set <name> <name> ...`.
The command validates prerequisites, cycles, duplicate positions and labels,
and the 26-plan maximum before writing any position. Consolidate related
plans when a sequence exceeds 26 plans. Archived predecessors remain context
nodes without execution labels.

## Execution ownership

`iterate-run plan claim <name> --owner cli --pid <session-pid> --json` atomically
claims a ready plan for a long-lived session process. The Conductor uses
`--owner conductor`. Every unfinished prerequisite gates both entry paths.
A planned, queued, staged or idle executing plan reserves no executor.
A matching live process or an owned orphaned child excludes another claim.

Use `plan heartbeat <name>` at validation and step boundaries,
`plan release <name>` at an ending, and `plan reconcile [<name>]` to finish a
dead lease while retaining checkpoint evidence. The binary writes the plan's
ownership key block under a project lock. Uncertain delivery evidence requires
diagnosis before replay.

After launching an owned child, `plan heartbeat <name> --child-pid <pid>` records
the child's observed process identity. The lease excludes another executor while
that child survives its supervisor.
