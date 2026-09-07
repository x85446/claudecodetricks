---
name: sw3-portpair
description: Configure server port-PAIRS on the Dell OS10 switch SW3 (USTXAR1-SW3, 10.7.158.25) as 50GbE LACP bonds on dev or production. Two adjacent 25G ports bond into one port-channel (2x25G=50G); dev=native760+tagged712, prod=native760+tagged714. Use when the user asks to put SW3 ports/pairs on dev or prod (e.g. "set 5+6 to prod", "po33 dev", "5:prod", "bond 37 and 39 production"). Wraps tools/dell-sw/sw3-portpair.sh.
---

# sw3-portpair — Dell SW3 server port-pair bonds

Wraps `tools/dell-sw/sw3-portpair.sh` to turn pairs of adjacent 25G ports on **SW3**
(`USTXAR1-SW3`, `10.7.158.25`, ssh alias `SW3`) into **50GbE LACP bonds** on dev or prod.

> Context: SW4's switch ASIC is dead, so SW3 runs standalone — these are plain local
> LACP port-channels (no VLT). The tool/README live in `tools/dell-sw/`.

## The model (and our shared shorthand)

- A **pair** = two adjacent ports `N` and `N+1` → one **port-channel `N`** → 2×25G = **50GbE**.
- The port-channel id is **the lower (start) port** of the pair.
- Canonical spec: **`<startport>:<role>`**, role = `dev` | `prod`.
- All of these mean the **same** thing → translate them to `N:role`:
  - `5:prod` · `5+6 prod` · `5-6 production` · `p5 prod` · `po5 prod` · "set 5 and 6 to prod"
- VLANs (always): **native/untagged 760** (MAAS/PXE) + **tagged 712 (dev)** or **714 (prod)**.
  Plus `lacp fallback` (node can PXE before its bond is up) and STP edge.

## How to run

1. **Parse** the user's request into one or more `N:role` specs (apply the shorthand table above).
   - If they name a pair by its second/even port (e.g. "6 prod"), normalize to the odd start (`5:prod`) and say so.
   - If role is ambiguous, default to what they said; never invent ports.

2. **Dry-run first — always.** Show the exact OS10 config; change nothing:
   ```bash
   tools/dell-sw/sw3-portpair.sh <spec> [<spec> ...]
   ```

3. **Confirm, then apply.** Note that ports 1–8 still carry the old `po1–8` (dead-VLT) teams,
   so applying those pairs **rebuilds** them. Once the user OKs:
   ```bash
   tools/dell-sw/sw3-portpair.sh --apply <spec> [<spec> ...]
   ```
   - `--apply` pushes over `ssh SW3` (key-based) and `write memory`s.
   - If `ssh SW3` isn't set up, it's `admin`/`admin` with legacy KEX — see `~/.ssh/config.d/df-austin`.

4. **Verify** after apply and report concisely:
   ```bash
   ssh SW3 "show port-channel summary"
   ssh SW3 "show running-configuration interface port-channel <N>"
   ```
   A bond shows `line protocol down` until the server runs LACP — with `lacp fallback`,
   the member goes **Individual** and forwards for PXE/commissioning. That's expected, not a failure.

## Rules

- **Never apply to the live switch without an explicit OK** — dry-run, show it, then ask.
- Pairs are adjacent `N`+`N+1`; warn (don't block) if a pair starts on an even port.
- Don't touch ports outside what the user asked for.
- This is SW3-only. SW4 is dead hardware; if asked about SW4, say so rather than configuring it.
- Keep the VLAN scheme fixed: dev→712, prod→714, native always 760. If the user wants a different
  VLAN, surface it — don't silently remap.

## Example

User: "put 1+2 and 3+4 on dev, 5+6 and 7+8 on prod"
→ specs: `1:dev 3:dev 5:prod 7:prod`
→ `tools/dell-sw/sw3-portpair.sh 1:dev 3:dev 5:prod 7:prod`  (dry-run, show it)
→ on OK: `tools/dell-sw/sw3-portpair.sh --apply 1:dev 3:dev 5:prod 7:prod`
→ `ssh SW3 "show port-channel summary"` and report po1/po3 (dev) + po5/po7 (prod) up/configured.
