# Command grammar and program flow — the reference `/uxmaster-cli` and `/uxmaster-implement` both obey

## 1. The house grammar

```
<tool> <noun> <verb> [<noun> <verb> …] [operands…] [--flags anywhere]
```

Read it aloud: **the tool, the thing, the action.** `iz vm create web-01 --memory 4G`,
`filemaster intake review 2024-invoices --dry-run`, `iterate-run plan show finch --json`.

| Rule | Why |
|---|---|
| **Noun before verb**, at every level | The noun narrows the world; the verb acts inside it. `vm create` groups every vm action together in `--help`, where `create vm` scatters them under one giant `create`. |
| **Nest as deep as the domain is deep** — `tool a b c` is legal | Depth is grouping, not ceremony. Three levels is comfortable; a fourth means the second level is really a flag. |
| **Same verb, same meaning everywhere** | `list`, `show`, `create`, `delete`, `edit` never mean different things under different nouns. |
| **Flags appear anywhere** | See §2 — this is the house's defining rule. |
| **`--` ends flag parsing** | Everything after is an operand, however it starts. Non-negotiable for filenames beginning with `-`. |
| **No catch-all subcommand** | A bare `tool <thing>` that guesses the verb blocks every future noun of the same name. |
| **No prefix abbreviation** | `tool cr` resolving to `create` becomes a broken script the day `crypt` is added. Ship completions instead. |

Singular nouns (`vm`, not `vms`); the verb carries plurality (`vm list`). Keep the top level small — if the
root `--help` doesn't fit a screen, the nouns are wrong.

## 2. Options anywhere — the law

**Every flag parses in every position**: before the noun, between subcommands, after operands, at the end.
These are all the same command:

```
tool --verbose vm create web-01 --memory 4G
tool vm --verbose create --memory 4G web-01
tool vm create web-01 --memory 4G --verbose
```

Two categories, both position-free:

- **Global/persistent flags** (`--verbose`, `--json`, `--color`, `--config`, `--dry-run`) — declared once at
  the root, accepted at every level.
- **Local flags** (`--memory` on `vm create`) — accepted anywhere *within* the command that owns them,
  including after its operands. A local flag used under the wrong subcommand is a usage error naming the
  command that does own it.

This is GNU-style permutation, not strict POSIX. `POSIXLY_CORRECT` stops permutation in C `getopt(3)`;
the house style does not honor it — but `--` always does.

### Getting it per parser

| Parser | What to do |
|---|---|
| Go — `spf13/pflag` + `cobra` | Interspersed flags are already on (don't call `SetInterspersed(false)`). Declare cross-cutting flags with `PersistentFlags()` on the root. Set `TraverseChildren = true` when a parent's **local** flags must be honored before a subcommand. |
| Go — stdlib `flag` | **Cannot do this.** `flag.Parse()` stops at the first non-flag argument, so `tool vm create --memory 4G` never sees `--memory`. Moving to `pflag`/`cobra` is the fix; note it in the finding as a dependency change. |
| Rust — `clap` (derive) | Mark cross-cutting flags `#[arg(global = true)]` so they're accepted under every subcommand. Local flags already permute. Don't set `args_conflicts_with_subcommands` unless you mean to forbid it. |
| Python — `argparse` | Subparsers only see flags typed **after** the subcommand, and parent flags only **before** it. Fix by defining shared flags on a `parents=[common]` parser attached to every subparser, or by a two-pass `parse_known_args`. |
| Python — `click` | Options bind to the command that declares them. Put shared options on both the group and its commands via a shared decorator; `chain`/eager options cover the rest. |
| Node — `commander` | Program options are already found anywhere, including after subcommands, unless `enablePositionalOptions()` is set. |
| Bash | Loop with `while [ $# -gt 0 ]`, `case` on `-*`, collecting non-flags into a positional array, and `--` breaking out. Never `getopts` — it stops at the first operand. |

Whatever the parser, the acceptance test is the same: the three invocations above produce byte-identical
output.

## 3. Flag vocabulary — use the names the world already knows

| Flag | Meaning | Notes |
|---|---|---|
| `-h`, `--help` | help for this exact level | ignores every other flag; exits 0 |
| `--version` | version, one line | `-v` is **verbose** in most modern tools — don't overload |
| `-v`, `--verbose` | more diagnostics on stderr | repeatable (`-vv`) is a nice touch |
| `-q`, `--quiet` | errors only | exit code carries the outcome |
| `--json` | machine output on stdout | see §6 |
| `-n`, `--dry-run` | print what would happen, change nothing | required for anything destructive |
| `-f`, `--force` | skip the confirmation | never skips `--dry-run` |
| `-y`, `--yes` | pre-answer prompts affirmatively | |
| `--no-input` | fail rather than prompt | for CI |
| `-o`, `--output` | destination file, `-` for stdout | |
| `-C`, `--config` | config file path | |
| `--color=<auto\|always\|never>`, `--no-color` | see `color.md` | |
| `-a --all`, `-d --debug`, `-p --port`, `-u --user` | as everywhere else | |

Short flags only for the frequent ones — the single-letter namespace runs out fast and cannot be reclaimed.
Long flags are `--kebab-case`. Booleans get a `--no-` counterpart (`--cache` / `--no-cache`) so config can
be overridden back. **Secrets never come in on a flag** (`ps` shows the whole argv) — use `--x-file`, an env
var, or stdin.

## 4. Help text — the layout

```
<tool> <noun> <verb> — one line, imperative, what it does

Usage:
  tool vm create <name> [flags]

Examples:
  tool vm create web-01                     # smallest useful invocation
  tool vm create web-01 --memory 4G --gpu   # the common real one
  tool vm create web-01 --json | jq .id     # scripted

Flags:
  -m, --memory <size>    RAM to allocate (default 2G)
      --gpu              attach the default GPU
  -n, --dry-run          print the plan, create nothing

Global flags:
  -v, --verbose          …

See also: tool vm list, tool vm delete
Docs: https://…
```

- **Examples come before the flag wall.** People read the first screen and stop.
- Every flag shows its default; every flag whose value isn't obvious shows a placeholder (`<size>`).
- Bare `tool` and bare `tool vm` print short help and exit **0**; a *missing required argument* prints short
  help plus the specific error and exits **2**.
- Help goes to **stdout** when asked for, **stderr** when it's the consequence of an error.
- Group flags when there are more than ~8. Keep the order stable — people scan by position.

## 5. Errors — the three-part shape

Every error message names **what failed, why, and the next action**:

```
Error: cannot create vm "web-01": a vm with that name already exists

  Choose another name, or replace the existing one:
    tool vm delete web-01 && tool vm create web-01
    tool vm create web-01 --replace
```

- One error, one exit — collect and group repeats (`3 files failed: …`) rather than screaming per item.
- The actionable line goes **last**; that's where the eye lands.
- No stack traces by default. Put them behind `--debug`, or write them to a file and print the path.
- Typos in a fixed command set get `did you mean` (Levenshtein ≤ 2).
- Never blame the user ("invalid argument"): say which argument, what it received, what it accepts.
- Errors, warnings, and progress go to **stderr**; nothing else does.

## 6. Streams, exit codes, and machine mode

| Stream | Carries |
|---|---|
| stdout | the **result** — the thing another program would consume |
| stderr | diagnostics, progress, prompts, warnings, errors |

| Code | Meaning |
|---|---|
| 0 | success (and only success) |
| 1 | the operation failed |
| 2 | usage error — bad flag, missing argument, unknown subcommand |
| 3+ | distinct failure classes the caller can branch on — document each one |
| 130 | interrupted (SIGINT); leave the terminal restored |

Machine mode (`--json`) is a **contract**: no banners, no progress, no color, no human prose on stdout;
stable field names across versions; errors as JSON on stdout too (or documented as stderr text + exit code,
but pick one and keep it). One JSON object per invocation, or newline-delimited JSON for streams.

Reading: support `-` as "stdin"/"stdout" wherever a file path is accepted, and read stdin when it isn't a
TTY and no operand was given.

## 7. Configuration precedence — one order, documented

```
command-line flag  >  environment variable  >  project config  >  user config  >  built-in default
```

`tool config show --origin` (or `--verbose`) telling the user *where* a value came from turns a support
thread into a five-second check. Env vars are `<TOOL>_<FLAG_IN_CAPS>`; project config lives in the repo,
user config under `$XDG_CONFIG_HOME/<tool>/` (falling back to `~/.config/<tool>/`).

## 8. Interactivity — the TTY matrix

| Condition | Behavior |
|---|---|
| stdin not a TTY | **never prompt** — use the flag value, the config, or fail with the flag to set |
| `--no-input` | never prompt; fail naming the missing flag |
| destructive + TTY | confirm by default; `--force`/`--yes` skips |
| destructive + irreversible | confirm by typing the resource name, and `--force` alone is not enough |
| long operation + TTY | progress on stderr, cursor restored on exit and on SIGINT |
| long operation, no TTY | one line per state change, no spinner, no `\r` |
| password/secret prompt | echo off; never accept it as a flag |

Every prompt has a flag that pre-answers it — prompting is a convenience layered on a scriptable path, never
the only way in. `Ctrl-C` always works, at every stage, and leaves the system in a state the user can
describe.

## 9. Flow — the shape of a whole session

1. **One command, one job.** If a command's description needs "and", it's two commands.
2. **Progressive disclosure.** The smallest useful invocation is short; power lives behind flags, not in
   required arguments. Defaults are what most people want, not what's easiest to implement.
3. **Discoverable next step.** Output ends by naming what a user typically does next (`Created web-01. Start
   it with: tool vm start web-01`) — suppressed under `--quiet` and `--json`.
4. **Idempotent where it can be.** Re-running doesn't double-apply; already-in-the-desired-state exits 0
   with "already exists", not an error — unless the user asked to create exactly once.
5. **Resumable when long.** A multi-step operation that dies at step 7 says which step, and how to resume or
   roll back. Never leave half-applied state without saying so.
6. **Dry run before damage.** `--dry-run` prints the exact plan in the same shape the real run will report.
7. **Confirm proportionally.** Trivial: no prompt. Reversible: prompt. Irreversible: type the name.
8. **First run is a flow of its own.** No config file is not an error — either a sane default or one clear
   instruction. Auth failures say exactly which command re-authenticates.
9. **Time is feedback.** Anything over ~1s gets progress; over ~10s gets an ETA or a count; anything the
   user might want to walk away from prints a completion line.
10. **Composable by construction.** Result on stdout, one record per line where it makes sense, no headers
    in machine mode — so `|`, `xargs`, `jq`, and `grep` work without flags.

## 10. Completions, pagination, and TUIs

- **Ship shell completions** (`tool completion bash|zsh|fish`) — they replace prefix abbreviation and make
  deep noun/verb nesting free to navigate.
- **Page long human output** only when stdout is a TTY, honoring `$PAGER` and defaulting to `less -FRX`
  (`-F` = don't page short output, `-R` = keep color, `-X` = don't clear the screen). Never page machine
  output.
- **TUI**: `q`/`Esc` exits, `?` lists keys, arrows **and** vim keys navigate, `Ctrl-C` always works, resize
  reflows, and the terminal is restored on every exit path — including panic. Every TUI action must also
  exist as a non-interactive command; a TUI is a view over the CLI, never a replacement for it.

## 11. Changing an existing CLI

Flags and subcommands are an API. When a change breaks one:

- Keep the old spelling working, print a deprecation warning **to stderr** naming the replacement, and give
  a removal version.
- Never silently change what a flag means — rename instead, so old scripts fail loudly rather than doing the
  wrong thing quietly.
- Adding a new flag or subcommand is safe; changing a default is a behavior break and gets the same
  treatment as a rename.
