---
name: "iterate-conductor"
description: "Control the binary conductor and its shared dashboard configuration; start, pause, inspect or configure project automations, AI subscriptions, launch sequences, schedules, power bands and overdrive."
---


# $iterate-conductor — control the binary conductor

**Version:** iterate family 5.27.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


`conductor` owns configuration, queue selection, schedules, budgets, leases, retries, process supervision and durable results. This skill translates the user's request into its CLI operations. The dashboard at http://localhost:8421 uses the same service and state.

## Usage

Argument: status | start | stop | pause | resume | run | kill | schedule <rule>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate` — ported.

Use `scripts/control.sh` beside this skill, or invoke `conductor` directly. The script only forwards literal arguments and preserves stdout, stderr and exit status. Run `conductor --help` before relying on command syntax. If the binary is unavailable, report the missing installation; do not substitute an AI scheduler.

## Read, explain and operate

- Version: read the installed skill’s family version from disk and run `iterate-run version`; report both. Do not create a plan or mutate configuration.
- Bare invocation or status: `conductor status --json`. Resolve the current canonical project through `project list --json`; use its stable ID with `--project ID` for project-scoped operations. Basenames are not identities.
- Explain: `conductor explain --project ID --json`. Queue counts, reasons, budgets and next selections come from this response; do not recalculate them.
- Start/on, resume, pause, stop/off, kill: read `config show --json`, then call the corresponding root verb with `--project ID --revision N --json`. Start/resume enable admission and launch a detached binary worker. Pause/stop finish the current checkpoint and prevent subsequent admissions. Kill interrupts only owned work. These are project-scoped unless the user explicitly requests global control.
- Run: `conductor tick --project ID --json`. This explicitly attempts binary dispatch; status, explain, simulate and dry-run do not dispatch.
- Usage: `usage show --json` or `usage refresh --subscription ID --json`. Binding an authenticated account uses `usage refresh --subscription ID --bind --revision N --json`; never invent account IDs, percentages or reset evidence.
- Activity: `run list --project ID --json`, then `event list --run RUN --json`. Report actual checkpoints and waiting reasons rather than interpreting token totals as remaining allowance.

Release and release-candidate plans (`mode: release` and `mode: rc`) require a direct human iterate invocation; conductor admission never runs them.

A plan whose `depends:` predecessor is unfinished is held: explain reports it as `plan_waiting` with its `wait_reason` and `held_by`, and admission skips it whether it would start or resume, while every plan with nothing unfinished behind it stays eligible in any order. Report a held plan as held behind its root, never as blocked. When the watch stands down, its notice names the root and how many plans it holds (`caiman holds 7 plans`); that root is the one thing to clear. `iterate-run plan graph -C <project>` prints the whole graph. Staging or unstaging a held plan changes nothing about the hold.

The binary owns worker wakeups and lifecycle. Do not create a cron, loop, timer, background AI session, second webserver or shell queue loop. Do not write conductor config/runtime/leases directly, edit `conductor.md`, invoke whole-plan iterate as a dispatch substitute, or kill processes by name.

## Conversational configuration

Read [configuration.md](references/configuration.md) for schedule, launch sequence, power-band or overdrive requests. Use one revision-checked transaction for related edits:

1. Read `config show --json`; keep `data` as the draft and `revision` as the expected revision. Resolve project, subscription and child definitions by stable ID and exact parent. Inspect existing names and semantics before creating anything.
2. Preserve unrelated fields and definitions. Reuse the matching user-named automation/window/sequence and its ID. An equivalent repeated request is a no-op; changed requests edit those IDs. If duplicate names are ambiguous, resolve the intended owner before writing.
3. Write only a private temporary draft, validate through `config validate --file DRAFT --json`, and inspect `config apply --file DRAFT --revision N --dry-run --json`. Preview/simulation requests stop here without arming anything.
4. For an authorized configuration request, `config apply --file DRAFT --revision N --json` commits atomically. On exit 3, reread, merge only the requested changes and retry once; a second conflict is reported without overwriting concurrent edits. A real configuration change does not imply permission to run a tick unless requested.
5. Reread config and explain. Summarize the selected launch sequences, cumulative bands, days/hours/timezone, run limit, closing action and affected shared-subscription automations. Delete the temporary draft.

Registration uses `project create --path ABSOLUTE_PATH --revision N --json` and starts disabled. Resolve its returned ID before configuration. Do not create or move repository files. Settings use AI Subscriptions, Named Power Bands, Named Time Windows, Named Automations and subscription-owned Named Launch Sequences; sequence names combine a fixed subscription prefix with an editable suffix.

Named Automations are shared definitions. Project attachments own local enablement and priority; attaching starts disabled. Use `automation-attachment` resources or the revisioned `automation_attachments` configuration array. Detach references before deleting a definition. Duplicate creates independent rules. Shared edits affect future admissions; active assignments retain their saved inputs.

Shared edits can affect several projects. Report those references from the actual config. Preserve existing enablement unless the user requests changing it. A planning or demo request keeps examples disabled and never exhausts live subscriptions.

## Results and boundaries

JSON output is one envelope containing `schema_version`, `ok`, `revision`, `data` or `error`. Exit 0 includes successful read-only waits; 1 is an operation failure, 2 invalid usage, 3 revision conflict and 4 unavailable/gated dispatch. Surface the binary's remedy. Exit zero alone never proves a plan complete.

The supervisor requests one typed task/validation pair, diagnosis, intake or delivery assignment. When operating inside such an assignment, follow its identity and result-file contract and return after that bounded work. Never start another pair or assume whole-plan delivery authority. Direct human $iterate retains its interactive whole-plan workflow; conductor assignments use the bounded entry path.

Execution admission uses `iterate-run plan claim <name> --owner <cli|conductor> --pid <session-pid>`. Only a live execution excludes the other executor. Dependencies gate typed and unnamed launches. A bare iterate selects the sole ready plan from `iterate-run plan graph --json`; several ready plans are listed without a claim. Use `plan heartbeat`, `plan release` and `plan reconcile` for lease lifecycle, preserving uncertain checkpoint evidence.
