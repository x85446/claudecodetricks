# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com).

## [gaur] - 2026-10-05

### Added
- `skills/tutorial/lib/box.sh`: the one box builder for the tutorial runtime. `tut_box <colour> <title> [row…]` sizes every frame to its widest row (ANSI stripped, one column per character), caps it at the terminal width (COLUMNS, then stdout's own terminal via `stty size`, then `tput cols`, then 80), wraps longer rows at word boundaries with a hanging indent, and draws `+ - |` outside UTF-8 or on `TERM=dumb`. Runs under macOS `/bin/bash` 3.2. It also holds the colour ladder (`tut_color_on`: NO_COLOR, FORCE_COLOR/CLICOLOR_FORCE, TERM=dumb, a TTY) and the symbol check (`tut_utf8`).
- `skills/tutorial/tests/box-test.sh` (`make tutorial-box-test`): drives `run.sh` and buckets on a real pty at 60/80/132 columns, cases box-1..box-7. It fails all seven against b421bc1c.
- `/iterate` rule 33: a contract expansion is a step, never an operator decision. The executor extends the contract, updates every consumer, re-runs their validations and continues; extend-or-withdraw is answered extend. Defects in the plan's deliverable and regressions its work caused are fixed in the same run. Planner rule 33 writes the expansion as the step, with a `Parity:` constraint.
- `skills/iterate/tests/contract-test.sh` (`make iterate-contract-test`): contract-1..contract-5 check the rule text in source and that the installed family matches. It fails all five against 9240afce.

### Changed
- Tutorial menu, `tut_title` and `tut_done` draw with `tut_box`. Every box is closed on all four sides.
- The menu duration is one dim line, `~45 min running (10 hands-on)` with `TUTORIAL-WALLCLOCK` and `10 min` without, aligned in one column. On a narrow terminal each duration moves under its title.
- Tutorial colour roles are the skill's standard (SKILL.md table). `↑ ↯` fall back to `^ !`, and `tut_done` says "1 step". Tutorial skill 1.4.0 copies three files.
- `/iterate`: the operator-only list is closed (a secret, physical access, an external party's act). A landing-test failure on clause 3 or 4 caused by the plan's own code is work, not a park.
- Conductor escalation ladder: contract-expansion and regression rows clear without a park, design choices fold into the ambiguity row, and the last row is "a secret, physical access, or an external party". Triage reads a park on a design decision as an executor defect, cleared with `fix problem N`.
- Iterate family 5.11.0, with the 13 Codex ports' version line stamped to match.
- CLAUDE.md states the contract-expansion rule and the clause-3/4 landing rule.

## [flamingo] - 2026-10-05

### Added
- **The nightly tick**: `iterate-run nightly install|uninstall|on|off|enroll|withdraw|status|tick`. A LaunchAgent (`com.x85446.iterate-nightly`, `StartInterval` 600, `ProcessType Standard`) walks the registered projects every ten minutes and, for each enrolled one with a queued, unblocked or stalled-executing plan that is not live, not already running, and inside both its launch-schedule and conductor-schedule, spawns a detached `claude -p "/iterate-conductor run" --permission-mode auto`. Each project runs under its own flock; results land in `~/.claude/log/iterate-nightly/` with a one-line `status.txt` per event. `--dry-run` prints every decision and its reason.
- `src/internal/iterrun/schedule.go`: the `/iterate-rules` launch-schedule grammar evaluated in Go — deny beats allow, any allow makes default-deny, windows wrap midnight, a day label matches the day the window opened, malformed lines fail closed; `conductor-schedule` intersects and can only narrow.
- `status: queued` as a first-class plan state. `/ip stage|approve|queue <name>` writes it, `unstage` removes it, `list` marks `(queued)`, and every refinement op removes it and says so. `iterate-run status` prints one `plan <name> <state>` line per plan (blocked, paused, unblocked, executing, queued, planned); the dashboard badges and filters `queued`.
- `iterate-run name peek`: the project's next plan letter and word, without claiming them.
- Statusline: orange letter for a queued plan (256-colour 208, bright-yellow fallback), the next plan's letter always rendered dim after the real ones, and `⏰` when the project is enrolled in the nightly tick (`⏰ off` when the global switch is off).
- `/iterate-conductor enroll|withdraw` and `tick-source: launchd` in `conductor.md`; `/iterate` understands `loop-mechanism: external` (arm nothing, cancel nothing, keep the field).
- `make statusline-check` (22 byte-level fixtures in `dotfiles/statusline-check.sh`) and `make nightly-install|nightly-uninstall|nightly-status|nightly-tick`.

### Changed
- Statusline: an executing plan whose heartbeat is older than `ITERATE_LIVE_SECS` is dark green (256-colour 28, plain-green fallback) instead of merely un-bold; live stays bold green with `⚡`.
- `/iterate`: clears `status: queued` on the transition to executing; a bare `/iterate` never launches a queued plan; rule 32 — pause is a human verb: the executor never ends a turn on a go-ahead request over machine time, and a drifted slow-tier case is run in-plan under `iterate-run run` rather than parked.
- `/iterate-conductor`: picks unblocked → executing → queued and never a bare `phase: planned`; under `tick-source: launchd` arms no cron, writes `loop-mechanism: external` into the plan it dispatches, and stands down without a cron to cancel; a `status: paused` with no operator pause line is a self-pause defect it logs, clears and resumes.
- `/iterate-planner`: plans are flat by default — "team this"/"teamify" opts in; the TESTMASTER finisher says drifted slow cases run inside the FFIV sweep.
- TESTMASTER: the slow tier is out of the default selection, not out of a plan's reach — a drifted slow case the plan names runs without confirmation; a bare `run all` mid-plan still downgrades.
- iterate family 5.10.0; CLAUDE.md legend and family docs follow.
- `.claude/iterate/plans/` is gitignored: live plans are per-machine state, archives stay tracked.

### Fixed
- The statusline's `⚙️` segment vanished for a project whose last plan had been archived: the root walk required `.claude/iterate/plans/` to exist; it now anchors on `.claude/iterate` (plans/ or archive/).
- The conductor could not dispatch `/iterate` at all — the Skill tool refuses a `disable-model-invocation` skill — and stood down with a staged plan in front of it; it now reads `/iterate`'s SKILL.md and follows it, the same sanctioned path the `/i` and `/ic` aliases use.

## [Unreleased]

### Added
- **Token spend on the dashboard**, per plan: who spent it (coordinator vs each team, with peak context), what it went on by tool, and what each wake-up cost. Built by joining the session transcripts Claude Code already writes to the hook event log on session and agent id — a teammate's transcript is `<session>/subagents/agent-<agent-id>.jsonl` and that id is the one the hook already recorded, so attribution is exact rather than inferred. Windowed to the plan's own `Executing:`→`Finished:` span, because one coordinator session routinely spans plans months apart. Also `iterate-run tokens [--plan <name>]` for the same three tables in the terminal.
  - A cron or `/loop` firing that arrived while the previous turn was still running is reported **coalesced** (it never got its own request and cost nothing), separately from one that **ran** and then made no tool call. On galago, 507 `/iterate` firings produced 139 real wake-ups carrying 58% of the run's spend.
  - Streaming rewrites are folded by message id keeping the **last** write: the earlier ones carry a placeholder `output_tokens`, so keeping the first undercounted one teammate's output 9x (16,239 vs 150,021) and truncated its tool calls. Synthetic all-zero usage records are not counted as requests at all.
  - Lane labels are trimmed to fit their column. Most lanes are short team names, but a plain Agent-tool dispatch is labelled with its own description — "Map profile editor for Destination control" — which is a sentence, and broke the CLI table's alignment outright on newcorder's xenops.
  - Transcripts are parsed incrementally — only bytes appended since the last render — and a line without its terminating newline is left for the next pass, so a 178 MB transcript being written to right now is safe to read and costs nothing on re-render.
- Dashboard activity timelines carry wall-clock hash marks, aligned to the bar track: hourly for a multi-hour run, `HH:MM` under 20 minutes apart for a short one, dates for a multi-day one. Marks land on real boundaries (:00, :15, 06:00) computed from local midnight with `time.Date`, so a run spanning a DST transition still labels real local hours and the hour that does not exist gets no mark.
- `/iterate pause [<plan>]` and `/iterate resume [<plan>]` — stop a run cleanly at a turn boundary and pick it back up. Paused plans show magenta in the status line, are skipped by the conductor, and report as `broken: 0` in triage.
- `## Running resources` ledger in every plan: each VM, container, background process or agent a step leaves running, with its exact stop and start commands. `pause` and `/ic kill` stop them (plus a leak sweep across the project's other and archived plans); `resume` starts them again; every ending stops the `scratch` ones.

### Fixed
- **The Requirements row was missing for any plan written in the executor's checkbox format.** The iterate family writes plan items in three shapes because its own skills disagree — `/iterate-planner`'s template emits `1. <task>`, while `/iterate`'s emits `- [ ] 1. <step>` for Steps and `- [ ] check 1: <criterion>` for Validation — and the dashboard read only the planner's. Measured across all 136 plan files on this machine: 3,193 plain items, 422 boxed-numbered, 79 boxed `check N:`. Result was 23 plan files with no Requirements row at all and 8 MIXED files showing only *some* of their steps, which is worse because a partial row looks correct. Three were live plans (personaldb's halibut, filemaster's wombat at 28 requirements, izbooter's badger at 26), and for an unteamed boxed plan `BuildRowsFromFilesystem` returned no rows at all, so its steps never reached the page. All three formats now parse. One extinct format from May 2026 (`1a.`/`1b.` pairs interleaved under one `## Steps` heading) is still unread — one archived plan, and nothing emits it any more.
- **A checked box is now progress, without ever claiming an unproven step is met.** A ticked Validation box reads `met`; a ticked Step box whose Validation box is explicitly unticked reads `partial` — attempted, not proven — because those two routinely disagree and the disagreement is the point: halibut has step 1 ticked and validation 1 unticked, which is exactly true, since the step ran and named the sites behind login walls and its validation is what the plan is blocked on. An explicit `step N done:` line in the Status/Log still wins over both, being more specific and carrying the note. The requirement detail panel no longer labels that case "gave up" — the orange bucket has two causes, and only one of them is giving up.
- **"Activity by team" no longer renders as an empty section on an unteamed plan.** Its one row is the coordinator, already shown in its own section above; a heading with nothing under it reads as missing data rather than as "this plan has no teams".
- **A plan's own page kept working after the run archived it.** `/plan?name=<x>` read only the live `plans/` directory, so the moment `/iterate` archived a finished plan its page degraded to a summary-less shell: no Steps (the Requirements row vanished), no Teams table (every row read "running", none "done"), and no `Executing:` — which removed the activity floor and let planning-phase tool calls from days earlier into the chart. galago rendered as "Running for 70h28m55s since 2026-09-12" with 10 severe gaps for a run that executed 10h46m32s on 2026-09-15 with 5. The route now follows the plan into the archive (most recent run of a reused codename), where time zero is `Executing:` — the first `/iterate` call — as it always should have been.
- **Every team's tool calls were being filed under the coordinator.** Teams share the plan's working tree by design, so the hook's `resolvePlanTeam` matched `.claude/iterate/current` from cwd first and returned them as coordinator-level work — the agent id, which encodes the team, was only consulted when cwd failed. On plan galago that put 498 of the 881 spans in the wrong row, and left the team rows with nothing but their log file and their `iterate-run run` wrapped commands: reporting showed 45 minutes of "severe downtime" across three gaps in which it had actually made 77 tool calls. A non-empty agent id is now a subagent, never the coordinator; the team comes from the id's own `a<plan>-<team>-<hash>` shape (a re-dispatch's `-2` folding into the same lane), with the label map as fallback and the raw id as a last resort — never the coordinator's key. The reader recovers the team from the agent id the same way, so the 215,000 events already on disk read correctly without re-running anything.
- Busy time and gaps are computed over the **union** of a row's spans, not the sum: a row carries both a coarse span and the fine calls inside it, and summing them reported more work than the row's own span is long.
- A team's log-file span is a **bounding box, not work** — it is used only when a team has no per-call data. Counting a file's existence as activity hid real outages: ledger's log spanned the six hours its model was exhausted, which erased the gap the coordinator correctly reported for the same window.

### Changed
- `/iterate-triage` reports only what is broken — `complete: x of y`, `broken: z of y`, `Detail`, `Fix` — with every `Fix` ending in one human act.
- Every skill's frontmatter is valid strict YAML; the Codex sync no longer freezes ports whose bodies its own version and diet passes rewrote.

## [axolotl] - 2026-09-10

### Added
- `iterate-run serve` binds a fixed default port (8420), accepts `--port 0` to take an ephemeral one and print it, and serves `GET /healthz` returning the build version.
- launchd agent `com.x85446.iterate-run-serve` (RunAtLoad + KeepAlive) with `make serve-install` / `serve-status` / `serve-uninstall`; logs to `~/Library/Logs/iterate-run/`.
- launchd agent `com.x85446.iterate-run-chrome` keeping a browser on the dashboard in a dedicated `--user-data-dir`, debug port 9242, `CHROME_BIN` overridable for Chromium-family browsers.
- Plan frontmatter field `harness:` (`claude-code` | `codex` | `unknown`), written by both harnesses' planners and carried through refinement and roll-forward.
- Dashboard renders a per-plan harness badge, a per-project rollup when a project mixes harnesses, harness-correct launch syntax, and a per-project conductor status line including the `NO TRIGGER` degraded state.
- Version resolution from either the frontmatter markers or the Codex ports' `**Version:** iterate family x.y.z` body line.
- `make run` — builds and serves the dashboard on a free port, printing its URL.
- `TestAnimalPool` asserting pool integrity: 26 letters, per-letter floor, total, no duplicates, lowercase ASCII, correct first letter.

### Changed
- Animal codename pool grown from 110 to 1,442 entries; per-letter floor of 12.
- Root Makefile migrated to the 2-layer convention: seven `##@` sections, `## help text` on all 31 targets, generated `make help`, launchd logic delegated to `makehelp.sh`.
- `TestNextPlanNameSeedsFromDisk` derives its seed from `animalsByLetter` instead of a hardcoded five-name list.
- README documents the dashboard, both always-on services, and plan naming.

### Fixed
- `iterate-run name next` no longer fails outright when a letter is exhausted: it advances to the next letter with capacity, and errors only when the whole pool is spent, reporting remaining counts per letter.
- `launchctl bootstrap` race under concurrent launchctl activity, via a `bootstrap_with_retry` helper that backs off and treats "already loaded" as success.
- Chrome's remote-debugging port moved off 9222, which would otherwise shadow the user's normal browser for anything attaching to that well-known port.
