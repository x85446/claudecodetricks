---
name: "product"
description: "PRODUCT — the product-definition meta: staged brief, competitors, PRD, features, roadmap, requirements, architecture, engineering and milestones docs. Routes all product-definition work; picks the child. \"define a new product\", \"what's our MVP\", \"which features ship in v1\", \"what order do we build it in\", \"break the roadmap into milestones\", \"is this worth building\", \"PRFAQ\", \"working backwards\". NOT end-user instructions ($user-docs), NOT building it ($iterate)."
---



<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# $product — idea to engineering-ready definition

**Version:** product family 1.3.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Nine documents, each earned from the one before it. The chain exists so that by the end **the MVP is not an opinion**: it is the top release slice of a feature inventory that came from a story map, built on a PRD that came from a real competitive landscape.

## Usage

Argument: "[status | next | <stage> | maintain | unlock <stage>] [--fast]". `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$ip` — ported.
- `$iterate` — ported.
- `$product-architecture` — ported.
- `$product-brief` — ported.
- `$product-competitors` — ported.
- `$product-engineering` — ported.
- `$product-features` — ported.
- `$product-iteratemilestones` — ported.
- `$product-milestones` — ported.
- `$product-prd` — ported.
- `$product-requirements` — ported.
- `$product-roadmap` — ported.
- `$user-docs` — ported.

## Children

| # | Stage | Child | Writes |
|---|---|---|---|
| 0 | Brief | `$product-brief` | `docs/brief.md` |
| 1 | Competitors | `$product-competitors` | `docs/competitors.md` + `docs/competitive_features.md` |
| 2 | PRD | `$product-prd` | `docs/prd.md` |
| 3 | Features | `$product-features` | `docs/features.md` |
| 4 | Roadmap | `$product-roadmap` | `docs/roadmap.md` |
| 5 | Requirements | `$product-requirements` | `docs/requirements.md` |
| 6 | Architecture | `$product-architecture` | `docs/architecture.md` |
| 7 | Engineering | `$product-engineering` | `docs/engineering.md` |
| 8 | Milestones | `$product-milestones` | `docs/milestones.md` |

After stage 8 comes the **plans step**: `$product-iteratemilestones` turns `docs/milestones.md` into one `$ip` plan per milestone and chain-stages them. It writes no document and has no gate. It is done once a plan in `./.claude/iterate/plans/` or `archive/` carries a `milestone:` key.

Invoke children with explicit `$name` invocation. Never write a stage's document yourself — the child owns its doc, and the meta owns the order, the gate and the state.

## Invocation

```
$product                      # resume at the first unfinished stage
$product status               # the state table, nothing else
$product next                 # run exactly one stage, then stop
$product <stage>              # jump to a stage by name or number
$product plans                # milestones → one chained $ip plan each (runs by itself after stage 8)
$product maintain             # all nine done: reconcile docs against reality
$product unlock <stage>       # reopen a locked document for revision
$product --fast               # run the whole chain unattended
```

## Step 1 — Read state, or establish it

State lives at `./.claude/product/state.md`. Read it first, every invocation.

If it does not exist, create it — and **detect the mode before asking the user anything**:

- **Brownfield** — the repo contains source (any language's project file, a `src/`, a `Makefile`, a non-trivial git history). The product partly exists. Read the code, the README, and `$user-docs` output if present, and pre-fill what the product already does. The user then confirms and fills gaps rather than dictating from scratch.
- **Greenfield** — no source, or source is scaffolding only. Everything comes from the user.

Say which mode you detected and what you found, in one line. A wrong detection is cheap to correct and expensive to hide.

```markdown
# product — <product name>

mode: greenfield | brownfield
gate: on | fast
verdict: — | go | needs-clarification | kill
started: <YYYY-MM-DD>

| # | stage | doc | status | locked | updated |
|---|-------|-----|--------|--------|---------|
| 0 | brief | docs/brief.md | pending | no | — |
| 1 | competitors | docs/competitors.md | pending | no | — |
| 2 | prd | docs/prd.md | pending | no | — |
| 3 | features | docs/features.md | pending | no | — |
| 4 | roadmap | docs/roadmap.md | pending | no | — |
| 5 | requirements | docs/requirements.md | pending | no | — |
| 6 | architecture | docs/architecture.md | pending | no | — |
| 7 | engineering | docs/engineering.md | pending | no | — |
| 8 | milestones | docs/milestones.md | pending | no | — |

## Open clarifications
- [ ] <stage> — <the question, verbatim from the marker>
- [x] <stage> — <question> → <the answer, dated>

## Stage notes
<decisions and corrections the user gave mid-stage, dated, newest first>

## Drift log
<maintain-mode findings, newest first>
```

One line per marker, checkbox unchecked while it is open. The gate counts unchecked lines; prose here is not countable and does not exist as far as the gate is concerned.

A state file written before a stage existed (one ending at row 7) gains the missing row as `pending` on the next read, so the next run picks it up rather than entering maintain mode.

`status`: `pending` · `in-progress` · `done` · `blocked`. `locked`: `yes` once the user approves it, and a locked document is never rewritten without `$product unlock <stage>`.

## Step 2 — Pick the stage

1. `status` → print the table and stop.
2. An explicit stage → run that one (warn, don't refuse, if its predecessors are unfinished: a PRD written before the landscape is a guess, and the user may know that).
3. All nine `done`, but no plan carries a `milestone:` key → **the plans step**. Invoke explicit `$name` invocation with `product-iteratemilestones` and stop when it reports. A finished definition with nothing planned has nothing built to reconcile, so maintain mode there would audit docs against code that does not exist yet.
4. All nine `done` and milestone plans exist → **maintain mode** (Step 6).
5. Otherwise → the first stage that is not `done`. Resume mid-stage work rather than restarting it.

## Step 3 — Run the stage

Set `in-progress`, invoke the child, let it write its document.

Two rules the children inherit and the meta enforces:

- **Never guess a fact you were not given** — an assumption that reads like a finding is the one failure that corrupts every downstream stage. But an unknown has three dispositions, and only one of them is a question for the user:

  | The unknown is… | Marker | Who clears it |
  |---|---|---|
  | Something a person with a browser could find out — a competitor's capability, a pricing tier, whether a product does X | `[NEEDS RESEARCH] <what to find>` | **The child, before its stage ends.** Go crawl, fetch, search. This marker is a to-do, not an output; it never reaches the gate. |
  | Something only the user knows or decides — preference, scope, priority, budget, a business fact with no public source | `[NEEDS CLARIFICATION] <the question>` | The user, in Step 4.5. |
  | Something nobody can know until the product or its data exists — "how does it perform on Melissa's real documents" | No marker. Write it under `## Deferred validations` as a planned check. | Stage 5 turns it into a requirement's acceptance criterion; stage 7 into a test. |

  The test between the first two: *could I find this out myself if I tried?* If yes, it is research and you owe it. Research that genuinely comes up empty — site blocked, nothing public — becomes a **cited absence**: "Not publicly documented as of <date>; checked <sources>." That is a finding with a source, not a marker. On symude, "which reusable engines provide learned indexes" and "does enCodePlus connect cross-city research to AI drafting" were both research the child owed, filed as questions for the user and labelled "research I owe; still open" — the exact thing this table forbids.
- **Stages 0–4 describe WHAT and WHY only.** No tech stack, no schema, no API shape, no library names. HOW begins at stage 6. Stage 8 is sequence: it orders what stages 4–7 already decided and adds no scope of its own. A technology named in the PRD is a decision nobody made, smuggled past the architecture stage.

## Step 4 — Analyze before the gate

Every stage ends with a consistency pass over what now exists. Report it as a short block — findings only, and `analyze: clean` when there are none.

- **Contradictions** — this document against every earlier one.
- **Gaps** — a required section that is empty or hand-waved.
- **Orphans and dangling ids** — an `R-NN` naming a nonexistent `F-NN`, a feature in no release slice, a release slice with no features, a competitor claim with no source, a requirement in no milestone or in two.
- **Altitude violations** — HOW leaking into stages 0–4.
- **Unresolved markers** — every `[NEEDS CLARIFICATION]` still open, named.
- **Unfinished research** — any `[NEEDS RESEARCH]` still in the document. This is a defect in the child's work, not a question for the user: **re-enter the child's research on exactly those items** (one subagent per item, in parallel) and re-run analyze. Two passes without progress → write the cited absence and move on. The user never sees a `[NEEDS RESEARCH]`.

Findings are fixed before the gate, not filed for later. Markers are not filed either — research is done, clarifications are asked, next.

## Step 4.5 — Resolve every marker before the gate

A `[NEEDS CLARIFICATION]` is a question the child could not answer. The user can. Ask them now, marker by marker, before anything is presented for approval — never fold the questions into the approval prompt, because "approve and continue" then silently approves the gaps too. That is exactly how markers survived stages 0–1 on symude.

1. Collect every open marker in this stage's document (and any earlier document still carrying one).
2. Ask with a plain numbered-list question, up to four per call. **Choices, not essays**: every option set is drawn from what the research actually turned up, and `multiSelect: true` whenever the honest answer could be several things (which formats, which platforms, which categories). Free text is for answers that are genuinely a name, a number, or a sentence nobody could enumerate — and even then, offer the likely candidates and let "Other" carry the rest. "I don't know yet" is always an option. Never ask in prose what a question block could ask in options: a paragraph that ends "should we use UDCs, PRDs and SOPs?" is a three-checkbox question wearing a coat.
3. Write each answer into the document at the marker's position, delete the marker, and tick the state-file line with the answer and date.
4. "I don't know yet" keeps the marker and the state line open. It is a legitimate answer, and it is the only way a marker reaches the gate still open.

Only when this loop has run does the gate open.

## Step 5 — The gate

**`gate: on` (default).** Present the document's headline content, its `## Calls made` list (the judgment calls the child made without asking), and the analyze block. Then ask for approval with a plain numbered-list question: approve and continue · approve and stop · revise (say what) · skip this stage. On approval set `done` and `locked: yes`, stamp the date, and continue to the next stage — or stop if that is what they chose.

**Open markers block approval.** If the state file still has an unchecked clarification for this stage after Step 4.5, the approval options are not offered. Instead: list the open questions, and offer revise · ask me again · **approve with open questions** (override). The override is the only way through, it must be chosen explicitly, and it is recorded on the stage row as `override: N open` — a locked document with a known hole says so in the state table forever, and every later stage's analyze pass names the inherited marker. Nobody arrives at stage 5 surprised that stage 1 never settled who the competitors were.

**`gate: fast` (`--fast`).** Run the whole chain without stopping, then present all nine documents and every analyze block together. Set every completed stage `done` but `locked: no` — nothing the user has not seen gets locked. Markers accumulate during the run; the final review runs Step 4.5 over all of them first, then gates each stage in order under the same open-marker rule.

**`--fast` does not override a `kill`.** If stage 0 returns `kill`, stop there and report it even in fast mode. Running six more stages on an idea the brief just killed is precisely the waste stage 0 exists to prevent. A `needs-clarification` verdict stops fast mode too — it means the brief could not answer its own questions, and everything downstream would inherit the guesswork.

## Step 6 — Maintain mode

All nine `done`, milestone plans written, and invoked again: the docs stop being a deliverable and become a claim about reality. Check the claim.

1. **Reconcile** each document against what is now true — the code as it stands, shipped behavior, the roadmap against what actually released, milestones against what main actually delivers, competitors re-checked if the research is over 90 days old. A release that has shipped means the next release's ten milestones get re-cut against what it proved.
2. **Report drift** as a list: the document, the stale claim, and what is true now. Append to the state file's drift log with the date.
3. **Fix on request.** Maintain mode reports; it rewrites a locked document only when the user says to. Unlocking is explicit.
4. Nothing drifted → `docs already true — no changes` and touch nothing.

## Output

Always end with the state table and one line naming what happens next:

```
product: newproduct  ·  brownfield  ·  gate on
  0 brief          done         2026-09-16
  1 competitors    done         2026-09-16
  2 prd            in-progress  —
  3 features       pending      —
  …
open clarifications: 2   ·   next: finish stage 2 (prd)
```

## Rules

1. **The meta never writes a stage document.** It routes, gates, and keeps state.
2. **Order is the product, not bureaucracy.** Each stage consumes its predecessors; skipping one means the skipped reasoning is now an assumption, and the analyze pass will say so.
3. **`[NEEDS CLARIFICATION]` over a plausible guess**, always — and every marker is put to the user before the gate, one question each. Approval with a marker still open exists only as an explicit, recorded override.
4. **A locked document needs an explicit unlock.** The user approved that text; do not quietly improve it.
5. **Stages 0–4 are WHAT/WHY. Stage 5 is testable behavior. Stages 6–7 are HOW. Stage 8 is order.**
6. **Traceability is enforced, not decorative.** Every requirement names its feature and its release. Orphans are reported as defects.
7. **This family stops at documents.** It does not implement, and no stage writes an iterate plan. Stage 8 shapes each milestone so `$ip` can take it as-is, and the plans step hands them over: typing `$product` past stage 8, or `$product-iteratemilestones` directly, is the user's move to `$ip`.
8. **`docs/` is the home**, alongside the code, in whatever repo `$product` runs in. Never a parallel doc tree beside a live one.
9. **`$user-docs` is downstream and separate** — it documents how to operate what shipped. Never edit end-user docs from here.
10. **Every document states the current decision, never its history.** A revision rewrites the affected text to the new truth. Nothing in it says "revised", "previously", "now" or "changed from". The state file's stage notes and drift log are the record of what changed and why.
