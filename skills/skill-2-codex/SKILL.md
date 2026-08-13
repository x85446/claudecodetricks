---
name: skill-2-codex
description: Use when someone asks to convert a Claude Code skill to Codex CLI/ChatGPT format, port a skill to Codex, make a Claude Code skill work in Codex, or install a Codex version of an existing skill.
argument-hint: <skill-name> <global|project-path>
disable-model-invocation: true
---

# /skill-2-codex — port a Claude Code skill to Codex, one direction only

Converts an existing Claude Code skill (`SKILL.md` + supporting files)
into a Codex-compatible skill folder and installs it where told. This is
**one direction only**: Claude Code → Codex. It never reads from or writes
back into a Codex skill, and it never diffs an existing Codex copy against
the Claude source — every run fully regenerates the Codex folder from
scratch. Editing the Claude original does not auto-refresh the Codex copy;
re-run this skill to pick up changes.

Full frontmatter rules, directory layout, discovery paths, and the
Claude→Codex mechanism mapping table live in
[references/codex-format.md](references/codex-format.md) — read it before
doing the body-conversion pass (step 4). It was researched against
OpenAI's official docs; don't re-derive this from memory or guess at
syntax that isn't in that table.

## Arguments

- `$1` — the Claude Code skill's name (its directory name under
  `skills/`, e.g. `dev-makefiles`, `iterate-planner`).
- `$2` — install target: the literal word `global`, or a project path
  (e.g. `~/workspace/x85446/newcorder`).

If `$2` is missing, ask via AskUserQuestion — this is a genuine per-run
decision with no sane default, offer "Global (`~/.agents/skills` — every
Codex session on this machine)" as one option and let the user supply a
project path as the free-text "Other" answer. Don't guess a target.

## Steps

### 1. Locate the source skill

Search in this order, stop at the first match:

1. `~/workspace/x85446/claudecodetricks/skills/$1/` — the canonical
   backup, always the source of truth if it exists.
2. `~/.claude/skills/$1/` — global install.
3. `.claude/skills/$1/` under the current working directory — project
   install.

If none exist, report "no Claude Code skill named '$1' found" and stop.
Do not guess at a similarly-named skill.

Read the full source: `SKILL.md`, every other `.md` file alongside it,
and any `scripts/`/`assets/` subdirectories.

### 2. Scaffold the mechanical conversion

Run:

```
~/workspace/x85446/claudecodetricks/skills/skill-2-codex/scripts/scaffold.sh \
  <source-skill-dir> \
  ~/workspace/x85446/claudecodetricks/codex-skills/<name>
```

`<name>` is the skill's name with any leading `-` (Claude Code's "orphan"
marker) stripped — Codex has no equivalent to that convention. This
always writes into a dedicated `codex-skills/` tree in the backup repo,
parallel to `skills/` but never mixed into it — the two directory shapes
(Codex's `references/`/`scripts/`/`assets/`, no extra frontmatter) are
different enough from Claude Code's convention to keep them visibly
separate.

Read the script's stdout report — it tells you whether the source had
`disable-model-invocation: true` or an `argument-hint`, and how many
reference files it copied.

### 3. Write `agents/openai.yaml` if flagged

If the scaffold report says `disable-model-invocation:true -> true`,
write `<output-dir>/agents/openai.yaml`:

```yaml
allow_implicit_invocation: false
```

(Only the `allow_implicit_invocation` key is confirmed against official
docs — see references/codex-format.md's "agents/openai.yaml" note. Don't
invent additional keys.)

### 4. Body-conversion pass (the judgment-call part)

Open the scaffolded `SKILL.md` and read through its body once, applying
[references/codex-format.md](references/codex-format.md)'s mapping
table. Concretely, for each Claude-Code-specific mechanism found:

- **Agent/Task tool, `run_in_background`, background-completion
  notifications** → rewrite as plain-language subagent orchestration
  ("spawn N agents to handle X and Y in parallel, then merge their
  results") — Codex's runtime maps this onto its own mechanism; do not
  write a fabricated tool-call syntax. On the *first* occurrence in the
  file, add a one-line HTML comment flagging it as unconfirmed-shape:
  `<!-- codex-port: subagent orchestration rewritten in plain language;
  verify against https://developers.openai.com/codex/subagents -->`
- **`/loop` + `ScheduleWakeup` auto-resume** → rewrite using Codex's
  `CronCreate`/`CronList`/`CronDelete` tools. Explicitly note the 7-day
  recurring-job auto-expiry (no Claude Code equivalent) anywhere the
  original assumed indefinite auto-resume — the converted skill needs a
  "re-arm the cron job" note for runs that could outlive a week.
- **`Skill(child-name)` delegation or `/other-skill` invocation** →
  rewrite as Codex's `$other-skill` syntax. Add every such dependency to
  a new "Dependencies" note near the top of the converted body: these
  other skills must ALSO be ported (this skill, on themselves) before the
  converted skill will actually resolve them.
- **`AskUserQuestion`** → rewrite as plain "ask the user to choose
  between..." prose. No confirmed Codex equivalent — flag it inline the
  first time: `<!-- codex-port: no confirmed structured-picker equivalent
  in Codex; verify manually -->`.
- **`$1`/`$N`/`$ARGUMENTS`** → leave untouched. Confirmed identical syntax.
- **Plain shell/CLI invocations** (e.g. a real installed binary on PATH)
  → leave untouched. Nothing Claude-specific about running a binary.
- **Everything else** (domain logic, file schemas, validation rules,
  business logic) → leave untouched. Don't rewrite prose that isn't
  actually about a Claude-Code-specific mechanism.
- **Internal links to moved supporting files.** `scaffold.sh` moves every
  non-`SKILL.md` `.md` file into `references/`, but a Markdown link
  inside the body pointing at the old bare filename (e.g.
  `[examples.md](examples.md)`) doesn't get rewritten automatically —
  grep the scaffolded `SKILL.md` for links to any file that just got
  moved into `references/` and fix the path.
- **Check every reference file too, not just `SKILL.md`.** A moved
  `references/*.md` can carry its own Claude-Code-specific mentions
  (worked examples referencing `/other-skill`, `/loop`, etc.) — run the
  same mapping-table pass over it. Don't assume only the top-level
  SKILL.md needs conversion.

### 5. Install to the requested target

Copy the finished `codex-skills/<name>/` folder to:

- `$2 == global` → `~/.agents/skills/<name>/`
- `$2` is a path → `<path>/.agents/skills/<name>/` — first confirm `<path>`
  exists (a real directory); if it doesn't, stop and report rather than
  creating an arbitrary tree. Create `.agents/skills/` under it if
  missing.

Overwrite an existing install at that target outright — this tool always
fully regenerates, there's no incremental patch mode.

### 6. Report

One block, in this order:

1. Source read from (which of the 3 search locations).
2. What scaffold.sh did mechanically (files/dirs copied).
3. Every body-level conversion applied, one line each, in the shape
   `<mechanism> -> <what it became>`.
4. Every item flagged as unconfirmed (the `codex-port:` comments) —
   pull these into the visible report, don't leave them buried only in
   the file.
5. Backup path (`codex-skills/<name>/`) and the actual install path.
6. One closing line: "One-directional — re-run this skill after editing
   the Claude Code original to refresh the Codex copy."

## Notes

- This skill only ever writes Codex-format output. It never edits the
  Claude Code source it read from.
- If the source skill references other Claude Code skills that haven't
  been ported yet, still perform the conversion (list them as
  Dependencies per step 4) rather than refusing — the user may port them
  separately, or the reference may be optional context rather than a hard
  dependency.
- Don't add a `skillinstall.sh` case for converted skills — that
  installer's `install_skill` function is specific to Claude Code's
  `.claude/skills/` layout. This skill handles Codex installs directly
  (step 5).
- If Codex's own documented format changes, update
  references/codex-format.md rather than re-deriving conversion rules ad
  hoc mid-run — keep the mapping table as the single source of truth.
