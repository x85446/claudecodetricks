<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase01-matrixbuilder.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 02-codebase-analysis**. When the text below says "Phase N",
> translate with this table:
>
> | Legacy phase | codeconverter stage |
> |---|---|
> | (service profile interview) | 01-service-profile |
> | Phase 1 | 02-codebase-analysis |
> | Phase 2 | 03-dependency-discovery |
> | Phase 3 | 04-test-baseline |
> | Phase 4 | 05-api-surface |
> | Phase 5 | 06-domain-analysis |
> | Phase 6 | 07-target-codebase |
> | Phase 7 | 08-gap-validation |
> | Phase 8 (bad actors) | 09-dependency-audit |
> | Phase 9 | 10-service-alignment |
> | Phase 10 (also titled "Phase 8 — Migration Plan") | 11-migration-plan |
>
> All file paths in this document have been rewritten to the
> `docs/codeconverter/` layout. There is no journey.md/journal in this pipeline —
> ignore any journaling instructions. Where this document conflicts with the
> stage's SKILL.md output contract (uniform headers, MANIFEST.md, output
> directory), **SKILL.md wins**.

---

# AI Coder Instructions — Full I/O Matrix + Rewrite Blueprint (25+ Iterations, No Early Stop)

## Mission (read carefully)
I need you to crawl this codebase repeatedly and produce a **complete, evidence-backed matrix** of **all inputs and outputs** so I can **re-write the entire system in a different language** by matching behavior, not by refactoring function-by-function.

You will **not** stop early. You will **iterate until 100% coverage** per the exit criteria below.

---

## What “counts” as Inputs/Outputs (use these definitions)

### External Inputs (anything entering the system boundary)
- HTTP/gRPC endpoints
- Webhooks
- CLI args, env vars, config files
- Scheduled jobs / cron triggers
- Message queue topics consumed
- Files/object storage reads
- stdin/sockets/other inbound channels

### External Outputs (anything leaving the system boundary)
- HTTP/gRPC calls to other services
- Webhook deliveries
- DB writes (Postgres included)
- Message queue publishes
- File/object storage writes
- Logs/metrics/traces (treat as outputs too)
- Emails/SMS/push notifications

### Internal Calls (within the repo)
- Module-to-module calls representing internal “service boundaries”
- Internal HTTP calls between services in the repo
- Background workers calling shared packages
- Repository/DAL calls
- Any adapter/client abstraction that represents an internal dependency

### Evidence rule (non-negotiable)
You must not guess. Every claim must include:
- exact file path(s)
- exact symbol(s): function/class/handler/route/RPC name
- and the specific line/section context (enough for me to verify quickly)

If you don’t have evidence, it is **not** “mapped” yet.

---

## Required Deliverables (living artifacts you continuously update)

### 1) External API Surface Table
For every externally reachable endpoint/RPC:
- Endpoint / RPC name
- Method
- Authn/Authz (who can call it, how enforced)
- Request schema (fields, types, required/optional)
- Response schema (fields, types)
- Status codes + error shape(s)
- Side effects (DB writes, outbound calls, queue publishes, file ops)
- “Why/When” (human-readable intent + triggering conditions)
- Evidence (paths + symbols)

### 2) Internal Call Graph
A caller → callee map:
- Caller module/service/function → callee module/service/function
- Call type (direct call, HTTP, async job, queue, DB access)
- Data passed (high-level schema / DTO)
- Conditions (“when does this call happen?”)
- Evidence

### 3) Storage/State Matrix (Postgres example required)
For each table/collection/bucket:
- Writes: who writes, when, why (include transaction boundaries if visible)
- Reads: who reads, when, why
- Migrations/constraints that shape behavior
- Idempotency/conflict patterns (upserts, unique constraints, retries)
- Evidence

### 4) External Dependency Matrix
For each third-party service:
- Purpose
- Inputs sent (payload shape)
- Outputs received (response shape)
- Retries/timeouts/circuit-breakers (if any)
- Failure modes + system behavior
- Evidence

### 5) Combination Matrix (inputs → outputs)
For each endpoint/RPC, enumerate:
- Valid variants (happy paths)
- Invalid/boundary variants (validation errors)
- Auth failures
- Not-found/empty cases
- Conflicts/idempotency cases
- Downstream failures (dependency returns error/timeout)
- DB errors/timeouts (if handled)
For each variant, document:
- Expected response output (status + schema)
- Side effects (DB/outbound/queue/file/log)
- Branch conditions (“if X then Y”)
- Evidence

### 6) Behavioral Spec for Rewrite (the blueprint)
A concise, implementation-agnostic spec derived from the matrices:
- What the system does
- What contracts must be preserved
- What side effects must be preserved
- What state transitions must be preserved
- What external dependencies must be re-implemented as adapters

This is the “build-to” document for the new language rewrite.

---

## Output Formats (so I can actually use this)

### Human-readable (required)
- Markdown docs for each deliverable
- Short, clear “why/when” paragraphs
- No walls of text

### Structured exports (required)
You must also emit:
- `io_matrix.json` containing:
  - endpoints/RPCs
  - schemas (field/type/required)
  - side effects
  - error variants
  - evidence pointers
- `call_graph.json` (nodes/edges)
- `storage_map.json` (tables + reads/writes + evidence)

No dangling references. Keep structure consistent across iterations.

---

## Step 0 — Repo Orientation (Iteration 0)
Before iteration 1, you will produce:
- How to run locally (commands/config assumptions)
- All entry points (server bootstrap, router registration, worker startup)
- Directory map: where routing, DB, clients, jobs live
- Initial endpoint list from static scan of route/RPC registration

### Exit criteria for Step 0
You can point to:
- the exact file(s) where routes/RPCs are registered
- the exact file(s) where DB/client initialization happens
- the exact file(s) where workers/cron jobs are registered (if present)

No Step 0 → no iteration 1.

---

## The Mandatory 25+ Iteration Loop (you must do at least 25 passes)
You will perform **at least 25 iterations**. Each iteration must update all living artifacts.

### Iteration N Template (N = 1..25+)
Each iteration must do ALL of the following:

#### 1) Pick a focus slice
Examples:
- one router/controller group
- one domain module
- one worker pipeline
- one DB table cluster
- one outbound client

#### 2) Extract I/O facts (with evidence)
- Add/update API Surface rows (external)
- Add edges to Internal Call Graph
- Add DB reads/writes to Storage Matrix
- Add dependency I/O to External Dependency Matrix
- Add cases to Combination Matrix

#### 3) Add “Why/When” narrative
For every newly mapped element, explain:
- why it exists (business/system purpose)
- when it triggers (conditions and upstream triggers)
- what it guarantees (invariants, constraints)

#### 4) Run a completeness check
Create/update an “Unknowns” list:
- missing schemas
- missing error shapes
- unclear side effects
- unclear branching conditions
Also create an “Evidence to find” checklist for the next iteration(s).

#### 5) Produce an Iteration Report (delta)
Must include:
- what changed since last iteration
- what remains unknown
- which files were examined (list)

### Exit criteria for each iteration
To count as a valid iteration, you must meet ALL:
- Add **≥ 10 new evidence-backed facts** OR close **1 major unknown area** (e.g., fully close an endpoint: schema + auth + errors + side effects + narrative).
- Every new fact includes evidence (path + symbol).
- The Unknowns list either shrinks or becomes more precise (no vague “TBD, probably”).

If you cannot meet this, you must widen the slice and continue until you do.

---

## Global Completion Rules (the “you are not done until 100%” gates)
You are not allowed to declare completion until ALL conditions A–E are satisfied.

### A) Endpoint Coverage = 100%
- Every externally reachable endpoint/RPC is in the API Surface Table.
- For each endpoint/RPC:
  - request schema is fully captured
  - response schema is fully captured
  - auth requirements are captured
  - error cases + shapes are captured
  - side effects are captured
  - “why/when” narrative exists
- No “TBD” fields remain.

### B) Side-Effect Closure = 100%
For every endpoint/worker/job:
- all DB writes identified (tables + fields when discoverable)
- all outbound calls identified (service + method/path)
- all queue publishes identified (topic names + payload shape)
- all file/object operations identified

### C) Storage Closure (Postgres example) = 100%
- every table referenced by the app has:
  - complete write map (who/when/why)
  - complete read map (who/when/why)
  - migration/constraint references
- no “mystery tables” in queries/models

### D) Combination Matrix Branch Completion
For each endpoint/RPC, you must cover at minimum:
- happy path
- auth failure
- validation failure
- not found
- conflict/idempotency
- downstream dependency failure
- DB failure/timeout (if handled)
Plus: **all code-visible branches** tied to input fields or dependency responses.

### E) Evidence Audit Pass
You must do a final audit pass that:
- samples at least 20 random rows across artifacts
- verifies evidence pointers are correct
- fixes any broken references immediately

### Hard exit criteria for “DONE”
You may only say “done” if:
- A–E are satisfied,
- the Unknowns list is empty,
- and you produce a rewrite blueprint that lists:
  - contracts (endpoints/RPC schemas)
  - modules to implement (behavioral grouping)
  - state transitions and DB behaviors
  - dependency adapters
  - required side effects and ordering constraints

If any gate fails, you must continue iterating.

---

## Special Questions You Must Answer for DB + Other Services
For every storage target or service integration, you must explicitly answer:
- What writes it? (exact code path)
- When does it write? (trigger + condition)
- Why does it write? (purpose)
- What reads it? (exact code path)
- What outputs does it feed next? (downstream impact)
- What inputs does it depend on besides the API server?  
  (workers, cron, queues, DB triggers, filesystem events, etc.)

---

## Disallowed Behaviors
- Don’t guess schemas.
- Don’t infer behavior without evidence.
- Don’t stop at “good enough.”
- Don’t map only happy paths.
- Don’t deliver a single big diagram and call it complete.

---

## How I Want You To Start (do this verbatim)
1) Perform Step 0 Repo Orientation now.
2) Output the initial endpoint list with evidence.
3) Begin Iteration 1 immediately.
4) Continue through Iteration 25.
5) If you reach Iteration 25 and any completion gate fails, keep iterating until all gates pass.

---

## Final Note (anti-early-stop)
Before you claim completion, you must prove:
- count of endpoints discovered vs. mapped (must match)
- count of tables referenced vs. mapped (must match)
- count of external dependencies discovered vs. mapped (must match)
Any mismatch means you are not done.
