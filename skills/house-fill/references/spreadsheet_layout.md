# Houses Section - Layout Guide

## How to Find Things

The Houses section is in each year's tab. Structure varies year to year.

### Finding the Section
1. Search for "Houses" in column A - that's the section header
2. Property names appear in a header row shortly after (columns F-K typically)
3. Field labels are in column D or E

### Finding a Property Column
```python
find_in_sheet("2023", "7207 Fence Line", 90, 120)
```
This returns the row and column where the property name appears.

### Finding a Field Row
Search for the label text in column D or E:
```python
find_in_sheet("2023", "Mortgage Interest", 90, 170)
```

## Common Field Labels

These are the typical labels to search for (may vary slightly):

- Ready
- Property Manager
- Remaining principle
- 1098 (tax doc link)
- escrow (esc23, esc24)
- Tenant
- move in date
- Deposit
- Rental Start / Rental end
- Rents
- Insurance
- Fighting prop taxes
- Mortgage Interest
- Repairs / Plumbing
- Taxes

## Cell Conventions

| Value | Meaning |
|-------|---------|
| `xxx` | Not applicable, skip |
| `x` | Empty, needs data |
| Empty | Needs data |
| `FALSE` | Ready status - incomplete |
| `TRUE` | Ready status - verified complete |

## Always Discover

Read the actual spreadsheet before writing. Don't assume positions.
