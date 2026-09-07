<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Stage note — read first.** Unlike stages 01–11, this stage was not adapted from
> the legacy "codeplanner" process; that process ended at the implementation plan and
> had no back-end decision phase. There is no phase-number translation table to apply
> and no journey.md/journal in this pipeline. Where this document conflicts with the
> stage's SKILL.md output contract (uniform headers, MANIFEST.md, output directory),
> **SKILL.md wins**.

---

# Stage 12 — Back-End Migration Q&A

## Mission

Take every decision the pipeline deferred, put the facts discovery found next to it,
and get an answer.

Stage 00 asked what the port is, before anything was known. Stage 12 asks what it
should do, now that everything is. In between, ten stages produced facts that make
several stage-00 questions answerable for the first time — and raised new ones that
only a human can settle.

The agenda is the artifact. A decision that arrives without its facts gets deferred
again; a decision that arrives with its facts gets made.

---

## The anatomy of an agenda item (keep this in view while working)

Every item has exactly these five parts. An item missing any of them is not ready to
be asked, and asking it anyway wastes the one resource this stage spends: the user's
attention.

| Part | What it is | Failure mode if missing |
|---|---|---|
| **Deferred** | What was postponed, by which stage, and why it could not be decided then | The user relitigates whether it needed deferring |
| **What discovery established** | The facts, each citing the artifact that holds it | The user reconstructs context from scratch — the exact failure this stage exists to prevent |
| **Options** | The genuine alternatives, including "do nothing" where it is real | The user is steered rather than asked |
| **Consequence of each** | What each option costs, breaks, or unblocks | The choice is made on preference instead of cost |
| **Recommendation** | A named preference with its reasoning | A neutral menu produces a slower, worse conversation than a recommendation that can be overruled |

A sixth field, **Affects**, is filled in when the item is decided: which stages the
decision changes, and whether it forces a re-scan.

---

## Step 0 — Readiness

```bash
# Prerequisite: the plan this stage interrogates
grep -m1 '^\*\*Status:\*\*' docs/codeconverter/11-migration-plan/MANIFEST.md

# Is this a fresh round or a continuation?
ls docs/codeconverter/12-migration-qa/agenda.json 2>/dev/null

# The seed sources, and whether each exists
for d in 00-guidance 05a-endpoint-consumers 05b-outbound-dependencies \
         05c-datastore-peers 07-target-codebase 09-dependency-audit \
         10-service-alignment 11-migration-plan; do
  printf '%-28s %s\n' "$d" "$([ -d docs/codeconverter/$d ] && echo present || echo MISSING)"
done
```

A missing seed source is not a blocker — it is a recorded zero. The agenda states
"source X: absent, contributed 0 items" so the gap is visible rather than silent.

---

## Step 1 — Seed the agenda

Walk each source and extract its open decisions. This is mechanical; do not filter for
importance yet.

| Source | What to extract as an item |
|---|---|
| `00-guidance/scope-charter.md` | every `unknown` answer; every row of `Carry to stage 12`; every `derived` answer needing human confirmation |
| `11-migration-plan/` | every open decision, Phase 0 gate, and assumption flagged as unverified |
| `10-service-alignment/` | every deferred absorb/split call; every endpoint whose destination was chosen provisionally |
| `05a-endpoint-consumers/` | every proposed drop whose consumer status is `unknown` (never `empty` — those are decided) |
| `05b-outbound-dependencies/` | every outbound dependency the target cannot currently reach |
| `05c-datastore-peers/` | the store-change verdict; every blocking table and its remediation-before-cutover question |
| `09-dependency-audit/` | every CRITICAL/HIGH finding with no owner or no pre-migration remediation |
| `07-target-codebase/analysis.md` | capabilities the target has that the source does not — these are often the *answer* to another item, not an item themselves |

Record the count each source produced, including zero, in a `Seeding` table. That
table is what makes the agenda auditable: an item that appears without a source, or a
source that silently contributed nothing, is a defect.

The table needs **two count columns**, not one, because items get seeded by more than
one source and a single column cannot be both honest and reconcilable:

| Column | Meaning | What it sums to |
|---|---|---|
| **Contributed** | every source that raised this item, counted once per source | more than the item count, whenever anything is multiply-sourced |
| **New items** | each item counted once, against the first source that raised it | **exactly** the distinct item count |

Reconcile them explicitly in the text under the table. A seeding table whose columns
do not add up is the same class of defect as an uncited count, and it is easy to
produce by accident when items merge.

## Step 2 — Merge items that are the same question

Two items are the same question when one answer settles both. The reliable test: does
the user's reasoning for one apply verbatim to the other?

The most common merge in practice is the **outbound-surface question**. "Keep
publishing to the old message broker during cutover?" and "keep serving the old data
store during cutover?" are both *does the old outbound surface stay up while consumers
migrate, and for how long?* Merge them into one item with two sub-decisions, so the
user answers the shape once and the specifics separately.

Merged items keep both source citations and both `Affects` lists.

## Step 3 — Brief each item

For every item, fill the five parts. The second part — *what discovery established* —
is the one that takes the work and the one the stage is judged on.

Rules for the facts block:

- Every fact cites the artifact and, where the artifact has them, the line or finding
  ID. A fact with no citation is a recollection.
- Include the counts. "Publishes to 4 exchanges with 5 known HIGH-severity consumers"
  is a decidable fact; "publishes some events" is not.
- Include the target's current state, from `07-target-codebase/analysis.md` at the
  pinned ref. A partial in-tree answer changes the question, and it is the single most
  common thing a briefing misses.
- State what is *not* known, explicitly. An option whose consequence is unknown must
  say so rather than be quietly omitted.

## Step 4 — Order the agenda

Order by **what each item unblocks**, not by severity:

1. Items gating Phase 0 or Phase 1 of the migration plan.
2. Items that change scope if answered one way (these force a re-scan; the earlier
   they are answered, the less work is thrown away).
3. Items gating later phases, in phase order.
4. Items that do not gate anything but need an owner.

Write the ordering rationale into the agenda so a later round does not silently
reshuffle it.

## Step 5 — Run the session

Interactive mode. One item at a time, in order.

1. Present the item's five parts. Do not summarize the facts away — the briefing is
   the point.
2. Ask the question as a decision, not as an open discussion.
3. Record the answer, the reasoning **in the user's terms**, and the date.
4. Immediately determine `Affects`: which stages change, and whether a re-scan is
   needed. Say it back before moving on, because the invalidation is part of the
   decision's cost.
5. If the user defers, that is a valid outcome: record `status: open`, the reason, and
   what would make it decidable.

Never batch questions. An answer to item 3 frequently changes item 5's options, and a
batched agenda loses that.

## Step 6 — Emit re-scan requests

Collect every decision whose `Affects` includes a completed stage into a single
`Re-scan requests` section:

```markdown
## Re-scan requests

| Decision | Stages to re-run | Why | Round |
|---|---|---|---|
| D-003 keep publishing to old broker for 2 quarters | 10, 11 | adds an outbound-compat module and a dual-publish phase that neither stage planned | 2 |
```

The orchestrator reads this section, resets those stages' STATE.md rows, increments
the round counter, and re-runs them. Stage 12 does not edit other stages' files.

## Step 7 — Later rounds

The agenda is standing. On a later round:

1. Re-read the sources; **new** open decisions become new items.
2. Items decided in an earlier round stay on the agenda with `status: decided` and a
   pointer to their `decisions.md` entry — they are not deleted.
3. An item can be **reopened** when a later fact contradicts its basis. Reopening
   records what contradicted it; it does not erase the original decision.
4. Increment the round in `agenda.json` and append to the round history.

---

## Worked example — the IAM → izcr deferred agenda

This is what a correctly briefed agenda looks like. These four items were deferred by
name on the IAM run, and each is shown with the facts that make it decidable. Use the
shape, not the content; derive the equivalent items for any other conversion from its
own artifacts.

### A-001 — Event publishing (RabbitMQ)

- **Deferred by:** stage 11 (Phase 0 gate). Could not be decided before the consumer
  map (05a) and the target's own event support (07) existed.
- **What discovery established:**
  - IAM publishes to **4 RabbitMQ exchanges** — `iam_accounts`, `iam_apikeys`,
    `iam_applications`, `iam_certificates` (05b outbound map; 02 `iteration19_rabbitmq_events.md`).
  - **5 known HIGH-severity consumers**: device-server (×2), device-catalog,
    firmware-catalog, deployment-service (09 audit, category 6).
  - The target (izcr) has **no broker** in its stack (07 analysis at the pinned ref).
  - The target *has* gained a partial in-tree answer: `pkg/apispec/v1/events/events.proto`
    with `RegisterConsumer` / `ListConsumers` / `RemoveConsumer` over tenant-scoped
    webhook sinks. This is the fact that changes the question.
- **Options:** (a) port the broker into the target; (b) drop events entirely and
  migrate consumers to the webhook sinks in lockstep; (c) **drop the broker internally
  but keep publishing outbound during cutover**, retiring it as each consumer moves.
- **Consequences:** (a) imports infrastructure the target deliberately does not have;
  (b) requires 5 consumer teams to move in lockstep, which the charter's Q5 answer
  says they cannot; (c) costs one compatibility publisher and a retirement schedule,
  and lets each consumer move on its own timetable.
- **Recommendation:** (c). It is the only option compatible with the lockstep answer,
  and the in-tree webhook sink gives each consumer a destination to move *to* rather
  than merely taking one away.

### A-002 — Policy storage

- **Deferred by:** stage 10, then stage 11. The 4-tier cascade had no obvious
  equivalent in the target's policy engine.
- **What discovery established:**
  - Source holds **15 policy tables** and a **4-tier cascade** (02 storage map;
    06 domain analysis).
  - Target uses OPA/Rego. Its existing `default.rego` reads only from `input.*` and
    contains **zero `data.` references** — permissions are pre-computed in Go
    (HashTree) and Rego is a thin decision procedure over the result.
  - That shape means the cascade does not have to be expressed in Rego at all; it has
    to be expressed in whatever produces the pre-computed input.
- **Options:** (a) translate the cascade into Rego `data` documents; (b) keep the
  cascade in Go, pre-compute, and leave Rego as the thin decision procedure the target
  already uses; (c) flatten the cascade at write time and store resolved permissions.
- **Consequences:** (a) fights both engines — Rego's static-compile model is a poor
  fit for a 4-tier runtime cascade; (b) matches the target's existing shape exactly,
  at the cost of porting cascade logic into Go; (c) fastest reads, but every tier
  change becomes a bulk rewrite and audit history is lost.
- **Recommendation:** (b). The target has already answered this question in its own
  code; matching it is cheaper than either alternative and keeps one policy model.

### A-003 — Data migration

- **Deferred by:** stage 11 (Phase 0 gate).
- **What discovery established:**
  - Source is PostgreSQL, target is BoltDB — **no shared schema is possible** and
    therefore **no dual-write cutover is available**. This is a property of the store
    pair, not a scheduling choice.
  - 56 tables in scope (02 storage map, re-derived; 05c per-table map).
  - Whatever is chosen must cover every table 05c marks as owned, and interacts
    directly with A-004.
- **Options:** (a) one-shot offline migration in a maintenance window; (b) export/import
  with a read-only period per domain; (c) run both stores in parallel with an
  application-level sync during cutover.
- **Consequences:** (a) simplest and testable, requires downtime the production-status
  answer may not permit; (b) longer, allows per-domain rollback; (c) most complex, and
  the sync layer is itself new code with no baseline.
- **Recommendation:** decide against the charter's Q2 answer. `production-critical`
  rules out (a); (b) is the default under partial lockstep.

### A-004 — Postgres/BoltDB peer study

- **Deferred by:** stage 00 Q4 (`unknown`), pending stage 05c.
- **What discovery established:** the store-change verdict and the blocking-table list
  in `05c-datastore-peers/datastore-peers.md`, plus the stage 09 cross-check — in
  particular the CRITICAL direct-database finding whose peer reaches `iamdb` from a
  Helm init container rather than from any service.
- **Options:** (a) remediate every direct-DB peer before cutover; (b) keep serving the
  old store outbound for the blocking tables until peers move; (c) migrate anyway and
  accept the breakage.
- **Consequences:** (a) blocks cutover on other teams' schedules; (b) same shape as
  A-001 — an outbound compatibility surface with a retirement schedule; (c) silent
  failure, discovered in production.
- **Recommendation:** merge with A-001 and answer the shape once. Whatever is decided
  for the broker should be decided for the store, for the same reason.

**Note the merge.** A-001 and A-004 are the same question — *does the old outbound
surface stay up while consumers migrate?* — asked about a broker and a database. Step
2 says to merge them; this example leaves both visible so the merge itself is legible.

---

## Step 8 — Write the outputs

### `agenda.md`

```markdown
<!-- codeconverter artifact -->
**Stage:** 12-migration-qa
**Artifact:** agenda.md — the standing agenda of deferred decisions, each pre-briefed
**Status:** final
**Produced by:** codeconverter-12-migration-qa on YYYY-MM-DD
**Inputs:** <every seed source>

---

# <Service> → <Target> — Migration Q&A Agenda

**Round:** N
**Mode:** interactive | --seed-only

## Seeding
| Source | Present | Contributed | New items |
|---|---|---|---|

<reconciliation sentence: what "contributed" sums to, what "new items" sums to, and why>

## Ordering rationale
<why the agenda is in this order, per Step 4>

## Agenda
### A-00N — <title>
- **Status:** open | decided (see D-00N) | withdrawn (<why>)
- **Deferred by:** <stage>, <why it could not be decided then>
- **What discovery established:** <facts, each cited>
- **Options:** <a/b/c>
- **Consequences:** <per option>
- **Recommendation:** <choice + reasoning>
- **Affects:** <stages, filled in when decided>

## Still open at end of round
| Item | Why still open | What would make it decidable |
```

### `decisions.md`

```markdown
<!-- codeconverter artifact -->
**Stage:** 12-migration-qa
**Artifact:** decisions.md — what was decided, by whom, and what it changes
**Status:** final
**Produced by:** codeconverter-12-migration-qa on YYYY-MM-DD
**Inputs:** docs/codeconverter/12-migration-qa/agenda.md

---

# Decision log

## D-00N — <decision>
- **Agenda item:** A-00N
- **Decided:** YYYY-MM-DD by <who>
- **Decision:** <what was chosen>
- **Reasoning:** <in the user's terms>
- **Affects:** <stages>
- **Re-scan required:** yes/no — <which stages, why>

## Re-scan requests
| Decision | Stages to re-run | Why | Round |
```

### `agenda.json`

```json
{
  "round": 1,
  "mode": "interactive",
  "seeding": [{"source": "00-guidance", "present": true, "contributed": 3, "new_items": 3}],
  "items": [
    {
      "id": "A-001",
      "title": "Event publishing (RabbitMQ)",
      "status": "open",
      "deferred_by": "11-migration-plan",
      "facts": [{"fact": "...", "cite": "05b-outbound-dependencies/outbound-dependencies.md#OUT-004"}],
      "options": [{"id": "c", "text": "...", "consequence": "..."}],
      "recommendation": "c",
      "merged_with": ["A-004"],
      "affects": [],
      "decision": null
    }
  ],
  "decisions": [],
  "rescan_requests": [],
  "round_history": []
}
```

---

## Verification before you declare done

```bash
J=docs/codeconverter/12-migration-qa/agenda.json

# Every item has all five briefing parts
python3 -c "
import json;d=json.load(open('$J'))
need=('deferred_by','facts','options','recommendation')
bad=[i['id'] for i in d['items']
     if any(not i.get(k) for k in need)
     or not all(o.get('consequence') for o in i.get('options',[]))]
print('items missing a briefing part:', bad or 'none')"

# Every fact carries a citation
python3 -c "
import json;d=json.load(open('$J'))
bad=[(i['id'],f['fact'][:40]) for i in d['items'] for f in i.get('facts',[]) if not f.get('cite')]
print('uncited facts:', bad or 'none')"

# Every scope-charter unknown / carry-to-12 entry became an item
python3 -c "
import json
c=json.load(open('docs/codeconverter/00-guidance/scope-charter.json'))
a=json.load(open('$J'))
blob=json.dumps(a['items']).lower()
miss=[x['item'] for x in c.get('carry_to_stage_12',[]) if x['item'].lower()[:20] not in blob]
print('carry-to-12 entries with no agenda item:', miss or 'none')" 2>/dev/null \
  || echo '00-guidance charter absent — record as a zero-contribution source'

# Every seed source is accounted for, including zeros, AND the two count columns
# reconcile: new_items must sum to the distinct item count. This check exists because
# a merged item is trivially easy to count twice.
python3 -c "
import json;d=json.load(open('$J'))
print('sources recorded:', len(d['seeding']), '(expect 8)')
print([(s['source'], s['contributed'], s['new_items']) for s in d['seeding']])
tot=sum(s['new_items'] for s in d['seeding'])
print('new_items sum:',tot,'| items:',len(d['items']),
      '|','OK' if tot==len(d['items']) else 'MISMATCH')
print('contributed sum:',sum(s['contributed'] for s in d['seeding']))"

# Decisions name their affected stages
python3 -c "
import json;d=json.load(open('$J'))
bad=[x['id'] for x in d.get('decisions',[]) if not x.get('affects')]
print('decisions with no affects list:', bad or 'none')"
```

Paste this output into the MANIFEST.

---

## Exit Criteria

Copy the exit criteria from `SKILL.md` into `MANIFEST.md` and check them honestly.
The agenda is the last thing standing between the plan and implementation — an item
that was never asked becomes a decision made by default, at the worst possible time.
