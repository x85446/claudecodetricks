# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Claude Code marketplace providing session hooks for voice announcements, AI-powered logging, and automatic git commits. Written in Go, this repository implements three integrated binaries that enhance Claude Code workflow through the hooks system.

## Skills

This repo is also the canonical backup/source for globally-installed Claude Code skills (deployed via `skills/skillinstall.sh`). Two work together as the iterate stack:

- **`/tutorial`** (`skills/tutorial/`) — builds and maintains self-running bash tutorials that live in the codebase at `docs/tutorials/` (fixed location, every repo): menu-driven buckets of 5-10 min, each step showing the real command pre-filled and editable, running it on Enter. Also `list`/`update`/`audit`/`reorganize`/`delete` as the code drifts. Runtime library in `skills/tutorial/lib/` is copied into projects, never rewritten per project.
- **`/uxmaster`** (`skills/uxmaster*/`) — UXMASTER, the UX/UI design meta: children are `analysis` (platform-agnostic audit), five platform experts (`macos`, `linux`, `windows`, `web`, `cli`), and `implement` (writes the real framework code — SwiftUI, GTK4/libadwaita, WinUI 3, web, TUI). Shared findings ledger at `./.claude/uxmaster/findings.md`; FFIV's Find step routes here for UI scopes. `cli` is the command-line authority and carries two references the implementer codes against: `skills/uxmaster-cli/grammar.md` (the house grammar `<tool> <noun> <verb> … [operands] [--flags anywhere]`, flag vocabulary, help layout, error shape, exit codes, session flow) and `skills/uxmaster-cli/color.md` (the color detection ladder, ANSI-16 semantic roles, contrast, and the six-invocation verification matrix). Flags parse in every position — a parser that can't (Go stdlib `flag`, shell `getopts`) is itself a finding.
- **`/testmaster`** (`skills/testmaster*/`) — TESTMASTER, the SQA suite meta with children adopt/derive/catalog/maintain/prune/run/report. `adopt` is the one-time onboarding pass every project runs first: it discovers an existing suite, seeds `catalog.json`, and computes each test's `covers` from real per-test coverage profiles (`covers_source: coverage | convention | manual` — a convention guess is never reported as measured). Nothing else in TESTMASTER means anything until it runs: drift is `git diff ∩ covers`, so an unadopted project reports zero drift and zero impact forever. `derive` turns a requirement in your own words into the cases it implies (negative, every-path, restore-state, interrupted); `catalog` is the organizing index — requirement → cases → covered code — and recomputes validity (valid/drifted/orphaned/unverified) as the code changes, so a green-but-drifted suite can't read as trustworthy. Real-world-measured timing registry at `./.claude/testmaster/registry.json` drives tiers (fast ≤10s, standard ≤2min, slow >2min — slow is nightly-only, never mid-plan); `testmaster-report` renders a self-contained HTML report card.
- **`/product-docs`** (`skills/product-docs/`) — keeps end-user product documentation true each iteration: adds new features' operating instructions, updates changed behavior, deletes removed features' docs.
- Every iterate plan ends with three standing finisher steps appended by the planner: `/dev-makefiles` maintenance, `/testmaster` run (fast+standard), then `/product-docs` sync. The planner also scans every plan for testable behavior and routes it through `/testmaster-derive` before writing validations.
- **`/iterate-notes`** (`skills/iterate-notes/`) — the notepad: "take a note" captures an idea for the next plan as one synthesized line and acks in one line; a stated decision lands in `## Decisions`. Two sections only, no discussion mode and no research appendix. Notes live at `./.claude/iterate/notes/`; "turn these notes into a plan" hands off to the planner's notes-to-plan op.
- **`/iterate-brainstorm`** (`skills/iterate-brainstorm/`) — the decision stage: investigates the project, its current implementation, and its toolsets, then presents 3 label-locked options (comparison table first, then a ~150-word paragraph each covering what it is, how to implement it, pros, cons) with one marked `★ Recommended`. The user interrogates and chooses; on request it emits `**Summary N**` (monotonic, never reused) which the user hands to `/ip` ("absorb the last summary" / "absorb summary 2"). Chat-only — writes no files, no notes, no plans, no branches.
- **`/iterate-planner`** (`skills/iterate-planner/`) — formalizes a task into a saved, oracle-aware plan (paired Step/Validation, optionally teamed for parallel execution). Plans, never executes. `/ip` (`skills/ip/`) is a pure alias for it.
- **`/iterate-rules`** (`skills/iterate-rules/`) — the gate: says when a run may *start*, in plain language ("don't run before 10pm", "weeknights only", "require a keyword"). Writes `./.claude/iterate/policy.md`, which `/iterate` enforces at launch. Two keys: `require-launch-keyword: <word>` (lock) and `launch-schedule:` — a cron-like list of `<allow|deny> [days] [HH:MM-HH:MM] [dates]` rules where deny beats allow and any allow makes it default-deny, so a policy edit can only ever narrow when runs happen. Two semantics that are easy to get wrong and are written into both skills: a time window **wraps midnight** when start > end, and a **day label matches the day the window opened**, so `allow mon-fri 22:00-06:00` includes Saturday 02:00. Rules gate starting only — never a run already in flight, which is why the gate sits below `/iterate`'s resume rule. newcorder is the first project with one.
- **`/iterate-triage`** (`skills/iterate-triage/`) — walk up to a stale terminal and get one short answer. Reads real state (plans, branch, uncommitted work, blockers) and returns a verdict, a numbered list of what needs a human, and the shortest path back to main. **Reaching triage is itself the finding**: `/iterate` is supposed to land an all-green plan on main by itself, so a feature branch sitting idle means automation failed — triage names what. Commits loose work immediately and without asking (a branch commit is not a merge, carries no risk, and the auto-commit hook does NOT cover it — snapshots never move HEAD); finishes an *owed* merge when the plan is genuinely all-green, because that is an obligation, not a decision; never merges anything that is not.
- **`/iterate-conductor`** (`skills/iterate-conductor/`) — the conductor: works the whole queue unattended. `/iterate` runs *a* plan; the conductor runs *the queue* — picks the next plan, lets `/iterate` drive it, handles the ending, sweeps again. Blockers get an escalation ladder (a **different approach**, never more retries — the 5-cycle cap stays, because an unattended loop is what ticked a dead plan for 13 hours) plus a per-plan wall-clock box so one stubborn plan can't eat the night. What genuinely needs a human moves to **one shared blocked plan**, and repeated blocker classes are batched — N unmergeable branches become one merge step, not N. Imports all open GitHub/GitLab issues as a single plan (capped per sweep, never re-imported, read-only on the forge). Verbs: `start|stop|pause|resume|run|status|kill` — `stop` and `pause` let the current plan finish, `kill` is the only one that interrupts. State at `./.claude/iterate/conductor.md`, which also holds an optional `conductor-schedule:` — same grammar as the launch schedule but **intersected** with it, so it narrows the conductor's own hours without ever widening past what `/iterate-rules` allows. `/ic schedule <rule>` delegates to `/iterate-rules`; the conductor reads `policy.md` and never writes it. Starting it is the standing authorization the launch keyword asks for; the launch *schedule* still governs when.
- **`/iterate`** (`skills/iterate/`) — executes a saved plan autonomously to completion, looping via `/loop` until every validation passes or it hits a genuine blocker. Dispatches one subagent per team on teamed plans.

Plans name their feature branch but do not create it — the branch is born at execution, so the status line keeps reading `main ✔` until real work starts, and the statusline's `⚙️` segment (one letter per plan: green executing, yellow planned, red blocked, dim closed) carries plan state instead of the branch name doing it badly.

Aliases: `/ip` → iterate-planner, `/i` → iterate, `/in` → iterate-notes, `/ibs` → iterate-brainstorm, `/ic` → iterate-conductor (all in `skills/`). `/ip` and `/in` delegate via the Skill tool; `/i`, `/ibs` and `/ic` cannot (their targets' flags block the Skill tool), so they read and follow the target's SKILL.md directly — the user typing `/i`, `/ibs` or `/ic` is the explicit invocation the flag reserves.

`/iterate`, `/iterate-brainstorm`, `/iterate-conductor`, `/i`, `/in`, `/ip`, `/ibs`, and `/ic` carry `disable-model-invocation: true`. For `/iterate` it is side effects (autonomous execution, PR merges); for `/iterate-brainstorm` it is that a decision session is something the user opens deliberately, never something natural language trips into; for `/iterate-conductor` it is the same side effects as `/iterate` but across the whole queue, unattended. `/iterate-planner` deliberately does NOT carry the flag: `/ip` delegates to it via the Skill tool, and that flag blocks Skill-tool delegation entirely. Plan state lives at `./.claude/iterate/plans/<name>.md` in whichever project they're run from; live status/dashboard for that state is `iterate-run` (`src/cmd/iterate-run/`, this repo's own Go binary — `iterate-run serve` for the web dashboard, `iterate-run status`/`timeline` for the CLI).

## Skills management (`skills/skillctl`)

`skillctl` is the agent-facing tool for skills. Machine-first output: silent on
success, one tab-separated fact per line, no colour, no banners. A whole-system
health check is one line and ~40 characters.

```bash
skills/skillctl status [-v]     # skills N stale N broken N no-source N
skills/skillctl sync [--apply]  # install everything stale
skills/skillctl install <name>… # install to mapped targets
skills/skillctl where <name>…   # targets + owner
skills/skillctl why <name>…     # source, ownership,each target and its path
skills/skillctl audit [-v]      # registry audit, one line
skills/skillctl targets         # symbolic target -> path
```

**`skills/skillmap.tsv` is the single source of truth** for "installed where and
why" — `name`, `targets`, `owner`. It replaced four sources that could and did
disagree: `skillinstall.sh`'s case statement, the Rust TUI's `skill-mappings.toml`
(both since removed), per-entry `.origin` files, and `external-sources.conf`. `skillinstall.sh` is now a thin
human-facing shim over `skillctl` rather than a second implementation.

`owner` is `self` when this repo authors the skill, or the path of the repo that
does — those are **mirrored** here and must be edited at the owner (izmachine).
`targets` of `-` means backup-only: registered so it is backed up, never
deployed. That is the normal state for most adopted skills, not a problem.

**`sync` refuses to push over a target that is newer than its source** unless
`--force`. Installing is a one-way push, so a newer target means someone edited
the installed copy directly — pushing would destroy that edit silently. oracle
is in exactly that state today.

## Codex skill mirror

Every globally-installed Claude Code skill is mirrored to Codex format under
`~/.agents/skills/`, generated from `skills/` into `codex-skills/` by
`/skill-2-codex`. The mirror is one-directional (Claude Code → Codex) and fully
regenerated, never diffed back.

```bash
skills/skill-2-codex/scripts/sync-all.sh --install --prune   # port every global skill
skills/skill-2-codex/scripts/install-daily.sh --status       # daily job state + last run
```

A launchd LaunchAgent (`com.x85446.codex-skill-sync`) runs the sync nightly at
03:15; logs land in `~/.claude/log/codex-sync/`, with a one-line `status.txt`.
Steady state is a 2-second no-op.

Two rules matter when editing a ported skill:

- **`.portstamp` protects judgment work.** A port marked `manual=true` (the
  whole iterate family) is never regenerated when its Claude source changes —
  the run reports it instead. `--force <skill>` is the deliberate override.
- **Codex's manifest budget is 8,000 chars, half Claude Code's.** Only
  implicitly-invocable skills spend it, and in Codex disabling implicit
  invocation does *not* block delegation — so children of a meta are folded to
  explicit-only for free (`FOLD_CHILDREN_OF` in `sync-all.sh`).
- **The description can never move to another file** — it is Codex's only
  routing signal. `scripts/diet.py` instead relocates the non-routing prose in a
  description (what the skill does, where it stores state) into a
  `## What this skill does` body section, which loads on trigger. Trigger
  phrases and negative scope always stay. Runs automatically at
  `MANIFEST_BUDGET=7600`.

## Build & Development Commands

### Core Commands
```bash
# Build all binaries (default target)
make build

# Clean and rebuild
make rebuild

# Remove built binaries
make clean

# Show available make targets
make help
```

### Testing Commands
```bash
# Run all tests (unit + hook tests)
make test

# Run only Go unit tests
make test-unit

# Run only hook integration tests
make test-hook

# Generate coverage report
make coverage

# Generate HTML coverage report and open in browser
make coverage-html
```

### Quality Checks
```bash
# Format all Go source files
make fmt

# Check if files are formatted (CI-friendly)
make fmt-check

# Run go vet
make vet

# Run golangci-lint (if installed)
make lint

# Run all quality checks (fmt-check, vet, test-unit)
make check
```

### Dependency Management
```bash
# Install and tidy Go dependencies
make deps

# Verify dependencies
make deps-check

# Update all dependencies to latest versions
make deps-update
```

### Installation
```bash
# Install hooks to ~/.claude/hooks/
make install

# Uninstall hooks
make uninstall
```

### Development Tools
```bash
# Watch for changes and rebuild (requires fswatch or inotifywait)
make watch

# Display version information
make version

# Display build configuration and status
make info

# Verbose mode (show all commands)
make V=1 build
```

### Building Individual Binaries
The Makefile automatically tracks dependencies and rebuilds only when necessary:
```bash
make plugins/session-hooks/hooks/voice-announcer
make plugins/session-hooks/hooks/session-logger
make plugins/session-hooks/hooks/git-committer
```

## Architecture

### Hook System Integration

All three binaries follow the Claude Code hooks protocol:
- **Input**: JSON via stdin conforming to `hooks.HookInput` structure (src/pkg/hooks/types.go:4)
- **Output**: Stderr for user-visible messages, exit code 0 always
- **Event Types**: Notification, UserPromptSubmit, PostToolUse, Stop

### Package Structure

```
src/
├── cmd/                          # Binary entry points (main.go files)
│   ├── voice-announcer/          # TTS event announcements
│   ├── session-logger/           # AI-powered session logging
│   └── git-committer/            # Auto-commit with Conventional Commits
├── internal/                     # Internal implementation packages
│   ├── claude/                   # Claude API client for summarization
│   │   ├── client.go            # HTTP client for Messages API (Haiku)
│   │   └── summarizer.go        # Transcript parsing & summarization
│   ├── git/                     # Git operations wrapper
│   │   ├── commit.go            # Core git commands (stage, commit, status)
│   │   └── conventional.go      # Conventional Commits logic
│   └── voice/                   # Voice synthesis integration
└── pkg/
    └── hooks/                   # Shared types for hook protocol
        └── types.go             # HookInput, TranscriptEntry structs
```

### Component Behavior

**voice-announcer** (src/cmd/voice-announcer/main.go)
- Listens for: Notification, UserPromptSubmit, Stop events
- Generates context-aware messages based on event type (src/cmd/voice-announcer/main.go:47)
- Calls external `kokoroSay.sh` script for TTS via src/internal/voice/kokoro.go
- Non-blocking: continues if TTS unavailable

**session-logger** (src/cmd/session-logger/main.go)
- Listens for: Stop events only (main.go:31)
- Generates log entries with timestamp, project name, and summary (main.go:58-59)
- Parses JSONL transcript files to extract tool calls via src/internal/claude/summarizer.go
- Uses Claude 3.5 Haiku API to generate 8-word Conventional Commits summaries
- Writes to `~/.claude/log/cAudit-YYYY-MM-DD.log` (main.go:52)
- Falls back to basic logging if API key missing or transcript unavailable (main.go:75-89)

**git-committer** (src/cmd/git-committer/main.go)
- Listens for: PostToolUse (Write/Edit tools), Stop events
- **PostToolUse**: Auto-commits immediately after Write/Edit operations
  - Skips if not in git repo, file in gitignore, or in plan mode
  - Generates semantic commit messages via src/internal/git/conventional.go:23
  - Truncates messages to 50 chars
- **Stop**: Prompts user to commit any remaining uncommitted changes
  - Interactive y/n prompt on stderr

### Conventional Commits Logic

The git package implements automatic commit type detection (src/internal/git/conventional.go):

**Commit Types**:
- `feat`: Write tool (new files)
- `fix`: Edit tool with bug fix keywords
- `docs`: .md, .rst, README files
- `test`: Files/dirs containing "test" or "spec"
- `chore`: Config files (package.json, go.mod, Makefile, etc.)
- `refactor`: Edits with significant size changes
- `style`: Formatting changes
- `perf`: Performance improvements

**Scope Detection** (src/internal/git/conventional.go:78):
- Uses last directory name in path
- Falls back to filename without extension
- Omitted if scope > 20 chars

**Message Format**: `<type>(<scope>): <subject>` where subject uses imperative mood (add, update, modify).

### Claude API Integration

The session-logger uses Claude API for transcript summarization (src/internal/claude/):
- Model: `claude-3-5-haiku-20241022` (src/internal/claude/client.go:15)
- Max tokens: 30
- Endpoint: `https://api.anthropic.com/v1/messages`
- Cost: ~$0.005 per summary
- Requires: `ANTHROPIC_API_KEY` environment variable

**Transcript Processing** (src/internal/claude/summarizer.go:50):
- Parses JSONL line-by-line
- Extracts significant tool calls (Write, Edit, Read, Bash)
- Limits to last 20 tool calls to avoid large prompts
- Prompt enforces exactly 8 words, Conventional Commits format

## Configuration

**Required for session-logger**:
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

**Required for voice-announcer**:
- `kokoroSay.sh` must be in PATH

## Testing Hooks Manually

```bash
# Voice announcer
echo '{"hook_event_name":"Stop","cwd":"/Users/travis/.claude"}' | \
  plugins/session-hooks/hooks/voice-announcer

# Session logger with transcript
echo '{"hook_event_name":"Stop","transcript_path":"/path/to/transcript.jsonl","cwd":"/Users/travis/.claude"}' | \
  plugins/session-hooks/hooks/session-logger

# Git committer PostToolUse event
echo '{"hook_event_name":"PostToolUse","tool_name":"Write","cwd":"'$(pwd)'","tool_input":{"file_path":"test.txt"},"permission_mode":"default"}' | \
  plugins/session-hooks/hooks/git-committer

# Git committer Stop event (requires git repo with uncommitted changes)
echo '{"hook_event_name":"Stop","cwd":"'$(pwd)'"}' | \
  plugins/session-hooks/hooks/git-committer
```

## Marketplace Integration

### Plugin Architecture

This repository follows Claude Code's marketplace plugin pattern, similar to [wshobson/agents](https://github.com/wshobson/agents):

**Directory Structure:**
```
claudecodetricks/
├── .claude-plugin/
│   └── marketplace.json          # Central marketplace registry
├── plugins/
│   └── session-hooks/            # Single plugin bundling 3 hooks
│       └── hooks/                # Hook executables
│           ├── voice-announcer
│           ├── session-logger
│           └── git-committer
├── src/                          # Go source code
└── Makefile
```

### Marketplace Definition

The marketplace is defined in `.claude-plugin/marketplace.json` (.claude-plugin/marketplace.json:1):

**Key Fields:**
- `source`: `"./plugins/session-hooks"` - Points to plugin directory (marketplace.json:15)
- `hooks`: Relative paths to executables from plugin root (marketplace.json:35-39)
- `category`: `"productivity"` - Plugin categorization (marketplace.json:33)
- `strict`: `false` - Marketplace entry serves as complete manifest (marketplace.json:34)

**Hook Paths:**
Paths in the `hooks` array are relative to the plugin's `source` directory:
```json
"source": "./plugins/session-hooks",
"hooks": [
  "./hooks/voice-announcer",      // Resolves to: ./plugins/session-hooks/hooks/voice-announcer
  "./hooks/session-logger",
  "./hooks/git-committer"
]
```

### Installation & Usage

**Add Marketplace:**
```bash
# Via Claude Code CLI
/plugin marketplace add /Users/travis/workspace/x85446/claudecodetricks

# Or manually edit ~/.claude/plugins/known_marketplaces.json:
{
  "claudecodetricks": {
    "source": {
      "source": "local",
      "path": "/Users/travis/workspace/x85446/claudecodetricks"
    },
    "installLocation": "/Users/travis/workspace/x85446/claudecodetricks",
    "lastUpdated": "2025-10-17T00:48:00.000Z"
  }
}
```

**Install Plugin:**
```bash
/plugin install session-hooks@claudecodetricks
```

**Enable/Disable:**
```bash
/plugin enable session-hooks@claudecodetricks
/plugin disable session-hooks@claudecodetricks
```

**Installation Locations:**
- Marketplace registry: `~/.claude/plugins/known_marketplaces.json`
- Plugin enablement: `~/.claude/settings.json` → `enabledPlugins.session-hooks@claudecodetricks`
- Session logs: `~/.claude/log/cAudit-*.log`
- Debug output: `/tmp/claudelog/` (stderr from hooks)

### Bundle Architecture

The `session-hooks` plugin uses a **bundled approach**:
- All three hooks install together as one unit
- Shared Go codebase in `src/` directory
- Common build infrastructure (Makefile)
- Unified versioning and releases

**Alternative Pattern:**
The [wshobson/agents](https://github.com/wshobson/agents) repository demonstrates a **single-purpose plugin** pattern where each plugin contains one focused capability. For this repository, that would mean three separate plugins (voice-announcer, session-logger, git-committer) with individual marketplace entries. The current bundled approach prioritizes simplicity and shared infrastructure.

## Key Implementation Details

### Makefile Features
The Makefile includes:
- **Automatic versioning**: Embeds git commit, tag, and build time into binaries via `-ldflags`
- **Colored output**: Green for success, yellow for warnings, blue for info, cyan for version details
- **Verbose mode**: `V=1` flag shows all executed commands
- **Dependency tracking**: Rebuilds only when source files change
- **Tool checking**: Verifies required tools (Go, gofmt, etc.) are installed
- **Graceful degradation**: Lints only if golangci-lint is installed
- **Coverage reporting**: Generates both text and HTML coverage reports
- **Watch mode**: Auto-rebuild on file changes (requires fswatch/inotifywait)
- **Comprehensive help**: Self-documenting via `##` comments

Version information is embedded at build time:
```bash
LDFLAGS += -X main.Version=$(VERSION)
LDFLAGS += -X main.Commit=$(GIT_COMMIT)
LDFLAGS += -X main.BuildTime=$(BUILD_TIME)
```

### Go Module
- Module path: `github.com/x85446/claudecodetricks`
- Go version: 1.25.3
- No external dependencies (uses only stdlib)

### Build Flags
- `-ldflags="-s -w"`: Strip debug info and symbol table for smaller binaries
- `-trimpath`: Remove absolute paths from binaries for reproducible builds
- Race detector enabled in tests: `go test -race`

### Error Handling Philosophy
All hooks exit with code 0 regardless of errors to avoid blocking Claude Code operation. Errors are logged to stderr for debugging but do not interrupt workflow.

### Git Operations
All git commands are executed via `os/exec` package (src/internal/git/commit.go). The `git-committer` uses:
- `git rev-parse --git-dir`: Check if in git repo (commit.go:14)
- `git status --porcelain`: Get modified files (commit.go:21, commit.go:59)
- `git add <files>`: Stage files (commit.go:33)
- `git commit -m <message>`: Create commit (commit.go:46)
- `git check-ignore <file>`: Check gitignore status (commit.go:85)
- `git rev-parse --show-toplevel`: Get repo root (commit.go:92)

The `PromptUser` function (commit.go:119) provides interactive y/n prompts on stderr for the Stop event handler.

### Transcript Format
Claude Code transcripts are JSONL files where each line is a `TranscriptEntry` (src/pkg/hooks/types.go:16). Tool use appears in `content` blocks with `type: "tool_use"` and nested `ToolUseBlock` containing tool name and input parameters.
