# Providers

Read this when choosing or overriding a provider, when a verb returns exit 12, or when a default (image, kind) surprised you.

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
| `vm` | `windows` | `utm` | `incus` |
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

## Providers do not implement the same verbs

`izmachine help --json` lists each one's declared verb set under
`providers[].verbs`, and `izmachine direct` renders the same thing as a matrix.
Read it rather than assuming — the differences are large:

| provider | kinds | platforms | shape of its verb set |
|---|---|---|---|
| `container` | container | linux | everything: create, run, ssh, snapshot ×4, snapshot-to-clone, clone, export, import |
| `docker` | container | linux | everything, same set as `container` |
| `lima` | vm | linux | create, run, ssh, clone, snapshot ×4 — no export/import |
| `incus` | vm, container | linux, windows | create, run, ssh, snapshot ×4 — no clone, no export/import |
| `lume` | vm | macos | create, run, ssh, clone — **no snapshot at all** |
| `utm` | vm | windows, macos, linux | start, stop, destroy, list, info, status, clone — **no create, no run, no ssh** |

Three consequences worth carrying:

- **`incus` serves Windows only as a VM, from an explicit Windows image.** A
  Windows container is a usage error. Without `--profile`, creation applies the
  host's `default` and `windows` profiles. Commands reach the guest through the
  Incus Windows agent, so Windows setup lives in the prepared image, not in
  izmachine.
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
- **`incus`** — serves both kinds. Its remotes are shared infrastructure that
  placement uses for VMs and containers alike: tag what you create with
  `project=<slug>` and stop or destroy only that.

## What you get when you omit `--image`

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

## `snapshot` is a noun with four verbs

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

## `--provider` costs, measured

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
