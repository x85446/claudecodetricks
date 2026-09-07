# Terminal color — the reference `/uxmaster-cli` and `/uxmaster-implement` both obey

Color in a terminal is a **semantic layer over text that must survive being stripped**. Every rule below
follows from one fact: you do not know the user's terminal, theme, background, color vision, or whether
your output is going to a human at all. Design for the strip; decorate for the TTY.

---

## 1. The detection ladder — evaluate top to bottom, first match wins

Resolve this **once at startup, per stream**, into a single `ColorTier` value the rest of the program reads.

| # | Signal | Result |
|---|---|---|
| 1 | `--color=always\|never\|auto` / `--no-color` on the command line | `always` → on, `never` → off, `auto` → keep going |
| 2 | `<APP>_COLOR` / `<APP>_NO_COLOR` (app-specific escape hatch) | on / off |
| 3 | `NO_COLOR` present and **non-empty** | **off** |
| 4 | `FORCE_COLOR` non-empty, or `CLICOLOR_FORCE` set and `!= 0` | **on, even when piped** |
| 5 | `TERM=dumb` (or `TERM` unset) | **off** |
| 6 | `isatty()` false **on the stream being written** | **off** |
| 7 | `CI` set with no `FORCE_COLOR` | off (CI logs eat escapes) |
| 8 | `COLORTERM` = `truecolor` or `24bit`, or `TERM` matches `*-direct` | tier = **24-bit** |
| 9 | `TERM` matches `*-256color` | tier = **256** |
| 10 | anything else | tier = **ANSI 16** (the safe floor) |

Rules that fall out of the ladder:

- **`NO_COLOR` beats force, an explicit flag beats `NO_COLOR`.** The user typing `--color=always` right now
  is a stronger signal than the variable they exported in `.bashrc` last year.
- **Empty `NO_COLOR=` means unset.** Only a non-empty value disables. Any non-empty value counts; never
  parse it for `true`/`0`.
- **Check the stream you are decorating.** `stdout` piped to `less` while `stderr` is still a terminal is
  the normal case — two independent `isatty()` calls, two independent decisions. Coloring stderr off
  stdout's TTY state is the single most common bug in this area.
- **`NO_COLOR` governs color only.** Bold, dim, underline, and reverse are not color; they may stay. Strip
  them too only for `TERM=dumb` or non-TTY.
- `$TERM` states how the terminal wants to be *treated*, not what it can do — most truecolor terminals
  still ship `TERM=xterm-256color`, so `COLORTERM` is the real 24-bit signal.

## 2. Prefer the user's 16 colors over your own palette

The 16 ANSI colors are **theme slots, not fixed values** — the terminal's theme decides what "red" is. Using
them means every tool in the user's terminal agrees, and their Solarized/Gruvbox/high-contrast theme keeps
working. A hardcoded 24-bit lime green wins in your screenshot and clashes in their theme.

| Tier | Escape | Use it for |
|---|---|---|
| ANSI 16 | `\e[31m` fg 30–37, bright 90–97; bg 40–47/100–107 | **the default for all semantic output** |
| 256 | `\e[38;5;<n>m` | opt-in themes, syntax highlighting, heat maps |
| 24-bit | `\e[38;2;<r>;<g>;<b>m` | brand color the user asked for, image/graph rendering |

Degrade down the tiers — never require the top one. Every 256/24-bit color needs a 16-color fallback that
still carries the meaning.

## 3. Semantic roles — assign meaning, then pick a color for the role

Define roles once in one module. Nothing outside that module names a color.

| Role | ANSI 16 | Paired signal (required) |
|---|---|---|
| error | red (31) + bold | `Error:` prefix, `✖`, on **stderr** |
| warning | yellow (33) | `Warning:` prefix, `⚠` |
| success | green (32) | `✔` |
| info / progress | default fg, or cyan (36) | prefix noun |
| heading | bold, no color | blank line above |
| identifier (path, id, flag, command) | cyan (36) or underline | quotes/backticks |
| muted (units, timestamps, hints) | dim (2) | position/parens |
| added / removed (diffs) | green / red | leading `+` / `-` |
| prompt | bold | trailing `? ` or `> ` |

- **Never color-alone.** ~8% of men have a red/green deficiency, and every user hits `grep`, `tee`, and CI
  logs where color is gone. Red-vs-green with no `✖`/`✔`, no prefix, and no position difference is a
  finding — the output must be fully readable after `sed 's/\x1b\[[0-9;]*m//g'`.
- **Two colors is a design, five is a christmas tree.** Color marks the exception; default foreground is
  the workhorse.
- **Emphasis before hue.** Bold and blank lines structure output more reliably than any color, in every
  terminal, at every color tier.

## 4. Contrast — the background is unknown

- **Set foreground only.** If you set a background you must set the foreground in the same sequence,
  otherwise the user's default fg lands on your bg and can match it exactly.
- **Blue (34) on black is unreadable** in many default themes. For emphasis on dark backgrounds use cyan,
  bright blue (94), or bold rather than plain blue.
- **Bright yellow on white is unreadable.** Yellow is a dark-background color; on light themes use it only
  with a symbol carrying the meaning.
- **Dim (SGR 2) is optional in the spec** — some terminals ignore it, a few render it invisible. Dim is for
  text the user can afford to miss, never for essential content.
- **Bold is not bright.** Some terminals render bold as the bright variant, some thicken the glyph, some do
  both. Never encode a distinction that only survives if bold means bright.
- Don't reset with `\e[0m` inside a styled region you didn't open — reset the specific attribute (`\e[39m`
  foreground, `\e[22m` intensity) or re-apply the enclosing style.

## 5. Beyond color — the rest of the escape budget

- **OSC 8 hyperlinks**: `\e]8;;https://example.com\e\\link text\e]8;;\e\\`. Supported by most modern
  terminals, harmless (invisible) elsewhere. Gate on the same TTY check and always keep the URL readable in
  plain text when the tier is 16 or stripped.
- **Unicode symbols need a locale check.** `✔ ✖ ⚠ → …` require a UTF-8 locale (`LC_ALL`/`LC_CTYPE`/`LANG`
  containing `UTF-8`). Ship an ASCII fallback map (`[ok] [x] [!] -> ...`) and use it when the locale isn't
  UTF-8 or `TERM=dumb`.
- **Emoji are wide and unpredictable** — they break column alignment in tables and lists. Confine them to
  end-of-line accents, never inside aligned columns.
- **Progress bars, spinners, and any cursor movement (`\r`, `\e[K`, `\e[?25l`) are TTY-only** and belong on
  **stderr**. Non-TTY gets nothing or one line per state change. Re-show the cursor on exit — including on
  SIGINT — or you leave the terminal wedged.
- **Alternate screen (`\e[?1049h`) is for full-screen TUIs only.** Restore on every exit path; a CLI that
  swallows its own output on exit has lost the user's data.

## 6. Implementation snippets — the shape, per ecosystem

**Go** — `github.com/fatih/color` (honors `NO_COLOR`, exposes `color.NoColor`) or `lipgloss` (auto-detects
tier and degrades). Check `term.IsTerminal(int(os.Stdout.Fd()))` per stream.

```go
useColor := colorTier(os.Stdout) > tierNone   // resolve once, at startup
errOut   := colorTier(os.Stderr) > tierNone   // separately
```

**Rust** — `anstream`/`anstyle` (the `clap` ecosystem's stack: writes to a stream that strips styles when
the destination can't take them) or `owo-colors` with `supports-color`. `clap` itself takes
`--color=<when>` via `ColorChoice`.

**Python** — `rich` (`Console(file=sys.stderr)`, respects `NO_COLOR`/`FORCE_COLOR`/`TERM=dumb`, degrades by
tier) or `click.secho` + `click.style` with `color=None` for auto.

**Node** — `chalk` (respects `NO_COLOR`, `FORCE_COLOR`, `--no-color`; `chalk.level` is the tier) or
`picocolors` when startup time matters.

**Bash** — resolve once, emit variables, and make every one of them empty when color is off, so the same
`printf` works both ways:

```bash
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-dumb}" != dumb ]; then
    RED=$'\e[31m'; GREEN=$'\e[32m'; YELLOW=$'\e[33m'; BOLD=$'\e[1m'; DIM=$'\e[2m'; RESET=$'\e[0m'
else
    RED=''; GREEN=''; YELLOW=''; BOLD=''; DIM=''; RESET=''
fi
printf '%s✔%s  %s\n' "$GREEN" "$RESET" "$msg"
```

## 7. The verification matrix — no color finding is filed or fixed without running all six

| Invocation | Expected |
|---|---|
| `cmd` in a terminal | full color at the terminal's tier |
| `cmd \| cat` | no escape sequences in stdout at all |
| `cmd 2>&1 \| cat` | no escapes; stderr messages still fully meaningful |
| `NO_COLOR=1 cmd` | no color; symbols/prefixes still carry every meaning |
| `TERM=dumb cmd` | no color, no cursor movement, no spinner |
| `cmd --color=always \| cat` | escapes present (force overrides the pipe) |

Add `cmd > file` and inspect for `\e[` when the tool writes files. Check the wedged-terminal case by
`Ctrl-C`-ing a progress bar and confirming the cursor is back.
