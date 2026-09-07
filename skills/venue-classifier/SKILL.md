---
name: venue-classifier
description: "Use when someone asks to classify merchant venue types, run venue classification, backfill venue_type, or asks what kind of business merchants are (restaurant, coffee shop, hotel, etc.). Trigger phrases: 'classify venues', 'venue types', 'run the venue classifier'."
argument-hint: "[--years 2024,2025,2026] [--dry-run]"
disable-model-invocation: true
---

## What This Skill Does

Fills `merchants.venue_type` in `db/personaldb.sqlite` with a controlled-vocabulary
business type (restaurant, coffee shop, hotel, gas station, …) plus
`venue_type_source` ('claude' | 'websearch' | 'manual') and
`venue_type_confidence` (0–1). Idempotent: only rows with `venue_type IS NULL`
are ever written; re-running never overwrites earlier classifications
(re-classify by manually NULLing a row first).

**Vocabulary:** `classification_map.json` (next to this file). venue_type values
MUST come from that file — the apply script rejects anything else.
**Helper script:** `scripts/venue_classify.py` (paths relative to the personaldb
project root; pass `--db` if running from elsewhere).

## Steps

1. **Parse arguments.** `--years 2024,2025` limits scope to merchants that have
   transactions in those years; no argument = all merchants. `--dry-run` means
   validate and report but write nothing.

2. **List candidates:**
   ```bash
   python3 .claude/skills/venue-classifier/scripts/venue_classify.py list --years <years> --limit 500
   ```
   Output: `id  name  txn-count  sites  sample-item` — the sample item text and
   sites give context for ambiguous names.

3. **Classify knowledge-first.** For each merchant, assign a venue_type from
   `classification_map.json` based on what you know (Starbucks → coffee shop,
   Marriott → hotel, "Transfer-…" → not-a-venue). Confidence: 0.95+ for
   household names, 0.8–0.9 for confident inferences from the name/sample item
   (e.g. "…TACO HOUSE… AUSTIN TX" → restaurant), below 0.8 means go to step 4.
   Source: `claude`.

4. **WebSearch fallback.** For descriptors you cannot confidently identify
   (e.g. "SE40820 (Corpus Christi TX)"), WebSearch the name plus its city.
   If identified, classify with source `websearch` and confidence per the
   evidence. If still unidentifiable after a search, use venue_type `unknown`,
   source `websearch`, confidence 0.0 — never guess a specific type without
   evidence.

5. **Write the batch JSON** to the scratchpad (or `temp/`):
   ```json
   [{"id": 123, "venue_type": "coffee shop", "source": "claude", "confidence": 0.95}, ...]
   ```

6. **Apply:**
   ```bash
   python3 .claude/skills/venue-classifier/scripts/venue_classify.py apply --json <file> [--dry-run]
   ```
   The script validates vocabulary/source/confidence, skips already-classified
   rows, and reports `applied / skipped / errors`. Fix any errors and re-apply.

7. **Repeat** steps 2–6 until `list` returns no rows for the requested scope
   (batches of ~100–200 keep each classification pass careful).

8. **Report:**
   ```bash
   python3 .claude/skills/venue-classifier/scripts/venue_classify.py stats --years <years>
   ```
   Show the coverage line and per-type counts, plus how many needed websearch
   and how many ended `unknown`.

## Notes

- **Never invent vocabulary.** If a business doesn't fit, pick the closest
  type or `unknown` — don't create new labels (that's a deliberate edit to
  classification_map.json).
- **not-a-venue** covers transfers, card payments, interest, payroll, fees,
  crypto — anything that isn't a place/business you buy from.
- **Multi-line businesses** (Amazon = online retail even though it sells
  everything; hotel restaurants → restaurant if the merchant is the restaurant,
  hotel if it's the property).
- The DB is live for the PersonalDB.app — no schema changes here, only the
  three venue columns, and only NULL→value transitions.
- Cost guardrail: WebSearch only merchants that knowledge can't handle;
  typically well under 20% of a batch.
