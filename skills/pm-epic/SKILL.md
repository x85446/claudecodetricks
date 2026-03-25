---
name: pm-epic
description: Create and manage epics (big-picture product capabilities). Crawl sources or interview humans to identify epics. Each epic decomposes into features.
argument-hint: [product crawl <path>|add|list|show <uuid>|edit <uuid>]
---

# PM Epic — Big-Picture Capabilities

Identify and write epics — large product capabilities that decompose into multiple features.

## Invocation

```
/pm-epic myriplay crawl ./src/
/pm-epic myriplay crawl ./docs/architecture.md
/pm-epic myriplay add
/pm-epic myriplay list
/pm-epic myriplay show <uuid>
/pm-epic myriplay edit <uuid>
```

## Database

**Path:** `.claude/db/marketing.sqlite`

Always run `PRAGMA foreign_keys=ON;` before writes.

**UUID generation:**
```sql
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' ||
  substr(hex(randomblob(2)),2) || '-' ||
  substr('89ab',abs(random()) % 4 + 1, 1) ||
  substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6)));
```

## Crawl Mode

1. Read source material (code, docs, URLs)
2. Identify top-level product capabilities — each one is an epic
3. For each epic write: name, description (2-3 sentences), rationale (why it matters)
4. Tag with high-level categories (ensure tags exist in `tags` table)
5. INSERT into `epics` with `human_approved = 0`, `source = 'docs:<path>'` or `'code:<path>'`
6. INSERT version 1 snapshot into `epic_versions`
7. INSERT tag associations into `epic_tags`
8. Show summary of discovered epics

## Interact Mode (h/ai iterate)

1. Ask: "What's the big-picture capability?"
2. Human gives rough description
3. AI structures into: name (short), description (2-3 sentences), rationale
4. Present for approval. Accept: y/yes/ok/good
5. Ask for tags or propose them
6. INSERT with `human_approved = 1`, `source = 'interview'`
7. INSERT version 1 snapshot into `epic_versions`
8. Show UUID: `Created epic: <uuid> "name"`

## Edit

1. Show current epic detail
2. Ask what to change
3. Run h/ai iterate on changed fields
4. Auto-increment `version` on `epics`
5. INSERT new snapshot into `epic_versions`
6. `base_version` stays as-is (epics are root — no parent)
7. Show updated UUID and version

## Queries

```sql
-- List epics for a product
SELECT e.id, e.name, e.version, e.status, e.human_approved,
  GROUP_CONCAT(t.name, ', ') AS tags
FROM epics e
JOIN our_products op ON op.id = e.product_id
LEFT JOIN epic_tags et ON et.epic_id = e.id
LEFT JOIN tags t ON t.id = et.tag_id
WHERE op.code = :product_code
GROUP BY e.id ORDER BY e.name;

-- Show epic detail
SELECT e.*, GROUP_CONCAT(t.name, ', ') AS tags
FROM epics e
LEFT JOIN epic_tags et ON et.epic_id = e.id
LEFT JOIN tags t ON t.id = et.tag_id
WHERE e.id = :uuid GROUP BY e.id;
```

## Rules

1. Every epic needs: name, description, rationale, at least one tag.
2. Crawl → `human_approved = 0`. Interact → `human_approved = 1`.
3. Every edit creates a new version snapshot. Versions are append-only.
4. Always show UUID after create/edit.
