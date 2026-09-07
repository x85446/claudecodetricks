---
name: codeconverter-05a-endpoint-consumers
description: Stage 05a of the codeconverter pipeline — map every endpoint in the API surface to the repos and call sites that invoke it, and name every endpoint with no known caller. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 05a.
context: fork
---

# Stage 05a — endpoint-consumers (fork)

**Goal:** For every endpoint in `05-api-surface/API.md`, the list of callers with
`file:line` evidence — and, just as importantly, the list of endpoints with **no
known caller**. Stage 05 says what the service exposes; this stage says who uses it.
Nothing else in the pipeline answers "who calls endpoint X".

**Why this stage exists.** On the first IAM run, a proposal to drop 84 API-key
endpoints was justified by "zero sibling services call `/v3/api-keys`". That was
true of the 33 base endpoints and wrong about the other 51 — 32 aggregator, 13
legacy-v1, 6 admin — whose callers are distributors, operator support tooling and
legacy customer integrations. A sibling-service scan cannot see any of those, and
nothing forced the difference between "we found no caller" and "there is no caller"
to be written down. Stage 09 did not catch it either: its job is *hidden* coupling
(database, wire formats, deployment), and it correctly classified the frontends as
"REST consumer, no hidden coupling". This stage produces the per-endpoint consumer
map that drop, merge and deprecate decisions in stages 10 and 11 actually need.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` — it carries the **sibling repos path** and
   **deployment manifests path** (recorded by stage 01). If missing, stop and report.
3. Read `docs/codeconverter/05-api-surface/API.md` — the endpoint inventory this
   stage maps. It is the contract; do not re-derive it, do not edit it.
4. Read `docs/codeconverter/03-dependency-discovery/references.md` for the known
   consumer list, and `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md`
   if it already exists — both are starting points, neither is a substitute for the
   scan.
5. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/05a-endpoint-consumers/`:

- `endpoint-consumers.md` — the readable map: callers per endpoint, the
  no-known-caller ledger, and the evidence-vs-proof statement.
- `endpoint-consumers.json` — the same data, machine-readable, one record per
  endpoint, for stages 10 and 11 to consume without re-parsing prose.
- `MANIFEST.md` — per the template.

Scan every sibling repo and every test/support repo named in STATE.md. Zero callers
is a real, reportable result — never a reason to leave an endpoint out.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/05a-endpoint-consumers/`, plus your row in STATE.md.
- `endpoint-consumers.md` starts with the standard artifact header block; Status
  `final` when done. The JSON artifact is header-exempt (JSON has no comment syntax) —
  `endpoint-consumers.md` carries the header for both.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] Every endpoint row in `05-api-surface/API.md` appears exactly once in
      `endpoint-consumers.json`; the two counts are equal and the counting command
      that proves it is shown.
- [ ] Every caller entry carries repo, `file:line`, a quoted snippet, and a match
      strength (`exact` or `inferred`).
- [ ] The no-known-caller ledger lists every zero-caller endpoint, grouped by
      audience, each with the searches that were run against it.
- [ ] A second ledger lists every endpoint whose *only* callers are `gateway-route`,
      `test-suite` or `doc-or-fixture` — the "no product caller" set. Where the
      service sits behind a route table this is the decision-relevant number and the
      zero-caller count is not.
- [ ] Three search passes were run — literal static path, templated/dynamically
      constructed path, and client-library / config / deployment reference — and the
      pattern list for each pass is shown.
- [ ] Variant endpoints (aggregator, admin, legacy-vN) are attributed separately from
      their base-path twins; no caller is inherited across variants by substring
      match alone, and every cross-variant `inferred` match says so explicitly.
- [ ] `endpoint-consumers.md` states plainly that "no known caller" means "not found
      in the repos we hold" — evidence, not proof — and names the consumer classes
      that live outside every scannable repo.

## Tips from experience

- **A mock of the service is not a caller of it.** Sibling repos ship mock IAM
  servers to test against (`connector-ca/mock/iam.js`, `mbed-billing/iam-mock/`),
  and a mock *implements* the endpoint — `app.post('/internal/v1/trusted-certificates', ...)`
  looks exactly like a call site to a path matcher. On the IAM run 40 records across
  four mock servers were classified `sibling-service`, the strongest kind, and one
  endpoint was kept out of the no-product-caller ledger on the strength of a mock
  alone. Treat `mock`/`stub`/`fake`/`wiremock` path segments as test paths. This is
  the one false positive that costs you something: it suppresses a true negative.
- The expensive mistake is a false negative, not a false positive. An endpoint
  wrongly marked "no known caller" gets dropped and breaks a customer; an endpoint
  wrongly marked "has a caller" merely gets built. When a match is ambiguous, record
  it as `inferred` against every endpoint it could reach rather than picking one.
- Frontends rarely contain the literal path. They call a generated SDK or a thin
  wrapper (`api.apiKeys.list()`); find the wrapper first, then map wrapper methods to
  paths, then count call sites of the wrapper.
- Aggregator paths (`/v3/accounts/{id}/thing`) and base paths (`/v3/thing`) share a
  tail. `grep "api-keys"` cannot tell them apart, and that is exactly how the
  original 51-endpoint miss happened. Anchor on the full path shape, not the noun.
- **If the service sits behind an API gateway, its route table will "call" almost
  every endpoint.** On the IAM run, `apis-cm.yaml` alone accounted for 2,249 of 5,535
  caller records and pushed 571 of 637 endpoints into "has a caller" — while only 201
  had a caller of any stronger kind. Report both numbers and say which one a drop
  decision reads.
