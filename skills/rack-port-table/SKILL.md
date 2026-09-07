---
name: rack-port-table
description: Generate a markdown port table from a switch portmap JSON. Every port gets its own row; unconnected/spare ports say "not connected". Use when documenting switch port assignments as a readable table alongside or instead of an SVG faceplate.
---

# rack-port-table — markdown port tables for rack gear

Generates a **one-row-per-port** markdown table from the same portmap JSON used by the
`rack-svg` skill. Every physical port is listed — spare/unused ports say "not connected".

## How to use

1. Use the same portmap JSON as `rack-svg` (lives at `infrastructure/docs/diagrams/<switch>.portmap.json`).
2. Generate:
   ```bash
   python3 .claude/skills/rack-port-table/gen_port_table.py \
     infrastructure/docs/diagrams/<switch>.portmap.json \
     infrastructure/docs/diagrams/<switch>.porttable.md
   ```
3. Embed in `network.md`:
   ```markdown
   <!-- include infrastructure/docs/diagrams/ustxar1-sw3.porttable.md -->
   ```
   or just copy-paste the table block.

## Output format

```
## USTXAR1-SW3-(dell) · S5148F-ON · 10.7.158.25

| Port        | Assignment | Connected Device                        | Status |
|-------------|------------|-----------------------------------------|--------|
| `Eth1/1/1`  | 712 DEV    | dev-po1 pair 1/1/1+1/1/2 2x25G=50G     | down   |
| `Eth1/1/2`  | 712 DEV    | dev-po1 pair 1/1/1+1/1/2 2x25G=50G     | down   |
| `Eth1/1/21` | —          | not connected                           | —      |
...
```

## Notes
- Spare ports (color `#e2e8f0` / `#f7fafc`, text empty or "unused") → "not connected".
- The portmap JSON is the single source of truth — update the JSON, re-run both scripts.
- Pairs: each port in a bond pair gets its own row (both show the bond description).
