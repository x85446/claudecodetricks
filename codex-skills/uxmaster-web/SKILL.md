---
name: uxmaster-web
description: UXMASTER child (invoked via $uxmaster): judges a web interface against web platform conventions and WCAG 2.2 AA.
---


# $uxmaster-web — does this behave like the web

Obey the shared contracts in `$uxmaster`'s SKILL.md. Findings are `kind: platform-convention` or `a11y`, with the WCAG/spec rule cited in `rule:`.

## Usage

Argument: <page or flow to review, e.g. "the signup flow">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$uxmaster` — ported.
- `$uxmaster-analysis` — ported.
- `$uxmaster-implement` — ported.

## Checklist — load the real page in a browser and exercise it

1. **Responsive** — works from 320px to wide desktop; no horizontal page scroll; touch targets ≥44px; content reflows rather than shrinking to unreadable.
2. **Semantic structure** — real landmarks (`header`/`nav`/`main`/`footer`), one `h1` with a sane heading order, lists as lists, buttons as `<button>` and links as `<a>` (a div with a click handler is a finding).
3. **Keyboard and focus** — every interactive element reachable by Tab in logical order, visible focus ring, skip-to-content link, focus moved into and restored out of dialogs, Escape closes overlays, no keyboard traps.
4. **Forms** — labels tied to inputs, appropriate `type`/`autocomplete`, inline validation that fires at the right time (not only on submit), errors announced (`aria-live`) and tied to their field, submit disabled-state never hiding why.
5. **States** — loading (skeleton/spinner with text), empty (explains what goes here and how), error (what happened + what to do), and success all designed. Flag any fetch >1s with no feedback.
6. **Browser-native behavior** — the back button works and does the expected thing, URLs are deep-linkable and shareable, refresh preserves sensible state, browser zoom to 200% doesn't break layout, and text can be selected/copied where it's content.
7. **Contrast and color** — 4.5:1 body text, 3:1 large text and UI boundaries, never color alone for meaning, `prefers-reduced-motion` and `prefers-color-scheme` honored.
8. **Performance as UX** — measure real Core Web Vitals (LCP, INP, CLS) on the running page; layout shift and input delay are user-facing findings, not just metrics.

## Steps

1. Load the real page/flow in a browser and exercise it (both a wide and a 320-narrow viewport, and once entirely by keyboard). Console errors seen during the walk are findings.
2. Append one finding per violation to `./.claude/uxmaster/findings.md`, citing the WCAG success criterion or spec in `rule:`.
3. Report: counts by severity, a11y blockers first, then the highest-impact UX fix.

## Rules

1. **Cite the criterion or don't file it** (WCAG number, or the concrete spec/browser behavior).
2. **Never file a finding you didn't observe in a real browser** — no "the markup suggests" findings.
3. Stay in your lane: flow/IA to `$uxmaster-analysis`, code to `$uxmaster-implement`.
