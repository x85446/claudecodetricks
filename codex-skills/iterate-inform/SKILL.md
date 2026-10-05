---
name: "iterate-inform"
description: "Send ANOTHER project a one-time bug report about a problem this session ran into, so that project's AI can plan the fix later. Writes one file to <project>/.claude/iterate/inbox/ and acks in one line. Triggers on \"$iterate-inform\" or its alias \"$ii\", \"tell <project> that…\", \"let <project> know…\", \"inform <project>\", \"file this with <project>\", \"report this to <project>\". The receiving project reads it with $ip show inbox and plans it with $ip plan the inbox. Never plans, fixes or executes anything itself."
---

<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# $iterate-inform — Tell the project that owns it

**Version:** iterate family 5.12.1

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


| Skill | Role |
|---|---|
| `$iterate-notes` (`$in`) | **Capture** an idea for this project's next plan |
| `$iterate-inform` (`$ii`) | **Inform** another project of a problem it owns |
| `$iterate-brainstorm` (`$ibs`) | **Decide** between options |
| `$iterate-planner` (`$ip`) | **Plan**: `$ip plan the inbox` turns received informs into a plan |
| `$iterate` (`$i`) | **Execute** autonomously |

## Usage

Argument: <project> <what to tell them — an instruction to the AI, e.g. "tell them what you need deployed">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$i` — ported.
- `$ibs` — ported.
- `$ii` — ported.
- `$in` — ported.
- `$ip` — ported.
- `$it` — ported.
- `$iterate` — ported.
- `$iterate-brainstorm` — ported.
- `$iterate-notes` — ported.
- `$iterate-planner` — ported.

Use it when this session hit a problem whose fix lives in **another
project**: its code, its deployment, its config. This session can't fix it
there and shouldn't. The other project's AI has none of this session's
context, so this skill writes what it would need, as a bug report, and drops
it in that project's inbox. That session reads it with `$ip show inbox` and
plans it with `$ip plan the inbox`.

Real cases:
- izuma-dm-platform's plan `gar` is blocked because teleport runs a stale
  exonet build. site-infra owns teleport's deployment, so
  `$ii site-infra tell them what you need deployed` reports it there.
- A filemaster intake session finds that OCR returned empty on a readable
  page. That's a filemaster code defect, so `$ii filemaster` reports it to the
  filemaster repo.

It is fire-and-forget: one write, one line, done. There is no reply channel.
The sender resumes its own work by hand once the other project has acted.

## Arguments

`$1` is the target project. Everything after it is an **instruction to you**
about what to tell them ("tell them what you need deployed", "explain the OCR
miss"). It is never text to quote into the report. You write the report from
what this session knows.

- No instruction → report the problem this session is currently stuck on or
  most recently identified as belonging to that project.
- No instruction **and** no such problem in context → reply
  `nothing to tell <project> — say what it should know` and stop.

## 1. Resolve the target project

A wrong target is the one mistake that sends the report nowhere, so this is
the one place you may ask.

1. `$1` is a path (contains `/`, or starts with `~` or `.`) and is a directory
   → use its git root (`git -C <dir> rev-parse --show-toplevel`), or the
   directory itself outside git.
2. Otherwise build the candidate list: the projects iterate-run knows, plus
   every git repo under `~/workspace` (projects that never had a plan still
   qualify):

   ```bash
   { jq -r '.[]' ~/.claude/iterate-run/projects.json 2>/dev/null
     find ~/workspace -mindepth 2 -maxdepth 4 -name .git -prune 2>/dev/null | xargs -n1 dirname
   } | grep -vE '^(/private)?/tmp/|/\.' | sort -u
   ```

   The filter drops scratch projects under `/tmp` and copies inside hidden
   directories (Syncthing's `.stversions` holds a second `site-infra`).
3. Exact basename match, case-insensitive. One hit → it.
4. No exact hit → near matches: one or two typos away (`site-ifra` →
   `site-infra`), or a unique prefix or substring. Exactly one → use it, and
   the ack names the correction.
5. Several hits at any stage (`izcrOS` and `izcros`), or several near matches
   → ask the user to pick from them as a choice. None → reply
   `no project named <x> — give a path` and stop.
6. The target resolves to **this** project's root → write nothing and reply
   `that's this project — $in takes a note here`.

## 2. Write the report

The report is a bug report for someone who wasn't here. Write it in the
**target's** terms: its files, hosts, make targets and repo-relative paths.
Leave out anything that only makes sense inside this session.

```markdown
# Inform — <one-line title of the problem>

From: <sender project> (<sender abs path>) · plan <name or none> · branch <branch> · commit <12-char sha>
Sent: <UTC, YYYY-MM-DDTHH:MM:SSZ>
status: open

## Problem
<What is wrong, observed vs expected, in 1–3 sentences.>

## Evidence
- <One fact per bullet, each with how it is known: `command` → the output
  line that shows it, path:line, a commit sha, a log line, a URL.>

## Needed
<The concrete change or act, as specific as you actually know it: exact
refs, files, commands. Name the outcome, not a plan.>

## Done when
<An observable check that shows it is fixed, runnable in the target's world,
or from the sender's side if that is the only place it is visible (say which).>

## Blocks
<What on the sender's side is waiting on this, and how the sender resumes,
e.g. `izuma-dm-platform plan gar — resume with $iterate gar`. Omit the whole
section when nothing is waiting.>
```

Section rules:

- **Evidence is what this session saw.** Cite the command and the output
  line, or the file and line. Something you are inferring rather than saw is
  prefixed `inferred:`. This is a one-time write, not a fresh investigation.
  A quick read-only lookup is fine to fill a gap (the full sha, the file
  that pins a ref), but don't re-run the diagnosis.
- **Needed is at the altitude you know.** If you know the exact fix (bump
  `exonet.ref` to `f2f8b39dad08` or later, then `make deploy HOST=teleport`),
  write it. If you only know the symptom, say so. Never invent steps, and
  never write their plan for them; that is their planner's job.
- **Self-contained.** Use absolute paths for the sender's side,
  repo-relative ones for the target's, and shas of at least 12 characters.
  No "as above" and no "this session". Output excerpts run to a few lines,
  never a transcript.
- **No secrets.** Never copy a token, password, key or credential into the
  report. Name where it lives instead.
- **Use the sender's identity from real reads:** the project root's basename,
  `./.claude/iterate/current` for the plan (`none` when absent),
  `git branch --show-current`, `git rev-parse --short=12 HEAD`.

## 3. Drop it in the inbox

```
<target>/.claude/iterate/inbox/<YYYYMMDDTHHMMSSZ>-<sender>-<slug>.md
```

`<slug>` is a kebab-case name for the problem, five words at most
(`teleport-stale-exonet-build`). Create `inbox/` if it is missing; the target
need not have any plans yet.

**One problem, one open file.** Before writing, read the target's open
inbox items. If one from this sender already reports this same problem,
rewrite that file in place with the current report and leave its filename
as it is. Never queue a second copy for their planner to plan twice.

Never `git add` or commit the file, and never touch the target's git
otherwise. Inbox files are local iterate state, like notes.

## 4. Ack in one line

```
→ informed site-infra: teleport runs a stale exonet build (.claude/iterate/inbox/20261005T175800Z-izuma-dm-platform-teleport-stale-exonet-build.md)
```

Write `↻ updated` instead of `→ informed` when you rewrote an existing item,
and add `(from "site-ifra")` after the project name when it was a near match.
Nothing else: no reprint of the report and no next steps.

## What happens on the other side

The receiving project sees the open item in three places: `📥N` in its
statusline's ⚙️ segment, an `Inbox:` block in `$ip status`, and a problem in
`$it`. `$ip show inbox` reads it without touching it. `$ip plan the inbox`
builds a plan from it and marks it `status: consumed (plan: <name>)`. When
their user drops it, it becomes `status: dismissed (<why>)`. This skill never changes an item's status
after writing it.

## Rules (hard)

1. **One file, one place.** Write the inbox file and nothing else: not the
   target's plans, notes, code or git, and nothing in this project.
2. **Never plan, never fix, never execute**, even when the fix looks trivial
   and the target repo is right there. Fixing another project's problem is
   that project's plan, run on its branch, under its rules.
3. **The instruction is not the content.** The user's words tell you what to
   report. The report is yours, written from what you know.
4. **Never ask a question, except to pick the target** (step 1). Anything
   else unclear goes into the report as `inferred:`, or is left out.
5. **One line out.** The ack is the whole reply. A doubt or contradiction
   you noticed while writing goes into the report as `inferred:`, never into
   the reply.

## `version`

`version` (or "what version") on **any** iterate skill reports the same thing:
the family version, because the stack is versioned as one unit:

```
iterate family 5.0.0
iterate-run iterate-v3.3 (commit 4dd09ec5, built 2026-08-27_17:02:20)
```

**Both lines come from a real read, never from memory, the family line
included.** Run these two, from any directory:

```bash
grep -m1 '^version:' ~/.agents/skills/iterate/SKILL.md   # the family version
iterate-run version                                      # the binary
```

**Never quote the `version:` in the skill body you already have in context.**
A session loads a skill body once and keeps it, so after a `skillctl family
iterate set` and reinstall, the copy in context is stale.

If members disagree, say so and name them: drift inside the family is a
defect, not a state, and `skillctl family iterate set X.Y.Z` is the only
correct way to bump.
