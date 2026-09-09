---
name: "profile-tools"
description: "Use when someone asks to add/move/find a shell function, alias, or export in their personal-mac-tools profile.d, edit ~/.profile helpers, audit profile.d for conflicts, or invokes $profile-tools."
---


# profile-tools

Maintains the shell helper scripts at `~/workspace/x85446/personal-mac-tools/profile.d/*.sh`. These files are sourced into the user's bash profile in numeric order. Each file has a specific **role**, and new additions need to land in the right file.

## Usage

Argument: <add|move|delete|audit|find|sort|reload> [natural-language request]. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

The skill takes a natural-language request, figures out the right kind of artifact (function, alias, or export), writes it to the correct file, and offers to reload the profile.

## Repository Layout

```
~/workspace/x85446/personal-mac-tools/profile.d/
├── 10-source-and-export.sh   # Early sourcing, basic env vars (no functions)
├── 10-base.sh                # Colors, base utilities, app open-wrappers
├── 20-alias.sh               # Aliases + tiny (<10 line) wrapper functions
├── 30-launchers.sh           # GUI launcher scripts
├── 40-utils.sh               # Heavy utility functions (real logic, >10 lines)
├── 70-remote-installers.sh   # install-* helpers that ssh out and install things
├── 90-apps.sh                # App-specific config & wrappers
├── 90-history.sh             # History config (no functions usually)
└── 99-exports.sh             # PROMPT_COMMAND, PS1, late env exports
```

Files are loaded **in numeric prefix order**. Loading order matters: `99-exports.sh` runs last and can rely on everything above.

### File role rubric (where to place a new artifact)

| Type of artifact | Target file |
|---|---|
| `export FOO=bar` (env var) | `10-source-and-export.sh` for normal vars; `99-exports.sh` only if it depends on functions defined earlier |
| `export FOO="$(cat ~/.ssh/foo.key)"` (env from a file) | `10-source-and-export.sh` (guard with `[ -r ... ]`) |
| `alias x="..."` (true alias) | `20-alias.sh` |
| Function < 10 lines, mostly a wrapper or `cd somewhere` | `20-alias.sh` |
| Function ≥ 10 lines or with real branching/loops | `40-utils.sh` |
| Function that launches a GUI app (`open -a "Foo" ...`) | `30-launchers.sh` or `90-apps.sh` (existing file with similar function wins) |
| Function named `install-*` that bootstraps a remote machine | `70-remote-installers.sh` |
| Tweaking `PS1`, `PROMPT_COMMAND`, history options | `99-exports.sh` or `90-history.sh` |
| `source` of another file or `eval "$(brew shellenv)"` style | `10-source-and-export.sh` |

When the rubric is ambiguous, prefer the file that already has a function with a similar shape. Look at existing functions in the candidate file and match the surrounding style.

## File Anatomy (preserve these on every write)

Every `*.sh` file has this structure:

1. **Shebang:** `#!/usr/bin/env bash`
2. **Load-tracking preamble:**
   ```bash
   script_name=$(basename "${BASH_SOURCE[0]}")
   _profilePrivate_loading "${script_name}"
   ```
3. **ASCII banner header** (figlet-style block)
4. **Section banners** (smaller hash-bar boxes between groups of related functions)
5. **Function/alias/export definitions**
6. **Load-tracking trailer** at end of file:
   ```bash
   _profilePrivate_loaded "${script_name}"
   ```

Some files vary (e.g., `10-base.sh` defines `_profilePrivate_*` before any loading call). **Never reorder these structural elements.** Insert new content **before** the trailing `_profilePrivate_loaded` line, after the last logical section banner.

## Arguments

Subcommand-style. If the first arg is a known subcommand, use it; otherwise treat the entire argument as a natural-language request and pick the right subcommand automatically (default to `add`).

| Subcommand | Form | Purpose |
|---|---|---|
| `add` | `$profile-tools add <natural language description>` | Add a function, alias, or export |
| `move` | `$profile-tools move <name> <from-file> <to-file>` | Move a definition between profile.d files |
| `delete` | `$profile-tools delete <name>` | Remove a function/alias/export |
| `audit` | `$profile-tools audit` | Find duplicate names across files, mis-placed functions, dead code |
| `find` | `$profile-tools find <name-or-substring>` | Show where a name is defined, where else it's referenced |
| `sort` | `$profile-tools sort [file]` | Sort functions alphabetically within sections in a file (preserve banners) |
| `reload` | `$profile-tools reload` | Print the source command and (with confirmation) suggest the user run it |

The example case `I need to export SAM_API_KEY with the content of ~/.ssh/sam.gov.key` → subcommand `add` → artifact type `export` → file `10-source-and-export.sh`.

## Workflow

### Step 1: Classify the request

For `add` (or bare natural-language input), parse the request into:

- **Artifact type:** `export` | `alias` | `function`
- **Name:** the identifier (env var name, alias name, function name)
- **Body:** the code (or a question to derive it if ambiguous)
- **Target file:** by rubric

Heuristics:

| Phrase pattern | Type |
|---|---|
| "export X", "set env var", "make X available as env" | `export` |
| "alias X to", "shortcut for" | `alias` |
| "function that", "add a command that", "I want a tool that" | `function` |

When the body references a file (`with the content of ~/.ssh/sam.gov.key`), guard the export so the profile doesn't break on a missing file:

```bash
[ -r "$HOME/.ssh/sam.gov.key" ] && export SAM_API_KEY="$(<$HOME/.ssh/sam.gov.key)"
```

For commands or paths, expand `~` to `$HOME` inside double quotes.

### Step 2: Check for name conflicts

Before adding, run a duplicate check across the whole `profile.d/`:

```bash
grep -rnE "^(function |alias |export )?[[:space:]]*<NAME>(\(\)| *=)" \
    ~/workspace/x85446/personal-mac-tools/profile.d/*.sh \
    --include="*.sh"
```

If `<NAME>` already exists anywhere (including in `*.backup` files — ignore those), report the existing location and ask whether to:

1. **Update in place** (the existing definition gets replaced)
2. **Skip** (cancel the add)
3. **Move** (delete the old, add the new in the target file)

Don't silently duplicate. Two definitions in two files = the later one wins on load order, but it's a footgun.

### Step 3: Backup the target file

Before any write:

```bash
cp <target-file> <target-file>.bak.skill
```

This is distinct from the user's own `.backup` files (e.g., `20-alias.sh.backup`). On verification failure, restore via `mv <file>.bak.skill <file>`.

### Step 4: Insert in the right place within the file

For each file, identify the **right section** for insertion:

- `10-source-and-export.sh` — group new exports near similar ones (e.g., paths with paths, API keys with API keys).
- `10-base.sh` — function group at top, app-open wrappers (`chrome`, `firefox`) near the bottom.
- `20-alias.sh` — alphabetical within a section. Section banners separate groups (e.g., editing aliases, w_* workspace nav, zoom_*).
- `40-utils.sh` — group by topic banner (ITERM-*, python-*, aider-*, etc.).
- `70-remote-installers.sh` — alphabetical, all functions start `install-*`.
- `99-exports.sh` — append before `setPS1` or after, depending on dependency.

If no obvious section exists, append before the trailing `_profilePrivate_loaded` line.

For new aliases in `20-alias.sh`: maintain alphabetical order **within** the closest existing section. Don't sort the whole file — banners exist to keep groups together.

### Step 5: Use shellcheck to verify

```bash
shellcheck -s bash <target-file> 2>&1 | head -20
```

If shellcheck isn't installed, skip silently. If it flags new errors **introduced by this edit** (compare against errors in the `.bak.skill` baseline), restore from backup and report the error.

Also `bash -n <target-file>` for a syntax-only check — fast, always available. If it fails, restore from backup.

### Step 6: Suggest reload

Print:

```
Edit complete: <file>
New <type>: <name>

Reload your shell to pick it up:
  source ~/.profile

Or alias 'sp' if you have it loaded.
```

For Auto Mode sessions, **don't** run `source ~/.profile` yourself — the Claude shell is a subprocess and sourcing it won't affect the user's interactive shell. Just print the instruction.

### Step 7: Report

Terse summary:

```
profile-tools: add done

File: ~/workspace/x85446/personal-mac-tools/profile.d/10-source-and-export.sh
Type: export
Name: SAM_API_KEY
Inserted at: line 22 (after existing API key exports)
Conflicts: none
Syntax: ok (bash -n)
Shellcheck: ok (no new warnings)
Reload: source ~/.profile
```

## Code Templates

### Export from a file (the SAM_API_KEY case)

```bash
# SAM.gov API key — sourced from ~/.ssh/sam.gov.key
[ -r "$HOME/.ssh/sam.gov.key" ] && export SAM_API_KEY="$(<$HOME/.ssh/sam.gov.key)"
```

Place in `10-source-and-export.sh` near other `export FOO=` lines.

### Simple alias

```bash
alias <name>="<command>"
```

### Simple wrapper function (≤10 lines → 20-alias.sh)

```bash
<name>() {
    <one or two lines>
}
```

### Complex utility (>10 lines or branching → 40-utils.sh)

```bash
<name>() {
    local <var>="$1"
    if [ -z "$<var>" ]; then
        echo "usage: <name> <arg>"
        return 1
    fi
    # ...
}
```

Match the style of the surrounding functions in the target file. Some files use `function name() { }`, others use `name() { }`. Both work; stay consistent locally.

## Audit Subcommand

`$profile-tools audit` runs these checks and reports findings without making changes:

1. **Duplicate names:** Same function/alias/export defined in more than one file (excluding `.backup`)
2. **Mis-placed by rubric:** Functions >20 lines living in `20-alias.sh` (should be in `40-utils.sh`), functions ≤3 lines in `40-utils.sh` (could move to `20-alias.sh`)
3. **Orphans:** Functions defined but never referenced from anywhere else in `~/workspace/x85446/personal-mac-tools/` or `~/.profile`
4. **Broken sources:** `source <path>` lines pointing to files that don't exist
5. **Stale backups:** Suggest archiving `20-alias.sh.backup` or any `*.backup` older than 30 days

Output:

```
profile-tools audit

Duplicates:
  brew()  — defined in 10-base.sh:14 and 20-alias.sh:78  (20-alias.sh wins)

Mis-placed:
  meld()  — 47 lines in 20-alias.sh:412  (suggest move to 40-utils.sh)

Orphans:
  edit_octa()  — 20-alias.sh:153, never called elsewhere

Stale backups:
  20-alias.sh.backup  (Nov 2025, 6 months old)

OK to apply suggestions? (no changes made yet)
```

## Move Subcommand

`$profile-tools move <name> <from-file> <to-file>`:

1. Extract the function block from `<from-file>` (from `<name>() {` or `function <name>() {` to the matching closing `}`).
2. Append to `<to-file>` in the appropriate section.
3. Delete from `<from-file>`.
4. Run bash -n on both files.
5. Restore both from `.bak.skill` if either fails.

For aliases and exports, the block is a single line.

## Guardrails

- **Always backup to `<file>.bak.skill` before writing.** Restore on any syntax failure.
- **Always run `bash -n`** after writing. A broken profile file breaks every new shell.
- **Never edit `*.backup`** files — those are the user's manual archives.
- **Never reorder the load-tracking preamble or trailer.** Insert content between them.
- **Never reorder banner headers.** They are visual structure the user maintains by hand.
- **Don't source `~/.profile` from this skill.** The change won't propagate to the user's interactive shell. Just print the instruction.
- **Don't touch `profile.old/`** — that's the user's archive of legacy profile files.
- **Don't quietly resolve duplicates.** Always ask before replacing or merging.
- **Don't add functions to `10-source-and-export.sh`.** It loads before `_profilePrivate_*` helpers may be defined in `10-base.sh` — keep it to exports and `source` lines only.
- **Don't run `chmod`** on profile.d files.

## What NOT to do

- Don't reformat existing functions — only the new/moved one.
- Don't promote a function to `40-utils.sh` just because it's 11 lines. The rubric is "real logic" not just line count.
- Don't strip comments — they're documentation.
- Don't auto-alphabetize whole files. The numbered banner sections matter; sort within sections.
- Don't create new profile.d files unless the user explicitly requests one.
- Don't quote-wrap `~` outside double quotes — use `$HOME` instead inside quotes.
