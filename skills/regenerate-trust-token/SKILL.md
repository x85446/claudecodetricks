---
name: regenerate-trust-token
description: |
  Re-issues an Incus trust token for a host that has lost or expired its
  `host:` remote registration. Issues a fresh token via
  `incus config trust add --restricted --projects agent-comms`, ships it to
  the affected container via `incus exec`, runs `incus remote add host`
  inside the container, and updates `infra/host-state/<host>.json`. AC124
  guarantees idempotence — running on a healthy host is a no-op (skipped
  when the trust still works).
when_to_use: |
  Invoked by health-check-machines when `incus exec <host> -- true` returns
  an authentication failure, OR by the operator session when manually
  re-keying a host. Honors DRY_RUN=1.
---

# regenerate-trust-token

Idempotent. Issues a scoped trust token (`--restricted --projects
agent-comms`) on every invocation, but only re-runs `incus remote add` on
the container if the existing remote is broken. The new token replaces the
prior one in the host's state file with `last_rotated_at`.
