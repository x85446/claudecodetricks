---
name: iterate
description: Use when given a multi-step task with validation criteria and asked to execute autonomously until done. The skill does NOT ask the user clarifying questions mid-run; it picks the most reasonable interpretation, executes, validates, loops, solves its own blockers, and only returns control when validation passes or the run is truly stuck. When the plan is teamed (see /iterate-planner's teamify), dispatches one subagent per independent team to run concurrently instead of working the Steps list serially. Runs on the plan's own feature branch (via the feature-branch skill) and, on all-green completion, automatically opens the PR, merges to the default branch, and deletes the branch; any other ending leaves the branch unmerged and says so. Re-invokable — running `/iterate` again resumes from the saved state file. Triggers on "/iterate", "iterate until done", "keep going until X", "work this until validation passes".
argument-hint: "<task + how to validate> | <plan> | pause [<plan>] | resume [<plan>] | version"
disable-model-invocation: true
version: 5.27.0
---

# /iterate

Read [iterate workflow](reference/workflow.md) in full before acting.
Follow its entry decisions, validation contract, ownership rules and terminal
report format. Preserve the user's complete arguments.

The workflow contains the execution steps, state schema, examples and constraints.
Supporting procedures remain required when the workflow references them.

Whole-plan admission uses `iterate-run plan claim <name> --owner cli --pid <session-pid>`.
Read `iterate-run plan graph --json` for the sole ready plan, its prerequisites
and execution order. A typed name obeys dependencies. Only a live owner excludes
the other executor. Use `plan heartbeat`, `plan release` and `plan reconcile`;
preserve checkpoint evidence and an alive owned child.
