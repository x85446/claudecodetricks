---
name: uxmaster-macos
description: UXMASTER child (invoked via $uxmaster): judges an interface against the Apple Human Interface Guidelines for macOS.
---


# $uxmaster-macos — does this feel like a Mac app

Obey the shared contracts in `$uxmaster`'s SKILL.md. Findings here are `kind: platform-convention` with the HIG rule cited in `rule:`.

## Usage

Argument: <surface to review, e.g. "the settings window">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$uxmaster` — ported.
- `$uxmaster-analysis` — ported.
- `$uxmaster-implement` — ported.

## Review checklist — exercise the app, then judge each

1. **Menu bar** — every command reachable from the menu bar, not just from in-window controls. App menu holds About/Settings/Services/Quit; File/Edit/View/Window/Help present and correctly populated; Edit has the standard editing commands with standard shortcuts.
2. **Settings** — a **Settings…** item in the app menu (⌘,), opening a separate Settings *window* (not a modal sheet, not an in-window tab strip), toolbar-tabbed when multiple panes. macOS 13+ says "Settings", not "Preferences".
3. **Windows** — resizable where content allows; state restored between launches; standard traffic-light controls; full-screen behaves; multiple documents get multiple windows, not a tab strip you invented.
4. **Toolbar** — customizable where it earns it, icons with labels available, no toolbar for a single action.
5. **Modality** — sheets for document-scoped tasks, alerts only for consequential interruptions (with a destructive-button convention and a real default), popovers for transient contextual controls. Flag every unnecessary modal.
6. **Keyboard** — standard shortcuts never repurposed (⌘W/⌘Q/⌘S/⌘Z/⌘F), full keyboard access works, Escape cancels, Return activates the default.
7. **Appearance** — Dark Mode supported via semantic colors (never hardcoded), system accent color respected, SF Symbols used for standard iconography, Dynamic Type and Increase Contrast honored.
8. **VoiceOver** — every control labeled and reachable; rotor navigation sensible.
9. **Platform integration** — drag-and-drop, Services, Share menu, Continuity, Spotlight, notifications, sandbox/permission prompts requested in context (not all at launch).

## Steps

1. Launch and exercise the surface named in `$1` (or the whole app when unscoped) — the checklist judges observed behavior, not source.
2. Append one finding per violation to `./.claude/uxmaster/findings.md`, citing the HIG rule in `rule:` and the concrete native alternative in `fix:`.
3. Report: counts by severity plus the "this doesn't feel like a Mac app" headline issues first.

## Rules

1. **Cite the rule or don't file it** — a convention finding without a HIG basis is taste, not a finding.
2. Stay in your lane: flow/IA problems belong to `$uxmaster-analysis`; writing Swift belongs to `$uxmaster-implement`.
3. **Never recommend flattening a platform difference away** — if Windows does it differently, that's per-platform behavior, and the meta reconciles it (rule 3 there).
