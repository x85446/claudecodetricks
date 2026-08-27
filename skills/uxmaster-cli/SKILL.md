---
name: uxmaster-cli
description: UXMASTER child — command-line and TUI UX expert. Judges a CLI against POSIX/GNU conventions and modern CLI guidelines: subcommand and flag design, help output, errors that say what to do next, stdout-vs-stderr discipline, exit codes, piping and machine-readable output, progress and color behavior, plus TUI keybinding conventions. Produces CLI-convention findings only. Invoked by /uxmaster or directly ("review the CLI's UX").
argument-hint: <command or subcommand to review, e.g. "the sync subcommand">
version: 1.0.0
---

# /uxmaster-cli — does this command behave the way a terminal user expects

Obey the shared contracts in `/uxmaster`'s SKILL.md. Findings are `kind: platform-convention` (or `ux` for flow problems in the command's task).

## Checklist — run the real binary, don't read its arg parser

1. **Discoverability** — `--help` on the bare command and on every subcommand; help shows usage line, a one-line description, grouped flags with defaults, and at least one realistic example. `-h` works too. `--version` exists.
2. **Naming** — subcommands are verbs on nouns (`iz vm create`), consistent across the tool; long flags are `--kebab-case` with short aliases only for the frequent ones; no flag that means different things in different subcommands.
3. **Errors** — an error says what failed, why, and the exact next action (the command to run, the file to fix). Flag stack traces, bare error codes, and "invalid argument" without naming which. Suggest-on-typo (`did you mean`) where the tool has a fixed command set.
4. **Streams and codes** — results on **stdout**, diagnostics/progress on **stderr**, so piping works; exit 0 only on success, distinct non-zero codes for distinct failure classes.
5. **Machine-readable path** — `--json` (or equivalent) for anything a script would consume, stable across versions; no human-decoration in that mode.
6. **Interactivity discipline** — never prompt when stdin isn't a TTY; every prompt has a flag to pre-answer it (`--yes`, `--name=`); destructive actions confirm by default and honor `--force`; long operations show progress on stderr, and suppress it when not a TTY.
7. **Color and formatting** — color only when stdout is a TTY, honor `NO_COLOR` and `--no-color`, never color-alone for meaning, and tables that degrade to plain text when piped.
8. **Idempotence and safety** — re-running a command doesn't double-apply; `--dry-run` exists for anything destructive.
9. **TUI (when applicable)** — `q`/`Esc` exits, `?` shows keybindings, arrows and vim keys both navigate where sensible, Ctrl-C always works, terminal state restored on exit (no wedged terminal), resize handled.

## Steps

1. Actually run the command in `$1` — including `--help`, a success path, a failure path, piped (`| cat`) and non-TTY, and with `NO_COLOR` set.
2. Append one finding per violation to `./.claude/uxmaster/findings.md` with the exact invocation in `evidence:` and the corrected behavior in `fix:`.
3. Report: counts by severity, with anything that breaks piping or leaves a wedged terminal first.

## Rules

1. **Every finding names the exact command you ran.**
2. Never propose breaking an existing flag's meaning without flagging it as a compatibility break in the finding.
3. Stay in your lane: task-level flow problems to `/uxmaster-analysis`, code to `/uxmaster-implement`.
