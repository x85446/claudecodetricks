---
name: codeconverter-00-guidance
description: Stage 00 of the codeconverter pipeline — the guidance interview that establishes what the port actually is (scope, production status, field upgrades, data-store change, consumer lockstep, definition of done) before any analysis runs. Produces the scope charter every later stage reads. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 00.
argument-hint: [target-repo] [--from-artifacts]
---

# Stage 00 — guidance (interactive)

**Goal:** Establish *what the port is* before a single line of analysis is written.
This stage is an interview. Its output is the **scope charter** — the document every
later stage reads to know how much of the world it is allowed to change.

**Why this stage exists.** The pipeline used to open at stage 01, which asks about
*the service*: its language, its owners, its repos. That question presumes the shape
of the job is already settled. It is not. Users arrive at a rewrite with wildly
different shapes and the pipeline cannot tell them apart from stage 01's answers
alone:

- **Scope** — replacing one library, several libraries, a whole service, or an entire
  suite of services. Stage 10's alignment decision is meaningless if the scope is one
  library, and stage 03's sibling sweep is under-scoped if it is a whole suite.
- **Production status** — live production code with paying users, or a prototype
  nobody depends on. This single answer sets whether a parity harness, a rollback
  path and a cutover window are mandatory artifacts or wasted effort.
- **Field upgrades** — whether deployed artifacts in the wild (devices, agents,
  on-prem installs, pinned SDK versions) must keep working against the replacement.
  If they must, the old wire format is frozen and no amount of "we'll clean up the
  API" is available.
- **Data-store change** — and this is the dangerous one. Swapping a store
  (e.g. PostgreSQL → BoltDB) is safe *only* if every peer reaching that store is
  isolated behind an API or a shared library. A peer that opens its own connection
  to the old database breaks silently on cutover day, and no endpoint-level analysis
  will ever see it. Stage 05c (`codeconverter-05c-datastore-peers`) is the stage that
  determines this; **stage 00 is what makes stage 05c mandatory or optional.**
- **Consumer lockstep** — whether consumers can be migrated at the same time as the
  service, or must keep working unchanged. This decides whether "drop this endpoint"
  is ever an available move.
- **Definition of done** — the condition under which the port is finished. Without
  it, stage 11's plan has no terminating criterion and stage 12's Q&A has no target.

Stage 00 asks these questions up front. **Stage 12 (`codeconverter-12-migration-qa`)
revisits them at the end**, once discovery has produced facts, because several of
these decisions genuinely cannot be made until then. The charter is the shared
document at both ends: stage 00 writes it with the user's intent, stage 12 amends it
with what discovery found.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, this is a fresh `init` and the
   orchestrator has already created it. All paths are relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` if it exists — a re-run in a later round starts
   from the existing charter, not from a blank page.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.
4. If `docs/codeconverter/00-guidance/scope-charter.md` already exists, this is a
   **re-interview**. Read it, and run the round-N procedure in `instructions.md`
   (Step 7) rather than the fresh-interview procedure.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/00-guidance/`:

- `scope-charter.md` — the six required answers plus the derived stage-applicability
  table, written so a later stage can read it without re-interviewing the user.
- `scope-charter.json` — the same answers, machine-readable, so stages can branch on
  them without parsing prose.
- `MANIFEST.md` — per the template.

**All six questions get an answer. `unknown` is a permitted answer and it is not a
failure** — but an `unknown` must name the stage that will resolve it and be carried
onto stage 12's agenda. What is *not* permitted is an omitted question.

### `--from-artifacts` (non-interactive seeding)

When the user is not present — an automated run, a forked validation, or a pipeline
resuming without a human — run with `--from-artifacts`. In that mode the six answers
are **derived from existing artifacts instead of asked**, every derived answer cites
the artifact and line it came from, and its confidence is recorded as `derived` rather
than `stated`. A charter produced this way is valid input for later stages, and every
`derived` answer is automatically added to stage 12's agenda for human confirmation.
Never present a derived answer as a stated one.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/00-guidance/`, plus your row in STATE.md.
- `scope-charter.md` starts with the standard artifact header block; Status `final`
  when done. The JSON artifact is header-exempt (JSON has no comment syntax) —
  `scope-charter.md` carries the header for both.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] All six required questions — **scope, production status, field upgrades,
      data-store change, consumer lockstep, definition of done** — have an answer in
      both `scope-charter.md` and `scope-charter.json`. None is omitted; any answered
      `unknown` names the stage that will resolve it.
- [ ] Every answer records its `source` (`stated` by the user, or `derived` with the
      artifact and line it came from) and its confidence.
- [ ] The **stage-applicability table** is present and assigns every pipeline stage
      one of `required` / `optional` / `not-applicable`, each with a one-line reason
      traceable to a charter answer.
- [ ] The data-store answer explicitly states whether stage 05c
      (`codeconverter-05c-datastore-peers`) is **required**. If the store changes, or
      the answer is `unknown`, 05c is required and the charter says so.
- [ ] The definition of done is written as a **checkable condition**, not an
      aspiration — something a later reader can evaluate as true or false.
- [ ] Every `unknown` and every `derived` answer appears in the
      `Carry to stage 12` list at the end of the charter.
- [ ] The charter is readable standalone: a later stage can act on it without
      re-interviewing the user and without reading the conversation that produced it.

## Tips from experience

- The scope answer is the one users under-state. "We're just replacing the auth
  library" and "we're replacing the auth service" produce completely different
  pipelines, and the difference is often only visible in the second answer, not the
  first. Ask what else ships in the same artifact.
- "Is it in production?" is not a yes/no in practice. Ask *who breaks* if the
  replacement is wrong: internal-only, one customer, every customer, or devices in
  the field. That version of the question gets a specific answer.
- The data-store question hides inside "we're switching frameworks". A framework
  change that drags a storage-engine change with it is a store change and needs the
  same peer study. Ask specifically what the persistence layer becomes.
- Users answer "can consumers migrate in lockstep?" optimistically. Follow up with
  "who deploys them, and on whose schedule?" — a consumer owned by another team on a
  quarterly release train is not in lockstep, whatever the intent.
- Do not let the interview turn into stage 01. Stage 00 is about the **shape of the
  job**; stage 01 is about the **service**. If an answer is really a service fact
  (language, repos, owners), note it for stage 01 and move on.
