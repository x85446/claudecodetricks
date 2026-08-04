---
name: pm-preflight
description: Validate environment before any PM skill runs. Checks sqlite3, database, schema. Hard stop on failure — no fallbacks.
argument-hint: (no arguments — runs automatically)
---

# PM Preflight — Dependency Guardian

Runs before ANY other PM skill. Validates the environment is ready. If it fails, STOP. No fallbacks. No workarounds.

## Invocation

```
/pm-preflight
```

Called automatically by `/pm` before dispatching. Can also be run standalone to verify setup.

## Check 1: sqlite3 Available

```bash
which sqlite3 || python3 -c "import sqlite3; print('python3-sqlite3')"
```

**If neither works:**
```
STOP: sqlite3 is not available on this system.

Install it:
  macOS:    brew install sqlite3
  Ubuntu:   sudo apt-get install sqlite3
  Alpine:   apk add sqlite
  Python:   sqlite3 module is built-in — use python3 instead of sqlite3 CLI

Cannot proceed without SQLite. PM skills MUST NOT write directly to markdown,
JSON, CSV, or any other format as a workaround.
```

Present this message to the user and STOP. Do not attempt alternative approaches.

## Check 2: Database Directory

```bash
ls .claude/db/ 2>/dev/null || mkdir -p .claude/db
```

## Check 3: Schema Initialized

```bash
sqlite3 .claude/db/marketing.sqlite ".tables" 2>/dev/null | grep -q epics
```

If `epics` table is missing, run the PM schema migration:

1. Check if competitive-intel base tables exist (`our_products`, `tags`). If not, warn user to initialize competitive-intel first.
2. Check if feature-tracker tables exist (`product_features`). If not, run feature-tracker schema.sql first.
3. Run [schema.sql](schema.sql) to add PM tables (epics, requirements, iterators, etc.)
4. Handle ALTER TABLE for existing tables — add columns if missing:

```sql
-- Check and add columns to product_features
SELECT COUNT(*) FROM pragma_table_info('product_features') WHERE name = 'epic_id';
-- If 0: ALTER TABLE product_features ADD COLUMN epic_id TEXT REFERENCES epics(id);

SELECT COUNT(*) FROM pragma_table_info('product_features') WHERE name = 'base_version';
-- If 0: ALTER TABLE product_features ADD COLUMN base_version INTEGER NOT NULL DEFAULT 1;

-- Check and add columns to product_feature_tests
SELECT COUNT(*) FROM pragma_table_info('product_feature_tests') WHERE name = 'requirement_id';
-- If 0: ALTER TABLE product_feature_tests ADD COLUMN requirement_id TEXT REFERENCES requirements(id);

SELECT COUNT(*) FROM pragma_table_info('product_feature_tests') WHERE name = 'base_version';
-- If 0: ALTER TABLE product_feature_tests ADD COLUMN base_version INTEGER NOT NULL DEFAULT 1;
```

## Check 4: Schema Version

```sql
SELECT version FROM schema_versions WHERE skill = 'pm';
```

Compare against expected version. If mismatch, run migration steps for the gap.

## Output on Success

```
PM Preflight: OK
  sqlite3: /usr/bin/sqlite3
  database: .claude/db/marketing.sqlite
  schema: pm v1
  tables: epics, requirements, iterators, iterator_values (+ existing)
```

## Rules

1. **Hard stop on any failure.** No fallbacks. No markdown workarounds.
2. **Only skill that runs DDL.** All CREATE TABLE and ALTER TABLE happen here.
3. **Idempotent.** Safe to run multiple times — uses IF NOT EXISTS and column checks.
4. **Never deletes data.** Only adds tables and columns.
