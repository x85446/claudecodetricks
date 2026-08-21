<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Stage note — read first.** Unlike stages 01–11, this stage was not adapted from
> the legacy "codeplanner" process; that process went from analysis straight to a
> phased plan with no empirical probe. There is no phase-number translation table to
> apply and no journey.md/journal in this pipeline. Where this document conflicts with
> the stage's SKILL.md output contract (uniform headers, MANIFEST.md, output
> directory), **SKILL.md wins**.

---

# Stage 10a — Pilot Slice

## Mission

Build a small piece of the migration for real, and let it correct the plan.

Everything before this stage is reading. This is the first stage that *does* the
thing, and the first that can be wrong in a way the pipeline notices immediately.

---

## The rule that governs every finding (keep this in view while working)

**Findings are rules, not incidents.**

| Incident (weak) | Rule (what to write) |
|---|---|
| "The router dropped the trailing slash on `/v3/accounts/`" | "grpc-gateway normalizes trailing slashes and the source does not — every contract path with an optional trailing slash needs an explicit binding. N of the 637 have one." |
| "The account ID serialized as a number" | "The source emits all IDs as strings, including numeric-looking ones. Any generated struct using an integer type breaks the wire contract. Audit every ID field in the IDL." |
| "Auth middleware ran after the handler" | "The target's interceptor order differs from the source's filter chain; anything relying on pre-handler auth context needs explicit ordering. Applies to every authenticated endpoint." |

The five recount failures on the IAM run were *one* systemic fault found five times.
An incident log would have listed five items; a rule would have named it once. Every
finding here gets the general statement, then the specific break as its evidence.

---

## Step 0 — Readiness

```bash
# Prerequisite
grep -m1 '^\*\*Status:\*\*' docs/codeconverter/10-service-alignment/MANIFEST.md

# The target tree must be the tree we cite
python3 -c "
import json,sys
p=json.load(open('docs/codeconverter/00-source-provenance/provenance.json'))
r=next(x for x in p['repos'] if x['name']=='<target>')
print('state',r['state'],'working_tree',r['working_tree'])
if r['state']=='unfetched' or r['working_tree']=='unrelated':
    sys.exit('target tree is not the pinned ref — a pilot built here teaches the wrong lessons')"

# What the plan currently assumes, so we know what we are testing
ls docs/codeconverter/11-migration-plan/ 2>/dev/null
ls docs/codeconverter/04-test-baseline/
```

## Step 1 — Choose the slice

Score candidate domains against the five criteria in `SKILL.md`, and write the scoring
down — the choice is a finding in itself, because it names what the pilot will and will
not learn.

```markdown
| Candidate domain | Endpoints | Has a write | Touches the store | Crosses a framework boundary | Baseline tests | Chosen? |
|---|---|---|---|---|---|---|
```

Then state, explicitly:

> **This slice does not exercise:** <the list>. Findings from it do not generalize to
> those areas, and stage 11 must not treat them as if they did.

That sentence is what stops a 5-endpoint pilot being cited as evidence about 549.

## Step 2 — Build it

Real code, in the target repo's real source tree, on the conversion branch.

Rules while building:

- **Do not adapt the target to make the pilot easy.** If existing behaviour has to
  change, stop and record it as a finding — that is one of the most valuable results
  this stage can produce, and papering over it destroys the signal.
- **Do not stub the store.** The persistence layer is usually the largest unknown in
  the whole migration; a pilot that mocks it has skipped the experiment.
- **Keep a running log of every surprise**, however small, with a timestamp. Small
  surprises are where the rules come from; you will not remember them afterwards, and
  the ones that felt trivial at the time are disproportionately the general ones.
- **Time the work.** Not to be precise — to have a floor.

## Step 3 — Run it

```bash
# Whatever the target's own run command is; record it verbatim with its exit status
<build command>;  echo "build exit=$?"
<run command> &
<the actual requests, as curl or the target's test client>
```

Record request and response for **every endpoint in the slice**, including the write.
A pilot whose output is "it worked" has produced no evidence. The responses are the
artifact — they are the first real comparison between old and new behaviour, and they
frequently contain the finding.

## Step 4 — Exercise against the baseline

Run the stage 04 baseline tests for the slice's domain against the new implementation.

Where those tests cannot run against the target — different language, different
harness, different protocol — define the equivalent check, run it, and justify the
substitution in writing. "The Java suite cannot target a Go binary" is a reason to
build an HTTP-level equivalent, not a reason to skip the step.

Record: tests run, passed, failed, and for each failure whether it is a **defect in
the pilot** or a **difference in the contract**. That distinction is the whole value:
a contract difference is a finding about the migration; a pilot defect is not.

## Step 5 — Write the findings

For each finding:

```markdown
### PS-00N — <the rule, stated generally>

**Rule:** <the general statement that would have prevented this>
**Revealed by:** <the specific break, with the file/endpoint/command>
**Evidence:** <request/response, error output, or diff>
**Scope:** <how many of the remaining endpoints this applies to, and how that was counted>
**Corrects:** <the stage-11 assumption this changes — or "none; new information">
**Cost if ignored:** <what happens if the plan proceeds without it>
```

The `Scope` line needs a count and the count needs its command — a rule that applies to
"many endpoints" cannot be scheduled.

## Step 6 — The effort ratio

```markdown
## Effort

| Metric | Value |
|---|---|
| Endpoints in the slice | N |
| Elapsed effort | <hours/days> |
| Per-endpoint | <ratio> |

**This is a floor, not an estimate.** It excludes one-time setup already paid here
(<list>), and the slice was chosen to be tractable, so it under-states the mean.
Multiplying it by the remaining endpoint count produces a number that is wrong in a
known direction. Stage 11 should use it to sanity-check its own estimates, not to
replace them.
```

Being explicit about the direction of the bias is the difference between a useful floor
and a number someone multiplies by 549.

## Step 7 — Write the outputs

### `findings.md`

```markdown
<!-- codeconverter artifact -->
**Stage:** 10a-pilot-slice
**Artifact:** findings.md — what the pilot broke, as rules the plan must absorb
**Status:** final
**Produced by:** codeconverter-10a-pilot-slice on YYYY-MM-DD
**Inputs:** 10-service-alignment/routing-table.md; 07-target-codebase/; 04-test-baseline/; the pilot code

---

# <Service> → <Target> — Pilot Slice Findings

## The slice
<domain, endpoints, why chosen, what it does NOT exercise>

## Run record
<commands, exit statuses, request/response per endpoint>

## Baseline result
<tests run / passed / failed; each failure classified defect-vs-contract>

## Findings
### PS-001 — <rule>
...

## Effort
<per Step 6>

## What stage 11 must change
| Finding | Stage 11 artifact | Assumption corrected |
```

### `findings.json`

```json
{
  "slice": {"domain": "...", "endpoints": [], "writes": 1, "not_exercised": []},
  "run": {"build_exit": 0, "commands": [], "responses": []},
  "baseline": {"tests_run": 0, "passed": 0, "failed": 0,
               "failures": [{"test": "...", "class": "contract|pilot-defect"}]},
  "findings": [{"id": "PS-001", "rule": "...", "revealed_by": "...", "evidence": "...",
                "scope": {"count": 0, "command": "..."}, "corrects": "...",
                "cost_if_ignored": "..."}],
  "effort": {"endpoints": 0, "elapsed": "...", "per_endpoint": "...", "floor": true,
             "excluded_one_time": []}
}
```

---

## Verification before you declare done

```bash
J=docs/codeconverter/10a-pilot-slice/findings.json

# The slice is the right size and includes a write
python3 -c "
import json;d=json.load(open('$J'))
n=len(d['slice']['endpoints'])
print('endpoints:',n,'OK' if 3<=n<=8 else 'OUT OF RANGE')
print('writes:',d['slice']['writes'],'OK' if d['slice']['writes']>=1 else 'NO WRITE — the store was not exercised')"

# It actually ran
python3 -c "
import json;d=json.load(open('$J'))
r=d['run']
print('build exit:',r['build_exit'])
print('commands recorded:',len(r['commands']),'| responses recorded:',len(r['responses']),
      '| per-endpoint responses OK' if len(r['responses'])>=len(d['slice']['endpoints']) else '| MISSING RESPONSES')"

# Every finding is a rule with a counted scope
python3 -c "
import json;d=json.load(open('$J'))
bad=[f['id'] for f in d['findings']
     if not f.get('rule') or not f.get('revealed_by') or not f.get('corrects')
     or not f.get('scope',{}).get('command')]
print('findings that are incidents, not rules:', bad or 'none')"

# The effort figure is labelled a floor and says what it excludes
python3 -c "
import json;d=json.load(open('$J'))
e=d['effort']
print('floor flagged:',e.get('floor'),'| one-time exclusions listed:',bool(e.get('excluded_one_time')))"

# Baseline failures are each classified
python3 -c "
import json;d=json.load(open('$J'))
bad=[f['test'] for f in d['baseline'].get('failures',[]) if f.get('class') not in ('contract','pilot-defect')]
print('unclassified failures:', bad or 'none')"
```

Paste this output into the MANIFEST.

---

## Exit Criteria

Copy the exit criteria from `SKILL.md` into `MANIFEST.md` and check them honestly.
The one criterion that cannot be waived is that the slice **ran** — a designed-but-not-
executed pilot is another plan, and the pipeline already has enough of those.
