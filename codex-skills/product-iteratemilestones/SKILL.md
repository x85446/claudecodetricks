---
name: "product-iteratemilestones"
description: "PRODUCT stage 10, the plans step: turns docs/milestones.md into one $iterate-planner plan per milestone, all defined milestones, each carrying its milestone's predecessors on its depends: line; once stage 9 (environment) is done it stages them all and the conductor's graph orders them."
---


# $product-iteratemilestones — milestones to execution plans

**Version:** product family 1.7.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Stage 10 maps the approved milestones into coherent execution plans. Read the
complete milestone set before choosing boundaries. A milestone marks acceptance;
a plan owns a branch, validation and delivery. Related checkpoints can share a plan.

## 1. Preflight

Read `docs/milestones.md`, `docs/requirements.md`, `docs/features.md`,
`docs/architecture.md`, `docs/engineering.md`, `docs/environment.md` and
`./.claude$product/state.md`. Missing milestones: report the missing document and
stop. When environment stage 9 is incomplete, write plans but stage none; report
the open operator acts. A direct invocation can plan unapproved milestones and
reports that fact. Outside git, write plans without staging them.

Scan active plans and archives for every `milestone:` line. Preserve completed
history. Skip milestones already owned; report stale provenance without rewriting
existing plans. Arguments may restrict the milestone set. Parse every selected
milestone's Goal, Demo, Delivers, Touches, Retires, Depends on and Exit criteria.

## 2. Group before writing

Group related lifecycle work, shared implementation context and dependent
checkpoints into coherent plans. Keep a separate plan only for independent
delivery, a real dependency, a distinct risk or an operational boundary. Write a
reason for every split, including a subsystem spread across plans. A checkpoint
alone supplies no reason for another branch, PR or finisher. Keep publication in
a distinct `$iterate-release` plan.

Write a lossless ownership map under `./.claude$iterate/`: every stable requirement
and acceptance ID, milestone, source step and validation maps to exactly one
execution plan. Preserve validation wording and runnable proofs. Audit both
source-to-plan and plan-to-source sets. A missing or duplicate ID prevents staging.

Use the same grouping policy as `$ip consolidate`. For each coherent group, read
`$iterate-planner` and create one plan, retaining each covered `milestone:` line.
Spell every covered milestone ID individually in the key block, with one
`milestone: M1-01` line per ID or a comma-separated list of complete IDs. Do not
write ranges such as `M1-01..M1-05`; the graph reads individual IDs.
The plan's predecessors are the group's external milestone predecessors, after
removing edges inside that group. Do not create stage-the-next-plan steps.
The Goal includes every checkpoint's observable outcome. Steps cover every cited
requirement; validations retain every acceptance clause. Each plan includes the
planner's make, tests and docs finishers once. Do not execute a plan.

Read environment Machines keys for guest placement through `$izmachine`. Reuse
the environment row's Proof for access validation; log `env gap: <need>` for an
unlisted dependency. License and Cost constraints restate the engineering verdicts.
Record milestone document dates and the grouping rationale in provenance.

## 3. Verify and order

Diff the ownership map against all generated files. Every requirement, checkpoint
and validation appears exactly once, and every predecessor names an existing
plan or milestone. Run `iterate-run plan graph --check` and resolve graph errors.
Read `position`, `label`, `release`, `waiting_reason` and `next` from the graph;
animal names remain identity. Persist returned positions and labels only in the
new plan files. Preserve every existing plan's bytes, including release plans.
Use `iterate-run plan order set <all live names in dependency order>` only when
this generation owns every live file or the user requests a project-wide reorder. Consolidate if more than 26 plans
would occupy one execution sequence.

For requested realignment, invoke `$ip consolidate <names>` with the ownership
map. Preserve the consumed plans' archives, checked history and requirement IDs.
Never rewrite another project's plans as part of improving these shared skills.

## 4. Stage and report

When stage 9 is done and the graph and ownership audits pass, stage every
approved non-release plan through `$ip stage`. With incomplete environment acts,
stage none and name the recorded acts. Preserve a live `current` pointer;
otherwise select the graph's `next` when it exists.

Report each group, its milestone IDs, plan name, requirement count, split reason
and waiting reason. List gaps separately. Plans remain unexecuted.
