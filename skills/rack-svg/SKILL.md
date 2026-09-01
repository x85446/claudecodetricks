---
name: rack-svg
description: Generate clean, color-coded horizontal faceplate SVG diagrams for rack network equipment (switches) from a simple JSON port-map. Ports are laid out left-to-right in pairs (top=odd, bottom=even), grouped in segments of 12, colored by network segment. Use when documenting a switch's port assignments in infrastructure/docs.
---

# rack-svg — faceplate SVGs for rack gear

Generates a **horizontal faceplate** SVG for a switch — ports laid out side-by-side in pairs
(top row = odd ports, bottom row = even ports), color-coded by network segment, groups of 12
separated by dashed lines. Compact and readable at a glance. SVG is referenced from markdown
as a file (`![](diagrams/foo.svg)`) — GitHub strips inline `<svg>`, so always write a `.svg` file.

For a per-port text table use the companion `rack-port-table` skill.

## How to use

1. Build a port-map JSON (one entry per port, in panel order). Schema:
   ```json
   {
     "title": "USTXAR1-SMAIN-(cisco) · Catalyst 3650 · 10.7.158.2",
     "subtitle": "one row per port · ssh cisco",
     "width": 520,
     "legend": [["#2b6cb0","758 MGMT2"], ["#1a202c","iz_trunk"], ["#ecc94b","network trunk"]],
     "ports": [
       {"port":"Gi1/0/1", "color":"#2b6cb0", "text":"Cruz — iLO", "status":"up"},
       {"port":"Gi1/0/2", "color":"#2b6cb0", "text":"MANDNKS2 — P1", "status":"up"}
     ]
   }
   ```
2. Generate:
   ```bash
   python3 .claude/skills/rack-svg/gen_rack_svg.py <portmap.json> infrastructure/docs/diagrams/<switch>.svg
   ```
3. Validate it's well-formed and (optionally) render:
   ```bash
   python3 -c "import xml.dom.minidom as m; m.parse('infrastructure/docs/diagrams/<switch>.svg')"
   rsvg-convert infrastructure/docs/diagrams/<switch>.svg -o /tmp/x.png   # if rsvg-convert present
   ```
4. Reference it in `network.md`: `![<switch> port map](diagrams/<switch>.svg)`.

## House color convention (Izuma)

| Segment | Color | |
|---|---|---|
| 758 MGMT2 / iLO | `#2b6cb0` | blue |
| iz_trunk (inter-switch / server trunk) | `#1a202c` | black |
| network trunk (to router/WAN) | `#ecc94b` | yellow |
| 712 dev | `#9f7aea` | purple |
| 714 prod | `#ed64a6` | pink |
| 760 MAAS | `#718096` | grey-blue |
| VLAN 1 / "38 net" | `#cbd5e0` | light grey |
| spare / unused | `#e2e8f0` | pale grey |

## Notes
- `status: "up"` → green tag at right; `"down"`/omitted → grey. Down rows render in muted text.
- Keep `text` short — `<machine> — <role>` (e.g. `Cruz — P1`, decoded from `P1_Cruz`).
- Port-map JSONs live alongside the diagrams (e.g. `infrastructure/docs/diagrams/<switch>.portmap.json`) so they're easy to regenerate after a re-cable.
