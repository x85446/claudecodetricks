---
name: reconcile-machines
description: |
  Top-level machine-reconciliation driver for the ai-operator agent. Reads
  infra/machines.yml + config.yml::incus, diffs against the running Incus
  fleet, and dispatches provision-machine / reconfigure-machine /
  health-check-machines / decommission-machine for each delta. chopper2-host
  is excluded — it is the bootstrap surface and never reprovisioned by code.
when_to_use: |
  Invoke once per ai-operator cycle (default every_minutes=15) AFTER
  `git pull --rebase` and BEFORE `reconcile-identities`. The skill is
  idempotent; running it on a steady-state cluster is a no-op. Provides
  AC17, AC18, AC21, AC125 coverage. Set DRY_RUN=1 to plan without applying.
---

# reconcile-machines

Drives the §22 machine-reconciliation phase. Inputs come exclusively from
the on-disk `infra/machines.yml` (declared desired state) and
`incus list --project agent-comms` (observed actual state). The skill is
single-pass: build the delta, dispatch sub-skills, return.

## Outputs

- `infra/host-state/<host>.json` updated for every reachable declared host
- `notify(host_decommissioned)` for every container removed from machines.yml
- `notify(incus_unreachable)` if the Incus daemon itself is unreachable;
  no other side effects in that case (AC125)

## Failure modes

- `incus list` exits non-zero → notify(incus_unreachable); exit 0; do not
  cascade failures across hosts.
- Per-host action failures (provision/reconfigure/decommission) are logged
  and recorded in `infra/host-state/<host>.json::status` but do NOT abort
  the loop. Other hosts continue.

## Test coverage

- `tests/happy_path.rs` — declared==actual → no-op
- `tests/missing_host.rs` — missing host → provision invoked
- `tests/stray_container.rs` — undeclared container → decommission invoked
- `tests/incus_unreachable.rs` — AC125 (no fan-out failures)
