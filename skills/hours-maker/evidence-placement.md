# Evidence-driven placement (the core method for Travis & Ed modes)

This is the heart of the skill. Travis/Ed hours are **derived from real activity
timestamps**, not synthesized. It mirrors what shared mode already does with
calendar events — except the evidence here is your Claude-Code session transcripts
and browser history (you've stated this is how ~80–90% of your real work happens).

## Why this exists (read once)

NEDO's form is rigid: **one activity per 30-minute slot**, activities in
**contiguous blocks**, 30-minute granularity. But real work is concurrent — you
run 2–3 Claude sessions at once, so a single wall-clock 30-min block can contain
several activities. This method takes the real, concurrent timeline and *unfolds*
it into NEDO's one-activity-per-slot grid. The **total reported time equals real
human wall-clock working time** (concurrent task-time collapsed to human time), so
the result is always conservative — at or below what you actually put in.

The old approach (pseudorandom tier placement + synthetic breaks, see
`patterns.md`) is **deprecated as a placement source**. It invented *when* work
happened. Do not use it to place hours. `patterns.md` survives only as a
style/format reference and a last-resort layout aid (see Fallback below).

## Integrity rules (hard — never violate)

1. **Only place a slot where there is real evidence of work in that 30-min wall-clock window.** No invented windows. No RNG-chosen days/times.
2. **Total placed time = real human working time, never more.** `--cap` can only trim *down* (e.g. a Softbank reporting ceiling). It can never pad up.
3. **Never auto-assign UNMAPPED activity to a task.** Anything not in `project_task_map.json` is reported separately for the user to decide. Non-Izuma projects (e.g. personal repos) must not become NEDO hours.
4. **If a real activity has no timestamp evidence** (whiteboard, phone, in-person), do NOT synthesize it. Ask the user for the actual window, or leave it out.

## Inputs

- `--week M/D/YY` — the Wednesday tab name / week start.
- `--section travis|ed` — which person's row band.
- `--map project_task_map.json` — project/domain → NEDO-task mapping. **The user owns this file**; it is the only place project→task judgement lives. Generic NEDO names only (no machine names/IPs).
- `--browser` — also use Chrome/Firefox history (recommended; it's the research half of the work).
- `--cap HOURS` — reporting ceiling; trims proportionally down to it. **Default 30.**
  Softbank's downstream NEDO report truncates anything over **30 h/week**, so the
  skill caps at 30 unless the user overrides (`--cap N`, or `--cap 0`/`--nocap` to
  place the full real total). This is honest downward truncation the user has
  authorized — it represents the reportable portion of real work, never inflates.
- `--occupied occ.json` — `[[col,row],...]` joint/meeting cells to avoid (from Step 3's writability read).
- `--tz` — local UTC offset (CDT = −5 May–Oct, CST = −6 Nov–Mar).

## Procedure

Run from the skill directory:

```
# 1. See what was really worked, when (sanity check before placing):
python3 evidence_hours.py timeline --week <tab> --section <travis|ed> \
        --map project_task_map.json --browser

# 2. Produce evidence-driven write-ranges:
python3 evidence_hours.py place --week <tab> --section <travis|ed> \
        --map project_task_map.json --browser \
        --occupied <occ.json> [--cap <hours>] --json /tmp/hm_place.json
```

What `place` does internally:
1. **Timeline** — bucket every Claude/browser timestamp into its real (day-column, 30-min row). Each cell collects the *set* of tasks active then (concurrency preserved).
2. **Drop occupied** — remove joint/meeting cells (already owned by shared mode).
3. **Quota** — each task's quota ∝ how many real slots it appears in, normalized so the sum = total real working slots (or `--cap`). This splits concurrent windows proportionally to where attention actually went.
4. **Assign** — walk each day in time order; give each real working slot to one task, preferring to continue the previous slot's task (contiguity → NEDO's "synchronous" rule), else the active task with the most remaining quota.
5. **Group** — merge contiguous same-task slots into write-ranges `(col, startRow, endRow, task)`.

The output ranges are then written by the mode file's write step — **only into cells the writability map (Step 3) marked empty.**

## Notes on faithfulness

- **Singles are allowed.** If you genuinely worked one 30-min slot on a task, a 1-row block is the truth. (The old skill banned singles only to make RNG output look hand-entered — irrelevant now.)
- **After-midnight / early-morning slots are valid** if the logs show work then. Real timing wins over the old "skip rows 2–19" convention.
- **Some fragmentation is real.** Heavy concurrency produces interleaved blocks; that reflects what happened. Don't smooth it into tidy fiction.
- **`--cap` for Softbank under-reporting:** trimming the total down to a ceiling is the user's call as the person attesting. The skill only ever reduces, proportionally, from the real total.

## Fallback (only when there is genuinely no timestamp evidence)

If `timeline` returns little/nothing for a week the user says they worked:
1. **Ask the user** for the real day(s)/time-window(s) and tasks. Place those.
2. Only then, as a pure layout aid *within a user-confirmed window*, may you use `patterns.md`'s tier ordering to sequence blocks — never to invent the window itself.
Never fill a week from `patterns.md` alone. That is synthesis, and it's prohibited.
