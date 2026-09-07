---
name: simulate-cycle
description: Run the chopper2 or leaf cycle in DRY_RUN=1 mode and print intended ops.
when_to_use: When the operator wants to preview what the next cycle will do without committing or pushing. Asserts zero filesystem mutations. (IT-S31, AC31)
audience: operator
---

# simulate-cycle

Sets `DRY_RUN=1`; invokes the target agent's `cron.sh` (or trunk skills directly) with every skill in `--dry-run`. Prints "would do: <op>" lines. Asserts zero filesystem mutations.
