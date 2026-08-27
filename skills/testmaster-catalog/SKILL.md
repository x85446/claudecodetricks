---
name: testmaster-catalog
description: TESTMASTER child (invoked via /testmaster): the organizing index — requirement to cases to covered code — recomputing validity (valid/drifted/orphaned/unverified) as the code changes.
argument-hint: <status | drift | coverage | link <test-id> <files...> | rebuild>
version: 1.1.0
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
4. **link `<test-id> <files...>`** — record which source files a test covers. Drift detection is only as good as `covers`, so this is how it gets accurate.
5. **rebuild** — re-derive the catalog from the test files' TESTMASTER headers plus registry.json, preserving existing requirement statements. Use after a big refactor or when the catalog and tree disagree.

## Steps for any invocation

1. Read `catalog.json` and `registry.json` (create the catalog from the registry if it's missing — every test becomes `unverified` under a synthetic `req-unclassified` until someone links it).
2. Recompute validity for every test before answering: resolve `covers` against the tree, run the git diff against `last_validated_commit`, read `runs`/`last_result` from the registry. Never answer from a stale cached state.
3. Write back the recomputed states, then render the requested view.
4. Report the one-line summary line last: `N requirements, M cases — <valid>/<drifted>/<unverified>/<orphaned>`.

## Rules

1. **Never delete anything.** Orphaned entries are *reported* for `/testmaster-prune` to act on with its evidence standard. This child is the index, not the gardener.
2. **Never write timing or results** — `avg_ms`, `runs`, and `last_result` belong to `/testmaster-run` alone.
3. **Validity is recomputed, never trusted from the file.** A cached `valid` from three commits ago is exactly the lie this catalog exists to prevent.
4. **A test with an empty `covers` cannot drift** — report it as such in `coverage` rather than silently calling it valid. Unknown coverage is a gap, not a pass.
5. **Requirement statements stay in the user's words**, verbatim from `/testmaster-derive`. Don't paraphrase them into implementation language.
