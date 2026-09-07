# Property-Specific Document Sources

Each property may have different documents as sources for the same fields. This file maps which documents contain which data for each property.

## 7207 Fence Line (Column F)

| Field | Source Document | Notes |
|-------|-----------------|-------|
| Mortgage Interest | 1098 | Box 1 |
| Remaining Principle | 1098 | Outstanding principal balance |
| Property Taxes | 1098 | Escrow disbursements section |
| Hazard Insurance | 1098 | Escrow disbursements section |
| Flood Insurance Date, Amount | escrow statement (esc##.pdf) | **Be diligent:** Check escrow docs for year, year+1, AND year-1. The historical activity section often contains prior year data. For 2023 flood, found in esc24's "Activity from Feb 2023 - Feb 2024" section. Row pattern: date row first, then amount row below. |
| Tenant, Deposit, Dates | rental agreement lease.pdf | |
| Fighting prop taxes | FiveStone.pdf | Invoice amount |
| Repairs/Plumbing | invoice-plumbing *.pdf | Sum all invoices |

---

## 1028 Quail Valley (Column G)

| Field | Source Document | Notes |
|-------|-----------------|-------|
| Mortgage Interest | 1098 | Box 1 |
| Remaining Principle | 1098 | Outstanding principal balance |
| Property Taxes | 1098 or escrow statement | |
| Hazard Insurance | **insurance payment.pdf** | Separate from 1098 |
| Tenant, Deposit, Dates | rental agreement lease.pdf | |
| Property Manager | Harrison Peterson | Tenant placement only |

---

## 305 Mesquite (Column H)

| Field | Source Document | Notes |
|-------|-----------------|-------|
| Mortgage Interest | 1098 | |
| Remaining Principle | 1098 | |
| Property Taxes | escrow statement | |
| Hazard Insurance | escrow statement or USAA doc | Insurance company: USAA |
| Property Manager | Goldstar | |
| Ownership | 50% | Partial ownership |

---

## 502 Pine (Column J)

| Field | Source Document | Notes |
|-------|-----------------|-------|
| Ownership | 50% | Partial ownership |
| (Add document sources as known) | | |

---

## 1913 Cypress PT E (Column K)

| Field | Source Document | Notes |
|-------|-----------------|-------|
| Rental | No | Homestead, not rental |
| Tenant | N/A Homestead | |
| (Add document sources as known) | | |

---

## Adding New Properties or Sources

When you learn new document mappings for a property, add them to this file. Format:

```markdown
## Property Name (Column X)

| Field | Source Document | Notes |
|-------|-----------------|-------|
| Field Name | document_name.pdf | Any special parsing notes |
```
