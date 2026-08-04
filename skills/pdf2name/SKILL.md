---
name: pdf2name
description: Use when someone asks to rename PDFs to the standard filename format, name or file bill/statement/invoice PDFs, clean up PDF filenames from their contents, or run pdf2name. Reads each PDF's internals and renames it to "YYMMDD accountID source $amount description.pdf".
disable-model-invocation: true
argument-hint: [PDF file or folder of PDFs]
allowed-tools: Bash, Read, Glob
---

## What This Skill Does

Opens PDF(s), reads what's actually inside, and renames each to the **standard filename**:

```
YYMMDD accountID source $amount description.pdf
```

Example: `250502 305 citimortgage $837.89 mortgage bill.pdf`

| Field | Meaning | How to determine it |
|---|---|---|
| `YYMMDD` | The document's **own** date | Prefer statement/bill/invoice date; fall back to service-period end, then due date. 2-digit year. |
| `accountID` | Which account | **For houses: the abbreviated street address** (e.g. `305` for 305 Mesquite, `1028` for 1028 Quail Valley). Otherwise the account holder/number. |
| `source` | Institution / vendor | Lowercase, short: `citimortgage`, `wellsfargo`, `cityofwater`, `atmos`, `farmers`. |
| `$amount` | The bill/payment amount | The amount due / payment / total charges, as `$xxx.xx`. |
| `description` | What the doc is | Short, abbreviated: `mortgage bill`, `water bill`, `electric bill`, `insurance`, `escrow analysis`. |

Naming this way makes the files sort chronologically and feeds directly into the `source2pdf` skill (which merges a `_sources/` folder in filename order).

## Steps

1. **Resolve the target** from `$ARGUMENTS`: a single `.pdf`, or a folder of PDFs (top-level by default; pass `--recursive` to the scan if the user wants nested). If no argument, ask for the path.

2. **Scan candidate fields** — run the helper to pull dates, amounts, institution and address keywords from each PDF:
   ```bash
   python3 ~/.claude/skills/pdf2name/scan.py "$ARGUMENTS"
   ```
   For any file the scanner marks as **"no extractable text"** (scanned image), open it with the Read tool to inspect it visually instead.

3. **Compose each field yourself** from the scan output + your own reading. The scanner suggests; you decide:
   - **Date** → convert the chosen `YYYYMMDD` to `YYMMDD`. Pick the *statement/bill* date, not the due date, unless only a due date exists.
   - **accountID** → for a property, abbreviate the address to its street number (or number + short street if numbers collide, e.g. `305 mesquite`). For other accounts, use a short stable identifier. If you can't tell the account, flag the file — don't guess.
   - **source** → normalize the institution to a short lowercase token.
   - **amount** → the primary amount due/paid. If a statement has several, use the total payment/amount due.
   - **description** → 1–3 words. Use `escrow analysis` for escrow docs (they file separately from monthly `mortgage bill`s).

4. **Propose renames as a table** (old → new) for the whole batch, and ask the user to confirm. Do NOT rename before confirmation (mode: infer + confirm).
   - Sanitize: no `/`, no newlines; collapse repeated spaces; keep the `$`.
   - If a required field can't be determined, list the file under "needs input" with what's missing — leave it unrenamed rather than inventing a value.

5. **On confirmation, rename in place** with `mv` (same directory; this skill renames, it does not move between folders). Handle collisions by appending ` (2)`, ` (3)`. Report every rename and every skipped file.

## Output (confirmation table)

```
Proposed renames (N files):
  OLD                                   ->  NEW
  citimortgage statements.pdf           ->  250502 305 citimortgage $837.89 mortgage bill.pdf
  scan0007.pdf                          ->  240115 1028 cityofwater $64.20 water bill.pdf

Needs input (not renamed):
  weird-doc.pdf  — no amount found; is this a statement or a notice?

Rename these? (yes / edit / skip)
```

## Prerequisites

Needs `pdftotext`, `pdfinfo` (poppler); `qpdf` optional (lets the scanner read encrypted statements). Install: `brew install poppler qpdf`.

## Notes

- **Read before renaming.** The filename on disk is often wrong or misleading — always derive fields from the PDF's contents, never from its current name.
- **Watch for mailing vs property address.** On statements for a property held by an estate/LLC, the printed address may be the *mailing* address, not the property. Disambiguate by loan/account number or payment amount, and use the **property** for `accountID`.
- **Don't fabricate.** If the date, amount, or account is genuinely unreadable, surface it for the user rather than guessing.
- **This skill only renames** (stays in the same folder). Moving files into house/account folders, or merging into a super-PDF, is out of scope — hand off to `source2pdf` for the merge step.
- Escrow analyses get `description = escrow analysis` so they can be separated from `mortgage bill` statements downstream.
