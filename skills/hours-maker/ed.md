# Ed mode workflow

Writes Ed's private work to **B52:H99** in the target tab. Placement is
**evidence-driven** via [evidence-placement.md](evidence-placement.md), run with
`--section ed` (row band B52:H99, row N → `(N-52)*30` min). Ed's "late-night /
post-midnight" character is no longer a hardcoded RNG weighting — it falls out
naturally because the engine places work at the **real times Ed's sessions
happened**, which for Ed skew late. Read evidence-placement.md first; its
integrity rules govern.

## Step 1e — Parse input

Same as Travis Step 1 (see `travis.md`). Tab name + tasks.

## Step 2e — Read the target tab

Call `mcp__workspace-mcp__read_sheet_values` with `range_name: '<tab>'!A1:H99`, `include_formulas: true`. Reading through row 99 captures Ed's section in one call.

## Step 3e — Build the writability map for B52:H99

Same writability rule as Travis: **writable iff rendered value is empty**.

Coordinate convention:
- B=Wed … H=Tue (same column mapping).
- Ed row N → time `(N-52)*30` min. **R52 = 0:00, R74 = 11:00, R99 = 23:30.**

## Step 4e — Evidence-driven placement

Identical to Travis Step 4 but with `--section ed`. The engine reads Ed's real
session/browser timestamps and places work at the real times. Ed's evidence must
be attributable to Ed — if you are reconstructing Ed's hours, the project→task map
and the session set must be Ed's, not Travis's. If Ed's activity isn't in *these*
local logs (different machine/account), use the Fallback in evidence-placement.md:
ask Ed/the user for the real windows; never synthesize.

```
python3 evidence_hours.py timeline --week <tab> --section ed --map project_task_map.json --browser
python3 evidence_hours.py place    --week <tab> --section ed --map project_task_map.json --browser \
        --occupied /tmp/hm_occ.json [--cap <hours>] --json /tmp/hm_place_ed.json
```

## Step 5e — Cap / under-report (downward only)

Same as Travis Step 5: placed total = real working time; `--cap` only trims down;
never pad above evidence.

## Step 5.5e — Back up the tab

Same as Travis Step 5.5. If `<tab>c` exists from a prior Travis run, reuse — don't error.

## Step 6e — Write to the sheet (in parallel)

Same as Travis Step 6 but read ranges from `/tmp/hm_place_ed.json` and write into
**B52:H99** (the engine already emits Ed-band rows because of `--section ed`).

**Critical: issue all writes in a single response with multiple parallel tool calls.** Sequential writes waste wall-clock time.

## Step 7e — Report

Same format as Travis Step 7.
