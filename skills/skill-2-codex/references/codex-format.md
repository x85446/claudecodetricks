# Codex skill format — reference

Researched from official OpenAI docs (developers.openai.com/codex/skills →
redirects to learn.chatgpt.com/docs/build-skills, and
learn.chatgpt.com/docs/agent-configuration/subagents) plus the
openai/skills repo's skill-creator SKILL.md, as of 2026-08. Codex skills
launched December 2025. Re-verify against those URLs if this drifts —
OpenAI's own docs are the only ground truth; blog posts and SEO
aggregators are not.

## Frontmatter — much stricter than Claude Code's

```yaml
---
name: skill-name
description: Use when someone asks to X, Y, or Z.
---
```

- `name`: lowercase, hyphens only, max 64 chars, **no leading hyphen**.
- `description`: the ONLY other allowed field. This is the sole trigger
  mechanism — "Include both what the Skill does and specific
  triggers/contexts for when to use it. Include all 'when to use'
  information here — not in the body."
- **The spec explicitly states: do not include any other YAML fields.**
  Claude Code's `argument-hint`, `disable-model-invocation`,
  `context: fork`, `model`, `allowed-tools` have NO frontmatter home in
  Codex. Some of that intent has a home elsewhere (see `agents/openai.yaml`
  below); the rest has no confirmed equivalent — drop it, don't guess a
  field name.

## Directory layout

```
skill-name/
├── SKILL.md              (required)
├── agents/
│   └── openai.yaml        (optional — UI metadata: display name, short
│                           description, default prompt, and reportedly
│                           an `allow_implicit_invocation` flag; exact
│                           schema not confirmed beyond this — verify
│                           against current docs before depending on a
│                           field name not listed here)
├── scripts/                (optional — executable code Codex can run
│                            for deterministic, repeatable operations)
├── references/             (optional — docs loaded into context on
│                            demand, not up front; direct analog of
│                            Claude Code supporting .md files)
└── assets/                 (optional — output resources: templates,
                             images, boilerplate; never loaded into
                             context, only used when producing output)
```

## Discovery locations (current — supersedes the Dec 2025 `~/.codex/skills` default)

Codex walks `.agents/skills` from the current working directory up to the
repo root, then falls back to user- and admin-level directories:

1. `$CWD/.agents/skills`, checked at every parent directory up to
   `$REPO_ROOT/.agents/skills` — repo/team-shared.
2. `$HOME/.agents/skills` — personal, every session.
3. `/etc/codex/skills` — admin/system-wide defaults.
4. Built-in system skills.

`~/.codex/skills` and `<project>/.codex/skills` were the original (Dec
2025) locations and may still be read by some runtimes for backward
compatibility, but are **not** the current default for new shared
skills — use `.agents/skills` for anything new.

## Invocation

- **Explicit**: `$skill-name` in Codex CLI/IDE, `@skill-name` in ChatGPT.
- **Implicit**: Codex auto-matches the task against every skill's
  `description` — same progressive-disclosure trick as Claude Code (only
  name+description sit in context normally, full body loads on trigger).
  Total description budget across all skills is capped (~8,000 chars /
  <2% of context) — keep descriptions tight the same way you would for
  Claude Code.
- **Picker**: `/skills` command / dedicated UI selector.
- `agents/openai.yaml`'s `allow_implicit_invocation: false` is the
  closest confirmed analog to Claude Code's `disable-model-invocation:
  true` — blocks auto-triggering, the skill still works when explicitly
  named.

## Argument substitution — same syntax as Claude Code

Codex custom prompts/skills support `$1`…`$9` positional args, `$ARGUMENTS`
for the full tail, `$$` for a literal dollar sign, and named args declared
in frontmatter expanding as `$name`. **This means Claude Code skill bodies
using `$1`/`$ARGUMENTS` port over unchanged** — no rewrite needed here,
unlike almost everything else in this table.

## Subagents / parallel execution — confirmed to exist, NOT a fixed tool-call API

Codex has a native multi-agent system: a parent session can spawn child
agents, route work, wait for results, and consolidate a response. BUT the
publicly documented interface is **natural language + `/agent` CLI
commands**, not a fixed tool schema:

- Trigger by plain instruction: "spawn two agents to work on X and Y in
  parallel", "delegate this to a subagent."
- Codex's runtime handles routing, waiting, and consolidating — "when
  many agents are running, Codex waits until all requested results are
  available, then returns a consolidated response."
- Steer, stop, or close a running subagent via `/agent` CLI commands or
  direct text instructions to that thread.
- Blog posts describe internal-sounding tool names (`spawn_agent`,
  `send_message`, `followup_task`, `wait_agent`, `list_agents`,
  `close_agent`) but the official docs do not expose these as something
  a skill author writes directly — **do not hardcode these names into a
  converted skill's instructions.** Write the orchestration intent in
  plain language instead (what to parallelize, what to wait for, what to
  merge) and let Codex's own runtime map that onto its actual mechanism.
- Whether a parent gets an automatic notification when a background
  subagent finishes, or must poll, is not confirmed in the public docs.
  A converted skill that depends on this (e.g. Claude Code's
  background-completion notification) should say "check on it" as an
  explicit polling step rather than assume a push notification exists.

## Scheduled / recurring re-firing — Cron tools are the confirmed analog

Codex has `CronCreate` / `CronList` / `CronDelete`: standard 5-field cron
jobs, recurring or one-shot. Session-only by default, or durable to
`.codex/scheduled_tasks.json`. Recurring jobs auto-expire after 7 days.
This is the closest match to Claude Code's `/loop` + `ScheduleWakeup`
auto-resume pattern — close enough to port directly, with one caveat:
**the 7-day auto-expiry has no Claude Code equivalent** — a long-running
converted skill that outlives a week needs its cron job re-armed, note
this explicitly in anything ported from an `/iterate`-style auto-resume
loop.

## No confirmed equivalent (flag, don't fabricate)

- **`AskUserQuestion`** (Claude Code's structured multiple-choice picker)
  — no confirmed Codex equivalent in the docs reviewed. Convert to plain
  "ask the user to choose between..." prose and flag it as unconfirmed
  rather than inventing a tool name.
- **`Skill(child-name)` programmatic delegation** — Codex's equivalent is
  the explicit `$child-name` invocation syntax (confirmed), but it
  requires the child skill to *also* exist under Codex's own discovery
  path. A converted skill that calls other skills must list every such
  dependency so the caller knows what else needs porting.

## Conversion mapping table (Claude Code → Codex)

| Claude Code mechanism | Codex equivalent | Confidence |
|---|---|---|
| `name`, `description` frontmatter | same fields, verbatim | confirmed |
| `argument-hint` | no frontmatter field; fold into body's usage text | confirmed (field doesn't exist) |
| `disable-model-invocation: true` | `agents/openai.yaml` → `allow_implicit_invocation: false` | best-effort — exact YAML key confirmed, full schema not |
| `context: fork`, `model`, `allowed-tools` | no confirmed equivalent | drop, note as dropped |
| `$1`/`$N`/`$ARGUMENTS` | identical syntax | confirmed |
| supporting `.md` files | `references/` | confirmed |
| `scripts/` | `scripts/` | confirmed |
| Agent/Task tool, `run_in_background`, background-completion notification | natural-language subagent orchestration + explicit poll/steer via `/agent` | orchestration exists (confirmed); exact API shape not — write intent, not tool calls |
| `/loop` + `ScheduleWakeup` auto-resume | `CronCreate`/`CronList`/`CronDelete`, note 7-day recurring expiry | confirmed close analog, one caveat |
| `Skill(child)` delegation, `/other-skill` | `$other-skill` explicit invocation | confirmed syntax; requires child also ported |
| `AskUserQuestion` | no confirmed equivalent — plain-language ask | unconfirmed, flag |
| Plain shell/CLI calls (e.g. `iterate-run ...`) | unchanged — Codex runs the same shell | confirmed (nothing Claude-specific about a real binary on PATH) |
