# Iterate Task — fix the weekly build and prove the runtime on Sequoia

name: waterbuck
Started: 2026-10-20T20:00:00Z (planned)
phase: executing
Executing: 2026-10-20T23:24:14Z
executor-version: 5.22.0
harness: claude-code
teamed: false
branch: feature/waterbuck-fixes

## Goal
Fix the weekly build and its leftovers. (Sanitized fixture shaped like the real Waterbuck plan.)

## Steps
- [x] 1. Verify the ci user can list instances and read run logs.
- [x] 2. Move the two retained guests into project `retained`.
- [x] 3. Tell the owners of the moved guests where they live.
- [x] 4. Make every macOS bake delete its container on every outcome.
- [x] 5. Fix the weekly build's coverage floor failure.
- [x] 6. Make the published Go assets build byte-identically on the Mac and the runner.
- [x] 7. Judge teardown cases by the evidence their own run recorded.
- [ ] 8. Run the full macOS live suite against Sequoia.

## Validation
- [x] 1. `incus list --project ci` prints the instances and `gh run view --log-failed` prints the floor line.
- [x] 2. Both guests are STOPPED in `retained` with the same snapshots.
- [x] 3. Each target inbox shows the inform.
- [x] 4. Cases bake.cleanup.failed, bake.cleanup.green and bake.cleanup.interrupted pass.
- [x] 5. `make test-coverage` reports cov.report.floor PASS.
- [x] 6. Case publish.reproducible passes and SHA256SUMS lines match.
- [x] 7. Cases td.evidence.run and td.evidence.negative pass.
- [ ] 8. Every listed Sequoia case passes through `testmaster suite run`.

## Status / Log
- 2026-10-21T06:47:55Z step 1 validation met: ci lists instances, config and snapshots. Step 2: export the protected stopped guest before any move.
- 2026-10-21T07:00:30Z step 4 partial: preserved logs in evidence/waterbuck-bakes; both identities created 2026-10-18. Deleted exactly macos-bake-sequoia and macos-bake-tahoe. Step 6 build DONE exit=0. Step 7 fixture execution: td.evidence.run and weekly.results.selector 2 passed, 0 failed (17.8s), includes the named leaked-guest negative evidence.
- 2026-10-21T07:06:37Z step 4 bake.cleanup.green rerun passed in 2.6s; failed/interrupted passed first run.
- 2026-10-21T07:11:24Z step2 ci.retained.moved passed (0.6s): both stopped, snapshot names preserved.
- 2026-10-21T07:30:34Z step8 Sequoia selection active, first whole-file invocation test-osx-runtime.
