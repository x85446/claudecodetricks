---
name: iterate-brainstorm
description: The decide-between-options half of the iterate stack. Investigates the project, its current implementation, and its available toolsets, then presents 3 label-locked options — comparison table first, then a paragraph each (what it is, how to implement it, pros, cons) — with one marked Recommended. The user interrogates, expands, and chooses; on request the skill emits a numbered summary the user hands to $ip. Chat-only: writes no files, no notes, no plans, no branches.
---

<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# $ibs — Reach a decision, don't record one

**Version:** iterate family 5.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


The decision skill for the iterate stack:

## Usage

Argument: <the decision you need help with, e.g. "need some help deciding on a communications protocol">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$i` — ported.
- `$ibs` — ported.
- `$in` — ported.
- `$ip` — ported.
- `$iterate` — ported.
- `$iterate-notes` — ported.
- `$iterate-planner` — ported.
- `$oracle` — ported.

| Skill | Role |
|---|---|
| `$iterate-notes` (`$in`) | **Capture** — take a note for the next plan |
| `$iterate-brainstorm` (`$ibs`) | **Decide** — options in, one decision out |
| `$iterate-planner` (`$ip`) | **Plan** — formalize into the 1a/1b schema |
| `$iterate` (`$i`) | **Execute** autonomously |

This skill exists because the family had no widening stage. `$in` records what the user already thought of; `$ip` structures what they already decided. `$ibs` is the one place the option set gets *bigger* before anything narrows — and then narrows it, in one sitting, on screen.

**It is chat-only.** It writes nothing: no brief, no notes file, no `.claude/` state, no branch, no plan. Its entire output is the conversation. If something needs to persist, that is `$in`'s job (a note) or `$oracle`'s (durable project knowledge) — never this skill's.

## Invocation

`$ibs <the decision>` — e.g. `$ibs need some help deciding on a communications protocol`.

`$ARGUMENTS` is the decision at hand. Nothing else is required; everything below runs automatically without the user asking for it.

If `$ARGUMENTS` is empty, ask for the decision in one line and stop. Do not investigate a topic that hasn't been named.

## The standing procedure

Phases 0–2 run in a single reply, unprompted, in this order. Phase 3 is the user's turn. Phase 4 runs only on request.

### Phase 0 — Investigate (silent)

Do the real work; show none of it. In order:

1. **Understand what this project is about** — read `CLAUDE.md`, the README, the top-level layout. What does it actually do, and for whom?
2. **Evaluate the current implementation** — how is the thing under discussion built *today*? Read the actual code. What exists, what's half-built, what's already been decided implicitly by the current structure?
3. **Evaluate the toolsets** — what's already on hand: the language and its stdlib, existing dependencies, the installed skills, the binaries this project already ships, what the Makefile can already do. An option that reuses what's here beats an equivalent one that adds a dependency.

Read as much as the decision deserves. **None of this gets printed.** Findings surface only where they shape an option, a pro, or a con. The output of investigation is better options, not a report.

### Phase 1 — The comparison table (always first)

Open the reply with the table. Nothing before it but at most one framing line.

```
| # | Option | <axis> | <axis> | <axis> | <axis> |
|---|---|---|---|---|---|
| 1 | <Name> | … | … | … | … |
| 2 | <Name> ★ Recommended | … | … | … | … |
| 3 | <Name> | … | … | … | … |
```

- **Exactly 3 options** by default. Honor an explicit override ("give me 5") and nothing else.
- **Axes are chosen per decision**, from what actually differentiates these three — not a fixed rubric. Effort, reversibility, blast radius, dependency added, who has to maintain it, how it fails. Four to five axes; if two axes say the same thing, cut one.
- **Cells are short** — a few words. The table is for scanning, the paragraphs are for reading.
- **Exactly one option carries `★ Recommended`.**
- **Cost and licence are axes whenever the options differ on them** — `free / MIT` scans in three words and is exactly what decides a lot of these. An option that costs money or carries a copyleft licence (GPL/LGPL/AGPL/SSPL, source-available) says so in the table, in those words, not buried in its paragraph.

### Phase 2 — One paragraph per option

After the table, one section per option, in numeric order, each in this fixed shape:

```
### 1 — <Name>

<What it is, then how to implement it — concretely, in this project's real terms:
 which files, which tool, which existing skill or binary. ~150 words.>

**Pros:** <the real ones, grounded in Phase 0 findings>
**Cons:** <the real ones, including the one that would actually bite>
```

The recommended option's paragraph **opens with why it's recommended**, in one sentence, before describing what it is.

**~150 words per option paragraph.** The investigation was unlimited; this is not. If a paragraph is overflowing, cut detail — the user can ask for depth on any option by its label.

### Phase 3 — Open the floor

Close the reply with one line offering to discuss further, expand any option, or answer questions on particulars. Then **stop**. Do not pick for the user, do not pre-answer, do not keep going.

During discussion:
- **Expand on N** → depth on that option only, still on screen, still no files.
- **Add an option** → it gets the next unused number. Never renumber, never reuse.
- **Revise an option** → its label stays exactly as it was.
- **Challenge the recommendation** → engage honestly. Change the `★` if the user's argument is better; say plainly that it moved and why.

### Phase 4 — The numbered summary (on request only)

When the user asks to summarize the decision, emit exactly this and nothing else:

```
**Summary N** — <one-line title>

**Decision:** <the chosen option by its locked label, e.g. "2 — Message Bus">
**Why:** <the reasons that actually drove it, including what lost and why>
**How:** <the implementation shape as concluded — concrete enough to plan from>
**Constraints surfaced:** <what the discussion turned up that must survive into the plan>
**Open:** <anything genuinely unresolved, or "none">
```

**Numbering `N`:** scan back through this conversation for the highest `**Summary N**` already emitted; this one is N+1. First summary in a session is `**Summary 1**`. Numbers are **monotonic and never reused** — re-summarizing after more discussion produces a *new* number, which is exactly what makes "the second to last summary" resolvable.

The summary is the handoff artifact. The user carries it to the planner themselves:

> `$ip absorb the last summary` — or `$ip absorb summary 2`

`$ip` reads recent conversation context on its default path, so no special support is needed on the planner side. Do **not** invoke `$ip` yourself; the handoff is the user's move.

## Rules (hard)

1. **Never write a file.** No brief, no notes file, no scratch file, no `.claude/` state, no branch, no commit. The output is the chat. The moment something wants to persist, it belongs to `$in` (a note) or `$oracle` (durable project knowledge) — hand it there and say so in one line.
2. **Never plan and never execute.** No 1a/1b pairs, no steps, no validations, no `/loop`. When the user wants a plan, the answer is the numbered summary plus "hand that to `$ip`" — not a plan written here.
3. **Investigation is unlimited; screen output is not.** Phase 0 never gets dumped. ~150 words per option paragraph. This is the rule `$iterate-notes` lost when its research appendix turned depth-on-screen into depth-in-a-bin nobody read — do not reintroduce that bin here in any form.
4. **Labels are locked for the session.** `1 — <Name>` never renumbers, never renames, never gets recycled, even when options are revised, replaced, or added to. The user references options by number or name and both must keep resolving to the same thing an hour later.
5. **Free and permissive is the default position.** Never hand the user a paid service or a copyleft dependency as `★ Recommended` unless that option's paragraph carries the argument: the free/permissive equivalent named, the specific capability it lacks, and whether the licence obligation actually attaches here (linking a library, or just running a container). Absent that argument, the permissive option wins and the paid one is a row in the table with its price in the cell. Options the user cannot adopt without a credit card or a lawyer are not really three options.
6. **Exactly one `★ Recommended`, always present.** Presenting three options with no position is the status check the user's standing rules forbid. Take a position; the decision still belongs to them. If the recommendation is genuinely close, say which axis decided it.
7. **Generate before asking.** No clarifying-question round ahead of the options — investigate and produce the table. At most ONE question, only when the answer changes which options are worth generating at all, and only inside the framing line.
8. **The user selects.** Recommend, argue, defend — but never announce a decision on their behalf and never move to Phase 4 unasked.
9. **Not a note-taker.** If the ask is capture ("take a note", "remember that"), say so in one line and route to `$in`. If the ask is already-decided formalization, route to `$ip`.
10. **One sitting, not a streak.** `$in` is fired rapid-fire; this is not. One invocation opens one decision and stays with it through discussion. A genuinely new decision is a new `$ibs` invocation, with its own option numbering — but summary numbering stays monotonic across the whole session.
11. **Standing risk acceptance carries.** If the user has said "this isn't prod" / "accept the risk" / "do it everywhere", the cons sections reflect that reality instead of re-raising it as a blocker.

## Example

> `$ibs need some help deciding on a communications protocol`

Investigates the repo (what it is, how components talk today, what's already vendored), then replies with a 3-row table across effort / latency / debuggability / added dependency, `★ Recommended` on row 2, three ~150-word paragraphs, and a closing offer to expand. The user argues for option 3, asks two questions, then says "summarize" — and gets `**Summary 1**`, which they hand to `$ip`.

## `version`

`version` (or "what version") on **any** iterate skill reports the same thing —
the family version, because the stack is versioned as one unit:

```
iterate family 5.0.0
iterate-run iterate-v3.3 (commit 4dd09ec5, built 2026-08-27_17:02:20)
```

Run `iterate-run version` for the second line — a real installed binary, never a
recalled string. If members disagree, say so and name them: drift inside the
family is a defect, not a state, and `skillctl family iterate set X.Y.Z` is the
only correct way to bump.
