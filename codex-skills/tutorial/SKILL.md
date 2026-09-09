---
name: "tutorial"
description: "Builds and maintains self-running bash tutorials that live in the codebase. A tutorial walks a human through a program by showing each real command pre-filled and editable, running it on Enter — no copy-paste, no setup, no thinking. Menu-driven and extensible; also updates, reorganizes, deletes, and audits existing tutorials as the code changes."
---

<!-- version: shared across the family; see the **Version:** line above. -->

# $tutorial — self-running tutorials that live with the code

**Version:** 1.2.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Builds a **bash tutorial the human drives with the Enter key**. Every step prints the real command already filled in — arguments and all — and runs it when they hit Enter. They can edit the line first; they never have to type one from scratch, look anything up, or set anything up. The point is training and demoing with zero friction.

## Usage

Argument: <what to build a tutorial for, or: list | update <name> | audit | reorganize | delete <name>>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$dev-makefiles` — ported.

**Use the conversation you're already in.** This skill is user-invoked only, so when it fires, the preceding conversation is the source: what was just built, which commands exist, what the demo should show. Read that context first — don't re-derive the program from scratch and don't interview the user about things already said.

## Where tutorials live

Tutorials are **code**. They are checked in, they live beside what they teach, and they rot when the code moves — which is why this skill owns their whole lifecycle, not just creation.

```
docs/tutorials/
├── run.sh              # menu launcher (auto-discovers buckets; never hand-edited)
├── tutorial.sh         # shared runtime library
├── 01-cli-basics.sh    # a bucket: 5-10 minutes of steps
├── 02-data-setup.sh
└── 03-web-dashboard.sh
```

Tutorials always live at **`docs/tutorials/`** — never the repo root, never a per-project variant. They are documentation that executes, so they belong with the docs; a fixed location also means `make tutorial` and the audit pass are identical in every repo. Both `run.sh` and `tutorial.sh` are copied verbatim from this skill's [lib/](lib/) directory — **never rewrite them per project**; fix the skill's copies and re-deploy if they need changes.

## Buckets and steps

- A **bucket** is one numbered file, **5–10 minutes** of walkthrough, one coherent subject. More than ~10 minutes means split it; under ~3 means merge it.
- Every bucket declares two header comments, which is how the menu builds itself:
  ```bash
  # TUTORIAL-TITLE: Walking the CLI
  # TUTORIAL-MINUTES: 7
  ```
- A **step** is one `tut_step` (what and why, briefly) followed by one `tut_run` (the command). Keep prose to a line or two: the command is the lesson.
- Buckets run in filename order and may depend on earlier ones (bucket 2 can assume bucket 1's database exists) — say so in the title line and in `tut_done`'s next-step hint.

## The runtime API

Source the library, then use only these:

| Call | What it does |
|---|---|
| `tut_title "Name" "one-line intro"` | Bucket header |
| `tut_section "Name"` | Group steps within a bucket |
| `tut_step "What" "why, briefly"` | Numbered step header |
| `tut_run "cmd --arg value"` | **The core.** Shows the command pre-filled and editable; Enter runs it |
| `tut_run_fixed "cmd"` | Same but not editable — for destructive or order-critical commands |
| `tut_open "http://localhost:8420/"` | Opens a Chrome tab (falls back to the default browser) |
| `tut_bulk_offer "Populate the DB" fn_name` | Offers "walk each step" or "[a] just run it all" |
| `tut_require cmd1 cmd2` | Fails early with a clear message if a prerequisite is missing |
| `tut_done "what to do next"` | Summary: step count, elapsed minutes, failures. **Exits non-zero if any step failed** |

`TUT_AUTO=1` (or `run.sh --auto`) runs a bucket unattended with no prompts — used for demos, recordings, and the audit pass.

### Pre-fill everything

This is the whole point, so it is a rule, not a preference: **`tut_run` receives the complete command with every argument already filled in.** Real paths, real flags, real values that work in this project right now. Never `--limit <N>`, never a placeholder the human must replace, never a command that errors unless edited. If a value genuinely can't be known ahead of time, compute it in the script and interpolate it.

## Steps for each operation — parse `$1`

### 1. Create (the default: "`$tutorial` this program", "walkthrough of the CLI")

1. **Read the conversation first**, then the code: the CLI's own `--help` output, subcommands, the Makefile targets, the routes or screens. Prefer running `--help` for real over reading the arg parser.
2. **Verify every command before it goes in a tutorial.** Run it. A tutorial that fails on step 3 in front of an audience is worse than no tutorial — this is the same exercise-don't-guess mandate the rest of the stack uses.
3. **Group into 5–10 minute buckets** by subject, ordered so each builds on the last. Setup and data population come first.
4. **Scaffold if absent**: create `docs/tutorials/`, copy `run.sh` and `tutorial.sh` from this skill's `lib/`, `chmod +x` both. If a project already has tutorials elsewhere (`tutorials/`, `scripts/tutorials/`), `git mv` them into `docs/tutorials/` and re-run `audit` — the move is part of the operation, not a follow-up.
5. **Write each bucket** with the header comments, real pre-filled commands, and `tut_bulk_offer` around any multi-step setup.
6. **Test it for real**: `./docs/tutorials/run.sh --auto <n>` for each new bucket. Every command must exit 0 (or be a deliberate failure the tutorial explains). Fix and re-run until clean — do not hand over an unrun tutorial.
7. **Report**: bucket list with titles and minutes, plus the one line the user types to start (`./docs/tutorials/run.sh`).

### 2. `list` — what tutorials exist

Print each bucket: number, title, declared minutes, step count, and last-modified date. Read-only.

### 3. `update <name>` — a tutorial drifted from the code

Re-run the bucket in `--auto`, find what broke, fix the commands against current behavior, re-run until clean. Update titles and minute estimates if the content changed materially.

### 4. `audit` — check every tutorial against the current code

Run all buckets with `--auto` and report per bucket: pass, or the exact step and command that failed. This is the maintenance pass that keeps checked-in tutorials honest as the code moves; it belongs in the same category as a drifted test. Fix what's broken, or report precisely what needs a human decision.

`./docs/tutorials/run.sh --auto all` exits non-zero when any bucket had a failing step, so the same pass runs unattended as a build check. Wire it under the Makefile's `##@ Test` section (`[skill: $dev-makefiles]`), not `##@ Development` — it executes real commands and asserts. Pass `all` explicitly: `--auto` with no bucket argument falls through to the interactive menu and blocks.

### 5. `reorganize` — buckets have grown lopsided

Re-split by the 5–10 minute rule: break up anything long, merge anything trivial, renumber files so order still reflects dependency. Renumbering renames files — use `git mv`, and re-run `audit` afterward since split buckets can lose setup their steps depended on.

### 6. `delete <name>` — the feature it taught is gone

Confirm the subject is actually gone from the code (grep for the commands it runs), delete the bucket file, renumber the rest with `git mv`, and re-run `audit`. Same evidence standard as pruning a dead test: a failing tutorial is a fix-or-update decision, not automatically a delete.

## Rules

1. **Every command is verified before it ships.** Run it during authoring and re-run the bucket with `--auto` before handing over. Unverified steps are the one failure this skill cannot tolerate.
2. **Pre-fill completely.** No placeholders, no blanks, nothing the human must know to type. Editing is an option they may take, never a requirement.
3. **The human's only required input is Enter.** Anything that needs a real decision gets a `tut_bulk_offer` shortcut or a pre-chosen sensible default — never an open question mid-walkthrough.
4. **Location is fixed at `docs/tutorials/`, and `run.sh` / `tutorial.sh` are copied there, never rewritten per project.** Improvements go back into this skill's `lib/` so every project's tutorials benefit. The menu auto-discovers buckets, so adding a file is the only way to add a menu entry.
5. **A tutorial keeps going after a failed command** — it reports the non-zero exit and continues, so one broken step doesn't end the demo. It still **exits non-zero at the end** if anything failed: the demo survives, the exit code tells the truth. That is what makes `--auto` wireable as a build check.
6. **Destructive commands use `tut_run_fixed`**, and anything that would delete real data, push, or deploy does not belong in a tutorial at all.
7. **Tutorials are maintained like code**: they are checked in, they drift, and `audit` is how that gets caught. A tutorial nobody has run since the CLI changed is a liability, not documentation.
