---
name: provision-machine
description: |
  Launches a fresh Incus container in the agent-comms project, issues a
  scoped child trust cert (--restricted --projects agent-comms), bootstraps
  it via the appropriate agents/<role>/machine.sh, and writes
  infra/host-state/<host>.json. On three consecutive failures the host is
  marked status: stuck and skipped on subsequent cycles (AC123).
when_to_use: |
  Invoked by reconcile-machines when a host declared in infra/machines.yml
  has no running container in `incus list --project agent-comms`. NEVER call
  for chopper2-host (AC125 invariant). Set DRY_RUN=1 to log the would-be
  Incus calls without launching.
---

# provision-machine

Idempotent, single-host provisioner. The skill never touches deploy-key
identity — `reconcile-identities` runs after machine reconciliation in the
same cycle.

## Sequence

1. `incus launch host:<image> <host_id> --project agent-comms`
2. `incus config trust add --restricted --projects agent-comms` → JWT
3. `incus exec apt-get install -y incus-client`
4. `incus exec incus remote add host <bridge_ip>:8443 --token <JWT>`
5. `incus file push agents/<role>/machine.sh /opt/machine.sh`
6. `incus exec /opt/machine.sh`
7. Write `infra/host-state/<host_id>.json` with `provisioned_at`,
   `image`, resource caps, `status: provisioned|provision_failed|stuck`.

## Failure handling

- Non-zero exit on any step → status: `provision_failed`,
  `notify(host_provision_failed)`, retain `consecutive_failures` counter,
  return Err.
- `consecutive_failures >= 3` → status: `stuck`,
  `notify(host_stuck_provisioning)`, refuse further provision attempts
  until manual reset (operator clears `infra/host-state/<host>.json`).
  Covers AC123.
