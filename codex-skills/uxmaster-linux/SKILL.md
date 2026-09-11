---
name: uxmaster-linux
description: UXMASTER child (invoked via $uxmaster): judges an interface against the GNOME HIG (GTK4/libadwaita) or KDE HIG (Qt/Kirigami).
---


# $uxmaster-linux — does this feel native on the Linux desktop

**Version:** 1.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$uxmaster`'s SKILL.md. Findings are `kind: platform-convention` with the HIG rule cited in `rule:`.

## Usage

Argument: <surface to review, optionally naming the toolkit, e.g. "settings, GTK4">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$uxmaster` — ported.
- `$uxmaster-analysis` — ported.
- `$uxmaster-implement` — ported.

**First, establish the target.** GTK4/libadwaita (GNOME) and Qt/Kirigami (KDE) have genuinely different conventions — judge against the one the project actually builds with (`meson.build`/`gtk4`/`libadwaita` vs `CMakeLists.txt`/`Qt6`/`kirigami`). If a project targets both, review against each separately; never blend them.

## GNOME / GTK4 / libadwaita checklist

1. **Header bar, not a menu bar** — window title area carries the primary actions plus a hamburger (primary) menu; no traditional menu bar. Destructive/suggested actions use their libadwaita style classes.
2. **Preferences** — an `AdwPreferencesWindow` with grouped rows (`AdwPreferencesGroup` / `AdwActionRow` / `AdwSwitchRow`), opened from the primary menu, not a bespoke settings pane. Explanatory text belongs in a row's subtitle or an info affordance — never a wall of inline paragraph text.
3. **Adaptive layout** — `AdwNavigationSplitView`/`AdwBreakpoint` so the window works from phone width to desktop; nothing that breaks under resize.
4. **Dialogs** — `AdwAlertDialog`/`AdwDialog` with clear body text and verb-labeled buttons (never "OK/Cancel" for a consequential choice); no nested modals.
5. **System integration** — a valid `.desktop` file with icon and categories, an app icon following the GNOME icon spec, Flatpak manifest where shipped, and **portals** for file access/screenshots/notifications rather than raw filesystem reach.
6. **Appearance** — libadwaita named colors so light/dark and accent follow the system; symbolic icons for actions; no hardcoded palette.
7. **Accessibility** — every widget has an accessible name/role for **Orca**; full keyboard traversal; focus visible.

## KDE / Qt / Kirigami checklist (when that's the target)

Kirigami page-stack navigation and global drawer; KConfigXT-backed settings in a standard configuration dialog; KDE HIG button ordering and wording; Breeze icon set; Qt Accessibility exposed for Orca.

## Steps

1. Build/run the app and exercise the surface in `$1` on a real session (Wayland where available — flag X11-only assumptions like absolute window positioning or global pointer grabs).
2. Append one finding per violation to `./.claude/uxmaster/findings.md`, citing the specific HIG rule and naming the native widget/pattern in `fix:`.
3. Report: counts by severity, toolkit judged against, headline non-native issues first.

## Rules

1. **Judge against the project's actual toolkit** — GNOME rules applied to a Qt app (or vice versa) are noise, not findings.
2. **Cite the rule or don't file it.**
3. Stay in your lane: flow/IA to `$uxmaster-analysis`, code to `$uxmaster-implement`.
4. Never recommend erasing a platform difference to match macOS or Windows — per-platform behavior is correct; the meta reconciles.
