---
name: grant-really-wants
description: Use when you need to cut through a vague government solicitation and state what the issuing institution ACTUALLY wants. Reads the source solicitation plus every Q&A and extracted fact and re-writes the true, de-jargoned requirement — the intent the posting was too cautious, too boilerplate, or too broad to say plainly. One paragraph.
argument-hint: [opportunity-id | path to a record JSON | pasted solicitation+Q&A]
disable-model-invocation: false
---

## What this skill does

Government solicitations are written to survive protest, not to communicate. The public description is often deliberately broad ("innovative solutions for data management"), while the *real* requirement — the thing that will actually win or lose the award — surfaces in the **Q&A**, the **attachments**, the **evaluation criteria**, and the **incumbent context**. This skill produces **one tight paragraph** that says what the institution really wants, in plain language, grounded in the full record.

It is analysis, not advocacy. It describes the buyer's true need whether or not anyone reading it can meet it.

## Inputs (use everything available, never title alone)

Pull the complete record for the opportunity, not just its summary:
- **Source description / synopsis body** (`description_text`, `synopsis_text`, `topic_text` primary tier).
- **All Q&A** (`qa_text`) — this is the highest-signal source. Vendors ask the questions the solicitation left ambiguous, and the government's answers are where the truth leaks: required certifications that weren't in the PWS, an incumbent being protected, a "we will not consider X" that narrows the field, a hard integration constraint, a real (vs. stated) timeline.
- **Attachments / solicitation PDFs** (`official_solicitation_text`, `instructions_text`, attachment bodies) — evaluation factors, CDRLs, mandatory standards, place-of-performance and clearance requirements.
- **Metadata** — agency, sub-office, NAICS/PSC, set-aside, funding vehicle, deadline, incumbent references.

If the record genuinely has thin data (SAM one-line notices, NSF subtopics with no per-topic body), say so in the paragraph rather than inventing specifics.

## How to write the paragraph

1. **Lead with the real deliverable**, not the marketing title. "Despite the broad 'secure data services' framing, the Q&A makes clear the government is buying **X**."
2. **Name the truth the description hid.** Prefer facts pulled from Q&A/attachments over the synopsis. Quote or paraphrase the specific answer that reveals it.
3. **State the hard constraints** that actually gate the award: clearance level, accreditation (e.g. an NSA-accredited CDS, ATO, FedRAMP), mandatory standard or reference architecture, incumbent advantage, on-site/CONUS requirement, hardware vs. software expectation.
4. **Distinguish "stated scope" from "operative scope."** If the PWS says one thing and the Q&A narrows or contradicts it, the Q&A wins — flag the divergence.
5. **Be honest about vagueness.** If even the full record is genuinely open-ended (a true "pitch us anything" CSO/BAA), say that plainly — that IS the finding.

## Output contract

- **Exactly one paragraph** (roughly 90–160 words). No headers, no bullet list, no preamble.
- Third-person, declarative, specific. Every non-obvious claim traceable to the source/Q&A.
- Never recommend, never assess anyone's fit — that is a different skill. This paragraph is purely: *what does the buyer really want?*

## Rules

- Q&A and attachments outrank the synopsis when they conflict. The synopsis is the ad; the Q&A is the confession.
- Do not fabricate certifications, incumbents, or constraints not supported by the record. Under-claim rather than invent.
- Do not reproduce copyrighted solicitation text at length — synthesize and paraphrase.
- If the record has no Q&A and only a stub description, produce a paragraph that says exactly that and states the most that can be honestly inferred.
