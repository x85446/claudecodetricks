---
name: "uxmaster"
description: "UXMASTER — the UX/UI design meta. Route ALL interface work here; the meta picks the child. Use for any UX/UI work: \"uxmaster\", \"review the UX\", \"audit the UI\", \"what's wrong with this interface\", \"design this screen\", \"design the settings\", \"make the interface better\", \"is this right for macOS/Windows/GNOME/Linux\", \"does this feel native\", \"check accessibility\", \"WCAG audit\", \"review the CLI's UX\", \"review the web UI\", \"implement this design\", \"build the design\", \"implement F3\"."
---

<!-- version: shared across the family; see the **Version:** line above. -->


<!-- codex-port: no confirmed structured-picker equivalent in Codex; every structured picker in this file became an ordinary numbered-list question -- verify the wording reads naturally where it mattered. -->

# $uxmaster — UX/UI design orchestrator (UXMASTER)

**Version:** 1.2.0

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

UXMASTER — the UX/UI design meta. Detects the project's platform and routes to its children: analysis (platform-agnostic audit), the platform experts (macOS, Linux, Windows, web, command-line), and implement (writes the real framework code). Findings land in ./.claude/uxmaster/findings.md. Pairs with the FFIV macro's UX/UI enhancement sweep.

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Meta skill. Routes to one child per concern and owns the shared contracts below. Do the routed work via explicit `$name` invocation — never inline a child's job here.

## Usage

Argument: <analyze | review [platform] | design <what> | implement [finding-ids] | status>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$iterate` — ported.
- `$uxmaster-analysis` — ported.
- `$uxmaster-cli` — ported.
- `$uxmaster-implement` — ported.
- `$uxmaster-linux` — ported.
- `$uxmaster-macos` — ported.
- `$uxmaster-web` — ported.
- `$uxmaster-windows` — ported.

| Child | Job |
|---|---|
| `$uxmaster-analysis` | Platform-agnostic UX audit — flows, information architecture, heuristics, accessibility |
| `$uxmaster-macos` | Apple platform conventions (macOS HIG) |
| `$uxmaster-linux` | GNOME/KDE conventions (GNOME HIG, libadwaita; KDE HIG, Qt) |
| `$uxmaster-windows` | Windows 11 conventions (Fluent, WinUI 3) |
| `$uxmaster-web` | Web/responsive conventions and WCAG |
| `$uxmaster-cli` | Command-line and TUI design — owns the house command grammar (`uxmaster-cli/grammar.md`) and the terminal color system (`uxmaster-cli/color.md`), which `$uxmaster-implement` implements against |
| `$uxmaster-implement` | Writes the actual UI code in the detected framework |

## Shared contracts (all children obey these)

**State lives at `./.claude/uxmaster/`** (project-local, created on first touch):
- `findings.md` — the shared findings ledger. Every child appends in this exact format:
  ```
  ### F<N> — <one-line title>
  - surface: <screen / flow / command / page>
  - kind: mistake | ux | ui | a11y | platform-convention
  - severity: high | medium | low
  - evidence: <what was actually observed, and how it was exercised>
  - fix: <the concrete change to make>
  - rule: <HIG/WCAG/spec citation when the finding is a convention violation, else —>
  - status: open | fixed | deferred (<reason>)
  ```
  `<N>` is monotonic across the whole file — never renumber, never reuse. Children append; only `$uxmaster-implement` and the user flip `status`.
- `decisions.md` — design decisions made and why (append-only, one line each). Prevents relitigating settled choices next session.

**Exercise, never guess.** Every finding must come from actually running the thing — launching the app, loading the page, invoking the command, clicking the flow. "Reading the view code suggests…" is not a finding. This is the same interactive mandate `$iterate` enforces on validations.

**Platform detection** (the meta does this once, passes the answer to children):
| Signal | Platform |
|---|---|
| `Package.swift`, `*.xcodeproj`, `*.swift` with SwiftUI/AppKit | macos |
| `meson.build`, `*.ui` + GTK, `flatpak`, `*.desktop` | linux |
| `*.csproj`, `*.sln`, WinUI/WPF/WinForms references | windows |
| `package.json` with a web framework, `index.html`, a served app | web |
| A binary/script with flag parsing and no GUI, or a full-screen TUI | cli |

Multiple matches = multiple platforms; route to each expert and reconcile (see rule 3). No match = ask which platform by asking the user to choose from a short numbered list, then proceed.

## Router — parse `$1`

1. **analyze** ("analyze", "audit the UX", "what's wrong with this UI") → `$uxmaster-analysis` first (platform-agnostic pass), THEN each detected platform's expert for convention findings. Merge into `findings.md`.
2. **review `<platform>`** ("review macos", "is this right for windows") → that one platform expert only.
3. **design `<what>`** ("design the settings screen", "how should this flow work") → `$uxmaster-analysis` for the flow/IA proposal, then the platform expert(s) to make it native. Output is a design proposal + `decisions.md` entries, no code. On a **cli** platform, `$uxmaster-cli` runs its design mode and returns the noun/verb command map, flag table, help text, error catalog, and color roles.
4. **implement** ("implement", "implement F3 F7", "build the design") → `$uxmaster-implement` with the finding ids (or all `status: open` fixes when unspecified).
5. **status** → read-only summary of `findings.md`: counts by severity and status, plus the open high-severity list. Touch nothing.
6. **default** (anything else describing UI/UX work) → decide analyze vs design vs implement by the work's nature and route as above.

## Rules

1. **The meta routes; it never writes findings or UI code itself.** Detection, dispatch, merge, report — that's the whole job.
2. **Analysis before convention, convention before code.** A flow that's wrong is still wrong after it's made perfectly native; and never implement a design no expert has reviewed.
3. **Cross-platform conflicts get reconciled explicitly, never silently averaged.** When macOS and Windows conventions disagree (window controls, menu placement, settings-vs-preferences), record BOTH platform rules in the finding and pick per-platform behavior — never a lowest-common-denominator UI that's foreign on both. Log the reconciliation in `decisions.md`.
4. **Findings are append-only and never renumbered** — other files, plans, and conversations cite `F<N>` by number.
5. **No taste-only findings.** Every finding names either an observed user-facing problem or a cited platform/accessibility rule. "I'd prefer blue" is not a finding.
