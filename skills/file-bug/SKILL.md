---
name: file-bug
description: Write a new slug JSON to the leaf's to-chopper/ directory, optionally linking to a parent bug or a blocked-by entry. Also writes the private validation markdown to tests/bugs/crossbugs/<slug>.md in the target repo.
when_to_use: When the agent has identified a new bug that belongs in the system. Do NOT use to update an existing bug.
---

# file-bug

File a new slug to chopper2's intake.

## Inputs

- `--title <text>` (required) — short bug title
- `--description <text>` (required) — what's broken (what was seen vs expected)
- `--validation <text>` (required) — how to verify a fix
- `--repo <name>` (required) — target repo
- `--priority <p0|p1|p2|p3>` (optional)
- `--parent <BUG-NNNNNN>` (optional) — parent bug for cross-repo children
- `--blocked-by <BUG-NNNNNN>` (repeatable, optional)
- `--dry-run` — log intended ops, write nothing

## Behavior

Generates a slug ID (sha256 prefix of the title), composes a slug JSON record matching `global/schemas/slug.schema.json`, and writes it to `<cwd>/to-chopper/<slug>.json`.

When `--parent` or `--blocked-by` is supplied, those fields are populated on the slug.

Also writes the private validation markdown (`<slug>.md`) to `/opt/repos/<repo>/tests/bugs/crossbugs/` so the tester can run it later (§17 cross-bug validation).

## GHLSTATE transition

`(none) → filed` (slug stage only). Bug acceptance happens in chopper2.

## Scenarios

S2 (leaf-direct filing), S15 (cross-repo child spawning).
