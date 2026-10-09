---
name: "product-iteratemilestones"
description: "Turns docs/milestones.md ($product stage 8) into one $iterate-planner plan per milestone, all 30, and chain-stages them so each plan queues the next when it finishes green."
---


# $product-iteratemilestones — milestones to plans

**Version:** product family 1.2.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Stage 8 ordered the product into thirty milestones. This skill turns every
one of them into a saved `$iterate-planner` plan and chains them, so the
build runs milestone by milestone with no one at the keyboard. The first
plan is staged now. Each plan's last step stages the next one, and only a
plan that finishes green reaches that step. So a blocked milestone holds
everything behind it, and nothing builds on work that never landed.

## Usage

Argument: "[M1-03 …]". `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$ip` — ported.
- `$iterate` — ported.
- `$iterate-planner` — ported.
- `$product` — ported.

Typing this command is the user's move that `$product` rule 7 reserves. The
`$product` family's documents end at stage 8, and this is the one bridge to
`$ip`.

`$ARGUMENTS` may name milestone ids (`M1-03 M2-01`) to plan only those.
Otherwise every milestone that has no plan yet is planned.

## 1. Preflight

Read, from the project root:

1. **`docs/milestones.md`** must exist. Missing → print
   `no docs/milestones.md — run $product milestones first` and stop.
2. **`./.claude/product/state.md` row 8.** If it is not `done`, say so in
   one line and continue. The user typed this, and planning from an
   unapproved sequence is their call.
3. **Existing plans.** Scan `./.claude/iterate/plans/*.md` and
   `./.claude/iterate/archive/*.md` for a key-block line `milestone: <ID>`.
   A milestone that already has a plan is skipped and listed. Rerunning never
   duplicates a plan. If an existing plan's provenance cites a milestones
   date older than the document's `**Date:**`, list it as `stale`. Report
   it, and never rewrite it.
4. **Git.** `$iterate` needs a git repository. Outside one, the plans are
   still written but nothing is staged (step 4). Say why in one line.

Parse every milestone block: its id, title, Goal, Demo, Delivers, Touches,
Retires, Depends on and Exit criteria. The chain order is the document's
order: M1-01 … M1-10, M2-01 … M3-10. A milestone's **successor** is the
next id in that order, and M3-10 has none.

## 2. Plan them, one subagent per milestone

The planner skill is ~23k tokens per load. Thirty loads in this session
would bury it, so each milestone is planned in its own subagent and returns
one line. **Run one release at a time, in order (v1, then v2, then v3), with
that release's subagents dispatched in parallel in one message.**
`iterate-run name next` allocates codenames under a lock, so parallel
planning is safe. Plans link to each other by **milestone id**, never by
codename, so no subagent needs another's name. Leave the model unset:
planning is judgment work.

The prompt, filled in per milestone:

```
Plan one milestone with $iterate-planner. Project root: <abs path>; work from there.

Milestone <ID> — <title>, from docs/milestones.md (dated <date>), verbatim:
<the milestone's whole block>

Invoke `$iterate-planner` explicitly and the request
"new plan for milestone <ID> — <title>", and build that plan with these
mappings in addition to everything iterate-planner already requires:
- Key block: add the line `milestone: <ID>` above the first `## ` heading.
- Goal: the milestone's Goal, then its Demo as the observable end state.
- Steps: derived from Delivers. Read each cited R-NN with its acceptance
  criteria in docs/requirements.md and each F-NN in docs/features.md; read
  docs/architecture.md and docs/engineering.md for the containers it
  Touches, the stack, the make targets and the test tiers.
- Validations: every Exit criterion becomes one. Sharpen it where the
  requirement's acceptance criteria are more exact; never weaken it.
- Constraints: one `Depends on: <what that milestone produces> (from
  milestone <dep ID>)` line per dependency. License and Cost lines restate
  docs/engineering.md's stack verdicts.
- Provenance: `Milestone <ID> from docs/milestones.md (<date>): <title>`.
- Last step, after the standing finishers: "Stage the next milestone
  (<successor ID>)", skill: iterate-planner. It finds the plan in
  ./.claude/iterate/plans/ whose key block reads `milestone: <successor ID>`
  and runs `$ip stage <that plan's name>`. Validation: that plan's key block
  reads `status: queued`.            <!-- omit this bullet for M3-10 -->
Do not stage this plan, run $iterate, edit anything under docs/, or change
./.claude/iterate/current. Never ask a question. If the milestone needs a
feature, requirement or decision the docs do not contain, do not invent it:
plan what the docs support and report the gap.
Reply with exactly one line: <ID><TAB><plan name><TAB><step count><TAB><gap, or ->
```

## 3. Verify the chain

After each release's subagents return, check the files, not the replies:

- Each planned id has **exactly one** plan carrying `milestone: <ID>`.
  - None: rerun that one subagent once. Still none: report it as failed.
  - Two (a retry duplicated it): keep the newer and remove the other with
    `$ip delete <name>`.
- Every plan except M3-10's names its successor id in its last step.
  Missing: send the planning subagent's request again as an `$ip` refinement
  (`add to <name>: …`).
- No plan has `status: queued` except the one step 4 stages.

## 4. Set current and stage the chain head

Skip this outside a git repository, or when a milestone plan is already
queued or executing, because the chain is already live.

The **chain head** is the earliest milestone whose plan is neither archived
nor closed. Write its name to `./.claude/iterate/current`, then invoke the
Skill tool with `iterate-planner` and `stage <name>`. From then on, the
conductor or the nightly tick runs it. Each plan stages its successor as its
last step.

## 5. Output

One line per milestone, in chain order, then the chain line:

```
milestones → plans: 30 of 30   (v1 10 · v2 10 · v3 10)
  M1-01  kestrel   9 steps
  M1-02  lemur     7 steps   gap: R-044 has no serial stack choice on Windows
  M1-03  —         skipped   plan marmot exists
  …
chain: kestrel (M1-01) staged — each plan stages the next when it finishes green
```

Outside git, the chain line reads
`chain: not staged — not a git repo; git init, then run this again`.

## Rules

1. **One plan per milestone, linked by milestone id.** The id outlives
   codenames, refinements and re-plans.
2. **Order is the document's order.** Dependencies point backwards (stage 8
   audits that), so a linear chain satisfies all of them.
3. **Only the chain head is staged here.** Every later plan is staged by
   its predecessor's green ending, never ahead of it.
4. **Never run `$iterate` and never touch `docs/`.** A locked milestone
   document changes only through `$product unlock milestones`.
5. **Never invent scope.** A gap goes in the output line, and the stage it
   belongs to is the user's to unlock.
6. **Stale plans are reported, never rewritten.** A re-cut milestones
   document means the user decides whether to `$ip delete` and rerun.
