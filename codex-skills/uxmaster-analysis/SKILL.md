---
name: uxmaster-analysis
description: UXMASTER child (invoked via $uxmaster): platform-agnostic UX audit — task flow, information architecture, state and error handling, accessibility fundamentals.
---


# $uxmaster-analysis — is the experience right, regardless of platform

**Version:** 1.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$uxmaster`'s SKILL.md (findings format, exercise-never-guess, state paths) — read it if not already in context.

## Usage

Argument: <surface to analyze, e.g. "the settings screen" or "first-run onboarding">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$uxmaster` — ported.
- `$uxmaster-implement` — ported.

## Steps

1. **Exercise the surface for real.** Launch/serve/run it and walk the actual task a user comes to do, start to finish. Note where you hesitate, backtrack, or can't tell what happened — those moments are the findings.
2. **Audit against these lenses**, in this order:
   - **Task flow** — can the primary task be completed without detours? Count the steps; flag every one that isn't load-bearing. Flag dead ends and flows that abandon the user after success.
   - **Information architecture** — does grouping match how a user thinks about the domain (not how the code is organized)? Flag settings/controls filed under the wrong concept, and any screen mixing unrelated concerns.
   - **State and feedback** — does every action produce visible confirmation? Are loading, empty, error, and success states all designed? Flag any operation >1s with no progress indication, and any silent failure.
   - **Error handling** — do errors say what happened, why, and what to do next, in the user's words? Flag raw exceptions, error codes without remedy, and validation that only fires after submit.
   - **Content** — labels, buttons, and headings phrased as user goals, not implementation nouns. Flag jargon leaking from the codebase into the UI. **Informational/explainer text sitting inline in a settings screen is a finding** — the fix is tucking it under an (i) affordance (this is the standing FFIV rule).
   - **Accessibility fundamentals** (platform-neutral): keyboard reachability of every control, focus order and visibility, text alternatives for meaningful images/icons, contrast, and never using color alone to carry meaning.
3. **Write each finding** to `./.claude/uxmaster/findings.md` in the shared format, with `kind` of `ux` (flow/IA/content), `mistake` (broken behavior), or `a11y`. Severity: **high** = blocks or misleads the user on a primary task; **medium** = costs real time or confidence; **low** = polish.
4. **Report** to the caller: counts by severity, the high-severity list one line each, and the single most valuable fix first.

## Rules

1. **No platform-convention judgments here** — "this menu belongs in the app menu on macOS" is a platform expert's finding. Stay on flow, IA, state, content, accessibility.
2. **No code, no mockup implementation** — describe the fix; `$uxmaster-implement` writes it.
3. **Every finding traces to something you actually did** (the click, the command, the input) — record it in `evidence`.
4. Never renumber existing findings; append only.
