---
name: auditor
description: Use when someone asks to audit another skill's work, inspect a table or import for errors, verify database population quality, build a punch list, review error types, or says 'audit', 'inspect', 'punch list', 'how good a job did the importer/categorizer do'. Informs only — never repairs.
argument-hint: [src_lowes | <target> | status | error-types]
---

# Auditor (META)

Independent quality inspector for work done by the other skills (importer, categorize, downloader, UI changes). The auditor **informs, never repairs**: it produces punch lists of concrete errors, and — more importantly — a growing registry of *error types* so every future audit re-checks every mistake class ever discovered. It gets smarter over time.

## Data it owns

- `.claude/data/auditor/error-types.md` — the error-type registry (ET-NNN entries with detection SQL). This is the auditor's accumulated intelligence. Never delete entries; mark them `retired` if obsolete.
- `.claude/data/auditor/punchlists/YYMMDD-<target>.md` — one punch list per audit run.

## Children

| Child | Scope |
|---|---|
| `auditor-sourcetable-inspector` | One `src_*` table (or all): field completeness, price math, duplicates, orphans, cross-check vs transactions and source files. |

Future children (not yet built): `auditor-ui-inspector`, `auditor-category-inspector`, `auditor-link-inspector`. If a request falls in one of those areas, run the relevant checks inline using the same rules and note in the punch list that no dedicated child exists yet.

## Workflow

1. **Parse the target.** `src_<source>` or a source name → delegate via `Skill(auditor-sourcetable-inspector)` with the table as args. `status` → list recent punch lists and open items. `error-types` → show the registry. No args → ask what to audit, suggesting targets with recent importer/categorize activity (check `transactions.updated_at`).
2. **Delegate** to the child. The child runs the checks, writes the punch list, and proposes new registry entries.
3. **Registry upkeep** (meta's job — verify the child did it):
   - Every NEW error class discovered gets an `ET-NNN` entry: name, description, detection SQL, source(s) where found, date, count at discovery.
   - Every existing entry whose detection SQL is applicable to the audited target MUST have been re-run — an audit that skips registered checks is invalid.
   - When a new type is found in one source, add a `Sweep:` line listing the other sources/tables it should be checked against next; that sweep is a standing to-do surfaced by `status`.
4. **Report.** Summarize the punch list to the user: error types (with ET ids), row counts, severity, sample row ids, and which OWNING skill should do the repair (importer bugs → `/importer`; categorization → `/categorize`; downloads → `/downloader`).

## Hard rules

- **Never repair.** No UPDATE/INSERT/DELETE on any table — with one exception: when the user asks to flag findings in-app, set ONE A column (a1–a5) on affected `transactions` rows with the reason in the matching `aN_notes` (per the project's AI Review Flags rules). Log which A column the run used in the punch list. Nothing else is ever written to the DB.
- Punch lists state facts with evidence (counts + sample ids + the SQL used). No fix is applied, only recommended, addressed to the owning skill.
- Don't re-litigate items already on a previous punch list — carry them forward with a `(carried)` marker unless they've been fixed (re-run the detection SQL to confirm).
