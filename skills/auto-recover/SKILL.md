---
name: auto-recover
description: Restore a malformed BUG-NNNNNN.json from git history and notify the breaking leaf.
when_to_use: When jq parsing or schema validation fails on any BUG-NNNNNN.json during chopper2 cycle step 4.
---

# auto-recover

Algorithm (§16):
1. `agent-comms-git::history_walk_for(<bug>)` → newest-first list of `(sha, content)`.
2. Walk back until `validate_bug(content)` passes.
3. `git checkout <sha> -- <path>` to restore.
4. Commit: `chopper2 auto-recovery: BUG-NNNNNN reverted to <sha>`.
5. `git blame` the broken commit → identify the breaking leaf author.
6. Drop `AUTO_RECOVERY-<id>.json` in that leaf's `from-chopper/`.
7. Append an `auto_recovered` event to the bug.
8. Call `notify(auto_recovery_triggered, …)`.

## Invocation

```
auto-recover --bug-path <path> --repo-root <path>
```
