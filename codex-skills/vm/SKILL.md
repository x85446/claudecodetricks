---
name: vm
description: Use when someone asks to run, build, test or debug something on a local macOS, Linux or Windows guest on this Mac, or to create, start, stop, ssh into, snapshot, restore, clone, tag or destroy one. Triggers on "run this on a linux box", "test it on windows", "check that on a mac VM", "spin up a VM", "make me a linux box", "I need a windows machine", "fresh mac VM", "give me a throwaway box to test on", "ssh into the VM", "snapshot this VM", "roll it back", "clone that VM", "destroy the test VM", "what VMs do I have".
---



<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# $vm — local VMs on this Mac, through `osvm`

The request: $ARGUMENTS

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

Brings the machine up on demand through the `osvm` command on PATH, runs the work, then leaves it running or tears it down as instructed.

## Usage

Argument: <what you want, e.g. "run the test suite on a linux box" or "fresh windows VM called win1" or "snapshot vtest then upgrade it">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

Everything goes through **`osvm`, resolved from PATH** (`~/bin/osvm`). Never
invoke it by a repo path, and never call `lume`, `limactl`, `utmctl`, `osxvm`,
`linuxvm` or `winvm` directly. Those toolkits carry safety behaviour `osvm`
routes through — most sharply, the lume CLI's own `stop` SIGINTs the lume daemon
and kills *every* running VM on the host. `osvm` never goes there.

## Live reference — the authoritative `osvm --help`

!`osvm --help`

**Read the block above rather than recalling anything.** It is injected fresh on
every invocation, so which commands exist, which OS implements which of them,
what `--version` accepts per OS, and how `ssh` behaves per OS are all current by
construction. Never quote a support matrix, an exit code or a flag from memory —
this skill deliberately does not restate them, because a restated copy goes stale
and a live one cannot. If the block above shows a bare command line instead of
osvm's actual output, the injection did not fire — run `osvm --help` yourself and
read it before going further.

The two blocks to look at first are `not every toolkit implements every command:`
and `ssh, per OS:`. If a command is missing for an OS, `osvm` exits **2** and
names the gap; do not silently substitute a different guest OS.

## Step 0 — confirm the tool

```bash
command -v osvm      # must print /Users/travis/bin/osvm
osvm doctor          # one line per OS; expect "3 of 3 toolkits ready"
```

If `command -v osvm` finds nothing, run `osvm install` from the osvms checkout
once — never work around it by calling a repo path or a toolkit binary.

A **not ready** toolkit is not a failure of the others. Commands for that OS exit
**3** and say which path is missing; the other two keep working. If the user
wants an OS whose toolkit is not ready, say so and offer the ones that are.

## Step 1 — decide which machine, in this order

1. **The user named a VM.** That name wins over everything below.
2. **Select by tag** — the reliable way, because names are freeform and guessing
   from them is how you end up on someone else's box:

   ```bash
   osvm list --os linux --tag project=<slug> --format json
   osvm list --os windows --tag role=build --format json
   ```

   `<slug>` is the current project directory's basename, lowercased, with
   anything outside `[a-z0-9-]` turned into `-`. Parse with `jq`; the fields are
   `name`, `os`, `state`, `ip`, `label`, `tags`, `provenance`.

   Names are also less unique in practice than they look. This host already
   carries **two** UTM bundles both called `flamedev2`. `osvm status` says so
   outright (`ambiguous: 2 bundles share the name flamedev2`, plus an `also`
   line) rather than silently picking one — but only a tag tells you which of
   the two you meant. Treat a name as a label a human typed, not an identifier.
3. **`<cwd>/.claude/vm.md`** — the project's declared inventory (schema at the
   bottom). Use the VM it names for the requested OS.
4. **Nothing matched — create one on demand.** Do *not* stop to ask. Name it
   `<slug>-<os>` (`myapp-linux`, `myapp-windows`); if `osvm list --format json`
   shows that name already taken by a VM that is not tagged `project=<slug>`,
   append `2`. Go to Step 2, then tag it in Step 3 so the next invocation finds
   it by tag instead of creating a second one.

`--label <dir>` is a second, provenance-based filter: it shows VMs created in a
given directory. Use it to explain where a VM came from, not to choose one —
tags are the selector.

Only ask the user (ask the user to choose from a short numbered list) when the request is genuinely ambiguous about
*which OS* it wants and the answer changes the work. "Run X on a linux box" is
not ambiguous. When you do ask, write the answer into `<cwd>/.claude/vm.md` so
the next invocation proceeds silently.

## Step 2 — bring it up on demand

Read the current state first; act on what it says.

```bash
state=$(osvm list --format json | jq -r --arg n "<name>" '.[]|select(.name==$n)|.state')
```

| `state` | do this |
|---|---|
| empty (no such VM) | mint it — see below |
| `stopped` | `osvm start <name>` — boots and waits until it answers |
| `running` | nothing; it is ready |

**Minting a new one.** Two commands do it and the difference is two orders of
magnitude:

```bash
osvm fresh <name> --os <os>     # clone the golden image — seconds. Try this first.
osvm create <name> --os <os>    # build from nothing — minutes. Fallback.
```

`fresh` copies a golden image, so it is fast but only works where a golden exists
for that OS/template. When it reports there is no golden, fall back to `create`.
`create` is also the right call when the user explicitly wants a VM built from
scratch or wants a non-default `--version` — **tell them the cost before you
start it** (see timings below). `--os` is required on `create`; it defaults to
`linux`.

Then confirm it really answers before you trust it:

```bash
osvm status <name>
```

If a step will run past a minute, say what you are starting before you start it.

## Step 3 — do the work

```bash
osvm run <name> <command>...    # guest's own exit code comes back — test that
osvm ssh <name>                 # interactive shell, all three OSes
```

Read the `a note on 'run'` block in the live help above before composing anything
non-trivial: the three toolkits disagree about quoting and expansion. In short —
**one command per `osvm run`, absolute paths over `$HOME`, no chaining**. Ask the
guest once (`osvm run <name> printenv HOME`) rather than assuming. Test the exit
code, not the output text.

**Windows, in the seconds right after a `fresh` or `start`:** `osvm ssh <name>`
already waits up to **30 seconds** for port 22, on purpose — `fresh` returns as
soon as the guest agent answers, and sshd finishes coming up a few seconds after
that. So let the one command block. Do **not** wrap it in your own retry loop,
and do not declare the guest broken inside that window; both are ways of turning
a normal boot into a false failure. `--wait 0` checks once and fails fast,
`--wait 120` gives a slow boot more room, and `--print` emits a paste-able
command line instead of connecting — `osvm` forwards all three straight through
to the toolkit. Guest exit codes survive over both the agent and ssh transports,
so `osvm run` remains the thing to test against.

Tag a machine you just minted so it is findable next time:

```bash
osvm tag <name> project=<slug> role=<what-it-is-for>
osvm tags <name>          # tags + provenance for one VM
```

`name`, `os`, `state` and `ip` are refused as tag keys — they are computed live
and a stored copy could only drift.

## Step 4 — leave it up, or tear it down

Do what the user asked, and **say which you did**.

- **Told to tear down / "throwaway" / "clean up after"** → `osvm stop <name>`
  then `osvm destroy <name> --yes`. Confirm with `osvm list` that the host is
  back where it started.
- **Told to leave it** → leave it running and name it in your reply, so the next
  request reuses it.
- **Not told either way** → leave it running and say so. Re-running work on a
  machine that is already up is the whole point; destroying by default throws
  that away.

## Step 5 — record what changed

After minting, destroying or goldening a VM, update `<cwd>/.claude/vm.md` so it
still matches reality.

## Rules that are not negotiable

- **Never destroy or restore over a VM you did not create.** Restoring is as
  destructive as deleting. The host's golden sources are off limits outright:
  `macos-tahoe` (macOS), `flamedev` and its `devbase` snapshot (Windows), and the
  lima instance `default`. If someone wants a throwaway, mint one.
- **`destroy` requires `--yes` and has no undo.** Confirm with the user before
  running it on anything not created in this session.
- **`snapshot`, `clone` and `restore` need the VM stopped.** `osvm stop <name>`
  first. Copying a disk that is being written to yields a torn image, not a
  snapshot. `osvm` surfaces the toolkit's refusal rather than working around it.
- **A Windows guest shell is PowerShell 5.1**, where `&&` is a syntax error.
  Chain with `;`, or use separate `osvm run` calls.
- **One heavyweight guest per OS at a time.** macOS takes 8 GiB, Linux up to 4,
  Windows 4. Stop one before starting another of the same kind.
- **Names are flat and globally unique across all three toolkits**, and mean the
  same VM from any directory. There is no project prefixing and no `--project`
  flag; `osvm list` is one host-wide table.

## How long things take

Measured, not estimated.

| | macOS | Linux | Windows |
|---|---|---|---|
| `fresh` | ~17s | ~18s | 67-72s |
| `create` | ~4-5 min | ~1 min cached template, 2-5 min for a new one | ~17.5 min |
| `start` (warm) | 5-15s | 5-15s | 5-15s |
| `snapshot` / `restore` | ~2-5s / ~6s | ~2-5s / ~6s | ~2-5s / ~6s |

## `<project>/.claude/vm.md` schema

```markdown
# VMs for <project>

## linux
- **name**: myapp-linux
- **version**: ubuntu24.04
- **tags**: project=myapp role=build

## macos
- **name**: myapp-mac
- **version**: tahoe

## windows
- (none)

## notes
Anything worth not rediscovering: what a VM is for, what is installed on it.
```

Only the sections that apply need to be present. `version` records the
`--version` selector so a rebuild lands somewhere predictable.
