---
name: codeconverter-09-dependency-audit
description: Stage 09 of the codeconverter pipeline — audit every external dependency ("bad actors") that would break on cutover day, across 11 categories of hidden coupling. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 09.
context: fork
---

# Stage 09 — dependency-audit (fork)

**Goal:** Identify every external dependency that would break on cutover day. Scans
all sibling repos across 11 categories of hidden coupling: direct database access,
git submodule/subtree references, source code dependencies, shared library
consumers, wire format dependencies, message bus consumers, internal API consumers,
hardcoded URLs/ports/hostnames, mock/stub implementations, shared infrastructure
access, and deployment coupling.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` — it carries the **sibling repos path** and
   **deployment manifests path** (recorded by stage 01). If missing, stop and report.
3. Read `docs/codeconverter/03-dependency-discovery/references.md` and
   `docs/codeconverter/05-api-surface/API.md` for context.
4. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce
`docs/codeconverter/09-dependency-audit/bad-actors-analysis.md` — every external
dependency across all 11 categories, verified, severity-classified, with remediation
timelines.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/09-dependency-audit/`, plus your row in STATE.md.
- `bad-actors-analysis.md` starts with the standard artifact header block; Status
  `final` when done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] All 11 coupling categories were searched; each category section exists even if
      empty ("none found" with the searches that prove it).
- [ ] At least 3 search passes with different patterns (obvious matches, then
      aliases/abbreviations, then indirect references).
- [ ] Cross-reference audit done: every consumer in `references.md` checked for
      non-API couplings.
- [ ] Every finding is verified, severity-classified, and has a remediation timeline.
- [ ] **Every external repo this stage read appears in
      `00-source-provenance/provenance.json`** with a fetch timestamp no older than
      this stage's start date, and none has `working_tree: unrelated`.
- [ ] Category 1 (direct database access) and category 10 (shared infrastructure)
      findings are reconciled against `05c-datastore-peers` where that stage has run.
      This audit is repo-first and 05c is table-first; the two must agree, and a
      disagreement is a finding in its own right.

## Tips from experience

- Start with deployment manifests (Helm charts, Kubernetes configs) — the most
  dangerous bad actors are deployment artifacts, not source dependencies.
- Shared libraries are the most common category. Audit each library's own source —
  if it contains database access or wire-format assumptions, every consumer inherits
  those dependencies.
