---
name: codeconverter-05c-datastore-peers
description: Stage 05c of the codeconverter pipeline — for every table or store namespace the source service owns, map which other peers read or write it and by what route (direct database connection, shared library, or API call), so a store change can be judged safe or unsafe. Invoked by the codeconverter orchestrator, or when the user explicitly asks to run codeconverter stage 05c.
context: fork
argument-hint: [target-repo]
---

# Stage 05c — datastore-peers (fork)

**Goal:** A per-table map of who else touches the source service's data store, with
every peer touch classified as **direct-DB**, **shared-library**, or **API**. The
distinction is the entire point: peers isolated behind an API or a library survive a
store change; peers holding their own connection do not.

**Why this stage exists.** Stage 05a answers "who calls our endpoints" and stage 05b
answers "what do we call out to" — both are about the *wire*. Stage 09 hunts hidden
coupling repo-by-repo and does find direct database access, but it reports it as a
*risk to remediate*, per-consumer, not as a *per-table map* — so it cannot answer the
question a store swap actually poses: **for table X, is every toucher isolated?**

That question is not academic. When the target stack changes the persistence engine
(PostgreSQL → BoltDB, MySQL → DynamoDB, a shared schema → a private one), a peer with
its own connection does not fail at cutover with a clear error. It fails later, on the
first write that no longer arrives, or on the first read of a table that stopped being
updated. Stage 00's charter marks this stage **required** whenever the store changes
or the answer is unknown.

The output is the input to two decisions: whether the old store must keep being served
outbound during cutover (the same shape as the message-broker question), and which
peers must be remediated *before* the migration rather than during it.

## Setup

1. Target repo: use the argument passed by the orchestrator. If empty, find the repo
   containing `docs/codeconverter/STATE.md`; if none, stop and report. All paths are
   relative to the target repo root.
2. Read `docs/codeconverter/STATE.md` — it carries the **sibling repos path**, the
   **deployment manifests path**, and the service/database credentials in its
   environment notes. Without the sibling and manifest paths this stage cannot run;
   stop and report rather than scanning a partial corpus.
3. Read `docs/codeconverter/00-guidance/scope-charter.md` if it exists — Q4 names the
   store change and the user's *believed* peer list. That belief is a starting point
   and is explicitly not evidence.
4. Read `docs/codeconverter/02-codebase-analysis/storage_map.json` — the table
   inventory. Use its **verified** table list, not its legacy summary count; where the
   two disagree, re-derive from the migration SQL and show the command.
5. Read `docs/codeconverter/09-dependency-audit/bad-actors-analysis.md` if it exists —
   its Category 1 (direct database access) and Category 10 (shared infrastructure)
   findings are a cross-check, not a substitute. Every one of them must appear
   somewhere in this stage's map or be explained.
6. Read `.claude/skills/codeconverter/templates.md` and this skill's `instructions.md`.

## Execute

Follow `instructions.md`. Produce, in `docs/codeconverter/05c-datastore-peers/`:

- `datastore-peers.md` — the per-table map, the route classification, the
  isolation verdict per table, and the cross-check against stage 09.
- `datastore-peers.json` — the same data, machine-readable, one record per table,
  for stages 10, 11 and 12 to consume.
- `MANIFEST.md` — per the template.

**Every table gets a row, including tables with no external peer.** "No peer found"
is a real, reportable result and the most common one; an omitted table is
indistinguishable from an unexamined one.

Non-relational namespaces count as tables: Redis key prefixes, object-store bucket
prefixes, and message-bus queues that carry state all get rows if the source service
owns them.

## Uniform artifact contract (mandatory)

- Write only into `docs/codeconverter/05c-datastore-peers/`, plus your row in STATE.md.
- `datastore-peers.md` starts with the standard artifact header block; Status `final`
  when done. The JSON artifact is header-exempt (JSON has no comment syntax) —
  `datastore-peers.md` carries the header for both.
- Finish by writing `MANIFEST.md` in the stage dir per the template, exit criteria
  below copied in and honestly checked.
- The stage-complete commit belongs to the orchestrator.

## Exit criteria (copy into MANIFEST)

- [ ] Every table / store namespace the source service owns appears exactly once in
      `datastore-peers.json`. The table count is **re-derived from source** (migration
      SQL / schema DDL), the command that derived it is shown, and it matches the row
      count in the map. A count inherited from a prior artifact without re-derivation
      does not satisfy this.
- [ ] Every peer touch carries a **route** — `direct-db`, `shared-library`, or `api` —
      plus repo, `file:line`, a quoted snippet, and the access mode (`read`, `write`,
      or `read-write`). A touch with no route is not a finding.
- [ ] Every table carries an **isolation verdict**: `isolated` (no peer, or every peer
      via api/shared-library), `direct-access` (at least one direct-db peer), or
      `unknown` (the scan could not decide). `unknown` is permitted and must name what
      would resolve it.
- [ ] All three route classes were searched with the pattern list shown for each:
      direct connection strings/clients, shared-library/ORM-entity reuse, and API
      calls that reach the table indirectly.
- [ ] The **stage 09 cross-check** is present: every Category 1 and Category 10
      finding in `09-dependency-audit/bad-actors-analysis.md` is matched to a row in
      this map by table name, or has a written explanation for why it has no row.
      At least one direct-DB finding is reconciled explicitly, by ID.
- [ ] The **store-change verdict** section states, for the store change named in the
      scope charter, which tables block a clean cutover and which do not — and names
      the peers that would have to be remediated first.
- [ ] `datastore-peers.md` states plainly that "no peer found" means "not found in the
      repos and manifests we hold" — evidence, not proof — and names the peer classes
      that live outside every scannable repo.

## Tips from experience

- The dangerous peers are not services. On the IAM run the CRITICAL direct-database
  finding was a **Helm init container** running `psql` from a shell script mounted
  into a pod — no Java, no ORM, no service inventory would ever have listed it. Scan
  the deployment manifests and the shell scripts they mount with the same weight as
  the source repos.
- Search for the **table name as a bare string** across everything, including YAML,
  shell, SQL, and notebooks. An ORM-mediated peer is findable by entity class; a
  `psql -c "SELECT ... FROM account_aliases"` peer is findable only by the table name.
- A shared library is a route, not a safe harbour. Classify it `shared-library` and
  then record *what the library does* — a library that issues its own SQL against
  your schema is direct access wearing a jar file. The route that matters is whether
  the peer speaks to your store or to you.
- Read replicas, analytics mirrors and CDC pipelines are peers. They are usually
  configured, not coded, so they appear in Terraform, Helm values and DSN lists rather
  than in any repo's source.
- Credentials are the highest-signal search surface for direct access. Grep the
  deployment manifests for the database's secret name and see which pods mount it —
  every mount is a peer, whether or not you can find its query.
