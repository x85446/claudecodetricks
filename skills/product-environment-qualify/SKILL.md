---
name: product-environment-qualify
description: "PRODUCT stage-9 child (invoked via /product-environment): walks the operator's decision tree of qualifying questions in questions.md, answers each from evidence, and returns the obligations the chosen branches impose on every environment, each with its probe and status."
version: 1.7.0
---

# /product-environment-qualify — the qualifying questions

Stage 9 opens with one list of yes/no questions whose answers decide what every environment must already have before the first plan launches: a build box, every guest in `.iz`, every release machine. The list is a decision tree the operator owns. This skill walks it, answers each question from evidence, follows the branch, and establishes every obligation at the leaf with a real probe in every environment. It returns the walk as one section for `product-environment` to place in `docs/environment.md`.

## The tree — `questions.md`

The tree lives beside this file as [questions.md](questions.md). **Only the operator edits it.** The grammar:

| Line | Means |
|---|---|
| `### Q<n> — <question>` | A yes/no question. |
| `- evidence: …` | Where its answer comes from: a document section, a command, `/accounts`. |
| `- yes → <Q<m> \| OBL-<x> \| end>` / `- no → …` | The branch each answer takes. |
| `### OBL-<x> — <title>` | An obligation block the leaf names. |
| `- [ ] <obligation> — probe: <what proves it>` | One obligation and the observation that establishes it. |

## Steps

1. **Read** `questions.md` and the inputs `product-environment` step 1 read: `docs/engineering.md`, `docs/architecture.md`, `./.iz`, `.claude/data/accounts.md`, `/accounts show`.

2. **Walk from Q1.** Answer each question in the family's three dispositions, and never by guessing:
   - Evidence in a document or on the machine → the answer, cited as `path:line` or as the command and its output line.
   - The `evidence:` line names a command → run it and answer from its output. Empty output is a cited absence, not a `no`.
   - Nothing answers it → `[NEEDS CLARIFICATION] Q<n>: <question>`. The walk stops there; the obligations beneath it are `pending clarification`. `product-environment` puts the question to the user at the gate as a choice, and the next run walks past it with the answer from `.claude/product/state.md`.

   Follow the branch. `end` with no obligation block means the tree imposes nothing, and the section says so.

3. **Establish each obligation** at the leaf, in every environment it covers: the owner's machine, every `[machine.<key>]` in `./.iz`, and every environment the deploy table in `docs/engineering.md` names. Run the probe in each. Status per obligation:

   | Status | Means |
|---|---|
   | `ready` | The probe passes in every environment, with its date. |
   | `operator: <one act> — <first milestone>` | A human must act once: grant a permission on the real Mac, hold an identity nobody holds. |
   | `gap: <what>` | An engineering decision is missing or contradicts the branch (the permission-holder is ad-hoc signed under OBL-A; no grant command exists under OBL-B). Reported for the user to unlock `docs/engineering.md`. |
   | `unmet: <environment>` | The probe fails in a guest `product-environment` provisions; its step 4 (Machines) does the work and re-probes. |
   | `pending clarification` | Beneath an unanswered question. |

   One environment failing makes the obligation that status, never `ready`.

4. **Return the section** below. Invoked by `product-environment`, that is the whole output. Invoked standalone (`/product-environment-qualify`), print it, and when `docs/environment.md` exists replace its `## Qualifying questions` section in place and change nothing else in the file.

## Output — the `## Qualifying questions` section

```markdown
## Qualifying questions

Walked <YYYY-MM-DD> against `questions.md`.

| Q | Question | Answer | Evidence | Branch |
|---|---|---|---|---|
| Q1 | Are we compiling a Mac app? | yes | `docs/architecture.md:27` platform statement; `Makefile` `install` target signs with `codesign` | → Q2 |
| Q2 | Do we sign with Apple's official identity? | yes | `docs/engineering.md` Deploy: Developer ID Application, team 53NRLZ85KW; `/accounts`: Apple Developer Program held | → OBL-A |

### OBL-A — Signed everywhere, permissions pre-set, headless
| Obligation | Environments | Proof | Status |
|---|---|---|---|
| Every environment holds the identity or receives binaries it signed | owner-build, mac-26, mac-27 | `codesign -dv --verbose=2 ~/.local/bin/fm` → `TeamIdentifier=53NRLZ85KW` in each · <date> | ready |
| Every OS permission pre-granted to the designated requirement | mac-26, mac-27, owner's Mac | guest daemon lists `~/Documents` · <date> | operator: grant Full Disk Access inside mac-27 after bootstrap — M1-10 |
```

A question with no evidence appears as `[NEEDS CLARIFICATION]` in the Answer column and the Branch column reads `stopped`. A walk that ends at `end` carries the question table and the line `No obligations: the tree ends at Q<n>.`

## Known paths

Reference for establishing the obligations, never a substitute for the probe:

- **SIP on an Incus macOS guest** flips headlessly with `osx-runtime sip <guest> on|off|status` (incusmachines); `osx-runtime <name> --sip on` launches a guest with it on. The osx-runtime golden images ship SIP off (`csr-active-config=0x67`), so `csrutil status` reads `unknown (Custom Configuration)` there, not `disabled`.
- **A permission under SIP on** cannot be written by script; it is granted once in the golden image to the stable designated requirement and baked into the `clean` snapshot (`product-environment` rule 11). A grant on the host never authorizes a guest.
- **A permission under SIP off** is a row in the TCC store the AI writes by command; it is keyed to the binary's code requirement, so an ad-hoc-signed binary needs the write again after every build.

## Rules

1. **The tree is the operator's.** This skill reads `questions.md` and never writes it: no added question, no reworded question, no answer recorded in it. A question the walk wished it had is reported under `## Calls made` as `question missing: <what was wanted>` for the operator to add.
2. **Never guess an answer.** A question without evidence is a clarification for the gate, and the obligations beneath it wait.
3. **Every environment, not the one at hand.** An obligation covers the owner's machine, every guest in `.iz` and every environment the deploy table names, and it is `ready` only when all of them are.
4. **Headless is the bar.** An obligation is met only when no later plan has to prompt a human for it. A probe that needs a console click fails the obligation.
5. **Re-walked every run.** Answers and probes are recomputed on every `/product environment`; nothing is cached between runs.
6. **It returns a section and writes nothing else.** `./.iz`, account pointers and `## Operator acts` are `product-environment`'s to write. Standalone, it touches one section of `docs/environment.md`.
7. **No secret value.** A probe names where an identity or a credential lives, never its value.
