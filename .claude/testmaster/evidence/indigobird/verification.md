# Conductor acceptance evidence

This record covers the binary conductor and single-site project workspace. The TESTMASTER registry and history contain execution results and timing; the catalog maps the 202 acceptance cases to automated assertions and the scoped native observations below. Private raw provider logs, allowance payloads and account identities remain outside the repository.

Final fast/standard execution: **297 passed, 0 failed**, 72.526 seconds. The Makefile acceptance target executes **126 passed, 0 failed**, 59.282 seconds. Catalog recomputation reports **384 valid cases**, including all **202 conductor acceptance cases**. Formatting, vet, build and 120 documentation scans pass.

## Native interface

Native Chrome checks exercise the installed localhost:8420 service at desktop and 320 px. The evidence covers disabled project registration, invalid folders, canonical queue links, project/Settings Back and Forward, full plan/archive timelines, compact rows and consistent Duplicate/Delete links versus Edit/Add buttons.

Settings checks cover parent-scoped sequence/preset CRUD, derived subscription prefixes, rename propagation, reference guards, fractional power curves, named policies, time-window persistence and schedule/end-action choices. Save, Cancel, Close and Escape preserve draft boundaries. Modal keyboard focus wraps and returns to its origin; the background retains its scroll position. Nested preset Save persists independently of an outer sequence Cancel.

Ordered simulators select the first eligible maximum-band row: Yellow includes Green, Red includes both. Native changes exercise reorder, repeated profiles, busy projects, empty queues, stale accounts, hard quotas and schedule times. Overdrive selectors contain only enabled participating account windows and use a shared sample instant. At 56% used with a 60% target, bright green ends at 60%, the black marker sits at 56%, and normal green beyond 60% remains dark. At the target, normal geometry returns. Zero/full use,100% targets, neutral losing rows, one highlighted winner, text-only bounce and reduced-motion suppression are observed. Fractional day 6.5 displays days and hours and survives a session reload.

A saved midnight overdrive window converts through the authoritative preview, saves and reloads its exact fractional hour offset from authenticated reset evidence. Disabled fixture definitions remain disabled. The preview preserves the current occurrence and explains future reset-relative/DST behavior; unknown and lossy conversions reject.

The installed project workspace and active plan timeline agree on canonical identity, stopped state, effective sequence, reason, next-opportunity caveat, usage source and advancing age. Full Coordinator and Requirements content remains accessible. Positive future openings, project-window intersections, fourteen-day bounds, DST and trusted reset requirements have executable service/HTTP assertions.

## Real execution and live events

Installed Codex and Claude each complete a bounded task/validation pair and typed delivery in disposable repositories. Exact receipt bytes, matching result identities, completed plan checkboxes and final archives are verified. Provider exit status alone does not count as completion. These fixture delivery policies require no external forge operation.

A managed web restart during a real Codex assignment preserves the child PID and run identity; the durable cursor advances 7→12 without duplicate sequences. Native Chrome observes intermediate activity, terminal completion and durable history after reload. A separate real pulse proves foreground latency: its session event at 2026-10-16T21:45:10.070481Z appears at 21:45:12.423Z,2.352519 seconds later. Another tool event appears within 2.550 seconds. Both are below two 3-second polling cycles. Expansion, focused summary, route and scroll remain stable while later events arrive. The pulse completes its pair and delivery with 12 and 10 unique events respectively.

During a controlled service outage, the page shows Reconnecting while retaining an unsaved sequence draft, focus and background scroll. Recovery restores Live and clears the stale style. Public HTTP tests separately verify expired cursor recovery, retained pagination, restart continuity and rejection of cross-project event scope.

## One service and safe deployment

Port 8420 serves the dashboard, settings, assets, APIs and full plan/archive routes. The supplied gar archive bookmark retains its historical content and read-only labels. Missing sources and malformed/traversal inputs return navigable errors. The preview listener on 51994 is stopped.

The installer refuses an owned-test unrelated listener on 8420 without killing or replacing it. Restoring the managed service succeeds. Repeated `make serve-install` and `make run` reuse the same managed PID and preserve the rollback binary hash. No alternate application port starts.

Acceptance fixture directories are removed and the original configuration is restored with a revision-checked write. Global admission is disabled, no projects are enrolled and the nine editable templates remain. Real project queues are not armed by acceptance.

## Reproduction and scope

- `make conductor-acceptance` runs the bounded conductor, budget, provider and integration selection through TESTMASTER.
- `testmaster suite run` runs the registered fast/standard suite and records real timings and results.
- `make fmt-check vet all` verifies formatting, vet and the repository build.
- `make conductor-cover-case COVER_PKG=PACKAGE COVER_TEST=TEST COVER_FILE=PROFILE` reproduces per-test measured coverage. `coverage.json` records source paths, initialization-subtracted block counts and profile hashes.
- `controls-case-test-map.json` and `engine-case-mappings.json` state each supporting test's assertions and limits. An individual test's partial scope is combined with the catalog's other evidence; it is not a claim that one test proves every layer.

Native observations are recorded on 2026-10-16. Weeks-long exhaustion and wall-clock soak are outside this bounded acceptance; injected clocks exercise rollover and recurrence. External CLI child binaries are not instrumented by their parent test coverage profile, and their coverage is not fabricated.
