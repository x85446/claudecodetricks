# Incus Routing & Defaults

Memory file the `$incus` skill consults. Edit values here without touching SKILL.md.

## Incus remotes catalog

These are the remotes registered on cypressMini (see `incus remote list`). Most are tls connections via tunneled localhost ports back to remote machines.

| Remote | URL (from cypressMini) | Lives at (host) | Purpose / Notes |
|---|---|---|---|
| `local` | `unix://` | cypressMini itself | local containers/VMs on cypressMini |
| `IncusOS (current)` | `https://localhost:2943` | cypressMini's incus-os layer | usually you don't target this directly |
| `H91` | `https://10.7.112.91:8443` | df-austin H91 host | direct tls (no ssh tunnel) |
| `h94-oidc` | `https://10.7.112.94:8443` | df-austin H94 host | OIDC auth |
| `cruz` | `https://10.7.114.70:8443` | (unknown — confirm before use) | direct tls |
| `dc_austin` | `https://localhost:2911` | tunneled to df-austin | |
| `houston` | `https://localhost:2910` | tunneled to ed-house houston | ed-house gateway |
| `echo` | `https://localhost:2913` | tunneled to ed-house echo | |
| `polaris2` | `https://localhost:2912` | tunneled to ed-house polaris2 | |
| `mercury` | `https://localhost:2921` | tunneled to ed-house mercury | |
| `explorer` | `https://localhost:2930` | tunneled to ed-house explorer | |
| `ranger` | `https://localhost:2931` | tunneled to ed-house ranger | IZUMA bridge available |
| `images` | `https://images.linuxcontainers.org` | upstream | image source — use as `images:almalinux/9` etc. |

Refresh this table with `ssh cypressMini incus remote list` whenever remotes change.

## Remote → SSH config file mapping

When wiring SSH access for a new VM, the Host block goes in the file determined by **where the remote physically lives**, not where you're invoking from.

| Remote(s) | SSH config file | Notes |
|---|---|---|
| `H91`, `h94-oidc`, `dc_austin`, `cruz` | `~/.ssh/config.d/df-austin` | df-austin doesn't need proxyjump |
| `houston`, `echo`, `polaris2`, `mercury`, `explorer`, `ranger` | `~/.ssh/config.d/ed-house` | ed-house typically needs proxyjump through `houston` |
| `local`, `IncusOS (current)` | `~/.ssh/config.d/cypress` | local network — no proxyjump |

Anything fieldstone-related goes in `~/.ssh/config.d/fieldstone`. Bryan-house too.

## Proxyjump rules

| Location | Proxyjump |
|---|---|
| cypress | none |
| fieldstone | none (fieldstone IS the gateway) |
| ed-house | yes — read the file to identify the active gateway (typically `houston`) |
| df-austin | none |

Always read existing entries in the target file to confirm the active proxyjump host before writing a new entry.

## Default specs (when the user doesn't specify)

| Field | Container default | VM default |
|---|---|---|
| cpu | 2 | 4 |
| memory | 4 GiB | 8 GiB |
| disk | 20 GiB | 40 GiB |
| image | `ubuntu/24.04` | `ubuntu/24.04/cloud` |

For specific OS requests:

| Family | Container image | VM image |
|---|---|---|
| AlmaLinux 9 | `images:almalinux/9` | `images:almalinux/9/cloud` |
| AlmaLinux 8 | `images:almalinux/8` | `images:almalinux/8/cloud` |
| Debian 12 | `images:debian/12` | `images:debian/12/cloud` |
| Ubuntu 24.04 (default) | `ubuntu/24.04` | `ubuntu/24.04/cloud` |
| Alpine edge | `images:alpine/edge` | n/a |

## SSH config templates

### Without proxyjump (df-austin, cypress, fieldstone)

```
Host {NAME}
    hostname {IP}
    user travis
    port 22
    identityfile ~/.ssh/to_generic.pub
    stricthostkeychecking no
    userknownhostsfile /dev/null
    identitiesonly yes
    forwardagent yes
```

### With proxyjump (ed-house — usually through houston)

```
Host {NAME}
    hostname {IP}
    user travis
    port 22
    identityfile ~/.ssh/to_generic.pub
    stricthostkeychecking no
    userknownhostsfile /dev/null
    identitiesonly yes
    forwardagent yes
    proxyjump {GATEWAY}
```

If the existing file uses a different identityfile, port, or option set, **match the file's existing pattern** over this generic template.

## File discipline

- Entries sorted alphabetically by `Host`.
- One blank line between Host blocks.
- Don't modify or reorder existing entries.
- If the target file doesn't exist, create it.

## Edge cases

- **Unknown remote**: ASK the user once; record the answer to the project's `.claude/incus.md`.
- **Same VM name appears on two remotes**: ASK which one the operation should target.
- **Remote not in this table**: assume it's tls via cypressMini's tunnels; the user can update routing.md.
- **`incusmagic` not yet remote-aware**: must ssh to the host where the VM lives and run incusmagic there. See SKILL.md "Known issues".
