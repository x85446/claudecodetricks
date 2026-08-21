---
name: codeconverter-00-source-provenance
description: Stage 00-provenance of the codeconverter pipeline — fetch every external repository the pipeline will analyse, pin each to a resolved commit SHA with its date and how far behind upstream it is, and refuse to let a stage analyse a tree whose provenance is unknown. Invoked by the codeconverter orchestrator before stage 01 and re-invoked by any stage that reads an external tree.
context: fork
argument-hint: [target-repo]
---

# Stage 00-provenance — source-provenance (fork)

**Goal:** Every repository the pipeline reads is fetched, pinned to a named ref, and
recorded with its SHA, commit date, and distance from upstream — before any stage
computes a number from it. A tree with no provenance row does not get analysed.

**Why this stage exists.** On the IAM run, the target repo `izcr` had **never been
fetched**. Stage 07 analysed a 2026-02-11 snapshot while upstream had moved 306
commits. Every quantitative claim about the target — including "izcr 189 vs 114
endpoints", which reached STATE.md as settled fact — was computed against a tree that
had not existed for six months. Nothing in the pipeline checks this: stage 07's only
repo-related exit criterion was "the replacement repo exists locally on its branch",
which a six-month-old clone satisfies perfectly.

The pinned ref turned out to matter more than freshness alone. The local branch the
legacy run had pinned was not merely behind upstream — it shared **no ancestry with
it at all**: different root commit, no merge base, unreachable from every remote ref.
That is not a condition "fetch more often" detects. It needs a stage that resolves,
records and reconciles refs on purpose.

This is not a discipline problem, it is a structural one. As *Software Engineering at
Google* (ch. 22) puts it: "your local environment is almost guaranteed to be
permanently out of sync with head as the codebase shifts like sand around your work" —
and the remedy is automated regeneration against current HEAD, not care.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, this runs at `init` time and the
   orchestrator supplies the path.
2. Read `docs/codeconverter/STATE.md` — the **target repo**, **sibling repos path**
   and **deployment manifests path** name the trees to pin. Where STATE.md does not
   exist yet, take the paths from the orchestrator's `init` arguments.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/00-source-provenance/`:

- `provenance.md` — the provenance table, the ref-choice rationale for any repo where
  the obvious ref was rejected, and the divergence record for rejected refs.
- `provenance.json` — the same table, machine-readable, so a later stage can assert
  its inputs' freshness without parsing prose.
- `MANIFEST.md` — per the template.

**Every repo the pipeline will read gets a row**, including the source repo itself.
A repo that cannot be fetched gets a row saying so, with the error — never an omission.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/00-source-provenance/`, plus your row in STATE.md
  and the **target-of-record block** in STATE.md (this stage owns that block).
- Never fetch destructively. `git fetch` only — no checkout, no pull, no merge, no
  branch creation, no push. Pinning is a recording operation, not a mutating one.
- `provenance.md` starts with the standard artifact header block; Status `final` when
  done. The JSON artifact is header-exempt.
- Finish by writing `MANIFEST.md` per the template, exit criteria below copied in and
  honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] Every repository the pipeline will read has a row: repo name, absolute path,
      remote URL, chosen ref, **commit SHA**, commit date, ahead/behind counts, and the
      fetch timestamp. No repo the pipeline reads is missing a row.
- [ ] Every row carries **both** statuses, kept separate: `state` (is the pinned ref
      admissible — `current` / `unfetched`) and `working_tree` (how the checkout relates
      to that ref — `same` / `ahead` / `behind` / `unrelated`). A stage greps the working
      tree but cites the ref, so a row that records only the pin hides exactly the
      failure this stage exists to catch.
- [ ] Each row shows the command that produced it. A ref recorded without its
      `git rev-parse` / `git rev-list` output is not pinned, it is asserted.
- [ ] The fetch actually ran this session, and its result is recorded — including
      failures. A repo whose fetch failed is marked `unfetched` with the error, and
      that status propagates: stages may not compute figures from an `unfetched` tree.
- [ ] **Ancestry is checked, not assumed.** For every repo where the local branch
      differs from the upstream default, the output records `git merge-base` and root-
      commit comparison, so "behind" is distinguished from "unrelated history".
- [ ] Any ref the pipeline rejected is recorded with the reason and its divergence
      numbers, so a later reader does not re-adopt it.
- [ ] Every repo whose `working_tree` is `behind` or `unrelated` has a **finding**
      that states, with commands, whether the drift touched what any stage actually
      scanned — including a `comm` of the search pattern across both refs, which is the
      only check that reveals what a scan *could not have found*. "Possibly stale" is
      not an acceptable conclusion where a two-command check settles it.
- [ ] The **target-of-record** — the one ref every claim about the target codebase
      cites — is named explicitly, written into STATE.md, and carries a copy-pasteable
      verification command.
- [ ] Every row's `fetched_at` is from this session. A stage re-invoking this skill to
      satisfy its own freshness criterion gets a fresh fetch, not a cached table.

## Tips from experience

- **Fetch before you look at anything.** The order matters: a grep run before the
  fetch produces a number attached to the wrong ref, and the number usually outlives
  the awareness of which tree it came from.
- "Behind upstream" is the easy case. The dangerous case is a local branch with a
  **different root commit** — no merge base, unreachable from any remote ref. Check it
  explicitly with `git merge-base` and `git rev-list --max-parents=0`; a plain
  `rev-list --left-right --count` on unrelated histories still prints two numbers and
  looks reassuring.
- A branch that was never pushed is not a target of record, however carefully a prior
  run pinned it. `git branch -r --contains <sha>` returning nothing is the tell.
- Record the *rejected* ref as thoroughly as the chosen one. Without a written
  divergence record, the next reader finds the local branch, sees a plausible name, and
  re-adopts it.
- Sibling repos are usually many and usually read-only. Pinning them by directory sweep
  is fine, but do not let a sweep silently skip a repo with no remote — that repo's rows
  are the ones most likely to be a stale hand-copy.
- **Pinning the ref is not the same as fixing the scan.** Every other stage greps the
  checked-out working tree. A repo can be flawlessly pinned and still hand every
  downstream stage results from a seven-month-old checkout. That is why `working_tree`
  is a separate column and why a drifted repo owes a finding, not a footnote.
