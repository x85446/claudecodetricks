---
name: uxmaster-implement
description: UXMASTER child — writes the actual UI code for an approved design or finding fix, in the platform's real framework (SwiftUI/AppKit for macOS, GTK4+libadwaita or Qt/Kirigami for Linux, WinUI 3/WPF for Windows, the project's web framework, or the CLI/TUI library in use). The only UXMASTER child that edits code. Invoked by /uxmaster with finding ids, or directly ("implement F3", "build this design").
argument-hint: <finding ids (e.g. "F3 F7") or a design to implement; empty = all open findings>
version: 1.0.0
---

# /uxmaster-implement — turn findings and designs into real UI code

Obey the shared contracts in `/uxmaster`'s SKILL.md. This is the ONLY child that writes code, and the only one (besides the user) that flips a finding's `status:`.

## Framework map — use what the project actually uses

| Platform | Default | Legacy/alt found in-repo |
|---|---|---|
| macOS | SwiftUI (AppKit for what SwiftUI can't express — `NSViewRepresentable` bridges) | pure AppKit |
| Linux/GNOME | GTK4 + libadwaita | GTK3 |
| Linux/KDE | Qt6 + Kirigami | Qt Widgets |
| Windows | WinUI 3 / Windows App SDK | WPF, WinForms |
| Web | the project's existing framework and styling system | — |
| CLI/TUI | the project's existing arg-parser and TUI library | — |

**Never introduce a second UI framework beside a working one**, and never rewrite a screen into a new framework as a side effect of a fix — if a finding genuinely requires a migration, say so and stop for the user's call.

## Steps

1. **Resolve the work**: finding ids from `$1` (else every `status: open` finding with a concrete `fix:`). Read each finding — the `fix:` line and its `rule:` are the spec. If a finding is vague, get it sharpened by the child that filed it rather than inventing an interpretation.
2. **Detect the framework** from the repo (table above) and follow the file/naming layout already in use.
3. **Implement each fix natively** — use the platform's real control for the job (an `AdwSwitchRow`, a `ContentDialog`, a `NavigationSplitView`), not a hand-rolled lookalike. Match surrounding code style, and keep localization/theming/accessibility attributes intact as you go: accessible name and role on every control you touch, semantic colors not literals, keyboard path preserved.
4. **Build it.** If the project has a Makefile, build through its target (`[skill: /dev-makefiles]` governs build targets, not ad-hoc compiler invocations).
5. **Verify by running it** — launch the app/page/command and exercise the exact flow the finding described. Screenshot-free is fine, but the check must be a real interaction, and the result goes in the finding's evidence trail. A fix that wasn't exercised isn't done.
6. **Update state**: flip each finding to `status: fixed` (or `deferred (<reason>)` when the user's call blocks it), and append a line to `./.claude/uxmaster/decisions.md` for any design judgment you had to make.
7. **Report**: one line per finding — `F<N> fixed — <what changed> (verified: <the interaction>)`.

## Rules

1. **Implement the approved fix, don't redesign.** New problems noticed while coding get filed as new findings for the right expert — never silently expanded scope.
2. **Every fix is verified by running it**, per step 5. This is the same interactive mandate `/iterate` enforces.
3. **Accessibility is not a follow-up** — a control shipped without a name/role/keyboard path is an incomplete fix, not a fixed finding.
4. **No framework migrations as a side effect** — flag and stop instead.
5. Never mark a finding `fixed` you didn't verify, and never renumber findings.
