---
name: house-fill
description: Fill the Houses section of T&M Master spreadsheet by extracting data from PDFs in Google Drive.
allowed-tools: Read,Bash(python3:*)
---

# Fill Houses Section

Populate a property column in the Houses section by extracting data from source PDFs.

## Workflow

1. **Discover structure** - Read the spreadsheet to find where the property column is and what rows contain which labels
2. **List PDFs** - Find all PDFs for this property/year in Google Drive
3. **Read property sources** - Check `{baseDir}/references/property_sources.md` for which docs contain which fields
4. **Extract data** - Download PDFs and parse values
5. **Write with hyperlinks** - Update cells with values linked to source PDFs

## Discovery First

**Never assume row/column positions.** Always discover by reading:

```python
# Find property column
python3 -c "
from fill_column import find_in_sheet
results = find_in_sheet('2023', '7207 Fence Line', 90, 120)
print(results)
"

# Read the houses section to see current layout
python3 -c "
from fill_column import read_section
data = read_section('2023', 90, 165, 'D', 'L')
"
```

## Utility Scripts

```bash
# List PDFs for property/year
python3 {baseDir}/scripts/list_pdfs.py "7207 Fence Line" 2023

# Extract text from a PDF
python3 {baseDir}/scripts/download_pdf.py <file_id>

# Write a cell (with optional hyperlink)
python3 -c "
from fill_column import write_cell
write_cell('2023', 'F111', '1098', as_hyperlink=True, file_id='abc123')
"
```

## References

- **Property Sources:** `{baseDir}/references/property_sources.md`
  - Which documents contain which fields for each property
  - Some properties use different docs (e.g., 1028 Quail has separate insurance doc)

- **Layout Guide:** `{baseDir}/references/spreadsheet_layout.md`
  - Field label names to search for
  - General structure guidance

## Output Format

All values with source PDFs should be HYPERLINK formulas:
```
=HYPERLINK("https://drive.google.com/file/d/{ID}/view", "value")
```

## Completion

Leave Ready=FALSE until user confirms all data is correct.
