<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Stage note — read first.** Unlike stages 01–11, this stage was not adapted from
> the legacy "codeplanner" process; that process had no consumer-mapping phase, and
> its absence is the reason this stage exists. There is no phase-number translation
> table to apply and no journey.md/journal in this pipeline. Where this document
> conflicts with the stage's SKILL.md output contract (uniform headers, MANIFEST.md,
> output directory), **SKILL.md wins**.

---

# Stage 05a — Endpoint Consumer Map

## Mission

For every endpoint in `docs/codeconverter/05-api-surface/API.md`, answer one
question with evidence: **who calls this?**

Two answers are acceptable. "These call sites call it, here they are, `file:line`."
Or "no caller was found in the repos we hold, and here are the searches that failed
to find one." Both are results. Only silence is a failure — an endpoint that does
not appear in the output is a bug in this stage, not an endpoint without callers.

You are not deciding whether an endpoint should survive the rewrite. Stage 10 does
that. You are producing the evidence it decides on.

---

## Why this stage exists (keep this in view while working)

On the first IAM run, a proposal to drop 84 API-key endpoints was justified by "zero
sibling services call `/v3/api-keys`". The claim was true and the conclusion was
wrong:

| Group | Endpoints | Who actually calls them |
|---|---|---|
| Base `/v3/api-keys...` | 33 | frontends, customer scripts |
| Aggregator `/v3/accounts/{id}/api-keys...` | 32 | distributors managing sub-accounts |
| Legacy `/v1/...` | 13 | old customer integrations still in the field |
| Admin `/admin/v3/...` | 6 | Izuma support tooling |

None of the last three groups has a caller inside any sibling *service* repo, so a
sibling-service scan reports zero and looks conclusive. Stage 09 does not cover the
gap: it hunts *hidden* coupling — direct database access, wire formats, deployment
artifacts — and it was right to classify `cloud-portal` and `admin-console` as
"frontend REST consumer, no hidden coupling". Nobody's job was "who calls endpoint
X". It is yours.

Two rules follow, and they are the whole point of the stage:

1. **Variants are not their base path.** A caller of `/v3/api-keys` is not a caller
   of `/v3/accounts/{account_id}/api-keys`. Never let a substring match launder one
   into the other.
2. **"Not found" is not "not called."** Say so in the artifact, in those words.

---

## Step 0 — Readiness

```bash
# The contract this stage maps
wc -l docs/codeconverter/05-api-surface/API.md

# Scan roots recorded by stage 01
grep -E "Sibling repos path|Deployment manifests path" docs/codeconverter/STATE.md

# Sibling repos are present and countable
ls {SIBLING_REPOS_PATH} | wc -l

# Known consumers from stage 03, as a starting reference (not a substitute)
grep -c "^|" docs/codeconverter/03-dependency-discovery/references.md
```

If `05-api-surface/API.md` is missing or its MANIFEST is not `complete`, stop and
report — this stage has nothing to map. Record the readiness output in a "Readiness"
section of `endpoint-consumers.md`.

---

## Step 1 — Build the endpoint inventory

Parse `05-api-surface/API.md` into a flat working inventory. Do not retype it by
hand and do not sample it — parse every row.

```bash
# API.md endpoint rows look like:
# | `GET` | `/v3/accounts/{account_id}/api-keys` | admin | `iam-identity` | `rest/X.java:120` | ... |
grep -oE '^\| `(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)` \| `[^`]+`' \
  docs/codeconverter/05-api-surface/API.md \
  | sed -E 's/^\| `([A-Z]+)` \| `(.*)`$/\1 \2/' \
  > /tmp/05a_endpoints.txt

wc -l /tmp/05a_endpoints.txt   # must equal API.md's stated total
```

Adapt the regex to the actual table shape if it differs — the requirement is the
count, not the one-liner. Compare against the **Total** in API.md's summary table.
If they disagree, fix your parse before doing anything else; every later count in
this stage is derived from this inventory.

For each endpoint keep: method, path, the audience section it appeared under
(external / admin / aggregator / legacy / internal / operational), module, and
handler `file:line`. Carrying the audience through is what makes the
no-known-caller ledger readable — "12 of 47 internal endpoints have no caller" is
actionable, a flat list of 637 rows is not.

### Derive a match key per endpoint

For each path, compute:

- **Static prefix** — the path up to the first `{param}`.
  `/v3/accounts/{account_id}/api-keys/{apikey_id}` → `/v3/accounts/`
- **Static tail segments** — the literal segments after the last `{param}`:
  `api-keys`, and any trailing literal like `/reset` or `/status`.
- **Shape regex** — the full path with each `{param}` replaced by a
  single-segment wildcard: `/v3/accounts/[^/]+/api-keys/[^/]+`.
- **Distinctive token** — the rarest literal segment, used only to *narrow* a search,
  never to *conclude* one.

The shape regex is the thing you match against; the tokens are how you find
candidates cheaply.

---

## Step 2 — Build the scan corpus

Scan roots, all of them, every run:

| Root | What it holds | Typical caller kinds |
|---|---|---|
| `{SIBLING_REPOS_PATH}` | the service fleet | sibling services, frontends, CLIs, operator tools |
| the source repo's own test dirs | integration/system tests | test suites (evidence the endpoint is exercised, not that a product needs it) |
| the test/support repos named in STATE.md | system-test suites, shared client libraries | test suites, generated SDKs |
| `{DEPLOYMENT_MANIFESTS_PATH}` | Helm/K8s/Terraform | gateway routes, probes, ingress rules |

Inventory the corpus before searching, so the artifact can state what was and was not
covered:

```bash
ls -d {SIBLING_REPOS_PATH}/*/ | wc -l
# language mix drives which client idioms to search for
for d in {SIBLING_REPOS_PATH}/*/; do
  printf '%s ' "$(basename "$d")"
  find "$d" -maxdepth 2 \( -name '*.go' -o -name '*.java' -o -name '*.ts' \
       -o -name '*.js' -o -name '*.py' -o -name '*.rb' \) -print -quit | head -1
  echo
done
```

Note any root you could not read (submodule not checked out, permissions). An
unreadable root is a hole in the evidence and belongs in the artifact, not in your
head.

---

## Step 3 — Pass 1: literal static path

The cheap pass. Find call sites that contain the path as written.

```bash
# Longest static prefix, across every text file in the corpus
grep -rn --binary-files=without-match "/v3/accounts/" {SIBLING_REPOS_PATH} \
  --include='*.go' --include='*.java' --include='*.ts' --include='*.js' \
  --include='*.py' --include='*.rb' --include='*.sh' --include='*.yaml' \
  --include='*.yml' --include='*.json' --include='*.md' --include='*.tf'

# Distinctive tail segment, to narrow within the prefix hits
grep -rn "api-keys" {SIBLING_REPOS_PATH} --include='*.ts' --include='*.js'
```

Then filter each candidate line against the endpoint's **shape regex**, not against
the token. This is the step that separates `/v3/api-keys` from
`/v3/accounts/{id}/api-keys`.

Also match on the HTTP method. A call site that hits `/v3/api-keys` with `POST` is
evidence for `POST /v3/api-keys` and not for `GET /v3/api-keys`. When the method is
not visible on the same line, read the surrounding function before assigning it; if
it is genuinely indeterminate, record the caller against every method registered on
that path and mark the match `inferred`.

---

## Step 4 — Pass 2: templated and dynamically constructed paths

Most real call sites never contain the whole path. They build it. Search for the
construction idioms of each language present in the corpus:

```bash
# JS/TS template literals and concatenation
grep -rnE '`[^`]*/v3/accounts/\$\{[^}]+\}/api-keys' {SIBLING_REPOS_PATH} --include='*.ts' --include='*.js'
grep -rnE "'/v3/accounts/' *\+|\"/v3/accounts/\" *\+" {SIBLING_REPOS_PATH}

# Go
grep -rnE 'fmt\.Sprintf\("[^"]*/v3/[^"]*"' {SIBLING_REPOS_PATH} --include='*.go'
grep -rnE 'path\.Join\([^)]*"v3"' {SIBLING_REPOS_PATH} --include='*.go'

# Java
grep -rnE 'String\.format\("[^"]*/v3/|UriBuilder|WebTarget|\.path\("' {SIBLING_REPOS_PATH} --include='*.java'

# Python
grep -rnE 'f"[^"]*/v3/|%s.*v3|\.format\([^)]*v3' {SIBLING_REPOS_PATH} --include='*.py'

# Shell / curl / Postman-style fixtures
grep -rnE 'curl .*(/v3/|/admin/v3/|/auth/|/internal/)' {SIBLING_REPOS_PATH}
```

Two things to do with the hits:

- **Resolve the variable when you can.** If the code is
  `` `${base}/v3/accounts/${accountId}/api-keys` ``, read what `base` is. If `base`
  already ends in `/v3`, the real path is different from what it looks like.
- **When you cannot resolve it, spread it.** A helper that takes a path fragment as
  an argument reaches every endpoint its callers can name. Record the match against
  each reachable endpoint with `"match": "inferred"` and an `inferred_reason` saying
  why. Do not silently pick the most likely one.

---

## Step 5 — Pass 3: client libraries, config, and deployment

The pass that catches frontends and operator tooling, which almost never contain a
literal path.

**a. Generated SDKs and hand-written client wrappers.** Find the wrapper, map its
methods to paths, then count call sites of the methods:

```bash
# OpenAPI/Swagger-generated clients and their specs
find {SIBLING_REPOS_PATH} -iname '*swagger*' -o -iname '*openapi*' -o -iname '*.raml' | head -50
grep -rln "operationId" {SIBLING_REPOS_PATH} --include='*.yaml' --include='*.json'

# Hand-written wrappers named after the service
grep -rln "ApiKeysApi\|AccountsApi\|IamClient\|iam-client\|authClient" {SIBLING_REPOS_PATH}
```

For every wrapper method that maps to an endpoint, its call sites are that
endpoint's callers. Record the wrapper file:line **and** at least one call site of
it; a wrapper with zero call sites is not a caller.

**b. Config, environment, and route tables.** Gateway route definitions, ingress
rules, nginx/OpenResty location blocks and API-gateway specs name paths explicitly:

```bash
grep -rnE '(location|path|route|uri|prefix)\s*[:=].*(/v3/|/admin/v3/|/auth/|/internal/)' \
  {DEPLOYMENT_MANIFESTS_PATH} {SIBLING_REPOS_PATH} \
  --include='*.yaml' --include='*.yml' --include='*.conf' --include='*.lua' --include='*.tf'
```

A gateway route is a caller of a weaker kind — it proves the path is *exposed*, not
that anyone walks through it. Record it with `kind: "gateway-route"` and say so.

**c. Documentation and fixtures.** API docs, Postman collections and recorded
HTTP fixtures (`*.har`, VCR cassettes, WireMock stubs) are evidence that someone
once called it:

```bash
grep -rln "postman_collection\|\.har\"\|wiremock\|vcr_cassettes" {SIBLING_REPOS_PATH}
```

Record these with their own kind. They are the weakest evidence class and must never
be the sole basis for calling an endpoint "used" without a note saying so.

---

## Step 6 — Classify every caller

| `kind` | Meaning | Weight |
|---|---|---|
| `sibling-service` | another backend service calls it at runtime | strongest |
| `frontend` | a UI (`cloud-portal`, `admin-console`, …) calls it, directly or via SDK | strong |
| `cli-tool` / `operator-tool` | support or ops tooling | strong |
| `sdk-client` | a generated or shared client library exposes it, with call sites | strong |
| `test-suite` | an integration/system test exercises it | medium — proves the behavior is pinned, not that a product needs it |
| `gateway-route` | a route/ingress entry exposes the path | medium — exposure, not traffic |
| `doc-or-fixture` | documented, stubbed, or recorded | weak |
| `traffic-log` | a captured request log shows the path was actually served | strongest of all — but names no caller, and a single capture window proves nothing by its absence |
| `policy-document` | another repo ships an authorization policy naming the path in a `resource` field | medium — not a call, but the path string is stored wire format: renaming it changes what existing policies authorise, which belongs in stage 09 |

The last two are not hypothetical. On the IAM run the corpus contained a captured
service log (`izuma-dm-platform/logs/mbed_auth.log`) and seven repos shipping IAM
policy statements with `'resource': '/v3/policy-groups'` in them. Classifying either
as `doc-or-fixture` would have thrown away the strongest evidence in the corpus and a
real piece of hidden coupling respectively.

Every caller record needs all of: `repo`, `file:line`, a quoted snippet, `kind`, and
`match` (`exact` | `inferred`). No exceptions, no "see source", no "several places in
cloud-portal".

---

## Step 6a — Two ledgers, not one

Build `no_known_caller` (empty caller list) **and** `no_product_caller` (no caller of
kind `sibling-service`, `frontend`, `operator-tool`, `sdk-client` or `traffic-log`).

Where the service sits behind an API gateway, the gateway's route table names nearly
every endpoint it declares, whether or not a client exists. On the IAM run that is the
difference between 66 endpoints and 435. Report both, and say plainly in the artifact
that a drop decision reads the second one: an endpoint whose only evidence is "the
gateway would route it" is no better attested than one with no evidence at all.

---

## Step 7 — The no-known-caller ledger

This is the deliverable that changes decisions. Build it last, from the endpoints
with an empty caller list.

For each such endpoint record the searches that were actually run against it — the
literal prefix, the shape regex, the tokens, the wrapper names. An empty caller list
with no `searches_run` is not a finding, it is an omission.

Group the ledger by audience and report a table:

| Audience | Endpoints | With callers | No known caller |
|---|---:|---:|---:|
| External — customer-facing | | | |
| Admin — root only | | | |
| Aggregator — multi-tenant | | | |
| Legacy vN | | | |
| Internal cross-service | | | |
| Operational | | | |

Then the ledger itself, endpoint by endpoint.

### The statement that must appear verbatim

`endpoint-consumers.md` must contain this, in a section of its own, near the top and
again at the head of the ledger:

> **"No known caller" means "not found in the repos we hold." That is evidence, not
> proof.** Whole classes of consumer live outside every repo this scan can reach:
> customer applications and scripts built against the public API, distributor and
> partner integrations against aggregator endpoints, operator and support tooling
> kept outside the fleet, legacy integrations still in the field on old API
> versions, and anything reached by hand with `curl` or Postman. An endpoint in this
> ledger is a *candidate* for a drop or deprecate decision, and the input that
> decision still needs is production traffic data, which no repository scan can
> supply.

Adapt the consumer classes to the service at hand; keep the first sentence exactly.

---

## Step 8 — Write the outputs

### `endpoint-consumers.md`

```markdown
<standard artifact header block>

# <Service> — Endpoint Consumer Map

## Readiness
<Step 0 output: endpoint count, scan roots, repo count, any unreadable root>

## What "no known caller" means
<the verbatim statement from Step 7>

## Summary

| Measure | Value |
|---|---:|
| Endpoints in 05-api-surface/API.md | N |
| Endpoints mapped here | N |
| Delta (must be 0) | 0 |
| Endpoints with >=1 caller | N |
| Endpoints with no known caller | N |
| Distinct calling repos | N |
| Caller records | N |

<the by-audience table from Step 7>

## Scan corpus
<every root scanned, repo count, languages, anything skipped and why>

## Search passes
<the exact patterns used in passes 1, 2 and 3 — reproducible>

## Callers by endpoint
### <audience section, matching API.md's sections>
| Method | Path | Callers | Kinds | Evidence |
|---|---|---:|---|---|
| `GET` | `/v3/api-keys` | 3 | frontend, sdk-client | `cloud-portal/source/api/keys.ts:88`; ... |

## Callers by repo
<the inverse view: per repo, which endpoints it calls — this is what a consumer
 owner reads when asked "what breaks for me">

## No known caller — ledger
<by audience; each entry with the searches that were run>

## Ambiguous / inferred matches
<every `inferred` match, with why it could not be resolved to one endpoint>
```

### `endpoint-consumers.json`

One record per endpoint, in the same order as API.md:

```json
{
  "stage": "05a-endpoint-consumers",
  "generated": "YYYY-MM-DD",
  "source_contract": "docs/codeconverter/05-api-surface/API.md",
  "endpoint_count": 637,
  "scan_roots": [
    {"path": "/abs/path/CLOUD", "repos": 39, "role": "sibling repos"},
    {"path": "/abs/path/IAM", "repos": 8, "role": "test and support repos"}
  ],
  "endpoints": [
    {
      "method": "GET",
      "path": "/v3/accounts/{account_id}/api-keys",
      "audience": "aggregator",
      "module": "iam-identity",
      "handler": "rest/AggregatorApiKeysResource.java:120",
      "callers": [
        {
          "repo": "cloud-portal",
          "kind": "frontend",
          "file": "source/api/apiKeys.ts",
          "line": 88,
          "snippet": "return http.get(`/v3/accounts/${accountId}/api-keys`)",
          "match": "exact"
        }
      ],
      "caller_count": 1,
      "no_known_caller": false,
      "searches_run": [
        "grep -rn '/v3/accounts/' <roots>",
        "shape regex /v3/accounts/[^/]+/api-keys",
        "wrapper scan: ApiKeysApi|iam-client"
      ]
    }
  ],
  "summary": {
    "with_callers": 0,
    "no_known_caller": 0,
    "by_audience": {},
    "by_repo": {},
    "inferred_matches": 0
  }
}
```

Rules for the JSON:

- Every endpoint from API.md appears exactly once, **including** the zero-caller ones
  (`"callers": []`, `"no_known_caller": true`, non-empty `searches_run`).
- `caller_count` equals `len(callers)`; `summary` totals equal the per-record totals.
- Duplicate method+path pairs in API.md (different handler classes registering the
  same route) stay as separate records, keyed by handler — same as API.md does.

Generating this with a small script kept beside the artifact is fine and encouraged;
stage 05 set that precedent with `extract_endpoints.py`. A script that produced a
committed artifact belongs in the stage dir.

---

## Four ways a path match lies, and the guard for each

Every one of these was found on the IAM run by opening a cited `file:line` and reading
it. Together they moved the caller-record count from 17,246 to 5,535 without losing a
single endpoint's coverage. Build the guards in from the start.

| The lie | What it looked like | Guard |
|---|---|---|
| **Substring laundering** | the filesystem path `iam/policies` matched `/v3/accounts/{account_id}/policies` by aligning `iam` with `{account_id}` — 923 records | a partial path is evidence only if its **first** segment lands on a *literal* endpoint segment, never on a `{param}` |
| **Prose** | `bind users/groups to the ClusterRoles` in a CHANGELOG matched `/v3/users/{user_id}` | a partial path must be preceded by a quote, backtick, interpolation, slash or `=`; a space means it is a sentence |
| **A mock of the service** | `connector-ca/mock/iam.js:23` is `app.post('/internal/v1/trusted-certificates', ...)` — an *implementation*, scored as `sibling-service`; 40 records across four mock IAM servers | treat `mock`/`stub`/`fake`/`wiremock` path segments as test paths. This is the false positive that matters, because it suppresses a true negative |
| **A lone generic noun** | Kubernetes' own OIDC test `"{{.URL}}/groups"` matched every endpoint ending in `/groups` — 253 records from one unrelated repo | a one-segment tail counts only when hyphenated or distinctive to this API |
| **Route-table method blur** | a `±7`-line context window over a route table with four lines per route credited a DELETE route to its GET neighbour | when a `http-method:`-style key sits within three lines below the path, it is the only answer |

And two ways a match is missed:

| The miss | What it looked like | Fix |
|---|---|---|
| **Constructed base** | `f'{SERVICES["ROOT"]}/v3/api-keys'` is one segment longer than the endpoint and matched nothing | also match base-trimmed variants, trimming only wildcard- or hostname-shaped leading segments |
| **Percent-escapes** | `/v3/accounts/%2F/users/root1/password` was chopped at the `%` and credited to `POST /v3/accounts` | admit `%` into the path character class |

---

## Verification before you declare done

```bash
# Same number of endpoints in, out
grep -oE '^\| `(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)` \| `[^`]+`' \
  docs/codeconverter/05-api-surface/API.md | wc -l
python3 -c "import json;d=json.load(open('docs/codeconverter/05a-endpoint-consumers/endpoint-consumers.json'));print(len(d['endpoints']))"

# Internal consistency
python3 - <<'PY'
import json
d = json.load(open('docs/codeconverter/05a-endpoint-consumers/endpoint-consumers.json'))
eps = d['endpoints']
assert all(e['caller_count'] == len(e['callers']) for e in eps), "caller_count mismatch"
assert all(e['no_known_caller'] == (len(e['callers']) == 0) for e in eps), "flag mismatch"
assert all(e['searches_run'] for e in eps if e['no_known_caller']), "zero-caller endpoint with no searches recorded"
for e in eps:
    for c in e['callers']:
        assert c.get('file') and c.get('line') and c.get('match') in ('exact','inferred'), (e['path'], c)
print(len(eps), "endpoints OK;",
      sum(1 for e in eps if e['no_known_caller']), "with no known caller")
PY

# The evidence-not-proof statement is present
grep -c 'not found in the repos we hold' docs/codeconverter/05a-endpoint-consumers/endpoint-consumers.md
```

---

## Exit Criteria

Stage 05a is complete when:

- [ ] Every endpoint row in `05-api-surface/API.md` appears exactly once in
      `endpoint-consumers.json`, counts equal, counting command shown.
- [ ] Every caller entry carries repo, `file:line`, a quoted snippet, and a match
      strength (`exact` or `inferred`).
- [ ] The no-known-caller ledger lists every zero-caller endpoint, grouped by
      audience, each with the searches that were run against it.
- [ ] The no-*product*-caller ledger exists alongside it, and the artifact says which
      of the two a drop decision reads.
- [ ] At least ten caller records were spot-checked by **opening the cited
      `file:line`** — stratified so one large route table cannot fill the sample — and
      the result is recorded, including any mis-attribution found and the guard added
      for it.
- [ ] All three search passes were run and their patterns are reproduced in the
      artifact.
- [ ] Variant endpoints (aggregator, admin, legacy-vN) are attributed separately from
      their base-path twins; every cross-variant `inferred` match is labelled.
- [ ] `endpoint-consumers.md` carries the "evidence, not proof" statement and names
      the unscannable consumer classes.
- [ ] `MANIFEST.md` exists, matches the template, and every exit criterion above is
      copied into it and honestly checked.
