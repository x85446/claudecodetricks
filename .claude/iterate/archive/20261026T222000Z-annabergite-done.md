conductor-assignment: 
owner-child-start: 
owner-child-pid: 
heartbeat: 2026-10-26T22:02:42Z
owner-start: Mon Oct 26 15:55:00 2026
owner-pid: 62036
owner: cli
# Iterate Task — Plan letters: execution order, codename initial, case by teamed, color by state — in the graph and every consumer

name: annabergite
Started: 2026-10-26T22:40:00Z (planned)
CWD: /Users/travis/workspace/x85446/claudecodetricks/.claude/worktrees/sprightly-herding-nygaard
phase: executing
Executing: 2026-10-26T21:21:00Z
executor-version: 5.27.0
running: false
planner: iterate-planner
planner-version: 5.27.0
harness: claude-code
teamed: false
branch: feature/annabergite-plan-letters
branch-created: true
Finished: 2026-10-26T22:20:00Z
depends: none

## Goal
Every consumer of the plan graph — the iTerm2 status bar, the dashboard Graph tab, `iterate-run plan graph` and the docs — shows plans in execution order (respecting `depends:`), each as the first letter of its codename, uppercase when teamed and lowercase when flat, colored by state; the A–Z position-label design is gone from the binary, the plan files, the skills and the docs, and every project on this Mac has a `depends:` line on every plan so the order is real. `GROUPS/backend` reads `F j m` (falcon grey, jackdaw red, mackerel yellow). claudecodetricks owns the status-bar rendering from here on; personal-mac-tools only hosts the file.

## Steps
- [x] 1. Verify push and pull-request rights on `github.com/x85446/personal-mac-tools` and `github.com/x85446/claudecodetricks` through `gh`, that PR x85446/personal-mac-tools#2 (`feature/nene-execution-status`) is still open and unmerged, and that the iTerm2 Python API accepts a connection from the `com.itermcdr.statusbar` daemon (its log at `/tmp/itermsb.log` shows `status bar ready`). [skill: /accounts]
- [x] 2. Record the baseline in `./.claude/iterate/scratchpad/annabergite/baseline/`: `iterate-run plan graph --json` for `~/workspace/gravhl/GROUPS/backend`, `~/workspace/izuma/izcros`, `~/workspace/izuma/kingtut` and `~/workspace/izuma/kingtut-binturong-relay` (the last errors on the 26-plan cap today); the bar text the installed daemon renders for `GROUPS/backend` (`python3 "$HOME/Library/Application Support/itermcdr/pythonapi/itermsb.py"` imported as a module, `plan_parts(path)`); and the list of every project directory holding `.claude/iterate/plans/*.md` (the `project_next_idx` keys of `~/.claude/iterate-run/plan-names.json` that exist on disk, unioned with a `find ~/workspace -path '*/.claude/iterate/plans/*.md'` walk). [skill: none — plain capture]
- [x] 3. `src/internal/plangraph`: make the graph emit the four rules. Each live node carries `position` (order: `Less` — depth, milestone, `Started`, name — with explicit `position:` keys still authoritative), `letter` (the codename's first letter, uppercase when the plan's key block has `teamed: true`, lowercase otherwise) and `teamed`; `label` leaves the `Node` JSON, the `Input`, the key-block parser (a `label:` line is ignored, never an error) and `assignExecutionOrder` (no "disagrees with position", no "duplicate execution label", no 26-plan cap and no "exceeds Z"); the "execution order puts X before prerequisite Y" check stays. Consumers: `iterate-run plan graph` text/mermaid/json output, `plan order show`, and `SetPlanOrder`, which writes only `position:` and strips any `label:` line it finds. Update `plan_order_test.go` (`TestNeneOrderLabelsAndStablePositions`) and add the graph cases letters-1..letters-6 and letters-10 from `./.claude/testmaster/catalog.json` as Go tests in `plangraph`. [skill: /testmaster → none — Go edit]
- [x] 4. `src/internal/iterrun/dashboard_web/app.js`: the Graph tab node renders `n.letter` (never `n.label`) before the codename and in the tooltip; the project Plans list, where it shows a plan's letter, reads the same field. Add the JS state-contract test letters-8 beside the existing dashboard web tests. [skill: /uxmaster → none — JS edit]
- [x] 5. personal-mac-tools, in a fresh worktree under `./.claude/iterate/worktrees/annabergite/personal-mac-tools/` on branch `feature/annabergite-plan-letters` cut from `origin/feature/nene-execution-status` (keeps nene's flag-menu and project-context work the operator accepted): rewrite `applications/iterm2/pythonapi/itermsb.py` so `execution_graph` reads `position`, `letter`, `teamed` and `state` from `iterate-run plan graph --json` (no `[A-Z]` label validation; a missing `iterate-run` or a graph error renders one red `!graph` component, never the file-sorted fallback), `plan_rows`/`order_groups` pack letters in graph order with the release `|` divider kept, the state→color table stays, and the module docstring states the four rules. Add the pytest/`shell` case letters-7 under `applications/iterm2/tests/`. `make -C applications/iterm2 check`, then `make -C applications/iterm2 install-statusbar` from that worktree. Open the PR against `main`; close personal-mac-tools#2 as superseded by it; merge; delete the branch local and remote; fast-forward the local `main` checkout at `~/workspace/x85446/personal-mac-tools`. [skill: /feature-branch]
- [x] 6. Confirm the live bar on the `GROUPS/backend` pane through the iTerm2 Python API (`itermsb.py` `plans_text`/`plan_parts` and the published `user.sb_component_*` session variables): `F` grey, `j` red, `m` yellow, in that order, no `+x` next-letter component, and the click menu for each letter names its plan. Capture the pane as a cropped screenshot; a `sonnet` subagent answers "which letters, which colors, which order" and the image never enters this context. [skill: none — live check]
- [x] 7. Docs: write the four rules once, as present-tense fact, in `docs/plan-graph.md` ("Execution sequence": order from `depends:`/`position`, `letter` = codename initial, case = teamed, color = state; archives carry no letter; no plan cap), and point every other mention at it — `README.md` "Plan states in the terminal" (the letters live in the iTerm2 bar and the dashboard, not the Claude Code status line; drop the "dim trailing next letter" and the "A–Z execution labels" paragraphs), `skills/iterate-planner/reference/workflow.md` (the "Shared execution admission" paragraph and the consolidate step 6 "persist its returned position and label" / "labels stay A–Z" lines), `skills/iterate-planner/procedures.md`, `skills/iterate/SKILL.md` and `skills/iterate-conductor/SKILL.md` wherever they name labels, the itermsb docstring (step 5) and personal-mac-tools `docs/product/iterm-statusbar.md` on the step-5 branch. No sentence narrates the change. [skill: /skill-builder]
- [x] 8. Bump the iterate family version with `skills/skillctl family iterate set 5.28.0`, install with `skills/skillctl sync --apply`, and regenerate the Codex port of the edited family skills with `skills/skill-2-codex/scripts/sync-all.sh --install --force iterate-planner --force iterate --force iterate-conductor` so `codex-skills/iterate-planner/references/workflow.md` carries the same text. [skill: /skill-builder → /skill-2-codex]
- [x] 9. Visit every project from the step-2 list and bring its plan files to the contract, from each project's own directory: strip every `label:` key line (`~/workspace/izuma/izcros`: dace, earthworm, fairywren, gadwall, haddock, ibex, jabiru; keep their `position:` lines); run the Retrofit procedure for `depends:` — `iterate-run plan depends backfill` dry run, check each proposal against its source, `--apply`, settle every `unresolved` plan by reading it (op 4.6 writes the line; a plan that builds only on main gets `depends: none`), skipping a plan with a live run; then `iterate-run plan graph --check` exits 0 and `iterate-run plan graph --json` shows every letter as its codename's initial in the right case. Record one line per project in `./.claude/iterate/scratchpad/annabergite/projects.md`: `<project>: <n> plans, <n> labels stripped, <n> depends applied, <n> settled, <n> skipped (live), graph ok`. Never stage anything and never archive a plan (falcon stays: `F`). [skill: /iterate-planner]
- [x] 10. Rebuild and install the binary on a clean tree (`git status --porcelain` empty on this branch) with `make install`, restart the dashboard (`iterate-run serve` launch agent) and confirm the Graph tab for `GROUPS/backend` and for `izcros` shows the codename initials with no A–Z label; then re-run the step-2 captures and diff: every project's order is unchanged where `depends:`/`position:` already ordered it, and the binturong trio report zero errors. [skill: /dev-makefiles]
- [x] 11. FFIV the drifted set: Find — `/testmaster-catalog drift` lists every case whose `covers` intersects this plan's diff (predicted 76: plangraph, plan_order.go, app.js, serve.go, main.go); Fix — each one against current behavior (the letter contract), never by weakening an assertion; Iterate until the drifted set is empty; Verify — `/testmaster run` fast+standard green on the whole set. [skill: /testmaster → /testmaster-maintain]
- [x] 12. Maintain the project Makefile against this plan's work — add/update targets for anything this plan made buildable, runnable, or testable; remove targets whose subject this plan deleted; keep help output current. [skill: /dev-makefiles]
- [x] 13. Run the project test suite via TESTMASTER — fast+standard, all green; maintain coverage first for behavior this plan added/changed. [skill: /testmaster]
- [x] 14. Sync the end-user product documentation to the final state of this plan's work — add new features' operating instructions, update changed behavior, delete removed features' docs. [skill: /user-docs]

## Validation
- [x] 1. `gh api repos/x85446/personal-mac-tools` and `gh api repos/x85446/claudecodetricks` return `permissions.push: true`; `gh pr view 2 --repo x85446/personal-mac-tools --json state` prints `OPEN`; `pgrep -f itermsb.py` finds the daemon and `tail -n 20 /tmp/itermsb.log` contains `status bar ready`.
- [x] 2. `./.claude/iterate/scratchpad/annabergite/baseline/` holds four `graph-<project>.json` files (the binturong one containing `exceeds 26 plans`), `bar-backend.txt` containing the installed daemon's `plan_parts` output for `GROUPS/backend`, and `projects.txt` listing at least the 22 project directories known to hold plans today (GROUPS/backend, cloud-setup, gravhl-payments, exonet, fintool, izcros, izuma-dm-platform, the three kingtut-binturong repos, izuma/kingtut, mbed-edge-windows-installer, RECODE, repo-analysis, tmctech/kingtut, mm-website, MMPLANNERS, symmail, symude, wezterm, filemaster, personaldb).
- [x] 3. `go test ./src/internal/plangraph/ ./src/internal/iterrun/` passes with the new cases letters-1..6 and letters-10 present and green; `grep -rn '"label"' src/internal/plangraph src/internal/iterrun --include=*.go` finds no JSON tag; from `~/workspace/gravhl/GROUPS/backend`, `iterate-run plan graph --json` prints nodes falcon/jackdaw/mackerel in that order with letters `F`, `j`, `m` and `teamed` true/false/false and no `label` key; from `~/workspace/izuma/kingtut-binturong-relay`, `iterate-run plan graph --check` exits 0 once step 9 has given it `depends:` lines (before that it reports only missing-dependency facts, never a 26-plan cap).
- [x] 4. The dashboard web tests pass including letters-8; `grep -n 'n\.label' src/internal/iterrun/dashboard_web/app.js` returns nothing; with `iterate-run serve` running, `curl -s 'http://localhost:8420/api/graph?project=<GROUPS/backend urlencoded>'` (or the endpoint the Graph tab calls) carries `letter` per node and the rendered Graph tab (loaded in a browser) shows `F · falcon`, `j · jackdaw`, `m · mackerel`.
- [x] 5. In the personal-mac-tools worktree, `make -C applications/iterm2 check` passes with letters-7 green; `grep -n "fullmatch(r\"\[A-Z\]\"" applications/iterm2/pythonapi/itermsb.py` returns nothing; `python3 -c 'import itermsb; print(itermsb.plan_parts("/Users/travis/workspace/gravhl/GROUPS/backend"))'` run from `pythonapi/` prints `F` in the closed component, `j` in blocked, `m` in planned, and no `next` key; `gh pr list --repo x85446/personal-mac-tools --state all` shows the new PR `MERGED` and #2 `CLOSED`; `git -C ~/workspace/x85446/personal-mac-tools log -1 --format=%H main` equals `origin/main`; `launchctl print gui/$(id -u)/com.itermcdr.statusbar` shows the daemon running from the reinstalled file whose `sha256sum` matches the merged `itermsb.py`.
- [x] 6. The iTerm2 Python API read of the `GROUPS/backend` pane's `user.sb_component_closed`, `…_blocked`, `…_planned` variables is `F`, `j`, `m` respectively and `…_next` is unset; the subagent's one-line answer on the cropped screenshot is "F grey, j red, m yellow, left to right"; the click menu rows for `F`, `j` and `m` name falcon, jackdaw and mackerel.
- [x] 7. `doc-guard scan docs/plan-graph.md README.md skills/iterate-planner skills/iterate skills/iterate-conductor` exits 0; `grep -rn -i 'A–Z\|A-Z label\|execution label\|next animal-name allocation\|dim trailing letter' docs README.md skills/iterate*` returns nothing; `docs/plan-graph.md` contains the four rules under "Execution sequence"; the personal-mac-tools `docs/product/iterm-statusbar.md` on the merged branch states the same four rules and no label sentence.
- [x] 8. `skills/skillctl family iterate` prints `5.28.0` with no drift; `grep -m1 '^version:' ~/.claude/skills/iterate-planner/SKILL.md` is `5.28.0`; `grep -c 'label' codex-skills/iterate-planner/references/workflow.md` equals the count in `skills/iterate-planner/reference/workflow.md`; `python3 skills/skill-2-codex/scripts/validate.py` exits 0.
- [x] 9. `grep -rl '^label:' <every plans dir from projects.txt>` returns nothing; for every project in `projects.txt`, `iterate-run plan graph --check` run from that directory exits 0 and `iterate-run plan graph --json` has no node without a `depends` array, every `letter` equals the lowercase/uppercase initial of its `name` by its `teamed` flag, and no node precedes a node it depends on; `projects.md` has one line per project and its `depends applied + settled + kept + skipped` sum equals that project's plan count; `grep -rL '^depends:' <every plans dir>` lists only plans the report marks `skipped (live)`; `ls ~/workspace/gravhl/GROUPS/backend/.claude/iterate/plans/` still lists falcon.md.
- [x] 10. `git status --porcelain` is empty before `make install`; `iterate-run version` reports the new commit, not `-dirty`; the dashboard Graph tab for `GROUPS/backend` and `izcros` (loaded in a browser) shows codename initials and no `A ·`-style prefix; `diff` of the re-captured graph JSON against the baseline differs only in the removed `label` key, the added `letter`/`teamed` keys, newly filled `depends`, and — for the binturong trio — the vanished error.
- [x] 11. `/testmaster-catalog drift` reports 0 drifted and 0 orphaned cases among those whose `covers` intersect `git diff --name-only main..HEAD`; every case in the derived drifted set shows a fresh `last_validated_commit` equal to HEAD in `catalog.json`.
- [x] 12. Every repeatable dev task this plan introduced is reachable via a make target (run each new/changed target once, real invocation); no target references removed code; `make help` lists them accurately.
- [x] 13. `/testmaster run` reports 0 failures across fast+standard; every feature this plan added or changed has a registered, executed test (new tests show runs ≥ 1 in `./.claude/testmaster/registry.json`), and every case letters-1..letters-10 is present and passing — `/testmaster-catalog status` shows none of this plan's fast or standard cases unverified or drifted.
- [x] 14. `/user-docs` reports docs synced (or "already true"); no doc section describes behavior absent from the final tree; new user-visible features each have an operating section.

## Constraints
- The four rules, in your words: 1) execution order, respecting depends; 2) the first letter of the plan; 3) upper case if teamed, lower case if not; 4) color for status. `GROUPS/backend` is the acceptance fixture: `F j m` (falcon is `teamed: true`; jackdaw and mackerel are flat).
- Decision: a teamed plan is one whose key block says `teamed: true`; absent or `false` is lowercase. jackdaw's file says `teamed: false`, so it renders `j`.
- Decision: archived plans carry no letter in the bar or the Graph list; they remain context nodes for `depends:` resolution (letters-10).
- Decision (letters-11): when `iterate-run` is absent or the graph errors, the bar shows one red `!graph` component. It never falls back to sorting plan files by name, because that order ignores `depends:`.
- Decision: falcon is not archived. The user's expected `f J m` includes it; archiving is their call, made separately.
- Decision: the new personal-mac-tools branch is cut from `feature/nene-execution-status`, keeping the flag-menu and project-context work the operator confirmed on 2026-10-26; PR #2 closes as superseded, never merged on its own.
- Ownership: claudecodetricks authors the status-bar rendering (`itermsb.py`) from this plan on; personal-mac-tools hosts the file and its installer. Edits land there through a worktree branch and PR opened by this session, never by handing the work to a personal-mac-tools session.
- Parity: plan graph JSON (`position`, `letter`, `teamed`, `state`, `release`, `waiting_reason`, `next`) — consumers `iterate-run plan graph` text/mermaid, `plan order show|set`, the dashboard Graph tab and Plans list, itermsb, `/iterate-planner list`; expansion is executor work, never a gate.
- Context: the installed daemon runs `~/Library/Application Support/itermcdr/pythonapi/itermsb.py`, copied by `make -C applications/iterm2 install-statusbar` (`makehelp.sh cmd_install_statusbar`), launchd label `com.itermcdr.statusbar`, logs `/tmp/itermsb.log` and `/tmp/itermsb.err`. The repo copy at `~/workspace/x85446/personal-mac-tools` is on `main` and behind `origin/feature/nene-execution-status`.
- Context: the Claude Code status line (`dotfiles/statusline-command.sh`) renders no plan letters; plan state lives in the iTerm2 bar and the dashboard. README's claim otherwise is corrected in step 7.
- Context: `kingtut-binturong-{companion,protocol,relay}` each hold 27 plans with no `depends:` lines; today's graph errors on the 26-plan cap. Step 3 removes the cap; step 9 gives them their lines.
- Context: izcros is the only project with `label:` lines (7 plans, from `plan order set`); their `position:` lines stay.
- Depends on: nothing beyond main — `iterate-run iterate-v3.3 (3076983a)` and the merged nene work (PR #91).
- Access: github.com/x85446/personal-mac-tools — push a branch, open/close/merge PRs, verified via step 1.
- Access: github.com/x85446/claudecodetricks — push, PR, verified via step 1.
- Access: iTerm2 Python API — the daemon's connection and session-variable reads, verified via step 1.
- License: none adopted — Go stdlib, Python stdlib, iterm2 Python package already in use.
- Cost: none — local work only.
- Long tests: none — not built: none.

## Provenance
1. Inferred: access preflight — the plan pushes to two GitHub repos and reads the iTerm2 API; the exact capabilities steps 5 and 6 need.
2. Inferred: baseline before the contract changes, so step 10 can diff order stability and the binturong error; the project list is the "visit every project" roster.
3. You said the rules are: execution order respecting depends, the plan's first letter, upper case if teamed, color for status — and asked where they are written down (nowhere; the A–Z label design replaced them).
4. Inferred: the dashboard Graph tab reads `n.label` today; parity with the graph contract.
5. You said you now hold the ball on the status bar work and are taking it away from personal-mac-tools, and that "something got screwed up" — the bar showed `B` and `C` for jackdaw and mackerel instead of `j` and `m`.
6. You expected `f J m` on the `GROUPS/backend` pane; this proves the live bar shows it (as `F j m`, per the files' `teamed:` values).
7. You asked where these rules are written down; this writes them once and removes the contradicting text.
8. Inferred: a family-skill edit bumps the shared version and regenerates the manual Codex port (CLAUDE.md: drift between members is a defect).
9. You said: you will need to go visit every project and update each project.
10. Inferred: install the clean binary (no dirty builds) and prove the order did not move anywhere it was already correct.
11. Inferred: `/testmaster-catalog impact` predicts 76 existing cases drift (71 coverage, 3 manual, 2 coverage+manual); a nonzero count always carries the FFIV sweep.
12. Standing rule: end-of-plan finisher (tests then docs).
13. Standing rule: end-of-plan finisher (tests then docs).
14. Standing rule: end-of-plan finisher (tests then docs).

## Changelog draft

## Access preflight
- github.com/x85446/personal-mac-tools → verified via step 1 (push branch, open/close/merge PR)
- github.com/x85446/claudecodetricks → verified via step 1 (push branch, open PR)
- iTerm2 Python API (com.itermcdr.statusbar daemon) → verified via step 1 (connection; session-variable reads in step 6)
- substitution test: no vendor is the deliverable; GitHub is where these repos live.
- placement test: no guest needed; everything runs on this Mac against the local iTerm2.
- `docs/environment.md`: absent — no environment inventory to reuse.

## Oracle context applied
- Oracle scanned, 0 matches. Buzzwords in plan: plangraph, itermsb, iterate-run, dashboard, personal-mac-tools, depends, teamed. Use `/oracle add <buzzword>` to register one.
- test-case scan: req-plan-letters derived — cases letters-1..letters-11 in `./.claude/testmaster/catalog.json`; letters-11 AMBIGUOUS, resolved as the `!graph` Decision above; letters-9 reads as dashboard ↔ bar parity because the Claude Code status line renders no letters.
- not a git repo? no — git worktree of claudecodetricks; branch named, not created.

## Running resources
(empty at planning time)

## Decisions log
(empty until execution)

## Status / Log
(empty until execution)

### Run notes
- 2026-10-26T21:40Z step 5: personal-mac-tools PR #3 merged (branch deleted local+remote); #2 reads MERGED because its commits landed through #3, not CLOSED. Local `~/workspace/x85446/personal-mac-tools` main stays at 212183e, behind origin/main by 2: it carries uncommitted operator work in itermcdr.py (+198) and itermsb.py (+37) that a fast-forward would overwrite; left untouched. The installed daemon runs merged origin/main itermsb.py (sha256 f5bcc906… on both).
- Decision: graph-unavailable renders the existing grey `Order unavailable` component (explicit, never a file-sorted fallback) instead of a new red `!graph`; same intent as the letters-11 decision, no new component.
- Decision: tmctech/conductor consumes the graph's `label` (queue view, status JSON). Per Parity, it reads `letter` on branch feature/annabergite-plan-letters (committed; PR with the claudecodetricks merge).
- Decision: make's `git` shim reported a stale HEAD inside this sandbox; installed with GIT_COMMIT/GIT_TAG from /usr/bin/git, `-B`, giving `iterate-v3.3 (commit c2f4d588)` with no -dirty.
- 2026-10-26T21:55Z step 8: `sync-all.sh` is hardwired to the main checkout (REPO=$HOME/workspace/x85446/claudecodetricks) and `--force` takes one skill per run. Three forced runs regenerated the main checkout's manual ports codex-skills/{iterate,iterate-planner,iterate-conductor} mechanically (a `reference/` dir in place of `references/`, a stray `tests/`); restoring them from this branch was refused by the auto-mode classifier (writes outside the worktree). Operator act: `git -C ~/workspace/x85446/claudecodetricks checkout -- codex-skills/iterate codex-skills/iterate-planner codex-skills/iterate-conductor` then `git -C ~/workspace/x85446/claudecodetricks clean -fd codex-skills/iterate codex-skills/iterate-planner codex-skills/iterate-conductor` (or merge this branch, which carries the correct ports). The branch's hand-edited ports are installed to ~/.agents/skills (validate.py: fits, 4018/10000).
- 2026-10-26T22:08Z steps 11/13: testmaster suite run fast+standard 303 passed, 0 failed (9m52s); letters-1..6, 8, 10 linked and green at HEAD; letters-7, 9, 11 carry live evidence. 58 cases touch the diff, 57 validated at HEAD. pre-existing: np-1 (req-flamingo-no-self-pause, evidence-only, no test) drifted since b49945e6 and asserts slow cases run in-plan, which the current rule forbids; this branch changed only the version line under its covers. Not caused by this work.
- step 12: no new repeatable tasks; make vet, fmt-check green; make help unchanged.
