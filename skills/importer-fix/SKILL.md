---
name: importer-fix
description: Use to repair a specific source's importer bugs and the bad DB rows they created — fix godaddy prices, repair homedepot/lowes/amazon price errors, resolve dangling transaction ids, or act on an auditor punch list for one source. Child of the importer meta. Repairs toward the source contract; the auditor's detection SQL going to zero is the exit criterion.
argument-hint: [godaddy | homedepot | amazon | lowes | etrade | <source>]
---

# Importer-Fix — per-source repair toward the contract

Repairs ONE source at a time: first the importer code path, then the already-imported rows in `db/personaldb.sqlite`. The compartment of responsibility is the SOURCE (its contract file), not a separate skill per vendor.

## Context to load (in order)

1. `docs/sources/<source>.md` — the contract (price definition, natural key, format eras, multi-format policy, known limitations). If missing, WRITE IT FIRST from `docs/sources/CONTRACT-TEMPLATE.md` + a data/ inventory; the contract is the repair target, no repairs without it.
2. `.claude/data/auditor/error-types.md` — registry entries whose detection SQL names this source.
3. `.claude/data/auditor/punchlists/*-<source>*.md` (newest first) — the findings, sample ids, owners.
4. The source's importer code: `scripts/<source>*_import*.py` / `scripts/*_<source>*.py` (e.g. `godaddy_csv_import.py`, `lowes_import.py`, `import_homedepot_pdf.py`, `amazon_csv_import.py`).

## Workflow

1. **Fix the importer code first.** Make the code path produce contract-correct values for every format era listed in the contract. A DB repair without the code fix just re-breaks on the next import.
2. **Prove the code fix on a real file** from `data/processed/` (dry-run/preview mode if the script has one): parsed output must satisfy the contract (line sums = receipt totals, whole cents, correct net/gross).
3. **Pre-repair safety ritual** (skip only if already done this session and no app ran since): quit host AND VM PersonalDB apps; verify `PERSONALDB_DB` is unset and the target resolves to `db/personaldb.sqlite`; announce "writing to main DB"; `VACUUM INTO 'db/backups/<UTCdate>-prerepair.sqlite'`; `PRAGMA integrity_check`; `PRAGMA wal_checkpoint(TRUNCATE)`.
4. **Repair the DB rows** the punch list identifies, deriving each new value from src data or source files per the contract — never from guesswork. Every UPDATE sets `updated_at = datetime('now')`; FK columns only.
5. **A-flag every touched row**: pick ONE unused A column for this run (`python3 scripts/ai_flag.py count` to see usage), set it on every modified/deleted-adjacent row, write the per-row reason to the matching `aN_notes`. Log which column in the run report. Never write `notes`.
6. **Apply the human_verified doctrine** (CLAUDE.md "Human validation doctrine"): any price/date change or deletion on a verified row sets `human_verified = 0` (the aN_notes reason says verification was cleared and why); validated category/cost center/merchant stay intact; context-preserving description edits keep the flag, meaning-class changes clear it.
7. **Re-run the auditor's detection SQL** for every error type this repair addresses: all must return 0 (or the documented-legit remainder). Then update `.claude/data/auditor/error-types.md` Last-checked counts and append a fix-confirmation line to the source's punch list. If a detection won't reach 0, do NOT force it — record the residue and why.
8. **Report**: rows repaired / deleted / un-verified (counts), A column used, detection SQL before→after counts, importer files changed, contract sections updated.

## Hard rules

- One source per run; one A column per run.
- Never edit the auditor's findings to match the data — repairs move DATA toward the CONTRACT, never the contract toward broken data (contract changes require the source files to prove the contract wrong).
- Deletions (double entries, phantom charges): verify against the source document before deleting; deletion of a verified row satisfies the re-validation rule by definition.
- Single-writer: no sqlite writes while either app runs; backups are output-only.
- Never clear A flags — Travis clears them after his in-app review (`python3 scripts/ai_flag.py clear <col>` is his command, not yours).
- src rows and transactions stay lifecycle-coupled: if a transaction is deleted, disposition its src row in the same run (delete or re-link — no new ET-006 danglers).
