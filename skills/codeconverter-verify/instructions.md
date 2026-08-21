<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Note — this is a helper, not a stage.** It has no output directory, no stage
> number, and no MANIFEST of its own. Its product is a verification record appended to
> the **calling stage's** MANIFEST. Where this document conflicts with the calling
> stage's output contract, the calling stage wins.

---

# codeconverter-verify — Independent Recount

## Mission

Re-derive an inherited figure by a different method, and make disagreement visible
before a later stage builds on it.

The pipeline's worst failures were not wrong analyses. They were correct analyses of
numbers nobody re-derived.

---

## Step 1 — State the claim precisely

A claim that cannot be re-derived cannot be verified, and vagueness is the most common
reason a verification comes back `blocked`.

Write the claim in this form before doing anything:

> *`<artifact>:<line>` asserts that `<subject>` numbers `<figure>`, where `<subject>`
> means `<precise definition>`.*

The definition is what does the work. "izcr has 189 endpoints" is not verifiable —
does an endpoint mean an HTTP rule in a proto, a gRPC method, a generated gateway
route, or a path in a spec? Four definitions, four numbers, all defensible, none
comparable. Pin the definition or return `blocked`.

## Step 2 — Choose an independent method

| The claim came from | Re-derive from |
|---|---|
| a spec/IDL file | route annotations or handler registrations in code |
| route annotations | the generated spec, or the router's own registration table |
| a prior stage's JSON | the source the JSON was built from, never the JSON |
| a ported legacy document | source, always — a ported document has no independent standing |
| a schema doc | the migration DDL |
| a config file | the code that reads the config, and vice versa |

If no independent method exists, say so in the record and return `blocked`. A
same-method re-run recorded as `pass` is worse than no verification, because it
launders the original number.

## Step 3 — Fix the exclusions before you count

Decide, write down, and apply identically to both methods:

- vendored third-party trees (`vendor/`, `node_modules/`, `third_party/`, `protoext/`)
- generated output (`*.pb.go`, `*_generated.*`, `target/`, `dist/`)
- test fixtures and sample data
- documentation and examples

**This is the field that broke the last correction.** An izcr endpoint recount swept
vendored `protoext/google/api/*` — Google's own apikeys, serviceusage, servicecontrol,
servicemanagement and cloudquotas protos, which izcr vendors and does not implement —
and produced 189 against a truth nearer 148. The exclusion list was never stated, so
the error was invisible in the number.

## Step 4 — Count

```bash
# Prefer counting NAMED things, deduplicated — it survives reformatting
<extract identifiers> | sort -u | wc -l

# Keep the identifier list, not just the count. The list is what makes a delta
# diagnosable instead of merely alarming.
<extract identifiers> | sort -u > /tmp/rederived.txt
```

For an external repo whose working tree is not the pinned ref, count **the ref**, not
the disk:

```bash
git -C <repo> ls-tree -r --name-only <ref> | grep <pattern>
git -C <repo> show <ref>:<path> | <extract>
```

## Step 5 — Compare, and diff the sets

```bash
# The number
echo "claimed=<n>  rederived=$(wc -l < /tmp/rederived.txt)"

# The sets — this is the part worth doing
comm -23 /tmp/claimed.txt /tmp/rederived.txt   # claimed only
comm -13 /tmp/claimed.txt /tmp/rederived.txt   # re-derived only
```

Equal counts over different sets is a real and deceptive outcome. When the claim is an
inventory, the set diff **is** the verification and the count is a summary of it.

## Step 6 — Write the record

Use the mandatory format in `SKILL.md`. Then, if the calling stage wants it machine-
readable, write `verification/<claim-id>.json` inside that stage's directory:

```json
{
  "claim_id": "endpoint-count",
  "stage": "07-target-codebase",
  "claim": 189,
  "claimed_by": "07-target-codebase/analysis.md:142",
  "rederived": 148,
  "claim_method": "unknown — not recorded by the original",
  "rederive_method": "git -C izcr ls-tree -r --name-only origin/main | grep '\\.proto$' | ...",
  "ref": "izcr@d3a0ca5 (2026-08-13)",
  "excluded": ["protoext/google/api/** — vendored Google protos izcr does not implement"],
  "delta": -41,
  "delta_pct": -21.7,
  "verdict": "fail",
  "set_diff": {"claimed_only": [], "rederived_only": []}
}
```

`claim_method` may be `"unknown — not recorded by the original"`. That is itself a
finding, and it should be reported as one: a figure whose method was never recorded
cannot be reconciled with any other figure, only replaced.

---

## Worked example — the count that started this

The IAM run published an izcr endpoint count of **114**, "corrected" it to **189**,
and a later team reported **148**. An independent check for the gap analysis produced
**171 / 102** depending on exclusions. Four numbers, three methods, none reconcilable —
because not one shipped with the command that produced it.

What a correct record would have looked like from the start:

```markdown
### Verification — izcr-endpoint-count

| Field | Value |
|---|---|
| Claim | 189 |
| Claimed by | 07-target-codebase/analysis.md (correction pass) |
| Re-derived | 148 |
| Method | `git -C izcr ls-tree -r --name-only origin/main \| grep '\.proto$' \| grep -v '^pkg/apispec/protoext/' \| while read f; do git show origin/main:$f; done \| grep -cE '^\s*(get\|post\|put\|delete\|patch):\s*"'` |
| Ref | izcr@d3a0ca5 (2026-08-13) |
| Excluded | `pkg/apispec/protoext/google/api/**` — vendored Google protos (apikeys, serviceusage, servicecontrol, servicemanagement, cloudquotas) that izcr carries but does not implement. Including them is what produced 189. |
| Delta | −41 (−21.7%) |
| Verdict | fail |
```

The `Excluded` row is the entire difference between 189 and 148. In a bare number it
is invisible; in this record it is the first thing a reader checks.

---

## How a calling stage uses this

At the top of any stage that consumes a prior artifact:

1. List every figure inherited. Not "the important ones" — every one, since the IAM
   failures were in figures nobody thought were load-bearing.
2. Invoke this skill per figure.
3. Paste each record into the stage MANIFEST under `## Verification records`.
4. If any verdict is `fail`, correct the inherited figure **and re-verify**. The stage
   stays `in-progress` until every record is `pass` or a `blocked` record names what
   would unblock it.

The orchestrator's uniformity check rejects a `complete` MANIFEST containing a `fail`.

---

## Verification of the verification

```bash
S=docs/codeconverter/<stage>

# Every record has the mandatory fields, and Excluded is never empty
python3 -c "
import json,glob
bad=[]
for p in glob.glob('$S/verification/*.json'):
    d=json.load(open(p))
    need=('claim','claimed_by','rederived','rederive_method','excluded','delta','verdict')
    if any(d.get(k) in (None,'',[]) for k in need): bad.append((p,'missing field'))
    if d['verdict'] not in ('pass','fail','blocked'): bad.append((p,'bad verdict'))
print('malformed records:', bad or 'none')"

# No fails survive into a complete stage
python3 -c "
import json,glob
f=[json.load(open(p))['claim_id'] for p in glob.glob('$S/verification/*.json')
   if json.load(open(p))['verdict']=='fail']
print('failing records:', f or 'none')"

# The stage's own MANIFEST carries every record
grep -c '^### Verification — ' $S/MANIFEST.md
ls $S/verification/*.json 2>/dev/null | wc -l
```

The last two numbers must match. A record written to JSON but not into the MANIFEST is
a verification a human reader will never see.
