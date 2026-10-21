# Izuma grant-scout — Operator Watch-List

Append-only. Operator-maintained. grant-scout **reads this every run** (referenced from `izuma-profile.md`) and applies it during scoring.

**How the algorithm uses this file:**
- Any corpus record matching a watch-list entry's signals gets a **roadmap-alignment boost** and is **surfaced explicitly in `_MASTER_<date>.md`** with a `WATCH: <entry>` flag — even if its baseline rubric score would otherwise leave it mid-pack.
- A watch-list flag never *forces* a passing grade; it guarantees the opportunity is shown to the operator with the watch reason, so a human makes the call.
- Entries are additive to the standard profile/rubric, not a replacement for it.

---

## WL-001 — Next-generation video transcriber (pixel-reconstruction CDR for video)

**What we're looking for:** Solicitations or programs that want a *novel* content-disarm-and-reconstruction engine for **video and streaming media** — one that sanitizes not by parsing the container/codec but by **rendering the untrusted source in a sandbox and re-capturing the rendered frames** (analog-hole / pixel reconstruction), then **resampling/perturbing** to destroy pixel-domain steganography while preserving human-visible content. Software-only, sandboxed/disposable renderer.

**Why it's on the list:** Cross-domain *video* transfer is one of the hardest guard problems and few guards do it well. Open-source CDR (Dangerzone, Qubes converters) covers documents/images only — video is the underserved gap. Fits Izuma posture: software-only, systems-not-silicon, dual-use, open-source-friendly. Research/prototype money is the right entry, not accredited-product bids.

**Signals to match (case-insensitive):** content disarm, CDR, cross-domain video, video sanitization / sanitisation, media guard, transcode guard, pixel reconstruction, render-and-recapture, rasterization, steganography removal, covert-channel removal, video filter for cross domain, verified/trusted media transform.

**Likely homes:** DARPA I2O office-wide BAA (resilient & secure software thrust, `HR001126S0001`), AFRL/RI BAAs, DoW SBIR monthly releases (watch DSIP first-Wednesday drops), NSF SaTC (eligibility-permitting).

**Action on match:** Flag prominently. Note the accreditation posture (research deliverable vs. fielded-product requirement) and any teaming need.

---

## WL-002 — Novel, non-accredited CDS opportunity

**What we're looking for:** Flag any solicitation seeking a **new / greenfield cross-domain capability that does NOT require a pre-existing accredited guard** — i.e., the deliverable is *invention* (research, prototype, proof-of-concept, novel approach), and NCDSMO accreditation is a future step rather than an entry gate.

**Why it's on the list:** Izuma structurally cannot win "bring an accredited, baseline-listed guard" bids (e.g., `ATEC_RIW26_005`, `DON26BZ05-DV082` — those stay a structural pass). But it *can* pursue "invent a better cross-domain transform" research where accreditation isn't the ante. This entry is the deliberate inverse of the accredited-product cross-domain topics.

**Signals to match:** "cross-domain" / "cross domain" / CDS / "security domain" / "data guard" / "multi-level security" appearing **together with** research-flavored language — develop, research, novel, prototype, proof of concept, Phase I, white paper, BAA — and **without** hard-accreditation gating — accredited, NCDSMO baseline, Raise the Bar, fielded, reference-verifiable required, on the baseline list.

**Distinguish from structural-pass topics:** If the solicitation requires an already-accredited / baseline-listed guard, it is NOT a WL-002 match — leave it as the normal structural pass. WL-002 is only the research/greenfield kind.

**Action on match:** Flag immediately and distinctly from accredited-product cross-domain topics. This is a "wake Travis" flag, not routine triage.

---

<!-- Append new watch-list entries below as WL-00N. Keep the same shape: What / Why / Signals / Homes (optional) / Action. -->
