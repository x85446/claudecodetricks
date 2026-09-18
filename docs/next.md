# Next

Parked context, captured verbatim from an earlier session so it survives a fresh start. The paste below arrived partly truncated — tables lost cell content in transit. Treat it as a pointer to the real state in `./.claude/iterate/plans/ichneumon.md`, not as a source of truth.

## Open item: statusline case encodes teaming

Requested: in the statusline's `⚙️` segment, show **flat plans as lowercase** and **already-teamified plans as uppercase**. Colour keeps carrying plan state (green executing, yellow planned, red blocked, cyan unblocked, magenta paused, dim closed); case adds teaming at no extra width.

## Answered: rule 30 and flat plans

The planner's note — "the one 5.5.1 feature a flat plan can't use yet is rule 30's per-team Model column" — means exactly what it says, and it is not a bug. The Model column lives in the Teams table, so a plan with no teams has no row to carry a model. A flat plan runs entirely in the session that launched it, at that session's model. Accepted as-is; no change wanted.

## Parked paste — plan `ichneumon`, replanned to family 5.5.1

Format/schema update only. The 30 steps and their content are unchanged.

What the 5.5.1 replan changed in the file:

| Change | Detail |
|---|---|
| Checkbox state (rule 29) | Steps and Validation `a` is the state machine `/iterate`, `/iterate-triage`, the `c` read to know where the plan is. Verified: 30 Steps ✓, 30 Valida*[truncated]*s. |
| Provenance left plain | Still N. (it's a 1:1 a*[truncated]*list) — 30 lines, correct. |
| planner-version | 5.3.0 → 5.5.1 |
| branch-created: false | added under `branch:` (p*[truncated]* |

Access preflight re-run: no new dependency (l*[truncated]*step 1; tax/merchant/Chase steps unchanged).
Oracle re-scan: no new matches. Nothing paid s*[truncated]* flat — rule 16 forbids a refinement from silently teamifying.

The one 5.5.1 feature a flat plan can't use *[truncated]* rule 30's per-team Model column (opus for building, sonnet for reading/reporting, haiku *[truncated]* eps across six independent subsystems, ichneumon is the textbook case for it. Say "t*[truncated]* cluster it with model tiers — roughly:

| Team | Steps | Model | Why |
|---|---|---|---|
| grouped-ui | 2–9, 12–15 | opus | Swift/Ap*[truncated]*ndow buttons, Like Group, E*[truncated]* editor, and the sele*[truncated]*gine/TransactionTableController, so same context |
| merchant-repair | 10–11 | opus | data ana*[truncated]*script |
| chase-images | 16–20 | opus | Python stor*[truncated]*-gated pull |
| tax-summary | 21–27 | opus | CLI + Ru*[truncated]*ge + its FFIV |
| finishers | 28–30 | — | coordina*[truncated]* |

(The real teamify pass decides the exact split — the selection-FFIV/grouped-UI merge above is the kind of shared-context call it makes.)

Offered next: changes, `teamify ichneumon`, or `/iterate`.
