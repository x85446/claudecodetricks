# Izuma Networks — Company Profile (for grant-scout)

This file is the source-of-truth profile the `grant-scout` skill uses to grade incoming grants. Update this file when the company posture changes — the skill re-reads it every run.

## One-line elevator

Izuma Networks builds secure, federated, software-only infrastructure for edge environments where connectivity, trust boundaries, and tenancy cannot be assumed — and where operations must continue when any of them fail.

## Heritage and posture

- Founded on the codebase formerly known as Arm Pelion / mbed Cloud — over a decade of platform evolution at carrier and IoT scale.
- SoftBank-backed.
- Treat us as a small business with proven-at-scale technology — not a fresh-out-of-stealth startup. Grants that demand a "first-time founder" or "early-stage research validation" posture will mis-fit; grants that want a small business with a working product fit well.
- Open-source-friendly: Apache 2.0 for the edge stack, contributes upstream to Kubernetes / vCluster / LwM2M ecosystems. No vendor lock-in is a deliberate positioning choice.

## Product portfolio

| Product | One-liner | Maturity |
|---|---|---|
| **Izuma Device Management** | Chip-to-cloud IoT — secure connectivity, FOTA, fleet ops for MCU → edge servers. LwM2M, X.509 mTLS, hardware root of trust. | Shipping (mature; ex-Pelion lineage) |
| **Izuma Edge** | Kubernetes for the edge — true k8s, offline containers, SD-WAN, Apache 2.0. | Shipping |
| **Myriplane** | Federated, cryptographically isolated Kubernetes control planes — one dedicated control plane per tenant, CRDT + gossip federation, designed for DDIL. | Active development |
| **Exonet** | Distributed encrypted multipoint networking — triple-layer encryption (TLS + MLS + Noise PSK), CRDT KV store, WireGuard mesh, software-only across Linux/macOS/Windows. | Active development |
| **Izuma Metalworks** | Cloud-managed bare-metal provisioning over LAN or WAN, two curated Izuma host OSes, TPM 2.0 attestation. | **2027 roadmap** |
| **VirtualCluster operator** | Open-source operator underlying Myriplane. Tenancy API, syncer, vn-agent. Being hardened toward `v1beta1` / `v1` API. | Open-source, modernizing |

## What makes Izuma "Izuma" (personality traits)

These traits are how Izuma builds. They shape the grade, not just the topic match.

1. **Open-source and open-standards bias.** Apache 2.0, Kubernetes, LwM2M, CoAP, WireGuard, MLS (RFC 9420), Noise, OPA, OIDC. Grants that demand proprietary lock-in deliverables, sole-source IP capture, or closed reference designs are a poor cultural fit even when the topic matches.

2. **Software-only — we don't fabricate.** We **integrate** hardware roots of trust (TPM 2.0, Arm TrustZone, PARSEC) but we do not design, fab, tape out, or build silicon, FPGAs, ASICs, sensors, radios, or any physical hardware. Grants asking us to lead silicon, chip, or device-fabrication work as the primary deliverable are out of scope.

3. **Long heritage, proven-at-scale.** Over a decade of platform evolution (Arm Pelion / mbed Cloud lineage), SoftBank-backed, deployed at carrier scale. Grants framed for brand-new startups or first-time entrants do not fit; grants that want demonstrated production-grade software from a small business do fit.

4. **Dual-use — civilian and defense, same stack.** We refuse to fork the product into a civilian SKU and a defense SKU. The same Exonet / Myriplane / Edge stack serves DDIL tactical and critical-infrastructure / industrial customers. Grants that force a civilian-only or defense-only path that would require a product fork are a personality misfit; grants that explicitly value dual-use are a strong personality fit.

5. **Systems, not models.** We build the substrate. We are not an AI/ML model R&D shop. Grants that fund the substrate AI/ML runs on are great; grants that fund the model itself are usually a skill mismatch.

6. **Honest about scope.** A grant we can't deliver is a grant we shouldn't pursue. The skill should penalize execution-overreach, not paper over it.

## Customer segments (highest fit → still in scope)

All four are valid. Weight roughly in this order when scoring "Customer Segment Alignment":

1. **DoD / defense.** Army, Navy, Air Force, Space Force, Marine Corps, SOCOM, DIU, DARPA, DTRA, DLA, DISA. Tactical edge, DDIL, encrypted mesh, sovereign infrastructure, classified-network-adjacent.
2. **Intelligence community.** ODNI, NSA, NGA, NRO, In-Q-Tel. Classified / SCIF deployments, air-gapped operation, sovereign control planes.
3. **Federal civilian — critical infrastructure focus.** DHS / CISA, DOE (grid resilience, ARPA-E), NIH (medical IoT, secure clinical data), NSF (CISE programs only), NASA (edge compute, mission ops). Less natural than DoD/IC but valid when the topic is critical infra.
4. **Industrial / OT / smart-grid.** Manufacturing, energy utilities, water, oil & gas. Often arrives as DOE / NIST co-funded programs or as commercial pilots layered on top of federal R&D money.

Out of scope: education, K-12 STEM, pure academic theory grants with no commercialization path, ad-tech, social media, consumer apps.

## 18-month roadmap weight

All four product directions are in active investment. Treat them as roughly equal-weight when mapping grants to roadmap fit, with mild emphasis on Exonet + Myriplane since those are the products in most active development right now:

1. **DDIL networking and secure mesh** (Exonet — strong)
2. **Multi-tenant federated Kubernetes** (Myriplane / VirtualCluster — strong)
3. **Bare-metal and hardware trust** (Metalworks — 2027 roadmap; grants here are good but bias toward design-partner/early-adopter framing)
4. **IoT device management and FOTA** (Device Management — mature; grants here only score high if they fund specific extensions: post-quantum cryptography migration, Wi-SUN at scale, Matter integration, OT/ICS protocol bridges)

## Realistic execution scope

- **SBIR / STTR Phase I** — always in scope. Comfortable solo.
- **SBIR / STTR Phase II** — always in scope. Comfortable solo or with one partner.
- **Direct-to-Phase-II** — in scope if we have a Phase-I-equivalent demonstration we can point to.
- **Mid-size cooperative R&D ($2M–$10M)** — only if (a) it's an exact product fit AND (b) we can find a prime / systems-integrator partner. The skill should flag the partner requirement explicitly.
- **Large multi-year prime program ($10M+, 3–5yr, must lead as prime)** — stretch. Score reflects that we'd need significant team scale-up; recommend treating as "needs partner" unless the user says otherwise.

## Anti-fit signals (NOT a blacklist — read each grant)

Per the user's explicit instruction, do not auto-fail grants by keyword. Read the grant, judge it against the rubric. The following are common patterns that *tend* to score poorly, but each should be evaluated on its merits:

- Grants where the primary deliverable is silicon, FPGA RTL, ASIC tape-out, sensors, radios, or other physical hardware.
- Grants where the primary deliverable is a trained ML model, dataset, or model R&D (not the substrate on which the model runs).
- Grants that require lab/wet-bench biology, materials science, or chemistry.
- Grants that demand proprietary closed-IP deliverables incompatible with our open-source posture.
- Grants that explicitly forbid dual-use or that would force a civilian/defense fork of the codebase.

If a grant looks like it should be a hard "F" but the rubric scores it higher, write the report anyway and flag the mismatch in the "Concerns" section. The user makes the call.

## Reference docs in this repo

When grading, also skim the current product one-pagers to ground in real capability claims (not stale profile copy):

- `products/device-management/one-pager.md`
- `products/edge/one-pager.md`
- `products/exonet/one-pager.md`
- `products/metalworks/one-pager.md`
- `products/myriplane/one-pager.md`
- `products/virtualcluster/status-current.md` (operator status, current branch state)
- `products/virtualcluster/roadmap.md` (planned operator hardening)

If a grant maps to something the one-pager doesn't claim, that's a signal — flag the skillset axis as `medium` confidence.
