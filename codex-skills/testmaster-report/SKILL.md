---
name: testmaster-report
description: TESTMASTER child (invoked via $testmaster): regenerates the self-contained HTML report card from the registry and run history.
---


# $testmaster-report — the HTML report card

**Version:** 1.1.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$testmaster`'s SKILL.md. Reads `./.claude/testmaster/registry.json` + `history.jsonl`; writes ONLY `./.claude/testmaster/report/index.html`. Never touches tests or the registry.

## Usage

Argument: (none — regenerates and opens the report). `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$testmaster` — ported.

## Steps

1. Read both state files. If the registry is empty/missing, still generate the page with a "no tests registered — run $testmaster maintain" banner.
2. **Generate `./.claude/testmaster/report/index.html`** — one fully self-contained static file (inline CSS/JS, no CDNs, no external assets) with these sections:
   - **Scorecard header**: total tests, pass/fail/unmeasured counts, last full-run timestamp, overall grade (A–F from pass rate, "I" incomplete when >20% unmeasured).
   - **Tier table**: per tier (fast/standard/slow/?) — count, combined avg duration, pass rate. This is the "what can iterate afford to run" view.
   - **Per-test table**, sortable by column (small inline JS): id, file, tier, `avg_ms` (human units), `last_ms`, runs, parallel yes/no, last result (green/red), last run age. Unmeasured tests show "unmeasured", never a guessed number.
   - **Slowest 10** — the promotion-watch list: tests nearest their tier boundary, with trend arrow (avg of last 3 runs vs lifetime avg, from history).
   - **Failures**: every currently-red test with its last error gist and how many consecutive runs it's been red (from history).
   - Footer: generated timestamp + "all durations are real measurements from history.jsonl".
3. **Open it**: `open ./.claude/testmaster/report/index.html` (macOS) / `xdg-open` (Linux).
4. Report one line: `report card regenerated — N tests, grade <X>, <path>`.

## Rules

1. **Every number on the page is a real measurement** (or "unmeasured") — the page must never contain an estimated duration.
2. Self-contained only — the file must render offline, from disk, forever.
3. Regenerate whole — never patch the old HTML; state files are the single source.
