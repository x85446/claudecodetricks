---
name: codeconverter-01-service-profile
description: Stage 01 of the codeconverter pipeline — interview the user to produce the service profile that all later stages depend on. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 01.
---

# Stage 01 — service-profile (interactive)

**Goal:** Capture what is being rewritten and why, before any analysis starts. Every
later stage assumes this context exists. This stage is an interview — the user must
be present.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, ask the user. All paths below
   are relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` and `.claude/skills/codeconverter/templates.md`
   (uniform artifact templates).
3. Read `instructions.md` in this skill's directory — the interview template listing
   every field the profile must answer.

## Execute

1. If `docs/codeconverter/01-service-profile/service-profile.md` already exists, show
   it and ask whether to revise or accept as-is.
2. Otherwise run the interview from `instructions.md`. Ask in small batches
   (AskUserQuestion), covering at minimum: service name and purpose, source
   language/framework, why it's being rewritten, target language preference, GitHub
   orgs to scan (used by stage 03), sibling repos path and deployment manifests path
   (used by stage 09), data stores, and known consumers.
3. Write `docs/codeconverter/01-service-profile/service-profile.md`. Show it to the
   user for confirmation before finalizing.
4. Record the GitHub orgs, sibling repos path, and deployment manifests path in
   STATE.md so forked stages don't have to ask.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/01-service-profile/`, plus your row and the
  fields above in STATE.md.
- `service-profile.md` starts with the standard artifact header block; flip its
  Status to `final` after user confirmation.
- Finish by writing `docs/codeconverter/01-service-profile/MANIFEST.md` per the
  template, with the exit criteria below copied in and honestly checked.
- Do not make the stage-complete commit — the orchestrator commits.

## Exit criteria (copy into MANIFEST)

- [ ] Every field of the profile template is answered or explicitly marked N/A.
- [ ] The user confirmed the final profile text.
- [ ] STATE.md carries the orgs-to-scan, sibling repos path, and manifests path.

## Notes

- This is the one stage that is meant to stay interactive permanently.
- Do not start analysis work here — no code reading beyond what's needed to ask
  informed questions.
