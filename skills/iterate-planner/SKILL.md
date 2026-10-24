---
name: iterate-planner
description: "The planning half of the iterate stack. Formalizes a task into paired 1a-task / 1b-validation format before autonomous execution, folding in the project oracle's checklists and gotchas. Names each plan's feature branch without creating it, records its predecessors on a depends: line, stages plans for the conductor, and ends every plan with three standing finishers (Makefile, TESTMASTER, user-docs). Plans live under ./.claude/iterate/plans/. Never executes; /iterate does."
when_to_use: "Triggers on \"/iterate-planner\" or \"/ip\", \"plan this for iterate\", \"restate the plan\", \"plan with the oracle\". Plan management: \"status\", \"show plan\", \"list plans\", \"add to <name>\", \"delete <name>\", \"from <name> remove <x>\", \"close <name>\", \"roll <name>\", \"notes-to-plan\", \"plan the inbox\". Ordering: \"<name> depends on <x>\", \"drop dependency\", \"graph\", \"retrofit\". Staging: \"stage <name>\" or a milestone range, \"approve\", \"queue\", \"unstage\". Release candidate: \"rc track <x>\", \"rc list\", \"rc plan\". Teaming: \"teamify\", \"team this\"; reverse with \"flatify\", \"un-team\". Also the FFIV macro (Find, Fix, Iterate, Verify) and the \"skip\" modifier (\"skip\", \"skip tests/docs/makefile\") that drops the standing finishers."
argument-hint: "<optional context, e.g. \"restate the plan from above\", \"plan: 1. do X, 2. validate Y\", \"stage <name>\" to approve a plan for the nightly tick, or \"teamify\" / \"flat\" to team or un-team the current plan>"
version: 5.27.0
---

# /iterate-planner

Read [iterate-planner workflow](reference/workflow.md) in full before acting.
Follow its entry decisions, validation contract, ownership rules and terminal
report format. Preserve the user's complete arguments.

The workflow contains the execution steps, state schema, examples and constraints.
Supporting procedures remain required when the workflow references them.

Group related acceptance checkpoints into coherent execution plans. Justify each
split, preserve every requirement and validation exactly once, and keep release
publication distinct. `consolidate <plans…>` preserves completed history, rewrites
dependencies together and validates the shared graph before staging.
