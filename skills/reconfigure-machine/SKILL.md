---
name: reconfigure-machine
description: |
  Applies declared resource/image/network values from infra/machines.yml +
  config.yml::incus to a running Incus container via `incus config set`.
  Non-destructive — never restarts the container; logs every changed key in
  the next ai-operator commit message. No-op if drift is empty.
when_to_use: |
  Invoked by reconcile-machines when a host's observed config differs from
  declared. Honors DRY_RUN=1 (logs intended set calls without applying).
---

# reconfigure-machine

Receives a JSON `Drift` payload (the diff computed by `reconcile-machines`)
on argv and applies one `incus config set <host> <key> <value>` per
non-empty field. The set is non-destructive; resource limits below the
running set will queue but not kill the container.
