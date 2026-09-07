---
name: chopper2-doctor
description: Verify environment and deployment health (§30.9 H1–H42 check matrix).
when_to_use: On-demand by operator; also by validate-skills CI.
---

# chopper2-doctor

Walks the §30.9 H1..H42 check matrix and reports per-item status. Non-zero
exit on any `Err`. Optional `--target-repos` runs branch-protection probes
against `gravhl/df-*`. Used by CI (`validate-artifacts.sh`) and ad-hoc by the
operator.

## Invocation

```
chopper2-doctor --repo-root <path> [--target-repos]
```
