---
name: testmaster-catalog
description: TESTMASTER child (invoked via /testmaster): the organizing index — requirement to cases to covered code — recomputing validity (valid/drifted/orphaned/unverified) as the code changes.
argument-hint: <status | drift | coverage | impact <plan> | link <test-id> <files...> | rebuild>
version: 1.3.0
---

# /testmaster-catalog — keep the suite organized and know what's still true

Obey the shared contracts in `/testmaster`'s SKILL.md. This child owns the organizing layer: what exists, what it proves, what it covers, and whether it's still valid after the code moved.

## The catalog

`./.claude/testmaster/catalog.json` — the index. `registry.json` stays the timing/result record; the catalog is the *meaning* record, and they join on test id.

```json
{
  "requirements": {
    "req-mute-on-play": {
      "statement": "<the requirement in the user's own words>",
      "source": "plan:quail step 4 | bug report | /testmaster-derive",
      "added": "<UTC date>",
      "cases": ["mute-1", "mute-2", "mute-3"]
    }
  },
  "tests": {
    "mute-1": {
      "requirement": "req-mute-on-play",
      "given": "...", "when": "...", "then": "...",
      "file": "history_test.go",
      "covers": ["history.go", "audio_device.go"],
      "covers_source": "coverage",
      "validity": "valid",
      "last_validated_commit": "6f15ace",
      "last_validated": "<UTC>"
    }
  }
}
```

## Validity states — this is what makes tests go valid and invalid with each change

| State | Meaning | How it's determined |
|---|---|---|
| `valid` | Passed, and nothing it covers has changed since | last run green AND `git diff --name-only <last_validated_commit>..HEAD` shares no file with `covers` |
| `drifted` | The code it covers changed after its last green run — the test may now be asserting the old behavior | that diff intersects `covers` |
| `orphaned` | The code it covers no longer exists | every path in `covers` is gone from the tree |
| `unverified` | Never executed (a freshly derived case) | `runs: 0` in registry.json |

**Drift is not failure.** A drifted test may still pass — it just hasn't been *proven* against the current code. It stops being drifted the moment `/testmaster-run` executes it green (which updates `last_validated_commit`). This distinction is the whole point: a suite that's all-green but 40% drifted is not a suite you can trust, and nothing else in TESTMASTER would tell you that.

## Router — parse `$1`

1. **status** (default) — the organized view, grouped by requirement:
   ```
   req-mute-on-play — "hitting play mutes the device when mute-devices is on"
     ✓ mute-1  valid      (history_test.go, 1.2s, fast)
     ~ mute-2  drifted    (history.go changed in 3 commits since last green)
     · mute-3  unverified (derived, no test written yet)
   Coverage: 12 requirements, 34 cases — 28 valid, 4 drifted, 2 unverified, 0 orphaned
   ```
2. **drift** — only the drifted and orphaned entries, with the commits that caused the drift. This is the post-change question: *what did I just invalidate?*
3. **coverage** — requirements with no cases, and cases with no test file. The gap list; hand it to `/testmaster-derive` (missing cases) or `/testmaster-maintain` (missing implementations).
4. **link `<test-id> <files...>`** — record which source files a test covers, by hand (`covers_source: "manual"`). Drift detection is only as good as `covers`, so this is how it gets corrected.

   To populate `covers` in bulk rather than one test at a time, that is `/testmaster-adopt` — it derives coverage from real per-test profiles. `link` is the correction, adoption is the feeder.
5. **impact `<plan>`** — project a planned change against the catalog, for `/iterate-planner`'s plan presentation. Returns exactly four numbers:
   - **current total** — cases in the catalog now.
   - **plan adds** — cases derived for this plan (Step 5.9 of the planner tags each with `source: plan:<name>`); 0 when the plan states no testable behavior.
   - **new total** — current + adds.
   - **could affect** — existing cases whose `covers` intersects the files the plan's steps will touch. This is *prospective drift*: those cases will need re-running to stay valid, whether or not they fail. Predict the touched files from the plan's Steps (named paths, the subsystem each step edits); when a step's target is genuinely unknowable, say so rather than inflating the count.

   **A nonzero `could affect` always means FFIV.** There is no "will not FFIV" branch: if this plan can put existing cases into drift, the plan sweeps them — Find the ones that actually drifted after the steps run, Fix them against current behavior, Iterate until the set is dry, Verify green. `could affect: 0` is the only case that skips it. The count is a *prediction*; which cases actually drifted is *derived* after execution from `git diff ∩ covers`, and the FFIV sweep acts on the derived set, not the prediction.

   **Report the confidence with the count.** `could affect` inherits the trust of the `covers` it read, so state the mix — `7 (covers: 7 coverage)` is a measured blast radius; `7 (covers: 2 coverage, 5 convention)` is mostly a naming-convention guess; `0 (no covers recorded — blast radius unknown)` is *not* a safe plan, it is an unadopted project. Never render a bare `0` that came from an empty catalog as though it meant "nothing is affected": route to `/testmaster-adopt` and say so.

   Report as the block in `/iterate-planner`'s Step 7 — no prose around it.
6. **rebuild** — re-derive the catalog from the test files' TESTMASTER headers plus registry.json, preserving existing requirement statements. Use after a big refactor or when the catalog and tree disagree.

   **`rebuild` maintains a catalog; it cannot create one from nothing.** A project that has never run TESTMASTER has no headers to read, so rebuild produces an empty index. Route it to `/testmaster-adopt` instead — that is the onboarding pass, and it is what every other op here silently depends on.

## Steps for any invocation

1. Read `catalog.json` and `registry.json` (create the catalog from the registry if it's missing — every test becomes `unverified` under a synthetic `req-unclassified` until someone links it).
2. Recompute validity for every test before answering: resolve `covers` against the tree, run the git diff against `last_validated_commit`, read `runs`/`last_result` from the registry. Never answer from a stale cached state.
3. Write back the recomputed states, then render the requested view.
4. Report the one-line summary line last: `N requirements, M cases — <valid>/<drifted>/<unverified>/<orphaned>`.

## Rules

1. **Never delete anything.** Orphaned entries are *reported* for `/testmaster-prune` to act on with its evidence standard. This child is the index, not the gardener.
2. **Never write timing or results** — `avg_ms`, `runs`, and `last_result` belong to `/testmaster-run` alone.
3. **Validity is recomputed, never trusted from the file.** A cached `valid` from three commits ago is exactly the lie this catalog exists to prevent.
4. **A test with an empty `covers` cannot drift** — report it as such in `coverage` rather than silently calling it valid. Unknown coverage is a gap, not a pass. Likewise **never treat a `convention` cover as a measured one**: `covers_source` travels with every count this skill reports, and a whole-catalog answer built from convention guesses says so.
5. **Requirement statements stay in the user's words**, verbatim from `/testmaster-derive`. Don't paraphrase them into implementation language.
