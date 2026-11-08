---
name: izmachine
description: "Use when a task needs a VM or container: choose where it runs by keeping a placement directive (needs or remote) in the project's .iz and letting izmachine rank compatible hosts, then create, run commands in, snapshot, clone, lock or destroy guests across all six providers (lume, lima, utm, container, docker, incus) and every registered remote. Also use for explicitly activated IZMachine capacity pools, native work requests and guest leases."
---

# `izmachine` — VMs and containers, wherever they run

Describe the guest, native host or capacity pool to operate, including its
purpose and any explicit placement constraints.

`izmachine` manages **guests** — VMs and containers — through one grammar, across six
providers and every registered remote. Every command answers in JSON, so you
never parse prose. The skill keeps the placement directive in the project's
`.iz`; izmachine chooses the host from it. Start with
[Where a guest runs](#where-a-guest-runs), and read a file under `references/`
when the task needs it (see [References](#references)).

## Where a guest runs

The skill maintains the placement directive; izmachine chooses the host.

1. **Run from the project root and read `./.iz`** (its keys are in
   [references/iz-files.md](references/iz-files.md)). Follow an existing
   definition's `remote` or `needs` as written. Preserve unrelated definitions,
   non-`[machine]` tables and the project's other tools' settings. Definitions
   the project file does not carry come from the user's `global.iz`
   (`izmachine config path --format json` names it); a project key replaces a
   global key wholesale; a global `[defaults]` and `[personal]` section and a
   sibling `.izlocal` layer onto it field by field (the layer rules are in
   the same reference).
2. **Author a definition when one is missing.** If `./.iz` is absent, run
   `izmachine init` (it prints a status line, not JSON, and writes `./.iz`),
   then maintain that file in place. Give the
   requested machine `provider`, `kind`, `os` and a placement directive. Add
   `needs = { kind = "vm" }` for a VM, or the corresponding container kind,
   including `arch` and `class` only when the request requires them. Add that
   directive to a definition that has neither `remote` nor `needs`.
3. **Choose a compatible provider from the live contract.** Consult
   `izmachine help --json` and registered remote capabilities. Preserve a
   declared provider that serves the request. For a Linux VM, Incus supports
   compatible Incus hosts; Lima supports local or SSH-hosted Lima. Use the
   available provider pool that serves the requested kind, OS and features.
   The `needs` directive lets the matcher rank that pool by observed load.
4. **Use `remote = "<name>"` when the user names a place or selector.** `remote` and
   `needs` are mutually exclusive. Preserve explicit location instructions;
   add `--local` or `--remote` only when the user names a place. An unspecified
   location receives a `needs` directive rather than an agent-chosen host.
5. **Validate after every write.** Run `izmachine validate --format json`,
   then `izmachine up <key> --dry-run --format json`. Read and report
   `resolution.remote` and the placement trace before running
   `izmachine up <key> --format json`. Validation and dry-run must succeed
   before creating anything. Preserve an existing guest's identity and locks.

For a macOS VM, declare Lume and let provider compatibility restrict placement
to an Apple host that offers Lume (local or an appropriately registered SSH
remote):

```toml
[machine.mac-test]
provider = "lume"
kind = "vm"
os = "macos"
needs = { kind = "vm" }
```

When the user wants the **physical host itself** — this Mac, or a registered
SSH host — and not a guest on it, declare a native machine: `provider =
"native"`, `kind = "native"`, an explicit `os`, and `remote = "local"` or the
SSH remote. It takes no `needs`, no image and no guest directives, and only
`[[provision]]` steps change it; `down` and `destroy` are a no-op. Keys stay
lowercase (`[machine.mbp]` with `remote = "cypressMBP"`). See
[references/providers.md](references/providers.md).

The command reference follows `izmachine help --json` from the executing binary.
Inspect the live command and flag definitions before authoring a CLI invocation.
Maintain this skill's reference with `make test-skill-parity` when that surface
changes, including selector expressions and saved selectors when the binary
supports them.

## Capacity pools

For a pool, read [references/pools.md](references/pools.md). Define its machine
with the ordinary placement rules, then reference it from
`[izmachine.pool.<name>]`. Registration captures a disabled revision; only
`izmachine pool up` activates maintenance. Ordinary `up` does not activate pools.
Use request IDs and generation claims from JSON, and inspect uncertain outcomes
before retrying work. Check the live provider manifest for supported pool
operations; native host support does not imply guest-backend certification.

## Remote selection

| Selector | Meaning |
| --- | --- |
| `D71` | Exact registered remote |
| `not:local` | Outside this computer, including proven-local Incus aliases |
| `@offbox` | Saved expression from `[selectors]` |
| `class=east,arch=arm64` | Match both keys |
| `class=east,class=west` | Match either value of the repeated key |

Type expressions unquoted: `izmachine remote find --kind vm --remote not:local --format json`
and `izmachine list --remote @offbox --format json`. Keys are name, class, arch,
kind and transport. `not:` negates a term or saved group; `name=local` means the
exact registered name. Expressions avoid whitespace, shell operators, globs,
parentheses, redirections, tilde and dollar expansion. Repeated flags form a
union; acting verbs require one guest. `remote find` JSON carries
`selector: {raw, expanded}` and excluded candidates' `why`.
Read [references/remotes.md](references/remotes.md) for saved groups and recipes.

## Watching usage — `top`

`izmachine top` samples CPU, memory and disk per host and per guest across the
selected remotes. Select remotes the way `list` does: positional `D71:`,
`--remote <selector>` (a name, `@saved`, `not:local`, `class=east,arch=arm64`;
repeat for a union) or `--local`. `-d/--interval` is the time between readings
(default `2s`, at least `1s`); `-n/--iterations` is the number of readings
(default `1`; in a terminal the default runs until quit); `--sort
cpu|mem|disk|name` orders the rows. The first reading takes two samples one
interval apart. A failed remote keeps its last reading and retries after 30
seconds.

In a terminal the keys are `q` or Esc to quit, `c` `m` `d` `n` to sort by CPU,
memory, disk or name, `r` to refresh and `?` to show the keys; Ctrl-C quits
and restores the terminal. **A pipe is batch mode**: the table prints once, and
`--format json` prints one reading per line:

```json
{"sampled_at":"2026-10-20T23:04:11Z","interval":2,
 "remotes":[{"name":"local","transport":"local","reachable":true,"age":0,
   "host":{"cpus":12,"cpu_percent":48.5,"memory_used":27907145728,"memory_total":34359738368,
           "disk_used":1949773037568,"disk_total":1995218165760,"disk_path":"/Users/travis/.izmachine"},
   "guests":[{"name":"dev1","provider":"docker","kind":"container","state":"running","cpus":2,
              "cpu_seconds":12.4,"cpu_percent":3.1,"memory_used":104857600,"memory_limit":null,
              "disk_used":342000,"disk_limit":null}]}],
 "totals":{"hosts":1,"guests":34,"running":1,"cpus":12,"memory_used":27222310912,
           "memory_total":34359738368,"disk_used":1949887287296,"disk_total":1995218165760}}
```

Bytes are integers; usage a provider does not report is `null` (`-` in the
table). A guest's `cpu_percent` counts one core as 100%; a host's spans all its
cores. `age` is the seconds since that remote's last successful reading, and an
unreachable remote (`reachable: false`) carries `error` saying why.

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
[references/model.md](references/model.md) for exactly what the table changes.

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
izmachine [global flags] pool <verb> [identity] [flags]
```

Per-guest verbs live under `direct`. The .iz-file commands — `init`, `up`,
`down`, `info`, `ssh`, `ssh-config`, `provision`, `validate`, `destroy` — are
root-level and scoped to the machines the file defines, as are the host-wide
reads `list`, `locks` and `top`. Four more root nouns ride on the .iz file's
machines: `call` runs an attached function, `fact` reads and stores cached
guest facts, `function` inspects the function library and the hosted
directory, `secret` manages the key that encrypts secret facts; `login`
stores a hosted-directory token. See
[references/iz-files.md](references/iz-files.md#lifecycle-functions-and-facts). `izmachine direct list` and `izmachine direct locks`
are exit-2 stubs naming the root forms.

Global flags: `--format table|json`, `--os <id|platform>`, `--provider <p>`,
`--local` (act only on this host: remote-backed guests are excluded),
`--remote <selector>` (a name, @saved or comma-joined terms; repeat for a union),
`--izfile <file>` (the project-layer .iz file the file-scoped commands read
instead of `./.iz`; the user's `global.iz` sits beneath either), `--yes`,
`-v/--verbose`, `-q/--quiet`, `--version`, and `--lease <request-id>@<generation>`
for mutations under a current pool acquisition.

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
| `izmachine top --local --iterations 1 --format json` | one batch reading of host and guest CPU, memory and disk on this computer: `{sampled_at,interval,remotes:[{name,transport,reachable,error,age,host,guests}],totals}` — see [Watching usage](#watching-usage--top) |
| `izmachine top --remote not:local --interval 1s --iterations 3 --sort mem --format json` | three readings a second apart across the selected remotes, sorted by memory; positional `D71:` selects a remote too |
| `izmachine remote find --kind vm --format json` | ranked placement candidates with load and reasons; exit 14 on no match |
| `izmachine remote list --format json` | every registered remote: `{remotes:[{name,transport,target,default}],default,source}` |
| `izmachine list --remote build --format json` | guests on the docker context `build`; `list build:` and `'build:web-*'` select the same way, and each guest carries `context`. A `--remote` selection asks only what it names |
| `izmachine remote show <name> --format json` | one remote plus the `reach` izmachine acts through |
| `izmachine remote --format json` | the remote command family |
| `izmachine daemon --format json` | daemon command family |
| `izmachine daemon status --format json` | user ownership, protocol, interval and process |
| `izmachine daemon status --remote B --format json` | the SSH remote `B`'s daemon status object, plus `remote` |
| `izmachine daemon install --remote B --dry-run --format json` | what installing `B`'s daemon would do: `{remote,platform,binary,path,command,dry_run:true,installed:false}`, writing nothing |
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
| `izmachine provision --dry-run --format json -f machine.iz` | the provision steps and the `on = ["provision"]` functions that would run, running none |
| `izmachine fact --format json` | the fact noun's four verbs |
| `izmachine fact list dev --format json` | `facts[]` with `name`, `source`, `secret` and TTL metadata — never values; omit the machine for every machine the file defines |
| `izmachine fact get dev ip --format json` | one cached fact, refreshed first when its `ttl` has lapsed; `--refresh` forces it, `--reveal` shows a secret instead of `<secret>`, `--file <path>` writes the value to a 0600 file |
| `izmachine function --format json` | the function noun's five verbs |
| `izmachine function list dev --format json -f machine.iz` | the functions attached to a machine: effective triggers, `where`, origin layer |
| `izmachine function search k3s --account ford.xyz --label role=k3s --os ubuntu --format json` | published directory functions matching the text and filters |
| `izmachine function show izuma:ford.xyz/fetchKubeconfig@1 --format json` | one published function: verified body, hash, signer |
| `izmachine secret --format json` | the secret noun's one verb |

**Mutating — these change host state. Know what you are touching first:**

| invocation | what it does |
|---|---|
| `izmachine daemon run --interval 30s --idle-after 10m --idle-refresh-interval 10m` | run foreground inventory daemon; `start` and `install` take the same policy flags |
| `izmachine daemon start --format json` | start this user's daemon explicitly |
| `izmachine daemon stop --format json` | stop an owned daemon |
| `izmachine daemon install --format json` | install user startup service |
| `izmachine daemon uninstall --format json` | remove user startup service |
| `izmachine daemon install --remote B --format json` | install the SSH remote `B`'s daemon: copy a matching build to `~/.local/bin/izmachine` there, retire its running daemon, run its `daemon install`, enable systemd lingering on Linux and wait for the daemon — `{remote,target,platform,binary,path,installed,version,service,policy,linger,daemon,service_pid}`; `--binary <path>` supplies the build. See [SSH hosts](references/remotes.md#cached-inventory-and-ssh-hosts) |
| `izmachine daemon uninstall --remote B --format json` | remove `B`'s startup service; `~/.local/bin/izmachine` stays |
| `izmachine remote subscribe incus --format json` | follow eligible client endpoints |
| `izmachine remote unsubscribe incus --format json` | stop source membership tracking |
| `izmachine remote subscribe docker --format json` | follow `docker context ls`: one entry per context, source `subscription:docker`; a collision or a context named `local` registers as `docker/<context>`; a discovery failure keeps cached entries with `stale: true` |
| `izmachine remote unsubscribe docker --format json` | stop following contexts; explicit `docker:` registrations stay |
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
| `izmachine remote add <name> <transport> --default --format json` | register a place guests run: `local`, `incus:<remote>`, `ssh:<[user@]host>` or `docker:<context>`; writes the user config |
| `izmachine remote add build docker:echo --format json` | register a docker context the client already knows; an unknown context exits 2 with a `docker context ls` hint. `docker:` runs this host's docker CLI with `--context`; `ssh:` runs izmachine on the host. See [Docker contexts](references/remotes.md#docker-context-remotes) |
| `izmachine remote default <name> --format json` | make a registered remote the `*` default for guest-targeting verbs |
| `izmachine remote delete <name> --format json` | forget a remote; nothing on the far side is touched |
| `izmachine up dev --provider docker --format json -f machine.iz` | create **and** provision from a .iz file |
| `izmachine up --format json --izfile machine.iz` | every definition in the file, prerequisites first; one `results[]` row per machine (`up`, `failed` or `blocked`), nonzero exit if any did not come up |
| `izmachine provision dev --format json -f machine.iz` | re-run the file's provision list on running machines, no recreate |
| `izmachine ssh dev --format json -f machine.iz -- uname -a` | one command in a .iz machine, guest's exit code passed through |
| `izmachine down --format json -f machine.iz` | apply each defined machine's teardown policy; the default stops it |
| `izmachine destroy dev --yes --format json -f machine.iz` | destroy the machines the file defines; `--yes` required, refused while locked |
| `izmachine up dev --recreate --format json -f machine.iz` | lifecycle override: destroy and build the guest fresh from the compiled definition, capturing a new baseline; `--restore[=tag]` puts it back on a snapshot without provisioning, `--reuse` starts what exists, `--snapshot=tag` names the baseline tag. The three modes are mutually exclusive (exit 2); a lock refuses recreate and restore (exit 11); `--dry-run` reports `lifecycle: {action, rule, source}` and touches nothing |
| `izmachine down --destroy --format json -f machine.iz` | teardown override: destroy instead of the file's `on_down`; `--snapshot=tag` snapshots then stops; `--dry-run` reports the decision |
| `izmachine call dev fetchKubeconfig --format json -f machine.iz -- --flag` | run a function attached to a machine, in the guest or on the host per its `where`; stdout comes back verbatim and the exit code passes through. A `where = "host"` function runs with Makefile trust |
| `izmachine fact set dev token --secret - --format json` | store a permanent operator fact (`-` reads the value from stdin); `--secret` encrypts it at rest |
| `izmachine fact gc --format json` | drop cached facts whose guest identity is gone, only for providers whose inventory succeeded |
| `izmachine function add izuma:ford.xyz/fetchKubeconfig@1 --project --format json` | pin a directory function into the personal library, or the project library with `--project` |
| `izmachine function publish fetchKubeconfig --public --format json` | push a library function to its directory record |
| `izmachine secret rotate --format json` | re-encrypt every stored secret fact under a replacement master key; resumable |
| `izmachine login ford.xyz --token <token> --format json` | store a hosted-directory account token; pass the token by flag from a secret store, never typed into a transcript |

**Pool control — activation, requests and cleanup change state:**

| invocation | effect |
|---|---|
| `izmachine pool --format json` | Read the pool command family |
| `izmachine pool acquire build --request-id worker-42 --format json` | Request an exclusive guest lease |
| `izmachine pool authority --format json` | Read the authority command family |
| `izmachine pool authority list --format json` | Read claimed resource domains |
| `izmachine pool authority release <domain> --yes --format json` | Release an authority after its pools and allocations drain |
| `izmachine pool cancel build-42 --format json` | Cancel queued demand or request confirmed termination |
| `izmachine pool destroy <allocation-id> --generation 1 --yes --format json` | Destroy an unoccupied retained allocation |
| `izmachine pool down build --format json` | Disable demand and drain idle capacity |
| `izmachine pool endpoint --format json` | Read the endpoint command family |
| `izmachine pool endpoint list --format json` | Read explicit capacity owners |
| `izmachine pool endpoint register build-host --format json` | Register an existing local or SSH remote as a capacity owner |
| `izmachine pool endpoint remove build-host --format json` | Remove the explicit owner selection |
| `izmachine pool prune <revision> --yes --format json` | Remove exact unused owned prepared images |
| `izmachine pool recover <allocation-id> --generation 1 --format json` | Verify an uncertain unoccupied allocation without replaying work |
| `izmachine pool register build --file machine.iz --format json` | Capture a disabled pool definition |
| `izmachine pool release worker-42 --generation 1 --format json` | Release a current acquisition for lifecycle cleanup |
| `izmachine pool renew worker-42 --generation 1 --format json` | Extend an unexpired current lease |
| `izmachine pool request build-42 --format json` | Read durable request state and results |
| `izmachine pool status build --format json` | Read capacity, occupancy, limits and diagnostics |
| `izmachine pool submit build work --request-id build-42 --format json -- hello 37` | Submit an approved guest function |
| `izmachine pool template --format json` | Read the template command family |
| `izmachine pool template list --format json` | Read captured revisions |
| `izmachine pool template prune <revision> --yes --format json` | Remove an unreferenced captured revision |
| `izmachine pool template select build <revision> --format json` | Select a retained revision while the pool is disabled |
| `izmachine pool unregister build --format json` | Forget a disabled pool after its resources drain |
| `izmachine pool up build --format json` | Activate maintenance for a registered pool |

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

**Close stdin from a harness.** `izmachine ssh <machine> -- <cmd>` and
`direct run` read all non-terminal stdin before running, so from a harness
whose stdin never closes they hang. Call them with `</dev/null` (or pipe the
input you mean to send).

**Use `run`, not interactive `ssh`.** `izmachine direct ssh <guest>` replaces
itself with an interactive shell and expects a terminal you are sitting at; it
is not for you. The file-scoped `izmachine ssh <machine> -- <command>` *is*
script-safe: it runs the one command and passes the guest's own exit code
through.

## Pass `--provider` on every command that names a guest

Carry the guest's provider (it comes back in `guest.provider` from `create`,
`list` and `info`) on every command that names it. An unqualified lookup asks
all six providers and costs about 3 s against 0.03–0.8 s qualified, and a name
held by two providers refuses with exit 2 / `usage` naming both. When two
guests share a name, `identity.value` tells them apart. Measurements and the
docker caveat are in [references/providers.md](references/providers.md).

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

**Pool occupancy requires a current lease.** Inspect `izmachine pool request`
and use its request ID and generation for a claimed mutation. Release or cancel
through the pool owner so cleanup preserves shared accounting. An expired lease
or low CPU does not establish that work ended; inspect `unknown` outcomes before
retrying. Preserve provider instance identity and existing locks.

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

## What to report

End every guest operation with these lines, filled from the JSON:

```
guest:     <name> (<provider>) on <remote>
placement: <needs … → chosen remote | remote = … as declared | --local/--remote as the user named>
.iz:       <created | edited [machine.<key>] | unchanged>
state:     <running | stopped | destroyed>
left:      <running for you | torn down as asked>
```

## References

Each loads only when read. Read the one the task needs.

- [references/iz-files.md](references/iz-files.md) — `up` and the `.iz` file: the
  project and global layers, the `[machine]` keys (including `remote` and `needs`),
  dependencies, provisioning and lowering. Read before authoring or editing `./.iz`.
- [references/pools.md](references/pools.md) — disabled registration, activation,
  approved functions, leases, drain, revisions and explicit remote owners. Read
  before operating a capacity pool.
- [references/remotes.md](references/remotes.md) — addressing a guest on a remote,
  listing selectors, `remote find`, the directory file, the inventory daemon and
  its idle policy, and SSH hosts.
- [references/providers.md](references/providers.md) — the default provider per
  (kind, platform), each provider's verb set, default images, the snapshot noun,
  `--provider` timings, a worked run and gotchas.
- [references/model.md](references/model.md) — kind, os and provider; the
  `guest.os` object; the four words; what the table abbreviates; `-c` columns;
  colour; configuration precedence.
