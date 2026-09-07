---
name: hours-maker
description: Use when someone asks to fill out hours maker, log weekly hours for Softbank, populate the hours-maker spreadsheet, log Travis's or Ed's hours, or report work to Softbank from Outlook calendar.
argument-hint: <travis|ed|shared> [week M/D/YY] [--cap HOURS]
disable-model-invocation: true
allowed-tools: mcp__workspace-mcp__read_sheet_values, mcp__workspace-mcp__modify_sheet_values, mcp__workspace-mcp__create_sheet, mcp__workspace-mcp__get_spreadsheet_info, mcp__claude_ai_Microsoft_365__outlook_calendar_search, AskUserQuestion, ToolSearch, Read, Bash
---

## What This Skill Does

Fills out a weekly tab in the **Hours Maker** Google Sheet (Softbank/NEDO reporting). Three modes — each implemented in its own workflow file:

| Mode | Section | Writes to | Workflow file |
|---|---|---|---|
| `travis` | Travis's private work | B2:H49 | `travis.md` |
| `ed` | Ed's private work | B52:H99 | `ed.md` |
| `shared` (fill) | Joint area from Outlook | B101:H148 + Ed/Travis propagation | `shared.md` |
| `shared` (validate) | Audit joint vs calendar | reads B101:H148, writes selectively | `validate.md` |

**Placement is evidence-driven.** All three modes place work from *real sources*,
never synthesized: shared mode from Outlook calendar events; travis/ed modes from
real Claude-session + browser timestamps via **[evidence-placement.md](evidence-placement.md)**
and the `evidence_hours.py` engine (+ user-owned `project_task_map.json`).
`patterns.md` is a deprecated distribution model kept only for sheet layout — do
not use it to place hours.

Spreadsheet ID (fixed): `1T6dw_I7Vz59pjeofvuFZWIbhven8SI-UaxemAqmXO-k`. User email for MCP calls: `travis.mccollum@gmail.com`.

Mode keyword can appear in any position in args (so both `/hours-maker travis 5/6/26 "..."` and `/hours-maker 5/6/26 travis "..."` work).

## Step 0 — Tool availability check (every invocation)

Before doing any work, verify the required MCP tools are loaded.

**Base tools (all modes):**
- `mcp__workspace-mcp__read_sheet_values`
- `mcp__workspace-mcp__modify_sheet_values`
- `mcp__workspace-mcp__create_sheet`
- `mcp__workspace-mcp__get_spreadsheet_info`

**Shared mode additionally needs:** `mcp__claude_ai_Microsoft_365__outlook_calendar_search`

**Validate sub-action additionally needs:** `AskUserQuestion`

Procedure:
1. Scan the tools available in this session. If any required tool is missing, call `ToolSearch` with `select:<tool1>,<tool2>,...` to load schemas (single call, comma-separated).
2. If `ToolSearch` returns "No matching deferred tools found" for any required tool, **stop** and tell the user:

```
⚠ Required MCP tool not available: <tool_name>
  - workspace-mcp tools: ensure the workspace-mcp server is configured (see ../.mcp.json at the workspace root, or the user-global Claude Code MCP config).
  - M365 calendar tool: enable the Microsoft 365 connector at claude.ai → Settings → Connectors.
Once enabled, restart Claude Code or reload tools, then re-run.
```

Don't try fallbacks — the skill needs these tools.

## Step 1 — Mode dispatch

Scan the arguments for one of `travis`, `ed`, or `shared` (case-insensitive). Whichever appears first decides the mode. If `validate` (or `verify`/`check`/`audit`) also appears with `shared`, the sub-action is **validate**.

- `travis` → Read `travis.md` from this skill's directory and follow that workflow.
- `ed` → Read `ed.md` and follow that workflow.
- `shared` (no validate keyword) → Read `shared.md` and follow the fill sub-action.
- `shared … validate` → Read `validate.md` (and `shared.md` for shared utilities like pre-validation and canonical map) and follow validate.
- none found → print usage `"Usage: /hours-maker <travis|ed|shared> [args]"` and stop.

Use the `Read` tool with the path `.claude/skills/hours-maker/<file>.md` (relative to project root) or the absolute path if running from a different cwd.

## Cross-cutting notes & guardrails

These apply to every mode and override anything in the per-mode files when they conflict.

- **Integrity — placement is reconstruction, not invention (HARD RULE).** Hours and their day/time come from real evidence (calendar events, Claude-session/browser timestamps). Never synthesize *when* work happened, never pad a total above real working time, never auto-assign UNMAPPED/non-Izuma activity to a NEDO task. A reporting ceiling (e.g. Softbank under-reporting) may only trim the total **down**. Work with no timestamp evidence must be supplied by the user (real windows), not fabricated. See [evidence-placement.md](evidence-placement.md). This rule exists because an earlier version of this skill used pseudorandom placement that produced plausible-but-fabricated timesheet entries; that mode is removed.

- **Section ownership** is strict:
  - Travis mode writes ONLY B2:H49
  - Ed mode writes ONLY B52:H99
  - Shared mode writes B101:H148 + propagates to B52:H99 (Ed literal values) + B2:H49 (Travis formulas resolve automatically because pre-existing `=B101` refs)
- **Never overwrite formula cells that resolve to non-empty values.** Formula cells that resolve to empty are *writable* — they're placeholders left by `newWeekTab`.
- **Never overwrite existing literal values.** Treat them as someone's manual entry.
- **Travis/Ed back up to `<tab>c` before writing.** If `<tab>c` already exists (prior run), reuse it — don't error. The backup preserves the pre-skill state of any of the three sections.
- **Shared mode does NOT back up.** A fresh `newWeekTab`-style creation has no prior state. The validate sub-action also doesn't back up (it's interactive).
- **All writes go in parallel** — single response with multiple tool calls. Independent writes never need serialization. Sequential writes waste ~1s wall-clock each.
- **Confirmation prompts only when truly needed.** Travis/Ed prefer immediate writes; only stop for: ambiguous input, missing tab, overflow, or validate-mode user input. Don't ask "should I proceed?" after every step.
- **Time format:** cells are 30-min slots. Always 0.5-hour increments.
- **Preserve task name typos and casing verbatim** when given by user. Use mapping tables (in `shared.md`) for canonical joint names only.
- **Break rule + variability are mandatory** for travis/ed modes. See per-mode files for specifics.
- See `patterns.md` for the empirical analysis (Jan–Apr 2026) that drove the day/tier/block defaults.

## Examples

### Travis mode — evidence-driven (default)

```
/hours-maker travis 5/13/26
/hours-maker travis 5/13/26 --cap 20      # report at most 20 hr (Softbank ceiling)
```

Result: runs `evidence_hours.py` over the real Claude-session + browser timestamps
for that week, places each task at the real time it was worked (concurrency
unfolded to one activity per slot), writes only empty cells in B2:H49, and reports
real-found vs reported hours plus any UNMAPPED activity. See `evidence-placement.md`.

### Travis mode — user-asserted (legacy, must match real work)

```
/hours-maker travis "I worked on the following for week 4/1/2026:
  Cluster API integration with incus (12 hours)."
```

Still supported, but hours must reflect real work and are placed at real times
where evidence exists; missing windows are asked for, never synthesized.

### Ed mode

```
/hours-maker ed 4/29/26 "Federation Stage 4 development (10). RBAC use of informer extensions (8). BGP research for private node (6). Code review and merges (4)"
```

Result: writes 56 slots into B52:H99, late-night-heavy with post-midnight (T6) entries, longer 8-slot blocks allowed, Sun regularly used.

### Shared mode — fill

```
/hours-maker shared
```

Creates next week's tab from the latest one, queries Outlook for Myriplane + Softbank events, writes joint area, propagates Ed (literal) — Travis auto-updates via existing formulas.

### Shared mode — validate

```
/hours-maker shared 5/6/26 validate
```

Reads existing joint area, computes calendar plan, classifies cells (matched/missing/extra/conflict), asks the user which differences to apply, writes selected changes.
