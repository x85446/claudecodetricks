# DARPA Equipment Explanation Context

Use this context when writing the `Explanation:` paragraph for a new equipment
item in the Izuma Networks DARPA proposal Equipment List.

## Project framing

- Sponsor: DARPA (proposal — Phase D2 / 26BZ R2 family of solicitations).
- Performer: Izuma Networks.
- The product under test is **Izuma Edge** — an edge / device-management
  platform Izuma deploys on constrained and industrial compute.
- The equipment in this list is purchased to **validate** Izuma Edge in
  representative deployment environments: vehicle networks (CAN/CAN-FD,
  J1939, automotive Ethernet), industrial control (PLC, Modbus, BACnet),
  edge compute hosts (Raspberry Pi, x86 mini-PCs, SMARC SOMs), and security
  components (TPMs, cellular routers, legacy/EOL hosts).
- Every item exists to make some validation or interoperability scenario
  possible on Izuma's test bench.

## Tone & format for the Explanation paragraph

- One short paragraph (1–3 sentences).
- Active voice. No bullet points.
- Always anchor on how the item supports Izuma Edge testing or
  validation — *don't* describe the product generically.
- Mention the specific class of network / hardware / scenario it enables
  (e.g. "automotive Ethernet networks", "BACnet MS/TP", "compact M.2 edge
  systems", "low-memory edge hosts").
- Avoid marketing words ("powerful", "industry-leading"). Keep it sober and
  proposal-grade.

## Style template

> Used to <enable / interface with / simulate / connect to> <specific
> scenario or network class>, supporting <validation goal> of Izuma Edge in
> <environment>.

## Worked examples (from existing list — use as voice reference)

- **Kvaser USBcan Pro 2xHS v2:** "Used to connect Izuma Edge to vehicle
  CAN/CAN-FD networks for collecting telemetry and validating remote
  monitoring and update functionality on representative vehicle systems.
  The device provides dual-channel CAN connectivity over USB for direct
  integration with test platforms."
- **Raspberry Pi 5 (8GB):** "Serves as a representative edge compute
  platform for deploying Izuma Edge in constrained environments, supporting
  evaluation of performance under limited compute and memory conditions."
- **AutomationDirect CLICK PLUS PLC:** "Represents an industrial control
  system used for testing integration with Modbus and MQTT protocols
  commonly used in industrial environments."

## What NOT to write

- Don't reference the proposal number, dollar amount, or budget line.
- Don't say "we will purchase" or "the team plans to buy" — the list is
  factual.
- Don't repeat the product name verbatim in the explanation; the heading
  and `Product:` line already carry that.
