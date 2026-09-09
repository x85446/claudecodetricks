---
name: "incus"
description: "Use whenever you need to interact with an incus VM or container — create, destroy, exec, snapshot, list, restore from snapshot, debug, or wire SSH access. Triggers on \"launch incus\", \"spin up vm\", \"create container\", \"fresh alma 9 box\", \"exec into\", \"snapshot\", \"destroy this vm\", \"I need a test machine\", \"give me a build env\", etc."
---



<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# $incus — incus VM/container management for Claude Code

Read [routing.md](references/routing.md) for the catalog of remotes, location → ssh-config-file map, and defaults. Re-read whenever defaults might have shifted.

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

Auto-detects whether incus is reachable locally (you're on cypressMini) or whether you must `ssh cypressMini` to reach it. Knows the project's preferred remote from `<project>/.claude/incus.md` (asks once and records if unknown). Reads global routing.md for the catalog of remotes (H91, explorer, polaris2, ranger, houston, mercury, cruz, dc_austin, IncusOS, h94-oidc, local, images). Handles `incusmagic ssh enable travis` for key install and maintains `~/.ssh/config.d/<file>` so the user can `ssh <name>` immediately after creation. Watches for create→destroy→create cycles and proposes snapshot/restore.

## Usage

Argument: <what you want to do, e.g. "make alma 9 vm called X on H91" or "exec into Y" or "snapshot Z">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Step 0: Locate incus

Where you are determines how every command is shaped.

```bash
# Detection (run on entry, cache for the session):
if command -v incus >/dev/null 2>&1 && incus version >/dev/null 2>&1; then
    INCUS_PREFIX=""        # local; you're on cypressMini
else
    INCUS_PREFIX="ssh cypressMini "   # all incus commands must be prefixed
fi
```

The user's incus universe lives on cypressMini (Mac). On any other host (the Warden Linux container, df-austin, etc.) you must `ssh cypressMini` to run incus commands. **`incusmagic` is remote-aware as of v1.3.0** — accepts `<remote>:<name>` syntax directly, or use the top-level `--remote NAME` / `-R NAME` flag, or set `INCUS_REMOTE=<name>` in the environment.

### Locating `incusmagic`

The script is **bundled inside this skill** at `<skill-dir>/incusmagic` (the same directory as SKILL.md and routing.md). Prefer that path over PATH lookup — it always exists and is always version-matched to the skill's instructions:

```bash
# Resolve once at start of session (the dirname trick works whether you load
# SKILL.md from ~/.agents/skills/incus/ or from the canonical repo):
SKILL_DIR="$(dirname "${BASH_SOURCE[0]:-${0}}")"   # if invoked as script; usually you know the path
# Or directly:
INCUSMAGIC="$HOME/.claude/skills/incus/incusmagic"   # global install
# Fallback to PATH if the bundled copy is somehow missing:
[[ -x "${INCUSMAGIC}" ]] || INCUSMAGIC="$(command -v incusmagic 2>/dev/null || echo incusmagic)"
```

The script is also symlinked into `~/bin/incusmagic` (and `~/bin/im`) on cypressMini for interactive shells. The canonical source-of-truth still lives at `~/workspace/izuma/site-infra/ansible/roles/incusHost/files/incusmagic`; run `bash <that-dir>/doit.sh skill` to re-sync the bundled skill copy after edits, or `doit.sh all` to deploy everywhere.

## Step 1: Identify the project's incus context

Each project may have a `<project>/.claude/incus.md` knowledge file recording:
- **Default remote** (e.g. `H91`)
- **Default ssh-config-file** (e.g. `~/.ssh/config.d/df-austin`)
- **VM inventory** (what's been provisioned for this project)
- **Activity log** (create/destroy/snapshot events — used for repetition detection)
- **Snapshots** (named snapshot points)

Workflow:

1. Read `<cwd>/.claude/incus.md` if it exists. Use those defaults.
2. If absent OR the user explicitly named a different remote, use that.
3. If still ambiguous AFTER step 2, ASK the user once (ask the user to choose from a short numbered list). Then **record the answer** in `<cwd>/.claude/incus.md` for future sessions.
4. Project's `CLAUDE.md` may also pin a remote — treat that as authoritative if present.

If `<cwd>/.claude/incus.md` doesn't exist, create it on first write (see [Knowledge file schema](#knowledge-file-schema)).

## Step 2: Parse the operation

Figure out which operation the user wants:

| Trigger phrases | Operation |
|---|---|
| "create", "launch", "spin up", "I need", "give me", "fresh box" | **create** |
| "exec into", "shell into", "run X on Y", "inside the vm" | **exec** |
| "destroy", "delete", "tear down", "wipe", "remove" | **destroy** |
| "snapshot", "save state", "checkpoint" | **snapshot create** |
| "restore", "roll back", "rewind to" | **snapshot restore** |
| "list", "show vms", "what's running" | **list** |
| "ssh access", "enable ssh", "let me ssh in" | **wire-ssh** |

If the operation is ambiguous, pick the most reasonable interpretation and log it. Don't ask just for clarification.

## Operations

### Create

For an incus container or VM. Inputs:

- **Name** (required) — ASK if missing.
- **Image** (default `ubuntu/24.04` if container, image override if VM). For Alma 9: `images:almalinux/9` (container) or `images:almalinux/9/cloud` (VM).
- **--vm** flag → VM. Default is container.
- **Specs**: cpu, memory (GiB), disk (GiB). Defaults from routing.md.
- **Remote** (per step 1).

Command shape:

```bash
$INCUS_PREFIX incus launch <image> <remote>:<name> \
  [--vm] \
  -c limits.cpu=<cpu> \
  -c limits.memory=<mem>GiB \
  -d root,size=<disk>GiB
```

After launch:

1. **Wait for boot** — poll `incus list <remote>:<name>` until status is `RUNNING` and an IPv4 is assigned. Up to 60s.
2. **Install SSH access** (see "Wire SSH" below).
3. **Update `~/.ssh/config.d/<location-file>`** so the user can `ssh <name>` immediately. Read routing.md to determine the file and proxyjump pattern.
4. **Append to the project knowledge file** activity log: `<UTC-ts> <remote>:<name> created (<image>, <cpu>c/<mem>G/<disk>G)`.
5. **Verify**: `ssh <name> echo ok`.
6. **Report** success: remote, name, IP, ssh alias, ssh config file edited.

**Before creating, check the repetition heuristic** (see below).

### Exec

For one-shot commands inside a VM/container:

```bash
$INCUS_PREFIX incus exec <remote>:<name> -- <cmd> [args...]
```

For an interactive shell session: prefer `ssh <name>` if SSH is wired (cleaner). Otherwise:

```bash
$INCUS_PREFIX incus exec <remote>:<name> -- bash
```

(Note: interactive shell over `incus exec` doesn't work inside a non-tty bash invocation. If user wants interactive, they should run it themselves; you can prepare the command line for them.)

### Destroy

```bash
$INCUS_PREFIX incus delete <remote>:<name> --force
```

After destroy:

1. Append to activity log: `<UTC-ts> <remote>:<name> destroyed`.
2. **Do NOT auto-remove the SSH config entry** — the user may recreate with the same name. Comment it out only on explicit request.
3. Report success.

**Before destroying, check the repetition heuristic** (see below).

### Snapshot create

```bash
$INCUS_PREFIX incus snapshot create <remote>:<name> <snap-name>
```

Snap name defaults to `<UTC-ts>` if not specified. Always log to project knowledge file Snapshots section.

### Snapshot restore

```bash
$INCUS_PREFIX incus snapshot restore <remote>:<name> <snap-name>
```

Append to activity log.

### List

```bash
$INCUS_PREFIX incus list <remote>:
```

Or scoped: `incus list <remote>:<name>`.

For a project-wide view, use the project knowledge file's VM inventory.

## Wire SSH access

After creating a new VM, the user typically wants `ssh <name>` to just work. This requires:

1. **Install SSH inside the VM and push the user's public key** via `incusmagic` (v1.3.0+ is remote-aware).

   ```bash
   $INCUS_PREFIX incusmagic ssh enable <remote>:<name> travis        # inline remote
   # OR
   $INCUS_PREFIX incusmagic --remote <name> ssh enable <vm> travis   # default remote flag
   # OR
   INCUS_REMOTE=<remote> $INCUS_PREFIX incusmagic ssh enable <vm> travis   # env var
   ```

   `incusmagic ssh enable ... travis` installs the `to_generic` public key inside the VM, brings up sshd, and prints/appends the SSH config block. The Host alias uses the bare VM name (no remote prefix).

2. **Capture the new VM's IP** (already done after launch):

   ```bash
   $INCUS_PREFIX incus list <remote>:<name> --format=csv -c 4
   ```

   Take the first IPv4. Strip any `(eth0)` suffix.

3. **Update `~/.ssh/config.d/<location-file>`**. Determine `<location-file>` from routing.md based on the remote. Read a sample existing entry in that file to match its identity/port/proxyjump pattern, then insert the new `Host <name>` block at the correct alphabetical position. **Don't touch existing entries.**

4. **Verify**: `ssh <name> echo ok`. If it works, report; if not, surface the error.

## Repetition heuristic

Before every **create** or **destroy** action, scan the project's activity log for entries matching `<remote>:<name>` (same name, regardless of remote).

- **2 prior cycles** (create + destroy ≥ 2 times each): print a soft notice. "FYI: this is the 3rd cycle on <name>. Want me to snapshot at a known-good point so we can restore instead of recreate?"
- **3 prior cycles**: don't ask twice. Proactively suggest a snapshot strategy. Lay out the proposal:
  - Snapshot the current good state (or, if mid-destroy, the previous create's state if still available).
  - Future iterations restore from the snapshot instead of full recreation.
  - Show the commands and wait for the user to confirm or say "no, keep recreating".

The heuristic is a HINT, not a hard rule. If the user has a reason (e.g. testing the create path itself), they'll say so. Log their preference in the knowledge file so you don't keep nagging.

## Knowledge file schema

`<project>/.claude/incus.md`:

```markdown
# Project Incus Knowledge

## Defaults
- remote: H91
- ssh config file: ~/.ssh/config.d/df-austin
- default image (container): ubuntu/24.04
- default image (vm): ubuntu/24.04/cloud

## VMs
| Name | Remote | Type | Purpose | Status | Last action |
|---|---|---|---|---|---|
| alma9-edge-build | H91 | vm | AlmaLinux 9 build/test for distro-pelion-edge | RUNNING | 2026-05-19T10:40Z created |

## Snapshots
- 2026-05-19T11:00Z H91:alma9-edge-build "clean-build-env" (post step 3)

## Activity log
(append-only)
- 2026-05-19T10:00Z H91:alma9-edge-build created (almalinux/9/cloud, 4c/8G/40G)
- 2026-05-19T10:15Z H91:alma9-edge-build destroyed
- 2026-05-19T10:20Z H91:alma9-edge-build created (almalinux/9/cloud, 4c/8G/40G)
- 2026-05-19T10:35Z H91:alma9-edge-build destroyed
- 2026-05-19T10:40Z H91:alma9-edge-build created (almalinux/9/cloud, 4c/8G/40G)

## User preferences
- (e.g. "do not suggest snapshot strategy for alma9-edge-build — testing the recreate path itself")
```

If the file doesn't exist, create it with at least the Defaults section.

## Hard rules

1. **Never invent a name.** Ask if missing.
2. **Never auto-cycle on destroy.** If a destroy would be the start of a 3rd cycle on the same name, ASK first and propose snapshot.
3. **Always use `$INCUS_PREFIX`** — never call `incus` directly unless you've confirmed you're on cypressMini.
4. **Always append to the activity log** before reporting success on create/destroy/snapshot/restore.
5. **Never modify existing `~/.ssh/config.d/<file>` entries** — only insert new ones at the correct alphabetical position.
6. **Don't delete machines you didn't create in this session** without explicit confirmation. The user may have important state.
7. **`incusmagic` is remote-aware (v1.3.0+).** Pass `<remote>:<name>` inline, or set the default via `--remote NAME` / `-R NAME` / `INCUS_REMOTE` env. The script lives at `~/workspace/izuma/site-infra/ansible/roles/incusHost/files/incusmagic`; on cypressMini it's symlinked into `~/bin/incusmagic` and `~/bin/im` so any interactive shell can invoke it. Non-container args (profile / network / image names) are unaffected by the remote prefix.

## Example invocations

> "I need an Alma 9 amd64 vm called alma9-edge-build on H91 with 4cpu, 8G mem, 40G disk."

1. detect cypressMini-or-ssh
2. read `.claude/incus.md` (or create if missing); confirm remote = H91 from user's explicit override
3. check repetition: 0 prior — proceed
4. `incus launch images:almalinux/9/cloud H91:alma9-edge-build --vm -c limits.cpu=4 -c limits.memory=8GiB -d root,size=40GiB`
5. wait for RUNNING + IP
6. `incusmagic ssh enable H91:alma9-edge-build travis` (or with `--remote H91`)
7. capture IP
8. read `~/.ssh/config.d/df-austin` to learn the pattern (no proxyjump for df-austin); insert new Host entry alphabetically
9. log to activity log + VM inventory
10. `ssh alma9-edge-build echo ok` → success
11. report

> "destroy alma9-edge-build"

1. detect
2. read knowledge file — find alma9-edge-build on H91
3. check repetition: this would be 3rd destroy of this name — STOP and propose snapshot strategy. "Want me to snapshot it first as 'pre-destroy' so you can restore later?"
4. user says "no, just destroy"
5. `incus delete H91:alma9-edge-build --force`
6. log to activity log
7. report

> "exec into alma9-edge-build and tell me the os release"

1. detect
2. read knowledge file — alma9-edge-build on H91
3. `incus exec H91:alma9-edge-build -- cat /etc/os-release`
4. show output
