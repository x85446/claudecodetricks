---
name: "oracle"
description: "The project's buzzword glossary. Surfaces the **5W+H** (Who, What, When, Where, Why, How) for any registered buzzword or buzzphrase the user might mention — subprojects (incusmagic, P64), tools, internal jargon, named workflows (\"mgmt.gravhl.com new-service workflow\"), domain terms. **Use IMMEDIATELY** when the user (a) mentions a likely buzzword that this project might have a glossary entry for, (b) asks \"what is X\", \"tell me about X\", \"what does X do\", \"where is X\", \"remind me about X\", \"we're going to work on X\", \"how do I use X\", \"explain X\", (c) shares new info about a topic (\"X is a tool that...\", \"X lives at /path/...\", \"X is owned by Y\"), or (d) invokes `$oracle` directly."
---



<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# $oracle — Buzzword → 5W+H knowledge base

The oracle is the project's mini-glossary. Each entry is one **buzzword** or **buzzphrase** (a subproject name, a tool name, a workflow name, internal jargon — anything the user might mention and expect you to recognize). Each entry holds the **5W+H**:

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

The project's buzzword glossary. The skill stores knowledge two places: `./.claude/data/oracle.md` (project-local entries) and `~/.agents/skills/oracle/known.md` (global / cross-project entries). On lookup, both are merged; project wins on conflict. The oracle is the lazy-loaded primer for everything the project assumes you already know.

## Usage

Argument: <show|list|add|discover|update|remember|move|forget|harvest> [buzzword] [args]. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$accounts` — ported.
- `$iterate-planner` — ported.

- **Who** — owners, primary users
- **What** — one-paragraph description
- **When** — when to use it; when not to
- **Where** — paths, repos, related skills, URLs
- **Why** — the problem it solves
- **How** — commands, entry points, gotchas, ritual

The oracle is **lazy-loaded** — entries only enter context when a buzzword is mentioned. The trigger lives in this skill's `description` (always loaded, small) so Claude knows when to invoke. Entry bodies stay on disk until needed.

The oracle is **NOT** a lessons-learned bin. Cross-cutting habits (test interactively, rollback-with-retry, don't ask status questions) belong in the relevant skill's SKILL.md rules, not here. The oracle is **subject-based, on-demand context**.

## Two stores

| Store | Path | Scope |
|---|---|---|
| Project | `./.claude/data/oracle.md` | Project-specific buzzwords (subproject names, project-internal workflows, project-only jargon) |
| Global | `~/.agents/skills/oracle/known.md` | Cross-project buzzwords (tools you use everywhere — incusmagic, ssh-config, your home network names, etc.) |

**On lookup:** read both, merge. If the same buzzword exists in both, project wins (project store can override or extend the global entry).

**On write: default to PROJECT.** don't stop to ask. The user opts into global explicitly via:
- a positional arg (`$oracle add incusmagic global`)
- a natural-language hint in the same message ("let's use the global", "make it global", "across all projects")
- relocating an existing entry later with `$oracle move incusmagic global`

Rationale: most entries are project-specific. Promoting to global is a deliberate decision the user makes once the entry has proven useful across more than one project. Defaulting to project keeps the global store clean and intentional.

## Schema

Each store is a single markdown file. Top section is the **index** (one line per buzzword for fast scanning). Each entry is a `## <buzzword>` section with 5W+H subsections.

```markdown
# Oracle — <project-name OR "global">

Last updated: <UTC timestamp>

## Index
- incusmagic
- P64
- mgmt.gravhl.com new-service workflow
- ed-macvm-cluster
- gravhl secrets pipeline (OpenBao + ESO)

---

## incusmagic

**Who:** Travis owns; used by anyone working with the gravhl incus fleet.

**What:** A bash wrapper around `incus` that bakes in project-specific conventions — key install, snapshot policy, naming, ssh wiring. Replaces ad-hoc post-creation scripts.

**When:** Use when spinning up VMs/containers in the gravhl incus fleet, OR when the `incus` skill needs the gravhl ritual applied. Don't use it for raw incus exploration — plain `incus` is fine for that.

**Where:**
- Source: `~/workspace/x85446/incusmagic/`
- Binary: `/usr/local/bin/incusmagic`
- Related skill: [[incus]] (auto-invoked for incus operations; defers to incusmagic when project conventions apply)

**Why:** Plain `incus launch` doesn't install your ssh key, doesn't apply the rolling snapshot policy, doesn't wire `~/.ssh/config.d/<site>` entries. Doing this by hand on every create gets dropped. incusmagic enforces it.

**How:**
- `incusmagic ssh enable <user>` — exec into the container, install `<user>`'s pubkey, add a `Host` block to `~/.ssh/config.d/<site>` if not present.
- `incusmagic snapshot policy <name>` — applies the rolling snapshot policy.
- `incusmagic destroy <name>` — destroys with a snapshot-first-confirm prompt.

---

## mgmt.gravhl.com new-service workflow

**Who:** Anyone adding a new Application behind the mgmt cluster's operator UI.

**What:** A 3-step ritual to add a new service so it's both deployed AND discoverable in the mgmt GUI.

**When:** Whenever a new Application is added to the mgmt cluster that should appear in the operator-facing link tree.

**Where:** GUI link tree at `~/workspace/gravhl/backend/mgmt/web-ui/links.yaml`. Deployed via the mgmt AppSet.

**Why:** The link tree is hand-maintained — there's no auto-discovery. Skipping the update means the service is reachable but invisible to operators.

**How:**
1. Deploy the service normally (Helm chart + Argo Application).
2. Edit `mgmt/web-ui/links.yaml` — add an entry under the right category.
3. Commit + push; ArgoCD picks up the change.
4. Load mgmt.gravhl.com in a browser; click the new link; confirm it routes.
```

If a section (Who / Where / etc) is empty for a given buzzword, omit it. The 5W+H is a template, not a constraint.

## Subcommands

| Subcommand | Form | Purpose |
|---|---|---|
| `show` | `$oracle show <buzzword>` | Print 5W+H for that buzzword (merged from both stores). |
| `list` | `$oracle list` | Print the index from both stores (no bodies). |
| `add` | `$oracle add <buzzword> [global\|project]` | Interactive 5W+H entry creation (user dumps knowledge by asking the user to choose from a short numbered list). **Defaults to PROJECT**. Add `global` to override. |
| `discover` | `$oracle discover <buzzword> [global\|project]` | **Research-driven** draft. Scans current conversation + project files + tool help, drafts a 5W+H, then lets you **Accept**, **Edit in typora**, or **Cancel**. Refuses if entry exists (use `update` instead). |
| `update` | `$oracle update <buzzword>` | Research the buzzword (grep, read docs, ask user), produce or update its 5W+H. Writes to whichever store currently holds the entry. |
| `remember` | `$oracle remember <buzzword>: <fact> [global\|project]` | Append a fact. **Defaults to PROJECT** if new buzzword; otherwise the store that already holds it. |
| `move` | `$oracle move <buzzword> <project\|global>` | Move an entry between stores. Source must exist; destination must not. Shows entry + confirms before move. |
| `forget` | `$oracle forget <buzzword> [global\|project]` | Remove the entry. If buzzword exists in both stores, asks which. Confirms before deleting. |
| `harvest` | `$oracle harvest [archive-path]` | Scan an iterate archive for new buzzwords or updates. Proposes; user picks per-entry which store. |

If `$ARGUMENTS` is empty: print usage + `list`.

### Store-selection grammar

The `[global|project]` token is **optional** on `add` / `remember` / `forget`. Default is always **project**. Three ways to request global:

1. **Explicit positional**: `$oracle add incusmagic global` or `$oracle remember incusmagic: lives at /usr/local/bin global`
2. **Natural-language hint in the same message** (when invoked auto-mode by a "new buzzword" / "remember about" trigger): phrases like "make it global", "let's use the global", "global one", "across all projects" → store the entry globally without asking.
3. **Explicit `$oracle move`** after the fact: `$oracle move incusmagic global` promotes a project entry to global; `$oracle move incusmagic project` demotes a global entry to project-only.

don't stop to ask for store choice unless genuinely ambiguous (e.g., user said both "global" and "this project" in the same message).

## Auto-invocation behavior

The skill description is broad on purpose. The auto-invocation logic inside this skill is:

1. **Identify candidate buzzwords** in the user's message:
   - Quoted strings: `"incusmagic"`, `'P64'`
   - Capitalized proper-noun-looking tokens: `Incusmagic`, `P64`, `OpenBao`, `ed-macvm-cluster`
   - Domain-y phrases the user has emphasized: "the X subproject", "we're working on Y", "<X> is broken"
   - Phrases matching "what is X", "tell me about X", "remind me about X", "explain X"
2. **Merge both store indexes** (project + global) into a single buzzword set.
3. **Match candidates against the index** (case-insensitive substring match; allow plural / verb form variations).
4. **For each match**: load that entry's 5W+H from the right store. Briefly note in the response: `[oracle: loaded entry for "incusmagic" from global]`.
5. **For non-matches that look intentional** (user invoked `$oracle X` directly, or asked "what is X"): offer to add an entry. One-line question: "No oracle entry for `<X>`. Add one now?" Default no — don't push.

The skill description is what makes auto-invocation fire. Its presence in always-loaded context lets Claude recognize the trigger; this body only loads when invoked.

## Workflow

### Step 1: Resolve stores and read indexes

1. Project store: `./.claude/data/oracle.md` (CWD-relative).
2. Global store: `~/.agents/skills/oracle/known.md`.
3. If a store doesn't exist for a write operation, create it (mkdir `-p` the parent) and seed with the template (header + empty Index).

### Step 2: Apply the subcommand

#### `show <buzzword>`

Look up `<buzzword>` in project store first, then global. If found in both, present the merged entry (project's sections override; show overridden fields with a `[global]` / `[project]` tag).

If not found: "No oracle entry for `<buzzword>`. Closest matches: <fuzzy-match-list, top 3>. Add one with `$oracle add <buzzword>`."

#### `list`

Print the index from both stores side by side:

```
PROJECT (./.claude/data/oracle.md):
  - <buzzword>
  - ...

GLOBAL (~/.agents/skills/oracle/known.md):
  - <buzzword>
  - ...
```

#### `add <buzzword> [global|project]`

1. Check both stores for an existing entry. If exists: "Entry already exists in <store>. Use `$oracle update <buzzword>` to extend, or `$oracle move <buzzword>` to relocate."
2. **Determine the target store** (do not ask):
   - If user passed `global` as a positional arg → global.
   - If user said "global" / "make it global" / "let's use the global" / "across all projects" in the natural-language message → global.
   - If user said "project" / "this project only" / "local" → project.
   - **Otherwise → PROJECT** (the default).
3. Walk through 5W+H interactively. For each W: one question. Skip fields the user leaves blank.
4. Compose the entry. Show it back to the user for confirmation before writing.
5. On confirm: append to the chosen store under `## <buzzword>`. Update the Index. Update `Last updated:`.
6. Report: "Added `<buzzword>` to <store>." Suggest `$oracle move <buzzword> global` if user might want to promote it later.

#### `discover <buzzword> [global|project]`

Research-driven NEW entry. Heavier than `add`. Use when the user wants the skill to figure out the 5W+H from current context rather than walking through Q&A.

1. **Refuse if entry already exists** in either store: "Entry exists in <store>. Use `$oracle update <buzzword>` to refine, or `$oracle forget` first if you want to rediscover from scratch."
2. **Determine target store** with the standard rules (positional arg / natural-language hint / default PROJECT). Store choice doesn't affect discovery; it only affects where the final entry lands on Accept.
2a. **Check for an existing draft** at `./.claude/data/oracle-drafts/<buzzword>.md`. If present (from a prior Cancel or Discard):
   - Show the draft's timestamp (`stat -f '%Sm' <path>` or equivalent) and its first ~10 lines as preview.
   - a plain numbered-list question — three options:
     - **Resume editing** → skip Steps 3-6 entirely. Jump to Step 8 (open in typora). The existing draft content is preserved verbatim.
     - **Use as-is** → skip Steps 3-7 entirely. Jump to Step 9 (write the existing draft content to the chosen store).
     - **Re-research from scratch** → continue to Step 3 (overwriting the draft file with fresh research output). Warn: "this will overwrite the existing draft — your prior edits will be lost."
   - Default focus on **Resume editing** (least destructive).
   - If no existing draft: proceed normally to Step 3.
3. **Research phase** (run all that apply, in parallel where possible):
   - Scan the **conversation history** for mentions of `<buzzword>` and the surrounding context.
   - `find . -iname "*<buzzword>*"` — locate files / dirs named after it.
   - `grep -rIl "<buzzword>" . --include="*.md" --include="*.sh" --include="*.yaml" --include="*.yml" --include="*.toml" --include="*.json" 2>/dev/null | head -20` — find references.
   - If `<buzzword>` looks like a command (in `$PATH` or with a binary at `/usr/local/bin/<buzzword>` / `~/bin/<buzzword>` / similar): `<buzzword> --help 2>&1 | head -100` and `which <buzzword>`.
   - If `<buzzword>` is a URL/hostname (contains `.` and looks DNS-y): check `~/.ssh/config.d/*` and project configs for it.
   - Read any obvious README or docs at the discovered paths.
   - Check `~/.ssh/config.d/<buzzword>` and `~/.agents/skills/<buzzword>/` directly.
4. **Draft the 5W+H** from the gathered evidence. Skip any W that you genuinely cannot infer (rather than guessing). For fields with low confidence, mark them with a `<!-- low confidence -->` HTML comment so the user knows what to verify on review.
5. **Write the draft** to `./.claude/data/oracle-drafts/<buzzword>.md` (mkdir `-p` first). The draft file uses the same schema as the final entry (just the `## <buzzword>` section onward, no surrounding index).
6. **Display the draft inline** to the user (the full text).
7. **Ask by asking the user to choose from a short numbered list** — three options:
   - **Accept** — write to the chosen store and clean up the draft file.
   - **Edit in typora** — launch typora on the draft path (see Step 8), wait for save, then re-read and write.
   - **Cancel** — leave the draft file in place; report its path; no write. (Re-invoking `$oracle discover <buzzword>` later will detect the draft and offer to Resume editing — see Step 2a.)
8. **If Edit in typora**:
   - Run: `typora ./.claude/data/oracle-drafts/<buzzword>.md &` (backgrounded so the shell returns; on macOS this opens the typora window).
   - If the `typora` command fails (`command -v typora` returns nothing), fall back to: `open -a Typora ./.claude/data/oracle-drafts/<buzzword>.md`. If that also fails, report: "typora not found. Edit `<path>` in your preferred editor, then tell me 'done' or 'discard'."
   - Tell the user: *"Opened `./.claude/data/oracle-drafts/<buzzword>.md` in typora. **Save the file (Cmd+S) before continuing.**"*
   - a plain numbered-list question: "Done editing — save what's in the file, or discard?"
     - **Save**: re-read the draft from disk (it may now reflect the user's edits), write that content to the chosen oracle store, clean up the draft file.
     - **Discard**: leave the draft file at its path. Report: "Draft preserved at `<path>`. Re-invoke `$oracle discover <buzzword>` later — it'll detect the draft and offer Resume / Use as-is / Re-research."
9. **On any write** (Accept or Save-after-edit): the same append-to-store + update-index + update `Last updated:` flow as `add` Step 5. Confirm to the user.
10. **Report**: `"oracle discover: wrote <buzzword> to <store>. Sourced from: <N files / N conversation excerpts>."`

#### `update <buzzword>`

1. Find the entry. If absent, redirect to `$oracle add`.
2. **Research mode:** if the buzzword is a path/tool/repo, do the following before asking the user:
   - `find` for files matching the buzzword name.
   - `grep -r` for the buzzword across docs/.
   - Read any obvious README/docs.
   - Run `<buzzword> --help` if it looks like a command.
   - Summarize what was found.
3. Compose proposed updates per 5W+H section.
4. Show the diff (existing entry vs proposed). Confirm by asking the user to choose from a short numbered list.
5. On confirm: write back. Update Last updated.

#### `remember <buzzword>: <fact> [global|project]`

1. **Find the entry**:
   - If exists in one store → use that store.
   - If exists in both → write to project (project wins on conflict; same as merge rule).
   - If absent → create entry. Determine the target store using the same rules as `add` (positional arg / natural-language hint / **default project**). Then run minimal `add` flow with this fact pre-populating the right section.
2. Decide which 5W+H section the fact belongs to:
   - "owned by X" / "used by X" → Who
   - "lives at /path" / "in repo Y" → Where
   - "command is X" / "run with Y" → How
   - "use when Z" → When
   - "exists because A" → Why
   - default / "is a thing that does X" → What
3. Append to the chosen section. Dedupe (skip if substring of existing text).
4. Confirm before writing. Report which store + section.

#### `move <buzzword> <project|global>`

1. Parse: `<buzzword>` and the destination store (`project` or `global`). If destination missing or invalid, print usage.
2. Find the source: the OTHER store. If the source doesn't have the entry, report: "No entry for `<buzzword>` in <source-store>. Use `$oracle add` to create."
3. Check destination doesn't already have an entry for `<buzzword>`. If it does, refuse: "Entry already exists in <destination>. Use `$oracle forget <buzzword> <destination>` first, or `$oracle update <buzzword>` to merge by hand."
4. Show the entry that will be moved + confirm by asking the user to choose from a short numbered list: "Move `<buzzword>` from <source> to <destination>?" Default yes.
5. On confirm:
   - Append the entry to the destination store under `## <buzzword>`.
   - Add to destination's Index.
   - Remove the entry AND index line from the source store.
   - Update `Last updated:` on both files.
6. Report: "Moved `<buzzword>` from <source> to <destination>."

#### `forget <buzzword> [global|project]`

1. **Determine which store** holds the entry:
   - If `[global|project]` arg passed → that store.
   - If buzzword exists in only one store → that store.
   - If buzzword exists in both → ask by asking the user to choose from a short numbered list which to remove (or both).
2. Show the entry that will be removed.
3. Confirm by asking the user to choose from a short numbered list: "Remove `<buzzword>` from <store>?" Default no.
4. On confirm: remove the section AND the index line. Write back.

#### `harvest [archive-path]`

1. Default archive: latest `*-done.md` in `./.claude/iterate/archive/`.
2. Read Decisions log + Status log. Extract:
   - New proper nouns / tool names that appeared repeatedly → candidate **new** buzzwords.
   - Mentions of existing buzzwords with new facts → candidate **updates** to existing entries.
3. Present as a list (ask the user to pick any number of them from a numbered list). Default: none checked.
4. For each accepted: run `add` or `remember` flow (with confirmation).
5. Report count: "Harvested N new entries, M updates."

### Step 3: Report

After any write:

```
oracle: <subcommand> done
Store: <project|global> (<path>)
<Action summary>
```

## Auto-invocation guardrails (when fired by buzzword mention, not by $oracle)

- **Surface, don't lecture.** Load the entry, briefly note `[oracle: loaded "incusmagic" from global]`, then answer the user's actual question with that context. Don't dump the full 5W+H on screen unless the user asked to see it.
- **Multiple buzzwords match → load all of them.** Note each: `[oracle: loaded "incusmagic", "P64" from global]`. Don't ask which to load.
- **Buzzword in a totally unrelated context** (e.g., user is talking about Python and drops "P64" while meaning something else): load the entry anyway, but suppress the visible notice. Let the context inform you silently.
- **No entry but obvious intent to learn** ("what is incusmagic?" with no entry): offer to add — once per session per buzzword. Don't pester.

## Rules (hard)

1. **One file per store.** Don't fragment into per-buzzword files. The single-file approach keeps the index scannable and a single `grep` finds everything.
2. **Always confirm before writing.** Even auto-invoked. The user vetoes anything they don't want preserved.
3. **Never store secrets.** No tokens, passwords, API keys, private IPs from sensitive networks. If the user's fact contains a secret-shaped string, refuse and ask them to rephrase or store via `$accounts`.
4. **Dedupe before append.** Substring match against existing entry text in the target section. Offer replace/append/skip if similar.
5. **Auto-load is silent unless useful.** When the user asks "what is X", surface the entry visibly. When the user mentions X in passing, load the entry and let it shape your answer, but don't make a fuss.
6. **Don't compete with CLAUDE.md.** If a fact is part of the project's permanent contract (build commands, architecture every contributor needs from day one), tell the user "this feels like a CLAUDE.md fact — want to put it there instead?"
7. **Don't compete with skills.** If a buzzword has its own skill (e.g., `incus`), oracle holds the *primer* (what it is, where it lives) and the skill holds the *behavior* (how to do things with it). They complement; oracle doesn't try to replace the skill.
8. **Index stays accurate.** Every entry has an index line. Every index line has an entry. On `add`, update both. On `forget`, remove both. Verify after each write.
9. **No archival.** Entries don't auto-expire. If wrong, the user uses `forget` or `update`. Don't auto-prune.

## What NOT to do

- Don't store cross-cutting *rules* ("always test interactively") — those belong in `iterate-plan` / `iterate` SKILL.md.
- Don't store *credentials* — those belong in `$accounts`.
- Don't store *commit-worthy project docs* — those belong in the repo (CLAUDE.md, README, docs/).
- Don't write entries spontaneously. Wait for an invocation trigger (auto or direct).
- Don't lecture the user with the full 5W+H when they only need one field.

## Example interactions

### User mentions a registered buzzword in passing

> User: "ok, spin up a new alma9 box on cypress with incusmagic"
>
> [oracle auto-fires: matches "incusmagic" in global store]
> [oracle: loaded "incusmagic" from global]
>
> [Claude proceeds, knowing incusmagic exists at /usr/local/bin/incusmagic and supports `ssh enable`]

### User asks about an unknown buzzword

> User: "what's WPLT?"
>
> [oracle auto-fires: "what is X" pattern; no entry for WPLT]
>
> No oracle entry for "WPLT". Closest matches: (none). Add an entry now? [Yes / No, just answer from context]

### User shares new info

> User: "we have a new internal tool called gh-mirror at /opt/gh-mirror that syncs github repos to gitea hourly"
>
> [oracle auto-fires: matches "we have a new internal tool called X" pattern]
>
> No oracle entry for "gh-mirror" yet. Want me to add one with: Where=`/opt/gh-mirror`, What="syncs github → gitea hourly"? I'll fill out the other W's interactively. [Yes / Just save what you have / No]

### Iterate-planner queries oracle

> User: `$iterate-planner add metrics service to mgmt.gravhl.com`
>
> [iterate-planner queries oracle for buzzwords in plan scope: "mgmt.gravhl.com"]
> [oracle returns: "mgmt.gravhl.com new-service workflow" entry's 5W+H]
> [iterate-planner folds the How (3 steps + browser smoke-test) into the plan as Steps 2 and 3 with paired interactive validations]
