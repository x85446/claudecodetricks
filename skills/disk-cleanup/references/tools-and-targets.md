# Tools and targets

Use installed tools and their local help. Select only the checks relevant to the requested machine and observed large directories. A missing tool does not require installation; `df` and `du` provide a useful fallback.

## Measurement

| Purpose | Command or approach |
|---|---|
| Filesystem capacity | `df -k "$HOME"` and `df -k /` |
| macOS writable volume | `df -k /System/Volumes/Data` when that path exists |
| Directory summary | `du -x -k -d 1 "$HOME"` on macOS or GNU systems; recurse only into useful candidates |
| Interactive inspection | `ncdu -x -rr "$HOME"`; verify installed help supports `-rr`, which disables deletion and shell spawning |
| Noninteractive inspection | `ncdu -x -o "$scan_file" "$HOME"`; choose a unique export file outside the measured tree and a destination with enough space |
| Import inspection | `ncdu -rr -f "$scan_file"` in an interactive terminal |
| Active log handles | `lsof +D "$target"` for a bounded directory; recursive inspection can be expensive |
| Unlinked open files | `lsof +L1`; scope results to the affected mount and account |
| macOS snapshots | Read-only `tmutil listlocalsnapshots /` or `diskutil apfs listSnapshots <volume>` for the relevant volume |

Avoid full-tree exports when the disk is critically full; narrow `du` scans need less temporary storage. Scan exports expose filenames and must stay local unless the user asks to share them. Do not hide scan errors. An ncdu view can delete files unless read-only mode is enabled; do not use it as an implicit deletion approval.

`du` usually measures allocated blocks, while file sizes can include sparse ranges. APFS clones and snapshots complicate attribution. Cloud placeholders can have large logical sizes with little local allocation; avoid opening or hashing them merely to rank cleanup targets.

## Ordinary candidates

Inspect before acting. Tool-owned caches can be regenerable while adjacent configuration and application state are not.

| Candidate | Inspect | Cleanup and consequence |
|---|---|---|
| Machine-specific disposable data | Applicable entry in [machine targets](machine-targets.md) | Follow the exact standing permission and retention rule. |
| Homebrew downloads and retained versions | `brew --cache`, measure that path, and inspect `brew cleanup -n` | Use a scoped `brew cleanup` when its preview matches the authorized targets. Cleanup can remove retained versions used for rollback. |
| Package caches | Measure pip, npm, uv, Cargo, Go, or other caches that actually exist | Prefer the package manager's cache command and installed help. Distinguish caches from virtual environments, installed toolchains, and source checkouts. Expect downloads or rebuilds. |
| Xcode build data | Measure `~/Library/Developer/Xcode/DerivedData` and check active builds | Remove selected disposable derived data when authorized. Preserve Archives, signing assets, device data, and simulator devices unless specifically approved. |
| Repository build outputs | Inspect repository status, documented clean targets, and active builds | Read a clean target before invoking it. Preserve dirty work, ignored user data, installed daemons, and artifacts needed by running services. |
| Docker or other container storage | `docker system df -v` and inspect selected resource ownership | Clean only identified disposable images or build cache within authorization. Volumes, stopped containers, and shared resources need explicit scope; avoid blanket `system prune` or volume pruning. |
| Application caches and crash reports | Measure selected cache/log directories and identify their owner | Use application controls or narrow known-disposable files; skip active writers. Preserve application support, mail stores, browser profiles, and databases. |
| Trash, Downloads, installers, archives | Inventory sizes and recognizable candidates without opening private contents | Present exact candidates for user selection. Trash can contain recoverable personal files and is not automatically disposable. |
| VM disks and snapshots | Inspect provider-reported allocation and ownership when requested | Present candidates; deletion, compaction, or snapshot removal needs explicit authorization. Never infer disposability from a guest being stopped. |
| Backups and OS snapshots | Read-only provider inventory and retention settings | Explain their allocated space and recovery purpose. Use the owning tool only after explicit approval; do not delete snapshot storage manually. |

Do not scan mail contents, financial records, or credentials to decide what to delete. Do not use generic duplicate deletion; matching hashes do not establish which copy the user needs.

## Prevent recurring growth

- For large terminal logs, propose rotation with a size cap, retention period, and handling for active writers. Discover the actual logging application before recommending its settings.
- For build caches, balance a size or age budget against rebuild time. Check whether the tool supports native eviction before proposing a custom job.
- For container images and toolchains, identify the versions projects still require before proposing removal.
- For large cold files, confirm a destination, verify the copy or backup, and obtain deletion permission before reclaiming the source.
- For cloud-managed files, use the provider's supported local-eviction action when requested. Filesystem deletion can propagate to the cloud and other devices.
