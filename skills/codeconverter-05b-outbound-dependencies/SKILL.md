---
name: codeconverter-05b-outbound-dependencies
description: Stage 05b of the codeconverter pipeline — enumerate every outbound call the source service makes (HTTP, database, cache, message bus, object store, SMTP, licensing, billing) with file:line evidence, so the replacement knows what it must still be able to reach. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 05b.
context: fork
---

# Stage 05b — outbound-dependencies (fork)

**Goal:** The complete outbound side of the source service's I/O. Stage 05 documents
what calls *in*; stage 05a documents *who* calls in; this stage documents everything
the service itself calls *out*. On cutover day the replacement must be able to reach
every one of them, speaking the same protocol, with the same credentials, against
the same names.

**Why this stage exists.** "External dependencies" was previously split between
stage 02 (which sketched a storage map) and stage 09 (which hunts for *other*
services' hidden coupling to *this* one — the opposite direction). Neither produces
a checklist of "the replacement must still be able to call X". Left undefined, an
outbound audit degenerates into "it talks to some services"; a rewrite then ships
without an SMTP path or a licensing client and discovers it in staging. This stage
names the categories so nothing can be quietly skipped.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` — the **Environment notes** and **Service
   ports** sections name the infrastructure the service actually runs against, and
   are the calibration floor for this stage.
3. Read `docs/codeconverter/02-codebase-analysis/` (storage map, I/O matrix) and
   `docs/codeconverter/05-api-surface/API.md` section 5 (message bus) for the
   already-known pieces. Verify each against source; neither is authoritative here.
4. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/05b-outbound-dependencies/`:

- `outbound-dependencies.md` — findings by category, with the config keys, protocols,
  credentials and failure modes for each.
- `outbound-dependencies.json` — the same data, machine-readable, for stages 10 and
  11 to consume.
- `MANIFEST.md` — per the template.

Search all eight named categories plus an unclassified sweep. A category with
nothing in it gets a "none found" section showing the searches that prove it —
never an omitted section.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/05b-outbound-dependencies/`, plus your row in STATE.md.
- `outbound-dependencies.md` starts with the standard artifact header block; Status
  `final` when done. The JSON artifact is header-exempt (JSON has no comment syntax) —
  `outbound-dependencies.md` carries the header for both.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] All eight categories were searched — **HTTP clients, database, cache, message
      bus, object store, SMTP/email, licensing, billing** — plus the unclassified
      sweep; every category has a section, "none found" ones showing the searches
      that prove it.
- [ ] Every finding carries `file:line` evidence and a quoted snippet; no finding
      rests on a config file alone without the code that reads it, or on code alone
      without the config key that points it somewhere.
- [ ] Every cited `file:line` was **opened and checked** — the recorded snippet is
      really on that line. Cite a count of citations checked and citations failed. On
      the IAM run 4 of 83 line numbers were wrong on first pass, all of them in config
      files, so this is not a formality.
- [ ] Every finding records: protocol, target identity (host/DB/exchange/bucket/
      queue as applicable), the setting that points it there **and where that setting
      comes from** (properties file, config class, Helm value, K8s secret, env var,
      or hardcoded), the credential and its source, direction, whether it is on the
      synchronous request path, and what breaks if the replacement cannot reach it.
- [ ] The calibration check passed: every piece of infrastructure named in STATE.md's
      environment notes, the source repo's compose/deployment manifests, and the
      dependency manifests appears in the findings or has a written explanation for
      its absence.
- [ ] Message-bus findings name every exchange/topic/queue individually with its
      direction and routing keys — not "publishes events".
- [ ] Counts in `outbound-dependencies.json` match the counts in
      `outbound-dependencies.md`, and the check is shown.

## Tips from experience

- Start from the dependency manifest (`pom.xml`, `go.mod`, `package.json`,
  `requirements.txt`) and the deployment/compose files, not from grep. A client
  library on the dependency list that appears in no finding is either dead weight or
  a dependency you missed — resolve which, and say so.
- Search the *names* before the protocols. Outbound calls hide behind domain-named
  abstractions — IAM's SMTP path is a class called `SimpleEmailService`, and grepping
  every checked-in config file for `smtp` returns nothing at all. Find the
  abstraction, then read what transport it uses.
- Config keys and environment variables are the highest-yield search surface, but the
  key is often not in a checked-in file: it can be a constant in a config class, a
  getter on a Kubernetes config object, or a Helm value. Follow the accessor chain to
  whatever supplies the value — that chain is what the replacement's operators will
  have to reproduce.
- A service can be an outbound dependency of itself across modules — one module
  calling another over HTTP rather than in-process. Record those; they are the ones a
  rewrite most often accidentally turns into in-process calls, changing failure
  semantics.
- **Some targets are database rows, not config.** A federation/SSO service calls
  whatever identity provider each tenant has configured — a per-account URL stored in
  a table and set through the API. There is no config key and there never will be one,
  so `config_keys: []` here means something different from "hardcoded": the deployment
  requirement is an **egress policy** (outbound HTTPS to arbitrary public hosts, with
  the public CA trust store), not a hostname. A replacement deployed behind a
  restrictive egress policy passes every test and fails every real login. This is the
  finding most likely to be missed, because nothing in the repo names the target.
- **A dependency's failure mode is part of its contract.** Record fail-open vs
  fail-closed as a fact read from the code, not an assumption. IAM's licensing client
  replies `false` when the license server is unreachable — unreachable is treated as
  "not licensed" — and decides entitlement by testing whether the response body
  lower-cased contains both `"true"` and `"allowed"`. Either detail changed silently
  in a rewrite is a customer-visible incident.
