---
name: uxmaster-windows
description: UXMASTER child — Windows desktop UX expert. Judges an interface against Windows 11 Fluent design and WinUI 3 conventions: navigation view, settings pages, content dialogs, title bar and Mica, keyboard and accelerator conventions, high contrast and Narrator support, MSIX packaging integration. Produces platform-convention findings only. Invoked by /uxmaster or directly ("is this right for Windows").
argument-hint: <surface to review, e.g. "the settings page">
version: 1.0.0
---

# /uxmaster-windows — does this feel like a Windows 11 app

Obey the shared contracts in `/uxmaster`'s SKILL.md. Findings are `kind: platform-convention` with the Fluent/WinUI rule cited in `rule:`.

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
3. Stay in your lane: flow/IA to `/uxmaster-analysis`, code to `/uxmaster-implement`.
