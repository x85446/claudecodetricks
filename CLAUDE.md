# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Claude Code marketplace providing session hooks for voice announcements, AI-powered logging, and automatic git commits. Written in Go, this repository implements three integrated binaries that enhance Claude Code workflow through the hooks system.

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
