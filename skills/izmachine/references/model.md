# The guest model and the human surface

Read this when a payload confuses you, when you render output for a person, or when a setting comes from somewhere you did not expect.

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

## Four words, four questions

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

## `-c/--columns` — for when a human wants to look

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

## Colour

`--color=auto|always|never` (also `--no-color`, `IZMACHINE_COLOR`, and the
`color` config key). **You never need it.** `--format json` is never coloured at
all, and every other output is plain the moment stdout is not a terminal, so a
piped or captured run already gives you clean bytes. Nothing is ever carried by
colour alone: every state, readiness mark, lock tier and error severity is also a
word, a prefix or a mark, so stripping escapes loses no information. `NO_COLOR`,
`--color=never` and `TERM=dumb` all produce byte-identical output to the piped
case.

## Configuration precedence

Every global is resolved through one chain, highest first: **flag > `IZMACHINE_*`
env var > `./izmachine.toml` > `$XDG_CONFIG_HOME/izmachine/config.toml` (falling
back to `~/.config/izmachine/`) > built-in default.** A file is TOML whose
top-level keys are spelled exactly as the flags are (`provider`, `remote`,
`format`, `os`, `color`, `local`, `quiet`, `verbose`, `yes`), plus the
`[[remotes]]` array of tables. `izmachine config show --origin` names the layer
that answered for each value; `izmachine config path` names every file consulted
and whether it exists, in four layers: `global` (`global.iz`, machine
definitions — see [iz-files.md](iz-files.md)), `project` (`izmachine.toml`),
`user` (`config.toml`) and `directory`. **No config file is not an error.**

Set `IZMACHINE_PROVIDER` rather than repeating `--provider` on every call when a
whole session targets one provider — the flag still wins wherever you need it to.
