---
name: health-check-machines
description: |
  Per-host health verification for declared hosts in infra/machines.yml.
  Verifies container reachability via `incus exec <host> -- hostname`,
  cross-checks the returned hostname against the expected id, and writes
  `infra/host-state/<host>.json` + `infra/health/<host>.json`. Unreachable
  hosts emit `notify(incus_unreachable)` and never affect peers (AC125).
  Provides the AC97 hourly health snapshot for the dashboard.
when_to_use: |
  Invoked once per ai-operator cycle (post-machine, pre-identity) for every
  declared host that already has a running container. Idempotent.
---

# health-check-machines

Walks `infra/machines.yml::hosts[]` and runs a single ping per host. Each
host's result is written to two JSON files:

- `infra/host-state/<host>.json` — log of last reconcile state (consumed by
  `provision-machine` next cycle).
- `infra/health/<host>.json` — public, dashboard-readable, with
  `last_health_check` (RFC3339), `status: healthy|unhealthy`, and any
  per-host details. Coverage: AC97 / IT-S42.

Failure of one host does not abort the loop.
