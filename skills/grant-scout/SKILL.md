---
name: grant-scout
description: Use when someone asks to score a government grant, evaluate grant fit for Izuma, check if a grant aligns with our roadmap, grade a federal funding opportunity, scout SBIR/STTR opportunities, triage Grants.gov / SAM.gov / DARPA / DSIP listings, or refresh the bulk grant corpus.
argument-hint: [--corpus|--refresh-corpus|file|directory|URL|opportunity-id|keyword|empty to scan grants/inbox/]
disable-model-invocation: true
---

## What This Skill Does

Reads government grant solicitations and grades each one against Izuma Networks' roadmap, customer segments, skillset, and personality. Produces a letter grade (A+ … F) and a percentage fit score so a human can decide whether to pursue.

**This is a triage tool, not a proposal writer.** It tells you whether a grant is worth your hour to read in full. It never auto-submits, never drafts proposal language, and never claims certainty when the source data is thin — it flags low-confidence calls explicitly.

## Pre-Flight Checks

1. **Profile loaded?** Verify `izuma-profile.md` is in the same directory as this SKILL.md. If missing, stop and tell the user the skill is broken — the rubric cannot run without the company profile.
2. **Grant inbox.** If invoked with no argument, verify `grants/inbox/` exists at the project root. If absent, create it (`mkdir -p grants/inbox`) and tell the user where to drop grants.
3. **Output directory.** Ensure `output/grants/` exists at the project root (`mkdir -p output/grants`). No hard stop — auto-create.
4. **Optional API key.** If the argument looks like a SAM.gov opportunity ID, check for `$SAM_API_KEY` in env. If missing, fall back to WebFetch on the public opportunity URL and warn the user that scoring may be less complete. **Never read `~/.ssh/sam.gov.key` directly** — the key is sourced into the shell via `profile.d/10-source-and-export.sh` and must reach the skill only through `$SAM_API_KEY`.

## Step 1: Detect Input Mode

Parse the argument and route to the right handler. The skill auto-detects shape:

| Argument shape | Mode | Handler |
|---|---|---|
| empty | batch-inbox | List every file in `grants/inbox/`; score each |
| `--corpus` | corpus-score | Score every record in `output/grants/_corpus/_all.json` |
| `--refresh-corpus` | refresh | Run `lookup/fetch_all.sh` to re-crawl all sources, then score the new corpus |
| existing file path | single-file | Read file, score one grant |
| existing directory path | batch-dir | List files in directory; score each |
| starts with `http://` / `https://` | url | WebFetch URL; score one grant |
| matches `^[A-Z0-9-]{8,}$` and looks like an opportunity ID | opportunity-id | First check `_corpus/_all.json` (across all 4 sources); if not present, fall back to `sam_lookup.sh by-solnum`; if still missing, WebFetch DARPA `/research/opportunities/<id>` and `sam.gov/workspace/contract/opp/<noticeId>/view` |
| anything else (free text) | keyword-search | Grants.gov Search2 API; score top 10 results |

**Detection examples:**
- `(no arg)` → scan `grants/inbox/`
- `grants/inbox/CBD-25-001.pdf` → single file
- `~/Downloads/grants/` → batch directory
- `https://www.grants.gov/search-results-detail/345678` → URL
- `W912CG-26-S-0001` → SAM.gov ID
- `tactical edge networking DDIL` → Grants.gov search

If ambiguous, ask the user with `AskUserQuestion`. Don't guess between "search keyword" and "file path that doesn't exist" — confirm.

## Step 2: Load the Izuma Profile

Read `izuma-profile.md` from this skill's directory. It contains:
- Product portfolio (Device Management, Edge, Myriplane, Exonet, Metalworks, VirtualCluster)
- Target customer segments and their priorities
- Roadmap weights (18-month horizon)
- Technical skillset (what we can actually deliver)
- Personality traits (open-source bias, software-only, dual-use, etc.)
- Anti-fit signals (not a hard blacklist — read and judge)

**Also read** the current product one-pagers under `products/*/one-pager.md` to keep grounded in current capability claims. If the profile and a one-pager disagree, the one-pager is authoritative (it gets updated more often).

## Step 2.5: Eligibility Pre-Filter (short-circuit to F)

**Run before the rubric.** Many federal grants restrict eligibility to specific applicant classes that exclude a for-profit small business like Izuma. When the applicant-eligibility field clearly excludes us, short-circuit straight to F with a single-line reason — don't run the full rubric.

Look at the synopsis fields `applicantEligibilityDesc` and the agency program type. Auto-F when eligibility is restricted to **any** of these and does not explicitly admit for-profit small businesses:

- States, U.S. territories, DC, federally recognized tribal governments, or local governments only
- Law enforcement agencies (state, local, federal)
- Fire departments, EMS, fire training academies
- K-12 schools, school districts, or state education agencies
- Public housing authorities
- Hospitals or healthcare providers (when as service recipient, not as research grantee)
- Public institutions of higher education only (no private; no companies)
- Non-profit organizations only

Auto-F is **not** triggered when:
- Eligibility says "SBIR/STTR small business" (we're eligible)
- Eligibility says "all eligible entities including for-profit" or lists "small businesses" explicitly
- Eligibility lists "Private institutions of higher education" alongside fire departments — we still cannot apply, but flag for academic-partner pursuit
- Eligibility is `unknown` or empty — skip the pre-filter and run the full rubric

When short-circuiting, write a slim evaluation file (eligibility section only — no rubric table) so the user understands the F is structural, not topic-based. Note in the per-grant file: `**Killed by eligibility, not topic.**`

## Step 2.6: Bulk Corpus (corpus-score / refresh modes)

The skill maintains a local corpus of currently-open federal funding opportunities under `output/grants/_corpus/`. The corpus is populated by `lookup/fetch_all.sh`, which chains four crawlers:

| Source | Crawler | Method | Auth |
|---|---|---|---|
| **SAM.gov v2** | `lookup/crawl_sam.sh` | paginate `opportunities/v2/search`, 11-month posted window | `$SAM_API_KEY` (env, never argv) |
| **Grants.gov** | `lookup/crawl_grants_gov.sh` | Search2 (`oppStatuses=posted\|forecasted`) + per-opp `fetchOpportunity` | none |
| **DARPA** | `lookup/crawl_darpa.sh` | parse the official `darpa.mil/rss/opportunities.xml` feed (recent active opportunities) | none |
| **SBIR.gov** | `lookup/crawl_sbir.sh` | paginated GET to `api.www.sbir.gov/public/api/solicitations?open=1` — covers DoD + civilian SBIR/STTR topics. (Replaces DSIP — DSIP's topic search requires CAC/login.gov; SBIR.gov mirrors the same DoD SBIR topics publicly plus all other agency SBIR/STTR.) | none |

Each crawler writes raw responses under `_corpus/<source>/raw/`. `lookup/normalize.py` then maps every source into a unified record schema (see file header). `lookup/merge.py` produces `_corpus/_all.json` with cross-source dedupe by solicitation number (priority: dsip > darpa.mil > sam.gov > grants.gov).

**`--refresh-corpus` flow:**
```bash
.claude/skills/grant-scout/lookup/fetch_all.sh
```
Runs all crawlers, normalizes, merges. Logs progress per source. Writes `_corpus/_metadata.json` with last-crawl timestamps and counts. ~10–30 minutes depending on Grants.gov + DARPA detail-page latency.

**`--corpus` flow:**
1. Read `_corpus/_all.json`. If absent or older than 7 days, prompt user to `--refresh-corpus` first.
2. For each record, run the same rubric used in Step 4. The unified schema has all the fields Step 3 extraction needs already populated — no per-grant API call.
3. Apply the eligibility pre-filter (Step 2.5) before scoring.
4. Write per-grant `evaluation.md` only for grades C+ and higher; collect F/D into a batch summary table.
5. Write `output/grants/_MASTER_<YYYY-MM-DD>.md` with the ranked top list across all sources.

**Cron refresh.** The user has set this up to run weekly via `/schedule` (Monday morning). When the corpus is fresh, `--corpus` is a pure local operation with no API calls or rate limits.

**Re-runs are idempotent.** Crawlers diff against existing files where possible (Grants.gov fetchOpportunity, DARPA detail pages) and re-download what's stale. SAM gets rewritten each run because per-page records change order. Removing the entire `_corpus/<source>/` dir forces a clean re-crawl.

**Gitignore.** Add `output/grants/_corpus/` to `.gitignore`. The corpus is rebuildable and large.

## Step 2.7: SAM.gov Adapter (opportunity-id mode, single lookup)

When the input is a SAM-style solicitation/notice ID, route through the bundled `sam_lookup.sh` helper rather than building the curl call inline. The helper enforces the credential-handling rules below; using it makes the rules unbreakable by accident.

**Invocation:**

```bash
# Per-grant lookup (writes raw.json + normalized.json into the output dir)
.claude/skills/grant-scout/sam_lookup.sh by-solnum <SOLICITATION-NUMBER> output/grants/<id>/

# Same shape but for SAM noticeId (the UUID surfaced in sam.gov UI links)
.claude/skills/grant-scout/sam_lookup.sh by-noticeid <NOTICE-ID> output/grants/<id>/

# One-shot key sanity check
.claude/skills/grant-scout/sam_lookup.sh validate
```

Exit codes: `0` = found, `2` = $SAM_API_KEY unset, `3` = HTTP non-200, `4` = zero results (audit-trail raw.json + `{"found": false, "totalRecords": 0}` normalized.json are still written so the evaluation can record "not found" with provenance).

**Normalized shape (`normalized.json` for `found: true`):**

| Field | Source SAM key | Maps to grant-scout Step 3 field |
|---|---|---|
| `id` | `solicitationNumber` (fallback `noticeId`) | Identifier |
| `title` | `title` | Title |
| `agency` | first segment of `fullParentPathName` | Agency |
| `subagency` | full `fullParentPathName` | Agency (sub) |
| `type` / `baseType` | `type`, `baseType` | Phase structure (Sources Sought, Solicitation, Award, etc.) |
| `postedDate` | `postedDate` | (informational) |
| `responseDeadline` | `responseDeadLine` (fallback `archiveDate`) | Deadline |
| `naicsCode` / `classificationCode` | `naicsCode`, `classificationCode` | Topic / scope hints |
| `setAside` | `typeOfSetAsideDescription` | Eligibility (SBIR/STTR/Small Business etc.) |
| `awardCeiling` | `award.amount` (if present) | Funding amount |
| `uiLink` | `uiLink` | Source URL recorded in evaluation |
| `description_short` | first 600 chars of `description` (often a URL to fetch full text) | Topic / scope seed |

If `description_short` looks like a URL (SAM frequently stashes the full text behind `https://api.sam.gov/prod/opportunities/v1/noticedesc?noticeid=...`), fetch that URL with WebFetch to fill the full topic before scoring.

**Credential rules (hard, non-negotiable):**

1. Read the key **only** from the `$SAM_API_KEY` environment variable. Never read `~/.ssh/sam.gov.key` directly with file tools, never accept the key on the command line, never write the key into any file inside the project.
2. The key reaches `curl` through `--config -` with an **unquoted** heredoc so the shell substitutes `$SAM_API_KEY` into the config stream only. The string `api_key=` must never appear in argv, in any Bash command echoed to the user, in any saved curl config under the repo, or in any committed artifact.
3. When recording the queried URL in `raw.json` or evaluation text, replace the key with the literal token `REDACTED`. `sam_lookup.sh` already does this; if you build a query inline (e.g., during debugging), do the same.
4. If `$SAM_API_KEY` is missing, fall back to WebFetch on the public sam.gov page (`https://sam.gov/workspace/contract/opp/<noticeId>/view`) and flag the evaluation `confidence: low — no SAM API` so the user knows the call wasn't authenticated.
5. SAM v2 imposes per-key rate limits and rejects date ranges ≥ 1 year. `sam_lookup.sh` already paginates two adjacent 6-month windows; do not write inline queries that span > 12 months.

**Detection heuristic for sam-id mode** (refines Step 1):

A token is treated as a SAM-style solicitation ID when it matches `^[A-Z0-9][A-Z0-9-]{6,}$` AND contains at least one digit AND is not a recognized Grants.gov shape (`HR001nnnXNNNN`, `Wnnnnn-nn-S-nnnn` award-IDs, etc. still go via SAM since SAM indexes both contracts and award notices). When unsure, run `sam_lookup.sh by-solnum <id>` first; if it returns `found:false`, then try Grants.gov.

## Step 3: Extract Grant Details

For each grant being scored, extract:

| Field | Notes |
|---|---|
| Identifier | Solicitation number, opportunity ID, or filename |
| Title | The funding opportunity title |
| Agency | DoD branch / DARPA / NSF / DOE / DHS / NIH / etc. |
| Topic / scope | The technical area being funded |
| Funding amount | Phase I budget cap, Phase II cap, total ceiling |
| Duration | Period of performance |
| Deadline | Submission deadline (calendar date) |
| Eligibility | SBIR/STTR small business required? Prime/sub allowed? Foreign-owned restrictions? |
| Phase structure | I → II → III; direct-to-Phase-II; OTAs; Broad Agency Announcement |
| Keywords | Verbatim technical keywords from the solicitation |

If a field can't be determined from the source, write `unknown` — don't guess. Unknown fields lower the confidence score on the final report.

## Step 4: Apply the Rubric

Score each grant on **five weighted axes**. Each axis is 0–100. The weighted sum produces the overall percentage.

### Axis 1 — Roadmap Alignment (35%)

Does the grant fund work on something we're already building? Map the grant's technical scope to our product roadmap:

| Map to | Examples |
|---|---|
| **DDIL networking / secure mesh** | Exonet — triple-layer encryption, CRDT data sync, disconnected ops, OSPF link-state mesh, WireGuard overlay |
| **Multi-tenant federated Kubernetes** | Myriplane / VirtualCluster — cryptographic tenant isolation, federated control planes, edge k8s, gossip-based sync |
| **Bare-metal & hardware trust** | Metalworks (2027 roadmap) — UEFI Secure Boot, TPM 2.0 attestation, signed OS, remote provisioning |
| **IoT device management & FOTA** | Device Management — LwM2M, delta firmware updates, device directory, Factory Flow PKI |
| **Cross-cutting platform** | Identity, PKI, observability, SD-WAN, Apache 2.0 edge software |

Scoring:
- 90–100: Grant topic IS one of our product areas (e.g., "tactical edge mesh networking with DDIL" → Exonet)
- 70–89: Grant adjacent to a product (e.g., "secure container orchestration at the tactical edge" → Myriplane-adjacent)
- 40–69: Touches a capability we have but isn't our core (e.g., "edge analytics" — we provide the substrate, not the analytics)
- 20–39: Tangential — we'd have to invent something new
- 0–19: Off-roadmap entirely

### Axis 2 — Customer-Segment Alignment (20%)

Does the funding agency / end-user match our target verticals?

| Segment | Examples | Default weight |
|---|---|---|
| DoD / defense | Army, Navy, Air Force, SOCOM, DIU, DARPA, Marine Corps, SBIR DoD topics | 100 |
| Intelligence community | ODNI, NSA, NGA, CIA In-Q-Tel | 95 |
| Federal civilian (critical infra) | DHS / CISA, DOE (grid), NIH (medical IoT), NSF (CISE), NASA (edge compute) | 80 |
| Industrial / OT / smart-grid | DOE manufacturing, ARPA-E, smart-grid co-funded programs | 70 |
| Other / academic-only | Pure-research NSF, education grants, K-12 STEM | 20 |

Adjust within band based on how directly the agency mission lines up. A DARPA topic on "resilient tactical comms" sits at 100; a DoD topic on "AI-assisted logistics scheduling" sits closer to 60.

### Axis 3 — Skillset / Capability Fit (25%)

Can Izuma actually build what the grant asks for?

Strong yes:
- Distributed systems, Kubernetes, edge orchestration
- Cryptographic protocols (TLS, MLS, Noise, WireGuard), PKI, hardware root of trust integration (TPM 2.0, TrustZone, PARSEC)
- CRDT-based data sync, gossip protocols, disconnected/partition-tolerant systems
- IoT device management, LwM2M, OMA standards, FOTA pipelines
- Multi-tenancy, RBAC, OPA, OIDC, audit trails
- Open-source operator development (controllers, CRDs, webhooks)

Weak / outside wheelhouse:
- Silicon design, chip fab, ASIC/FPGA RTL — we **integrate** hardware roots of trust, we don't design them
- Large-model AI training, model R&D, GPU cluster orchestration as the product
- Pure ML application work (we host the substrate; we don't build the model)
- Wet-lab / biotech / materials science
- Mechanical engineering / robotics control loops

Scoring:
- 90–100: Direct overlap with proven capability (we ship this today or are months away)
- 70–89: Strong overlap with capabilities we have, modest engineering reach
- 40–69: We could deliver with focused work but it's a stretch
- 20–39: Outside core; would need to acquire skills or major partnership
- 0–19: We physically cannot deliver (silicon fab, biology, etc.)

### Axis 4 — Execution Feasibility (15%)

Can we realistically run this program at our current scale?

- **SBIR/STTR Phase I** ($150K–$250K, 6–9 months): score 90–100. Comfortable solo.
- **SBIR/STTR Phase II** ($1M–$2M, 18–24 months): score 80–95. Comfortable solo or with one partner.
- **Direct-to-Phase-II** ($1.5M–$3M): score 75–90. Need solid Phase I-equivalent evidence.
- **Mid-size cooperative R&D** ($2M–$10M, 24–36mo): score 50–80, conditional. **Only if** (a) it's an exact product fit AND (b) we can plausibly find a prime/integrator.
- **Large multi-year prime program** ($10M+, 3–5yr, must lead as prime): score 20–50. Stretch — flag explicitly that scaling team is required.
- **Cost-share / matching-funds required at high %**: deduct 10–20 unless funding amount is meaningful.

### Axis 5 — Personality Fit (5%)

Bonus or penalty based on cultural alignment with how Izuma builds:

- **+10**: Grant is dual-use (civilian + defense), open-standards-friendly, allows open-source deliverables, software-only deliverable.
- **+5**: Grant treats us as the right size (small business set-aside, prefers proven small companies).
- **0**: Neutral.
- **−10**: Grant demands proprietary lock-in deliverables, foreign-ownership restrictions we don't satisfy, or forces civilian/defense fork in the product.
- **−20**: Grant requires Izuma to lead silicon, fab, biology, or model-training work as the **primary** deliverable.

This axis is small (5%) on purpose — personality nudges the grade up or down a half-step, it doesn't dominate.

### Combining the Axes

```
overall_pct = 0.35*A1 + 0.20*A2 + 0.25*A3 + 0.15*A4 + 0.05*A5
```

Apply the grade band:

| Overall % | Grade | Meaning |
|---|---|---|
| 95–100 | A+ | Drop everything, write this proposal |
| 90–94 | A | Excellent fit |
| 85–89 | A− | Strong fit |
| 80–84 | B+ | Good fit (the example the user gave at 70% lands here) |
| 75–79 | B | Solid match — pursue |
| 70–74 | B− | Good fit with caveats; likely needs a partner |
| 65–69 | C+ | Borderline; pursue only if low-effort |
| 60–64 | C | Marginal |
| 55–59 | C− | Skip unless desperate |
| 45–54 | D | Off-roadmap or wrong scope |
| 0–44 | F | Hard no — we cannot or should not deliver this |

Also produce a per-axis grade so the user can see WHY (e.g., "Roadmap: A (92), Customers: A (95), Skillset: B (78), Execution: B+ (82), Personality: +5 bonus → overall B+ 84%").

**Note on the user's example**: a grant scored "B+ 70%" was their intuition. With these weights, 70% lands in B−. If the user wants B+ to mean ~70%, they'll tell us and we'll re-band — for now, this rubric uses the conventional grade-school bands.

## Step 5: Write the Output

For each grant, write `output/grants/<grant-id>/evaluation.md`. The `<grant-id>` is derived in priority order: solicitation/opportunity number → safe-slug of the title → input filename stem.

Use this exact template (substitute fields):

```markdown
# Grant Evaluation: <Title>

**Grant ID:** <ID>
**Agency:** <Agency>
**Topic / Scope:** <one-line topic>
**Deadline:** <YYYY-MM-DD or "rolling" or "unknown">
**Funding:** <$ amount and duration>
**Phase:** <SBIR Phase I / II / direct-to-II / BAA / OTA / etc.>

---

## Grade: **<Letter> (<overall_pct>%)**

| Axis | Weight | Score | Grade |
|---|---|---|---|
| Roadmap alignment | 35% | <0-100> | <letter> |
| Customer segment | 20% | <0-100> | <letter> |
| Skillset / capability | 25% | <0-100> | <letter> |
| Execution feasibility | 15% | <0-100> | <letter> |
| Personality fit | 5% | <-20 to +10> | <bonus/penalty> |

**Confidence:** <high | medium | low> — based on how many input fields were `unknown`.

---

## Why this grade

**Roadmap map:** <which product(s) this aligns to and how — one paragraph>

**Strengths (what fits):**
- <bullet>
- <bullet>

**Concerns (what doesn't fit, what's missing, what would block):**
- <bullet>
- <bullet>

**Partner requirement:** <none | recommended | required as prime>

---

## Recommended next action

<one short paragraph: "Pursue solo", "Pursue with X-type partner", "Pass — reason", "Need more info — fetch [URL] or read full solicitation">

---

## Source

- Input: <file path / URL / opportunity ID / search query>
- Extracted on: <YYYY-MM-DD>
- Skill version: grant-scout v1
```

## Step 6: Summarize to the User

**Single-grant mode**: print the grade card (top 4 lines + grade + one-line "why" + recommended next action). Mention the file path of the written evaluation. Don't paste the full markdown into chat.

**Batch mode**: print a ranked table sorted by score, then list low-confidence and "needs more info" items separately:

```
## Batch evaluation: <N> grants from <source>

| Grant ID | Title (truncated) | Grade | Score | Deadline | Action |
|---|---|---|---|---|---|
| W912CG-26-S-0001 | Resilient Tactical Mesh ... | A | 92% | 2026-07-15 | Pursue solo |
| HR001126S0007 | Federated Edge Compute ... | B+ | 83% | 2026-08-01 | Pursue with prime |
| ... |

**Needs more info** (low confidence): <list of grant IDs and what's missing>
**Skipped** (parse failure / not a grant): <list>
```

Then drop a one-line callout for any A+/A grades that have a deadline within 30 days — these need immediate attention.

## Notes

- **Don't pretend to know what you can't read.** If a grant PDF is image-scanned and OCR isn't available, mark the evaluation `confidence: low` and recommend the user paste the topic abstract.
- **Don't apply for grants.** Triage only. If the user asks for proposal text, redirect them to `marketing-doc-formatter` for branded prose or to a future `grant-proposal-drafter` skill (does not exist yet).
- **Re-read product one-pagers each run.** They change. Don't hard-code product claims into this skill — the skill must always reflect current state.
- **No keyword blacklist.** Per Izuma's stated preference, the skill judges each grant on the rubric. Even a topic that sounds off-fit (e.g., "ML at the edge") may score well if Exonet/Myriplane is the substrate the work would run on.
- **Currency.** All funding amounts assume USD unless the grant explicitly says otherwise. If a grant is non-USD (e.g., EU Horizon Europe), score it but flag the currency mismatch.
- **Date discipline.** When extracting deadlines, convert to ISO YYYY-MM-DD. If the source uses a relative date ("90 days after publication"), record `unknown — relative` and recommend the user resolve manually.
- **Profile drift.** When Izuma's positioning changes (e.g., a new product ships, a customer segment is dropped), update `izuma-profile.md`. The skill reads it fresh each run — no rebuild needed.
- **Rubric tuning.** If grades feel systematically off, adjust the axis weights at the top of Step 4. Keep the weights summing to 100% (Personality stays a bonus/penalty outside the sum).
