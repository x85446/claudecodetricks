# Travis mode workflow

Writes Travis's private (non-joint) work to **B2:H49** in the target tab.
Placement is **evidence-driven** — derived from real Claude-session + browser
timestamps via `evidence-placement.md`, NOT synthesized. Read
[evidence-placement.md](evidence-placement.md) before running; its integrity
rules override anything here.

## Step 1 — Parse input & pick the input mode

Extract the **tab name** in `M/D/YY` format (no leading zeros) from the args.

Then decide the input mode:

- **Evidence mode (default).** No explicit hours given, or the user asks to
  "fill from my work / sessions / history". Hours and timing come from the logs
  (Step 4). The user may pass a `--cap` ceiling (e.g. Softbank under-reporting).
- **User-asserted mode (legacy).** The user supplies explicit `task (N hours)`
  pairs. Only allowed when those correspond to real work; you still place them at
  real times where evidence exists, and ask for the window where it doesn't
  (never synthesize — see evidence-placement.md Fallback).

If the week is ambiguous, stop and ask. Do not guess.

## Step 2 — Read the target tab

Spreadsheet ID: `1T6dw_I7Vz59pjeofvuFZWIbhven8SI-UaxemAqmXO-k`

Call `mcp__workspace-mcp__read_sheet_values` with `range_name: '<tab>'!A1:H49`, `include_formulas: true`, `user_google_email: travis.mccollum@gmail.com`.

If the tab doesn't exist, stop and tell the user the tab is missing.

## Step 3 — Build the writability map

Treat each cell in B2:H49 as **writable** iff its rendered value is the empty string (regardless of whether that emptiness comes from a literal blank or from a formula referencing an empty joint cell).

**Protected** = cells whose rendered value is non-empty (joint content or someone's literal text).

Coordinate convention:
- **B=Wed, C=Thu, D=Fri, E=Sat, F=Sun, G=Mon, H=Tue**
- Row N → time `(N-2)*30` minutes after midnight. R2 = 0:00, R49 = 23:30.

## Step 4 — Evidence-driven placement (replaces the old RNG allocator)

Do **not** synthesize a distribution. Run the evidence engine, which buckets your
real Claude-session + browser timestamps into the grid and unfolds concurrency
into NEDO's one-activity-per-slot format. Full method: [evidence-placement.md](evidence-placement.md).

1. Build the occupied-cells file from Step 3's writability map (the joint/meeting
   cells already filled): write `[[col,row],...]` to `/tmp/hm_occ.json`.
2. Confirm the project→task map reflects this user's attribution:
   `project_task_map.json` (edit with the user if any relevant project is UNMAPPED).
3. (Recommended) Show the real timeline first so the user can sanity-check:
   ```
   python3 evidence_hours.py timeline --week <tab> --section travis \
           --map project_task_map.json --browser
   ```
4. Produce write-ranges from evidence:
   ```
   python3 evidence_hours.py place --week <tab> --section travis \
           --map project_task_map.json --browser \
           --occupied /tmp/hm_occ.json [--cap <hours>] --json /tmp/hm_place.json
   ```
   Use `--tz -5` for May–Oct (CDT), `-6` for Nov–Mar (CST).
5. The script prints, and writes to `/tmp/hm_place.json`, the `(col,start,end,task)`
   ranges — each corresponding to a real 30-min window where that task was worked.
   These are what Step 6 writes (into empty cells only).

**UNMAPPED report:** if the script lists UNMAPPED projects, stop and ask the user
whether each maps to an Izuma/NEDO task (add to the map) or is excluded
(e.g. personal/non-Izuma repos). Never auto-assign them.

## Step 5 — Cap / under-report (downward only)

The placed total equals the real human working time the logs show. If the user
wants to report less (e.g. a Softbank ceiling), re-run `place` with `--cap <hours>`;
it trims proportionally down to that ceiling. Report both numbers:

```
Real working time found: X hr.  Reporting (cap): Y hr.
```

Never pad above the real total. Never invent slots to hit a target. If the user
wants *more* than the evidence shows, that is the Fallback case in
evidence-placement.md: ask for the real additional windows; do not synthesize.

## Step 5.5 — Back up the tab

Call `mcp__workspace-mcp__get_spreadsheet_info` to check tab list. If `<tab>c` already exists, proceed (it's from a prior run; reuse). Otherwise call `mcp__workspace-mcp__create_sheet` with `source_sheet_name=<tab>`, `sheet_name=<tab>c` to duplicate.

## Step 6 — Write to the sheet (in parallel)

Write to the **original** tab, not `<tab>c`.

Read the ranges from `/tmp/hm_place.json` (from Step 4). For each
`(col, start, end, task)`, and **only for cells the Step 3 writability map marked
empty**, call `mcp__workspace-mcp__modify_sheet_values` with:
- `range_name: '<tab>'!<col><start>:<col><end>`
- `values: [[task name], [task name], ...]`

If any range overlaps a protected/occupied cell, split it and write only the empty
parts (the engine already excludes occupied cells you passed in Step 4, so this is
a safety check).

**Critical: issue all writes in a single response with multiple parallel tool calls.** They are independent — don't serialize.

## Step 7 — Report

```
✓ Backup: <tab>c
✓ Filled <tab> from evidence (Claude sessions + browser):
  Real working time found: X hr   |   Reported (cap): Y hr
  Wed  H hr  -  <tasks>
  ...
Per task: <task> Nh, <task> Nh, ...
UNMAPPED (excluded, for your awareness): <project> Nh, ...
```

Always surface the **real-vs-reported** numbers and any UNMAPPED activity, so the
user sees exactly what was counted and what wasn't.
