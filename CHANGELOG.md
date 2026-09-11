# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com).

## [axolotl] - 2026-09-10

### Added
- `iterate-run serve` binds a fixed default port (8420), accepts `--port 0` to take an ephemeral one and print it, and serves `GET /healthz` returning the build version.
- launchd agent `com.x85446.iterate-run-serve` (RunAtLoad + KeepAlive) with `make serve-install` / `serve-status` / `serve-uninstall`; logs to `~/Library/Logs/iterate-run/`.
- launchd agent `com.x85446.iterate-run-chrome` keeping a browser on the dashboard in a dedicated `--user-data-dir`, debug port 9242, `CHROME_BIN` overridable for Chromium-family browsers.
- Plan frontmatter field `harness:` (`claude-code` | `codex` | `unknown`), written by both harnesses' planners and carried through refinement and roll-forward.
- Dashboard renders a per-plan harness badge, a per-project rollup when a project mixes harnesses, harness-correct launch syntax, and a per-project conductor status line including the `NO TRIGGER` degraded state.
- Version resolution from either the frontmatter markers or the Codex ports' `**Version:** iterate family x.y.z` body line.
- `make run` — builds and serves the dashboard on a free port, printing its URL.
- `TestAnimalPool` asserting pool integrity: 26 letters, per-letter floor, total, no duplicates, lowercase ASCII, correct first letter.

### Changed
- Animal codename pool grown from 110 to 1,442 entries; per-letter floor of 12.
- Root Makefile migrated to the 2-layer convention: seven `##@` sections, `## help text` on all 31 targets, generated `make help`, launchd logic delegated to `makehelp.sh`.
- `TestNextPlanNameSeedsFromDisk` derives its seed from `animalsByLetter` instead of a hardcoded five-name list.
- README documents the dashboard, both always-on services, and plan naming.

### Fixed
- `iterate-run name next` no longer fails outright when a letter is exhausted: it advances to the next letter with capacity, and errors only when the whole pool is spent, reporting remaining counts per letter.
- `launchctl bootstrap` race under concurrent launchctl activity, via a `bootstrap_with_retry` helper that backs off and treats "already loaded" as success.
- Chrome's remote-debugging port moved off 9222, which would otherwise shadow the user's normal browser for anything attaching to that well-known port.
