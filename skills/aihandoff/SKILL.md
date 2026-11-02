---
name: aihandoff
description: "Hand a working session from one AI to another — Claude Code to Codex or back — when a plan's tokens are running out or the context is nearly full. `/aihandoff write` writes a briefing for the successor to ./.claude/iterate/handoff/; `/aihandoff read` ingests the newest one into a fresh session. Triggers: \"hand off to codex\", \"hand off to claude\", \"write a handoff\", \"read the handoff\", \"pick up where the other ai left off\", \"aihandoff\"."
argument-hint: write | read [<file>]
disable-model-invocation: true
---

# /aihandoff — brief the successor, don't narrate the past

Two verbs. `write` ends a session by producing a briefing the next AI can act on. `read` opens a session by ingesting it. Claude Code and Codex both carry this skill (`/aihandoff` and `$aihandoff`), so either can be the writer or the reader.

A skill cannot run `/compact`, `/exit`, `/clear` or `/new`: those are CLI commands the user types. The flows below show where they go.

## Flows

```
# room left                          # context nearly full (last 5–10%)
/aihandoff write                     /compact preserve for an aihandoff: goal, done, in flight, dead ends with reasons, decisions, gotchas, files touched
/exit                                /aihandoff write
                                     /exit
```

Then on the other side: `claude "/aihandoff read"` or `codex "$aihandoff read"` — the exact command `write` prints as its last line.

Write directly whenever the meter allows. The briefing is a summary either way; the full history is a better input to that summary than a compaction summary is. `/compact` first only when producing a page of output is itself at risk.

## Files

    ./.claude/iterate/handoff/<UTC ts>-<from>.md            unconsumed
    ./.claude/iterate/handoff/<UTC ts>-<from>.consumed.md   read by a successor

`<from>` is `claude` or `codex`. The directory is gitignored under the `.claude/iterate/` rules; add the line when a project lacks it. Consumed files stay as the trail.

## `write`

Write for a colleague taking over tomorrow, not for a historian. State the present, record every dead end with its reason, drop the path.

1. Gather: `pwd`, `git branch --show-current`, `git status --porcelain`, the current iterate plan if `./.claude/iterate/plans/` has one in flight (`iterate-run status` when installed).
2. Write the file in this schema, nothing else:

```markdown
# AI handoff

project: <basename of pwd>
from: claude | codex
written: <UTC timestamp>
branch: <git branch>
plan: <plan name or none>

## Goal
<what done looks like, one line>

## Done
<landed and verified — the successor never re-checks these>

## In flight
<what exists, what is half-wired, the next concrete step>
confidence: high | medium | low — <why, when not high>

## Dead ends
- <approach> — failed because <reason> (<evidence: error string, test name, number>). Do not retry.

## Decisions
- <choice> — <why>. Not reopened.

## Gotchas
- <what cost time and is not obvious from the code>

## Uncommitted
<`git status --porcelain` output, or none>

## Next
<the first command the successor runs>
```

3. Rules for the content:
   - `Dead ends` and `Decisions` are the two sections only this session can supply; everything else the successor could rediscover from the code. Never leave them thin. "It didn't work" is not a reason; the error string is.
   - `In flight` carries doubt. If the current attempt is not converging, say so and name the alternative, so the successor inherits the judgment and not only the activity.
   - No chronology, no tool output, no sub-variants of a failed approach. One line per dead end.
   - Commit nothing. Mid-edit is the common case; `Uncommitted` records it.
4. Print the path written, then the pickup command on its own last line, chosen by the target:
   - successor is Codex: `codex "$aihandoff read"`
   - successor is Claude Code: `claude "/aihandoff read"`
   When the user did not name the successor, print the command for the other AI. Put that line on the clipboard (`pbcopy` on macOS, `xclip -selection clipboard` or `wl-copy` elsewhere, silently skipped when none exists).

## `read`

1. Resolve the file: the argument when given, else the newest `./.claude/iterate/handoff/*.md` that is not `.consumed.md`. None → say `no handoff pending in <project>` and stop.
2. Read it whole into context.
3. Compare `branch:` with `git branch --show-current`. If they differ, report that first, in one line, before anything else — it is the likeliest drift between write and read.
4. Rename the file to `<same>.consumed.md` so a second session does not ingest it again.
5. Ack in three lines: the file read, the `In flight` line, the `Next` command. Then wait. Run `Next` only when the user says to.

## Rules

1. `write` never runs `/compact`, exits, or commits. `read` never runs `Next` unasked.
2. Newest wins. Several unconsumed files mean several sessions wrote without a pickup; `read` names the ones it skipped.
3. A handoff is not a note and not a plan. An idea for later is `/in`; a decision to record for good is `/oracle`.
4. The schema is fixed. A section with nothing in it says `none`; it is never omitted, so a reader can tell "no dead ends" from "forgot to write them".
