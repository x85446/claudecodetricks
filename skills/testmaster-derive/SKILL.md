---
name: testmaster-derive
description: TESTMASTER child (invoked via /testmaster): derives test-case specs from a stated requirement, including the negative, every-path, restore-state, and interrupted cases. Specs only, no code.
argument-hint: <the requirement in the user's words, e.g. "hitting play mutes the device when mute-devices is on">
version: 1.1.0
---

# /testmaster-derive — turn a stated requirement into the test cases it implies

Obey the shared contracts in `/testmaster`'s SKILL.md (state paths, tiers, real-world mandate) — read it if not already in context.

A requirement stated in plain language almost always specifies the happy path and leaves the rest implicit. The implicit cases are where the bugs live. This skill makes them explicit **before** any code is written.

## Steps

1. **Extract the state matrix.** From `$1`, identify:
   - the **trigger(s)** — what the user does (press play, press stop, natural end)
   - the **condition(s)** — the settings/state that gate the behavior (a checkbox, a mode, a permission)
   - the **expected effect** — what must be true afterward
   - the **scope** — which surface (a screen, a command, an endpoint)
2. **Enumerate the cases, including the ones not stated.** For every trigger × condition combination, write a case. Always include, explicitly:
   - the **negative case** — condition OFF, effect must NOT happen (the single most-skipped test)
   - **every path to the same end state** — a requirement naming two ways to finish ("naturally stops, or pressing stop") is two cases, not one
   - the **restore-state case** — what if the thing was already in the target state before the trigger? Restoring blindly can clobber the user's own setting
   - the **interrupted case** — the trigger starts and something aborts it (quit, error, crash) — is the state left correct?
3. **Flag genuine ambiguity as a question, don't invent an answer.** When the requirement doesn't determine the expected result (the restore-state case usually doesn't), write the case with `expected: AMBIGUOUS — <the two candidate behaviors>` and surface it to the caller. The planner turns these into `Inferred:` decisions it logs; a human can overrule.
4. **Write the specs** to `./.claude/testmaster/catalog.json` as a new requirement entry (see `/testmaster-catalog` for the schema): the requirement statement in the user's own words, plus one case per enumerated scenario with `id`, `given`, `when`, `then`, and `tier: "?"`.
5. **Report** to the caller: one line per case — `<id>: given <state>, when <trigger>, then <effect>` — with ambiguous ones marked. Keep it to the case list; no commentary.

## Worked example

Requirement, in the user's words: *"when hitting play in the history, if the 'mute devices' option is selected, mute the device. When it naturally stops, or pressing stop, it should unmute."*

Derived cases:

| id | given | when | then |
|---|---|---|---|
| mute-1 | mute-devices ON | press play | device is muted |
| mute-2 | mute-devices OFF | press play | device is NOT muted (negative case) |
| mute-3 | mute-devices ON, playing | playback reaches its end | device is unmuted |
| mute-4 | mute-devices ON, playing | press stop | device is unmuted |
| mute-5 | mute-devices ON, device already muted by the user | press play, then stop | AMBIGUOUS — restore to muted (user's setting preserved) or unmute (rule applied literally)? |
| mute-6 | mute-devices ON, playing | app quits / playback errors | device is not left muted |

Two cases the prose never mentioned (mute-5, mute-6) and one it stated only as a clause (mute-3 vs mute-4). That gap is the entire point of this skill.

## Rules

1. **Specs only — never write test code here.** Hand the cases to `/testmaster-maintain`, which implements them against the real product.
2. **The negative case is mandatory.** A requirement with a condition always yields at least one case where the condition is off.
3. **Never resolve an ambiguity silently.** Mark it `AMBIGUOUS` and name both candidate behaviors; the caller decides and logs the decision.
4. **Use the user's own words for the requirement statement** — it is what a future reader matches the tests back to.
5. Derive from the requirement, not from the implementation: this runs at planning time, when there is often no code yet.
