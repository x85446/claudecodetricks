# /iterate-planner — on-demand operation procedures

Full procedures for the router ops that don't run on every plan write: **status** (op 0.5), **flatify** (op 6), **close** (op 7), and **roll-forward** (op 8). SKILL.md routes here; follow the matching section exactly. Everything these procedures reference (the Teams schema, "One plan = one feature branch", the changelog sweep, rule numbers) is defined in SKILL.md.

## Status report procedure (op 0.5 — read-only snapshot)

One glance at where this project's git state and iterate plans stand. Gather everything with real commands and real file reads — never from memory — then print EXACTLY this shape (it's a fixed report, not prose):

```
Git:
-------------------------
current branches: <count>
on branch: <branch>
ahead of main: <count>

Plans:
-------------------------
Active: <count>
Partially complete: <count>
  - <name>:
      complete tasks: <n>
      problem tasks: <n>
      questions (for human): <n>
  - ...
Archived: <count>
```

**Git section** (skip the whole section with one line `not a git repo` if there's no `.git`):
- `current branches` — count of local branches: `git branch --list | wc -l`.
- `on branch` — the branch actually checked out right now: `git branch --show-current` (empty output means detached HEAD — print `detached @ <short-sha>` instead). This line answers "am I on main or on a plan's feature branch?" — never infer it from a plan's `branch:` field.
- `ahead of main` — commits on HEAD not on the default branch: `git rev-list --count <default>..HEAD` (default = `main` or `master`, whichever exists). On the default branch itself this is 0.

**Plans section** (all counts from `./.claude/iterate/`; if the directory doesn't exist, print `Active: 0`, `Partially complete: 0`, `Archived: 0`):
- `Active` — count of plan files in `plans/`.
- `Partially complete` — the subset of active plans with at least one complete step AND at least one incomplete step. For each, read its file and report:
  - `complete tasks` — steps whose validation has passed (checked `## Steps` entries, or steps the Status/Log records as validated green).
  - `problem tasks` — incomplete steps the Status/Log shows have actually FAILED or blocked at least once (failed validation, blocked-on-operator, access-check failure). Never-attempted steps are not problems — don't count them here.
  - `questions (for human)` — open items only a human can clear: an unreached-but-pending `human-gate:` step, blocked-on-operator asks still unanswered, explicit questions parked in the Status/Log or Decisions log. 0 when none.
- `Archived` — count of files in `archive/` (count `.md` files only, not `.teams/` directories).

Print nothing else — no advice, no next-step suggestions, no per-plan prose beyond the fixed fields. If a count is genuinely unreadable (corrupt plan file), print `?` for that cell rather than guessing. Then **stop**.

## Flatify procedure (op 6 — the reverse of teamify, always cheap)

Unlike Teamify, this is never expensive — Teams is purely additive metadata (see the Teams schema in SKILL.md: "the flat lists themselves are untouched"), so undoing it is just removing that layer, not re-deriving anything.

1. Resolve the target plan (named, else current).
2. If the plan has `phase: executing`: **refuse** — "that plan is executing; stripping its Teams table mid-run would orphan any dispatched team's tracked status. Let it finish or stop the loop first." Same guard as the delete op. Do not modify the file.
3. If the plan is already flat (no `## Teams` section / `teamed` not `true`): report "`<name>` is already flat" and **stop** — a no-op, not an error.
4. Delete the `## Teams` section from the plan file entirely. Set `teamed: false` (keep the field, same convention as a plan that was never teamed).
5. The `## Steps` / `## Validation` lists are untouched — no step content, numbering, or validation is affected. Any step that had its own access-preflight step (inserted per-team) stays exactly where it is in the flat list, just no longer grouped under a team name.
6. Re-print the full plan (now without a Teams block) with a one-line confirmation: `<name> flattened — N steps now run serially`.

## Close procedure (op 7 — archive incomplete, merge withheld)

The user is deliberately ending a plan that didn't finish all-green. Honor it — don't argue, don't try to finish the remaining steps first — but make the consequences visible.

1. Resolve the target plan (named, else current).
2. If `running:` is a fresh timestamp (within 90s — a live `/iterate` run): refuse — "a run is live on `<name>`; stop the loop first (`/loop` with no args), then close." Otherwise proceed (a stale heartbeat or `running: false` is the normal case here).
3. Cancel any auto-resume loop/cron the plan set up (`/loop` no-args, or the recorded `cron_job_id:`), so nothing re-fires against an archived plan.
4. Set `running: false`. In the plan file, leave the Steps/Validation checkboxes exactly as they stand — the archive IS the record of what didn't finish.
4.5. **Publish the partial changelog — through the same final sweep.** If `## Changelog draft` has entries, run `/iterate`'s two-pass sweep (consolidate multi-attempt lines into as-landed lines; validate every claim against the actual tree — a closed plan is especially likely to hold draft lines for work later abandoned) and distill the survivors into `CHANGELOG.md` + `RELEASES.md`, entry heading suffixed `(partial — plan closed unfinished)`. Completed work deserves its record even when the plan didn't; an empty or fully-swept-away draft publishes nothing.
5. Move `plans/<name>.md` → `archive/<UTC-timestamp>-<name>-closed.md` (and `plans/<name>.teams/` → `archive/<UTC-timestamp>-<name>-closed.teams/` if teamed). Repoint/clear `current` same as the delete op.
6. **Report — the merge line is mandatory, first, and unmissable:**
   ```
   <name> closed and archived with N of M steps unfinished (list the unfinished step numbers + one-line gists).
   ⚠ feature branch `<branch>` has NOT been merged — the PR <is still open at <url> | was never opened>. The work on it is not on <default branch>.
   Say "merge <name>'s branch" to merge it as-is, or "roll <name>" was the moment to carry the work forward — the branch is preserved either way.
   ```
   If the plan has no `branch:` (not a git repo), omit the merge lines. If the user's close order ALSO said to merge ("close it and merge", "merge what we have and close"), that's an explicit merge order — run the merge flow (push → PR → merge → delete branch, via `/feature-branch finish` + PR merge, same flow `/iterate` uses on all-green) before archiving, and report the merge as done instead.

## Roll-forward procedure (op 8 — unfinished steps to a new plan, same branch)

The user wants the unfinished work to continue as a fresh plan without losing the branch state accumulated so far.

1. Resolve the source plan (named, else current). Same fresh-`running:` refusal as Close step 2; same loop/cron cancellation as Close step 3.
2. Identify the unfinished steps: unchecked entries in `## Steps` (and any step whose paired Validation isn't met per the Status/Log). Completed steps stay behind in the source plan.
3. Create the new plan: name from `iterate-run name next`, `phase: planned`, `Started: <now>`, carrying over — renumbered 1:1 from 1 — the unfinished Steps + their paired Validations, the Goal (reworded to cover only the remaining scope if the original is now too broad), all still-relevant Constraints (including `Access:` ones whose steps carried over), and **`branch: <the source plan's branch>` verbatim — do NOT create a new branch and do NOT invoke `/feature-branch start`**. The whole point is the new plan continues on the same branch, on top of the commits already there. Carry each surviving step's `## Provenance` line with it (renumbered alongside). Also carry the source's `## Changelog draft` lines forward verbatim into the new plan's draft section — nothing publishes now; when the successor completes, its distillation covers the whole branch's accumulated story. If the source was teamed, re-run Teamify fresh over the carried steps (the old table's step numbers are meaningless after renumbering).
4. Set `current` = the new plan.
5. Archive the source: `archive/<UTC-timestamp>-<name>-rolled-to-<newname>.md` (+ `.teams/` dir if present), checkboxes left as they stand.
6. **Report — same mandatory merge line as Close:**
   ```
   rolled <old> → <new>: N unfinished steps carried over (renumbered), M completed steps archived with <old>.
   ⚠ feature branch `<branch>` has NOT been merged — <new> continues on it; the merge happens when <new> finishes all green.
   Type /iterate (or /iterate <new>) to resume execution.
   ```

## FFIV macro expansion (Find, Fix, Iterate, Verify)

**FFIV = Find, Fix, Iterate, Verify** — a quality sweep over a scope, hunting everything sweep-findable: mistakes (bugs, broken flows, errors, inconsistencies) AND UX/UI enhancement opportunities. When the planning request or an added step contains "FFIV" (any case — "FFIV the dashboard", "/ip FFIV", "add FFIV for the settings page"), expand it into these four paired steps scoped to what was named (unscoped = the whole project's user-facing surface):

- **Find** — Na: Sweep `<scope>` by exercising it for real (load the pages, click the flows, run the commands) hunting mistakes and UX/UI enhancement opportunities; record each as a numbered finding in the plan's Status/Log. `[skill: /uxmaster]` when the scope is a user interface — its analysis + platform-expert children ARE this sweep, and their findings ledger is the record. Nb: findings list recorded, with the sweep's actual coverage named (which pages/flows/commands were exercised).
- **Fix** — Na: Address every recorded finding. Nb: each finding is fixed, or carries a logged defer decision with reason — none silently dropped.
- **Iterate** — Na: Re-sweep `<scope>`; fix anything new; repeat. Nb: the latest sweep produced ZERO new findings (dry), with the dry sweep's coverage logged.
- **Verify** — Na: Exercise every fixed area end-to-end as a user would. Nb: each fix demonstrated live, no regressions in adjacent flows.

**Standing FFIV finding rules** — patterns every FFIV sweep hunts by default, beyond ad-hoc mistakes (append here as the user declares new ones):
- **Settings informational text** → any informational/help/explainer text sitting inline in a settings screen is a finding; the standard fix is relocating it under an **(i)** info affordance (tooltip, popover, or expandable) so the setting's control stands alone and the explanation is one tap away.

The four steps get normal treatment — skill tags, provenance (`You asked for FFIV over <scope>.` on all four), team classification (they usually stay one team or unassigned: each phase needs the previous phase's context). Findings fixed during the sweep feed `## Changelog draft` like any other change. FFIV never replaces the standing finishers (Step 5.8) — TESTMASTER and product-docs still run after it.
