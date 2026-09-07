<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Stage note — read first.** Unlike stages 01–11, this stage was not adapted from
> the legacy "codeplanner" process; that process had no data-store peer phase. There
> is no phase-number translation table to apply and no journey.md/journal in this
> pipeline. Where this document conflicts with the stage's SKILL.md output contract
> (uniform headers, MANIFEST.md, output directory), **SKILL.md wins**.

---

# Stage 05c — Data-Store Peer-Access Map

## Mission

For every table the source service owns, name every other thing that touches it and
**how** it gets there.

Stage 05a maps who calls in. Stage 05b maps what we call out to. This stage maps who
reaches around the API entirely and touches the data.

The output answers one question per table: *if we change the store under this table,
does anything else break?* A peer behind an API or a shared library does not break —
it keeps calling the same method against a new backend. A peer with its own
connection string breaks, and it breaks quietly.

---

## The three routes (keep this in view while working)

| Route | What it means | What happens on a store change |
|---|---|---|
| `api` | The peer calls this service's HTTP/gRPC API; the service does the storage. | **Safe.** The wire contract is unchanged; the backend swap is invisible. |
| `shared-library` | The peer links a library that mediates access — an ORM entity set, a client SDK, a DAO jar published by this service. | **Conditionally safe.** Safe if the library talks to the *service*. Unsafe if the library issues its own SQL against the schema — that is direct access wearing a jar file. Record which. |
| `direct-db` | The peer opens its own connection: JDBC/DSN, `psql`, a migration runner, an init container, a BI tool, a CDC connector. | **Unsafe.** Nothing about a store swap is visible to it until data stops arriving. |

A fourth value, `unknown`, is permitted per touch when the scan finds the peer but
cannot decide its route. `unknown` is treated as `direct-db` for the purposes of the
store-change verdict — because the cost of guessing wrong in that direction is
recoverable and the cost of guessing wrong in the other is not.

---

## Step 0 — Readiness

```bash
# The table inventory this stage maps — and its provenance
python3 -c "
import json;d=json.load(open('docs/codeconverter/02-codebase-analysis/storage_map.json'))
print('legacy summary count:', d.get('summary',{}).get('total_tables'))
print('verified count      :', len(d.get('verified_tables',[])))"

# Re-derive the table list from source. Never trust the inherited count.
grep -rhoiE 'CREATE TABLE( IF NOT EXISTS)? +[\"a-z0-9_.]+' --include=*.sql . \
  | awk '{print tolower($NF)}' | tr -d '"' | sed 's/.*\.//' | sort -u | wc -l

# The scan corpus
ls <sibling-repos-path> | wc -l
ls <deployment-manifests-path>

# What stage 09 already found (cross-check, not substitute)
grep -n 'Category 1 \|Category 10 \|Direct Database' docs/codeconverter/09-dependency-audit/bad-actors-analysis.md
```

Record all of this in a "Readiness" section of `datastore-peers.md`, including the
re-derived table count and the command that produced it. If the re-derived count
disagrees with the inherited one, **the re-derived count wins** and the disagreement
is written down with both numbers and both methods.

---

## Step 1 — Build the table inventory

The unit of this map is the **store namespace**, which is a table for a relational
store and its equivalent elsewhere:

| Store | Namespace unit |
|---|---|
| SQL | table |
| Redis | key prefix (`session:`, `token:`, `lock:`) |
| Object store | bucket + key prefix |
| Document store | collection |
| Message bus | queue/exchange, **only** where it carries durable state rather than transient events (transient ones belong to 05b) |

Build the list from source, not from the prior stage's summary:

```bash
# SQL: distinct CREATE TABLE across every migration
grep -rhoiE 'create table( if not exists)? +[`"a-z0-9_.]+' --include=*.sql . \
  | awk '{print tolower($NF)}' | tr -d '`"' | sed 's/.*\.//' | sort -u > /tmp/tables.txt
wc -l < /tmp/tables.txt

# Redis: key prefixes actually written by the source
grep -rhoE '"[a-z_]+:"' --include=*.java --include=*.go --include=*.js . | sort | uniq -c | sort -rn | head -40
```

Cross-check the derived list against the prior stage's verified list in both
directions — tables in one and not the other are the interesting rows, and each one
needs a resolution (renamed, dropped by a later migration, view rather than table,
or genuinely missed).

## Step 2 — Build the scan corpus

```bash
SIBLINGS=<sibling-repos-path>
MANIFESTS=<deployment-manifests-path>

ls -d $SIBLINGS/*/ | wc -l          # how many repos are in scope
du -sh $SIBLINGS $MANIFESTS         # how big the corpus is
```

The corpus is **every sibling repo plus every deployment manifest tree**, and the
manifests carry equal weight. The single most damaging peer class — scripts mounted
into init containers — exists only in the manifest tree and appears in no repo's
source at all.

Record the corpus (repo count, manifest paths, total size) in the output. A scan whose
corpus is not stated cannot be reproduced or trusted.

## Step 3 — Pass 1: direct-DB peers by table name

The highest-yield pass, and the one that catches the peers no service inventory lists.

```bash
# Every table name, as a bare string, across every file type
while read -r t; do
  hits=$(grep -rl --binary-files=without-match -w "$t" $SIBLINGS $MANIFESTS 2>/dev/null \
         | grep -v '^'"$SOURCE_REPO" | head -20)
  [ -n "$hits" ] && printf '### %s\n%s\n' "$t" "$hits"
done < /tmp/tables.txt
```

Then the shapes that carry SQL without carrying an ORM:

```bash
# Raw SQL in shell, YAML, and config
grep -rniE 'select .* from |insert into |update .* set |delete from ' \
  --include=*.sh --include=*.yaml --include=*.yml --include=*.tpl --include=*.sql \
  $SIBLINGS $MANIFESTS | head -60

# Command-line database clients
grep -rniE '\b(psql|mysql|mongosh|redis-cli|pg_dump|pgloader|sqlcmd)\b' $SIBLINGS $MANIFESTS | head -60

# Connection strings and DSNs pointing at the source service's database
grep -rniE 'jdbc:|postgres(ql)?://|mysql://|mongodb(\+srv)?://|redis://' $SIBLINGS $MANIFESTS | head -60
```

And the credential pass, which finds peers whose query you cannot see:

```bash
# What is the database secret called, and which pods mount it?
grep -rn '<db-secret-name>' $MANIFESTS | head -40
grep -rn -B5 -A15 '<db-secret-name>' $MANIFESTS | grep -iE 'kind:|name:|containers:|image:' | head -60
```

Every mount of the store's credential is a peer, whether or not its SQL is findable.
Record it, and if the query is genuinely not visible, record the route as `direct-db`
with access mode `unknown` rather than dropping the finding.

## Step 4 — Pass 2: shared-library peers

```bash
# Who depends on a library published by the source service?
grep -rn '<source-org-groupid>' $SIBLINGS --include=pom.xml --include=build.gradle \
  --include=go.mod --include=package.json | head -60

# ORM entity classes / models named after the tables
while read -r t; do
  camel=$(echo "$t" | sed -E 's/(^|_)([a-z])/\U\2/g')
  grep -rln --include=*.java --include=*.go --include=*.ts --include=*.py "\b$camel\b" $SIBLINGS 2>/dev/null | head -5
done < /tmp/tables.txt

# Migration tooling pointed at the same schema — a peer that writes DDL is the
# highest-consequence shared-library peer there is
grep -rln --include=*.xml --include=*.yaml -iE 'flyway|liquibase|goose|alembic|migrate' $SIBLINGS | head -30
```

For every shared-library peer found, answer the follow-up that decides its route:
**does the library talk to the service, or to the store?** A library that opens a
connection is `direct-db` regardless of how it is packaged. Record the evidence for
that determination — the class or function that opens the connection, or the client
call that does not.

## Step 5 — Pass 3: API peers (the safe ones, which must still be listed)

```bash
# Peers already known to reach the data through the API
python3 -c "
import json,collections
d=json.load(open('docs/codeconverter/05a-endpoint-consumers/endpoint-consumers.json'))
# emit repo -> endpoints, so each can be attributed to the tables those endpoints touch
" 2>/dev/null || echo "05a not present — API peers must be derived from source"
```

Map each endpoint's callers onto the tables that endpoint reads or writes, using
`02-codebase-analysis/io_matrix.json` (endpoint → storage) joined with
`05a-endpoint-consumers/endpoint-consumers.json` (endpoint → caller). Where 05a has
not run, derive API peers directly and say so.

API peers are listed for one reason: **the isolation verdict is only meaningful if the
denominator is complete.** A table with three API peers and no direct peers is a table
that is safe *and known to be exercised* — a different fact from a table nothing
touches at all.

## Step 6 — Classify and verify every touch

For each candidate, open the file and confirm the touch before recording it. A
grep hit is a candidate; a read line is a finding.

Record per touch:

| Field | Values |
|---|---|
| `peer` | repo or component name |
| `route` | `direct-db` \| `shared-library` \| `api` \| `unknown` |
| `mode` | `read` \| `write` \| `read-write` \| `ddl` \| `unknown` |
| `evidence` | `file:line` + quoted snippet |
| `mechanism` | how it connects: JDBC URL, mounted secret, SDK call, library class |
| `on_cutover` | what breaks if the store changes under it |

Then the per-table verdict:

- `isolated` — no peers, or every peer is `api`, or `shared-library` where the library
  proven to call the service rather than the store.
- `direct-access` — at least one `direct-db` or `unknown` peer.
- `unknown` — the scan could not decide; name what would resolve it.

## Step 7 — Cross-check against stage 09

This is a required step, not a courtesy. Stage 09 found direct database access
repo-first; this stage found it table-first. The two must reconcile.

```bash
# Pull stage 09's Category 1 and Category 10 finding IDs
grep -nE '^#### BA-[0-9]+' -A6 docs/codeconverter/09-dependency-audit/bad-actors-analysis.md \
  | grep -B1 -iE 'Direct Database|Shared infrastructure' | head -40
```

For each such finding, state in a table: finding ID → the table(s) it touches → the
row in this map that carries it → agree / disagree. A stage 09 finding with no row
here is either a miss in this stage or a table this stage did not know it owned;
resolve which, and write the resolution down.

Reconcile at least one direct-DB finding explicitly and by ID, showing the evidence
both stages hold. If they disagree on the facts — different file, different table,
different severity — the disagreement is the finding, and it goes to stage 12.

## Step 8 — The store-change verdict

The section stage 00, 10, 11 and 12 all read. For the store change named in the scope
charter:

```markdown
## Store-change verdict

**Change:** <from> → <to>, per 00-guidance/scope-charter.md Q4

**Tables that block a clean cutover** (N of M):
| Table | Peer | Route | What breaks | Remediation | Must happen before |
|---|---|---|---|---|---|

**Tables that do not block** (M−N of M): every peer is `api` or a service-mediated
library. Listed in full in the per-table map; not repeated here.

**If remediation is not possible before cutover:** the old store must keep being
served for the blocking tables above, by <mechanism>, until <condition>. This is the
same shape as the message-broker question and belongs on stage 12's agenda.
```

The last paragraph is the one that matters. "Keep serving the old store outbound
during cutover" is a real and often correct answer; what is not acceptable is
discovering on cutover day that it was needed.

---

## Step 9 — Write the outputs

### `datastore-peers.md`

```markdown
<!-- codeconverter artifact -->
**Stage:** 05c-datastore-peers
**Artifact:** datastore-peers.md — who else touches each table, and by what route
**Status:** final
**Produced by:** codeconverter-05c-datastore-peers on YYYY-MM-DD
**Inputs:** <storage map, sibling repos, deployment manifests, 09 audit, scope charter>

---

# <Service> — Data-Store Peer-Access Map

## Readiness
<table count re-derived + command; corpus; what prior stages believed>

## What "no peer found" means
<evidence, not proof; the peer classes outside every scannable repo>

## Summary
| Metric | Count |
|---|---|
| Tables / namespaces | |
| Tables with no external peer | |
| Tables with only `api` peers | |
| Tables with a `shared-library` peer | |
| Tables with a `direct-db` peer | |
| Tables `unknown` | |

## Search passes
<the pattern list actually run for each of the three passes>

## Per-table map
### <table>
- **Verdict:** isolated | direct-access | unknown
- **Peers:** <table of peer / route / mode / evidence / mechanism / on_cutover>
  (or "no external peer found — searched: <patterns>")

## Peers by repo
<the inverse index: for each peer, every table it touches and its worst route>

## Stage 09 cross-check
| BA finding | Table(s) | Row here | Agree? | Note |

## Store-change verdict
<per Step 8>

## Open questions for stage 12
```

### `datastore-peers.json`

```json
{
  "generated": "YYYY-MM-DD",
  "table_count": 0,
  "table_count_method": "<exact command>",
  "corpus": {"sibling_repos": 0, "manifest_paths": []},
  "tables": [
    {
      "name": "account_aliases",
      "store": "postgresql:iamdb",
      "owner_writes": true,
      "verdict": "direct-access",
      "peers": [
        {
          "peer": "connector-statistics-server",
          "route": "direct-db",
          "mode": "read",
          "evidence": "charts/apps/services/statistics/files/org_lookup.sh:30",
          "snippet": "psql ... -c \"SELECT account_id, name FROM account_aliases\"",
          "mechanism": "mounted secret iam.postgres.iam-user; shared-postgres-proxy:5432",
          "on_cutover": "init container fails; main container never starts",
          "cross_check": "09:BA-001"
        }
      ]
    }
  ],
  "summary": {"no_peer": 0, "api_only": 0, "shared_library": 0, "direct_db": 0, "unknown": 0},
  "blocking_tables": [],
  "open_questions": []
}
```

---

## Verification before you declare done

```bash
J=docs/codeconverter/05c-datastore-peers/datastore-peers.json

# Table count re-derived == rows in the map
python3 -c "
import json;d=json.load(open('$J'))
print('rows:',len(d['tables']),'declared:',d['table_count'],
      'OK' if len(d['tables'])==d['table_count'] else 'MISMATCH')
print('method:',d['table_count_method'])"

# Every table has a verdict; every peer has a route and evidence
python3 -c "
import json;d=json.load(open('$J'))
nov=[t['name'] for t in d['tables'] if t.get('verdict') not in ('isolated','direct-access','unknown')]
bad=[(t['name'],p.get('peer')) for t in d['tables'] for p in t.get('peers',[])
     if p.get('route') not in ('direct-db','shared-library','api','unknown') or not p.get('evidence')]
print('tables without a valid verdict:',nov or 'none')
print('peer touches without route/evidence:',bad or 'none')"

# Verdict is consistent with the peer routes
python3 -c "
import json;d=json.load(open('$J'))
bad=[]
for t in d['tables']:
    routes={p['route'] for p in t.get('peers',[])}
    want='direct-access' if (routes & {'direct-db','unknown'}) else 'isolated'
    if t['verdict']!='unknown' and t['verdict']!=want: bad.append((t['name'],t['verdict'],sorted(routes)))
print('verdict/route inconsistencies:',bad or 'none')"

# Summary counts match the rows
python3 -c "
import json;d=json.load(open('$J'))
s=d['summary']; tot=sum(s.values())
print('summary total:',tot,'rows:',len(d['tables']),'OK' if tot==len(d['tables']) else 'MISMATCH')"

# Markdown and JSON agree on the table count.
# Scope the count to the per-table map — other sections legitimately use `###`
# (the stage 09 cross-check does), so a bare `grep -c '^### '` over-counts.
awk '/^## Per-table map/{f=1;next} /^## Peers by repo/{f=0} f&&/^### /{c++} END{print c}' \
  docs/codeconverter/05c-datastore-peers/datastore-peers.md

# Every stage 09 direct-DB finding is reconciled — list the IDs, do not just count them
grep -o 'BA-[0-9]*' docs/codeconverter/05c-datastore-peers/datastore-peers.md | sort -u
```

Paste this output into the MANIFEST. Counts that were checked but not shown are the
exact failure mode this pipeline has already had once.

---

## Exit Criteria

Copy the exit criteria from `SKILL.md` into `MANIFEST.md` and check them honestly.
The store-change verdict is acted on by stages 10, 11 and 12 without re-derivation —
an unchecked criterion here is a silent cutover failure later.
