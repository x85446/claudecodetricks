---
name: "uxmaster-windows"
description: "UXMASTER child (invoked via $uxmaster): judges an interface against Windows 11 Fluent design and WinUI 3 conventions."
---


# $uxmaster-windows — does this feel like a Windows 11 app

**Version:** 1.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$uxmaster`'s SKILL.md. Findings are `kind: platform-convention` with the Fluent/WinUI rule cited in `rule:`.

## Usage

Argument: <surface to review, e.g. "the settings page">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$uxmaster` — ported.
- `$uxmaster-analysis` — ported.
- `$uxmaster-implement` — ported.

**Establish the stack first**: WinUI 3 / Windows App SDK (the current default for new apps), WPF, or WinForms (legacy — judge against Fluent where the stack allows, and flag the stack itself only if the project is actively modernizing).

## Checklist

1. **Navigation** — `NavigationView` (left pane, or top for few destinations) with a breadcrumb for depth; back button behavior consistent; no invented navigation metaphor.
2. **Settings** — a dedicated settings page inside the app (not a modal dialog), organized as expandable `SettingsCard`/`SettingsExpander` groups. Each setting is a row with a short description; long explanatory copy belongs in an info affordance, not inline paragraphs.
3. **Dialogs** — `ContentDialog` for consequential choices with verb-labeled primary/secondary buttons in Windows order (primary action left in the WinUI ordering, unlike macOS); `TeachingTip` for contextual guidance; `InfoBar` for persistent status; toasts for transient notifications.
4. **Title bar and window** — extended/custom title bar done via the App SDK (draggable regions correct), Mica or Acrylic backdrop where appropriate, snap-layout support, sensible min size, window state restored.
5. **Commanding** — `CommandBar` with primary/secondary commands; right-click context menus present; accelerators (Ctrl+S/Ctrl+Z/F1) standard and access keys (Alt) defined.
6. **Appearance** — theme resources so light/dark and the system accent follow Windows; Fluent icons (Segoe Fluent Icons) for standard glyphs; corner radii and spacing per Fluent; **High Contrast themes honored** (never hardcoded colors).
7. **Accessibility** — UI Automation names/roles on every control for **Narrator**, full keyboard operability with visible focus, text scaling at 200%.
8. **Integration** — MSIX packaging where shipped, jump list / taskbar behavior, file-type associations, and permission prompts requested in context.

## Steps

1. Build and run the app; exercise the surface named in `$1` on a real Windows session (or the closest available target — say so in `evidence` if the run was on a VM or a remote host).
2. Append one finding per violation to `./.claude/uxmaster/findings.md`, citing the rule and naming the concrete WinUI control in `fix:`.
3. Report: counts by severity, stack judged against, headline non-native issues first.

## Rules

1. **Cite the rule or don't file it.**
2. **Windows button order and wording are not macOS's** — never "fix" one platform's dialog to match the other; per-platform is correct, the meta reconciles.
3. Stay in your lane: flow/IA to `$uxmaster-analysis`, code to `$uxmaster-implement`.
