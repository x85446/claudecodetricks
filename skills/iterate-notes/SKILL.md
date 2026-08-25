---
name: iterate-notes
description: The brainstorm half of the iterate stack — capture and discuss ideas for the NEXT iterate plan, before any formal planning. Triggers on "/iterate-notes", "take a note" (direct capture for the next plan), and on "explore <topic>", "discuss <topic>", "what about <x>" when the conversation is about ideas for upcoming iterate work. Two modes — note mode appends the ask to the current notes file and acks in one line; discussion mode thinks hard but answers curated (direct questions ≤50 words, analysis ≤200 words), parking full depth in the notes file's research appendix and recording every decision reached. Notes live under ./.claude/iterate/notes/ and hand off via "turn these notes into a plan" (routed to /iterate-planner). This skill never plans, never executes, never creates branches.
argument-hint: <a note to take, or "explore/discuss <topic>", or "list notes" / "show notes" / "new notes <topic>">
version: 1.0.1
---
<!-- version: bump on EVERY behavioral change (minor for additions, major for schema/contract changes, patch for wording). -->

# /iterate-notes — Brainstorm the next plan, don't build it

The idea-capture skill for the iterate stack:

| Skill | Role |
|---|---|
| `/iterate-notes` | **Brainstorm** — collect ideas, discuss them tersely, record decisions |
| `/iterate-planner` (`/ip`) | **Plan** — formalize into the 1a/1b schema |
| `/iterate` | **Execute** autonomously |

This skill does NO formal plan investigation — no oracle merge, no access preflight, no teamify, no feature branch, no step/validation pairs. It is a curated conversation with a persistent memory file. The formal machinery starts only when the user says "turn these notes into a plan" (that's `/iterate-planner`'s notes-to-plan op, not yours).

## Notes files

    ./.claude/iterate/notes/<topic>.md      (project-local; create dirs on first touch)
    ./.claude/iterate/notes/current         (pointer: name of the current notes file)

`<topic>` is a short kebab-case slug you infer from the first note's subject (e.g. `dashboard-redesign`, `vm-cleanup`) — not an animal name; animal names are assigned by `iterate-run` at PLAN creation, and a notes file isn't a plan yet. Resolve "the current notes" like plans: `current` pointer first, else the sole notes file, else the most-recently-modified `status: open` one. "the latest notes" = most-recently-modified. Default every note/discussion to the current notes file; start a fresh file only when the user says "new notes" or the topic is unmistakably a different future plan.

### Schema

```markdown
# Iterate Notes — <topic title>

Started: <UTC timestamp>
CWD: <pwd>
status: open                  # → consumed (plan: <name>) when /iterate-planner turns it into a plan

## Notes
- [YYYY-MM-DD] <the ask, synthesized to its simplest form — keep the user's distinctive words>

## Decisions
- [YYYY-MM-DD] <decision reached in discussion, one line, with the one-phrase why>

## Open questions
- <raised but not yet answered/decided>

## Research appendix
<uncurated working depth from discussion mode — long analysis, comparisons, dead ends.
 Never printed to screen unprompted; the planner may mine it later.>
```

## Mode router — parse `$1` and the user's phrasing

1. **note mode** — "take a note", "note:", "note this", "add a note", or `$1` is a plain statement of something wanted in the next plan: append one line to `## Notes` (synthesize a ramble down to its simplest recognizable form — same discipline as the planner's provenance lines), then ack in EXACTLY one line: `+ note → <topic> (N notes, M decisions)`. No reprint, no commentary, no follow-up questions. Rapid-fire streaks stay one line each.

2. **discussion mode** — "explore …", "discuss …", "what about …", "should we …", or a direct question about a candidate idea: engage, but **curated**:
   - Think and research as hard as the question deserves — read code, check files, compare approaches. Effort is unlimited; screen space is not.
   - **A direct question gets a direct answer in ≤50 words.** Answer first, one supporting reason if needed, stop.
   - **Analysis (trade-offs, feasibility, "what about X") gets ≤200 words on screen** — the boiled-down verdict of however much thinking happened, not the thinking itself. However much reasoning it took — there is no ceiling on the thinking — the user gets only the ~200 words that change their decision.
   - Overflow depth that's genuinely worth keeping (option comparisons, discovered constraints, rejected-and-why) goes into `## Research appendix` — silently; mention it only as `(depth in appendix)` at the end of the reply.
   - **When the discussion reaches a decision** (user says "let's do X", "yes, that way", "decided", "skip that"), append one line to `## Decisions` right then and ack it inline: `✔ decided: <one line>`. A raised-but-unresolved question goes to `## Open questions` instead.
   - Anything in the discussion the user clearly wants IN the next plan also gets a `## Notes` line — don't make them say "take a note" for something they just plainly asked for.

3. **list notes** — "list notes", "show notes files", "what notes do we have": one line per file: `<topic>: started <date>, N notes, M decisions, K open questions (status)`. Mark the current one. Then stop.

4. **show notes** — "show notes", "show the notes", "read back the notes": print the current (or named) notes file's Notes + Decisions + Open questions sections verbatim — NOT the research appendix (say `(+ research appendix, ask to see it)` if non-empty). Then stop.

5. **new notes** — "new notes <topic>" or a clearly-distinct new future-plan topic: create a fresh file, set `current`, ack in one line. The old file stays `open` — multiple idea streams may brew at once.

6. **handoff** — "turn these notes into a plan", "turn the latest notes into a plan", "plan this up", "make it a plan": do NOT plan here. Invoke the Skill tool with skill `iterate-planner` and args `notes-to-plan <topic>` (resolve `<topic>` per the current/latest rules above first). The planner reads the notes file, builds the real plan with its full machinery, and marks the notes `status: consumed (plan: <name>)`.

## Rules (hard)

1. **Never plan, never execute.** No step/validation pairs, no oracle, no branches, no `/loop`. The moment the user wants a plan, hand off to `/iterate-planner` (mode 6).
2. **Screen output is curated, always.** ≤50 words for a direct answer, ≤200 for analysis, one line for a note ack. Working hard and writing long are different things — the appendix absorbs the length. When a reply is about to exceed the budget, cut detail (it's in the appendix), never cut the answer.
3. **Every decision reached in discussion lands in `## Decisions` the moment it's made** — never reconstructed later from memory. Decisions are what make the eventual plan faithful to the conversation.
4. **Notes are synthesized, not transcribed.** Boil each ask to its simplest one-line form, keeping the few distinctive words that make it recognizably the user's — the same test as planner provenance: reading it back, the user instantly says "yes, that's what I meant".
5. **Don't ask clarifying questions in note mode.** A note is capture, not analysis — take it, ack it, done. Ambiguity gets resolved later, in discussion or at planning time. (Discussion mode may ask ONE sharp question when the answer genuinely forks the analysis — inside the 50-word budget.)
6. **The research appendix is write-mostly.** Park depth there; never dump it to screen unprompted. It exists so curation on screen doesn't mean losing the work.
7. **Notes files are cheap and local — never git-managed by this skill.** No branch, no commit choreography; they live with the other iterate state under `./.claude/iterate/`.
