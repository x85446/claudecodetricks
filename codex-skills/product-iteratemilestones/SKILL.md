---
name: "product-iteratemilestones"
description: "PRODUCT stage 10, the plans step: turns docs/milestones.md into one $iterate-planner plan per milestone, all 30, each carrying its milestone's predecessors on its depends: line; once stage 9 (environment) is done it stages them all and the conductor's graph orders them."
---


# $product-iteratemilestones — milestones to plans

**Version:** product family 1.7.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Stage 8 ordered the product into thirty milestones. This skill turns every
one of them into a saved `$iterate-planner` plan whose key block names its
predecessors on a `depends:` line, so the build runs with no one at the
keyboard. Once stage 9 is done every plan is staged, and the conductor runs
each one as soon as every plan it depends on has landed: two milestones
that both need only M1-03 run in either order, and a blocked milestone holds
only the plans that depend on it. Nothing builds on work that never landed.

## Usage

Argument: "[M1-03 …]". `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$ip` — ported.
- `$iterate` — ported.
- `$iterate-planner` — ported.
- `$product` — ported.

It is `$product`'s plans step, stage 10. The meta runs it when stages 0–9
are done and no plan carries a `milestone:` key, and the user can also type
it directly. Either one is the user's move that `$product` rule 7 reserves.

`$ARGUMENTS` may name milestone ids (`M1-03 M2-01`) to plan only those.
Otherwise every milestone that has no plan yet is planned.

## 1. Preflight

Read, from the project root:

1. **`docs/milestones.md`** must exist. Missing → print
   `no docs/milestones.md — run $product milestones first` and stop.
2. **`./.claude/product/state.md` rows 8 and 9.** Row 8 not `done`: say so
   in one line and continue. The user typed this, and planning from an
   unapproved sequence is their call. Row 9 (environment) not `done`: the
   plans are still written, and step 4 stages nothing, because plans
   launched into walls nobody listed park on the first one. Say so in one
   line and name the count of open operator acts from `docs/environment.md`.
3. **Existing plans.** Scan `./.claude/iterate/plans/*.md` and
   `./.claude/iterate/archive/*.md` for a key-block line `milestone: <ID>`.
   A milestone that already has a plan is skipped and listed. Rerunning never
   duplicates a plan. If an existing plan's provenance cites a milestones
   date older than the document's `**Date:**`, list it as `stale`. Report
   it, and never rewrite it.
4. **Git.** `$iterate` needs a git repository. Outside one, the plans are
   still written but nothing is staged (step 4). Say why in one line.

Parse every milestone block: its id, title, Goal, Demo, Delivers, Touches,
Retires, Depends on and Exit criteria. A milestone's **predecessors** are
the internal milestone ids on its `**Depends on:**` line; items marked
`(external)` name no plan and are dropped. `—` means none.

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
- Key block: add the lines `milestone: <ID>` and `depends: <predecessor
  ids, comma-separated>` (or `depends: none`) above the first `## ` heading.
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
- Environment: read docs/environment.md when it exists. A step that needs
  a machine uses one of its Machines keys (`izmachine up <key>`, rolled back
  to its `clean` snapshot for an install check), never a new host or a new
  definition. The access preflight's verification step for a need the
  document lists reuses that row's Proof as its Nb. A need the document
  does not list is an environment gap: still write the verification step,
  and report `env gap: <need>` in your reply line.
- Provenance: `Milestone <ID> from docs/milestones.md (<date>): <title>`.
Do not stage this plan, run $iterate, edit anything under docs/, or change
./.claude/iterate/current. Never ask a question. If the milestone needs a
feature, requirement or decision the docs do not contain, do not invent it:
plan what the docs support and report the gap.
Reply with exactly one line: <ID><TAB><plan name><TAB><step count><TAB><gap, or ->
```

## 3. Verify the graph

After each release's subagents return, check the files, not the replies:

- Each planned id has **exactly one** plan carrying `milestone: <ID>`.
  - None: rerun that one subagent once. Still none: report it as failed.
  - Two (a retry duplicated it): keep the newer and remove the other with
    `$ip delete <name>`.
- Each plan's `depends:` line equals its milestone's predecessors. A wrong
  or missing line is fixed with `$ip <name> depends on <ids>`.
- `iterate-run plan graph --check` exits 0. It reports an unknown id or a
  cycle line by line; each is a `docs/milestones.md` defect to report, and
  the plan keeps the ids the document names.
- No plan has `status: queued` before step 4.

## 4. Set current and stage the plans

Skip this outside a git repository or when state row 9 (environment) is not
`done`.

Stage every milestone plan that is neither archived, closed, executing nor
already queued: invoke explicit `$name` invocation with `iterate-planner` and
`stage <first ID>..<last ID>` (one milestone range). Write the earliest
ready plan's name (`iterate-run plan graph`, no unfinished predecessor) to
`./.claude/iterate/current`. From then on the conductor or the nightly tick
runs every ready plan, and each waiting one starts when its predecessors
land.

## 5. Output

One line per milestone, in document order, then the graph line:

```
milestones → plans: 30 of 30   (v1 10 · v2 10 · v3 10)
  M1-01  kestrel   9 steps
  M1-02  lemur     7 steps   gap: R-044 has no serial stack choice on Windows
  M1-03  —         skipped   plan marmot exists
  …
graph: 29 staged · ready now: kestrel (M1-01) · 28 wait on their predecessors
```

Outside git, the graph line reads
`graph: not staged — not a git repo; git init, then run this again`. With
stage 9 open it reads `graph: not staged — environment has <N> operator
acts open (docs/environment.md); $product environment after each, then run
this again`.

## Rules

1. **One plan per milestone, linked by milestone id.** The id outlives
   codenames, refinements and re-plans.
2. **Order lives on each plan's `depends:` line.** It copies the
   milestone's `**Depends on:**` ids; stage 8 audits that they point
   backwards and form no cycle. No plan carries a "stage the next
   milestone" step.
3. **Staging approves; the graph orders.** Every plan is staged together in
   step 4, and the conductor holds each one until its predecessors land.
4. **Never run `$iterate` and never touch `docs/`.** A locked milestone
   document changes only through `$product unlock milestones`.
5. **Never invent scope.** A gap goes in the output line, and the stage it
   belongs to is the user's to unlock.
6. **Stale plans are reported, never rewritten.** A re-cut milestones
   document means the user decides whether to `$ip delete` and rerun.
7. **Staging waits for the environment.** Plans are written whenever the
   user asks; they are staged only once stage 9 is `done`, so the first
   unattended launch meets no wall stage 9 did not list.
