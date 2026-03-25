---
name: pm-iterator
description: Manage named reusable value lists (iterators). Like C macros — define once, reference everywhere. Used in features, requirements, and tests.
argument-hint: [product create NAME "desc" val1,val2|list|show NAME|add NAME val|remove NAME val]
---

# PM Iterator — Reusable Named Lists

Manage named value lists that other skills reference instead of hardcoding. Like a C macro or enum — define once, reference by name everywhere.

## Invocation

```
/pm-iterator myriplay create SOFTWARE_ARCHS "Target build architectures" x86,amd64,arm64,mips
/pm-iterator myriplay list
/pm-iterator myriplay show SOFTWARE_ARCHS
/pm-iterator myriplay add SOFTWARE_ARCHS riscv64
/pm-iterator myriplay remove SOFTWARE_ARCHS mips
/pm-iterator myriplay rename SOFTWARE_ARCHS BUILD_TARGETS
/pm-iterator myriplay delete SOFTWARE_ARCHS
```

## Database

**Path:** `.claude/db/marketing.sqlite` | `PRAGMA foreign_keys=ON;`

Uses `iterators` and `iterator_values` tables.

**UUID generation:**
```sql
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' ||
  substr(hex(randomblob(2)),2) || '-' ||
  substr('89ab',abs(random()) % 4 + 1, 1) ||
  substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6)));
```

## Create

1. Validate name is UPPER_SNAKE_CASE
2. Check for uniqueness within the product
3. INSERT into `iterators`
4. INSERT each value into `iterator_values` with sequential positions
5. Show: `Created iterator: SOFTWARE_ARCHS (x86, amd64, arm64, mips)`

## List

```sql
SELECT i.name, i.description, GROUP_CONCAT(iv.value, ', ' ORDER BY iv.position) AS vals
FROM iterators i
JOIN our_products op ON op.id = i.product_id
LEFT JOIN iterator_values iv ON iv.iterator_id = i.id
WHERE op.code = :product_code
GROUP BY i.id ORDER BY i.name;
```

## Show

Display iterator detail with all values and where it's referenced:

```sql
-- Iterator values
SELECT iv.value, iv.position FROM iterator_values iv
JOIN iterators i ON i.id = iv.iterator_id
WHERE i.name = :name AND i.product_id = :pid
ORDER BY iv.position;
```

To find references, search text fields in epics, features, requirements, and tests for the iterator name.

## Add Value

1. Get max position: `SELECT MAX(position) FROM iterator_values WHERE iterator_id = :id`
2. INSERT new value at position + 1
3. Show updated list

## Remove Value

1. DELETE from `iterator_values` WHERE iterator_id = :id AND value = :val
2. Reorder remaining positions sequentially
3. Show updated list

## Rename

1. UPDATE `iterators` SET name = :new_name WHERE id = :id
2. Warn: "Text references to the old name in existing entities will NOT auto-update. Search and replace manually if needed."

## Why Iterators Matter

1. **Single source of truth.** Change once, every reference updates.
2. **Explicit exclusions.** If `armv9` is NOT in `SOFTWARE_ARCHS`, that's deliberate.
3. **Test completeness.** "Test against each SOFTWARE_ARCHS" — defined, not implied.
4. **Requirement precision.** "Support each SUPPORTED_PROTOCOLS" — list grows, coverage follows.

## Display Rules

- Iterator names stored as-is in text fields: "test against each SOFTWARE_ARCHS"
- NEVER auto-expand inline — the reader sees the exact name
- Glossary at top of any output lists referenced iterators with current values

## Rules

1. Names must be UPPER_SNAKE_CASE.
2. Names are unique per product.
3. Never expand iterators in entity text. Always reference by name.
4. Warn on rename that text references need manual update.
