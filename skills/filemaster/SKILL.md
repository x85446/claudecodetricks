---
name: filemaster
description: >-
  Use when someone asks to process an intake folder, identify/rename/route a
  scanned document, invoice or statement, stand up a new intake with init, or
  resolve a file filemaster escalated to review. The judgment layer for the
  filemaster CLI: recognize the document, then route up / route down / act.
argument-hint: "[init | <file or intake name>]"
allowed-tools: Bash, Read, Glob
---

# filemaster skill

You are the judgment layer for a filemaster intake. The `filemaster` CLI does the
deterministic work; you decide only what it can't. Architecture reference:
`docs/master.md` in the filemaster repo.

## Processing a file

1. Run `filemaster identify <file> --intake <name>` — it extracts text and scores
   confidence against the intake's own types plus every type composed up from its
   downstream intakes (via `routes`).
2. If `escalate` is false, let the CLI's decision stand: `filemaster process <file>
   --intake <name>` runs rename + route/act to completion. Add `--dry-run` first to
   preview the decision — method, type, confidence, new name, destination — with
   zero filesystem writes.
3. If `escalate` is true, read the extracted text yourself, decide the type and
   destination, then call `filemaster rename <file> --to <name> --apply` followed by
   `filemaster act <file> --intake <name>`, `filemaster route-down <file> --to <name>`, or
   `filemaster route-up <file> --intake <name>`.
4. Record anything the tool got wrong in the intake's own `.filemaster/feedback.md`
   (at the intake folder's root) so it can be handled deterministically next time —
   a case escalated once should not need escalating twice.

## Standing up an intake

`filemaster init <path> --name <name> [--parent <parent>]` writes a folder-local
`.intake` rules file and a `.filemaster/` directory for run artifacts at `<path>`,
then registers the install in `~/.claude/filemaster/registry.yaml` (name, path,
parent only — no rules are copied there). Ask what the intake should recognize and
where each type goes, then write those into `.intake` — types, naming templates,
thresholds, routes, and actions are all per-intake data. The engine is shared and
identical for every intake. `filemaster deregister <name>` (or `--all`) undoes a
registration without touching the folder or its `.intake`.

**Wire `routes`, not just `--parent`.** `--parent` only sets the child's route-up
target; it does not add a reverse `routes` entry on the parent. Detection composes
upward strictly through `routes` — a parent with no `routes` entry pointing at a
child cannot detect any of that child's types, and will misclassify or fail to
match its documents with no error raised. Every intake with children needs at
least one `routes` entry per child; once a child is reachable through any entry,
all of its types (not just the one named by that entry's key) become detectable
above it.

## Rules

- **Never delete.** Superseded files move to `.trash/`; unclassifiable files move
  to `.review/` with a `.note.txt` explaining what was tried.
- **Rename before acting.** Actions key off the standardized filename.
- **Only registered ops exist.** An intake's `action.op` must name an op in the
  shared library (`file`, `file_and_stack`). Needing new behavior means adding an
  op to the library, not special-casing one intake.
