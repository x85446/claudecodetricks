# Machine targets

This reference contains user-specific cleanup permissions and retention rules. Read it before local cleanup. Verify the machine, account, and canonical path at runtime. Do not transfer a permission to a different host or expand it to neighboring directories.

## cypressMini: travis

Match the macOS LocalHostName `cypressMini`, account `travis`, and home directory `/Users/travis`. If the machine identity differs, treat this entry as a candidate for inspection and ask before relying on its standing deletion permission.

### Terminal session logs

| Field | Rule |
|---|---|
| Target | `/Users/travis/Documents/Log Files` (`~/Documents/Log Files`) |
| Contents | Terminal session logs |
| Permission | The user permits clearing these logs whenever requesting disk cleanup. No additional confirmation is required within this target. |
| Retention | No minimum retention period for closed terminal logs. |
| Scope | Delete closed regular log files within this directory, including nested log directories. Preserve the root directory, permissions, symlinks, other mounts, and any unexpected non-log content. |
| Active files | Skip logs with open handles. If activity cannot be established, skip the uncertain files and report the limitation. Do not terminate terminals or truncate active logs. |

Measure this directory each run. The user's supplied estimate of 56 GiB is a discovery lead, not a current measurement or a promised recovery amount. Inspect names and file types without printing log contents, which can contain credentials or private terminal output.

Recognize logs by `.log` filenames or a verified terminal application's session-log naming pattern. Preserve ambiguous files. Treat an open-file check as inconclusive if it reports permission errors or cannot inspect the relevant processes.

For recurring growth, inspect the terminal application's logging settings and recommend a bounded retention or rotation policy. Standing permission to clear logs does not authorize changing application settings or installing a scheduled job.

## Adding a target

Record the machine/account match, exact path, content type, deletion permission, retention rule, active-writer handling, and exclusions. Distinguish user approval from an unapproved candidate. A large directory or a recommendation does not create standing permission.
