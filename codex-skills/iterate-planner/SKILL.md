---
name: "iterate-planner"
description: "Never executes — the user runs $iterate for that. Triggers on \"$iterate-planner\" or its alias \"$ip\", \"plan this for iterate\", \"give me an iterate plan\", \"restate the plan\", \"plan with the oracle\". Plan management: \"status\", \"publish\" / \"show plan\", \"list plans\", \"add to <name>\", \"delete <name>\", \"from <name> remove <x>\", \"close <name>\" (archive unfinished, branch left unmerged), \"roll <name>\" (carry unfinished steps to a new plan, same branch), \"turn these notes into a plan\" / \"notes-to-plan\", \"plan the inbox\" (turns $mailbox messages into a plan). Ordering: \"<name> depends on <x>\", \"graph\", \"retrofit\". Release candidate: \"release candidate task to track: <x>\", \"rc list\", \"rc plan\". Teaming: \"team this\", \"teamify\", \"team up the plan\", \"reorganize into teams\"; reverse with \"flat\", \"flatify\", \"un-team\", \"remove teams\". Also recognizes the FFIV macro (Find, Fix, Iterate, Verify) for quality sweeps over a named scope, and the \"skip\" modifier (\"skip\", \"skip finishers\", \"skip tests/docs/makefile\") which suppresses the standing end-of-plan finishers for that plan."
---

# $iterate-planner

**Version:** iterate family 5.26.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Read [iterate-planner workflow](references/workflow.md) in full before acting.
Follow its entry decisions, validation contract, ownership rules and terminal
report format. Preserve the user's complete arguments.

The workflow contains the execution steps, state schema, examples and constraints.
Supporting procedures remain required when the workflow references them.

Group related acceptance checkpoints into coherent execution plans. Justify each
split, preserve every requirement and validation exactly once, and keep release
publication distinct. `consolidate <plans…>` preserves completed history, rewrites
dependencies together and validates the shared graph before staging.
