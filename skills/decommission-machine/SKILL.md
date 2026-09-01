---
name: decommission-machine
description: |
  Removes an Incus container that is no longer in infra/machines.yml.
  Sequence: notify(host_decommissioned) FIRST → revoke deploy keys for
  agents that lived on the host (delegated to reconcile-identities) →
  `incus delete --force` → delete `infra/host-state/<host>.json`. The
  notification fires BEFORE any destructive operation so observers know
  the host was about to die. AC18 / S18 coverage.
when_to_use: |
  Invoked by reconcile-machines when a running container has no entry in
  machines.yml. Idempotent — safe to retry. Honors DRY_RUN=1 (logs but
  performs no deletions).
---

# decommission-machine

Hard-coded refusal to decommission `chopper2-host` — the bootstrap surface
is excluded from machine reconciliation entirely (architectural invariant).

The skill is a sequencer. It does NOT enumerate which agents lived on the
host (that is identity reconciliation's job); it ONLY records the host id
in a queue file so `reconcile-identities` can revoke their keys on the
same cycle.
