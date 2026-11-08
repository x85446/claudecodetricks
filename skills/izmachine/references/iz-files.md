# `.iz` files and `up`

Read this before authoring or editing a project's `./.iz`. The placement workflow in SKILL.md says when to write one; this says what goes in it.

## `up` — create and provision from a .iz file

`create` gives you a bare guest. `izmachine up` takes a **.iz file** — a
description of a machine, not a script — and goes from nothing to provisioned in
one command. The file is **TOML plus five assignment operators** (the Papyrus
layered-definition format, see [Layers](#layers)). A **project machine** is
`[machine.<key>]` in `./.iz` in the current directory, or in the file the
global `--izfile <file>` (or the command's own `-f/--file`, the same selection)
names instead. A named machine (`izmachine up <key>`) resolves across the
project and global layers; a bare verb (`izmachine up`, `down`, `info`,
`validate`) acts on the project machines when a project file exists, else on
the global ones. Inputs (`[[file]]` payloads, provision files) resolve relative
to the file that defines the field. `validate --format json` and `info --format
json` carry each machine's `source` (`{layer, path}`) and per-field `origins`:
each names the winning assignment's `layer`, `path`, `origin`, `line`, `column`
and `operator`; its `shadows` is a list, lowest first, of every lower
assignment the winner displaced, each `{layer, path, line, column, operator,
value}`; its `ignored` lists the assignments that never took effect — a `?=`
that found the key set, or a personal value a `:=` kept out. Papyrus computes
the merge and this provenance; `definition show --format json` returns the
same origins for the whole stack (see [Viewing and editing the stack](#viewing-and-editing-the-stack));
`izmachine config path --format json` lists `global.iz` under the `global`
layer. A named file such as `backup.iz` is **never** a fallback: with no
project file and no global file, every file-scoped command exits 14 naming
`izmachine init` and `--izfile`, and lists the named files it did not read.
Two different paths (`--izfile a.iz -f b.iz`) are exit 2; the same path twice
is fine. There is no parent-directory search.

## Layers

A definition is assembled from four layers, applied in this order:

| order | layer | file | supplies |
|---|---|---|---|
| 1 | envelope defaults | `global.iz` `[defaults]` | `provider`, `remote`, `needs`, `resources` — a machine can override each |
| 2 | machine definition | `[machine.<key>]` in the project file, else `global.iz` | everything |
| 3 | personal contents | `global.iz` `[personal]` | `packages`, `user`, `file`, `provision` — merged after the machine, before local |
| 4 | local overrides | `.izlocal` beside the project file | any field, private to this checkout |

`global.iz` is `$XDG_CONFIG_HOME/izmachine/global.iz` (falling back to
`~/.config/izmachine/global.iz`). Any other key under `[defaults]` or
`[personal]` is an error. **A project definition replaces a global definition
with the same key wholesale, before the field merge** — never key by key. A
project `:=` value excludes the corresponding personal value; `.izlocal` can
still override it. A project `dependency.up` must name a project-declared key.
`izmachine init` adds `.izlocal` to `.gitignore` once; `validate` warns when
Git tracks it. A machine defined only in `.izlocal` needs explicit selection
when a project file exists.

```iz
# global.iz
[defaults]
provider = "docker"

[defaults.resources]
cpus = 2

[personal]
packages = ["git", "curl"]
```

```toml
# .iz — complete on its own
[machine.dev]
provider = "docker"
kind = "container"
image = "debian:bookworm"
packages = ["git", "curl"]
```

```iz
# .izlocal — private adjustments
[machine.dev]
packages -= ["curl"]
labels := { owner = "developer" }

[machine.dev.resources]
cpus ?= 4
```

Here `packages` becomes `["git"]`, and `cpus` stays 2 because `?=` fills only
an absent value.

**`--bare` reads only the selected project file.** `up --bare`,
`validate --bare` and `info --bare` skip `global.iz` (defaults and personal
contents) and `.izlocal`. Use it to prove the project file reproduces the
intended definitions on a machine with no global layer — the reproducibility
check before you call a `.iz` done.

### Viewing and editing the stack

`izmachine definition show [machine]` renders the stack the way the merge saw
it. Papyrus records, for every field, the layer that won and every assignment
it displaced; the three views are that record. Layer names are `default`,
`library:<dir>` (one per `IZMACHINE_FUNCTION_PATH` directory that supplies
functions), `global`, `personal`, `project` and `local`.

| view | prints |
|---|---|
| `--view merged` (default) | the merged definition as plain Papyrus, ready to pipe into a file; it re-parses to the tree `validate` compiles |
| `--view blame` | each assignment behind a gutter of layer and `file:line`, then `# was <value> <file>:<line> <op>` for every displaced lower assignment and `# ignored <op> <value> <file>:<line>` for one that never took effect; the text right of the gutter is still Papyrus |
| `--view layers` | one row per field and one column per layer, lowest first, `★` on the winner and the operator beside a `?=` or `:=` value; below 110 columns the table shows the winner and its shadows and a note names `--format json` |

`--layer <name>` prints one layer as parsed, operators included, without
merging. Without a machine every machine the stack defines is printed. An
unknown machine exits 14 naming the keys the stack defines; an unknown
`--view` or `--layer` exits 2. `--format json` returns `machines[]` with
`fields`, `layers` (`{name, path}` lowest first), per-field `origins` with the
`shadows` chain and `ignored` list, `functions` (attached library functions
with their own `origins`) and `detached_functions`. Definition files hold no
secret values and the command never reads the fact store.

For a project file whose `[machine.k3s-golden]` sets `provider`, `kind`, `os`
and `image = "alpine:3.19"` on lines 2–5, with an `.izlocal` beside it setting
`image = "alpine:3.20"`, `definition show k3s-golden --view blame` prints:

```text
project  .iz:2       provider = "docker"
project  .iz:3       kind = "container"
project  .iz:4       os = "alpine"
local    .izlocal:2  image = "alpine:3.20"  # was "alpine:3.19" .iz:5 =
```


`izmachine definition edit <machine> [field]` changes exactly one layer.
`--set <field>=<value>` writes the value into the winning layer's file through
the Papyrus writer, which keeps every comment, blank line, key order and
foreign table byte for byte; it repeats, and the edits apply atomically. The
value is Papyrus: a quoted string, a number, `true`, a `[list]` or an
`{ inline = "table" }`. `--layer local` sends a `--set` into `.izlocal` as an
override instead. A field a library owns is read-only here; `--layer local`
writes the override under the machine's own `functions.<name>`. `--format
json` returns `edits[]` as `{field, value, file, layer, line}`. A value that
does not parse, an unknown machine, a library-owned field without `--layer
local`, or an unset `$EDITOR` without `--set` exits 14 and changes nothing;
`--set` without `=` exits 2. With a field and no `--set`, `$EDITOR` opens the
file of the layer that won it at that line; without a field it opens
`.izlocal` at `[machine.<key>]`, creating the file and the header when absent.
Editor mode is for humans; an agent passes `--set`.

```sh
izmachine definition show dev --view layers
izmachine definition show --layer local
izmachine definition edit dev --set 'image="alpine:3.20"'
izmachine definition edit dev --set 'packages=["git"]' --layer local
```

### Operators

The set is exactly `=`, `+=`, `-=`, `:=`, `?=`; `=~` and `~=` are invalid.
Operators go in key assignments, never table headers or inline-table entries.
Put whitespace before `-=`: `packages -= ["curl"]` removes a package,
`packages-= ["curl"]` assigns a key named `packages-`.

| value shape | `=` / `+=` | `-=` | `:=` | `?=` |
|---|---|---|---|---|
| scalar | `=` replaces; `+=` is an error | error | replace | set only if absent |
| scalar list | stable union, lower-layer order kept | remove matching values | replace the whole list | set only if absent |
| table | merge keys recursively | remove named keys | replace the whole table | set only if absent |
| record array | merge records by identity; append unmatched | remove matching identities | replace the whole array | set only if absent |

Record identities: `user.name`, `file.path`, `ports.guest` (also `port`),
`provision.id`, `mount.target`, `tuning.systemd.name`. A provision step without
an `id` matches only an identical record — give steps you expect a layer to
touch an `id`. Removing an unknown identity produces a note, and `+=` with no
lower value produces a positioned note; read `validate`'s notes.

### References

`{{env.NAME}}` reads an environment variable after all layers merge;
`{{env.NAME | "value"}}` supplies a default. An unset variable with no default
is exit 14 with the source path, line and column. Only the `env` scope
resolves; other scopes stay literal with a note. Variables only — no sections,
loops, partials or filters. Escape a literal with a backslash, `\{{` inside a
TOML basic string.

```iz
[machine.referenced]
provider = "docker"
kind = "container"
image = '{{env.IZMACHINE_DOC_IMAGE | "debian:bookworm"}}'
```

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

For a one-off guest name, edit a copy of the file and set `name = "<guest>"`
in the section. Select that copy with `--izfile`. For maintained project
machines, edit the project's `./.iz` in place and validate the result.

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

## The file, and the one key that is required

The file is TOML with the operators above — a shared envelope in which
izmachine owns the `[machine]` section and preserves every other top-level
table untouched. A
`[machine.<name>]` section holds:

| key | required? | what it is |
|---|---|---|
| `provider` | **REQUIRED** | which provider runs it: `lume`, `lima`, `utm`, `container`, `docker`, `incus`, or a plugin's name |
| `name` | optional | the guest's real name; defaults to the section key |
| `remote` | optional | a remote from `izmachine remote list`, or `<remote>:<guest>` naming the guest too; mutually exclusive with `needs` |
| `needs` | optional | placement constraints: `kind`, `arch`, `class`; izmachine ranks compatible remotes by load; mutually exclusive with `remote` |
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

## Lifecycle, functions and facts

Read this when a definition carries a `lifecycle`, `functions` or `facts`
table, when `up` reuses a guest you expected rebuilt, or before `call`, `fact`
or `function`. The full contract is `docs/iz.md` in the izmachine repo.

### Lifecycle — what `up` and `down` do to a guest that already exists

`[machine.<key>.lifecycle]` (defaults from `[defaults.lifecycle]` in
`global.iz`, each key overridable per layer):

| key | values | default |
|---|---|---|
| `on_up` | `reuse` (start what exists), `recreate` (destroy and build from the definition), `restore` (stop, restore `snapshot`, start without provisioning) | `reuse` |
| `on_down` | `stop`, `destroy`, `snapshot` (snapshot then stop) | `stop` |
| `snapshot` | the baseline tag; required by `restore` and `on_down = "snapshot"` | unset |
| `on_drift` | `warn`, `recreate` — when the guest's recorded digest differs from the definition's | `warn` |
| `ttl` | a Go duration (`8h`); ages from the recorded creation time | unset |

First creation captures the baseline after provisioning; an existing guest
with no baseline is captured before its first restore. The digest covers
executable artifacts and the envelope — labels, lifecycle, facts and functions
do not change it. A provider with no snapshot or restore verb reports a note
and reuses. A lock refuses recreate and restore with exit 11. `validate
--format json` shows the effective policy and each field's origin;
`up --dry-run` reports `lifecycle: {action, rule, source}`.

Command-line overrides: `up --recreate | --restore[=tag] | --reuse`
(mutually exclusive, exit 2) and `--snapshot=tag`; `down --destroy |
--snapshot=tag`.

### Functions — reusable commands attached to machines

`[function.<name>]` declares one, with exactly one body: `run` (a command),
`script` (a file relative to the definition), `exec` (a host executable), or a
pinned directory `from`. `where = "guest" | "host"` (default guest; a host
function runs with the same trust as a Makefile). `on` lists triggers:
`demand`, `up` (after provisioning), `down` (before stopping), `provision`,
`every <duration>` (runs on the inventory daemon, registered by `up`, for the
daemon's lifetime). `match = { os, kind, provider, labels }` is eligibility
only — **matching never attaches**. A machine attaches with
`functions = ["name"]`; `attach = "auto"` attaches to every eligible machine
and `functions -= ["name"]` detaches one. `[machine.<key>.functions.<name>]`
declares a machine-local function or narrows a library one's triggers.
Libraries merge from `IZMACHINE_FUNCTION_PATH` directories, `global.iz`, the
project file and `.izlocal`; `--bare` excludes the external layers. Dry runs
execute no function.

`call <machine> <function> [-- args]` returns stdout verbatim and the
function's exit code. Host executables receive `IZMACHINE_NAME` and the canned
facts as `IZMACHINE_FACT_<NAME>` (dots become underscores). `{{fact.NAME}}`
resolves inside a body at run time.

### Facts — cached values keyed by guest identity

`[machine.<key>.facts]` holds static values, or tables with `from = "<function>"`,
`ttl`, `secret = true`, `publish`. A function's own `fact = { name, ttl,
secret }` declares its cached output. Canned names, never shadowable:
`name`, `identity`, `provider`, `remote`, `kind`, `os`, `osversion`, `arch`,
`ip`, `state`, `uptime`, `ssh.user`, `ssh.port`, `ssh.key`, `created_at`,
`digest`, `snapshot` — each carries `observed_at`, empty when the provider does
not report it. A TTL is lazy: the next read refreshes; a failed producer keeps
the cached value, warns and exits 0, or exits 14 with nothing cached. An
operator value (`fact set`) never expires. A replaced guest under the same name
has its own cache (Incus keys include the instance UUID); `fact gc` drops the
orphans.

Secrets: AES-256-GCM, master key in the OS keychain or
`~/.izmachine/master.key` (0600; `IZMACHINE_MASTER_KEY_FILE` overrides,
`IZMACHINE_FACTS_DIR` moves the cache). JSON masks them as `<secret>` unless
`--reveal`; `fact get --file` writes 0600; `secret rotate` re-encrypts and
resumes after an interruption. **Never put `--reveal` output in a transcript.**
