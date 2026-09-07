# Shared mode workflow

Calendar-driven population of the joint area **B101:H148** + propagation to Ed (literal values in B52:H99) and Travis (auto-updates via existing `=B101` formulas in B2:H49).

Two sub-actions, distinguished by whether the keyword `validate` (or `verify`/`check`/`audit`) appears anywhere in args:

| Invocation | Sub-action | What it does |
|---|---|---|
| `/hours-maker shared` | **fill** | Creates next week's tab, fills joint from calendar, propagates. |
| `/hours-maker shared <tab>` | **fill** | Same, on an existing tab. |
| `/hours-maker shared <tab> validate` | **validate** | See `validate.md` — never creates a tab. |

## Pre-validation: tab name vs A1 resolved date

Before any shared work runs, validate that the source/target tab's name matches the date A1 resolves to. Catches date-chain drift (e.g., a tab named `5/6/26` with `A1='4/22/26'!A1 + 7` resolving to `4/29`).

For the tab the workflow will touch:
1. Read `'<tab>'!A1` with `include_formulas=true`. Get formula and resolved date.
2. Parse `<tab>` as `M/D/YY` → expected date.
3. Format the resolved A1 as `M/D/YY` (no leading zeros).
4. If they don't match, **stop**:
   ```
   ⚠ Date mismatch on '<tab>':
     - Tab name implies <expected>
     - A1 ('<formula>') resolves to <actual>
   Likely fix: A1 = '<prev_week_tab>'!A1 + 7
   Reply when fixed.
   ```
5. Only proceed once the user confirms or fixes A1.

## Step 1s — Determine the next week and create the tab

If a tab name was provided, skip to Step 3s (pre-validation already ran on the supplied tab).

Otherwise:
1. Call `mcp__workspace-mcp__get_spreadsheet_info`. Filter sheet names to regex `^\d{1,2}/\d{1,2}/\d{2}$`.
2. Pick latest by chronological date. **Run pre-validation on it** — if A1 doesn't match name, stop and ask user to fix (otherwise we propagate bad date to the new tab).
3. Compute `new_tab` = latest + 7 days, formatted `M/D/YY` (no leading zeros).
4. Duplicate: `mcp__workspace-mcp__create_sheet(source_sheet_name=<latest>, sheet_name=<new_tab>)`.
5. **In parallel** (single response, multiple tool calls), clear the three data zones and set A1:
   - `modify_sheet_values clear_values=true` on `'<new_tab>'!B2:H49`
   - `modify_sheet_values clear_values=true` on `'<new_tab>'!B52:H99`
   - `modify_sheet_values clear_values=true` on `'<new_tab>'!B101:H148`
   - `modify_sheet_values` on `'<new_tab>'!A1` with value `='<latest_tab>'!A1 + 7`
6. Verify A1 resolves correctly: `read_sheet_values '<new_tab>'!A1:H1` — confirm Wed date matches the new tab name.

For freshly-created tabs, you can **skip Step 6s's Travis-formula sample check** — the duplicate preserves Travis's `=B101` formulas from the source tab. Only user-supplied tabs need that check.

## Step 2s — Establish the week's date range

The new (or supplied) tab's Wednesday defines the week. In **Central Time**:
- `week_start` = Wednesday 00:00 CT
- `week_end` = next Wednesday 00:00 CT (exclusive)

Format as ISO 8601 with offset (e.g., `2026-05-13T00:00:00-05:00` for CDT).

## Step 3s — Query Outlook calendar

Verify `mcp__claude_ai_Microsoft_365__outlook_calendar_search` is loaded (see SKILL.md tool-availability section).

Make **two parallel calendar searches** in a single response:
1. `query="Myriplane"`, `afterDateTime=<week_start>`, `beforeDateTime=<week_end>`, `limit=25`, `order="oldest"`
2. `query="Softbank"`, same window.

Repeat with `offset` if `moreResults=true`.

**Filter** each event:
- `isCancelled` is false
- `attendees` contains `travis.mccollum@izumanetworks.com`
- Title matches `/myriplane|softbank/i`

Deduplicate by event `id`.

## Step 4s — Map events to joint cells

For each event:

1. **Convert UTC → Central** (CDT in summer, CST in winter; the spreadsheet locale is Central). Outlook returns UTC ISO 8601.
2. **Column** = day-of-week in Central → B=Wed, C=Thu, D=Fri, E=Sat, F=Sun, G=Mon, H=Tue. Events spanning midnight split into two segments — one in current column, one in next.
3. **Row range**:
   - `start_row = 101 + (local_hour * 2) + (local_minute // 30)`
   - `slots = ceil(duration_minutes / 30)`
   - `end_row = start_row + slots - 1`
4. **Canonical name** — map title case-insensitively:

| Title pattern | Canonical |
|---|---|
| `Myriplane & DM Daily Standup` (any variant) | `Myriplane Standup` |
| `Myriplane standup / dive deep` or `/planning` | `Myriplane Standup` |
| `Myriplane sync` | `Myriplane meeting` |
| `Myriplane - Code Reviews` or `Code review` (Myriplane series) | `Code review` |
| `Myriplane standup / dedicated` | `Myriplane Standup` |
| any other `Myriplane *` | `Myriplane meeting` |
| `SoftBank Weekly Meeting` | `SoftBank Weekly Meeting` |
| `SoftBank PMO Meeting` | `SoftBank PMO Meeting` |
| `Internal Softbank sync` or `Internal Softbank meeting` | `Internal Softbank sync` |
| any other `Softbank *` | `Softbank meeting` |

**Overlap resolution** when events map to overlapping cells in the same column:

1. **Longest-duration event wins all its slots.** Shorter overlapping events get only non-overlapping slots.
2. **If a shorter event has zero non-overlapping slots, drop it entirely** and flag in the report.
3. Tie-breaker (same duration): recurring > one-off; then specificity (PMO > Weekly > generic Softbank; Standup > Sync > generic Myriplane).

## Step 5s — Write joint cells (in parallel)

For each (col, start_row, end_row, name) tuple, call `mcp__workspace-mcp__modify_sheet_values`:
- `range_name: '<new_tab>'!<col><start_row>:<col><end_row>`
- `values`: 2D list of N copies of `[[name]]`

**Critical: issue all joint writes in a single response with multiple parallel tool calls.** Independent — don't serialize.

These cells are empty (cleared in Step 1s.5 or empty in user-supplied tab). No conflict checking needed.

## Step 6s — Propagate joint to Ed (Travis auto-updates)

For tabs created via `newWeekTab` (or duplicated from one — including the Step 1s output), the Travis section already has `=B101` / `=C102` / … formula refs in every B2:H49 cell. Joint changes auto-propagate to Travis. **Don't write Travis.**

The Ed section holds literal values — must be explicitly written.

**Procedure:**

1. **For freshly-created tabs (Step 1s flow), skip the formula check** — duplication preserves Travis formulas. For user-supplied tabs, sample-check `'<tab>'!B2` for a formula. If it doesn't start with `=B`, fall to legacy fallback (write Travis formulas too).
2. **Write Ed mirror cells in parallel.** For each joint cell at row N with non-empty value, write the literal value to `<col><N-49>` (Ed row offset = −49 from joint row). Single response, multiple parallel tool calls.

**Legacy fallback** (only when B2 has no formula — pre-newWeekTab tabs):
- Write Ed literal AND Travis formula `=<col><52+r>` with `value_input_option=USER_ENTERED`. Only write empty Travis cells.

**Manual fallback**: tell user to open the new tab and click **IZ Toolz → Hours Maker → Copy Joint (black) to Travis and Ed sections**.

## Step 7s — Report

```
✓ Created tab: <new_tab>  (cloned from <latest_tab>, A1 = +7 days → <date headers>)
✓ Calendar events found: N (M Myriplane, K Softbank, after dedupe)
✓ Joint area written: X cells across <D> days
✓ Ed mirror: Y literal-value writes; Travis auto-updates via existing formula refs

Events placed:
  Wed  <start>–<end>  (<range>)  <canonical name>
  ...

Events dropped/merged due to overlap (if any):
  Tue 23:00–00:00  Myriplane - Sprint Planning  (overlapped by SoftBank Weekly)
```

If no calendar events matched: `"No Myriplane/Softbank meetings found for <new_tab> week. Joint area left empty."`
