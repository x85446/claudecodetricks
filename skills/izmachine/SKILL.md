---
name: izmachine
description: Use when you need a local virtual machine or container on this Mac — create, inspect, run a command inside, snapshot, clone, lock or destroy one, or bring machines up from a machine.iz definition file — through the `izmachine` binary. Covers all six providers (lume, lima, utm, container, docker, incus) behind one grammar. Triggers on "spin up a linux box", "give me a throwaway VM", "run this in a container", "what guests are on this host", "make a macOS VM", "snapshot that guest", "destroy the test box", "is the provider ready", "machine.iz", ".iz file", "izmachine".
argument-hint: <what you want, e.g. "a throwaway linux box called t1" or "run the test suite in dev1" or "destroy t1">
---

# `izmachine` — local VMs and containers

The request: $ARGUMENTS

`izmachine` manages **guests** — VMs and containers — through one grammar, across six
providers. Read this file, then drive the binary. Every command answers in JSON,
so you never parse prose.

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
- `pretty` is the one-line human name and is what the table's OS column shows.
  It carries no contract; do not parse it.

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

**1. Always pass `--format json`.** The human table abbreviates values; the JSON
never does. Never read a table by column position.

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

### Exit codes

| code | `error.code` | meaning | what to do |
|---|---|---|---|
| 0 | — | success | continue |
| 1 | `failure` | general failure | read `message` |
| 2 | `usage` | unknown flag, unknown provider, unknown kind or os, bad arguments, missing `--yes` | fix your command line |
| 10 | `no_such_guest` | no guest by that name | `izmachine list` to see what exists |
| 11 | `locked` | refused: the guest is locked, or carries a lock recorded under an identity that has since moved | do **not** work around it — see Safety |
| 12 | `unsupported_verb` | this provider does not implement this verb | pick a provider that does |
| 13 | `provider_unavailable` | toolkit missing or daemon down | `izmachine doctor` |
| 14 | `izfile_error` | .iz parse or lowering error | fix the .iz file; no file at all also lands here, naming `izmachine init` |

**`izmachine direct run` is the deliberate exception: it returns the guest command's own
exit code, unmodified**, anywhere in 0–255. So a `12` from `run` is ambiguous by
number alone — and is resolved by data, not by counting. Under `--format json`:

- guest exited 12 → `{"exit_code": 12, ...}` on **stdout**, **no `error` key**
- izmachine refused the verb → `{"error":{"code":"unsupported_verb",...}}` on stderr

Check for the `error` key. Never guess from the number.

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
`--local` (act only on this host: remote-backed guests are excluded), `--yes`,
`-v/--verbose`, `-q/--quiet`, `--version`.

`--kind vm|container` and `--osversion <v>` are **`direct create` flags, not
global ones** — they describe a guest being built. Every other verb reads the
kind off the guest it already found.

### Verbs, and which ones touch the host

**Safe — run these speculatively, they mutate nothing:**

| command | answers |
|---|---|
| `izmachine doctor` | every provider's readiness, toolkit path, version; the kinds and platforms each serves; the default provider per (kind, platform) pair |
| `izmachine list` | every guest on the host, whatever runs it |
| `izmachine direct info <guest>` | full detail for one guest |
| `izmachine direct status <guest>` | the uniform header plus that provider's own native view |
| `izmachine locks` | every lock held on this host |
| `izmachine up --dry-run [--print-artifact]` | what a .iz file *would* build, without building it |
| `izmachine validate [-f file.iz]` | the full compile diagnostics — every note, every machine — creating nothing |
| `izmachine info [-f file.iz]` | each machine the .iz file defines: its definition and its current state |
| `izmachine ssh-config [machine]` | ssh_config Host blocks for the .iz machines; a comment, never a fake block, for providers with no sshd |
| `izmachine help --json` | the command tree |

**Mutating — these change host state. Know what you are touching first:**

| command | what it does |
|---|---|
| `izmachine direct create <name> [--kind --os --osversion --provider --image --cpus --memory --disk --tag]` | create a guest |
| `izmachine direct start <guest>` / `izmachine direct stop <guest>` | power state, without destroying |
| `izmachine direct destroy <guest> --yes` | destroy it; `--yes` is required |
| `izmachine direct clone <guest> <new-name>` | copy a guest to a new one |
| `izmachine direct snapshot create <guest> <tag>` | take a snapshot under a tag; refused if the tag exists, `--force` replaces it |
| `izmachine direct snapshot list <guest>` | list a guest's snapshots; empty is exit 0, not an error |
| `izmachine direct snapshot restore <guest> <tag>` | put the guest back on that snapshot; exit 11 while locked |
| `izmachine direct snapshot delete <guest> <tag>` | remove a snapshot and reclaim what backed it |
| `izmachine direct snapshot-to-clone <guest> <tag> <new-name>` | snapshot, then restore it under a new name |
| `izmachine direct export <guest> <path>` / `izmachine direct import <name> <path>` | move a guest to and from a file |
| `izmachine direct lock <guest> [--reason … \| --permanent]` / `izmachine direct unlock <guest>` | see Safety |
| `izmachine direct run <guest> -- <command>` | whatever the command does inside the guest |
| `izmachine init` | scaffold a starter machine.iz; refuses to clobber an existing `[machine]` section |
| `izmachine up [machine] [-f file.iz] [--provider p]` | create **and** provision from a .iz file |
| `izmachine ssh [machine] [-- command]` | a shell — or one command, exit code passed through — in a .iz machine |
| `izmachine provision [machine] [-f file.iz]` | re-run the .iz provision list on running machines, without a recreate |
| `izmachine down [-f file.iz]` | stop every running machine the .iz file defines; idempotent, never destroys |
| `izmachine destroy [machine] --yes [-f file.iz]` | destroy the machines the .iz file defines; `--yes` required, refused while locked |

Argument **order** matters and guessing it costs you a round trip. Every command
in `izmachine help --json` carries its own `usage` string — read that rather than
inferring from the verb's name. A wrong shape is exit 2 / `usage` with a message
naming the correct form, so a mistake here is cheap and self-correcting.

### `run` and the `--` terminator

```bash
$IZ direct run dev1 --format json -- uname -a
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

### `up` — create and provision from a .iz file

`create` gives you a bare guest. `izmachine up` takes a **.iz file** — a
description of a machine, not a script — and goes from nothing to provisioned in
one command. Every file-scoped command resolves the file the same way:
`./machine.iz`, else the directory's sole `*.iz` file, unless `-f` says
otherwise. Two `.iz` files and no `-f` is exit 2 naming both; no file at all is
exit 14 naming `izmachine init`, which scaffolds a commented starter.

**The `[machine]` argument is a lookup key into the file, not a name for the
new guest.** It must match a `[machine.<name>]` section; with a single section
you can omit it. There is no rename mechanism — no `--name`, no `--as` — and
`create`, which does take a name, cannot consume a .iz file. Asking for a name
the file does not define is exit 14:

```
izmachine up mybox
→ {"error":{"code":"izfile_error","exit":14,
    "message":"machine.iz: no machine \"mybox\"; this file defines dev"}}
```

So if you need the guest to land under a different name, **edit a copy of the
file** and change the section's name. Say that you did.

```bash
izmachine up dev                              # the provider named in the file
izmachine up dev --provider incus             # another provider of the same kind
izmachine up dev --dry-run --print-artifact   # compile and show, create nothing
izmachine validate                            # every diagnostic, nothing created
```

**`--dry-run` is the speculative form and the one to reach for first.** It
compiles and stages without creating anything, and `--print-artifact` writes the
generated artifact to stdout instead of leaving it in a build directory — so you
can read exactly what a definition will do before it does it. `izmachine
validate` prints the full diagnostics for every machine in the file. A malformed
file is exit 14 / `error.code: izfile_error`, with the file:line:col in
`message`.

The file is **TOML** — a shared envelope in which izmachine owns the
`[machine]` section and preserves every other top-level table untouched. A
`[machine.<name>]` section holds the scalars `kind`, `os`, `osversion`,
`provider`, `image` and `packages`, a `resources` table, a `tuning` table,
`[[user]]`/`[[file]]`/`[[mount]]` entries and an ordered `[[provision]]` list of
typed steps (`shell`, `cloud-config`, `ansible`, `dockerfile`, `devcontainer`);
every payload is inline or imported from a file next to the .iz file, exactly
one of the two. The full spec is `docs/iz.md` in
the izmachine repo, and `izmachine init` writes a worked starter; read one of
those before writing a .iz file rather than guessing at key names.

**`--provider` overrides the file, but only within the machine's `kind`.**
Pointing a `kind = "vm"` machine at `docker` is exit 2 naming what docker
actually runs; change `kind` in the file if you meant a container. Kind `vm` on
platform linux is served by `lima` and `incus`; kind `container` on linux by
`container`, `docker` and `incus`.

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

Omit `--provider` and you get the **documented static default for the (kind,
platform) pair** — a fixed value, not "whichever toolkit is installed":

| kind | platform | default provider |
|---|---|---|
| `vm` | `linux` | `lima` |
| `vm` | `macos` | `lume` |
| `vm` | `windows` | `utm` |
| `container` | `linux` | `docker` |

`--kind` defaults to `vm`, so `create --os linux` gets you a Lima VM and
`create --kind container --os linux` gets you a Docker container. **Say which
one you meant** — the kind is the difference between a 40-second VM and a
2-second container.

Any command that resolves a provider implicitly names it back to you in the JSON
under `resolution` — `{"kind","os","family","platform","osversion",
"provider","provider_implicit"}` — so you never have to guess which one ran.
`os` is what you asked for verbatim, `family` the distribution lineage it
resolved to, `platform` the axis the provider was chosen on.

Providers do **not** implement the same verbs. `izmachine help --json` lists each one's
declared verb set under `providers[].verbs`, and its `emulated` map explains
where a verb is reached by a different mechanism than you would assume — a lima
clone rebuilds from the source's config rather than copying its disk; a
`container`/`docker` snapshot create captures the filesystem but not running processes;
`ssh` on the container providers execs a shell because there is no sshd. Read
`emulated` before relying on a verb's exact semantics.

**Run `izmachine doctor --format json` before you commit to a provider.** A provider
being installed does not mean it is usable — a daemon can be down, and you find
out as exit 13 / `provider_unavailable` partway through your work instead of in
one cheap read-only call up front. `doctor` reports `ready` per provider with the
reason, so choose from what it says is ready, not from the list below.

Rough guide when you just need a Linux box:

- **`container` or `docker`** (kind `container`) — seconds to create, full verb
  set including snapshot/restore/clone/export/import. Best default for a
  throwaway. These are two distinct providers, not synonyms for "a container":
  `container` is Apple's container CLI, `docker` is Docker. Separate daemons, so
  either can be down while the other is fine, and "use a container provider"
  does not pick one for you — name the one you mean.
- **`lima`** (kind `vm`) — a real VM with its own kernel, ~40 s to create. No
  snapshot verb. Reach for it when the work needs a kernel the host's cannot
  provide — kernel modules, a different kernel version, an init system under
  test.
- **`incus`** — serves both kinds, but read-mostly here (`list`, `info`,
  `status`, `run`), and its
  remotes are real infrastructure rather than local scratch. Not a place to
  create throwaways.

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

## Safety — the rules that matter most

**Only destroy guests you created.** This host is shared. `izmachine list` shows
guests belonging to other people and other sessions; they are not yours to stop,
destroy or reconfigure. Give anything you create a distinctive throwaway name so
you can tell yours apart later.

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
  was placed, then the guest was renamed or destroyed and recreated, so the
  identity moved out from under it. The hint names both keys —
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
# Naming --provider leaves the kind to it; --kind container --os linux picks
# the same shape when you would rather let the default provider decide.
P=container
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
  13) echo "provider not ready"; $IZ doctor --format json ;;
  *)  echo "failed ($rc): $out" ;;
esac
```

## Gotchas

- **`create` flags are advisory on container providers.** `--cpus`/`--memory` are
  hints there and `--disk` is meaningless; they are real on the VM providers.
  `--tag` is repeatable and comes back in `guest.tags` — a cheap way to mark the
  guests that are yours.
- **A `warnings` array on `list` is not a failure.** A provider whose daemon is
  down reports itself there while the command still exits 0 with everyone else's
  guests. Read it, do not abort on it — and note the key is **absent entirely**
  when nothing went wrong, so index it defensively rather than assuming an empty
  array is there.
- **`izmachine list` reaches remote providers too.** An `incus` remote can return
  hundreds of instances that are real infrastructure, not local scratch. Pass
  `--local` when you only care about this host's guests, or narrow with
  `--provider` or `--os`.
- **`error.code` is the stable identifier, not `message`.** Match on the code.
- **`izmachine --version` prints version, commit, build date and Go toolchain** — worth
  capturing when you report a problem.
