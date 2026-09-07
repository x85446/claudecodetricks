#!/usr/bin/env python3
"""Generate a markdown port table from a JSON port-map.

Usage:  python3 gen_port_table.py <portmap.json> [out.md]
        (omit out.md to print to stdout)

Every physical port gets its own row. Ports with no text or color set to spare
palette (#e2e8f0 / #f7fafc) are listed as "not connected".

Output columns: Port | Assignment | Connected Device | Status
"""
import json
import sys

SPARE_COLORS = {"#e2e8f0", "#f7fafc", ""}
SPARE_TEXTS  = {"spare", "unused", "25g sfp28 — unused", "100g — spare", ""}


def is_spare(p):
    color = p.get("color", "").lower()
    text  = p.get("text", "").strip().lower()
    return color in SPARE_COLORS and (not text or text in SPARE_TEXTS)


def assignment(p):
    color = p.get("color", "")
    text  = p.get("text", "").strip()
    color_map = {
        "#9f7aea": "712 DEV",
        "#ed64a6": "714 PROD",
        "#718096": "760 MAAS",
        "#2b6cb0": "758 MGMT2",
        "#1a202c": "iz_trunk",
        "#ecc94b": "network trunk",
        "#cbd5e0": "VLAN 1",
    }
    role = color_map.get(color, "")
    if not role and text:
        role = "—"
    return role or "spare"


def main():
    if len(sys.argv) < 2:
        sys.exit("usage: gen_port_table.py <portmap.json> [out.md]")
    pm = json.load(open(sys.argv[1]))
    ports = pm["ports"]
    out = open(sys.argv[2], "w") if len(sys.argv) >= 3 else sys.stdout

    title = pm.get("title", "Switch Port Table")
    out.write(f"## {title}\n\n")
    out.write("| Port | Assignment | Connected Device | Status |\n")
    out.write("|------|-----------|-----------------|--------|\n")

    for p in ports:
        port  = p.get("port", "")
        st    = p.get("status", "down") or "down"
        text  = p.get("text", "").strip()
        spare = is_spare(p)
        device  = "not connected" if spare else (text if text else "—")
        role    = "—" if spare else assignment(p)
        status  = st if not spare else "—"
        out.write(f"| `{port}` | {role} | {device} | {status} |\n")

    if out is not sys.stdout:
        out.close()
        print(f"wrote {sys.argv[2]} ({len(ports)} rows)")


if __name__ == "__main__":
    main()
