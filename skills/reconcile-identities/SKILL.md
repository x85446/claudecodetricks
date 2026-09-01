---
name: reconcile-identities
description: |
  Per-agent deploy-key identity reconciler. For every agent in
  unique(machines.yml::hosts[].agents) ∪ {chopper2, ai-operator, operator}:
  ensures an ed25519 keypair on the host that runs the agent, registers the
  pubkey on `gravhl-code-factory` (and on the agent's target repo if a
  leaf) with title=<agent>, and updates the agent's cron.sh
  GIT_SSH_COMMAND. Revokes any GitLab deploy key whose title doesn't
  match a desired agent. Reappearance within 24h → identity_drift_unstable
  (AC126). All mutations append to `infra/identities.json::audit[]`.
when_to_use: |
  Invoked by ai-operator's CLAUDE.md AFTER reconcile-machines, once per
  cycle. Honors DRY_RUN=1 (no GitLab calls, no key writes).
---

# reconcile-identities

External seams (`GitlabClient`, `KeyGenerator`, `HostFs`, `Clock`) are
trait objects so the in-process FakeGitlab + FakeHostFs in tests reproduce
S19 / S20 / AC94 / AC95 / AC126 deterministically.

## Audit trail

Every mutation appends one entry to `infra/identities.json::audit[]`:

```
{ "ts": "<rfc3339>", "agent": "...", "event": "regenerated|registered_on_<repo>|revoked_stray_key|identity_drift_unstable", "from_host": "..." }
```

The 24-hour reappearance check uses an injectable `Clock` so tests can
fast-forward without sleeping.
