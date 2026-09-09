---
name: "uxmaster-implement"
description: "UXMASTER child (invoked via $uxmaster): writes the actual UI code for an approved design in the project's real framework. The only UXMASTER child that edits code."
---


# $uxmaster-implement — turn findings and designs into real UI code

**Version:** 1.2.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$uxmaster`'s SKILL.md. This is the ONLY child that writes code, and the only one (besides the user) that flips a finding's `status:`.

## Usage

Argument: <finding ids (e.g. "F3 F7") or a design to implement; empty = all open findings>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$dev-makefiles` — ported.
- `$iterate` — ported.
- `$uxmaster` — ported.
- `$uxmaster-cli` — ported.

## Framework map — use what the project actually uses

| Platform | Default | Legacy/alt found in-repo |
|---|---|---|
| macOS | SwiftUI (AppKit for what SwiftUI can't express — `NSViewRepresentable` bridges) | pure AppKit |
| Linux/GNOME | GTK4 + libadwaita | GTK3 |
| Linux/KDE | Qt6 + Kirigami | Qt Widgets |
| Windows | WinUI 3 / Windows App SDK | WPF, WinForms |
| Web | the project's existing framework and styling system | — |
| CLI/TUI | the project's existing arg-parser and TUI library — **and the contract below** | — |

**Never introduce a second UI framework beside a working one**, and never rewrite a screen into a new framework as a side effect of a fix — if a finding genuinely requires a migration, say so and stop for the user's call.

## CLI/TUI contract — read before writing a single line of terminal code

When the surface is a command line or TUI, `$uxmaster-cli`'s two references are the spec, not background
reading: **`uxmaster-cli/grammar.md`** (command grammar, flags, help, errors, exit codes, flow) and
**`uxmaster-cli/color.md`** (the detection ladder, tiers, semantic roles, verification matrix). Read the
sections the finding cites, then implement to them.

Three things are structural — get them wrong and no later fix is cheap:

1. **The grammar is `<tool> <noun> <verb> … [operands] [--flags anywhere]`.** Wire the parser so every flag
   parses in every position: `pflag`/`cobra` with `PersistentFlags()` (and `TraverseChildren` where a
   parent has local flags), `clap` with `#[arg(global = true)]`, `argparse` with a shared `parents=[…]`
   parser on every subparser, `click` with the shared option on group *and* commands. Go's stdlib `flag`
   and shell `getopts` cannot do it — say so and stop rather than shipping position-sensitive flags.
2. **Color resolves once, at startup, per stream.** One function returns the tier by the `color.md` §1
   ladder; `stdout` and `stderr` get independent `isatty()` checks. Nothing downstream re-decides, and no
   call site names a raw escape code.
3. **Semantic roles, not colors, at the call sites.** One module maps role → (ANSI-16 color, symbol,
   prefix). Every message picks a role. A message whose meaning disappears when escapes are stripped is not
   finished code.

Prefer a library that already implements the ladder — `lipgloss`/`fatih/color` (Go), `anstream`+`anstyle`
(Rust), `rich`/`click` (Python), `chalk`/`picocolors` (Node) — over hand-rolled escapes. In shell, resolve
to variables that are empty strings when color is off, so one `printf` serves both paths.

## Steps

1. **Resolve the work**: finding ids from `$1` (else every `status: open` finding with a concrete `fix:`). Read each finding — the `fix:` line and its `rule:` are the spec. If a finding is vague, get it sharpened by the child that filed it rather than inventing an interpretation.
2. **Detect the framework** from the repo (table above) and follow the file/naming layout already in use.
3. **Implement each fix natively** — use the platform's real control for the job (an `AdwSwitchRow`, a `ContentDialog`, a `NavigationSplitView`), not a hand-rolled lookalike. Match surrounding code style, and keep localization/theming/accessibility attributes intact as you go: accessible name and role on every control you touch, semantic colors not literals, keyboard path preserved.
4. **Build it.** If the project has a Makefile, build through its target (`[skill: $dev-makefiles]` governs build targets, not ad-hoc compiler invocations).
5. **Verify by running it** — launch the app/page/command and exercise the exact flow the finding described. Screenshot-free is fine, but the check must be a real interaction, and the result goes in the finding's evidence trail. A fix that wasn't exercised isn't done.
   **For CLI/TUI fixes, verification is fixed and non-negotiable**: the six invocations in `color.md` §7
   (`cmd`, `cmd | cat`, `cmd 2>&1 | cat`, `NO_COLOR=1 cmd`, `TERM=dumb cmd`, `cmd --color=always | cat`)
   plus the same command with its flags in three different positions producing byte-identical output. Paste
   the results into the evidence trail.
6. **Update state**: flip each finding to `status: fixed` (or `deferred (<reason>)` when the user's call blocks it), and append a line to `./.claude/uxmaster/decisions.md` for any design judgment you had to make.
7. **Report**: one line per finding — `F<N> fixed — <what changed> (verified: <the interaction>)`.

## Rules

1. **Implement the approved fix, don't redesign.** New problems noticed while coding get filed as new findings for the right expert — never silently expanded scope.
2. **Every fix is verified by running it**, per step 5. This is the same interactive mandate `$iterate` enforces.
3. **Accessibility is not a follow-up** — a control shipped without a name/role/keyboard path is an incomplete fix, not a fixed finding.
4. **No framework migrations as a side effect** — flag and stop instead.
5. **On a terminal surface, `$uxmaster-cli`'s references outrank your instincts** — implement the ladder,
   the roles, and the options-anywhere parser as written, or file a finding explaining why the project can't.
6. Never mark a finding `fixed` you didn't verify, and never renumber findings.
