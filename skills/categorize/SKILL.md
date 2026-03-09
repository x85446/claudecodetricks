---
name: categorize
description: Use when categorizing transactions, finding miscategorizations, auditing category hierarchy, fixing uncategorized entries, recovering #NAME? data, or managing category rules in the personaldb database.
argument-hint: [action] [scope]
disable-model-invocation: false
---

# Transaction Categorizer

Maintain, audit, and fix categorizations in the personaldb SQLite database. All changes are presented as recommendations for approval before applying.

If `$ARGUMENTS` is provided, interpret it as the action and scope (e.g., "audit personal", "uncategorized amex", "hierarchy cleanup", "recover").

## Database Location

```
db/personaldb.sqlite
```

Always use sqlite3 CLI via Bash. Enable foreign keys and column headers in every query:
```bash
sqlite3 db/personaldb.sqlite "PRAGMA foreign_keys=ON; .headers on; .mode column; <QUERY>"
```

## Critical Rule: Dual-Column Sync

The database stores categories in TWO forms that MUST stay in sync:
- **Text columns**: `category`, `sub_category`, `sub_sub` (human-readable, denormalized)
- **FK columns**: `tier1_id`, `tier2_id`, `tier3_id` (normalized references)

Every UPDATE must set BOTH forms. Use this pattern:
```sql
UPDATE transactions SET
  category = '<tier1_name>',  sub_category = '<tier2_name>',  sub_sub = '<tier3_name>',
  tier1_id = <id>,            tier2_id = <id>,                tier3_id = <id>,
  updated_at = datetime('now')
WHERE id = <txn_id>;
```

## Company Context

Transactions belong to either personal or a business entity:
- **Personal**: `company = '-'`, `company_id = 1`
- **Business**: `company = 'TMCTECH'|'GRAVHL'|'1913'|...` with corresponding `company_id`

The `company_category_rules` table defines which tier1 categories are valid per company. Always validate against it before recommending a category.

## Actions

### 1. `audit` — Find Inconsistencies

Scope: `personal`, `<company_code>`, or `all` (default: `personal`)

**Step 1: Query for conflicts**
Find items where the same description maps to different categories:
```sql
SELECT item, COUNT(DISTINCT category) as cat_count,
       GROUP_CONCAT(DISTINCT category) as categories,
       COUNT(*) as txn_count
FROM transactions
WHERE company = '-'  -- adjust for scope
  AND item != '#NAME?' AND item != ''
GROUP BY LOWER(TRIM(item))
HAVING COUNT(DISTINCT category) > 1
ORDER BY txn_count DESC
LIMIT 30;
```

**Step 2: For each conflict, show the breakdown**
```sql
SELECT item, category, sub_category, COUNT(*) as count
FROM transactions
WHERE LOWER(TRIM(item)) = LOWER(TRIM('<item>'))
GROUP BY category, sub_category
ORDER BY count DESC;
```

**Step 3: Present findings**
Format as a table showing:
- Item description
- Most common categorization (likely correct)
- Minority categorizations (likely wrong)
- Transaction count affected

**Step 4: Recommend fixes**
For each conflict, recommend aligning to the majority categorization. Present as:
```
RECOMMENDATION: Recategorize "<item>" → <majority_category> / <majority_subcategory>
  Affects: N transactions currently categorized as <minority_category>
  IDs: [list of transaction IDs to update]
```

Wait for approval before generating UPDATE statements.

### 2. `uncategorized` — Categorize Missing Entries

Scope: `personal`, `<company_code>`, `<source>` (e.g., `mint`, `amex`), or `all`

**Step 1: Survey uncategorized transactions**
```sql
SELECT site, company, COUNT(*) as count
FROM transactions
WHERE (tier1_id IS NULL OR category IS NULL OR category = '')
GROUP BY site, company
ORDER BY count DESC;
```

**Step 2: Group by similar descriptions**
```sql
SELECT LOWER(TRIM(item)) as normalized_item,
       COUNT(*) as count, site, company,
       GROUP_CONCAT(DISTINCT id) as ids
FROM transactions
WHERE (tier1_id IS NULL OR category IS NULL OR category = '')
  AND company = '-'  -- adjust for scope
GROUP BY normalized_item, site
ORDER BY count DESC
LIMIT 30;
```

**Step 3: Look for existing categorization patterns**
For each uncategorized group, check if the same item has been categorized elsewhere:
```sql
SELECT category, sub_category, sub_sub, COUNT(*) as count
FROM transactions
WHERE LOWER(TRIM(item)) = LOWER(TRIM('<item>'))
  AND tier1_id IS NOT NULL
GROUP BY category, sub_category, sub_sub
ORDER BY count DESC;
```

**Step 4: Check source-table metadata**
Some sources have their own categories that can inform the decision:
- `src_amex.amex_category`
- `src_chase.chase_category`
- `src_mint.mint_category`
- `src_amazon.amazon_category`

```sql
SELECT s.amex_category, COUNT(*) as count
FROM src_amex s
JOIN transactions t ON s.transaction_id = t.id
WHERE t.tier1_id IS NULL
GROUP BY s.amex_category
ORDER BY count DESC;
```

**Step 5: Recommend categories**
For each group, recommend a category based on:
1. Existing patterns for the same item (highest priority)
2. Source-specific category mapping
3. Description keyword matching against the category hierarchy
4. Company-category rules validation

Present recommendations grouped by confidence:
- **High confidence**: Same item categorized consistently elsewhere
- **Medium confidence**: Source category or keyword match
- **Low confidence**: Best guess, needs human review

Wait for approval before applying.

### 3. `hierarchy` — Audit Category Structure

**Step 1: Find orphaned categories (no transactions)**
```sql
SELECT t1.id, t1.name as tier1, 'tier1' as level, 0 as txn_count
FROM categories_tier1 t1
LEFT JOIN transactions tx ON tx.tier1_id = t1.id
WHERE tx.id IS NULL
UNION ALL
SELECT t2.id, t2.name, 'tier2', 0
FROM categories_tier2 t2
LEFT JOIN transactions tx ON tx.tier2_id = t2.id
WHERE tx.id IS NULL
UNION ALL
SELECT t3.id, t3.name, 'tier3', 0
FROM categories_tier3 t3
LEFT JOIN transactions tx ON tx.tier3_id = t3.id
WHERE tx.id IS NULL;
```

**Step 2: Find near-duplicate categories**
Look for categories with similar names (potential merges):
```sql
SELECT a.name as cat_a, b.name as cat_b, a.id as id_a, b.id as id_b
FROM categories_tier2 a, categories_tier2 b
WHERE a.id < b.id
  AND (LOWER(a.name) = LOWER(b.name)
    OR a.name LIKE '%' || b.name || '%'
    OR b.name LIKE '%' || a.name || '%');
```

**Step 3: Find hierarchy mismatches**
Categories that seem personal but are assigned to business, or vice versa:
```sql
SELECT c.code as company, t1.name as tier1,
       COUNT(*) as txn_count
FROM transactions tx
JOIN companies c ON tx.company_id = c.id
JOIN categories_tier1 t1 ON tx.tier1_id = t1.id
LEFT JOIN company_category_rules ccr
  ON ccr.company_id = c.id AND ccr.tier1_id = t1.id
WHERE ccr.id IS NULL
GROUP BY c.code, t1.name
ORDER BY txn_count DESC;
```

**Step 4: Show category usage stats**
```sql
SELECT t1.name as tier1,
       COUNT(DISTINCT t2.id) as tier2_count,
       COUNT(DISTINCT t3.id) as tier3_count,
       COUNT(tx.id) as txn_count
FROM categories_tier1 t1
LEFT JOIN categories_tier2 t2 ON t2.tier1_id = t1.id
LEFT JOIN categories_tier3 t3 ON t3.tier2_id = t2.id
LEFT JOIN transactions tx ON tx.tier1_id = t1.id
GROUP BY t1.id, t1.name
ORDER BY txn_count DESC;
```

**Step 5: Present findings and recommendations**
Group proposals as:
- **Merge**: Two categories that should be one (present which to keep)
- **Rename**: Category name is unclear or inconsistent
- **Move**: Category is under the wrong parent
- **Delete**: Orphaned category with no transactions
- **Add rule**: Missing company_category_rules entry

Wait for approval. For merges, generate both the category UPDATE and the transaction re-mapping.

### 4. `recover` — Fix #NAME? Entries

**Step 1: Count and survey**
```sql
SELECT site, company, COUNT(*) as count
FROM transactions
WHERE item = '#NAME?'
GROUP BY site, company;
```

**Step 2: Attempt recovery from source tables**
Try to replace #NAME? with real descriptions from source-specific tables:
```sql
-- For amex source
SELECT t.id, t.item, s.description as recovered_item,
       s.amex_category as source_category
FROM transactions t
JOIN src_amex s ON s.transaction_id = t.id
WHERE t.item = '#NAME?';

-- For chase source
SELECT t.id, t.item, s.description as recovered_item,
       s.chase_category as source_category
FROM transactions t
JOIN src_chase s ON s.transaction_id = t.id
WHERE t.item = '#NAME?';

-- For amazon source
SELECT t.id, t.item, s.title as recovered_item,
       s.amazon_category as source_category
FROM transactions t
JOIN src_amazon s ON s.transaction_id = t.id
WHERE t.item = '#NAME?';

-- For mint source
SELECT t.id, t.item, s.description as recovered_item,
       s.original_description, s.mint_category as source_category
FROM transactions t
JOIN src_mint s ON s.transaction_id = t.id
WHERE t.item = '#NAME?';
```

Repeat for all source tables that have a description/title column.

**Step 3: Present recovery results**
Group into:
- **Recoverable**: Source table has a real description (show it)
- **Unrecoverable**: Source table also has no useful description

For recoverable entries:
```
RECOMMENDATION: Recover item description
  Transaction ID: <id>
  Current: #NAME?
  Recovered: "<description from source>"
  Source category hint: <source_category>
```

**Step 4: After recovery, re-run categorization**
Once item descriptions are restored, the recovered transactions become candidates for `uncategorized` action (since many likely have broken categories too).

Wait for approval before applying UPDATEs.

### 5. `rules` — Manage Category Rules

**Step 1: Show current rules**
```sql
SELECT c.code as company, t1.name as allowed_category
FROM company_category_rules ccr
JOIN companies c ON ccr.company_id = c.id
JOIN categories_tier1 t1 ON ccr.tier1_id = t1.id
ORDER BY c.code, t1.name;
```

**Step 2: Find transactions violating rules**
```sql
SELECT t.id, c.code as company, t1.name as category, t.item
FROM transactions t
JOIN companies c ON t.company_id = c.id
JOIN categories_tier1 t1 ON t.tier1_id = t1.id
LEFT JOIN company_category_rules ccr
  ON ccr.company_id = c.id AND ccr.tier1_id = t1.id
WHERE ccr.id IS NULL AND t.company_id IS NOT NULL
ORDER BY c.code, t1.name;
```

**Step 3: Recommend either**
- Add a new rule (if the category is legitimately used by that company)
- Recategorize the transactions (if they were misassigned)

Present both options and wait for the user to choose.

## Presentation Format

Always present recommendations in this format:

```
## Findings: [Action] [Scope]

### Summary
- Transactions analyzed: N
- Issues found: N
- Estimated fixes: N

### Recommendations

#### Group 1: [Description]
| ID | Item | Current Category | Recommended Category | Confidence |
|----|------|-----------------|---------------------|------------|
| ...| ...  | ...             | ...                 | High/Med/Low|

**Apply this group?** (yes/no/skip)
```

After approval, generate and execute the UPDATE statements, then show a summary of changes made.

## Guardrails

- **Never delete transactions** — only update categorization columns
- **Never modify source tables** (src_amex, src_amazon, etc.) — they are immutable import records
- **Always validate** tier1_id against company_category_rules before recommending
- **Always update both** text columns AND FK columns in the same UPDATE
- **Back up before bulk changes**: suggest `cp db/personaldb.sqlite db/personaldb.sqlite.bak` before applying >50 changes
- **FIXME category** (tier1): Use this as a temporary marker when no clear category exists, never as a final answer
- **Log changes**: After applying, query the updated rows to confirm the changes took effect

## Quick Reference: Category Lookup

To find the correct FK IDs for a category path:
```sql
SELECT t1.id as t1_id, t1.name as tier1,
       t2.id as t2_id, t2.name as tier2,
       t3.id as t3_id, t3.name as tier3
FROM categories_tier1 t1
LEFT JOIN categories_tier2 t2 ON t2.tier1_id = t1.id
LEFT JOIN categories_tier3 t3 ON t3.tier2_id = t2.id
WHERE t1.name LIKE '%<search>%'
   OR t2.name LIKE '%<search>%'
   OR t3.name LIKE '%<search>%';
```

## See Also

- `schema.sql` — Full database schema definition
- `import_db.py` — Import script with category mapping logic
- Views: `v_uncategorized`, `v_valid_categories`, `v_spending_by_category`
