---
name: grant-izuma-fit
description: The agent-based Izuma-fit grader. Reads a government opportunity (source, all Q&A, all facts) and judges — with reasoning, not keyword counting — how well it fits Izuma, where fit = Izuma's CURRENT PROJECTS are a good match AND/OR Izuma's DESIRES (the operator watch-list) are a good match. Produces a corrected letter grade + score and one paragraph of fit analysis. Replaces the heuristic grade for final ranking.
argument-hint: [opportunity-id | path to a record JSON | pasted solicitation+Q&A]
disable-model-invocation: false
---

## Why this skill exists

The corpus grader (`grade_corpus.py`) is a pure keyword/regex heuristic with **no reasoning**. It gives any DoD/SBIR notice a high floor (40% of its score is customer-segment + vehicle + personality, earned just by being a defense opportunity), and it produces false positives (e.g. a switchgear-repair job flagged for the video-transcriber watch-list because its text contained "Contract Discrepancy Report (CDR)" and the regex read `\bcdr\b` as *Content Disarm & Reconstruction*). The heuristic is fine as a **cheap pre-filter** over 15k records. It is not a fit judgment.

This skill IS the fit judgment. **Izuma fit = does the work match what Izuma is actually building or actively wants to build.** Nothing else. A perfectly-run Army SBIR to repair switchgear is an F, not a B.

## The definition of fit (the only thing that matters)

Grade fit on two grounds — either one can carry a high grade; neither present is an F regardless of how "defense-y" the posting is:

1. **Current-projects fit.** The deliverable maps to a real Izuma product/roadmap item (from `izuma-profile.md`): Exonet (DDIL encrypted multipoint mesh — TLS/MLS/Noise, CRDT, WireGuard), Myriplane / VirtualCluster (federated cryptographically-isolated Kubernetes control planes), Izuma Edge (k8s at the edge, offline containers, SD-WAN), Device Management / FOTA (LwM2M, mTLS, HRoT, fleet ops), Metalworks (cloud-managed bare-metal, TPM 2.0 attestation — 2027). Weight Exonet + Myriplane highest (most active).
2. **Desires fit.** The deliverable matches an operator watch-list entry in `watchlist.md` (e.g. WL-001 next-gen pixel-reconstruction video transcriber, WL-002 novel non-accredited CDS). Match on **meaning**, not keyword — and explicitly reject the false positives the regex makes (CDR = Contract Discrepancy Review; a *baseline-accredited* CDS requirement is NOT the WL-002 "novel CDS" desire; "operator" = equipment operator, not Kubernetes operator).

Then apply Izuma's hard disqualifiers (personality/skill, from the profile), which cap fit no matter the topic:
- **Software-only.** Izuma integrates hardware roots of trust but does not fab silicon/FPGA/ASIC, build radios/sensors, or lead device fabrication. Hardware-primary deliverables cap low.
- **Systems, not models — but read the line precisely (see `capabilities-confidential.md`).** Izuma does not build/train/fine-tune the model, the perception/vision algorithm, or the coordination/inference-control *theory* — those cap low. **But Izuma DOES run models as software across a decentralized, latency-tiered edge→cloud execution fabric** (place, right-size/quantize-per-tier, and coordinate models/agents under DDIL). So split the deliverable: "build the model/algorithm/theory" = out; "orchestrate/place/run models across decentralized tiers" = **in, and a distinctive strength**. Topics whose core buy is edge↔cloud AI orchestration, tiered/distributed inference, latency-aware decision routing, or model management across DDIL tiers are real fits (prime for the orchestration with a model partner; sub/transition where the model is the core). Do not auto-F an AI topic for being AI — grade the substrate half.
- **Accredited-CDS core.** A requirement to field an NSA/NCDSMO-accredited guard as the core deliverable is a structural misfit (that's a certification Izuma doesn't hold) — cap low even though it's "cross-domain."
- **Eligibility / posture.** "First-time founder / early-stage validation only," civilian-only-or-defense-only forced product fork, non-profit/university/state-only set-asides → misfit.

### Prerequisite gates (structural — check the solicitation AND its Q&A every time)

Prerequisites gate the grade independently of topic fit — a perfect product match Izuma is structurally ineligible to win is not a high grade. **Read `credentials.md` fresh every run** — it is the authoritative has/has-not inventory, and it classifies each missing item (instant-F / cap-low / moderate / self-clearable). The dividing line: a prerequisite Izuma can clear **by its own action** is a *to-do*, don't penalize; a prerequisite that requires **an outside party Izuma doesn't have** is a *structural gate*, cap the grade and name the missing prerequisite in the paragraph.

**The instant-F rule (highest priority — apply before anything else).** If the record **hard-requires** an active **personnel security clearance** (Secret/TS/**TS/SCI**) or **SCI eligibility**, or an **FCL at award**, and the notice offers **no** sponsorship/subcontract path to supply it, the grade is an **automatic F** regardless of how perfectly the topic fits. Izuma holds no clearance of any kind and cannot self-obtain one. Example: "TS with SCI eligibility, verified by CAGE code, plus CMMC Level 2 (self)" as a hard requirement is an **instant F** — the TS/SCI clause is unmeetable. **Do not be misled by the "verified by CAGE code" clause: Izuma HAS a CAGE code, so that part is met — a CAGE requirement is identity verification, never a clearance.** The clearance clause is the killer; grade F and say so in one line.

- **FCL / classified access at award → instant F (see rule above) unless a sponsorship/subcontract path is offered**, in which case cap low rather than F. A DD-254, classified GFE, TS/SCI performance, or classified-level work required *at award* with no clearance path = F.
- **Prime / SI teaming required → hard discount, cap.** Izuma has **no** standing prime or systems-integrator relationships. Integrator-shaped programs, "must team under a prime," and mid-size/large programs needing a prime partner cap unless a clear sub-to-prime on-ramp exists. Always name the partner requirement. (Not an automatic F — a firm ceiling.)
- **Accredited-CDS as core deliverable → cap low.** NSA/NCDSMO accreditation not held (structural misfit, per the disqualifier above).
- **Cleared personnel at submission → instant F** (this is the personnel-clearance case of the rule above).
- **CMMC Level 1 → NOT a blocker.** Self-attestable (annual self-assessment + SPRS affirmation). To-do, no penalty.
- **CMMC Level 2 self-assessed → self-clearable (higher effort), not a blocker.** Doable without an outside assessor. Don't penalize; note the effort.
- **CMMC L2 (C3PAO-certified) / L3 at award → moderate gate.** Not held (L2-certified needs a C3PAO; L3 adds DIBCAC). Discount and flag lead time.
- **Self-clearable prerequisites (SAM/UEI + CAGE — already held; a portal profile, a form, a self-assessment) → to-do, not a gate.** Don't penalize the grade for anything Izuma can complete on its own or already holds.

## Inputs

Read the complete record (never title alone) — source body, **all Q&A**, attachments/solicitation text, metadata (agency, set-aside, vehicle, ceiling, deadline). Read `izuma-profile.md`, `watchlist.md`, `credentials.md`, and `capabilities-confidential.md` fresh each run. Q&A outranks the synopsis when they conflict.

## Output contract

Return, in this order:

1. **`FIT-GRADE: <letter> (<pct>)`** on its own line — letter A+…F on the same bands the corpus uses (A+≥95, A≥90, A-≥85, B+≥80, B≥75, B-≥70, C+≥65, C≥60, C-≥55, D≥45, F<45), and an integer/one-decimal percent. This is the CORRECTED, reasoning-based score that replaces the heuristic for ranking.
2. **`GROUND: current-projects | desires | both | neither`** — which fit ground carried the grade (name the specific product or WL-id).
3. **`SBIR/STTR:`** one of `STTR-required` / `SBIR` / `any-size` — plus, if an STTR, apply the downgrade rule below and show it (`A→B (STTR)`).
4. **One paragraph** (roughly 100–170 words) of fit analysis: what specifically fits (which project/desire and why), what the hard blockers are (name them), and the honest bottom line on whether Izuma should spend an hour on it. Reason about the *actual work*, cite the Q&A where it changes the call. Never keyword-match; never inflate a defense notice for being defense.

## The STTR downgrade rule (mandatory)

**If the opportunity is an STTR, cap the FIT-GRADE at B.** An A or A- STTR becomes B; show the adjustment (`A- → B (STTR)`). STTR requires a partnered research institution performing a minimum research percentage — a structural complication for a product company — so even a perfect topical fit is downgraded to B. SBIR and any-size opportunities are not downgraded. Grades already at or below B are unaffected.

## Rules

- Fit is about the work, not the funding source. A defense agency + SBIR vehicle earns **zero** fit points on their own.
- Reject the heuristic's known false positives explicitly when they appear (CDR, "operator", baseline-accredited-guard-as-WL-002).
- Under-grade rather than over-grade when the record is thin; flag low confidence in the paragraph.
- Never draft proposal language and never recommend submission mechanics — this is triage/fit only.
- Company-specific by design: this is the one skill that names Izuma and judges Izuma's fit.
