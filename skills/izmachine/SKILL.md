---
name: izmachine
description: Use for every virtual machine or container request, wherever the guest should run — create, inspect, run a command inside, ssh into, snapshot, clone, roll back, lock or destroy one, or bring machines up from a .iz definition file — through the `izmachine` binary. Covers all six providers (lume, lima, utm, container, docker, incus) behind one grammar and places each guest by izmachine's own rules (the project's .iz `remote`/`needs`, then the project or user `remote` setting, then the `*` default remote). Triggers on "run this on a linux box", "test it on windows", "check that on a mac VM", "spin up a VM", "make me a linux box", "I need a windows machine", "fresh mac VM", "give me a throwaway box to test on", "run this in a container", "ssh into the VM", "snapshot this VM", "roll it back", "clone that VM", "destroy the test VM", "what VMs do I have", "what guests are on this host", "is the provider ready", ".iz file", "izmachine".
argument-hint: <what you want, e.g. "a throwaway linux box called t1" or "run the test suite in dev1" or "destroy t1">
---

# `izmachine` — VMs and containers, wherever they run

The request: $ARGUMENTS

`izmachine` manages **guests** — VMs and containers — through one grammar, across six
providers and every registered remote. Read this file, then drive the binary.
Every command answers in JSON, so you never parse prose. Where a guest runs is
izmachine's decision, made by the rules in [Where a guest runs](#where-a-guest-runs);
the skill adds no placement opinion of its own.

## The model

A guest has three independent properties. Do not conflate them:

| property | values | what it is |
|---|---|---|
| **kind** | `vm`, `container` | how it runs: its own kernel, or the host's |
| **os** | an id + family + version, e.g. `ubuntu` / `debian` / `24.04` | the operating system inside it |
| **provider** | `lume`, `lima`, `utm`, `container`, `docker`, `incus` | what actually runs it |

`izmachine` dispatches on **kind and platform together**, so a Linux container
and a Linux VM are both Linux and still land on different providers.

`guest.os` is an object, not a string. Read the field you actually mean:

```json
{"id":"ubuntu","family":"debian","version":"24.04","codename":"noble",
 "pretty":"Ubuntu 24.04.3 LTS","build":"","kernel":"6.14.0-33-generic","arch":"aarch64"}
```

- `id` and `family` are the anchors — `id` is what the OS calls itself, `family`
  is what it is compatible with: `debian`, `rhel`, `alpine`, `suse`, `arch`,
  `darwin`, or a bare `linux` when it could not be narrowed. So `ubuntu` is
  family `debian` and that is the field to read before you reach for `apt`.
  Match on these, never on `pretty`.
- **Every other field can be empty, and empty is an answer, not a gap.** Alpine
  publishes no codename. `kernel` and `arch` come from the running guest, so a
  stopped guest has neither — `izmachine` reports what the provider knows rather
  than waking it up to ask. `id` goes empty too when the image names no
  distribution `izmachine` knows; `family` and `pretty` always answer.
- `version`, `codename` and `build` narrow the release. `version` is the number
  the OS reports for itself (`24.04`, `15.7.9`), `codename` is the release's name
  where the distribution publishes one (`noble`, `bookworm`), and `build` carries
  the build identifier for the OSes that have one instead of a version — Windows
  and macOS answer here where a Linux distribution answers with `version`. Read
  `build` before assuming a guest with an empty `version` is unidentified.
- `pretty` is the one-line human name and is what the table's OS column shows.
  It carries no contract; do not parse it.

**One place `os` is a plain string, deliberately:** the file-scoped `izmachine
info` reports each machine's *declared* os from the .iz file — `"os":"alpine"` —
because a machine that has not been created yet has no guest to ask. The
structured object is what a **guest** reports, so it comes from `izmachine direct
info` / `direct status` / `list`, never from `izmachine info`.

### Four words, four questions

`izmachine` uses a separate word for each of these, and none of them borrows
another's. When a payload confuses you, find which question you are asking:

| word | question | values |
|---|---|---|
| `kind` | how does it run? | `vm`, `container` |
| `family` | which distribution is it? | `debian`, `rhel`, `alpine`, `suse`, `arch`, `darwin`, `windows`, `linux` |
| `platform` | what does dispatch turn on? | `linux`, `macos`, `windows` |
| `lowering` | which artifact does the .iz file compile to? | `cloud-init`, `oci` |

`family` is what decides `apt` vs `dnf` vs `apk`; `platform` is that family
reduced to what a provider can actually be built for. A guest's `os.family` is
always the distribution; `doctor` and `resolution.platform` speak platforms; and
`--os` accepts either an id or a platform standing for any member of it.

One command shows all four — `up ci --dry-run --format json` on a
`kind = "container"`, `os = "alpine"` machine via incus reads kind `container`,
family `alpine`, platform `linux`, lowering `cloud-init`. The lowering is
`cloud-init` there rather than `oci` because **the artifact is a property of the
provider, not of the kind**: an incus container runs an init and reads
cloud-init.

## Finding the binary

Prefer `izmachine` on PATH. If it is not there, use the repo build at
`/Users/travis/workspace/izuma/izmachine/bin/izmachine` (build it with `make build` in
`/Users/travis/workspace/izuma/izmachine` if it is missing). Set it once and reuse:

```bash
IZ=$(command -v izmachine || echo /Users/travis/workspace/izuma/izmachine/bin/izmachine)
```

## The machine contract — read this before your first command

**1. Always pass `--format json`. Never read the human table.** This is not a
style preference: the table is a *lossy rendering*, on purpose. It holds itself
to 80 columns, abbreviates values, and marks what it cut with `…`. JSON is the
lossless one and is never abbreviated, never elided, and never narrowed. See
[Never read the table](#never-read-the-table) for exactly what the table changes.

**2. Success goes to stdout. Errors go to stderr. Both are JSON.** If you capture
only stdout, every failure looks like an empty string. Capture both:

```bash
out=$($IZ direct info dev1 --format json 2>&1); rc=$?
```

**3. Read the exit code as many-valued, never as a boolean.** Capture it into a
variable and match it. Never pipe a command into `grep -q` to test it — under
`pipefail` you get 141 and learn nothing.

**4. On failure, the JSON is a single `error` object** — act on `error.code`, the
stable string. `exit` repeats the numeric code; `message` and `hint` are for
humans; `guest` and `provider` appear when they apply.

```json
{"error":{"code":"no_such_guest","exit":10,"message":"no such guest: dev9",
          "hint":"izmachine list shows the guests on this host","guest":"dev9"}}
```

**5. Two commands answer in prose even under `--format json`, because their
output *is* text meant to be pasted, not data:** `izmachine init` (which prints a
status line and writes the file) and `izmachine ssh-config` (which prints
`ssh_config` Host blocks, or a `#` comment for a provider with no sshd). Do not
try to parse either as JSON; read their exit code and, for `init`, the file.

### Exit codes

Generated from the binary; `izmachine help --json` carries the same table under
`exit_codes`, so it can be re-read rather than trusted.

| code | `error.code` | meaning | what to do |
|---|---|---|---|
| 0 | — | success | continue |
| 1 | `failure` | general failure | read `message` |
| 2 | `usage` | unknown flag, unknown provider, unknown kind or os, bad arguments, missing `--yes`, an ambiguous guest name | fix your command line |
| 10 | `no_such_guest` | no guest by that name | `izmachine list` to see what exists |
| 11 | `locked` | refused: the guest is locked, or carries a lock recorded under an identity that has since moved | do **not** work around it — see Safety |
| 12 | `unsupported_verb` | this provider does not implement this verb | pick a provider that does |
| 13 | `provider_unavailable` | toolkit missing or daemon down | `izmachine doctor` |
| 14 | `izfile_error` | .iz parse or lowering error | fix the .iz file; a missing `provider =` line and no file at all both land here |

**`izmachine direct run` is the deliberate exception: it returns the guest command's own
exit code, unmodified**, anywhere in 0–255 (`help --json` records it as
`guest_exit`). So a `12` from `run` is ambiguous by number alone — and is
resolved by data, not by counting. Under `--format json`:

- guest exited 12 → `{"exit_code": 12, ...}` on **stdout**, **no `error` key**
- izmachine refused the verb → `{"error":{"code":"unsupported_verb",...}}` on stderr

Check for the `error` key. Never guess from the number.

## Never read the table

The human surface and the machine surface are deliberately different, and the
difference is a contract you can rely on in both directions.

**The table abbreviates. These are the substitutions it makes:**

| field | table shows | JSON always carries |
|---|---|---|
| kind | `vm`, `ctr` | `vm`, `container` |
| state | `up`, `down`, `pause`, `?` | `running`, `stopped`, `paused`, `unknown` |
| platform | `linux`, `osx`, `win` | `linux`, `macos`, `windows` |
| provider source (`doctor`) | `core`, `plug` | `builtin`, `plugin` |
| anything too wide | truncated, ending `…` | the full value |

A value with no abbreviation defined prints unchanged — nothing is ever
truncated into an invented word.

**`--format json` is never abbreviated, never elided, and never narrowed by
`-c`.** The column selector below is a *table* feature. Passing `-c n` alongside
`--format json` does not remove a single field from the JSON; you still get every
guest with every key. That is a design rule of the tool, not an accident, and it
means you never have to ask for columns to get data.

### `-c/--columns` — for when a human wants to look

`izmachine list` takes `-c/--columns`, one character per column, in the order you
want them. Reach for it only when you are *rendering something for a person*;
for your own reading, `--format json` is always the answer.

| char | column | char | column |
|---|---|---|---|
| `n` | name, with a `*` suffix when locked | `p` | provider |
| `k` | kind (`vm`/`ctr`) | `s` | state |
| `o` | os, as one line | `4` | IPv4 address |
| `O` | os id (`ubuntu`, `alpine`, `macos`) | `i` | provider-stable identity, never abbreviated |
| `f` | os family | `l` | label: where the guest came from |
| `V` | os version | `t` | tags |
| `K` | kernel release, on a running guest | `L` | locked: `yes`/`no` |
| `a` | architecture, on a running guest | `w` | remote it runs in (REMOTE) |
| | | `b` | labels, as `k=v` pairs |

The default layout is `nkops` — NAME, KIND, OS, PROVIDER, STATE — and becomes
`nkopsw` when more than one remote is registered or a listed guest is non-local, so the REMOTE column says
where each guest lives. Commas are optional, so `-c nkops`, `-c n,k,o,p,s` and
`-c nk,ops` select the same columns, and a column named twice renders twice.

### Colour

`--color=auto|always|never` (also `--no-color`, `IZMACHINE_COLOR`, and the
`color` config key). **You never need it.** `--format json` is never coloured at
all, and every other output is plain the moment stdout is not a terminal, so a
piped or captured run already gives you clean bytes. Nothing is ever carried by
colour alone: every state, readiness mark, lock tier and error severity is also a
word, a prefix or a mark, so stripping escapes loses no information. `NO_COLOR`,
`--color=never` and `TERM=dumb` all produce byte-identical output to the piped
case.

### Configuration precedence

Every global is resolved through one chain, highest first: **flag > `IZMACHINE_*`
env var > `./izmachine.toml` > `$XDG_CONFIG_HOME/izmachine/config.toml` (falling
back to `~/.config/izmachine/`) > built-in default.** A file is TOML whose
top-level keys are spelled exactly as the flags are (`provider`, `remote`,
`format`, `os`, `color`, `local`, `quiet`, `verbose`, `yes`), plus the
`[[remotes]]` array of tables. `izmachine config show --origin` names the layer
that answered for each value; `izmachine config path` names every file consulted
and whether it exists. **No config file is not an error.**

Set `IZMACHINE_PROVIDER` rather than repeating `--provider` on every call when a
whole session targets one provider — the flag still wins wherever you need it to.

## Discovering the surface

`izmachine help --json` prints the whole command tree as structured data — every
command, its flags with types and defaults, plus top-level `exit_codes`,
`providers` (each with its declared `verbs`, `kinds` and `platforms`),
`default_providers` (one `{kind, platform, provider}` row per pair) and
`version`.
It exits 0 and mutates nothing.

**Use it instead of trusting this file when the two disagree** — it is generated
from the binary you are actually running. Worked examples live on the two group
nodes (`izmachine` and `izmachine direct`), not on the leaves.

`izmachine help <command>` prints one command's help, including the sections the
tree does not carry (`izmachine help list` documents every column character;
`izmachine help direct` prints the provider capability matrix).

## The grammar

```
izmachine [global flags] <command> [args] [flags]
izmachine [global flags] direct <verb> [guest] [flags]
```

Per-guest verbs live under `direct`. The .iz-file commands — `init`, `up`,
`down`, `info`, `ssh`, `ssh-config`, `provision`, `validate`, `destroy` — are
root-level and scoped to the machines the file defines, as are the host-wide
reads `list` and `locks`. `izmachine direct list` and `izmachine direct locks`
are exit-2 stubs naming the root forms.

Global flags: `--format table|json`, `--os <id|platform>`, `--provider <p>`,
`--local` (act only on this host: remote-backed guests are excluded),
`--remote <name>` (the remote to act on; `list` takes it more than once),
`--izfile <file>` (the .iz file the file-scoped commands read instead of
`./.iz`), `--yes`, `-v/--verbose`, `-q/--quiet`, `--version`.

`--kind vm|container`, `--osversion <v>`, `--image`, `--cpus`, `--memory`,
`--disk`, `--tag` and `--opt` are **`direct create` flags, not global ones** —
they describe a guest being built. Every other verb reads the kind off the guest
it already found.

## Command index

Every command in the tree, with a machine-readable invocation for each. This is
the parity contract: `tests/skill-parity.py` walks `izmachine help --json`, and a
command in the tree with no row here is a failure that names the command.

Guest-naming commands carry `--provider` because
[you should always pass it](#pass-provider-on-every-command-that-names-a-guest).

<!-- command-index:begin -->

**Read-only — safe to run speculatively, these mutate nothing:**

| invocation | answers |
|---|---|
| `izmachine --version` | version, commit, build date and Go toolchain |
| `izmachine help --json` | the whole command tree, exit codes, providers, defaults |
| `izmachine doctor --format json` | every provider's readiness, toolkit path, version, kinds, platforms, and the default provider per (kind, platform) pair |
| `izmachine list --format json` | every guest in every remote, whatever runs it, plus `warnings` and `unreachable` arrays |
| `izmachine list --state up --kind container --name 'web-*' --tag role=build --format json` | the same listing narrowed: `--state` (up/down/pause, or their long spellings), `--kind` (vm/container), `--name` (a glob — quote it), `--tag` (`k=v` or a bare `k`, repeatable, ANDed, matching labels and provider tags alike). Composes with `--os`, `--provider`, `--remote`, `--local`. **An empty result is exit 0 with an empty `guests` array, never an error** |
| `izmachine locks --format json` | every lock held on this host, with reason and tier |
| `izmachine remote find --kind vm --format json` | ranked placement candidates with load and reasons; exit 14 on no match |
| `izmachine remote list --format json` | every registered remote: `{remotes:[{name,transport,target,default}],default,source}` |
| `izmachine remote show <name> --format json` | one remote plus the `reach` izmachine acts through |
| `izmachine remote --format json` | the remote command family |
| `izmachine daemon --format json` | daemon command family |
| `izmachine daemon status --format json` | user ownership, protocol, interval and process |
| `izmachine remote migrate --dry-run config.toml --format json` | copied-file migration preview |
| `izmachine config show --origin --format json` | every resolved global as `{values:[{name,value,origin,source,what}]}`; origin is `flag`, `env`, `project`, `user` or `default` |
| `izmachine config path --format json` | every file consulted, whether it exists, and the `IZMACHINE_*` variables read |
| `izmachine config --format json` | the config noun's two verbs |
| `izmachine direct --format json` | the provider capability matrix — which provider implements which verb |
| `izmachine direct info <guest> --provider docker --format json` | full detail for one guest: `{"guest":{name,kind,os,provider,state,ip,identity,label,tags,locked}}` |
| `izmachine direct status <guest> --provider docker --format json` | the same `guest` object plus `native_heading` and a `native` array — that provider's own view |
| `izmachine direct snapshot --format json` | the snapshot noun's four verbs |
| `izmachine direct snapshot list <guest> --provider docker --format json` | a guest's snapshots; empty is exit 0, not an error |
| `izmachine validate --format json -f machine.iz` | full compile diagnostics for every machine in the file, creating nothing |
| `izmachine info --format json -f machine.iz` | each machine the file defines: `{definition,guest,kind,os,provider,exists,running}` |
| `izmachine up dev --dry-run --print-artifact --format json -f machine.iz` | what the file *would* build — `resolution`, `lowering`, `image_mapping`, `artifacts`, `notes` — building nothing |
| `izmachine up --dry-run --format json --izfile machine.iz` | every definition's plan, prerequisites first: `{plan:[{order,definition,guest,remote,provider,after}]}` when there is more than one machine — building nothing |
| `izmachine ssh-config dev -f machine.iz` | `ssh_config` Host blocks (prose, not JSON — see contract rule 5) |

**Mutating — these change host state. Know what you are touching first:**

| invocation | what it does |
|---|---|
| `izmachine daemon run --interval 30s` | run foreground inventory daemon |
| `izmachine daemon start --format json` | start this user's daemon explicitly |
| `izmachine daemon stop --format json` | stop an owned daemon |
| `izmachine daemon install --format json` | install user startup service |
| `izmachine daemon uninstall --format json` | remove user startup service |
| `izmachine remote subscribe incus --format json` | follow eligible client endpoints |
| `izmachine remote unsubscribe incus --format json` | stop source membership tracking |
| `izmachine remote sync --format json` | reconcile subscribed membership |
| `izmachine init -f machine.iz` | scaffold a starter .iz naming a ready provider; refuses to clobber an existing `[machine]` section (prose, not JSON) |
| `izmachine direct create <name> --kind container --os linux --osversion 3.20 --provider docker --image alpine:latest --cpus 2 --memory 2GiB --disk 20GiB --tag mine --opt key=value --format json` | create a guest |
| `izmachine direct start <guest> --provider docker --format json` | power on, without creating |
| `izmachine direct stop <guest> --provider docker --format json` | power off, without destroying |
| `izmachine direct destroy <guest> --provider docker --yes --format json` | destroy it; `--yes` is required |
| `izmachine direct run <guest> --provider docker --format json -- uname -sm` | run a command inside; returns `{guest,provider,command,stdout,stderr,exit_code}` |
| `izmachine direct ssh <guest> --provider docker` | **interactive shell — not for you.** Use `direct run` |
| `izmachine direct clone <guest> <new-name> --provider docker --format json` | copy a guest to a new one |
| `izmachine direct snapshot create <guest> <tag> --provider docker --force --format json` | take a snapshot under a tag; refused if the tag exists, `--force` replaces it |
| `izmachine direct snapshot restore <guest> <tag> --provider docker --format json` | put the guest back on that snapshot; exit 11 while locked |
| `izmachine direct snapshot delete <guest> <tag> --provider docker --format json` | remove a snapshot and reclaim what backed it |
| `izmachine direct snapshot-to-clone <guest> <tag> <new-name> --provider docker --format json` | snapshot, then restore it under a new name |
| `izmachine direct export <guest> <path> --provider docker --format json` | write a guest to a file |
| `izmachine direct import <name> <path> --provider docker --format json` | build a guest back from that file |
| `izmachine direct lock <guest> --provider docker --reason held-by-agent --format json` | refuse destroys of this guest until unlocked; `--permanent` is one-way — see Safety |
| `izmachine direct unlock <guest> --provider docker --format json` | lift a lock (never `--permanent`) |
| `izmachine direct label <guest> owner=you role=build --provider docker --format json` | set operator labels; `--remove` takes bare keys. Recorded by izmachine host-wide, so every provider carries them; the `note` field names a provider whose own toolkit cannot also carry them |
| `izmachine remote add <name> <transport> --default --format json` | register a place guests run: `local`, `incus:<remote>` or `ssh:<[user@]host>`; writes the user config |
| `izmachine remote default <name> --format json` | make a registered remote the `*` default for guest-targeting verbs |
| `izmachine remote delete <name> --format json` | forget a remote; nothing on the far side is touched |
| `izmachine up dev --provider docker --format json -f machine.iz` | create **and** provision from a .iz file |
| `izmachine up --format json --izfile machine.iz` | every definition in the file, prerequisites first; one `results[]` row per machine (`up`, `failed` or `blocked`), nonzero exit if any did not come up |
| `izmachine provision dev --format json -f machine.iz` | re-run the file's provision list on running machines, no recreate |
| `izmachine ssh dev --format json -f machine.iz -- uname -a` | one command in a .iz machine, guest's exit code passed through |
| `izmachine down --format json -f machine.iz` | stop every running machine the file defines; idempotent, never destroys |
| `izmachine destroy dev --yes --format json -f machine.iz` | destroy the machines the file defines; `--yes` required, refused while locked |

<!-- command-index:end -->

Argument **order** matters and guessing it costs you a round trip. Every command
in `izmachine help --json` carries its own `usage` string — read that rather than
inferring from the verb's name. A wrong shape is exit 2 / `usage` with a message
naming the correct form, so a mistake here is cheap and self-correcting.

### `run` and the `--` terminator

```bash
$IZ direct run dev1 --provider lima --format json -- uname -a
```

Two rules, both of which bite:

1. **`--` is mandatory.** Without it `izmachine direct run dev1 ls -la` has an ambiguous
   `-la`, and the failure is silent and confusing.
2. **Global flags go BEFORE the `--`.** Everything after it belongs to the guest.
   `run dev1 -- uname -a --format json` hands `--format json` to `uname`, and you
   get prose back and conclude the contract is broken. It is not; you moved the
   flag past the fence.

Under `--format json`, `run` returns `{"guest","provider","command","stdout",
"stderr","exit_code"}` — the guest's streams as data, not mixed into yours.

**Use `run`, not interactive `ssh`.** `izmachine direct ssh <guest>` replaces
itself with an interactive shell and expects a terminal you are sitting at; it
is not for you. The file-scoped `izmachine ssh <machine> -- <command>` *is*
script-safe: it runs the one command and passes the guest's own exit code
through.

### `snapshot` is a noun with four verbs

```bash
$IZ direct snapshot create  dev1 before-upgrade --provider docker --format json
$IZ direct snapshot list    dev1                --provider docker --format json
$IZ direct snapshot restore dev1 before-upgrade --provider docker --format json
$IZ direct snapshot delete  dev1 before-upgrade --provider docker --format json
```

The guest comes before the tag: `snapshot create <guest> <tag>`. There is no
`snapshot-create` verb — snapshots are an object with a noun of their own, and
the bare verb form does not exist. `snapshot-to-clone` is the one hyphenated
form that remains, because it is a single compound operation rather than a
snapshot verb.

What a snapshot *is* differs by provider and `izmachine` tells you rather than
leaving it in a document: a container snapshot captures the filesystem, not
running processes; a VM snapshot may or may not include memory state depending on
the driver. Read `providers[].emulated` in `izmachine help --json` before relying
on a verb's exact semantics.

### `up` — create and provision from a .iz file

`create` gives you a bare guest. `izmachine up` takes a **.iz file** — a
description of a machine, not a script — and goes from nothing to provisioned in
one command. Every file-scoped command reads `./.iz` in the current directory
and nothing else, unless the global `--izfile <file>` (or the command's own
`-f/--file`, the same selection) names another file. A named file such as
`backup.iz` is **never** a fallback: with no `./.iz`, every file-scoped command
exits 14 naming `izmachine init` and `--izfile`, and lists the named files it
did not read. Two different paths (`--izfile a.iz -f b.iz`) are exit 2; the same
path twice is fine. There is no parent-directory search and no merging.

**The `[machine]` argument is a definition key, not a guest name.** It must
match a `[machine.<key>]` section, or name the target a section declares as
`<remote>:<guest>`. The guest's real name is the section's `name = "..."`, or
the guest half of `remote = "<remote>:<guest>"`; with neither, the guest is
called what the key is. `remote = "D71:dev"` and `remote = "D71"` plus
`name = "dev"` say the same thing; a `name` that contradicts a qualified remote
is exit 14. Locks, labels, identity and provisioning use the real guest name.
Asking for a key the file does not define is exit 14, and a target no
definition declares is exit 14 too — `up` never invents a definition:

```
izmachine up mybox
→ {"error":{"code":"izfile_error","exit":14,
    "message":".iz: no machine \"mybox\"; this file defines dev"}}
```

So if you need the guest to land under a different name, **edit a copy of the
file** and set `name = "<guest>"` in the section. Say that you did.

**Bare `up` brings up every definition in the file.** A section can wait for
others with `dependency.up = "machine.<key>.up"` (or an array of such quoted
references to *keys*); `up` of any subset brings its prerequisites up first,
each once, in dependency order with ties broken by key. A missing or self
reference, a phase other than `up`, and a cycle are exit 14 before anything is
written. If a machine fails, the ones waiting on it are `blocked` with the
reason, independent ones still come up, and the exit is nonzero. A machine
already up satisfies a dependency; a rerun creates nothing that exists and
tears nothing down.

```bash
izmachine up                                  # every definition, prerequisites first
izmachine up dev                              # [machine.dev] and its prerequisites
izmachine up D71:dev                          # the definition that targets dev on D71
izmachine up dev --provider incus             # another provider of the same kind
izmachine up dev --dry-run --print-artifact   # compile and show, create nothing
izmachine up --dry-run                        # the ordered plan, create nothing
izmachine --izfile backup.iz up               # read backup.iz instead of ./.iz
izmachine validate                            # every diagnostic, nothing created
```

**`--dry-run` is the speculative form and the one to reach for first.** It
compiles and stages without creating anything, and `--print-artifact` writes the
generated artifact to stdout instead of leaving it in a build directory — so you
can read exactly what a definition will do before it does it. `izmachine
validate` prints the full diagnostics for every machine in the file. A malformed
file is exit 14 / `error.code: izfile_error`, with the file:line:col in
`message`.

#### The file, and the one key that is required

The file is **TOML** — a shared envelope in which izmachine owns the `[machine]`
section and preserves every other top-level table untouched. A
`[machine.<name>]` section holds:

| key | required? | what it is |
|---|---|---|
| `provider` | **REQUIRED** | which provider runs it: `lume`, `lima`, `utm`, `container`, `docker`, `incus`, or a plugin's name |
| `name` | optional | the guest's real name; defaults to the section key |
| `remote` | optional | a remote from `izmachine remote list`, or `<remote>:<guest>` naming the guest too |
| `dependency.up` | optional | `"machine.<key>.up"` or an array of them: definitions to bring up first |
| `kind` | optional | `vm` or `container` |
| `os` | optional | an os id (`ubuntu`, `alpine`, …) or a platform (`linux`) |
| `osversion` | optional | the version to build |
| `image` | optional | OCI reference for a container runtime, template name for a VM |
| `packages` | optional | array of package names |

plus a `resources` table, a `tuning` table, `[[user]]`/`[[file]]`/`[[mount]]`
entries, and an ordered `[[provision]]` list of typed steps (`shell`,
`cloud-config`, `ansible`, `dockerfile`, `devcontainer`); every payload is inline
or imported from a file next to the .iz file, exactly one of the two.

**Write `provider = "..."` into every `[machine.<name>]` section you author.** A
machine without it does not compile — the operator names the provider, izmachine
does not guess it for a file:

```
izmachine validate
→ {"error":{"code":"izfile_error","exit":14,
    "message":"machine \"dev\" names no provider: add a provider = \"<name>\" line",
    "hint":"izmachine doctor lists the providers ready here; or use --provider",
    "guest":"dev"}}
```

`izmachine init` writes a scaffold that already names a provider ready on this
host, so scaffolding and editing beats writing one from scratch. The full spec is
`docs/iz.md` in the izmachine repo.

**`--provider` on the command line overrides the file, but only within the
machine's `kind`.** Pointing a `kind = "vm"` machine at `docker` is exit 2 naming
what docker actually runs; change `kind` in the file if you meant a container.

The same file comes up on any provider, because izmachine compiles it to
whatever that provider already consumes: the `cloud-init` artifact — user-data
plus meta-data — for lume, lima, utm and incus, or the `oci` artifact, a build
context plus devcontainer.json, for `container` and docker. `up` and `validate`
report which one under `lowering`. The portable parts (packages, users, files,
shell and ansible provisioning) produce the same guest state either way. **The machine envelope does not**:
`resources.cpus` and `.memory` are advisory on a container provider and
`resources.disk` means nothing there. izmachine names each one
in a note when it is crossed rather than dropping it silently — read those notes
rather than assuming the envelope carried over.

After the guest is up, `izmachine provision` re-runs the file's provision list
live through the provider's exec channel — the guest is not recreated and its
identity does not change. `izmachine down` stops the file's machines without
destroying them; `izmachine destroy --yes` is the file-scoped teardown.

## Pass `--provider` on every command that names a guest

This is the single habit that most improves driving `izmachine`. It is not a workaround
and not a special case — it is how you should write every invocation that targets
a guest by name.

**It is dramatically faster.** Without `--provider`, a lookup asks all six
providers and waits for every one of them. Measured on this host with
`izmachine direct info <guest>`, five calls per form:

| form | per call |
|---|---|
| `--provider container` | **~0.03 s** |
| `--provider lima` | **~0.04 s** |
| `--provider docker` | **~0.8 s** |
| unqualified | **~3.0 s** |

An agent doing this in a loop pays that on every step, so qualifying is worth
roughly **80x** against the unqualified path.

Note the docker row: **qualifying removes the six-way fan-out, but it does not
make every provider equally cheap.** Docker's own CLI round-trip costs ~25x what
Apple's `container` costs here. If you are about to do many lookups and either
provider would do, that difference is worth knowing before you pick.

**It also avoids a refusal you cannot resolve any other way.** Guest names are
only unique *within* a provider, so the same name can exist on two of them. An
unqualified lookup on an ambiguous name refuses rather than guessing:

```json
{"error":{"code":"usage","exit":2,
  "message":"\"izai-amb\" names a guest on 2 providers: container, docker; re-run with --provider to say which"}}
```

This applies to `destroy` too — it refuses rather than picking one, which is the
behaviour you want but not one to lean on. Note the code is plain `usage`, so
ambiguity is identified by the message rather than by a code of its own; passing
`--provider` from the start means you never have to read it.

You already know the provider: it comes back in `guest.provider` from `create`,
`list` and `info`. Carry it forward rather than dropping it.

When two guests really do share a name, `identity.value` is where they visibly
diverge — `container` uses the name itself as the ID, `docker` an opaque
64-character hash — so that is the field to compare when you need to prove you
are looking at two distinct guests rather than one seen twice.

## Choosing a provider

**Say which provider you mean.** In a .iz file it is required and there is no
default at all. On `direct create` there *is* a documented static default per
(kind, platform) pair — a fixed value, not "whichever toolkit is installed" —
but leaning on it is how you end up with a 40-second Lima VM when you wanted a
2-second container:

| kind | platform | default provider | others serving that pair |
|---|---|---|---|
| `vm` | `linux` | `lima` | `incus`, `utm` |
| `vm` | `macos` | `lume` | `utm` |
| `vm` | `windows` | `utm` | — |
| `container` | `linux` | `docker` | `container`, `incus` |

`--kind` defaults to `vm`, so `create --os linux` gets you a Lima VM and
`create --kind container --os linux` gets you a Docker container. Naming
`--provider` explicitly leaves `--kind` to the provider.

Any command that resolves a provider implicitly names it back to you in the JSON
under `resolution` — `{"kind","os","family","platform","osversion",
"provider","provider_implicit"}` — so you never have to guess which one ran.
`os` is what you asked for verbatim, `family` the distribution lineage it
resolved to, `platform` the axis the provider was chosen on, and
`provider_implicit` is `false` exactly when you or the file named it.

### Providers do not implement the same verbs

`izmachine help --json` lists each one's declared verb set under
`providers[].verbs`, and `izmachine direct` renders the same thing as a matrix.
Read it rather than assuming — the differences are large:

| provider | kinds | platforms | shape of its verb set |
|---|---|---|---|
| `container` | container | linux | everything: create, run, ssh, snapshot ×4, snapshot-to-clone, clone, export, import |
| `docker` | container | linux | everything, same set as `container` |
| `lima` | vm | linux | create, run, ssh, clone, snapshot ×4 — no export/import |
| `incus` | vm, container | linux | create, run, ssh, snapshot ×4 — no clone, no export/import |
| `lume` | vm | macos | create, run, ssh, clone — **no snapshot at all** |
| `utm` | vm | windows, macos, linux | start, stop, destroy, list, info, status, clone — **no create, no run, no ssh** |

Two consequences worth carrying:

- **`utm` serves Linux as well as Windows and macOS**, so it shows up as an
  alternative for `vm`/`linux`. But it cannot create or exec, so it manages VMs
  that already exist rather than building you one.
- **`incus` is not read-only.** It creates, starts, stops, destroys, execs and
  snapshots. Its *remotes* are real infrastructure rather than local scratch,
  which is a reason to be careful there, not a limit on the verb set.

`providers[].emulated` explains where a verb is reached by a different mechanism
than you would assume — a lima clone rebuilds from the source's config rather
than copying its disk; a `container`/`docker` snapshot create captures the
filesystem but not running processes; `ssh` on the container providers execs a
shell because there is no sshd; `incus create` leaves the instance stopped.

**Run `izmachine doctor --format json` before you commit to a provider.** A provider
being installed does not mean it is usable — a daemon can be down, and you find
out as exit 13 / `provider_unavailable` partway through your work instead of in
one cheap read-only call up front. `doctor` reports `ready` per provider with the
reason, so choose from what it says is ready, not from the table above.

Rough guide when you just need a Linux box:

- **`container` or `docker`** (kind `container`) — seconds to create, the full
  verb set. Best default for a throwaway. These are two distinct providers, not
  synonyms for "a container": `container` is Apple's container CLI, `docker` is
  Docker. Separate daemons, so either can be down while the other is fine, and
  "use a container provider" does not pick one for you — name the one you mean.
- **`lima`** (kind `vm`) — a real VM with its own kernel, ~40 s to create. Reach
  for it when the work needs a kernel the host's cannot provide — kernel modules,
  a different kernel version, an init system under test. Its snapshot verbs serve
  only `qemu` instances; a `vz` instance is exit 12.
- **`incus`** — serves both kinds, and its remotes are real infrastructure.
  Not a place to create throwaways.

### What you get when you omit `--image`

Every provider has a default, so `create` with no `--image` always produces
something — just not necessarily what you assumed:

| provider | default when `--image` is omitted | what `--image` takes |
|---|---|---|
| `container`, `docker` | `alpine:latest` | a registry reference — `debian:latest`, `alpine:3.20`, `ubuntu:24.04` |
| `lima` | `template://default` | a template — `template://debian`, or a bare `debian` (auto-prefixed), or a path to a `.yaml` |

**If the distribution matters, pass `--image` — do not inherit the default.**
Either way, the image actually used comes back in the create response as
`guest.label`, and the OS comes back as the `guest.os` object — read
`guest.os.id` and `guest.os.family`, not `guest.os.pretty`.

You do not need to ask the guest yourself. Once it is running, `izmachine` reads
`/etc/os-release` (or `sw_vers`, or the Windows registry) plus `uname -r`/`-m`
and that **is** `guest.os`. A stopped guest reports what its provider knows,
with `kernel` and `arch` empty; start it and ask again if you need those.

## Where a guest runs

Placement happens before any create, and izmachine makes it:

1. **Run from the project root.** The project's `.iz` (`remote =` or `needs =`)
   and `izmachine.toml` (`remote` setting) apply only from there. A guest
   created elsewhere lands wherever that directory's rules say.
2. **Never add `--local` or `--remote` unless the user named a place.** "Spin up
   a linux box" names no place, so the command carries neither flag and
   izmachine resolves the remote: the file's `remote`/`needs`, then the
   project or user `remote` setting, then the `*` default. The user saying
   "on D71" or "here on this Mac" is what puts `--remote D71` or `--local` on
   the line.
3. **Say where it landed.** Read `resolution.remote` from
   `izmachine up --dry-run --format json` before an `up`, or the REMOTE column
   (JSON `remote`) of `izmachine list` after a `direct create`, and name that
   remote in your reply.

## Safety — the rules that matter most

**Only destroy or restore guests you created.** Every remote is shared.
`izmachine list` shows guests belonging to other people and other sessions;
they are not yours to stop, destroy, roll back or reconfigure. Give anything
you create a distinctive throwaway name so you can tell yours apart later.

**Golden sources are off limits.** `macos-tahoe`, `flamedev` with its
`devbase` snapshot, and lima `default` are the bases other guests are cloned
from. Clone from them when asked; never start, stop, restore, relabel or
destroy them.

**Select by tag before by name.** Guest names collide — this host carries two
UTM bundles both named `flamedev2` — so a name alone can act on the wrong one.
Create with `--tag project=<slug>`, then find yours with
`izmachine list --tag project=<slug> --format json` and act on the `identity`
or the `remote:guest` form it reports.

**A Windows guest shell is PowerShell 5.1.** `&&` is a syntax error there;
chain with `;` or separate `direct run` calls, and read `exit_code` from each.

**Leave it running or tear it down as instructed, and say which.** A guest
you created is either left up for the user, or stopped and destroyed when
they asked for a throwaway; the reply names the guest, its remote and which
of the two happened.

**`destroy` requires `--yes`.** Omitting it is a refusal that names the guest
(exit 2), not a prompt. That refusal is the safety rail — do not add `--yes`
reflexively to every command you write.

**A lock means stop.** `izmachine direct lock <guest> --reason "..."` refuses every
destroy of that guest until it is unlocked. Exit 11 / `error.code: locked` is a
decision someone made deliberately, and the refusal is a true no-op — the guest
is left exactly as it was. **Do not unlock someone else's guest to get past it.**
Check before you act: every guest in `list` and `info` carries a `locked`
boolean, and `izmachine locks` shows the whole registry with each lock's reason
and tier.

**Two different situations produce exit 11, and both mean stop.** `error.code` is
`locked` for each, so matching the code is enough to know to halt; the `hint`
tells you which one you hit:

- **The lock matches the guest in front of you.** Ordinary refusal.
- **The guest carries a lock recorded under a *different* identity.** The lock
  identifies a different guest identity from the current guest. The hint
  names both keys —
  `locked as docker:66b3a1…, now docker:6ee7ee…` — and says the guest was
  renamed or the lock outlived a destroyed guest of the same name.

The second shape is the safety-critical one: a registry miss is **not** treated
as permission. `izmachine direct unlock` reaches a lock in either state, including one
whose original guest is gone, so a stale lock is always liftable — but lifting
someone else's is still not yours to do.

Two more things about locks that the provider tables will not tell you:

- **Locking is not a provider verb.** It is a host-level registry layered
  uniformly over all six providers, so `lock`, `unlock` and `locks` work
  everywhere and never return exit 12. You will not find them in
  `providers[].verbs` in `izmachine help --json`, and that absence means "not
  provider-gated", not "unsupported".
- **`--permanent` is a one-way door.** It records a tier **no command can lift**
  — not even `unlock`. Use it only for guests that cannot be rebuilt: a licensed
  install, a golden image whose source media is gone. **If you intend to unlock
  or destroy the guest later, use a plain `lock` and leave `--permanent` alone.**

The lock is recorded against the guest's provider-stable identity, not its name,
so it survives a rename and a stop/start.

## A worked run

Create a throwaway, prove it works, tear it down:

```bash
IZ=$(command -v izmachine || echo /Users/travis/workspace/izuma/izmachine/bin/izmachine)

# What is ready on this host?
$IZ doctor --format json

# Create. Name the provider, then carry it on every command below.
P=docker
$IZ direct create scratch1 --os linux --provider $P --format json

# Inspect: state, ip, kind, os, identity, locked.
$IZ direct info scratch1 --provider $P --format json

# Do work inside it. Flags before the --.
$IZ direct run scratch1 --provider $P --format json -- sh -c 'echo hello; uname -sm'

# Tear down. --yes is required.
$IZ direct destroy scratch1 --provider $P --yes --format json

# Confirm it is gone: expect exit 10, error.code no_such_guest.
out=$($IZ direct info scratch1 --provider $P --format json 2>&1); echo "exit=$?"; echo "$out"
```

Checking each step in a script, correctly:

```bash
out=$($IZ direct info "$g" --provider "$P" --format json 2>&1); rc=$?
case $rc in
  0)  ;;                                   # fine
  2)  echo "usage: $out" ;;                # incl. an ambiguous name if $P was empty
  10) echo "no such guest: $g" ;;          # it never existed, or is already gone
  11) echo "locked — stop here" ;;         # either shape; do not work around it
  12) echo "provider does not do this verb" ;;
  13) echo "provider not ready"; $IZ doctor --format json ;;
  14) echo "izfile: $out" ;;               # incl. a machine with no provider = line
  *)  echo "failed ($rc): $out" ;;
esac
```

## Gotchas

- **`create` flags are advisory on container providers.** `--cpus`/`--memory` are
  hints there and `--disk` is meaningless; they are real on the VM providers.
  `--tag` is repeatable and comes back in `guest.tags` — a cheap way to mark the
  guests that are yours. `--opt key=value` is repeatable and passes a
  provider-specific option straight through (incus takes `--opt vm=true` for a
  virtual machine, `--opt network=<net>` for a NIC).
- **A `warnings` array on `list` is not a failure.** A provider whose daemon is
  down reports itself there while the command still exits 0 with everyone else's
  guests. Read it, do not abort on it — and note the key is **absent entirely**
  when nothing went wrong, so index it defensively rather than assuming an empty
  array is there. Under `--format json` each warning carries the toolkit's own
  words in full; the human table is where they are reduced to a refusal plus a
  next action.
- **`izmachine list` reaches remote providers too.** An `incus` remote can return
  hundreds of instances that are real infrastructure, not local scratch. It also
  makes the command slow (~2 s on this host). Pass `--local` when you only care
  about this host's guests, or narrow with `--provider` or `--os`.
- **`error.code` is the stable identifier, not `message`.** Match on the code.
- **`izmachine --version` prints version, commit, build date and Go toolchain** — worth
  capturing when you report a problem.

## Remote and placement recipes

Guest `name` in JSON and NAME in tables is bare. JSON `remote` and table REMOTE
say where it runs; the stable lock identity still carries `remote:instance`.
Use `izmachine direct info --remote cypress k3s-dev`,
`izmachine direct info cypress:k3s-dev`, or
`izmachine direct info cypressLinux:k3s-dev`. A bare name prefers the `*`
default; if absent there and present in two other remotes, exit 2 names
both and asks for `--remote`. An exact `remote:guest` works for every direct
verb on Incus and SSH remotes; a conflicting `--remote`, a glob or several
targets are refused without writes, and two providers holding the name on that
remote require `--provider`. Acting verbs refuse two different `--remote`
values.

**Select remotes in a listing** with positional `remote:` (all guests there),
`'remote:glob'` (quote it) and a repeatable `--remote`. They form one
deduplicated union; `--name`, `--state`, `--kind`, `--tag`, `--os` and
`--provider` intersect it. Only the selected remotes are collected, so another
remote's failure never appears. Names are case-sensitive. An unknown or
wrong-case name (with a did-you-mean), a selector with no colon, a malformed
glob, or `--local` beside a selector is refused before anything is scanned.
`IZMACHINE_REMOTE` narrows a listing only when the command line has no
selectors.

`izmachine remote find --kind vm --arch arm64 --class build --format json`
returns `{chosen,candidates:[{name,transport,load,why}]}`. `--class` repeats;
all tags must match. No match exits 14. Unknown load ranks last; ties prefer
`*`, then alphabetical order. SSH candidates use the destination daemon snapshot capabilities and load.
Missing capabilities do not qualify a candidate.

On `[machine.dev]`, use `remote = "cypress"` or `remote = "izuma:<uuid>"`,
or `needs = { kind = "vm", arch = "arm64", class = ["build"] }`.
Never use both keys. `up` and `direct create --remote cypress` create on the
resolved remote. `validate --format json` has each machine's
`resolution.remote` and optional `resolution.placement`; `up --dry-run`
returns the same trace under `resolution`.

Visible remotes merge directory → project → user by name. The directory
file is `$XDG_CONFIG_HOME/izmachine/directory.toml`, overridable by `directory`
in config or `IZMACHINE_DIRECTORY`. Directory names cannot be shadowed; a
locked directory default stays `*`. `remote show --format json` includes
id, source, locked, capabilities and load inside `remote`.

Needs placement reuses an existing eligible guest when load changes; it ranks
new machines by load. Multiple eligible copies require `--remote`.


## Cached inventory and SSH hosts

`make install` (including dev/production modes) installs and starts the daemon,
with a per-user startup service on macOS/Linux. Reinstallation restarts it;
`make uninstall` removes the service and binary. For binary-only installs,
start each user's daemon explicitly: `izmachine daemon start`. Use
`izmachine list --refresh --format json` to wait for initial inventory.
Ordinary lists are cache-only; read `snapshots` freshness/observation errors
alongside `warnings` and `unreachable`. Warming or stale inventory is not
proof that a guest is absent. Missing/incompatible daemons need start/upgrade;
listing never installs a service or falls back to scanning.

`list --refresh` returns a collection that started after the request and waits
up to the fetch timeout. A remote or provider whose rows stay stale for
`hide_after_stale` drops out of the listing; a note names it, how long it has
been stale, and `izmachine list --refresh` to retry. JSON `snapshots` carry
`stale_since` and `hidden` counts. Staleness survives daemon restarts and a
successful refresh restores the rows. Treat hidden rows as unknown, not absent.

The daemon bounds its load on remotes: each Incus endpoint is listed at most
once per refresh across every cached scope, never twice at once, and backs off
after a listing outruns `fetch_timeout`; guest OS details are probed once per
guest boot. A scope unread for 10 minutes stops refreshing (the next read
answers stale and wakes it). If the daemon is loading a remote you do not
own, run `izmachine daemon uninstall` then `izmachine daemon stop`; never
restart the remote's services instead.

The cache policy is the `[daemon]` table of the user config
(`~/.config/izmachine/config.toml`; project and directory files are ignored):
`refresh_interval` (30s), `stale_after` (1m, at least `refresh_interval`),
`fetch_timeout` (30s) and `hide_after_stale` (5m; `0s` never hides). Values
are positive duration strings, unknown keys are errors, and flags on
`daemon run`, `start` and `install` (`--interval`, `--stale-after`,
`--fetch-timeout`, `--hide-after-stale`) beat the file. Settings apply when
the daemon restarts. `izmachine daemon install --dry-run` validates them;
`daemon status` and `config show` report the effective values and origins.
The daemon restarts itself on a replaced executable; a long-running daemon on
an old build makes macOS Local Network privacy deny LAN remotes with "no route
to host".

`daemon run` is foreground operation; `daemon status`, `start`, and `stop`
manage private per-user IPC. `daemon install` / `uninstall` explicitly manage
user startup services on macOS/Linux. Windows startup services are unsupported.

`remote add B ssh:computerB` registers one machine, not a registry source.
Canonical form: `ssh://[user@]host[:port]?providers=lume,docker,lima,utm`.
The same far-side CLI queries that user's daemon for physical-local guests only.
Reciprocal registrations never recurse; `--local` contacts no peers.
`direct` provider operations and single-machine `.iz` actions delegate
through that CLI/daemon, with live destination checks and lock guards. Use
`direct run` for agent commands, preserving argv, stdin and guest exit code;
do not retry a disconnected mutation without inspecting its outcome.
`--tty` is for interactive SSH shells in table output.

`remote subscribe incus`, `unsubscribe incus`, and `sync` follow changing
private Incus instance-serving endpoints. Explicit `incus:<alias>` registrations
monitor only those targets. Public image sources are excluded; derived IDs
remain stable across URL changes. Source failures retain stale membership;
manual/directory entries win. `incus/local` is an endpoint label, not proof of
physical locality. Host/member evidence determines the guest's displayed remote.

`remote migrate --dry-run <paths>...` previews selected legacy files;
`remote migrate <paths>...` applies backup and atomic replacement. Conflicting
declarations/backups are refused. Keep the canonical remote vocabulary in new
commands/files; native third-party protocol keys and migration fixtures are
exceptions.
