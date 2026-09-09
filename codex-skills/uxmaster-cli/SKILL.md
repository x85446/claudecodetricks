---
name: uxmaster-cli
description: UXMASTER child (invoked via $uxmaster): the command-line expert — designs and audits CLI/TUI command grammar, program flow, help, errors, and terminal color.
---


# $uxmaster-cli — the terminal is the interface, and it has a grammar

**Version:** 2.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$uxmaster`'s SKILL.md. This skill owns two references — **read the one your
task needs before doing anything else**, and cite its section in every finding:

## Usage

Argument: <command or subcommand to review, or "design <the flow>">. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$uxmaster` — ported.
- `$uxmaster-analysis` — ported.
- `$uxmaster-implement` — ported.

| File | Owns |
|---|---|
| `grammar.md` | command grammar, options-anywhere law, flag vocabulary, help layout, error shape, exit codes, streams, config precedence, TTY matrix, session flow, completions, TUI, deprecation |
| `color.md` | the color detection ladder, tiers, semantic roles, contrast, symbols/locale, progress, per-language implementation, the six-invocation verification matrix |

**The house grammar** (every design proposal and every naming finding assumes it):

```
<tool> <noun> <verb> [<noun> <verb> …] [operands…] [--flags anywhere]
```

Nouns before verbs, nested as deep as the domain, and **every flag parses in every position** — before the
noun, between subcommands, after the operands. `--` still ends flag parsing. `grammar.md` §1–2 has the law
and the per-parser configuration that actually delivers it.

## Mode A — review an existing command

Run the real binary. An arg parser read in source is a hypothesis, not evidence.

1. **Grammar** (`grammar.md` §1) — noun-verb order, consistent verbs across nouns, no catch-all subcommand,
   no prefix abbreviation, singular nouns, root help fits a screen.
2. **Options anywhere** (§2) — run the same command with the flags moved to three different positions. Any
   invocation that parses differently is a **high-severity** finding, and name the parser fix.
3. **Flags** (§3) — standard names carry standard meanings, `-v` isn't version, long flags kebab-case,
   booleans have `--no-` counterparts, no secrets on the command line, defaults shown.
4. **Help** (§4) — `-h`/`--help` at every level, examples before the flag wall, defaults and placeholders,
   bare command exits 0, missing-argument exits 2.
5. **Errors** (§5) — what failed, why, exact next action; grouped repeats; no stack traces by default;
   did-you-mean on a fixed command set.
6. **Streams and codes** (§6) — result on stdout, everything else on stderr, distinct documented non-zero
   codes, `--json` clean of decoration and stable.
7. **Config precedence** (§7) — one documented order, and a way to see where a value came from.
8. **Interactivity** (§8) — never prompt off a non-TTY stdin, every prompt has a pre-answer flag,
   destructive confirms, `--dry-run` exists, Ctrl-C always works.
9. **Flow** (§9) — one job per command, sane defaults, next-step hint, idempotence, resumability,
   proportional confirmation, first-run path, progress on anything slow.
10. **Color** (`color.md`) — run the **entire six-invocation matrix** in §7. Ladder honored (`NO_COLOR`,
    `--color`, `TERM=dumb`, per-stream `isatty`), 16-color semantics rather than hardcoded RGB, no
    color-alone meaning, no unreadable blue-on-black, symbols with an ASCII fallback, cursor restored.
11. **Completions, pagination, TUI** (§10) — completions shipped, pager only on a TTY, TUI exits clean and
    restores the terminal.

## Mode B — design or repair a flow (`$1` starts with "design", or there's no command yet)

Produce a **command map**, not prose. Deliverables, in this order:

1. **Noun/verb tree** — every command in the house grammar, one line each with its one-line description.
   Flag anything that needed an "and" in its description; that's two commands.
2. **Flag table** — per command: long, short (only if frequent), value placeholder, default, and which are
   global. Reuse the standard vocabulary in `grammar.md` §3.
3. **Help text** for the root and for the two most-used commands, in the §4 layout, examples first.
4. **Error catalog** — the failure modes, each in the three-part shape, with its exit code.
5. **Color role table** — the roles from `color.md` §3 this tool actually needs (usually 3–5), each with its
   ANSI-16 color *and* its non-color signal, plus the ladder the implementation must resolve at startup.
6. **The walkthrough** — the first-run session and the most common repeat session, typed out verbatim as the
   user would see them, including what's on stdout versus stderr.

Record the design as `decisions.md` entries; file anything the current code contradicts as findings. Design
mode writes no code — `$uxmaster-implement` does that, reading these same two references.

## Steps

1. Read `grammar.md` and/or `color.md` for the mode you're in.
2. Exercise the command in `$1`: `--help` at every level, a success path, a failure path, flags in three
   positions, `| cat`, non-TTY stdin, `NO_COLOR=1`, `TERM=dumb`, `--json` if it exists.
3. Append one finding per violation to `./.claude/uxmaster/findings.md` — the **exact invocation** in
   `evidence:`, the corrected behavior in `fix:`, and `grammar.md §N` / `color.md §N` (or the POSIX/GNU rule)
   in `rule:`.
4. Report counts by severity, listing first anything that breaks piping, breaks a script's exit code, or
   leaves the terminal wedged.

## Rules

1. **Every finding names the exact command you ran**, and color findings name which of the six matrix
   invocations exposed it.
2. **Options-anywhere is the house standard.** A parser that can't do it (Go stdlib `flag`, `getopts`) is a
   finding whose fix names the replacement — say so plainly as a dependency change rather than working
   around it.
3. **Never propose breaking an existing flag's meaning** — rename, deprecate with a warning to stderr, and
   give a removal version (`grammar.md` §11).
4. **Color is a layer, never the message.** Any output whose meaning dies when escapes are stripped is a
   finding, at minimum medium severity.
5. Stay in your lane: task-level flow problems above the command line go to `$uxmaster-analysis`, code goes
   to `$uxmaster-implement`.
