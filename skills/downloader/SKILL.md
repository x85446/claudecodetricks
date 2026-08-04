---
name: downloader
description: "META skill owning the browser download layer. Use when someone asks to download statements or documents, run the downloaders, fetch or close document gaps, check download status/coverage, regenerate the WANT manifest, or set up a download sweep. Trigger phrases: 'download the gaps', 'get the missing statements', 'run the downloader', 'download status', 'sweep <site>'."
argument-hint: "[status | want | sweep <site> | gaps]"
---

# Downloader — the browser download layer (META)

Fourth META skill (organizer, categorize, importer, **downloader**). Owns
getting source documents from financial institutions onto disk. Its job **ends
at the filesystem**: files land in `data/incoming/` under the filename
convention; `/importer` (usually via `/organizer`) parses them into the DB.
The two never run in the same step.

**Authoritative contract: `docs/tampermonkey.md`.** Read it before changing
any handler. The essentials:

1. **The database is the sole authority on what is missing.** `accounts` +
   `account_documents` record, per account, every expected statement period /
   transaction image and whether it is `have | missing | ignored`. Never
   decide from what's on screen.
2. **`scripts/gen_want_manifest.py` bakes gaps into the userscript** — it
   rewrites the `AUTO-GENERATED:WANT` block in
   `scripts/browser/tampermonkey/downloaders.body.js`. WANT/HAVE blocks are
   generated, NEVER hand-edited: edit the DB, regenerate.
3. **Handlers sweep every account unattended.** One click of the injected
   Download button iterates all accounts, selects years, expands sections,
   fetches only WANT-missing periods, stops at the current period. Needing a
   human to expand/select anything is a defect, not a limitation.
4. **Human-gated login only.** The agent never enters credentials or bypasses
   CAPTCHA/bot-detection. Travis signs in; once a session exists, the injected
   button (or the agent via claude-in-chrome) drives the run.
5. After import, `scripts/migrate_accounts.py` flips filled gaps
   `missing`→`have`; regenerating WANT shrinks the next sweep. A re-run after
   a successful sweep downloads nothing.

## Document trackers (query BEFORE prescribing any download)

`accounts` + `account_documents` = every monthly statement (status have/missing/ignored, file_path). `source_documents` = every transaction document ref (amazon/lowes/homedepot/ebay/godaddy orders & receipts, incl. docs living inside master PDFs/JSON containers via container_path). `scripts/gen_want_manifest.py` writes gaps-only WANT + DOC_HAVE blocks into downloaders.body.js from these tables — handlers download ONLY refs absent from DOC_HAVE. Refresh flow: `python3 scripts/build_doc_catalog.py && python3 scripts/migrate_source_documents.py && python3 scripts/gen_want_manifest.py`. A download request that hasn't checked `v_missing_statements` / `v_missing_source_docs` is a bug.

## Toolbox

| Tool | Role |
|---|---|
| `scripts/browser/tampermonkey/downloaders.body.js` | single userscript body: all site handlers + WANT/HAVE blocks + injected controls |
| `scripts/browser/tampermonkey/downloaders.stub.user.js` | Tampermonkey stub (pasted once; @require's the body from disk — needs "Allow access to file URLs") |
| `scripts/gen_want_manifest.py` | DB → WANT block regeneration |
| `scripts/migrate_accounts.py` | seed/refresh `accounts` + `account_documents`; flips missing→have after import |
| `scripts/download_sources.json` | Download Reporter config: one row per (source, account) with launch URL |
| Download Reporter (app, Tools menu) | per-account last-download/statement/record view + Launch-in-Chrome |
| claude-in-chrome MCP | open tabs, watch for login, click injected buttons, finalize scaffold selectors live |
| `/importer` | ingest handoff — NEVER imported from here |

Site handler status (Downloads/Through/→incoming/→DB) lives in the
"Supported sites" + "Features implemented" tables in `docs/tampermonkey.md`;
update them (terse, present tense) whenever a handler's state changes.

## Children

| Child | Scope |
|---|---|
| `downloader-orderdocs` | Per-order transaction documents (ebay order pages, amazon invoices, HD invoices, lowes order details) — closes `source_documents` missing refs. |

## Invocations

### /downloader status
Report the gap picture without changing anything: per (source, account) from
`accounts`/`account_documents` — counts of have/missing/ignored, the missing
period ranges, last file in `data/processed`+`data/incoming`, and the site's
handler status from docs/tampermonkey.md. SQL:
```sql
SELECT a.account_key, a.source,
       SUM(d.status='have') have, SUM(d.status='missing') missing,
       SUM(d.status='ignored') ignored, MIN(CASE WHEN d.status='missing' THEN d.period END) first_gap,
       MAX(CASE WHEN d.status='missing' THEN d.period END) last_gap
FROM accounts a LEFT JOIN account_documents d ON d.account_id=a.id
WHERE a.active=1 GROUP BY a.account_key ORDER BY missing DESC;
```

### /downloader want
Refresh the manifest: `python3 scripts/migrate_accounts.py` (if inventory
changed) then `python3 scripts/gen_want_manifest.py`; show the WANT diff.
Idempotent — a second run produces no diff.

### /downloader sweep <site>
Run one site's download end-to-end:
1. `/downloader want` first so the WANT block is current.
2. Open the site's URL (from `download_sources.json`) in Chrome via
   claude-in-chrome; if the domain is blocked, ask the operator to allow it in
   the extension. Wait for Travis's signed-in session (poll the tab).
3. Trigger the injected Download button (or `GM_registerMenuCommand`). For
   scaffold handlers (lowes/homedepot/ebay/paypal/godaddy) the first live run
   logs candidate controls — read the console, finalize the selectors in
   `downloaders.body.js` (reload re-reads it from disk), re-run.
4. Watch the status pill/console until `DONE … 0 wanted remaining`, or record
   the shortfall as institution retention (e.g. USAA ~18 months) in the report.
5. Confirm files landed in `data/incoming/` named per the convention
   (`YYMMDD-<suffix>.<ext>`, suffix from `download_sources.json`).
6. Hand off: tell the user to run `/importer` (or `/organizer process`) —
   do not import here. After import completes, run `/downloader want` to
   shrink the manifest and update the docs status tables.

### /downloader gaps
Like `status` but focused: print only accounts with `missing > 0`, as a
download worklist ordered by missing count, each with its launch URL.

## Guardrails

- Downloads end at the filesystem. Importing is `/importer`'s job.
- Never hand-edit AUTO-GENERATED WANT/HAVE blocks.
- Never enter credentials, never bypass CAPTCHA/bot-detection; login is
  Travis's step. Chrome may need per-site "allow multiple downloads".
- "Best-effort" and "the currently-selected account" are defects — handlers
  sweep everything unattended or they're unfinished (fix the handler).
- Gaps older than the institution's online retention are marked `ignored` in
  `account_documents`, not chased forever.
- Children follow the taxonomy: `downloader-<child>`; back up new skills to
  `~/workspace/x85446/claudecodetricks/skills/` and register in
  `skillinstall.sh` ($PERSONALDB).
