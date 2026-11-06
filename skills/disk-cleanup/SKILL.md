---
name: disk-cleanup
description: "Audit disk usage, free disk space, clean up disposable files, and recommend ways to limit storage growth. Use for hard disk cleanup, low disk space, large caches, or finding what consumes storage."
---

# Disk cleanup

Find the largest useful cleanup opportunities, execute authorized cleanup, and report measured results. Support audit-only requests and space-conservation advice without deleting files.

## Scope and authorization

- Read [machine targets](references/machine-targets.md) before selecting local cleanup actions. Match its machine and account scope; a path match alone does not establish authorization on another machine.
- An audit, recommendation, or request to create this skill authorizes inspection only. A cleanup request authorizes disposable, regenerable caches and build outputs within the requested scope, plus applicable standing permissions in the machine reference. Honor narrower instructions such as “show me first.”
- For other destructive actions, first measure the specific targets and present their expected benefit and consequences. Ask only for the permission that is missing. Do not ask again for an action the user already authorized.
- Keep personal documents, Downloads, photos, mail, financial data, credentials, source code, uncommitted work, agent transcripts, databases, backups, VM disks, and container volumes unless the user explicitly authorizes that target. Age, size, an ignored Git path, or an “unused” label does not establish disposability.
- Stay on the requested machine and filesystem. Do not mount disks, contact remote hosts, elevate privileges, stop services, install tools, or create scheduled cleanup jobs without authorization for that action.

## Inspect and prioritize

1. Record the host, account, OS, requested mount, and available space with `df -k`. On macOS, distinguish the writable Data volume from the system volume. Avoid double-counting APFS volumes in a shared container.
2. Inspect applicable machine targets first. Use the commands and target guidance in [tools and targets](references/tools-and-targets.md) as needed.
3. Scan in stages: top-level usage, the largest relevant directories, then exact candidates. Use `ncdu` in read-only mode when available; use `du` otherwise. Stay on one filesystem and avoid following symlinks. In a noninteractive session, use an ncdu export or `du` instead of opening a curses UI.
4. Rank candidates by allocated size, recoverability, and disruption. Report inaccessible directories as unmeasured. Do not repeatedly retry permission failures or infer that inaccessible directories are empty.

Keep a compact candidate table: target, measured size, proposed action, and authorization or retention concern. Separate estimates from measured values. Never add a parent directory's size to its children's sizes.

## Execute and verify

- Recheck each exact target immediately before mutation. Resolve its location, confirm it belongs to the selected scope, and reject unexpected symlinks or mounts. Use quoted paths and bounded file selection; avoid broad wildcard deletion.
- Prefer the owning tool's cleanup operation when its scope matches the request. Inspect dry-run output when available. Apply each action separately so a failure does not widen the next action.
- Check for active writers before removing logs or caches used by running applications. Skip files with open handles or uncertain activity unless the user authorizes coordinated rotation or service interruption. Deleting an open file can leave its blocks allocated.
- Preserve each target's root directory and permissions. Do not truncate active logs, wipe application support directories, empty all Trash, or prune all container resources as a generic cleanup step.
- Remeasure target usage and free space on the same filesystem. Report bytes removed separately from the observed free-space delta. APFS clones, snapshots, sparse files, and concurrent writers can make those figures differ.
- If free space does not increase, inspect the discrepancy before deleting more. Check open deleted files, snapshots, and concurrent growth as relevant; explain any unverified cause.

Finish with the actions completed, measured space recovered, current free space, and remaining high-value candidates. State what was skipped and why.

## Conserve space

Base recommendations on measured growth sources. Useful options include log rotation with size and age limits, application log settings, bounded package caches, build artifact retention, removing redundant toolchains, and archiving user-selected cold data to a verified destination. Explain the tradeoff, such as slower rebuilds or reduced diagnostic history.

Separate one-time reclamation from prevention. Recommend concrete settings when verified from the installed tool's help or authoritative documentation. Apply retention settings or automation only when the user requests them.

Maintain machine-specific knowledge in the machine reference when the user supplies a target or retention rule. Record its scope and exact permission. Keep measured sizes in run reports; they are not permanent properties of a target.
