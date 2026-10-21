# Remotes, placement recipes and the inventory daemon

Read this when addressing a guest on a remote, narrowing a listing, previewing placement with `remote find`, or when inventory looks stale.

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
remote require `--provider`. Repeated `--remote` values form a union; acting verbs require one matching guest.

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
guest boot. A scope unread for `idle_after` (10m) becomes idle: it leaves the
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
