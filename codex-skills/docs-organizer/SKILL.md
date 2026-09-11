---
name: docs-organizer
description: Use when someone asks to organize docs, clean up the docs folder, consolidate documentation, restructure project documentation, or invokes $docs-organizer.
---



<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# docs-organizer

Reads every file under `docs/`, understands the content, then **consolidates and reorganizes** into a clean subdirectory structure. Uses `git mv` to preserve history. Auto-executes once conventions are decided.

## Usage

Argument: [optional subdir to focus on]. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

The problem this solves: over time, teams accumulate overlapping documents (`fast-connect.md`, `fastconnect-notes.md`, `fc-integration.md`) that each describe the same feature from a different angle. Directory structure drifts. Filenames go stale. This skill **merges** overlapping docs into one coherent file with a canonical name, reorganizes into subdirs, and **remembers project-specific conventions** so it doesn't re-ask decisions it already made.

## Fixed Subdirectory Conventions

These are mandatory. Every organized `docs/` has these subdirs (created if missing, left empty if no matching content):

| Subdir | Contents |
|---|---|
| `docs/arch/` | Architecture decisions, system design, technology choices, data flow, integration patterns, ADRs |
| `docs/developer/` | Dev setup, workflow, conventions, troubleshooting, build system, contribution guidelines |
| `docs/tests/` | Test plans, test specs, acceptance criteria |

## Project Memory File

`docs/.docs-organizer.md` is the skill's **persistent memory** for that project. It records decisions made in past runs so it doesn't re-ask.

**Always gitignored.** Every run ensures `docs/.docs-organizer.md` is listed in the project root's `.gitignore` — the memory file is working state, not shared project state.

**Structure:**

```markdown
# docs-organizer memory

<!-- Managed by $docs-organizer. Edit if you want to adjust conventions. -->

## Subdirs in use
- `arch/` — architecture
- `developer/` — dev workflow
- `tests/` — test specs
- `api/` — API reference (added 2026-04-20: >5 files about REST endpoints)

## Merge history
- `arch/fast-connect.md` consolidated from: fast-connect.md, fc-notes.md, fastconnect-integration.md (2026-04-20)

## Never merge
- `prd.md` and `product-vision.md` — different audiences, keep separate

## Never move
- `CHANGELOG.md` — stays at docs/ root
```

**Rules:**
- Read this file **first** on every run. Apply its conventions silently.
- Update it after each run with new decisions made.
- If the user tells you "don't merge X and Y" or "keep file Z at root", record it under "Never merge" or "Never move".

## Workflow

### Step 1: Load memory + survey

1. Read `docs/.docs-organizer.md` if it exists. Absorb its conventions.
2. **Ensure `.gitignore` contains `docs/.docs-organizer.md`.** The memory file is local-only — not shared project state. Find the project root's `.gitignore` (create if missing) and append the entry if absent. Do this silently before doing anything else that touches the memory file.
3. Recursively list everything under `docs/`. If `$ARGUMENTS` is provided (e.g., `$docs-organizer arch`), restrict scope to that subdir.
4. Build a file inventory: path, size, git-tracked?, first-heading, first-300-chars summary.

### Step 2: Read & classify

Read every markdown file fully. For each, determine:

1. **Topic** — what feature/subsystem it covers
2. **Category** — `arch` / `developer` / `tests` / existing-custom-subdir / `root`
3. **Overlaps** — which other files cover the same topic

### Step 3: Detect new subdir needs

If you see ≥5 files at `docs/` root or scattered across subdirs that share a clear topic not covered by `arch/`/`developer/`/`tests/` (e.g., API reference, runbooks, product, security), **propose a new subdir**.

Ask the user **once** by asking the user to choose from a short numbered list:
- "I see 6 files about REST API endpoints. Create `docs/api/`? (yes / no / suggest different name)"

Whatever they choose, record it in `docs/.docs-organizer.md` under "Subdirs in use" so you never re-ask. If they say "no", record under "Never create: api (user declined 2026-04-20)".

### Step 4: Plan consolidation

For each topic group with multiple files:

- **Merge** if files cover overlapping ground. Produce one canonical file:
  - Short opening paragraph (what and why)
  - Merged body covering all decisions, constraints, and context from originals
  - No duplicate sections — harmonize the narrative
  - Preserve every non-obvious decision, trade-off, and external link from any source
- **Keep separate** if files cover genuinely distinct aspects (architecture vs. runbook) or if memory says "Never merge"

Prefer **canonical short names**:
- `docs/arch/fast-connect.md` (not `fast-connect-service-architecture-overview.md`)
- `docs/developer/setup.md` (not `developer-environment-setup-guide.md`)

### Step 5: Execute — use git to preserve history

**Moves and renames — always use `git mv`:**

```bash
git mv docs/schema-design.md docs/arch/database-schema.md
```

**Merges — preserve the canonical file's history, append others, then `git rm`:**

1. Choose the "anchor" file: the one whose name will become the canonical path, or the oldest/richest file if creating a new canonical name
2. If the anchor needs a new path: `git mv anchor.md docs/arch/canonical.md`
3. Rewrite the anchor file with the merged content (Write tool — this shows as a modification, not a delete+create)
4. For each other source file: `git rm docs/fc-notes.md` (after confirming its content is preserved in the anchor)

This keeps the canonical file's git history intact and shows merged files as deletions with a single modification — much cleaner than deleting everything and creating new.

**Untracked files:** Use plain `mv` if `git ls-files --error-unmatch <file>` fails. Still prefer moving over delete+create.

**Create subdirs:** `mkdir -p docs/arch docs/developer docs/tests` plus any new approved ones.

### Step 6: Update cross-references

Any markdown file (inside or outside `docs/`) that links to a moved/renamed/merged file needs its link updated. Search the whole repo:

```bash
grep -rln "old-path.md" --include="*.md"
```

Fix each match with the new path. Include this count in the report.

### Step 7: Write/update `docs/README.md`

Keep it short — structure overview and a few key links:

```markdown
# Documentation

- `arch/` — Architecture decisions and system design
- `developer/` — Developer setup and workflow
- `tests/` — Test specifications
- [additional subdirs listed here if present]

## Key Files
- [Architecture overview](arch/README.md) — if present
- [Developer setup](developer/setup.md) — if present
- [PRD](prd.md) — if present
```

### Step 8: Update memory

Append this run's decisions to `docs/.docs-organizer.md`:
- New subdirs approved
- Merge groups (anchor ← sources)
- New "Never merge" / "Never move" entries if user flagged any

### Step 9: Report

Terse stdout summary:

```
docs-organizer: 12 files processed

Merged (git mv anchor + git rm sources):
  arch/fast-connect.md ← fast-connect.md + fc-notes.md + fastconnect-integration.md
  developer/setup.md ← env-setup.md + dev-install.md

Moved (git mv):
  arch/database-schema.md ← schema-design.md
  developer/troubleshooting.md ← troubleshooting.md

Renamed (git mv):
  arch/auth.md ← authentication-system-overview-v2.md

New subdir: docs/api/ (user approved)

Left alone: README.md, prd.md, SECURITY.md

Cross-references updated: 4 files

Memory updated: docs/.docs-organizer.md

Review: git status && git diff docs/
```

## Consolidation Heuristics

**Too many files (the problem):**
- 15+ files in `arch/` where 6 share one feature — merge those 6
- File <30 lines whose topic is fully covered by a sibling — merge
- Two files with >60% content overlap — merge, keep the richer detail

**Not enough detail (the failure mode):**
- Never drop non-obvious decisions, constraints, rationale, trade-offs, or external links when merging
- Prefer bullet points and sub-headings over prose if length grows
- When in doubt between terse and detailed, keep detail

**Filename smell test:**
- Vague (`notes.md`, `doc.md`, `misc.md`) → rename based on content
- Dated (`meeting-2024-03-15.md`) → fold into topical file if still relevant; delete if stale
- Versioned (`-v2`, `-final`, `-new`) → drop the suffix via `git mv`
- Overly long → shorten to canonical noun phrase

## Guardrails

- **Git history is sacred.** Prefer `git mv` for everything. Use `git rm` only for files whose content is fully preserved elsewhere. Never `rm -f` a tracked file.
- **Never delete unique content.** Every non-redundant paragraph from deleted files must appear in a merged file.
- **Never touch files outside `docs/`** except to update markdown links pointing into `docs/`.
- **Skip binary files** (.pdf, .png, images) — leave them.
- **Skip symlinks** — don't follow or modify.
- **If `docs/` doesn't exist**, print a message and exit.
- **If the repo is dirty in `docs/`**, warn the user but proceed (auto-execute).
- **Don't invent content.** Only merge/reorganize what exists.
- **Respect memory.** Once `docs/.docs-organizer.md` records a decision, honor it without re-asking.

## What NOT to Do

- Don't create subdirs for <3 files — leave them at `docs/` root or in existing subdirs
- Don't rename just for aesthetics — only when name is vague, dated, or versioned
- Don't expand prose — the goal is fewer, tighter files
- Don't create ADR-style numbered files unless the project already uses that pattern
- Don't ignore `docs/.docs-organizer.md` — it's the contract with past-you
