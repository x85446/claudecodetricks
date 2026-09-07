---
name: infra-access
description: How to log into Izuma/Datafoundry infrastructure — SSH aliases, jump hosts, and per-device quirks for MAAS, Dell/Huawei/Cisco switches, BMCs/iDRACs, pfSense, and jump hosts. Use whenever a task needs you to reach a piece of infrastructure and you're working out how to connect, or when an SSH/login attempt fails. Triggers on "how do I access / log into / SSH into / get into <device>", "connect to maas / the switch / the dc", or any access-denied while reaching infra.
---

# infra-access — how to get into our infrastructure

When a task requires reaching a device, look it up here **before** guessing at SSH config.
Aliases below resolve via `~/.ssh/config.d/*` (mostly `df-austin`). Match on the user's
casual name — the **Aliases** column lists what they tend to call each thing.

## Access table

| Thing | Aliases the user uses | How to connect | Notes |
|---|---|---|---|
| **MAAS** (Austin / Datafoundry) | maas, maas machine, df, datafoundry, datacenter, dc, "10.7 network", "10.7.160 network" | `ssh maas-austin-maas` | Incus container `10.7.160.9`, user `travis`. Snap MAAS 3.4. For the `maas` CLI: `KEY=$(sudo maas apikey --username=maasadmin); maas login adm http://localhost:5240/MAAS/ "$KEY"`. BMCs it manages are on `10.7.158.x` (routed via pfSense `10.7.160.1`). |
| **Dell PowerSwitch SW3** | dells, SW3, ".25", "fast switch" | `ssh SW3` (OS10 CLI) or `ssh SW3-linux` (OS10 Linux shell) | `USTXAR1-SW3`, `10.7.158.25`, user `admin`/`admin`. Standalone (VLT gone), local LACP bonds. See also the `sw3-portpair` skill for port config. |
| **Dell PowerSwitch SW4** | dells, SW4, ".26" | `ssh SW4` (OS10 CLI) / `ssh SW4-linux` — user `admin`/`admin` | `USTXAR1-SW4`, `10.7.158.26`: **ALIVE — it was NEVER a dead ASIC** (the old "DEAD/hardware-failure" note was wrong). It was stuck because `admin` couldn't authenticate to OS10's local NETCONF service (:830) — its `/config/home/admin/.ssh/authorized_keys` was invalid, so clish hung at `System is loading` / `% Error: System is not ready` and refused config mode. **Fixed by Ed 2026-06-03** (wiped `startup.xml` + Redis `dump.rdb` → reboot, repaired admin's local NETCONF key, hostname marker `S5148F-PERSIST`, `write memory`, verified on OS10-A). See [[os10-cli-system-not-ready]]. |
| **Huawei CloudEngine (Austin)** | CE1, CE2, huawei | `ssh CE1` (10.7.158.27) / `ssh CE2` (10.7.158.28), user `izadmin` | For scripted access use a **single** attempt with `-o IdentitiesOnly=yes -i ~/.ssh/to_huawei_switch.pub` — offering all agent keys trips the VRP login-failure lockout (blocks the source IP = pfSense `10.7.158.1`). CE1/CE2 iStack being decommissioned. |
| **Huawei CloudEngine (remote site)** | CE3, CE4, CE5 | `ssh CE3` / `CE4` / `CE5` (38.97.2.12–14), user `izadmin` | Same VRP single-attempt caution. |
| **Cisco Catalyst 3650 (rack core)** | cisco, rack core, 3650 | `ssh cisco` (`10.7.158.2`), user `admin`, password auth | Legacy KEX/ciphers configured in `df-austin`. pfSense, CE1, office switch, cogent uplink all land here. |
| **Cisco 2960X office switch** | USTXA-OFFICE01, office switch | `ssh -i ~/.ssh/to_ustxa-office1 -l izadmin -o PubkeyAcceptedAlgorithms=+ssh-rsa -o HostKeyAlgorithms=+ssh-rsa -o KexAlgorithms=+diffie-hellman-group14-sha1 -J pfsense-travis-int 10.7.158.4` | Legacy mgmt also at `38.97.2.3`. |
| **pfSense** | pfsense, firewall, gateway | `ssh pfsense-travis-int` | The jump host for almost everything in `10.7.158.x` / `10.7.160.x`. |
| **C6525 iDRAC (per-node BMC)** | idrac, c6525, bmc | `ssh -fNL 8443:10.7.158.<host#>:443 pfsense-travis-int` then browse `https://localhost:8443` (e.g. D71 → `10.7.158.71`) | Per-node iDRACs are at `10.7.158.<host#>` (D71 = `10.7.158.71`, confirmed; matches the host-number scheme). The old chassis BMC `10.7.158.155` is **stale/retired** — don't use it. Confirm exact IPs via MAAS power-parameters; racadm helpers in `tools/racadm/`. |

## SSH / key model (read this when you get "Permission denied")

The user relies on **KeePassXC as the ssh-agent** holding most private keys; `~/.ssh/*`
holds public keys and *sometimes* privates.

- **`Permission denied (publickey)` almost always means KeePassXC isn't running or its
  database is locked** — not a broken SSH config. Say so and ask the user to open/unlock
  KeePassXC, rather than rewriting config or hunting for key files.
- `~/.ssh/config.d/*` files reference some IdentityFiles as `.pub` paths; in batch/non-
  interactive mode that errors with "invalid format". When scripting a connection, override
  with `-o IdentitiesOnly=no -o IdentityFile=/dev/null` and let the agent supply the key,
  or point `-i` at the actual private key.
- Check the agent with `ssh-add -l`. Empty/low-count list ⇒ KeePassXC not feeding the agent.
- For VRP (Huawei) gear, **never** let the agent offer all keys — one wrong key trips a
  source-IP lockout. Always `-o IdentitiesOnly=yes -i <specific key>`, single attempt.

## MAAS-deployed hosts (the OS, not the iDRAC)

A freshly MAAS-deployed node (e.g. D72 = `10.7.161.157` on the `10.7.160.0/23` provisioning
net) is reached as user **`ubuntu`** with the **MAAS deploy key** — *not* `travis`/`to_generic`
like the H9x cluster. MAAS pushes exactly one authorized key: comment **`df-maas-main`**,
which locally is **`~/.ssh/to_maastarget.pub`** (private key held in the KeePassXC agent).
So the config entry is `user ubuntu` + `identityfile ~/.ssh/to_maastarget.pub` +
`identitiesonly yes`. Confirm which key MAAS deploys with `maas adm sshkeys read`.
The matching iDRAC is the `<host>-ILO` entry (different IP, `10.7.158.x`, key `~/.ssh/to_idrac`).
The D/P `<host>-ILO` aliases (D71-D74, P61-P64) are fully configured — `to_idrac` key,
`+ssh-rsa` algos, and `proxyjump pfsense-travis-int` (the BMCs sit on `10.7.158.x`, only
reachable through pfSense). `ssh D71-ILO` lands in the iDRAC's racadm shell; pass a racadm
subcommand (e.g. `ssh D71-ILO racadm getsysinfo`), not arbitrary shell — a bare command
returns "ERROR: Invalid command specified." Confirmed working on D71 (2026-06-01).

## Maintaining this

This is the single source of truth for infra access. CLAUDE.md's "Logins" section should
point here rather than duplicate. When you learn a new device/alias/quirk, add a row.
