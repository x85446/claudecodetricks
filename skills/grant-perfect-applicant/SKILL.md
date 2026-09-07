---
name: grant-perfect-applicant
description: Use to describe the IDEAL company for a given government solicitation — the applicant the issuing office is actually hoping shows up. Reads the source solicitation, all Q&A, and every extracted fact and profiles the perfect bidder's capabilities, past performance, certifications, size/vehicle, and team. One paragraph. Company-agnostic — never mentions Izuma.
argument-hint: [opportunity-id | path to a record JSON | pasted solicitation+Q&A]
disable-model-invocation: false
---

## What this skill does

Every solicitation has an implied "perfect applicant" — the vendor whose profile the evaluation factors, Q&A answers, and mandatory requirements were (consciously or not) written around. This skill produces **one paragraph** describing that ideal company: what it can do, what it has already done, what it is certified/cleared to do, its size and contract vehicle, and the shape of its team. It is a **neutral, company-agnostic** profile — it never names or evaluates any specific bidder (that is `grant-izuma-fit`'s job).

Pairing this with `grant-really-wants` gives the two halves of the picture: what the buyer wants, and who they want it from.

## Inputs (use everything available)

Same complete record as `grant-really-wants`:
- Source description / synopsis body.
- **All Q&A** — reveals the disqualifiers and the unstated must-haves ("offerors must already hold…", "prior work on program Y is expected").
- Attachments / solicitation PDFs — evaluation factors, past-performance thresholds, mandatory standards, security requirements, CDRLs.
- Metadata — agency, set-aside, NAICS/PSC size standard, funding vehicle (SBIR/STTR/BAA/OTA/CSO/IDIQ), ceiling, period of performance.

## How to write the paragraph — the ideal bidder's profile

Cover the dimensions the award will actually turn on:

1. **Core technical capability** — the specific engineering the deliverable requires (not the topic buzzwords — the real work).
2. **Past performance** — the kind and scale of prior contracts/deployments the evaluators expect ("a vendor with a fielded system at ≥ program scale," "prior transition from SBIR Phase II," "existing ATO on a comparable enclave").
3. **Certifications / clearances / accreditations** — facility clearance level, cleared personnel, ATO/RMF, FedRAMP, CMMC level, NSA/NCDSMO accreditation, ITAR registration — whatever the record makes mandatory.
4. **Size & vehicle fit** — small business / 8(a) / SDVOSB set-aside status, the right NAICS size, SBIR/STTR eligibility (incl. the STTR research-institution partner), or the primes-welcome reality of a large BAA.
5. **Team shape** — solo prime vs. prime+sub, required research-institution partner (STTR), integration partners, domain SMEs, on-site/CONUS staffing.

State it as the profile of a *winning* bidder, tuned to THIS posting. If the solicitation is genuinely wide-open, describe the ideal profile as broad — but note what still separates a credible bidder from noise (relevant past performance, the right vehicle eligibility).

## Output contract

- **Exactly one paragraph** (roughly 90–160 words). No headers, no bullets, no preamble.
- Third-person, describing a hypothetical ideal company. **Never** name Izuma or any real firm; never say "you" or "we."
- Every requirement traceable to the record (esp. Q&A/evaluation factors). Under-claim rather than invent thresholds.

## Rules

- The "perfect applicant" is defined by the *buyer's* requirements, not by who happens to be reading. Stay company-agnostic.
- Mandatory requirements surfaced in Q&A ("must already hold an accredited guard," "offeror must be the OEM") define the ideal profile even when the base PWS omitted them — weight them.
- If STTR: the ideal applicant necessarily includes a partnered research institution performing the required minimum research percentage — always state this.
- Do not reproduce copyrighted solicitation text at length — synthesize.
