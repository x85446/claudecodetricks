---
name: "iterate-notes"
description: "The note-taking half of the iterate stack — capture ideas for the NEXT iterate plan, before any formal planning. Triggers on \"$iterate-notes\", \"take a note\", \"note this\", \"add a note\". Appends the ask to the current notes file in one synthesized line and acks in one line. Also \"list notes\", \"show notes\", \"new notes <topic>\", and the handoff \"turn these notes into a plan\" (routed to $iterate-planner). This skill never brainstorms, never plans, never executes — deciding between options is $iterate-brainstorm, planning is $iterate-planner."
---

<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# $iterate-notes — Write it down, don't work it out

**Version:** iterate family 5.1.0

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

Notes live under ./.claude/iterate/notes/.

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


The capture skill for the iterate stack:

## Usage

Argument: <a note to take, or "list notes" / "show notes" / "new notes <topic>" / "turn these notes into a plan">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$i` — ported.
- `$ibs` — ported.
- `$in` — ported.
- `$ip` — ported.
- `$iterate` — ported.
- `$iterate-brainstorm` — ported.
- `$iterate-planner` — ported.
- `$oracle` — ported.

| Skill | Role |
|---|---|
| `$iterate-notes` (`$in`) | **Capture** — write the idea down, one line, move on |
| `$iterate-brainstorm` (`$ibs`) | **Decide** — options in, one decision out |
| `$iterate-planner` (`$ip`) | **Plan** — formalize into the 1a/1b schema |
| `$iterate` (`$i`) | **Execute** autonomously |

This skill is a **notepad with a memory**. It does no investigation, no analysis, no option-weighing, no oracle merge, no access preflight, no teamify, no feature branch, no step/validation pairs. Capture is the whole job, and the ack is one line.

**It does not brainstorm.** Working a decision — comparing approaches, weighing trade-offs, "what are my options for X" — is `$iterate-brainstorm`. A question asked in passing gets answered by Claude in ordinary conversation; it does not turn this skill into a discussion mode. If a note-taking session starts becoming a design session, say so in one line and point at `$ibs`.

## Notes files

    ./.claude/iterate/notes/<topic>.md      (project-local; create dirs on first touch)
    ./.claude/iterate/notes/current         (pointer: name of the current notes file)

`<topic>` is a short kebab-case slug inferred from the first note's subject (e.g. `dashboard-redesign`, `vm-cleanup`) — not an animal name; animal names are assigned by `iterate-run` at PLAN creation, and a notes file isn't a plan yet. Resolve "the current notes" like plans: `current` pointer first, else the sole notes file, else the most-recently-modified `status: open` one. "the latest notes" = most-recently-modified. Default every note to the current notes file; start a fresh file only when the user says "new notes" or the topic is unmistakably a different future plan.

### Schema

```markdown
# Iterate Notes — <topic title>

Started: <UTC timestamp>
CWD: <pwd>
status: open                  # → consumed (plan: <name>) when $iterate-planner turns it into a plan

## Notes
- [YYYY-MM-DD] <the ask, synthesized to its simplest form — keep the user's distinctive words>

## Decisions
- [YYYY-MM-DD] <a decision the user states as settled, one line, with the one-phrase why>
```

Two sections, nothing else. **Never add an `## Open questions` or `## Research appendix` section** — those belonged to the discussion mode this skill no longer has, and the appendix in particular is what turned a notepad into a place depth went to die unread. Existing notes files that still carry those sections are left exactly as they are (the planner still mines them on handoff); just don't write new ones.

## Mode router — parse `$1` and the user's phrasing

1. **note mode (the default)** — "take a note", "note:", "note this", "add a note", or `$1` is a plain statement of something wanted in the next plan: append one line to `## Notes` (synthesize a ramble down to its simplest recognizable form — same discipline as the planner's provenance lines), then ack in EXACTLY one line:

   `+ note → <topic> (N notes, M decisions)`

   No reprint, no commentary, no follow-up questions, no analysis of the note's merits. Rapid-fire streaks stay one line each.

2. **decision capture** — the user states something as settled ("we decided X", "going with Y", "skip that"): append one line to `## Decisions` and ack `✔ decided: <one line>`. This is capture of a decision already made elsewhere — it is not this skill reaching one.

3. **list notes** — "list notes", "show notes files", "what notes do we have": one line per file: `<topic>: started <date>, N notes, M decisions (status)`. Mark the current one. Then stop.

4. **show notes** — "show notes", "show the notes", "read back the notes": print the current (or named) notes file's Notes + Decisions verbatim. If the file is an older one carrying legacy `## Open questions` / `## Research appendix` sections, print the questions too and note `(+ research appendix, ask to see it)` rather than dumping it. Then stop.

5. **new notes** — "new notes <topic>" or a clearly-distinct new future-plan topic: create a fresh file, set `current`, ack in one line. The old file stays `open` — multiple idea streams may brew at once.

6. **handoff** — "turn these notes into a plan", "turn the latest notes into a plan", "plan this up", "make it a plan": do NOT plan here. Invoke `$iterate-planner` explicitly and args `notes-to-plan <topic>` (resolve `<topic>` per the current/latest rules above first). The planner reads the notes file, builds the real plan with its full machinery, and marks the notes `status: consumed (plan: <name>)`.

## Rules (hard)

1. **Never brainstorm, never plan, never execute.** No option tables, no trade-off analysis, no step/validation pairs, no oracle, no branches, no `/loop`. Deciding between approaches is `$ibs`; planning is `$ip`; both are one line away.
2. **One line in, one line out.** The ack is the entire response to a note. Capture is not an invitation to discuss what was captured.
3. **Notes are synthesized, not transcribed.** Boil each ask to its simplest one-line form, keeping the few distinctive words that make it recognizably the user's — the same test as planner provenance: reading it back, the user instantly says "yes, that's what I meant".
4. **Never ask clarifying questions.** A note is capture, not analysis — take it, ack it, done. Ambiguity gets resolved later, in `$ibs` or at planning time.
5. **Two sections only.** `## Notes` and `## Decisions`. No appendix, ever — depth that matters belongs in `$ibs`'s conversation or in `$oracle` as durable project knowledge, not in a write-mostly bin inside a notepad.
6. **Notes files are cheap and local — never git-managed by this skill.** No branch, no commit choreography; they live with the other iterate state under `./.claude/iterate/`.

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
