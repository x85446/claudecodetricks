---
name: iterate-rules
description: Read and write the iterate launch policy for THIS project in plain language — "don't run iterate before 10pm", "require a keyword to launch", "weeknights only", "no runs over the holidays", "show the rules", "would a run start right now". Writes ./.claude/iterate/policy.md, which /iterate enforces at launch. Rules gate launching a run; they never touch plans, and never stop a run that is already going.
argument-hint: <the rule in your own words, or "show" / "test" / "remove <rule>" / "clear">
version: 1.1.0
---

<!-- version: bump on EVERY behavioral change (minor for additions, major for schema changes, patch for wording). The schema here is the contract /iterate reads; changing it means changing /iterate too. -->

# /iterate-rules — say when a run is allowed to start

The front door to `./.claude/iterate/policy.md`. You describe the rule; this
writes the schema `/iterate` actually enforces.

| Skill | Role |
|---|---|
| `/iterate-notes` (`/in`) | **Capture** |
| `/iterate-brainstorm` (`/ibs`) | **Decide** |
| `/iterate-planner` (`/ip`) | **Plan** |
| `/iterate-rules` | **Gate** — when a run may start |
| `/iterate` (`/i`) | **Execute** |

**Scope, and it is narrow.** These rules gate *starting* a run. They never
touch plans, never change what a run does, and — this one matters — never stop
a run already in flight. A run legitimately started at 23:00 keeps resuming at
07:00; see `/iterate`'s entry rules for why gating resumption would kill every
overnight run at dawn.

**Per-project, always.** The file lives in the project's own tree. Never write
a rule into a project the user is not currently in, and never carry one
project's rules to another.

## The schema

```markdown
---
require-launch-keyword: permission
launch-schedule:
  - allow mon-fri 22:00-06:00
  - allow sat,sun 20:00-08:00
  - deny 2026-12-24..2026-12-26
---

# Iterate policy — <project>

## Why the launch gate exists

<Free text. /iterate's refusal quotes this verbatim, so write it as the
sentence the user wants to read when they are told no.>
```

### `require-launch-keyword: <word>` — the lock rule

A fresh launch must carry `<word>` anywhere in its argument
(`/iterate owl permission`). One keyword authorizes one launch; it is not
remembered. **It also overrides the schedule** — see the evaluation order
below.

### `launch-schedule:` — the cron-like ruleset

A list, each entry `<allow|deny> [days] [time] [dates]`. Every component is
optional; an omitted component matches everything.

| Component | Forms | Omitted means |
|---|---|---|
| days | `mon` · `mon-fri` · `sat,sun` · `weekdays` · `weekends` · `daily` | every day |
| time | `HH:MM-HH:MM`, 24-hour, local | all day |
| dates | `YYYY-MM-DD` · `YYYY-MM-DD..YYYY-MM-DD` | every date |

**Evaluation, in this order:**

1. the launch keyword was required and supplied → **permit**, and report the
   override (unless `launch-window-strict: true`)
2. any `deny` matches now → **refuse**
3. else any `allow` matches now → **permit**
4. else, allow rules exist but none match → **refuse**
5. no `launch-schedule` at all → **permit** (no time gate)

Step 1 is the important one. Both gates answer the same question — *is the
machine free to spend?* The schedule **guesses** at it from hours the user is
usually away; the keyword is the **answer**, from the person who knows. When
they disagree the answer wins. Refusing someone who has just said they are
leaving for the afternoon is the gate mistaking its proxy for its purpose, and
it is the one failure that makes people stop trusting the whole mechanism.

`launch-window-strict: true` restores the hard behaviour for a project that
really does want the hours enforced against an explicit keyword.

Deny wins over allow, and the presence of any allow rule makes the schedule
default-deny. Both so that adding a rule can only ever narrow when runs
happen — a policy edit should never accidentally open a window.

**Two semantics that are easy to get wrong, and are the whole reason this is
written down:**

- **A time window wraps midnight when start > end.** `22:00-06:00` is 22:00
  through 05:59 next morning. A plain `start <= now <= end` refuses 02:00 as
  "before 22:00" — for this window that rejects every hour it exists to allow.
- **The day label matches the day the window STARTED, not the current
  instant.** `allow mon-fri 22:00-06:00` includes Saturday 02:00, because that
  window opened Friday at 22:00. This is what "weeknights" means to a person.
  Matching the instant instead would silently cut every one of those windows
  at midnight.

`launch-window: HH:MM-HH:MM` is still accepted as shorthand for a single
`allow daily <window>`. If both keys are present, `launch-schedule` wins and
you say so.

## Operations

Route on `$ARGUMENTS`:

### 1. show — bare invocation, "show rules", "what are the rules", "list rules"

Read `policy.md`. Print each rule **in plain English**, not as raw YAML, then
the current verdict:

```
newcorder — iterate launch rules

  lock      requires the keyword "permission" on every fresh launch
  schedule  allow  weeknights 22:00–06:00
            allow  weekends   20:00–08:00
            deny   2026-12-24 to 2026-12-26

  right now (Mon 10:49)  -> REFUSED: outside every allow window
                            next opens today at 22:00
```

No policy file: say the project has no launch rules and a run may start any
time. That is a statement, not a problem to fix.

### 2. add/change — anything describing a rule

Translate to the schema and write it. Read the real clock/date with
`date +'%F %H:%M %a'` before anything time-dependent — never estimate it.

- Create `policy.md` if absent, with a `## Why the launch gate exists` section
  in the user's own words. If they gave no reason, ask for one line — it is
  the sentence `/iterate` will quote back at them, and a refusal with no
  reason is a bad refusal.
- **Merge, never clobber.** An existing unrelated rule survives untouched.
- **Fold, don't stack.** A new rule covering the same ground as an existing one
  replaces it. Two overlapping allow windows for the same days is a policy
  nobody can read.
- Re-print the full ruleset (op 1) so the user sees the result, and state the
  current verdict. Writing a rule and not saying whether it takes effect now
  is how a rule gets misunderstood.

### 3. test — "test", "check", "would it run now", "test 23:00", "test sat 02:00"

Evaluate and explain, without writing anything. Name the deciding rule:

```
test Sat 02:00 -> PERMITTED by: allow mon-fri 22:00-06:00
                  (window opened Fri 22:00 — weeknight windows carry past midnight)
                  lock rule applies: launch as /iterate <plan> permission

test Sat 07:13 -> outside every allow window
                  but "permission" overrides the schedule, so
                  /iterate <plan> permission WILL launch
```

Always report both gates together, and always say whether the keyword would
carry it. Reporting "outside the window" without adding that the keyword
overrides is the answer that sends someone away believing they cannot run —
which is exactly the failure this rule exists to prevent.

### 4. remove / clear — "remove the time rule", "drop the keyword", "clear the rules"

Remove what was named and re-print. `clear` empties the rules but keeps the
file and its reason text, so the intent survives a temporary lift. Deleting
`policy.md` entirely is only on an explicit "delete the policy file".

## Rules

1. **Never enforce anything.** This skill reads and writes policy. `/iterate`
   enforces it. Do not refuse, warn about, or block a run from here.
2. **Never edit plans, notes, or branches.** The only file this touches is
   `./.claude/iterate/policy.md`.
3. **A key `/iterate` does not read is not a rule.** The schema above is the
   whole contract. Writing an invented key produces a file that looks like a
   rule and enforces nothing — if the user asks for a gate the schema cannot
   express, say so plainly and offer the closest thing it can.
4. **Read the real clock.** `date` every time. A stale in-session timestamp
   inverts a time gate as surely as a wrong comparison.
5. **State the verdict after every write.** "Rule saved" alone leaves the user
   guessing whether a run can start right now.
6. **Don't argue with the rule.** If they want runs only between 03:00 and
   03:05, write it. Warn once if a rule can never match (`allow mon 22:00-22:00`
   is an empty window), then do as asked.
