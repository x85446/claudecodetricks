# skills-harness team log

2026-09-11T01:12:30Z — Starting. Team: skills-harness. Branch: feature/axolotl-iterate-infra (already checked out, will not switch).
Goal: add `harness:` frontmatter field (values: `claude-code` | `codex` | `unknown`) to iterate plan files, carried through by both harnesses.
Step 2a/2b: Claude-side planner writes `harness: claude-code`; /iterate carries it through refinement/roll-forward untouched; missing field parses as `unknown`.
Step 3a/3b: Codex port writes `harness: codex`; regenerate family port with --force; validate.py must pass, manifest cap respected.

Plan: read skills/iterate-planner/SKILL.md (Step 6 schema), skills/iterate-planner/procedures.md (roll-forward/close), skills/iterate/SKILL.md (frontmatter carry-through), then codex-skills/iterate-planner/ port, then sync-all.sh.

2026-09-11T01:18:00Z — CONTRACT SETTLED (for team `dashboard` to consume now):
  Frontmatter field: `harness:`
  Values: `claude-code` | `codex` | `unknown`
  Placement: plan frontmatter, alongside `planner:`/`planner-version:` (Claude side) — right after those two lines.
  Semantics: records which harness's planner/executor produced the plan file. Written ONCE at plan creation
  (by /iterate-planner on Claude Code -> claude-code; by the Codex port of iterate-planner -> codex) and
  carried through untouched by every subsequent operation (refinement, roll-forward, execution transitions,
  resume, close). A plan file with the field absent, blank, or holding any value other than the two known
  ones reads as `unknown` — dashboard code must NOT default an absent field to either harness. This is a
  parse-time fallback, not a third harness.
  Confirmed by reading skills/iterate/SKILL.md: /iterate only ever does additive frontmatter edits (Set X,
  Add Y line) on existing plans -- it never rewrites the whole frontmatter block -- so `harness:` survives
  every /iterate transition automatically once written by the planner. Only two write sites need the new
  line: iterate-planner's fresh-plan schema (Step 6) and iterate's own direct-fresh-task schema (Step 2,
  the no-planner path where /iterate creates a plan itself while running on Claude Code).

2026-09-11T01:24:00Z — Pre-existing breakage hit during validation (not my blocker, noting and continuing per
execution rules): `iterate-run name next` fails everywhere right now with "every animal codename in the pool
is already in use" — this is exactly what team `names` (steps 11-13, same plan) is fixing in parallel. Applying
the skill's own documented fallback for when the naming tool is unusable ("pick any common animal not already
present in this project's plans/") to unblock my own validation: scratch plan named `gecko`.

2026-09-11T01:28:00Z — Step 2 exercised end-to-end in isolated scratch git repo (not this project's live
iterate state): created plan `gecko` per the updated Step 6 schema -> confirmed `harness: claude-code`
present in frontmatter on read-back. Simulated a refinement (appended step 2 + validation 2) -> frontmatter
incl. harness unchanged. Simulated roll-forward to `gila` per updated procedures.md step 3 -> harness carried
verbatim (`claude-code`), branch carried verbatim, same as contract. Built a plan file with the `harness:`
line entirely absent (pre-existing/legacy shape) and ran the parse rule the schema documents -> resolves to
`unknown`, no exception. Scratch repo left at
/private/tmp/claude-503/-Users-travis-workspace-x85446-claudecodetricks/5ff1de2d-0a50-47ba-8638-4f1cd28ad64a/scratchpad/harness-validation
for anyone who wants to inspect it; not part of this project's tree.
##ITERATE-VALIDATION## {"step":2,"status":"met","note":"scratch plan create/refine/roll-forward all preserve harness: claude-code; field-absent legacy shape resolves to unknown with no crash"}

2026-09-11T01:34:00Z — Step 2 (planner schema + carry-through) done:
  - skills/iterate-planner/SKILL.md: added `harness: claude-code` to the Step 6 plan-file schema (next to
    planner:/planner-version:); updated the refinement-preserve line to include `harness`; added rule 27.4
    documenting the contract, worded harness-agnostically (no "here" self-reference) so it stays true verbatim
    once mirrored into the Codex port.
  - skills/iterate-planner/procedures.md: roll-forward procedure (op 8, step 3) now carries `harness:` verbatim
    alongside `branch:`.
  - skills/iterate/SKILL.md: added `harness: claude-code` to the direct-fresh-task Step 2 schema (the no-planner
    path where /iterate creates a plan itself); added an explicit "do NOT touch harness:" note to the
    planned->executing transition paragraph, matching the existing "do NOT touch Started:" pattern.
  - Invoked /skill-builder per the step's tag to audit style consistency of these edits -- no changes needed
    (its checklist is about skill frontmatter/naming/family routing, which none of this touches; the numbered-
    rule and schema-comment style already match neighboring content).
  - Did NOT bump planner-version or the shared iterate family version, per plan constraint (coordinator decides
    at the end).

Moving to step 3: Codex mirror.

2026-09-11T01:42:00Z — Ran the prescribed regen command first pass, and validate.py (the real gate) reported
iterate-planner AND iterate-conductor as BROKEN (invalid YAML in the frontmatter `description:` — unquoted,
contains internal ": " sequences). Traced root cause: scaffold.sh's description emission was unquoted in the
version I first ran against. Also discovered validate.py itself did not exist in this branch/working tree
minutes earlier (confirmed via git: it ships only on the unmerged `fix/conductor-stands-down` branch, commit
b0b95046 "the cap is a gate, and broken ports never ship"). While investigating, the exact same fix
(fmfield.py + quoted scaffold.sh emission + validate.py) appeared UNCOMMITTED in this shared working tree —
someone/something else is concurrently backporting it in parallel with my work (not something I initiated or
was asked to do). Confirmed my own convert.py edit (the harness: codex SAFE rule) is untouched and isolated
from that concurrent change. Re-running my regen now that scaffold.sh's quoting fix is present, to get a
correctly-quoted, harness-correct port.

2026-09-11T01:50:00Z — Step 3 done and validated:
  - Added one SAFE, deterministic rule to skills/skill-2-codex/scripts/convert.py (item 7): a plan schema's
    literal `harness: claude-code` default line becomes `harness: codex` in any Codex port — anchored to
    frontmatter/schema-style line start so it never touches prose mentioning "claude-code" elsewhere. This is
    the mechanism that makes "regenerating the family carries it" true generically, not just for iterate-planner.
  - Reworded rule 27.4 in skills/iterate-planner/SKILL.md (done in step 2) to state the claude-code/codex mapping
    without a self-referential "here", so the same sentence stays correct verbatim in the Codex mirror with no
    translation needed.
  - Ran the exact prescribed command: `skills/skill-2-codex/scripts/sync-all.sh --only iterate-planner --force
    iterate-planner`. Along the way found and worked around a live shared-working-tree situation: another
    process was concurrently backporting an unrelated pre-existing fix (validate.py + fmfield.py + quoted
    frontmatter emission in scaffold.sh, matching commit b0b95046 on the unmerged `fix/conductor-stands-down`
    branch) into this same tree while I worked. Waited for that to land, then re-ran regen.
  - RESULT, confirmed by direct file read: codex-skills/iterate-planner/SKILL.md now contains `harness: codex`
    in its Step 6 schema (was `harness: claude-code` on the Claude side — correctly flipped). Frontmatter is
    valid YAML (`yaml.safe_load` succeeds). `.portstamp` shows a fresh mechanical build (manual=false — correct,
    since --force wiped any prior hand-edit and there is currently none to protect).
  - `python3 skills/skill-2-codex/scripts/validate.py codex-skills/iterate-planner` -> exit 0, no BROKEN lines:
    this skill's port is individually clean.
  - `python3 skills/skill-2-codex/scripts/validate.py codex-skills` (whole mirror) -> exit 1, manifest
    6238/8000 (fits), but 19 BROKEN entries: accounts, filemaster (x2 reasons), iterate-brainstorm,
    testmaster + 7 of its children, uxmaster + 6 of its children. NONE of these are iterate-planner or
    iterate-conductor (both clean). These are pre-existing, unrelated to harness work, unmodified by this
    step, and match (same count, same shape) the exact backlog the concurrent b0b95046 backport describes
    fixing project-wide — that is a separate, much larger regeneration pass outside step 3's scope (named
    only iterate-planner). Noting and continuing per standing execution rules rather than expanding scope to
    fix 19 unrelated skills.
  - Dry-ran (no force-regen, out of scope) the same convert.py substitution against skills/iterate/SKILL.md's
    own schema line and confirmed it also correctly flips to `harness: codex` -- the mechanism generalizes to
    the rest of the family whenever they're next regenerated, as 3a requires, without needing to force every
    member now.
  - Installed the three Claude-side skills I edited: `skills/skillctl install iterate-planner iterate
    skill-2-codex`. `skillctl status` for all three: clean (no longer stale/broken). Whole-repo `skillctl
    status`: 0 broken, 0 no-source (15 stale entries remain, all in other teams'/other projects' scope,
    none touched by this work). `skillctl audit`: the drifted/unregistered entries are all unrelated
    repos/skills (izuma, gravhl, warden, prd-graph, voicemode) -- pre-existing, not caused by this step.
  - Did not bump planner-version or the shared iterate family version, per plan constraint.
##ITERATE-VALIDATION## {"step":3,"status":"met","note":"codex-skills/iterate-planner/SKILL.md stamps harness: codex, valid YAML, validate.py passes for this skill individually and manifest fits (6238/8000); whole-mirror validate.py still red from 19 pre-existing unrelated broken ports (accounts/filemaster/testmaster*/uxmaster*/iterate-brainstorm), none touched by this step"}

TEAM DONE: harness: field added to the plan schema (claude-code/codex/unknown), carried through untouched by
refinement, roll-forward, and every /iterate execution transition on the Claude side; Codex port of
iterate-planner regenerated and now stamps harness: codex with valid YAML; contract validated end-to-end in an
isolated scratch repo and against the real validate.py gate. Pre-existing, unrelated breakage noted: the
codename pool exhaustion (being fixed by team `names`) and 19 already-broken Codex ports outside this step's
scope (accounts, filemaster, iterate-brainstorm, testmaster family, uxmaster family) — whole-mirror validate.py
will only go fully green once those are separately regenerated.

2026-09-11T02:05:00Z — Received two messages from team-lead (stop-before-install, then tooling-restored/proceed).
Acknowledging, not replying (per instruction), with the facts:
  - I never passed --install to sync-all.sh at any point, before or after these messages -- confirmed just now
    by checking ~/.agents/skills/iterate-planner/SKILL.md: still the pre-session copy (mtime Sep 9 15:20, no
    `harness:` line), i.e. untouched by any Codex-mirror install. The only "install" I ran was `skillctl
    install iterate-planner iterate skill-2-codex`, which is the unrelated Claude-Code-side push to
    ~/.claude/skills/ (skillmap.tsv-driven), not the Codex mirror install team-lead's messages were about.
  - I never used `git stash` in the shared working tree at any point.
  - Independently found and already logged (2026-09-11T01:42:00Z-01:50:00Z entries above) the exact same
    situation team-lead's two messages describe: validate.py/fmfield.py missing on this branch, a concurrent
    backport landing them plus the 8000 budget and quoted-description fix, and the resulting whole-mirror
    BROKEN list (accounts, filemaster, iterate-brainstorm, testmaster + 7 children, uxmaster + 6 children) --
    confirmed unrelated to and untouched by this step.
  - Re-ran with the now-fully-restored tooling per team-lead's stated clean success condition ("regenerate,
    then validate.py codex-skills --budget 8000 must exit 0 with zero BROKEN lines; if regenerating everything
    is out of scope, validate only the ports you touched and say so"): `sync-all.sh --only iterate-planner
    --force iterate-planner` -> same result as before (iterate-planner/iterate-conductor clean, 19 unrelated
    pre-existing ports still BROKEN, manifest 6238/8000 fits). Scoped check `validate.py
    codex-skills/iterate-planner --budget 8000` -> exit 0, zero BROKEN lines. `harness: codex` confirmed still
    present and correct in the regenerated file. This matches exactly what I already reported as step 3's
    result -- nothing changes.
  - Per team-lead's own instruction ("if regenerating everything is out of your step's scope, say so... do not
    silently leave the mirror red") -- said so already, saying so again here: regenerating the other 19 broken
    skills is outside step 3 (named only iterate-planner) and I have not attempted it. The mirror stays red
    until whoever owns that broader regeneration (or a later plan) runs it.
  - Not touching convert.py further; team-lead confirmed they deliberately left it alone.

No change to prior conclusions. Steps 2 and 3 remain done and validated as recorded above.
