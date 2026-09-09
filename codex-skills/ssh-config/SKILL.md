---
name: "ssh-config"
description: "Use when someone asks to update/edit/add/delete/move an SSH host or site, wire up an SSH key (identityfile) for a host, modify ~/.ssh/config or ~/.ssh/config.d, manage SSH sites (cypress, df-austin, ed-house, brian), sort SSH config files, set up SSH access to a new machine, **speed up SSH connections** (ControlMaster / multiplexing / \"make ssh faster\" / \"keep connection alive\" / \"reuse the connection\"), or invokes $ssh-config."
---



<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# ssh-config

Manages per-site SSH config files in `~/.ssh/config.d/`. Each file is one **site** (a physical location or logical grouping). Each `Host` block within a file is one **node**.

## Usage

Argument: <add|delete|move|sort|list|new-site|speedup> [args]. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

Adds nodes, removes nodes, moves nodes between sites, and keeps host blocks alphabetical within each site — while preserving the ASCII banner header at the top of each file.

## Layout

- Each file `~/.ssh/config.d/<site>` holds the nodes for one site.
- The main `~/.ssh/config` already does `Include ~/.ssh/config.d/*`.
- Banner header at the top of a file (lines starting with `#`, often ASCII art) is **preserved verbatim** on any sort or rewrite.
- Host blocks start with `Host <name>` (column 0), followed by indented option lines until the next `Host` line or EOF.

### Known sites

| File | Description |
|---|---|
| `cypress` | Local home (Travis's house). No banner. |
| `df-austin` | Izuma Data Foundry site in Austin. ASCII banner: "the site in austin". |
| `ed-house` | Ed's house / Ed's garage. ASCII banner: "ya' know, ed's grarrage". |
| `brian` | Brian's house. Banner to be created on first `new-site brian`. |

### Files to ignore

Skip these when iterating `~/.ssh/config.d/*`:

- Zero-byte files (`1`, `octa`)
- `*.bak`
- `*.sync-conflict-*`
- `old`
- `gke`, `incus-uc-vms` (specialized non-site groupings — only touch if user names them explicitly)

When in doubt about whether a filename is a site, ask once.

## Arguments

The first argument is a subcommand. Subsequent arguments are subcommand-specific.

| Subcommand | Form | Purpose |
|---|---|---|
| `add` | `$ssh-config add <site> <host-name>` | Add a new `Host` block to a site file |
| `delete` | `$ssh-config delete <site> <host-name>` | Remove a `Host` block from a site file |
| `move` | `$ssh-config move <host-name> <from-site> <to-site>` | Move a node between sites |
| `sort` | `$ssh-config sort [site]` | Alphabetize host blocks within a site (or all sites if omitted) |
| `list` | `$ssh-config list [site]` | Print host names in a site (or every site if omitted) |
| `new-site` | `$ssh-config new-site <name> [description]` | Create a new empty site file with a banner |
| `speedup` | `$ssh-config speedup <host-name> [<host-name> ...]` | Add SSH ControlMaster multiplexing to one or more existing host blocks (first connection opens a socket, subsequent reuse it = near-instant) |

If `$ARGUMENTS` is empty or the subcommand is unknown, print a usage line listing the subcommands above and exit.

## Conventions

**Indentation:** 4 spaces inside a Host block. Option names are lowercase (`hostname`, `user`, `port`, `identityfile`, `proxyjump`, `localforward`, etc.).

**Identity files:** Use the **path to the public key** (`~/.ssh/to_xxx.pub` or absolute) so the agent can match against the corresponding private key. If the user gives only a private key path, suggest its `.pub` sibling and confirm.

**Jump hosts:** When a host needs to traverse a jump, set `proxyjump <jumphost-name>`. The jump host itself must be defined as its own `Host` block somewhere (often in the same site file).

**Standard options** to include on most hosts unless the user says otherwise:

```
    identitiesonly yes
    forwardagent yes
    stricthostkeychecking no
    userknownhostsfile /dev/null
    passwordauthentication no
    serveraliveinterval 200
```

These mirror the patterns already in the existing site files. Match the surrounding style of the file being edited — e.g., `cypress` uses lowercase `port`, `df-austin` uses lowercase, `ed-house` mixes `User`/`HostName` casing in some blocks. Stay consistent with what's already in the target file.

## Workflow

### Step 1: Parse args and locate file

1. Read `$1` as subcommand. Reject if unknown.
2. Resolve site name → file path: `~/.ssh/config.d/<site>`.
3. Verify the file exists (or, for `new-site`, that it does not).

### Step 2: Parse the file into structured blocks

Read the file. Split it into:

- **Header** — the contiguous run of comment/blank lines at the top, up to (but not including) the first `Host ` line.
- **Blocks** — each `Host <name>` line and all following lines up to the next `Host ` line or EOF.

Treat the block name as the entire token after `Host ` (case-sensitive). Empty lines and comments inside a block belong to that block.

### Step 3: Apply the subcommand

#### `add <site> <host-name>`

If the host name already exists in any site file, refuse and report which file already has it (don't silently overwrite).

Prompt the user by asking the user to choose from a short numbered list for the minimum fields:

1. **hostname** (IP, FQDN, or `localhost`)
2. **user**
3. **port** (default `22`)
4. **identityfile** — suggest `~/.ssh/to_<something>.pub` based on the site (e.g., `to_cypressPro.pub` for `cypress` site, `to_generic.pub` for most others). Show recent identityfiles seen in the target file as quick options.
5. **proxyjump** (optional) — list existing host names from the same site file as candidates; allow "(none)".
6. **localforward / remoteforward** (optional, can be multiple)

Compose the new Host block using the standard options listed above. Append to the file, then run a sort pass (Step 4).

#### `delete <site> <host-name>`

Find the block. Confirm with the user showing the block's contents before removing (one round, default yes). Drop the block plus its trailing blank line. Write the file back. No sort needed (deletion preserves order).

#### `move <host-name> <from-site> <to-site>`

1. Extract the block from `from-site`.
2. Verify `host-name` does not already exist in `to-site`. If it does, refuse.
3. Remove the block from `from-site` (write back).
4. Append the block to `to-site`, then sort `to-site`.

If `to-site` doesn't exist, suggest running `new-site <to-site>` first; don't auto-create here.

#### `sort [site]`

For each target file:

1. Parse into header + blocks (Step 2).
2. Sort blocks by host name (case-insensitive ASCII, stable).
3. Rejoin: header (verbatim) + sorted blocks separated by a single blank line.
4. Write back atomically (write to `<file>.tmp` then `mv`).

If `[site]` is omitted, run sort on every non-skipped site file.

#### `list [site]`

Print each site's host names. Plain text, one per line, grouped by site:

```
cypress (35 hosts):
  bryanHouse
  chopper2-host
  cypress-dryden
  ...

df-austin (52 hosts):
  ...
```

#### `new-site <name> [description]`

1. Refuse if `~/.ssh/config.d/<name>` already exists.
2. Create the file with a minimal banner. Use the figlet-style block style already in `df-austin` and `ed-house` for visual consistency, but if `figlet` is not installed, fall back to a simple hash-bar banner. Generate via:

   ```bash
   if command -v figlet >/dev/null 2>&1; then
       figlet -f standard "<name>"
   fi
   ```

   Wrap each line with `# ... #` to match the existing 103-column box style.
3. Banner template (simple fallback if no figlet):

   ```
   #-----------------------------------------------------------------------------------------------------#
   #                                                                                                     #
   #                                          <name>                                                     #
   #                                          <description>                                              #
   #-----------------------------------------------------------------------------------------------------#

   ```

   (One trailing blank line so the first added host has spacing.)
4. Report the file path so the user can immediately follow up with `add`.

#### `speedup <host-name> [<host-name> ...]`

Adds SSH ControlMaster multiplexing to one or more existing host blocks. First connection opens a master socket; subsequent connections to the same host reuse the socket — multi-second cold connects become sub-second warm ones.

For each `<host-name>` in the argument list, run the per-host flow below:

1. **Locate the block.** Search all non-skipped files in `~/.ssh/config.d/`. Resolve to (filename, block-start-line, block-end-line).
   - Not found → refuse: "host '<name>' not found in `~/.ssh/config.d/*`. Try `$ssh-config list` to see what's registered." Continue with the next host arg.
   - Found in multiple files → refuse: "host '<name>' found in multiple files: <list>. Fix the duplicate first." Continue with the next host arg.

2. **Inspect existing controlmaster state.** Parse the option lines inside the block (case-insensitive):
   - If `controlmaster auto` AND `controlpath` AND `controlpersist` are ALL already present → report "already sped up: controlmaster=<value>, controlpath=<value>, controlpersist=<value>. Skipping." No write. Continue with next host.
   - If any of the three is present but not all (partial state) → ask by asking the user to choose from a short numbered list: "Partial speedup config exists. Replace with defaults?" Default yes. On yes: remove the existing controlmaster/controlpath/controlpersist lines (Step 4 will re-insert). On no: skip this host.
   - If none present → proceed.

3. **Backup the file** to `<file>.bak.skill` (per the main workflow's Step 6).

4. **Edit the block in place.** Insert these three option lines inside the `Host <name>` block, between the last existing option line and the next `Host ` line (or EOF). Use 4-space indentation to match the block's existing style:
   ```
       controlmaster auto
       controlpath ~/.ssh/cm-%C
       controlpersist 10m
   ```
   The `%C` is OpenSSH's connection-hash token (resolves to a hash of `user@host:port` — collision-free across hosts, no manual naming needed).
   
   If you removed partial state in Step 2: insert at the position where the old `controlmaster` line was, so the block stays grouped sensibly. Otherwise: append after the last existing option line.

5. **Verify** with:
   ```bash
   ssh -G <name> 2>&1 | grep -iE '^(controlmaster|controlpath|controlpersist) '
   ```
   Expect all three lines: `controlmaster auto`, `controlpath <expanded-absolute-path>`, `controlpersist 600`. If any missing or wrong: **restore from `<file>.bak.skill`** and report the parse error.

6. **Measure the speedup** (informative; skip on unreachable host):
   ```bash
   T1=$( { time ssh -o ConnectTimeout=5 -o BatchMode=yes <name> true 2>/dev/null; } 2>&1 | awk '/real/ {print $2}' )
   T2=$( { time ssh -o ConnectTimeout=5 -o BatchMode=yes <name> true 2>/dev/null; } 2>&1 | awk '/real/ {print $2}' )
   ```
   `BatchMode=yes` prevents interactive password prompts on unreachable hosts. If the first ssh fails (non-zero exit), skip measurement and report "host unreachable — config applied, couldn't measure live."

7. **Report** in the standard format:
   ```
   ssh-config: speedup done
   
   File:           ~/.ssh/config.d/<site>
   Host modified:  <name>
   Added:          controlmaster auto
                   controlpath ~/.ssh/cm-%C
                   controlpersist 10m
   Backup:         ~/.ssh/config.d/<site>.bak.skill
   Verified with:  ssh -G <name>
   
   Speedup measured:
     1st ssh (cold, opens master): <T1>
     2nd ssh (warm, reuses):       <T2>
   ```

After ALL hosts in the args list are processed, append a single caveat block (once, not per-host):

```
Caveat — controlpersist holds the master for 10 minutes after the last child exits. If your laptop sleeps or your IP changes, the next `ssh <name>` may hang trying to use a dead socket. Fix with:
  ssh -O exit <name>
If that becomes routine, lower controlpersist (or set `controlpersist no` to close immediately after each child).
```

No sort needed (in-place edit doesn't change block order).

### Step 4: Sort after mutations (add / move)

After any `add` or `move`, always run the sort routine on the modified file(s). This is the invariant: **on disk, every site file is alphabetical**.

### Step 5: Verify

After any write, run `ssh -G <one-affected-host>` to confirm the file still parses (ssh prints the resolved config). If `ssh -G` fails, **restore from a backup** (Step 6) and report the parse error to the user. Don't leave the file broken.

### Step 6: Backup before destructive ops

Before any `add`, `delete`, `move`, or `sort` that touches a site file, copy the file to `<file>.bak.skill` (overwriting the previous skill-backup). This is distinct from the existing `.bak` files (which are user-managed). Restore by `mv <file>.bak.skill <file>` if verification fails.

Don't commit these `.bak.skill` files anywhere — they're working scratch only.

### Step 7: Report

Terse summary on stdout:

```
ssh-config: <subcommand> done

File: ~/.ssh/config.d/<site>
Hosts before: 35
Hosts after: 36
Added: bryanLab (proxyjump fieldstone)

Verified with: ssh -G bryanLab
Sorted: yes
```

## Common scenarios

### Spinning up the new "brian" site and moving Bryan's host out of cypress

```
$ssh-config new-site brian "bryan's house"
$ssh-config move bryanHouse cypress brian
```

After this, `cypress` no longer contains `bryanHouse`, and `brian` contains it as the first (only) block, sorted alphabetically.

### Adding a node to an existing site

```
$ssh-config add df-austin newserver
```

Skill prompts for hostname/user/identityfile/proxyjump, composes the block, appends, sorts df-austin.

### Just alphabetize everything

```
$ssh-config sort
```

Sorts every site file. Banners preserved.

### Speed up a slow ssh host (or two)

```
$ssh-config speedup darkfactory
$ssh-config speedup darkfactory kilroyfactor
```

Adds ControlMaster multiplexing to each named host's block. First ssh opens a master socket; subsequent ssh sessions to the same host reuse it (sub-second). Reports the timing diff and a one-line caveat about stale sockets after laptop sleep.

## Guardrails

- **Never overwrite an existing host block silently.** Refuse with a pointer to where the name already lives.
- **Never delete a file** in `~/.ssh/config.d/` even on `new-site` collision — refuse and report.
- **Always verify with `ssh -G`** after writing. A broken file breaks every shell that touches SSH.
- **Always backup to `<file>.bak.skill`** before any write. Restore on verification failure.
- **Preserve banner headers verbatim.** Don't reformat, re-wrap, or strip comments above the first `Host` line.
- **Don't touch `~/.ssh/config` itself** — only files in `config.d/`. The main config's `Include` line already wires everything together.
- **Skip junk files** listed in "Files to ignore". If user names one explicitly, ask once whether to proceed.
- **Lowercase host names** are preferred when adding (`bryanlab` not `BryanLab`), but if the user gives a specific case, honor it.
- **Don't rewrite a block's option style** when moving — preserve original indentation, casing, and option ordering. Sort operates at the block level, not inside blocks.

## What NOT to do

- Don't merge or dedupe option lines inside a block — preserve what the user wrote
- Don't change the file's existing host blocks just because they don't follow the "standard options" convention — only new blocks added via `add` follow that template
- Don't follow symlinks if any file in `config.d/` is a symlink — leave it alone, ask user
- Don't `chmod` or `chown` — leave permissions alone
- Don't run `ssh-add` or modify the ssh-agent — this skill edits config files and runs read-only `ssh -G` (verification) + `ssh <name> true` (speedup timing). It does NOT touch agent state, keys, known_hosts, or remote files.
