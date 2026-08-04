# Hours Maker — empirical patterns (1/7/26 → 3/25/26)

> ⚠️ **DEPRECATED AS A PLACEMENT SOURCE.** This file documents a statistical
> distribution model (day shares, time-band tiers, RNG jitter, synthetic breaks).
> Using it to *place* hours invents when work happened — that is synthesis, and
> the skill no longer does it. **Placement is now evidence-driven** from real
> timestamps; see [evidence-placement.md](evidence-placement.md), which governs.
>
> This file is retained only for: (a) sheet-layout reference (coordinates, row↔time
> maps, recurring joint slots) and (b) a last-resort *sequencing* aid **within a
> user-confirmed real working window** when no timestamp evidence exists. It must
> never be used to fabricate the window itself.

Reference data extracted from 12 completed weekly tabs.

## Spreadsheet layout

- **Spreadsheet ID:** `1T6dw_I7Vz59pjeofvuFZWIbhven8SI-UaxemAqmXO-k`
- **Tab naming:** `M/D/YY` (no leading zeros), e.g. `4/1/26`. Tabs are weeks starting Wednesday.
- **Travis section:** `B2:H49` (48 rows × 7 cols)
- **Ed section (blue):** `B52:H99`
- **Joint section (black):** `B101:H148`
- **Row 1:** date headers (formulas `=DATE(...)` and `=B1+1`)
- **Col A:** time labels (formulas `=B1`, `=A2 + 1/48`)
- **J3:** total-hours formula `=calculateTimeSpent(...)` summing B–H

The Apps Script `copyBlackToGreenBlue` in `scripts/hours_maker/Code.js` writes a formula reference into Travis cells *only when the joint cell is non-empty*. So a Travis cell is empty (writable) iff the corresponding joint cell is empty.

## Day → column mapping

| Col | Day |
|---|---|
| B | Wed |
| C | Thu |
| D | Fri |
| E | Sat |
| F | Sun |
| G | Mon |
| H | Tue |

## Row → time mapping

`row N = 30 minutes × (N - 2)` after midnight. So:

| Row | Time | | Row | Time |
|---|---|---|---|---|
| 2 | 0:00 | | 26 | 12:00 |
| 18 | 8:00 | | 36 | 17:00 |
| 20 | 9:00 | | 44 | 21:00 |
| 24 | 11:00 | | 49 | 23:30 |

Formula: `row = floor(hour*2) + floor(minute/30) + 2`.

## Weekly totals (12-week sample)

| Week | Total hr | Notes |
|---|---:|---|
| 1/7 | 23 | |
| 1/14 | 23 | Heavy Sun (7 hr) |
| 1/21 | 23.5 | |
| 1/28 | 25.5 | |
| 2/4 | 16 | Light week |
| 2/11 | 21 | |
| 2/18 | 21.5 | |
| 2/25 | 27 | Sat 2.5 + Sun 7 |
| 3/4 | 29.5 | |
| 3/11 | 26.5 | |
| 3/18 | 27 | |
| 3/25 | 25.5 | |

**Mean ≈ 24 hr/week** (range 16–29.5). User-stated target is ~20 hr; recent trend is upward.

## Day-of-week distribution (average hr)

| Day | Avg hr | Share |
|---|---:|---:|
| Thu | 7.0 | 26% |
| Wed | 6.0 | 23% |
| Mon | 4.0 | 15% |
| Tue | 4.0 | 15% |
| Fri | 3.5 | 13% |
| Sun | 1.5 | 6% |
| Sat | 0.5 | 2% |

**Weekend share ≈ 8%.** Of 12 weeks: 3 had a real Sunday work session (5–7 hr), 1 had a real Saturday session. Most weekends are 0 hr.

## Time-of-day priority (where to place blocks)

Revised after the 4/1/26 run feedback: Travis does **night work often**, so 21:00–23:30 is now weighted equal-to afternoon, ahead of late-afternoon and early-evening bands.

| Tier | Rows | Time | Weight | Notes |
|---|---|---|---|---|
| 1 | 28–31 | 13:00–15:30 | 4 | Afternoon deep-work core |
| 2 | 44–49 | 21:00–23:30 | 4 | **Night work** — frequent when joint slots are empty (was previously underweighted) |
| 3 | 20–25 | 09:00–12:30 | 2 | Morning |
| 4 | 32–35 | 16:00–17:30 | 1 | Late afternoon |
| 5 | 36–43 | 17:30–21:00 | 1 | Early evening — lowest priority of the active bands |
| — | 2–19 | 00:00–9:00 | 0 | Skip — almost never used |

Selection method: weighted-random per the `Weight` column, with the constraint that consecutive blocks on the same day prefer different tiers.

## Block sizing observations

- Most blocks are **2–7 rows (1–3.5 hr)** in past tabs. Skill caps blocks at 5 rows (2.5 hr) due to the break rule.
- Standalone single-row (30-min) entries: standups, brief check-ins, calls (joint area only — skill avoids these).
- Common joint single rows: `Myriplane standup` at R24 (11:00) and R49 (23:30).
- "Focused work" blocks in past tabs: 4–6 rows (2–3 hr).
- Multi-day same-task blocks are common — e.g., 1/21 "CAPI GKE research" on Wed AND Thu.

## Break rule (30 min every 2.5 hr)

**Hard rule:** no two adjacent blocks in the same column without ≥ 1 writable slot of gap (30 min) between them. Equivalent: max **5 contiguous filled rows** per column. This applies regardless of whether the blocks are the same task or different tasks.

In past tabs this rule was implicitly followed because the joint area broke up Travis's afternoon work. With the new skill placing blocks freely, the rule must be enforced explicitly.

## Columnar grouping style

A "block" = N adjacent rows in the same column with identical task text. The visual effect in the sheet is a vertical run of the same words.

Examples from past weeks:

```
Row 19  CAPI AZure research
Row 20  CAPI AZure research
Row 21  CAPI AZure research
Row 22  CAPI AZure research      ← 4-row block = 2 hr
Row 23  (empty)
Row 24  Myriplane standup        ← 1-row block = 30 min (joint)
```

Multi-day blocks (same text, different columns):
```
Col B (Wed)         Col C (Thu)
CAPI GKE research   virtual cluster research on GKE
CAPI GKE research   virtual cluster research on GKE
CAPI GKE research   virtual cluster research on GKE
```

## Recurring joint entries (informational)

These show up in most weeks as formulas in Travis cells (already populated by the joint area):

- **R24 (11:00) "Myriplane standup"** — recurring on Wed, Thu, Mon, Tue
- **R49 (23:30) "myri standup" / "Myriplane standup"** — recurring on Sun, Mon, Tue
- **Wed R44–48 (21:00–23:00) "Softbank weekly planning meeting"** — most weeks
- **Tue R46–47 (22:00–22:30) "Internal Softbank sync"** — about half of weeks
- **R2 (0:00) "myri standup"** — recurring on Mon, Tue

These are typically NOT places where Travis adds individual work because the joint area already covers them.

## Task naming conventions seen

Lowercase mixed with sentence case, often descriptive:
- `Cluster API integration with incus`
- `CAPI AZure research`
- `dynamically allocating virtual / physical nodes`
- `Loki/Grafana expermient for myriplane` (typo preserved verbatim across the week)
- `network booting virtual/physical nodes`
- `GKE wrangler clusrter bringup dynamic` (typo preserved)
- `Softbank Team meeing` (typo preserved)
- `Myriplane CAPI analysis`
- `K8s Wan node support (private node` (truncated/unclosed-paren style preserved)

**Lesson:** when the user provides a task name, write it **exactly as given**, including any typos or odd casing. Don't auto-correct.

## Anti-patterns (4/1/26 retrospective)

The first run of the skill on 4/1/26 produced a placement that the user flagged as too uniform. Things that went wrong and how to avoid them:

- **Every day started at R28 (13:00).** Fix: jitter start rows within tier ranges; track which tiers each day has used and prefer unused ones.
- **No 30-min breaks.** Multiple blocks were placed back-to-back filling R28→R37 with no gap. Fix: enforce the break rule explicitly (≥1-slot gap, max 5-row run).
- **Same block size repeated** (4+2+2 on Wed, 4+4+2 on Thu). Fix: per-day size variety rule — same size used max twice per day.
- **All work in one tier per day** (Tier 1 only). Fix: multi-band-days rule — ≥6 slots/day → ≥2 tiers; ≥10 slots/day → ≥3 tiers.
- **Night band entirely unused** despite Travis doing night work often. Fix: bump Tier 2 (night) weight to equal Tier 1 (afternoon).

The skill's Step 4d/4e algorithm exists to prevent recurrence. If a future run produces a uniform-looking grid, that's a regression and the algorithm needs another look.

## Ed's profile (B52:H99) — Jan–Apr 2026

Researched from 7 sample tabs (1/7, 1/14, 2/4, 2/25, 3/4, 3/25, 4/15). Ed's workflow differs from Travis's in several material ways:

### Volume & day distribution

- **~30 hr/week** average (range ~26–34 hr). Heavier than Travis (~24 hr).
- Day distribution:

| Day | Avg hr | Share |
|---|---:|---:|
| Wed | 7 | 24% |
| Thu | 7 | 24% |
| Fri | 5 | 17% |
| Mon | 4 | 13% |
| Tue | 4 | 13% |
| Sun | 3 | 10% |
| Sat | 0 | 0% |

**Key differences from Travis:** Sun is genuinely used (10% vs 6%), Fri heavier (17% vs 13%), Sat never used at all (0% vs 2%), Mon/Tue lighter.

### Time tiers (Ed)

Spreadsheet row numbers (these are Ed's actual rows in the sheet, **not** Travis-section rows):

| Tier | Rows | Time | Weight | Notes |
|---|---|---|---:|---|
| T2 | 94–99 | 21:00–23:30 | 5 | **Dominant** — heaviest band by far |
| T6 | 52–55 | 0:00–1:30 | 3 | **Post-midnight (Ed-only)** — continuation of late-night work |
| T1 | 78–81 | 13:00–15:30 | 3 | Afternoon |
| T5 | 86–93 | 17:30–21:00 | 2 | Early evening — used regularly |
| T3 | 70–75 | 9:00–12:30 | 1 | Morning — less used than Travis |
| T4 | 82–85 | 16:00–17:30 | 1 | Late afternoon |

T6 (post-midnight) is **unique to Ed** — Travis almost never has 0:00–1:30 work, but Ed routinely continues past midnight from his late-night sessions.

### Block sizing (Ed — looser than Travis)

- Blocks of **2–8 slots (1–4 hr)** common. Sometimes 7-8 slot contiguous blocks.
- Break rule: ≥1 writable slot (30 min) gap between blocks. Effective rule: **max 8 contiguous rows / 30-min break every 4 hr**.
- Multi-day same-task blocks are very common — e.g., "Federation implementation Phase 2" spans Mon AND Tue in 3/25/26.

### Common Ed task names

Longer and more technical than Travis's. Preserve typos verbatim.

- `Federation implementation Phase 2`
- `Federation stag 3 development` (typo "stag" preserved)
- `Federation Stage 3 testing`
- `Federation Stage 3 Test Development ` (trailing space preserved)
- `Adressing efficiency issues in gossip protocol` (typo "Adressing" preserved)
- `Store layer update - fixes for Informer`
- `Design for RBAC federation`
- `RBAC use of informer`
- `RBAC basic implementation`
- `RBAC prototype work`
- `RBAC work`
- `RBAC update ` (trailing space preserved)
- `Code review`
- `Code review and Merge ` (trailing space preserved)
- `Code merge`
- `Merge code & code review`
- `Network research for virtual node`
- `Networking research for private node use case`
- `CNI research for private nodes`
- `BGP research for private node`
- `Federation support`
- `Meeting prep`
- `Meeting prep / research consolidation`
- `Minor security fixes`
- `Research on federation and gossip protocol`
- `Futher research on networking inside myriplane` (typo "Futher")

### Sample week breakdowns

**2/4/26** (a heavy 34hr week):
- Wed: 0:00-1:30 Federation support (1.5hr), 13:30-14:30 (1hr), 19:30-23:00 (3.5hr), 23:30 (0.5hr) = ~7hr
- Thu: 10:30-12:30 RBAC (2hr), 15:00-16:00 (1hr), 20:00-23:30 (4hr) = ~7hr
- Fri: 0:00-2:00 RBAC (2hr early morning), 19:30-23:00 (3.5hr) = ~5.5hr
- Sun: 17:00-23:30 Min security 6hr block + Merge spillover = ~6.5hr
- Mon: 0:00 standup, 9:00 internal, 13:30-14:30, 19:30-23:30 Merge = ~5hr
- Tue: 19:00-23:30 Merge code = ~4hr

**3/25/26** (a typical 29hr week):
- Wed: 0:00-1:30 gossip protocol research (1.5hr), 12:30-13:30 Code review (1hr), 19:30-21:00 + 22:00-23:30 mix = ~7hr
- Thu: 0:00-1:00 review (1hr), 11:30-13:00 review (1.5hr), 20:00-23:30 RBAC fed + Code (~3.5hr) = ~7hr
- Fri: 0:00-0:30 + 11:30-12:30 + 20:00-21:00 = ~2.5hr (light Fri)
- Sun: 21:30-23:00 BGP research = ~1.5hr
- Mon: 18:30 + 20:30-23:00 Federation = ~3hr (light Mon)
- Tue: 0:00-1:00 perf+Federation, 11:30 bug review, 19:30-23:30 Federation = ~6hr

### Ed anti-patterns (things to avoid replicating from prior buggy runs)

- Don't write to B2:H49 in Ed mode — that's Travis's territory.
- Don't ignore T6 (post-midnight) — Ed's defining feature; using it is a feature, not a bug.
- Don't cap blocks at 5 like Travis — Ed often has 6-8 slot blocks. Cap at 8.

## Joint area (B101:H148) — shared mode source

### Coordinate convention

The joint area is rows 101–148, mapping directly to 0:00–23:30 (R101=0:00, R148=23:30). Same column mapping as Travis/Ed (B=Wed, C=Thu, …, H=Tue).

### Canonical task names observed historically

Across Jan–Apr 2026 the joint area contained these recurring entries (with several variants — the skill normalizes to the form on the right):

| Variants seen | Canonical form |
|---|---|
| "Myriplane standup", "myriplane standup", "Myriplane Standup", "myri standup" | **Myriplane Standup** |
| "Myriplane stand up" (with space) | **Myriplane Standup** |
| "Myriplane meeting", "Myriplane Discussion" | **Myriplane meeting** |
| "myriplane team meeting" | **Myriplane meeting** |
| "Myriplane deep dive" | **Myriplane Standup** (if context implies the daily standup) |
| "Myriplane defect review and implementation strategy" | (write verbatim, no canonical form) |
| "Code review", "Myriplane - Code Reviews" | **Code review** |
| "Softbank Weekly Meeting", "Softbank weekly planning meeting", "Softbank Team meeing" (typo) | **Softbank Weekly Meeting** |
| "Softbank PMO Meeting", "Softbank PMO meeing" | **Softbank PMO Meeting** |
| "Softbank meeting" (generic) | **Softbank meeting** |
| "Internal Softbank sync", "Internal Sync" | **Internal Softbank sync** |
| "Internal Meeting", "Internal myriplane planning" | **Internal Meeting** |

### Recurring time slots

These are the time slots where the joint area is almost always populated (across the sample weeks).

**Joint row → time conversion**: `time_minutes = (row - 101) * 30`, so R101 = 0:00, R124 = 11:30, R145 = 22:00, R148 = 23:30. Earlier draft of this doc was off by one row in this section — corrected below.

| Row | Time | Days populated | Typical entry |
|---|---|---|---|
| R123 | 11:00 | Mon, Tue (since 4/2026) | Myriplane meeting (from "Myriplane sync" cal event) |
| R124 | 11:30 | Wed, Thu, Mon, Tue | Myriplane Standup (recurring) |
| R125 | 12:00 | Wed, Thu | Myriplane Standup extension (when "dive deep" event covers 11:30–12:30) |
| R145 | 22:00 | Wed | SoftBank Weekly Meeting (start) |
| R145–R148 | 22:00–23:30 | Wed (Sun in some weeks) | SoftBank Weekly Meeting / PMO Meeting (1.5–2 hr) |
| R146–R147 | 22:00–22:30 | Tue (some weeks) | Internal Softbank sync |
| R148 | 23:30 | Sun, Mon, Tue | Myriplane Standup (from "Myriplane & DM Daily Standup" cal event) |
| R102–R104 | 0:30–1:30 | Thu (some weeks) | Late-night Softbank continuation |

### Calendar event → canonical map (for shared mode)

The shared-mode skill maps Outlook event titles to canonical joint names using this table (matched case-insensitively, longest-match-wins):

| Event title contains | Canonical name |
|---|---|
| "Myriplane & DM Daily Standup" (any variant) | Myriplane Standup |
| "Myriplane standup / dive deep" | Myriplane Standup |
| "Myriplane standup / planning" | Myriplane Standup |
| "Myriplane sync" | Myriplane meeting |
| "Myriplane - Code Reviews" | Code review |
| any other "Myriplane *" | Myriplane meeting |
| "SoftBank Weekly Meeting" | Softbank Weekly Meeting |
| "SoftBank PMO Meeting" | Softbank PMO Meeting |
| "Internal Softbank sync" | Internal Softbank sync |
| any other "Softbank *" | Softbank meeting |

Excludes anything not matching Myriplane or Softbank (e.g., NEC, EPF). Excludes cancelled events. Requires Travis as an attendee.

### Overlap resolution

When multiple calendar events map to overlapping cells in the same column:

1. **Longest-duration event wins all its slots.** A 2-hr SoftBank Weekly beats a 30-min Standup that ends in the same slot.
2. **Shorter overlapping events get only their non-overlapping slots.** If a 1-hr Sprint Planning overlaps a 2-hr Weekly for both slots, Sprint Planning is dropped entirely.
3. **Drop and flag.** Anything fully overlapped is dropped and listed in the Step 7s report so the user can manually reconsider.
4. **Tie-breaker (same duration)**: recurring beats one-off; within the same series, more specific canonical name wins (PMO > Weekly > generic Softbank; Standup > Sync > generic Myriplane).

Real example (5/13/26 week): Tue 22:00–24:00 SoftBank Weekly (4 slots) won over Tue 23:00–24:00 Sprint Planning (2 slots) and Tue 23:30 Daily Standup (1 slot), both of which were dropped.

### Calendar timing notes

- All Outlook event times come back as UTC. **Convert to Central Time** before mapping to rows (the spreadsheet locale is Central).
- A typical Myriplane standup at "11:00 CST" appears in Outlook as a 30-min event starting at 16:00 UTC (CST) or 17:00 UTC (CDT).
- Events spanning midnight Central should be split into two segments — one in the original day's column, one in the next day's.

## Overflow scenarios seen

Of the 12 weeks, none had user-driven overflow because they were filled by hand. The skill needs to handle this case because it auto-allocates: if the joint area fills slots in Tier 1/2 bands, available capacity for individual work shrinks.

A typical "filled" week leaves ~30–40 writable cells in B2:H49 (out of 336 total). At 0.5 hr per cell, that's a hard ceiling of ~15–20 hr/week of individual work capacity. If the user requests more, the skill must trim and report.
