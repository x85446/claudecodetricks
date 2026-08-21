<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Stage note — read first.** Unlike stages 01–11, this stage was not adapted from
> the legacy "codeplanner" process; that process had no guidance phase and opened
> directly on service analysis. There is no phase-number translation table to apply
> and no journey.md/journal in this pipeline. Where this document conflicts with the
> stage's SKILL.md output contract (uniform headers, MANIFEST.md, output directory),
> **SKILL.md wins**.

---

# Stage 00 — Guidance / Scope Charter

## Mission

Find out what the port actually is, before anything analyses anything.

Six questions. Every later stage's scope is a function of the answers, and the
pipeline currently has no mechanism that would ever notice it had guessed them wrong.

---

## The six required questions (keep this in view while working)

| # | Question | Why the pipeline needs it |
|---|---|---|
| Q1 | **Scope** — one library, several libraries, a whole service, or a whole suite? | Sets what stage 02 reads, how wide stage 03 sweeps, and whether stage 10's alignment decision applies at all. |
| Q2 | **Production status** — production with paying users, internal-only, or throwaway? | Decides whether parity harness, rollback path and cutover window are mandatory or wasted. |
| Q3 | **Field upgrades** — are there deployed artifacts in the wild that must keep working? | If yes, the wire format is frozen; "clean up the API while we're in there" is off the table. |
| Q4 | **Data store** — does the store change, and what else touches it? | A store swap is only safe if every peer is isolated behind an API or library. Makes stage 05c required. |
| Q5 | **Consumer lockstep** — can consumers be migrated at the same time, or must they keep working unchanged? | Decides whether "drop this endpoint" is ever an available move in stages 10 and 11. |
| Q6 | **Definition of done** — what condition means the port is finished? | Stage 11's plan needs a terminating criterion; stage 12's Q&A needs a target. |

Each question gets a section in the charter whether or not the user had a confident
answer. **`unknown` is a valid answer**; an omitted question is not. An `unknown`
must name the stage that will resolve it and go onto stage 12's agenda.

---

## Step 0 — Readiness

```bash
# Is this a fresh run or a re-interview?
ls docs/codeconverter/00-guidance/scope-charter.md 2>/dev/null

# What does the pipeline already believe, if anything?
sed -n '1,40p' docs/codeconverter/STATE.md 2>/dev/null

# Which stages have already run (a re-interview must know what it may invalidate)
ls docs/codeconverter/*/MANIFEST.md 2>/dev/null
```

If `scope-charter.md` exists, skip to **Step 7 — re-interview**.

---

## Step 1 — Q1: Scope

Ask, in this order. Stop when the shape is unambiguous.

1. "What is being replaced — a library, a set of libraries, one service, or a group
   of services?"
2. "What else ships inside the same deployable artifact as the thing you're
   replacing?" — this is the question that catches an under-stated scope. Users who
   say "just the auth library" frequently mean a library that is the only meaningful
   content of a service.
3. "If this rewrite succeeds, which repositories have changed?" — a scope answer that
   names exactly one repo but implies changes in four is not settled.

Record one of: `single-library` | `several-libraries` | `single-service` |
`service-suite`, plus the explicit list of units in scope and the explicit list of
things a reader might expect to be in scope that are **not**.

The non-goals list is load-bearing. Half of scope creep in a rewrite is a thing
nobody ever said was out.

## Step 2 — Q2: Production status

Do not ask "is this in production?" — it gets a yes/no that hides the real answer.
Ask **who breaks**:

> "If the replacement goes live and gets something wrong, who notices? Nobody / the
> team / one customer / every customer / devices already deployed in the field?"

Record one of: `throwaway-prototype` | `internal-only` | `production-limited` |
`production-critical`, with the blast radius in the user's own words.

Then the consequence, stated in the charter so later stages inherit it:

| Answer | What becomes mandatory |
|---|---|
| `throwaway-prototype` | nothing; parity work is optional |
| `internal-only` | rollback path |
| `production-limited` | rollback path, parity harness (stage 11) |
| `production-critical` | rollback path, parity harness, cutover window, staged traffic shift |

## Step 3 — Q3: Field upgrades

> "Is there anything already deployed that you cannot redeploy on your own schedule —
> devices, on-prem installs, agents, mobile apps, pinned SDK versions in customer
> code?"

Record `yes` / `no` / `unknown`, and if `yes`: what those artifacts are, what
protocol/version they speak, and how long they must keep being served.

If `yes`, the charter states the consequence plainly: **the wire format visible to
those artifacts is frozen for the duration named**, and stages 10 and 11 may not
drop, rename, or change the shape of anything they call.

## Step 4 — Q4: Data store — the dangerous one

Four sub-questions, all of them required:

1. "What is the persistence layer today, and what does it become?"
2. "Does any other service or job read or write that store?"
3. "For each one — does it go through this service's API, through a shared library,
   or does it open its own connection?"
4. "Are there batch jobs, reporting tools, migration scripts, init containers, or
   analytics pipelines pointed at it?" — this sub-question exists because the answer
   to (2) is usually given about *services* and misses everything that is not one.
   The IAM run's CRITICAL direct-DB finding was a Helm init container running `psql`,
   which no service inventory would have listed.

Record: `store-unchanged` | `store-changed` | `unknown`, the from/to pair, and the
user's *believed* peer list — clearly labelled as belief, not fact.

Then write the trigger, verbatim, into the charter:

> **Stage 05c is REQUIRED.** The store changes (or the answer is unknown), so every
> peer that touches it must be classified as direct-DB / shared-library / API before
> the migration plan can be trusted. A peer reaching the store directly does not
> break loudly on cutover; it breaks silently, later.

05c is required whenever Q4 is `store-changed` **or** `unknown`. It is optional only
when the store demonstrably does not change.

## Step 5 — Q5: Consumer lockstep

> "When the replacement ships, can the things that call it be updated at the same
> time — or do some of them have to keep working exactly as they do now?"

Then the follow-up that gets the true answer:

> "Who deploys each of those, and on whose release schedule?"

A consumer owned by another team on a quarterly train is not in lockstep, whatever
the intent. Record `full-lockstep` | `partial-lockstep` | `no-lockstep` | `unknown`,
plus the named consumers in each bucket.

Write the consequence into the charter: under `no-lockstep` or `partial-lockstep`,
**no endpoint may be dropped without a citation to its row in the stage 05a consumer
map** — which is the rule stage 10 and stage 11 enforce.

## Step 6 — Q6: Definition of done

> "What has to be true for you to call this finished?"

Push until it is checkable. "Feature parity" is not checkable; "every endpoint in
`05-api-surface/API.md` returns the same response for the replay corpus, and the old
service is off" is. The test to apply: *could a later reader evaluate this as true or
false without asking anyone?*

Record the condition, plus anything explicitly **not** required for done (the
non-goals of completion — the most useful half of this answer).

## Step 7 — Re-interview (round N ≥ 2)

A re-interview happens when a later stage produced a fact that contradicts a charter
answer, or when stage 12 resolved an `unknown`. Do not start over.

1. Read the existing `scope-charter.md` and its `Round history` section.
2. Present each of the six answers back with the fact that challenged it.
3. Change only the answers the user changes. Everything else keeps its original
   round number and source.
4. Append a `Round history` row: round number, date, what changed, what triggered it,
   and **which completed stages the change invalidates**.
5. Update `scope-charter.json`'s `round` field, and report the invalidated stages to
   the orchestrator so it can reset their STATE.md rows.

The invalidation list is the point of the round mechanism. A charter that changes
without naming what it invalidates leaves stale output downstream, which is worse
than not re-interviewing at all.

---

## Step 8 — Write the outputs

### `scope-charter.md`

```markdown
<!-- codeconverter artifact -->
**Stage:** 00-guidance
**Artifact:** scope-charter.md — what this port is: scope, risk, constraints, and done
**Status:** final
**Produced by:** codeconverter-00-guidance on YYYY-MM-DD
**Inputs:** interview with <user>, docs/codeconverter/STATE.md

---

# <Service> → <Target> — Scope Charter

**Round:** N
**Mode:** interactive | --from-artifacts

## The charter in one paragraph
<Three or four sentences a new reader can act on.>

## Q1 — Scope
**Answer:** single-library | several-libraries | single-service | service-suite
**Source:** stated | derived (<artifact>:<line>)
**Confidence:** high | medium | low | unknown
**In scope:** <explicit list>
**Explicitly NOT in scope:** <explicit list>

## Q2 — Production status
**Answer:** throwaway-prototype | internal-only | production-limited | production-critical
**Blast radius:** <who breaks>
**Mandatory because of this answer:** <rollback / parity harness / cutover window / …>
(same Source + Confidence fields)

## Q3 — Field upgrades
**Answer:** yes | no | unknown
**Artifacts in the wild:** <what, speaking what, for how long>
**Frozen surface:** <what may not change, or "none">

## Q4 — Data store
**Answer:** store-unchanged | store-changed | unknown
**From → to:** <e.g. PostgreSQL 11 → BoltDB + Raft>
**Believed peers (user's belief, not verified):** <list>
**Stage 05c required:** yes | no — <reason>

## Q5 — Consumer lockstep
**Answer:** full-lockstep | partial-lockstep | no-lockstep | unknown
**Lockstep consumers:** <list>
**Non-lockstep consumers:** <list, with owning team and release cadence>
**Drop rule in force:** <the citation requirement, or "drops permitted">

## Q6 — Definition of done
**Done when:** <checkable condition>
**Explicitly not required for done:** <list>

## Stage applicability
| Stage | required / optional / not-applicable | Reason (traceable to a charter answer) |
|---|---|---|
| 01-service-profile | required | always |
| … | … | … |

## Unknowns
| # | Question | Why unresolved | Stage that resolves it |
|---|---|---|---|

## Carry to stage 12
- <every unknown and every derived answer, with the context stage 12 needs>

## Round history
| Round | Date | What changed | Triggered by | Stages invalidated |
|---|---|---|---|---|
| 1 | YYYY-MM-DD | initial charter | — | — |
```

### `scope-charter.json`

```json
{
  "round": 1,
  "mode": "interactive",
  "produced": "YYYY-MM-DD",
  "answers": {
    "scope":            {"value": "...", "source": "stated", "confidence": "high",   "detail": {...}},
    "production_status":{"value": "...", "source": "stated", "confidence": "high",   "detail": {...}},
    "field_upgrades":   {"value": "...", "source": "stated", "confidence": "medium", "detail": {...}},
    "data_store":       {"value": "...", "source": "stated", "confidence": "high",   "detail": {...}},
    "consumer_lockstep":{"value": "...", "source": "stated", "confidence": "medium", "detail": {...}},
    "definition_of_done":{"value":"...", "source": "stated", "confidence": "high",   "detail": {...}}
  },
  "stage_applicability": {"01-service-profile": "required", "...": "..."},
  "requires_05c": true,
  "drop_rule": "citation-required",
  "unknowns": [{"question": "...", "resolved_by": "05c-datastore-peers"}],
  "carry_to_stage_12": [{"item": "...", "context": "..."}],
  "round_history": [{"round": 1, "date": "...", "changed": "initial charter", "invalidated": []}]
}
```

Every `source` is `stated` or `derived`. There is no third value, and `derived`
without a citation is not a valid record.

---

## `--from-artifacts` mode

Used when no human is present. Derive each answer from artifacts, in this precedence
order, and cite the file and line:

| Q | Derive from |
|---|---|
| Q1 scope | `01-service-profile/` profile (units, repos); STATE.md source-service line |
| Q2 production status | `09-dependency-audit/` severities; deployment manifests under the manifests path; presence of live consumers in `05a-endpoint-consumers/` |
| Q3 field upgrades | `03-dependency-discovery/` (SDKs, device/agent repos); `09-dependency-audit/` wire-format findings |
| Q4 data store | STATE.md target stack line; `02-codebase-analysis/storage_map.json`; `07-target-codebase/analysis.md` stack section |
| Q5 consumer lockstep | `03-dependency-discovery/references.md` repo ownership; `05a-endpoint-consumers/` caller repos |
| Q6 definition of done | `11-migration-plan/` completion criteria if it exists; otherwise the API surface count as the parity target |

Rules for this mode:

- Confidence is never `high` for a derived answer. The ceiling is `medium`.
- Every derived answer goes on the `Carry to stage 12` list without exception.
- If an artifact needed for a derivation does not exist, the answer is `unknown` —
  never a guess. `unknown` with a named resolving stage is a correct output here.

---

## Verification before you declare done

```bash
# All six questions present in both artifacts
for q in scope production_status field_upgrades data_store consumer_lockstep definition_of_done; do
  printf '%-22s json=%s\n' "$q" "$(python3 -c "
import json;d=json.load(open('docs/codeconverter/00-guidance/scope-charter.json'))
print(d['answers'].get('$q',{}).get('value','MISSING'))")"
done
grep -c '^## Q[1-6] ' docs/codeconverter/00-guidance/scope-charter.md   # expect 6

# Every answer has a source and a confidence; every derived answer has a citation
# and obeys the medium-confidence ceiling
python3 -c "
import json;d=json.load(open('docs/codeconverter/00-guidance/scope-charter.json'))
a=d['answers']
print('answers missing source/confidence:',
      [k for k,v in a.items() if not v.get('source') or not v.get('confidence')] or 'none')
print('derived answers with no citation:',
      [k for k,v in a.items() if v['source']=='derived' and not v.get('cite')] or 'none')
print('derived answers claiming high confidence:',
      [k for k,v in a.items() if v['source']=='derived' and v['confidence']=='high'] or 'none')"

# Every stage has a valid applicability value
python3 -c "
import json;d=json.load(open('docs/codeconverter/00-guidance/scope-charter.json'))
sa=d['stage_applicability']
print('stages assigned:',len(sa),'| invalid values:',
      [k for k,v in sa.items() if v not in ('required','optional','not-applicable')] or 'none')"

# The 05c trigger is consistent with the data-store answer
python3 -c "
import json;d=json.load(open('docs/codeconverter/00-guidance/scope-charter.json'))
ds=d['answers']['data_store']['value']
print('data_store=',ds,'requires_05c=',d['requires_05c'],
      'CONSISTENT' if (d['requires_05c'] == (ds in ('store-changed','unknown'))) else 'INCONSISTENT')"

# Every unknown answer, every derived answer, and every entry in the Unknowns table
# is carried to stage 12. Match on exact keys — `carry_to_stage_12[].item` holds the
# answer key or the unknown's id, never prose. A substring match here would pass on
# an accidental overlap and is not good enough for a completeness check.
python3 -c "
import json;d=json.load(open('docs/codeconverter/00-guidance/scope-charter.json'))
carried={c['item'] for c in d['carry_to_stage_12']}
need={k for k,v in d['answers'].items() if v['value']=='unknown' or v['source']=='derived'}
need|={u['id'] for u in d.get('unknowns',[])}
print('required to carry:',len(need),'| not carried:', sorted(need-carried) or 'none')"
```

Paste the output of these commands into the MANIFEST. A verification that was run
but not shown is indistinguishable from one that was not run.

---

## Exit Criteria

Copy the exit criteria from `SKILL.md` into `MANIFEST.md` and check them honestly.
The charter is a document later stages act on without a human in the loop — an
unchecked criterion here propagates into every stage that follows.
