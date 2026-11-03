# Remotes, placement recipes and the inventory daemon

Read this when addressing a guest on a remote, narrowing a listing, previewing placement with `remote find`, or when inventory looks stale.

## Remote and placement recipes

Guest `name` in JSON and NAME in tables is bare. JSON `remote` and table REMOTE
say where it runs; the stable lock identity still carries `remote:instance`.
Use `izmachine direct info --remote cypress k3s-cypress`,
`izmachine direct info cypress:k3s-cypress`, or
`izmachine direct info cypressLinux:k3s-cypress`. A bare name prefers the `*`
default; if absent there and present in two other remotes, exit 2 names
both and asks for `--remote`. An exact `remote:guest` works for every direct
verb on Incus and SSH remotes; a conflicting `--remote`, a glob or several
targets are refused without writes, and two providers holding the name on that
remote require `--provider`. Repeated `--remote` values form a union; acting verbs require one matching guest.

**Pin an Incus server by identity, not by the operator's alias.** In a
machine's `remote`: `incus:server=<name>`, `incus:sha256:<64-hex fingerprint>`
or `incus:server=<name>,sha256:<fingerprint>`. A pin carries no `:guest`
suffix — put the guest in `name =`. It still excludes `needs`. `remote show
<alias>` reports `server_name` and `fingerprint`; a resolution carries
`remote.identity` with the `pin` and the `alias` it bound to (unprobed aliases
under `identity.unprobed`). Two aliases with different fingerprints under one
name pin exit 2 naming both — pin the fingerprint; a strict mismatch exits 14
with expected and observed; no match exits 14 with an `incus remote add` hint.

**Hosted directory records** are `izuma:<account>.<domain>.<uuid>` (the
`izuma:` prefix is optional; first label is the account, last is the UUID,
the rest is the domain), usable as a machine's `remote`. Authenticate once
with `izmachine login <account>.<domain> --token <token>`. The first use of an
unpaired record asks for consent: pass `--yes`, because under `--format json`
without it the command returns top-level `pending_action: {pair, alias,
address}` and exits 14 — read it, do not retry blindly. Directory function
refs are `izuma:<account>.<domain>/<name>@<version>`. A cached record carries
`stale: true` and `warnings` when the directory is unreachable.
`IZMACHINE_DIRECTORY_FAKE` points the client at file fixtures for tests.

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
up to the fetch timeout. The first list of a scope waits up to `fetch_timeout`
too, and reports `warming` when the collection outruns it. A remote or provider whose rows stay stale for
`hide_after_stale` drops out of the listing; a note names it, how long it has
been stale, and `izmachine list --refresh` to retry. JSON `snapshots` carry
`stale_since` and `hidden` counts. Staleness survives daemon restarts and a
successful refresh restores the rows. Treat hidden rows as unknown, not absent.

The daemon bounds its load on remotes: each Incus endpoint is listed at most
once per refresh across every cached scope, never twice at once, and backs off
after a listing outruns `fetch_timeout`; guest OS details are probed once per
guest boot. A provider with no running guest in a scope, or an unavailable
one, rests: it is listed once per `idle_refresh_interval` (10m) instead of
every refresh, and its rows stay fresh through that interval plus
`stale_after`. A guest that starts through izmachine, any successful mutating
command, or `izmachine list --refresh` queries it at once. Docker inventory is
one `docker ps` per refresh, with `docker inspect` only for running or paused
containers; `docker stats` runs only for `izmachine top`, so a listing alone
lets Docker Desktop's Resource Saver idle. A scope unread for `idle_after`
(10m) becomes idle: it leaves the
`refresh_interval` cadence and is refreshed every `idle_refresh_interval`
(10m; `0s` disables idle refresh), so its snapshot stays fresh through
`idle_refresh_interval` + `stale_after`; the next read wakes it. A scope is
keyed by its resolved collection inputs (remotes, providers, filters), so the
same query from two working directories shares one cached snapshot. If the
daemon is loading a remote you do not
own, run `izmachine daemon uninstall` then `izmachine daemon stop`; never
restart the remote's services instead.

The cache policy is the `[daemon]` table of the user config
(`~/.config/izmachine/config.toml`; project and directory files are ignored):
`refresh_interval` (30s), `stale_after` (1m, at least `refresh_interval`),
`fetch_timeout` (30s), `hide_after_stale` (5m; `0s` never hides),
`idle_after` (10m) and `idle_refresh_interval` (10m; `0s` disables, otherwise
at least `refresh_interval`). Values are positive duration strings, unknown
keys are errors, and flags on `daemon run`, `start` and `install`
(`--interval`, `--stale-after`, `--fetch-timeout`, `--hide-after-stale`,
`--idle-after`, `--idle-refresh-interval`) beat the file. Settings apply when
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

An SSH remote answers through its own daemon, and
`izmachine daemon install --remote B --format json` is how `B` gets one. It
reads `B`'s platform with `uname` and copies a build for it to
`~/.local/bin/izmachine` there: this executable when the platforms match,
else `--binary <path>`, else `izmachine-<os>-<arch>` beside this executable or
in `../dist` (`make release`). The ELF or Mach-O header is checked before the
copy. It then retires `B`'s running daemon, runs `B`'s own `daemon install`,
enables systemd lingering on Linux and waits for the daemon to answer.
`--dry-run` reports the remote, platform, build, path and command and writes
nothing. `daemon status --remote B` returns `B`'s status object plus
`remote`; `daemon uninstall --remote B` removes `B`'s service and leaves
`~/.local/bin/izmachine` in place.

Only a typed `--remote` moves a daemon verb: a configured default remote or
`IZMACHINE_REMOTE` never does. Exit 2 (`usage`) refuses `--remote` naming
this host, an Incus remote, a selector or an unknown name; two `--remote`
flags; `--socket` with `--remote`; and `--binary` without `--remote`.

`remote subscribe incus`, `unsubscribe incus`, and `sync` follow changing
private Incus instance-serving endpoints. Explicit `incus:<alias>` registrations
monitor only those targets. Public image sources are excluded; derived IDs
remain stable across URL changes. Source failures retain stale membership;
manual/directory entries win. `incus/local` is an endpoint label, not proof of
physical locality. Host/member evidence determines the guest's displayed remote.

## Docker context remotes

`remote add build docker:echo` registers a context the docker client already
knows; an unknown name exits 2 with a `docker context ls` hint, and a host
with no docker client registers the name and says so. `docker:<context>` runs
this host's docker CLI with `--context <name>` and `DOCKER_HOST` cleared;
`ssh:<host>` runs izmachine on the host. Use `docker:` when a context exists
and no izmachine runs on the far side. `list --remote build`, `list build:`,
`'build:web-*'` and `direct start build:web` address it.

`remote subscribe docker` / `unsubscribe docker` follow `docker context ls`
as `subscribe incus` follows `incus remote list`: one entry per context,
source `subscription:docker`. A context named `local` or one colliding with an
explicit entry registers as `docker/<context>`. A discovery failure keeps the
cached entries with `stale: true`; removing a context drops only its
subscription-owned entry; unsubscribing keeps explicit registrations.
`DOCKER_CONFIG` selects the client configuration.

Registered contexts add to the client's current context in a listing; two
contexts reaching one engine list a container once. `--local` means this
host's sockets: a `unix://` or `npipe://` endpoint is this host, `ssh://` and
`tcp://` are not, even over loopback. `list --local` asks every context on
this host's socket and never reaches another engine; verbs under `--local`
act through the current context when it is on this host, else the first
context that is, and refuse when none is. A guest on an unregistered
other-engine current context appears under the context's name, never
`local`.

`remote show <name>` prints `endpoint`, `engine_id`, `engine_name` and
`version`, under `remote.identity` in JSON. A `.iz` pins the engine with
`remote = "docker:id=<engine-id>"`: it binds whichever context reaches that
engine for `up`, `down`, `provision`, `ssh`, `info`, `destroy` and
`validate`, reporting `resolution.remote.identity` with `pin: {engine_id}`,
`alias` and `aliases`; no match exits 14 with a `docker context create` hint,
and unprobed contexts appear in `identity.unprobed`. Engine evidence is
shared across requests for five minutes.

JSON: a guest listed through a named context carries `context`; on another
engine `identity.kind` is `context-container-id` with value `<context>:<id>`.
A failing context reports under `unreachable` with transport `docker` and as
`docker/<remote>` in `snapshots.local.observations`; the other remotes stay
listed.

`remote migrate --dry-run <paths>...` previews selected legacy files;
`remote migrate <paths>...` applies backup and atomic replacement. Conflicting
declarations/backups are refused. Keep the canonical remote vocabulary in new
commands/files; native third-party protocol keys and migration fixtures are
exceptions.

## Remote selector expressions

Use selectors unquoted in bash and zsh:

```sh
izmachine remote find --kind vm --remote not:local --format json
izmachine list --remote @offbox --format json
izmachine list --remote class=east,arch=arm64 --format json
```

| Form | Meaning |
| --- | --- |
| `D71` | Exactly the registered remote D71 |
| `@offbox` | A saved selector expression |
| `class=east,arch=arm64` | Every distinct key must match |
| `class=east,class=west` | A repeated key accepts either value |
| `not:local` | Exclude this computer and Incus aliases proven to run here |
| `not:class=east` | Exclude the east tag |
| `not:@east` | Complement of the saved east group |
| `name=local` | Exactly the registered local remote |

Keys are `name`, `class`, `arch`, `kind`, and `transport`. `transport=local`
means this computer, including proven-local Incus aliases. A term beside
`@saved` narrows that group. Selectors contain no whitespace, shell operators,
globs, parentheses, redirections, tilde or dollar expansion; they never start
with a dash or equals sign. Guest globs remain quoted positional selectors.

Repeated `--remote` flags form a union. A direct action requires exactly one
matching guest; two matches exit 2 naming both remotes, and no match exits 10
with an inventory hint. Creation ranks matching compatible hosts by load,
reports the chosen remote and fails there without trying another host. Supply
Incus's required `--image` when creating an Incus guest. An implicit provider
follows an Incus destination or a compatible provider advertised by an SSH host.
An explicit provider restricts compatibility; local defaults follow kind and OS.

`remote find --format json` reports `selector: {raw, expanded}` and keeps
excluded candidates with their failing term in `why`. A selector matching no
remote exits 14 with the candidate trace. Bare remote names remain exact.

Saved expressions live in `[selectors]` in directory, project and user config:

```toml
[selectors]
offbox = "not:local"
east = "class=east"
east-arm = "@east,arch=arm64"
```

The merge order is directory, project, user; the later layer owns a repeated
name. Directory-owned expressions are locked to directory-owned references.
`remote list --format json` lists selectors with name, source, locked, selector,
expanded and any error. Invalid references are refused on use and listed with
their error; unrelated commands keep working. Cycles name the reference chain.
Malformed tables, non-string values and names containing whitespace or `@`
exit 2 naming the file and key. `IZMACHINE_REMOTE` and the config `remote`
setting accept the same expressions and retain their reported origin.
