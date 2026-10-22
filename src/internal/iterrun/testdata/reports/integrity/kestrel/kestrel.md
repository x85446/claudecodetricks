# Iterate Task — kestrel mixed-state fixture

name: kestrel
Started: 2026-10-22T08:00:00Z (planned)
phase: executing
Executing: 2026-10-22T09:00:00Z
executor-version: 5.22.0
harness: claude-code
teamed: true
branch: feature/kestrel

## Goal
Exercise every lane and requirement state at once.

## Steps
- [x] 1. Alpha writes the parser.
- [x] 2. Alpha writes the printer.
- [ ] 3. Beta signs the build.
- [x] 4. Gamma wires the CLI.
- [ ] 5. Gamma wires the TUI.
- [ ] 6. Delta writes the docs.
- [ ] 7. Delta records the demo.
- [ ] 8. Coordinator merges the work.

## Validation
- [x] 1. Parser cases pass.
- [x] 2. Printer cases pass.
- [ ] 3. The signed build verifies.
- [ ] 4. CLI cases pass.
- [ ] 5. TUI cases pass.
- [ ] 6. doc-guard passes.
- [ ] 7. The demo plays.
- [ ] 8. main holds every merge.

## Teams
| Team | Steps | Focus | Depends on | Agent | Status |
|---|---|---|---|---|---|
| alpha | 1,2 | Parser and printer | — | general-purpose | done |
| beta | 3 | Signing | — | general-purpose | blocked: no signing identity on this host |
| gamma | 4,5 | CLI and TUI | alpha | general-purpose | in-progress |
| delta | 6,7 | Docs and demo | gamma | general-purpose | pending |

## Status / Log
- 2026-10-22T09:00:00Z execution started.
