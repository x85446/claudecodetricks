<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Stage note — read first.** Unlike stages 01–11, this stage was not adapted from
> the legacy "codeplanner" process; that process had no provenance phase, which is
> precisely how it analysed a six-month-old target tree without noticing. There is no
> phase-number translation table to apply and no journey.md/journal in this pipeline.
> Where this document conflicts with the stage's SKILL.md output contract (uniform
> headers, MANIFEST.md, output directory), **SKILL.md wins**.

---

# Stage 00-provenance — Source Provenance

## Mission

Pin every tree the pipeline reads to a named, fetched, dated commit — and make the
pin checkable.

A figure computed from an unpinned tree is not wrong, exactly. It is unverifiable,
which is worse, because it looks the same as a right answer and survives longer.

---

## The two statuses every repo row carries (keep this in view while working)

Every row carries **two independent statuses**, and conflating them is the single
easiest mistake to make here.

**`state` — is the pinned ref itself admissible?**

| `state` | Meaning | May a stage cite figures against this ref? |
|---|---|---|
| `current` | fetched this session; the ref resolves to a SHA | yes |
| `unfetched` | fetch failed, or no remote configured | **no** |

**`working_tree` — how does the checked-out branch relate to the pinned ref?**

| `working_tree` | Meaning | May a stage **grep the working tree** and cite the result? |
|---|---|---|
| `same` | the checkout is the pinned ref | yes |
| `ahead` | local has commits the pinned ref lacks, but contains it | yes, noting the local-only commits |
| `behind` | local is an ancestor, N commits behind | **only after checking whether the N commits touch what was scanned** |
| `unrelated` | local shares **no ancestry** with the pinned ref | **no** — a scan of this tree is a scan of a different codebase |

Why both are needed: a stage `grep`s the **working tree**, not the ref it cites. A repo
can be perfectly pinned (`state: current`) while every grep result comes from a
seven-month-old checkout (`working_tree: behind`). Recording only the pin makes that
invisible — the row looks clean and the figures are still wrong.

`unrelated` is the state a freshness check alone misses. "Behind" is a distance;
"unrelated" is a different object. They are not points on the same scale, and
`git rev-list --left-right --count` prints a plausible-looking pair of numbers for
both.

**When `working_tree` is `behind`, the row must say whether it mattered.** Do not
guess: diff the pinned ref against the checkout, restricted to the paths the stage
actually scanned, and record the result. That check is cheap and it is the difference
between "possibly stale" and "these findings stand".

---

## Step 0 — Enumerate the trees

```bash
# From STATE.md, or from the orchestrator's init arguments
TARGET=<target repo path>
SOURCE=<source service repo path>
SIBLINGS=<sibling repos path>
MANIFESTS=<deployment manifests path>

for d in "$TARGET" "$SOURCE" "$MANIFESTS" "$SIBLINGS"/*/; do
  [ -d "$d/.git" ] && echo "$d"
done | sort -u
```

Every path printed gets a row. So does any path that *should* have printed and did
not — a directory under the siblings path with no `.git` is a hand-copy, and that is a
provenance finding in its own right.

## Step 1 — Fetch

```bash
for repo in <every repo>; do
  echo "=== $repo"
  git -C "$repo" fetch --all --prune 2>&1 | tail -3 || echo "FETCH FAILED"
done
```

Fetch only. **No checkout, no pull, no merge, no branch creation, no push.** This
stage records state; it does not change it. A pipeline that quietly moves a user's
working tree to pin a ref has done something far worse than analyse a stale one.

Record the fetch result per repo, including failures, with the error text.

## Step 2 — Resolve the ref

For each repo, decide the ref the pipeline will cite, then record it:

```bash
R=<repo>
git -C $R rev-parse --abbrev-ref HEAD                 # what is checked out
git -C $R symbolic-ref refs/remotes/origin/HEAD       # what upstream calls default
git -C $R rev-parse <ref>                             # the SHA — this is the pin
git -C $R log -1 --format='%cI  %s' <ref>             # date and subject
git -C $R remote get-url origin
```

Prefer, in order: the ref the user or STATE.md names; `origin/HEAD`; the upstream
default branch. A local-only branch is chosen **only** with a written reason.

## Step 3 — Measure divergence, and check ancestry separately

```bash
# The easy number
git -C $R rev-list --left-right --count <local>...<upstream>   # "N  M" = ahead behind

# The check that number cannot make. Run BOTH.
git -C $R merge-base <local> <upstream>; echo "merge-base exit=$?"
git -C $R rev-list --max-parents=0 <local>
git -C $R rev-list --max-parents=0 <upstream>
git -C $R branch -r --contains $(git -C $R rev-parse <local>)
```

Read them together:

- `merge-base` exits non-zero **and** the two root commits differ → `working_tree` is
  `unrelated`. The left/right count is meaningless; report it only alongside the
  ancestry evidence that explains why it is meaningless.
- `branch -r --contains` prints nothing → the ref was never pushed. It is not a
  target of record no matter how carefully a prior run pinned it.
- Otherwise the left/right count means what it says: `same`, `ahead` or `behind`.

Then, for every repo whose `working_tree` is `behind` or `ahead`, run the check that
turns a worry into a fact:

```bash
# Did the drift touch anything a stage actually scanned?
git -C $R diff --name-only <local>..<upstream> -- <each path a stage cited>

# Did upstream gain files a scan of the working tree could not have seen?
git -C $R grep -l '<the pattern the stage searched>' <upstream> | sed 's#^[^:]*:##' | sort > /tmp/up
git -C $R grep -l '<the pattern the stage searched>' <local>    | sed 's#^[^:]*:##' | sort > /tmp/lo
comm -13 /tmp/lo /tmp/up      # files upstream has and the scanned tree does not
```

The second command is the one that pays. A diff over cited paths tells you existing
findings still hold; only the `comm` tells you what the scan **could not have found**.

## Step 4 — Name the target-of-record

The target codebase gets one designated ref that **every** claim about it cites. Write
it into `provenance.md` and into STATE.md, with a verification command a reader can
paste:

```markdown
**Authoritative ref:** origin/main
**Commit SHA:** <full sha>
**Dated:** <commit date> — <subject>
**Pinned:** <today>, after `git fetch`

$ git -C <target> rev-parse origin/main
<full sha>
```

If a different ref was rejected, record the rejection with its numbers — not as a
footnote, as a labelled section. The next reader will find that branch and needs to
know why it is not the answer.

## Step 5 — Write the outputs

### `provenance.md`

```markdown
<!-- codeconverter artifact -->
**Stage:** 00-source-provenance
**Artifact:** provenance.md — every tree the pipeline reads, pinned and dated
**Status:** final
**Produced by:** codeconverter-00-source-provenance on YYYY-MM-DD
**Inputs:** docs/codeconverter/STATE.md; the repos listed below

---

# <Conversion> — Source Provenance

**Fetched:** YYYY-MM-DD (this session)

## Target-of-record
<per Step 4>

## Provenance table
| Repo | Ref | SHA | Commit date | Ahead | Behind | Merge base | State | Working tree |
|---|---|---|---|---|---|---|---|---|

## Findings
### PROV-00N — <repo> (<severity>)
<what drifted, the impact CHECKED rather than assumed, the commands, the action>

## Rejected refs
### <ref> — rejected
<reason, with the merge-base / root-commit / branch -r evidence>

## Repos with no remote
<the hand-copies; each one is a provenance risk>

## Fetch failures
| Repo | Error | Consequence |
```

### `provenance.json`

```json
{
  "fetched_at": "YYYY-MM-DDTHH:MM:SSZ",
  "target_of_record": {"repo": "...", "ref": "origin/main", "sha": "...", "date": "..."},
  "repos": [
    {"name": "...", "path": "...", "remote": "...", "ref": "...", "sha": "...",
     "commit_date": "...", "local_branch": "...", "ahead": 0, "behind": 0,
     "merge_base": "yes", "state": "current", "working_tree": "same",
     "fetched_at": "...", "commands": ["git -C ... rev-parse origin/main"]}
  ],
  "rejected_refs": [{"ref": "...", "reason": "...", "evidence": "..."}],
  "findings": [{"id": "PROV-001", "repo": "...", "severity": "material",
                "text": "...", "impact": "...", "commands": [], "action": "..."}],
  "fetch_failures": []
}
```

---

## How other stages use this

Any stage that reads an external tree asserts against this table before computing
anything:

```bash
python3 -c "
import json,sys
p=json.load(open('docs/codeconverter/00-source-provenance/provenance.json'))
need='<repo name>'
r=next((x for x in p['repos'] if x['name']==need), None)
if not r: sys.exit(f'{need} has no provenance row — do not analyse it')
if r['state']=='unfetched': sys.exit(f'{need} is unfetched — figures from it are not admissible')
if r['working_tree']=='unrelated': sys.exit(f'{need} working tree is unrelated to the pinned ref — a scan of it is a scan of a different codebase')
if r['working_tree']=='behind': print(f'WARNING {need} working tree is {r[\"behind\"]} commits behind {r[\"ref\"]} — check whether the drift touches what you scan')
print(need,'pinned at',r['sha'][:12],r['commit_date'],'state',r['state'],'working_tree',r['working_tree'])"
```

Add to stages 03, 07 and 09 the exit criterion: *every external repo this stage read
appears in the provenance table with a fetch timestamp no older than this stage's
start date.* A stage that finds a stale or missing row re-invokes this skill rather
than proceeding.

---

## Verification before you declare done

```bash
J=docs/codeconverter/00-source-provenance/provenance.json

# Every repo has the full row
python3 -c "
import json;d=json.load(open('$J'))
need=('name','path','remote','ref','sha','commit_date','state','fetched_at')
bad=[r.get('name','?') for r in d['repos'] if any(not r.get(k) for k in need)]
print('rows missing a field:', bad or 'none')"

# Every row carries the command that produced it
python3 -c "
import json;d=json.load(open('$J'))
print('rows with no command:', [r['name'] for r in d['repos'] if not r.get('commands')] or 'none')"

# Every fetch is from this session
python3 -c "
import json,datetime;d=json.load(open('$J'))
today=datetime.date.today().isoformat()
print('rows not fetched today:', [r['name'] for r in d['repos'] if not r['fetched_at'].startswith(today)] or 'none')"

# Both statuses are valid, and the target-of-record is admissible
python3 -c "
import json;d=json.load(open('$J'))
print('invalid state:', [r['name'] for r in d['repos'] if r['state'] not in {'current','unfetched'}] or 'none')
print('invalid working_tree:', [r['name'] for r in d['repos'] if r['working_tree'] not in {'same','ahead','behind','unrelated'}] or 'none')
t=d['target_of_record']
row=next(x for x in d['repos'] if x['name']==t['repo'])
print('target-of-record: state',row['state'],'working_tree',row['working_tree'],
      '| pin ADMISSIBLE' if row['state']=='current' else '| pin NOT ADMISSIBLE')"

# Every repo whose working tree drifted has a finding saying whether it mattered
python3 -c "
import json;d=json.load(open('$J'))
drift={r['name'] for r in d['repos'] if r['working_tree'] in ('behind','unrelated')}
covered={f['repo'] for f in d.get('findings',[])} | {x['repo'] for x in d.get('rejected_refs',[])}
print('drifted repos with no finding:', sorted(drift-covered) or 'none')"

# The pin verifies against the actual repo, right now
git -C <target> rev-parse <ref>
```

Paste this output into the MANIFEST, including the live `rev-parse`. The whole point
of this stage is that its claims are re-runnable; a provenance table that does not
show its own verification has reproduced the failure it exists to prevent.

---

## Exit Criteria

Copy the exit criteria from `SKILL.md` into `MANIFEST.md` and check them honestly.
Every downstream stage's numbers inherit the correctness of this table.
