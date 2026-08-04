# Shared mode — validate sub-action

Invoked as `/hours-maker shared <tab> validate` (or `verify`/`check`/`audit`). Tab arg is **required** — validate never creates a tab.

See `shared.md` for the canonical-name map, overlap rules, and pre-validation procedure (used by Step 2v).

## Step 1v — Parse & sanity-check

1. Extract `<tab>` and confirm it exists via `mcp__workspace-mcp__get_spreadsheet_info`. If missing, stop.
2. **Run the shared pre-validation** (see `shared.md`) on `<tab>`. If A1 doesn't resolve to its name's date, stop and ask the user to fix A1 before validating. Otherwise the calendar window we compute (from the tab name) won't match the dates actually in row 1, and the diff is meaningless.

**Optimization:** combine the A1 pre-validation read with Step 3v's joint read into one call: read `'<tab>'!A1:H148` (with `include_formulas=true`) once. Extract A1 for pre-validation; extract B101:H148 for the joint actual map. Saves one round-trip.

## Step 2v — Compute "calendar plan"

Same as `shared.md` Steps 2s + 3s + 4s:
- Establish week range from tab name (Wednesday → +7 days, Central Time).
- **Two parallel Outlook searches** (Myriplane + Softbank).
- Map each event to (col, row, canonical_name) — one entry per 30-min slot.

Output: `calendar_plan = dict[(col, row)] → canonical_name`.

## Step 3v — Read "joint actual"

If you already did the combined read in Step 1v, extract B101:H148 from that. Otherwise read `'<tab>'!B101:H148`.

Build `joint_actual = dict[(col, row)] → existing_text` (omit empty cells).

## Step 4v — Classify each cell

For each (col, row) in the union of `calendar_plan` ∪ `joint_actual`:

| Calendar | Joint | Classification |
|---|---|---|
| X | X | **MATCHED** |
| X | empty | **MISSING** (add X?) |
| X | Y (≠X) | **NAME CONFLICT** |
| nothing | Y | **EXTRA** (keep or remove?) |

Group adjacent rows in same column with same value into **blocks** for display (separately by category).

## Step 5v — Present diff & ask user

```
Validating joint area of <tab> against calendar (window: <start> – <end> CT):

✓ MATCHED (N blocks):
  Wed 11:30       (B124)          Myriplane Standup
  ...

⚠ MISSING (N blocks) — calendar has these, joint doesn't:
  [1] Thu 12:00       (C125)          Myriplane Standup       ← cal: Myriplane standup / dive deep
  ...

⚠ EXTRA (N blocks) — joint has these, calendar doesn't:
  [3] Tue 11:30       (H124)          Myriplane Standup       (no matching event)

⚠ NAME CONFLICT (N cells):
  [4] Wed 11:30       (B125)          joint: "Myriplane Discussion"   cal: "Myriplane meeting"
```

Then ask via `AskUserQuestion`, batched by category:

1. **Missing block question** (if any): "Add the missing entries?" — options: Add all (recommended) / Add specific (user lists numbers) / Add none.
2. **Extra block question** (if any): "Remove the extras?" — options: Keep all / Remove all / Remove specific.
3. **Conflict resolution** (max 4 at once): per conflict, options Keep current / Use calendar / Other (specify). Paginate if >4.

If user picks "specific", parse their reply (e.g., "1, 3, 4") into a set of indices.

## Step 6v — Apply selected changes (in parallel)

For each accepted action:

- **Add (missing block)**: write canonical name to joint range. Also write Ed mirror cell (literal, offset −49 from joint row). **Don't touch Travis** — its `=B101` formula auto-resolves. (Fall to legacy if Travis formulas aren't present; see `shared.md` Step 6s.)
- **Remove (extra block)**: clear joint cell with `clear_values=true`. Also clear the Ed mirror cell.
- **Conflict — use calendar / Other**: overwrite joint cell with new value. Also overwrite Ed mirror cell.
- **Conflict — keep current**: no-op.

**Critical: issue all writes in a single response with multiple parallel tool calls.** Sequential writes waste wall-clock time.

After writes, summarize:

```
✓ Applied changes to <tab>:
  Added: N blocks
  Removed: M blocks
  Updated (conflicts): K cells
  Matched (no change): J blocks
```

If user picks "Add none" / "Keep all" / "Keep current" for everything: `No changes applied.` and exit.
