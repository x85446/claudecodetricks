---
name: codeconverter-06-domain-analysis
description: Stage 06 of the codeconverter pipeline — deep-dive documents on the specialized domain logic of the source service, plus a gap analysis against the target platform. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 06.
---

# Stage 06 — domain-analysis (interactive)

**Goal:** Understand the specialized features this service implements so the
replacement can do better, not worse. The document list is custom per service type —
the user approves it before writing begins.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, ask the user. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md`, `docs/codeconverter/02-codebase-analysis/`,
   and `docs/codeconverter/05-api-surface/API.md`.
3. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Begin with Step A: present your assessment of this
service's domain and your recommended analysis documents, then **wait for the user's
confirmation before writing any documents**.

Each approved document gets two sections: **Product Manager Perspective** and
**Technical Analysis**. After all documents are written, produce
`GAP_ANALYSIS.md` comparing what the source implements against what the target
platform provides natively.

Output goes to `docs/codeconverter/06-domain-analysis/` — one `{DOMAIN}.md` per
approved domain, plus `GAP_ANALYSIS.md`.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/06-domain-analysis/`, plus your row in STATE.md.
- Every domain doc and GAP_ANALYSIS.md starts with the standard artifact header
  block; Status `final` when done.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] The user approved the document list before writing started.
- [ ] Every approved domain has its document, with both PM and Technical sections.
- [ ] `GAP_ANALYSIS.md` compares source features against target-platform natives.

## Tips from experience (auth/IAM sources)

- The policy-engine document is the highest-value artifact when the target supports
  OPA natively — the gap between "turn on OPA" and porting a custom cascade to Rego
  is large.
- An RBAC document must cover the multi-tenancy model, not just role hierarchy —
  most RBAC middleware handles role checks but not account-scoped isolation.

## Interactivity note

This stage is interactive for now (document-list approval). After one full pipeline
run, the orchestrator will ask whether to convert it to fork mode with a default
document list.
