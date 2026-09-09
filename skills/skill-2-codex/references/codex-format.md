# Codex skill format — reference

Verified against official OpenAI docs (`developers.openai.com/codex/skills` →
308-redirects to `learn.chatgpt.com/docs/build-skills`; machine-readable twin at
`learn.chatgpt.com/docs/build-skills.md`; and
`learn.chatgpt.com/docs/agent-configuration/subagents`), as of **2026-08-31**.

Codex skills implement the **Agent Skills open standard** (published by
Anthropic 2025-12-18, since adopted by Claude Code, Codex, Gemini CLI, Cursor,
VS Code and others). The portable core — a directory holding `SKILL.md` with
YAML frontmatter plus optional supporting resources — is shared across
harnesses. What differs per harness is everything *around* that core, which is
what this table is for.

Re-verify against those URLs if this drifts. OpenAI's own docs are the only
ground truth; blog posts and SEO aggregators are not.

## Frontmatter — much stricter than Claude Code's

```yaml
---
name: skill-name
description: Use when someone asks to X, Y, or Z.
---
```

- `name`: lowercase, hyphens only, **no leading hyphen**. The docs specify no
  maximum length; Claude Code's 64-char limit is a safe ceiling to keep.
- `description`: the ONLY other allowed field, and the sole trigger mechanism.
  The spec's instruction is to "explain exactly when this skill should and
  should not trigger" — **all "when to use" information belongs here, not in
  the body.**
- **No other YAML fields are documented.** Claude Code's `argument-hint`,
  `disable-model-invocation`, `when_to_use`, `context: fork`, `model`,
  `allowed-tools`, and `version` have no frontmatter home in Codex. Some of
  that intent relocates (see `agents/openai.yaml` below); the rest is dropped.
  Don't guess a field name.
- **`when_to_use` must be merged into `description`, never dropped.** Claude
  Code splits triggers into a second field; Codex has one field and explicitly
  wants the triggers in it. A converted skill that silently drops
  `when_to_use` loses every trigger phrase it held and will never fire
  implicitly. `scaffold.sh` performs this merge automatically.

## Directory layout

```
skill-name/
├── SKILL.md              (required)
├── agents/
│   └── openai.yaml        (optional — OpenAI-specific metadata + policy)
├── scripts/               (optional — executable code Codex runs for
│                           deterministic, repeatable operations)
├── references/            (optional — docs loaded into context on demand,
│                           not up front; direct analog of Claude Code's
│                           supporting .md files)
└── assets/                (optional — output resources: templates, images,
                            boilerplate; never loaded into context, only
                            used when producing output)
```

## `agents/openai.yaml` — full schema (confirmed)

```yaml
interface:
  display_name: string          # name in the skill picker
  short_description: string     # subtitle in the picker
  icon_small: string            # path
  icon_large: string            # path
  brand_color: string           # hex
  default_prompt: string        # prefilled prompt when launched from the UI

policy:
  allow_implicit_invocation: boolean   # default: true

dependencies:
  tools:
    - type: string
      value: string
      description: string
      transport: string
      url: string
```

**`allow_implicit_invocation` is nested under `policy:` — it is NOT a
top-level key.** A top-level `allow_implicit_invocation: false` is silently
ignored and the skill keeps auto-firing. This is the single most common
porting mistake; Codex genuinely does not honor `disable-model-invocation`
without a correctly-nested `agents/openai.yaml`.

Setting `policy.allow_implicit_invocation: false` excludes the skill from the
implicitly-available model context while leaving it explicitly invokable as
`$skill-name` — an exact behavioral match for Claude Code's
`disable-model-invocation: true`. Keep the two in sync: a skill is
user-invoked in both harnesses or in neither.

## Discovery locations (current — supersedes the Dec 2025 `~/.codex/skills` default)

Codex walks `.agents/skills` from the working directory upward, then falls
back to user- and admin-level directories:

1. `$CWD/.agents/skills`
2. `$CWD/../.agents/skills` — each parent directory, within a git repo
3. `$REPO_ROOT/.agents/skills` — repo/team-shared
4. `$HOME/.agents/skills` — personal, every session
5. `/etc/codex/skills` — admin/system-wide
6. Bundled system skills (OpenAI)

`~/.codex/skills` and `<project>/.codex/skills` were the original (Dec 2025)
locations and may still be read for backward compatibility, but are **not**
the current default — use `.agents/skills` for anything new.

## Invocation

- **Explicit**: `$skill-name` in Codex CLI/IDE, `@skill-name` in ChatGPT.
- **Implicit**: the host matches the task against every skill's `description`
  — the same progressive-disclosure trick as Claude Code (only
  name+description sit in context normally; the body loads on trigger).
- **Picker**: the `/skills` command / dedicated UI selector.
- **Manifest budget**: the skill list uses "at most 2% of the model's context
  window, or 8,000 characters when the context window is unknown."
  Descriptions are shortened first when space runs short — so keep them tight
  and put the highest-value trigger first, exactly as in Claude Code.

## Argument substitution — confirmed for custom prompts, NOT documented for skills

`$1`…`$9`, `$ARGUMENTS`, `KEY=value` named placeholders, and `$$` for a
literal dollar sign are documented for Codex **custom prompts**
(`~/.codex/prompts/*.md`) — which OpenAI now marks **deprecated in favour of
skills**. The skills documentation says nothing about argument substitution
either way.

Practical consequence: **leave `$1`/`$ARGUMENTS` in place** (they cost
nothing if unsupported and work if the runtime shares the prompt expander),
but a converted skill must not *depend* on expansion silently happening.
Where the original leaned on `$1`, also state in prose what the argument
means — "the first word of the user's request is the plan name" — so the body
reads correctly whether or not the token expands. Since `argument-hint` has
no Codex home, that prose is where the usage contract has to live anyway.

## Subagents / parallel execution — confirmed to exist, NOT a fixed tool-call API

Codex has a native multi-agent system: a parent session spawns child agents,
routes work, waits, and consolidates. The documented interface is **natural
language + `/agent` CLI commands**, not a tool schema:

- Trigger by plain instruction: "spawn two agents to work on X and Y in
  parallel", "delegate this to a subagent."
- **Codex waits automatically.** "Codex waits until all requested results are
  available, then returns a consolidated response." The parent does **not**
  poll — do not port a Claude Code polling loop into a converted skill; drop
  it and rely on the consolidated return. Results come back as summaries
  rather than raw intermediate output.
- `/agent` switches between active agent threads and inspects ongoing work;
  `/permissions` adjusts sandbox settings for agent work.
- Blog posts describe internal-sounding tool names (`spawn_agent`,
  `send_message`, `wait_agent`, `close_agent`) but the official docs do not
  expose these to skill authors — **do not hardcode them.** Write the
  orchestration intent in plain language and let Codex's runtime map it.

## Scheduled / recurring re-firing — NO in-session equivalent

**`CronCreate` / `CronList` / `CronDelete` are Claude Code tools, not Codex
tools.** An earlier version of this file asserted Codex had them, with Claude
Code's own tool description attached ("session-only by default, durable to
`.codex/scheduled_tasks.json`, recurring jobs expire after 7 days"). That was
wrong and it shipped: ported skills instructed Codex to call tools that do not
exist, and Codex reported `this session doesn't expose them`.

For Codex they are an **open feature request** — [openai/codex#25466][1], filed
2026-05-31 by panbergco, still open with no maintainer response. The proposal
describes `CronCreate`/`CronList`/`CronDelete` plus `ScheduleWakeup`, "session-only
by default or durable to `.codex/scheduled_tasks.json`", and a `/loop` command.

**That wording is where this file's error came from.** The earlier version
transcribed the proposal as shipped capability, right down to the
`scheduled_tasks.json` path. A proposal read as documentation is the specific
failure mode to watch for here: check the issue state, not just the prose.

A working implementation exists on the author's fork —
`github.com/panbergco/codex/tree/feat/session-scheduling-tools`, ~12 files
registering handlers in `ToolExecutor`/`CoreToolRuntime` — filed as an issue
because PRs to openai/codex are collaborator-only. Running it means running an
unofficial Codex build: rebasing against upstream forever, no support, and a
port that silently breaks the day it diverges. Not worth it when an OS cron job
gets the same outcome. Re-check the issue before trusting any claim that these
landed.

[1]: https://github.com/openai/codex/issues/25466

**What Codex actually has:**

| Mechanism | Who sets it up | Notes |
|---|---|---|
| **Automations / Scheduled Tasks** | the **user**, in the ChatGPT desktop or web UI | natural-language creation, a "Scheduled" view for active/paused/completed runs, RFC 5545 RRULE for advanced patterns; runs unattended under the sandbox settings; can run standalone or inside an existing chat, and on a dedicated worktree for git repos |
| **External OS scheduler + `codex exec`** | the **user**, via cron/launchd/systemd | `codex exec` is the single-shot scriptable entry point; the OS owns the schedule |

**The consequence for any ported skill: it cannot arm its own resumption.**
Claude Code's `/loop` and Cron tools let a skill schedule *itself*. Codex has no
in-session primitive for that, so a skill that depended on self-scheduling must
be rewritten to **ask the user to set up an Automation or an OS cron job**, and
must state plainly that it cannot do so itself. Do not invent a tool call.

Anything ported from an `/iterate`-style auto-resume loop needs that
substitution made explicitly, not papered over.

## No confirmed equivalent (flag, don't fabricate)

- **`AskUserQuestion`** (structured multiple-choice picker) — no documented
  Codex equivalent. Convert to plain "ask the user to choose between..."
  prose and flag it rather than inventing a tool name.
- **`Skill(child-name)` programmatic delegation** — Codex's equivalent is the
  explicit `$child-name` invocation (confirmed), but it requires the child to
  also exist under Codex's discovery path. List every such dependency so the
  caller knows what else needs porting.

## Conversion mapping table (Claude Code → Codex)

| Claude Code mechanism | Codex equivalent | Confidence |
|---|---|---|
| `name`, `description` frontmatter | same fields, verbatim | confirmed |
| `when_to_use` | **merge into `description`** — Codex has one trigger field and wants triggers in it | confirmed (field doesn't exist) |
| `argument-hint` | no frontmatter field; fold into the body's usage text | confirmed (field doesn't exist) |
| `version` | no frontmatter field; drop | confirmed (field doesn't exist) |
| `disable-model-invocation: true` | `agents/openai.yaml` → `policy.allow_implicit_invocation: false` (**nested, not top-level**) | confirmed |
| `context: fork`, `model`, `allowed-tools` | no equivalent | drop, note as dropped |
| `$1`/`$N`/`$ARGUMENTS` | same syntax in custom prompts; undocumented for skills | keep token, ALSO state the meaning in prose |
| supporting `.md` files | `references/` | confirmed |
| `scripts/`, `assets/` | `scripts/`, `assets/` | confirmed |
| Agent/Task tool, `run_in_background` | natural-language subagent orchestration; Codex waits and consolidates | orchestration confirmed; exact API shape not — write intent, not tool calls |
| background-completion notification / polling loop | **drop the poll** — Codex returns a consolidated result synchronously | confirmed |
| `/loop` + `ScheduleWakeup` auto-resume | **no in-session equivalent** — the skill must ask the user to create a Codex Automation or an OS cron job running `codex exec` | confirmed absent; Cron tools are Claude Code's, and an open feature request for Codex (issue #25466) |
| `Skill(child)` delegation, `/other-skill` | `$other-skill` explicit invocation | confirmed syntax; requires child also ported |
| `AskUserQuestion` | no equivalent — plain-language ask | unconfirmed, flag |
| MCP/tool prerequisites stated in prose | `agents/openai.yaml` → `dependencies.tools[]` | confirmed schema; optional to populate |
| Plain shell/CLI calls (e.g. `iterate-run ...`) | unchanged — Codex runs the same shell | confirmed |
