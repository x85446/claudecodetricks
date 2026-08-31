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

### 3. Verify `agents/openai.yaml`

`scaffold.sh` now writes this file itself whenever the source carried
`disable-model-invocation: true`, because the mapping is fully confirmed
and mechanical. Your job is to confirm it exists and that the key is
**nested**:

```yaml
policy:
  allow_implicit_invocation: false
```

A top-level `allow_implicit_invocation: false` is silently ignored and
the skill keeps auto-firing — that is the single most common porting
mistake. If the scaffold report says `-> true` but no
`agents/openai.yaml` is present, write it by hand in the shape above.

The full schema (`interface`, `policy`, `dependencies`) is in
references/codex-format.md. Populate `interface.display_name` /
`short_description` only if the user asks for picker polish; `policy` is
the only part this conversion requires.

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
- **`$1`/`$N`/`$ARGUMENTS`** → leave the token in place, but don't let the
  skill *depend* on it expanding. Substitution is confirmed only for Codex
  **custom prompts** (now deprecated in favour of skills) and is undocumented
  for skills themselves. Wherever the original leaned on `$1`, add a prose
  sentence saying what that argument means — which is also where the dropped
  `argument-hint` needs to land anyway.
- **Background polling loops** ("check whether the subagent finished yet",
  `run_in_background` + poll) → **delete the poll.** Codex waits until all
  requested subagent results are available and returns a consolidated
  response, so a ported polling step is not just unnecessary, it describes
  behaviour that doesn't exist.
- **`when_to_use`** → already merged into `description` by `scaffold.sh`.
  Confirm the merged description still leads with the highest-value trigger:
  Codex shortens descriptions from the tail when the manifest gets tight.
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

## Bulk sync and the daily job

Porting one skill by hand is steps 1-6 above. Porting *every* global skill, and
keeping them current as the Claude Code originals change, is
`scripts/sync-all.sh`. Run it directly rather than looping this skill by hand.

```
scripts/sync-all.sh --install --prune      # port every globally-installed skill
scripts/sync-all.sh --only <skill> --install
scripts/sync-all.sh --force <skill>        # override port protection (see below)
scripts/sync-all.sh --all                  # whole backup repo, not just global
```

**Scope is the globally-installed set** (`~/.claude/skills`), not the backup
repo. The repo is the canonical backup for every project and holds
project-scoped skills that were never globally available; porting those would
give Codex a larger global surface than Claude Code itself has. A skill
installed globally with no repo backup is ported from the live install and
called out in the report.

### The two-layer split

`scripts/convert.py` does the body pass in two halves, and the split is what
makes unattended running safe:

- **SAFE** — deterministic rewrites with one correct answer: `/name` → `$name`
  (orphan `/-name` included), `~/.claude/skills` → `~/.agents/skills`, links to
  files that moved into `references/`, Skill-tool delegation → explicit
  `$name`, structured pickers → numbered-list questions, and the `argument-hint`
  → Usage / `$`-dependency sections. Applied every run; re-running is a no-op.
- **FLAG** — constructs whose port is a real judgment call: auto-resume loops,
  Agent-tool call shapes, background-completion polling. **Never rewritten
  automatically.** Reported, so a model or a human does that pass deliberately.

### Port protection

Each ported skill carries `codex-skills/<name>/.portstamp`:

```
src=<digest of every source file>
out=<digest of every generated file>
manual=true|false
```

Four states per run: **fresh** (no stamp) and **clean** (source moved, output
untouched) regenerate; **unchanged** costs nothing; **MANUAL** — source moved
*and* the port carries `manual=true` or has been edited since it was stamped —
is reported and left strictly alone.

That last state is the whole point. `iterate`'s Codex port rewrote team dispatch
around Codex's consolidated-wait and swapped `/loop` for cron; no converter can
reproduce that from the source. Set `manual=true` after any judgment pass, and
use `--force <skill>` only once that work has been redone or is known
disposable.

### Manifest budget: fold children into their meta

Codex charges description budget only for *implicitly*-invocable skills, and —
unlike Claude Code — turning implicit invocation off does **not** block
delegation, because `$child` explicit invocation still resolves. So the family
convention Claude Code can only recommend, Codex can enforce for free.
`FOLD_CHILDREN_OF` in `sync-all.sh` lists the metas whose children get
`policy.allow_implicit_invocation: false` automatically. The iterate stack is
deliberately excluded: it is a peer pipeline, not a meta with workers, and each
stage is a front door users reach by natural language.

### Manifest diet: what can and cannot move to another file

Codex loads three levels — the startup manifest (`name` + `description` + file
paths), the `SKILL.md` body **on trigger**, and `references/` **on demand**.
Only the first is charged against the budget.

**The description itself cannot be relocated.** It is the only routing signal
Codex has; the spec defines no trigger file, so a phrase that is not in the
description can never fire implicit invocation. Anything that moves out of the
description stops routing, full stop.

What *can* move is everything in there that was never routing signal. A
description written against Claude Code's 16,000-char budget typically carries
what the skill does, where it stores state, and which rules it enforces —
documentation that belongs one level down, read at the moment it matters
instead of paid for in every session. The official guidance is exactly this:
"keep the description exhaustive about when to trigger; keep the body focused
on execution steps," and "front-load the key use case and trigger words so a
host can still match the skill if descriptions are shortened."

`scripts/diet.py` performs that split:

```
scripts/diet.py ~/.agents/skills                    # dry run, shows the plan
scripts/diet.py ~/.agents/skills --budget 7600 --apply
```

Every sentence carrying a quoted phrase, a trigger verb, a `$name`
self-reference, **or negative scope** ("out of scope", "not for" — the spec
asks a description to say when a skill should *and should not* fire) stays in
the description. Everything else moves into a `## What this skill does`
section at the top of the body. Nothing is deleted; text changes level.

Three properties make it safe to run unattended: a skill with no clearly-trigger
sentence is left completely alone; skills are trimmed largest-payload-first and
only until the budget is met, so most are never touched; and it is idempotent —
a second run moves nothing.

`sync-all.sh` runs it automatically at `MANIFEST_BUDGET` (7,600, holding slack
under Codex's real 8,000 so adding one skill does not immediately overflow),
after child-folding and **before** stamping — so the relocation is part of the
generated output rather than a change that later reads as a hand-edit.

Overflow is graceful, not silent: Codex shortens descriptions first, and only
omits skills under severe overcrowding, with a warning. Fitting the budget is
still worth doing — a shortened description loses its tail, and the tail is
where the least-common trigger phrases live.

### Porting to a target other than Codex

Everything target-specific in this skill is already in one place:
`references/codex-format.md` holds the frontmatter rules, directory layout,
discovery paths, and the full mechanism-mapping table. The scripts are
target-agnostic apart from the output paths. A `skill-2-<target>` sibling is
that same shape with its own `references/<target>-format.md` — write the
mapping table from the target's official docs, and do not re-derive conversion
rules ad hoc mid-run.

### Daily automation (macOS)

```
scripts/install-daily.sh --at 03:15     # arm
scripts/install-daily.sh --status       # plist + service state + last run
scripts/install-daily.sh --run-now      # trigger immediately
scripts/install-daily.sh --uninstall    # disarm
```

A launchd LaunchAgent (`com.x85446.codex-skill-sync`), **not** crontab. launchd
is the supported mechanism on macOS, survives reboot, and runs a missed
`StartCalendarInterval` job when the machine wakes instead of silently skipping
the night the laptop was closed — which matters for a job whose entire purpose
is "the Codex copies are never more than a day stale."

`scripts/daily-sync.sh` is the wrapper it runs: it establishes a PATH (launchd
provides almost no environment), syncs, appends a residual-flag scan and a
manifest-budget measurement, writes `~/.claude/log/codex-sync/sync-<date>.log`,
keeps 14 days, and leaves a one-line `status.txt`. A steady-state run is a
2-second no-op.

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
