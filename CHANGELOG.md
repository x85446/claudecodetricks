# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com).

## [Unreleased]

### Added
- **Token spend on the dashboard**, per plan: who spent it (coordinator vs each team, with peak context), what it went on by tool, and what each wake-up cost. Built by joining the session transcripts Claude Code already writes to the hook event log on session and agent id — a teammate's transcript is `<session>/subagents/agent-<agent-id>.jsonl` and that id is the one the hook already recorded, so attribution is exact rather than inferred. Windowed to the plan's own `Executing:`→`Finished:` span, because one coordinator session routinely spans plans months apart. Also `iterate-run tokens [--plan <name>]` for the same three tables in the terminal.
  - A cron or `/loop` firing that arrived while the previous turn was still running is reported **coalesced** (it never got its own request and cost nothing), separately from one that **ran** and then made no tool call. On galago, 507 `/iterate` firings produced 139 real wake-ups carrying 58% of the run's spend.
  - Streaming rewrites are folded by message id keeping the **last** write: the earlier ones carry a placeholder `output_tokens`, so keeping the first undercounted one teammate's output 9x (16,239 vs 150,021) and truncated its tool calls. Synthetic all-zero usage records are not counted as requests at all.
  - Transcripts are parsed incrementally — only bytes appended since the last render — and a line without its terminating newline is left for the next pass, so a 178 MB transcript being written to right now is safe to read and costs nothing on re-render.
- Dashboard activity timelines carry wall-clock hash marks, aligned to the bar track: hourly for a multi-hour run, `HH:MM` under 20 minutes apart for a short one, dates for a multi-day one. Marks land on real boundaries (:00, :15, 06:00) computed from local midnight with `time.Date`, so a run spanning a DST transition still labels real local hours and the hour that does not exist gets no mark.
- `/iterate pause [<plan>]` and `/iterate resume [<plan>]` — stop a run cleanly at a turn boundary and pick it back up. Paused plans show magenta in the status line, are skipped by the conductor, and report as `broken: 0` in triage.
- `## Running resources` ledger in every plan: each VM, container, background process or agent a step leaves running, with its exact stop and start commands. `pause` and `/ic kill` stop them (plus a leak sweep across the project's other and archived plans); `resume` starts them again; every ending stops the `scratch` ones.

### Fixed
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
