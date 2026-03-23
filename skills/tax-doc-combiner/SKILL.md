---
name: tax-doc-combiner
description: Combine and summarize tax-related documents by category. Use when combining house invoices, creating master invoice summaries, processing expense receipts, or when user mentions combining/merging tax documents for a property.
allowed-tools: Read, Bash, Glob, Grep, Write
---

# Tax Document Combiner

Combines tax-related documents by category, creating summary pages and organized archives.

## Document Types

| Type | Input Pattern | Output | Summary |
|------|---------------|--------|---------|
| **Invoice** | `YYYY house ####-Street invoice*.pdf` | `YYYY houses ####-Street master-invoice.pdf` | Expense table by category |

## Filename Convention

**Property identifier format:** `####-<FirstWordOfStreet>`
- `1234-Main` for 1234 Main Street
- `5678-Oak` for 5678 Oak Avenue

---

# Invoice Combination

## Workflow

1. **Find invoices** matching pattern `YYYY house ####-Street invoice*.pdf`
2. **Read each PDF** to extract vendor, amount, date, and service description
3. **Auto-categorize** each expense based on content
4. **Generate summary table** as first page
5. **Combine PDFs** with summary first, then all invoices
6. **Move originals** to `processed/` subfolder

## Expense Categories

| Category | Keywords/Indicators |
|----------|---------------------|
| `plumbing` | plumber, drain, pipe, faucet, water heater, toilet, leak, sewer |
| `electrical` | electrician, wiring, outlet, panel, breaker, lighting, switch |
| `hvac` | heating, cooling, AC, furnace, duct, thermostat, HVAC, air conditioning |
| `yard` | landscaping, lawn, tree, irrigation, sprinkler, fence, mowing, garden |
| `roofing` | roof, shingle, gutter, flashing, leak repair (roof) |
| `appliance` | appliance, refrigerator, dishwasher, washer, dryer, oven, microwave |
| `flooring` | floor, carpet, tile, hardwood, laminate, vinyl |
| `painting` | paint, painter, interior, exterior, stain |
| `pest` | pest, termite, exterminator, rodent, insect |
| `cleaning` | cleaning, maid, janitorial, pressure wash |
| `general` | handyman, repair, maintenance, misc (fallback category) |

## Summary Table Format

The first page of the master invoice contains:

```
╔══════════════════════════════════════════════════════════════════════════╗
║                    PROPERTY EXPENSE SUMMARY                               ║
║                    YYYY - ####-Street                                     ║
╠══════════════════════════════════════════════════════════════════════════╣
║ Date       │ Vendor                │ Category    │ Description  │ Amount ║
╠════════════╪═══════════════════════╪═════════════╪══════════════╪════════╣
║ 2024-03-15 │ ABC Plumbing          │ plumbing    │ Water heater │ $450.00║
║ 2024-05-20 │ Green Lawn Services   │ yard        │ Spring maint │ $275.00║
║ 2024-08-10 │ Cool Air HVAC         │ hvac        │ AC repair    │ $850.00║
╠════════════╧═══════════════════════╧═════════════╧══════════════╧════════╣
║                                              TOTAL:            │$1,575.00║
╠══════════════════════════════════════════════════════════════════════════╣
║ CATEGORY TOTALS                                                          ║
╠═════════════════╪════════════════════════════════════════════════════════╣
║ plumbing        │ $450.00                                                ║
║ yard            │ $275.00                                                ║
║ hvac            │ $850.00                                                ║
╚═════════════════╧════════════════════════════════════════════════════════╝
```

## Instructions

### Step 1: Find invoices for a property

```bash
ls "<YEAR>/"*invoice*.pdf 2>/dev/null | grep -i "house"
```

Or for a specific property:
```bash
ls "<YEAR>/"*"house"*"<PROPERTY>"*"invoice"*.pdf
```

### Step 2: Read each invoice PDF

For each invoice, extract:
- **Date**: Invoice date or service date
- **Vendor**: Company name
- **Amount**: Total amount (look for "Total", "Amount Due", "Balance")
- **Description**: Service performed

### Step 3: Categorize expenses

Match invoice content against category keywords table above. Use `general` as fallback.

### Step 4: Generate summary and combine

Run the combiner script:
```bash
python3 .claude/skills/tax-doc-combiner/combine_invoices.py \
    --year YYYY \
    --property "####-Street" \
    --input-dir "<YEAR>/" \
    --output "<YEAR>/YYYY houses ####-Street master-invoice.pdf"
```

### Step 5: Move processed files

```bash
mkdir -p "<YEAR>/processed"
mv "<YEAR>/YYYY house ####-Street invoice"*.pdf "<YEAR>/processed/"
```

### Step 6: Report results

List:
- Number of invoices combined
- Total expenses by category
- Grand total
- Output file location

## Dependencies

Install required Python packages:
```bash
make -C .claude/skills/tax-doc-combiner install
```

## Notes

- Always read PDFs before processing to verify they are invoices
- If amount cannot be extracted, ask user for the total
- If category is ambiguous, ask user to confirm
- Keep original files in `processed/` folder (don't delete)
- Master invoice filename uses plural "houses" to distinguish from source invoices

---

# Future Document Types

_Additional document type workflows will be added here as needed._

<!-- Template for new types:
## [Type] Combination

### Pattern
Input: `...`
Output: `...`

### Workflow
1. ...

### Summary Format
...
-->
