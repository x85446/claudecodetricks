#!/usr/bin/env python3
"""Generate a horizontal faceplate-style rack-equipment SVG from a JSON port-map.

Usage:  python3 gen_rack_svg.py <portmap.json> <out.svg>

Port-map JSON schema:
{
  "title":    "USTXAR1-SMAIN-(cisco) ...",
  "subtitle": "Catalyst 3650 · 10.7.158.2",   # optional
  "group":    12,                               # ports per visual group (default 12)
  "legend":   [["#2b6cb0","758 MGMT2"], ...],  # optional colour key
  "ports": [
    {"port":"Gi1/0/1", "color":"#2b6cb0", "text":"Cruz - iLO", "status":"up"},
    ...
  ]
}

Layout: horizontal faceplate. Ports laid out left-to-right in pairs (top=odd, bottom=even).
Groups of N ports separated by dashed vertical lines. Title+subtitle at top. Legend at right.
"""
import json
import sys
import math


def esc(s):
    return (s or "").replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: gen_rack_svg.py <portmap.json> <out.svg>")
    pm = json.load(open(sys.argv[1]))
    ports = pm["ports"]
    legend = pm.get("legend", [])
    group = pm.get("group", 12)

    PORT_W = 28    # port block width
    PORT_H = 24    # port block height
    PAD_L  = 16    # left padding before first port
    PAD_TOP = 52   # space for title + subtitle
    GAP     = 6    # gap between groups
    PORT_GAP = 2   # gap between port columns within a group

    # Calculate layout: pair columns, with group separators
    n_cols = math.ceil(len(ports) / 2)  # number of column positions
    n_groups = math.ceil(n_cols / group)
    # total width = PAD_L + n_cols * (PORT_W + PORT_GAP) + (n_groups-1)*GAP + legend + PAD_L
    LEGEND_W = 180 if legend else 0
    total_cols_w = n_cols * PORT_W + max(0, n_cols - 1) * PORT_GAP + max(0, n_groups - 1) * GAP
    W = PAD_L + total_cols_w + LEGEND_W + PAD_L
    H = PAD_TOP + PORT_H * 2 + 20 + 30  # header + two port rows + labels + legend rows

    legend_rows = math.ceil(len(legend) / 2) if legend else 0
    legend_h = legend_rows * 20 + 10
    H = max(H, PAD_TOP + PORT_H * 2 + 30 + legend_h)

    s = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" '
         f'viewBox="0 0 {W} {H}" font-family="Helvetica,Arial">']
    s.append(f'<rect x="0" y="0" width="{W}" height="{H}" fill="#f7fafc"/>')
    s.append(f'<text x="{PAD_L}" y="22" font-size="15" font-weight="bold">{esc(pm.get("title",""))}</text>')
    if pm.get("subtitle"):
        s.append(f'<text x="{PAD_L}" y="40" font-size="10" fill="#555">{esc(pm["subtitle"])}</text>')

    # Build port positions: pair columns, top=odd index, bottom=even index
    # ports[0]=col0-top, ports[1]=col0-bottom, ports[2]=col1-top, ports[3]=col1-bottom ...
    col = 0
    x = PAD_L
    for i in range(0, len(ports), 2):
        # group separator (dashed vertical line before each group except the first)
        if col > 0 and col % group == 0:
            line_x = x - GAP // 2
            s.append(f'<line x1="{line_x}" y1="{PAD_TOP}" x2="{line_x}" y2="{PAD_TOP + PORT_H*2 + 4}" '
                     f'stroke="#a0aec0" stroke-width="1" stroke-dasharray="3,2"/>')
            x += GAP - PORT_GAP  # already added PORT_GAP below

        top_port = ports[i]
        bot_port = ports[i+1] if i+1 < len(ports) else None

        # top port (odd)
        tc = top_port.get("color", "#e2e8f0")
        num_top = top_port["port"].split("/")[-1] if "/" in top_port["port"] else top_port["port"]
        text_col = "#ffffff" if tc not in ("#e2e8f0", "#cbd5e0", "#f7fafc") else "#1a202c"
        s.append(f'<rect x="{x}" y="{PAD_TOP}" width="{PORT_W}" height="{PORT_H}" '
                 f'rx="2" fill="{tc}" stroke="#444" stroke-width="0.7"/>')
        s.append(f'<text x="{x + PORT_W//2}" y="{PAD_TOP + 15}" font-size="10" '
                 f'text-anchor="middle" fill="{text_col}" font-family="monospace">{esc(num_top)}</text>')

        # tooltip title for top port
        if top_port.get("text"):
            s.append(f'<title>{esc(top_port["port"])}: {esc(top_port.get("text",""))}</title>')

        # bottom port (even)
        if bot_port:
            bc = bot_port.get("color", "#e2e8f0")
            num_bot = bot_port["port"].split("/")[-1] if "/" in bot_port["port"] else bot_port["port"]
            text_col_b = "#ffffff" if bc not in ("#e2e8f0", "#cbd5e0", "#f7fafc") else "#1a202c"
            s.append(f'<rect x="{x}" y="{PAD_TOP + PORT_H + 2}" width="{PORT_W}" height="{PORT_H}" '
                     f'rx="2" fill="{bc}" stroke="#444" stroke-width="0.7"/>')
            s.append(f'<text x="{x + PORT_W//2}" y="{PAD_TOP + PORT_H + 2 + 15}" font-size="10" '
                     f'text-anchor="middle" fill="{text_col_b}" font-family="monospace">{esc(num_bot)}</text>')

        x += PORT_W + PORT_GAP
        col += 1

    # Group range labels below the ports
    label_y = PAD_TOP + PORT_H * 2 + 16
    col = 0
    x = PAD_L
    for i in range(0, len(ports), 2):
        if col > 0 and col % group == 0:
            x += GAP - PORT_GAP
        x += PORT_W + PORT_GAP
        col += 1
    # Draw group labels
    col = 0
    gx = PAD_L
    for g in range(n_groups):
        start_col = g * group
        end_col = min(start_col + group, n_cols)
        # pixel x of start of this group
        lx = PAD_L + start_col * (PORT_W + PORT_GAP) + g * GAP
        first_port_num = ports[start_col * 2]["port"].split("/")[-1] if start_col * 2 < len(ports) else ""
        last_idx = min((end_col - 1) * 2, len(ports) - 1)
        last_port_num = ports[last_idx]["port"].split("/")[-1] if last_idx < len(ports) else ""
        s.append(f'<text x="{lx}" y="{label_y}" font-size="9" fill="#666">{esc(first_port_num)}–{esc(last_port_num)}</text>')

    # Legend box
    if legend:
        leg_x = W - LEGEND_W + 8
        leg_y = PAD_TOP
        s.append(f'<rect x="{leg_x - 4}" y="{leg_y - 4}" width="{LEGEND_W - 4}" height="{legend_h + 8}" '
                 f'rx="4" fill="none" stroke="#888" stroke-dasharray="4,3"/>')
        for i, (hexc, lbl) in enumerate(legend):
            row = i // 2
            col_pos = i % 2
            lx = leg_x + col_pos * 86
            ly = leg_y + row * 20
            s.append(f'<rect x="{lx}" y="{ly}" width="12" height="12" rx="2" fill="{hexc}" '
                     f'stroke="#888" stroke-width="0.5"/>')
            s.append(f'<text x="{lx + 16}" y="{ly + 10}" font-size="9">{esc(lbl)}</text>')

    s.append('</svg>')
    open(sys.argv[2], "w").write("\n".join(s))
    print(f"wrote {sys.argv[2]} ({len(ports)} ports, {W}x{H})")


if __name__ == "__main__":
    main()
