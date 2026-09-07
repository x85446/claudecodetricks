---
name: renaming-finance-files
description: Rename and organize financial documents by extracting metadata and applying a standardized naming convention. Supports PDF, PNG, JPG, DOCX, DOC, TXT, MD, CSV, XLS, and XLSX files.
disable-model-invocation: false
---

# Renaming Finance Files

Rename and organize financial documents by extracting metadata and applying a standardized naming convention. Supports PDF, PNG, JPG, DOCX, DOC, TXT, MD, CSV, XLS, and XLSX files.

## Trigger Keywords

Use this skill when: renaming invoices, organizing finance files, processing receipts, moving financial documents, invoice filing, document organization, rename finance, file invoices

## Invocation

```
/rename-finance <source-directory> <destination-directory>
```

Or describe the task naturally: "Rename the finance files in ./inbox and move them to ./archive"

---

## Filename Format

```
<date> <dollar-value> <name> [invoicenum] [tag].<ext>
```

| Field        | Format                   | Example            | Required |
| ------------ | ------------------------ | ------------------ | -------- |
| date         | YYMMDD                   | 260115             | Yes      |
| dollar-value | $x,xxx.cc                | $1,234.56          | Yes      |
| name         | Short, no spaces         | Deel, ReevesSnyder | Yes      |
| invoicenum   | From document or omit    | INV-2026-001       | No       |
| tag          | From entity tags or omit | supplies           | No       |

**Example:** `260115 $1,234.56 Deel INV-2026-001.pdf`

### Name Formatting Rules

- **No spaces in names** - all names must be a single word
- **Personal names**: Combine with PascalCase → "Reeves Snyder" becomes `ReevesSnyder`
- **Company names**: Use the primary name only, drop suffixes like Inc, LLC, Corp → "Deel Inc" becomes `Deel`
- Rarely concatenate company words - keep it simple

---

## Filename-Pattern Renaming

Some files can be renamed purely from their original filename, without reading content. Check these patterns **before** the normal extraction workflow.

### Chase Activity CSV

| Pattern | `Chase<last4>_Activity_<YYYYMMDD>.csv` or `Chase<last4>_Activity<YYYYMMDD>.csv` |
|---------|------|
| Example input | `Chase3839_Activity_20260215.csv` |
| Example output | `260215 Chase 3839 ALLREC.csv` |

**Rules:**
1. Match filename against pattern: `Chase` + 4 digits + `_Activity` + optional `_` + 8-digit date + `.csv`
2. Extract `<last4>` (account last 4 digits) and `<YYYYMMDD>` (download date)
3. Convert date to YYMMDD
4. Rename to: `YYMMDD Chase <last4> ALLREC.csv`
5. No content reading required — move immediately

---

## Workflow Checklist

Copy this checklist for each processing session:

```
- [ ] Step 1: List files in source directory
- [ ] Step 2: Load entities from data/entities.json
- [ ] Step 2.5: For each file, check Filename-Pattern Renaming rules first (rename + move if matched, skip to next file)
- [ ] Step 3: For each remaining supported file (process ONE AT A TIME):
  - [ ] 3a: Read/extract file content
  - [ ] 3b: Match entity name from lookup (or add new)
  - [ ] 3c: Check if entity has custom rules for this file type
  - [ ] 3d: Extract date (use entity rules if defined) → convert to YYMMDD
  - [ ] 3e: Extract dollar amount (use entity rules if defined) → format as $x,xxx.cc
  - [ ] 3f: Extract invoice number (omit if not found)
  - [ ] 3g: Determine tag if entity has tags defined
  - [ ] 3h: Construct new filename
  - [ ] 3i: Move file immediately to destination with new name
  - [ ] 3j: Confirm move succeeded before proceeding to next file
- [ ] Step 4: Report summary
```

---

## Sequential File Processing (1:1)

**Process each file individually: read it, rename it, move it, then proceed to the next file.**

This prevents mismatches between files and their extracted metadata.

### Processing Pattern

For each file:

1. **Read the file** - Extract all content
2. **Analyze immediately** - While the content is fresh in context
3. **Construct the new filename** - Based on what you just read
4. **Move the file now** - Before reading the next file

```bash
mv "<src>/original-filename.pdf" "<dest>/YYMMDD $x,xxx.cc Entity.pdf"
```

### Example Processing Flow

```
Processing: invoice_001.pdf
  → Reading file...
  → Entity: Deel
  → Date: 250724
  → Amount: $0.00
  → Invoice: REC-2025-10
  → New name: 250724 $0.00 Deel REC-2025-10.pdf
  → Moving file...
  ✓ Moved successfully

Processing: statement.pdf
  → Reading file...
  → Entity: Ramp
  → Date: 230825
  → Amount: $11,821.70
  → New name: 230825 $11,821.70 Ramp.pdf
  → Moving file...
  ✓ Moved successfully
```

### Key Rules

- **Read one file at a time** - Do not read multiple files before moving any
- **Move immediately after analysis** - The file you just read is the file you move
- **Confirm each move** - Verify success before proceeding
- **Quote all paths** - Handle spaces and special characters in filenames

---

## File Type Handling

| Extension    | Method                                        |
| ------------ | --------------------------------------------- |
| .pdf         | Read tool (Claude reads PDFs natively)        |
| .png, .jpg   | Read tool (Claude reads images natively)      |
| .docx        | `python scripts/extract_docx.py <filepath>`  |
| .doc         | `python scripts/extract_docx.py <filepath>` (convert to .docx first if fails) |
| .txt         | Read tool                                     |
| .md          | Read tool                                     |
| .csv         | Read tool                                     |
| .xls         | `python scripts/extract_xlsx.py <filepath>`  |
| .xlsx        | `python scripts/extract_xlsx.py <filepath>`  |

### Image Files (PNG, JPG)

Images of invoices/receipts (screenshots, scanned documents) are read natively by Claude. The Read tool displays the image visually, and metadata is extracted from the visible text. The renamed file keeps its original extension (.png or .jpg).

### Word Documents (DOCX, DOC)

DOCX files are extracted using `scripts/extract_docx.py`, which reads paragraphs and table content. For old .doc format files, attempt extraction with the same script; if it fails, ask the user to convert to .docx first. Requires `python-docx` package (`pip install python-docx`).

---

## Extraction Rules

### Date

Search document for (in priority order):

1. "Invoice Date:", "Date:", "Dated:", "Bill Date:" followed by date
2. ISO: YYYY-MM-DD
3. US: MM/DD/YYYY or MM-DD-YYYY
4. Written: "January 15, 2026"

Convert to YYMMDD format.

### Dollar Amount

Search for (in priority order):

1. "Total:", "Amount Due:", "Invoice Total:", "Balance Due:"
2. "Grand Total:", "Net Amount:"
3. Largest dollar amount if no labeled amount found

Format as $x,xxx.cc (always two decimal places).

### Invoice Number

Search for:

1. "Invoice #:", "Invoice No:", "Invoice Number:"
2. "Inv #:", "Reference:", "Transaction:"
3. Patterns: INV-XXXX, #XXXX

**Omit from filename if not found** - do not use placeholders.

### Entity Matching

1. Load `data/entities.json`
2. Search document text for entity names and aliases
3. Match is case-insensitive
4. If no match found → prompt user to add new entity or select existing

### Tag Determination

1. Check if matched entity has tags defined (not null)
2. For each tag, search document for its keywords
3. Apply first matching tag to filename
4. If no keywords match or entity has no tags → omit tag

### Entity-Specific Rules

Some entities have custom extraction rules that override the defaults. Check the entity's `rules` field in `data/entities.json`.

**IMPORTANT:** After matching an entity, check if it has `rules` defined. If so, use those rules instead of the default extraction logic.

Example entity with rules:

```json
{
  "name": "Ramp",
  "rules": {
    "date": {
      "pdf": "Use 'Statement End' date from the header",
      "csv": "Use the last date in the 'Transaction Date' column"
    },
    "amount": {
      "pdf": "Use 'Current Balance' amount",
      "csv": "Use the final 'Outstanding Balance' value"
    }
  }
}
```

**Extraction order:**

1. Match entity first
2. Check if entity has `rules` for the current file type
3. If rules exist → follow them exactly
4. If no rules → use default extraction logic

---

## Entity Management

The entity lookup builds incrementally. When no entity matches:

```
No entity match found. Document contains text like:
  "Johnson & Associates", "Professional Services Invoice"

Options:
1) Add new entity
2) Select from existing entities
3) Skip this file

If adding new entity:
- Name: Johnson & Associates
- Aliases (comma-separated): J&A, Johnson Associates LLC
- Tags: null (or define tag keywords)
```

After adding, update `data/entities.json` with the new entity.

---

## Error Handling

| Situation          | Action                     |
| ------------------ | -------------------------- |
| Date not found     | Ask user for date          |
| Amount not found   | Ask user for amount        |
| No entity match    | Prompt to add/select/skip  |
| Invoice not found  | Omit from filename         |
| File unreadable    | Skip with warning          |
| Destination exists | Ask: overwrite/rename/skip |

---

## Validation

If `scripts/validate_filename.py` exists, validate each filename before moving:

```bash
# Validate the filename for the current file
python scripts/validate_filename.py "260115 $1,234.56 Deel.pdf"
```

Only proceed with the move if validation passes. If the script doesn't exist, verify the filename matches the format manually:

- `YYMMDD $x,xxx.cc Name [invoicenum] [tag].ext`

---

## Summary Report

After processing, display:

```
=== Processing Complete ===
Files processed: X
Successfully renamed: X
Skipped: X
  - filename.pdf: reason

Destination: <destination-directory>
```

---

## Reference Files

- **REFERENCE.md** - Detailed extraction patterns and edge cases
- **EXAMPLES.md** - Input/output examples for each file type
- **data/entities.json** - Entity lookup table
