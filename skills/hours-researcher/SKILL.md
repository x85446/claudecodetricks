---
name: hours-researcher
description: Use when someone asks to research/gather their real hours, build the activity truth base, classify work vs personal, figure out which work is NEDO-eligible, or produce a month.md of grouped hour-block activity from Claude sessions and browser history. The data-gathering phase that feeds hours-maker.
argument-hint: <YYYY-MM> [--end YYYY-MM-DD] [--cap-validate]
disable-model-invocation: true
allowed-tools: Bash, Read, AskUserQuestion, ToolSearch, mcp__google-sheets__get_sheet_data, mcp__google-sheets__get_sheet_formulas, mcp__claude_ai_Microsoft_365__outlook_calendar_search
---

## What This Skill Does

The **research / data-gathering phase** for Softbank/NEDO hours reporting. It builds
a granular, *truthful* activity base from real evidence — **it does not write to the
Hours Maker sheet**. Its output (`<month>.md`) is the **input that `/hours-maker`
consumes** to place hours into the form.

```
  hours-researcher  →  temp/<month>.md (truth base)  →  hours-maker  →  Google Sheet
  (gather + classify, read-only)                        (place, write)
```

It exists because earlier months were built partly by hand and drifted toward
synthesis. This skill produces the base **only from real timestamps**: your Claude
Code session transcripts (~80–90% of how you work) and Chrome/Firefox history.

For each real 30-min slot it classifies the activity via a **user-owned map**
(`classification_map.json`):
- **work vs personal**
- for work: **company** (izuma / gravhl / tmctech / …)
- for work: **NEDO-eligible?** — NEDO = k8s work, machine bringup, cluster
  bringup/maintenance, k8s component work (cilium, longhorn, CAPI, ArgoCD, incus…),
  *regardless of company*.

## How the classifier reads activity (the map has 4 sections)

`classification_map.json` is matched in this order for each event:
1. **`projects`** — substring of the Claude-session cwd. The label shows a
   workspace-relative path (e.g. `claude:izuma/wrangler/scripts`), not a bare
   basename, so `projects`/`tmp` etc. are disambiguated.
2. **`drop`** — browser hosts that are pure noise (youtube, x.com, accounts.google,
   remotedesktop…). Discarded entirely; only tallied as "dropped N".
3. **search queries** — for google/bing/ddg, the `?q=` is extracted and run through
   `keywords`. So "search: kubernetes longhorn" → NEDO; "search: blood sugar" →
   personal/health. The actual query is shown as the evidence.
4. **`domains`** — host substring. For title-bearing hosts (chatgpt, docs.google,
   meet.google, sharepoint, github) the page **title** is shown — the conversation /
   doc / meeting name. (ChatGPT message *bodies* live on OpenAI's servers and can't be
   pulled; the tab title is what's available and is usually descriptive.)
5. **`keywords`** — ordered regex rules applied to page title + URL path when domains
   miss, so unmapped pages classify by what they're *about* ("see what I was
   researching from the url + content"). First match wins.
Anything still unmatched → UNMAPPED, listed with sample titles/queries to classify next.

## Integrity rules (hard — this skill's whole reason to exist)

1. **Pure analysis of real timestamps.** Nothing is synthesized, normalized, or placed. No RNG.
2. **UNMAPPED is never guessed.** Any project/domain not in the map is reported in an UNMAPPED section for the user to classify. Non-Izuma / personal work never silently becomes NEDO hours.
3. **Daily totals never double-count.** Each 30-min slot is attributed to ONE dominant activity for the rollup; the per-block listing still shows ALL concurrent activities (the real, multi-session picture).
4. **The map is the judgement, and it's the user's.** The skill computes; the human owns `classification_map.json`. Review it before trusting totals.

## Workflow

### Step 1 — Confirm/extend the classification map
Open `classification_map.json`. Run Step 2 once; if the UNMAPPED section is large or
contains anything that should count, ask the user how to classify it and add entries.
Re-run. Repeat until UNMAPPED is only genuinely-irrelevant noise.

### Step 2 — Generate the truth base
From this skill's directory:
```
python3 classify.py --month <YYYY-MM> [--end <YYYY-MM-DD>] \
        --map classification_map.json --browser --tz <-5 CDT | -6 CST> \
        --out <project>/temp/<month>.md
```
- `--end` caps at "today" for the current month (don't fabricate future days).
- `--tz`: CDT (−5) Mar–Nov, CST (−6) Nov–Mar. (All sources store UTC; verified.)

Output `<month>.md` per the format below — hour blocks, daily summary, evidence counts.

### Step 3 — Validate the shared calendar tabs (read-only)
The joint/meeting area of each week tab is calendar-driven. For tabs that already
exist (e.g. Ed created them), **validate, don't recreate**:
- For each week tab in the month, read its joint area `B101:H148` (via
  `mcp__google-sheets__get_sheet_data`) and compare against Outlook
  (`mcp__claude_ai_Microsoft_365__outlook_calendar_search`, queries "Myriplane" and
  "Softbank", week window in Central). This is the **validate** sub-action already
  specified in `../hours-maker/validate.md` and `../hours-maker/shared.md` — follow
  that logic (canonical-name map, overlap rules). Report matched / missing / extra /
  conflict. Do **not** write unless the user approves specific fixes.
- If the M365 calendar tool isn't loaded, `ToolSearch` for it; if unavailable, report
  that calendar validation needs the Microsoft 365 connector and skip it.

### Step 4 — Hand off to hours-maker
Tell the user the base is ready and that `/hours-maker travis <week>` (and `ed`) will
place the NEDO-eligible portion into the sheet, capped at the 30 h/week Softbank
ceiling. hours-researcher itself writes nothing to the individual sections.

## Output format (`<month>.md`)

```
# June 2026 — Activity Research (granular truth base)
# Source: N claude events, M browser events; window ...
# INTEGRITY: derived from real timestamps; UNMAPPED reported; nothing synthesized.

## June 1 2026  [active 24.0h | NEDO 6.0h | work 20.5h | personal 3.5h]
   by master: NEDO·Machine-Bringup 5.0, Izuma·USGOV 13.5, Personal·Shopping 3.0, ...
0:00-1:00
 - NEDO·Machine-Bringup | izuma — MaaS / node bringup  (browser:10.7.160.9:5240 x9)
 - NEDO·Cluster | gravhl — ArgoCD / k8s GitOps  (browser:argocd-mgmt.gravhl.com x4)
 - Izuma·USGOV | izuma — SBIR auth  (browser:login.gov x8)
1:00-2:00
 - Personal·Health | — health  (browser:athenahealth.com x3)
 - Personal·Cars | — cars  (browser:autozone.com x2)
...
## UNMAPPED — decide & add to classification_map.json (never auto-counted)
    34  claude:kilroy   e.g. <sample title/query>
```

- **Every line leads with its MASTER** (controlled vocabulary), then `| company — sub-detail`.
- The per-day **`by master`** line rolls up that day's dominant-per-slot hours by master bucket.
- Line order per block: NEDO-work first, then other work, then personal, then UNMAPPED.
- Evidence in parens = real sources + counts. This is the audit trail.

## MASTER vocabulary (controlled, report-item level)

Every map entry carries an explicit `master`. The list:
- **NEDO (report-eligible):** `NEDO·Cluster` (CAPI, migration, ArgoCD, maintenance, control-plane), `NEDO·K8s-Components` (cilium, longhorn, kube-vip, storage), `NEDO·OS` (Wrangler/AlmaLinux, edge-core, Yocto, container-OS), `NEDO·Machine-Bringup` (servers, MaaS, BMC/iDRAC, fans/PCIe/cooling — incl. new-server procurement), `NEDO·Networking` (incus net, BGP, EdgeOS, Entra/OIDC).
- **Work non-NEDO:** `Izuma·Business`, `Izuma·USGOV`, `Izuma·Corp`, `Gravhl`, `tmctech`.
- **Personal:** `Personal·{Health,House,Cars,Kids,Travel,Finance,Entertainment,Genealogy,Shopping,Education,Misc}`.

To re-bucket anything, edit the entry's `master` (and/or `nedo`) in `classification_map.json` — it's explicit, never inferred at runtime.

## Notes & guardrails

- **Read-only.** Reads logs, browser DBs (copied to /tmp to dodge locks), sheet joint
  areas, and Outlook. Writes only `<month>.md`. Never edits the Travis/Ed sections.
- **Current month:** always pass `--end <today>` so future days aren't invented.
- **Don't classify search engines / generic hubs** (google.com, etc.) — leave them
  UNMAPPED so they don't bias work/personal. The Claude cwd captures the real project.
- **Relationship to hours-maker:** this is upstream. hours-maker's own evidence engine
  can place hours directly, but `<month>.md` is the human-reviewable base + audit
  artifact; prefer reviewing it before any placement.
- **tmctech / kilroy / newcorder / ebay / amazon** are marked REVIEW in the starter
  map — they're judgement calls (is tmctech cluster work NEDO? are eBay visits server
  procurement?). Resolve with the user; don't assume.
