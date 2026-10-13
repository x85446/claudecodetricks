# `.iz` files and `up`

Read this before authoring or editing a project's `./.iz`. The placement workflow in SKILL.md says when to write one; this says what goes in it.

## `up` — create and provision from a .iz file

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

The file is **TOML** — a shared envelope in which izmachine owns the `[machine]`
section and preserves every other top-level table untouched. A
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
