# Claude Code Tricks

A Claude Code marketplace providing:
- **Agent Teams**: Pre-configured multi-agent teams for product development
- **Session Hooks**: Voice announcements, AI-powered logging, and automatic git commits

## Plugins

### new-product-team

A 41-agent product development team for greenfield projects. Includes managers and specialists across 8 domains:

| Domain | Agents |
|--------|--------|
| Architecture | Manager, Data Architect, Infrastructure Architect, Security Architect, Solutions Architect |
| Backend | Manager, API Engineer, Database Engineer, Integration Engineer, Performance Engineer, Security Engineer |
| Frontend | Manager, React Developer, UI Engineer, Accessibility Specialist, Performance Engineer, Testing Specialist |
| Marketing | Manager, Brand Strategist, Content Specialist, SEO Specialist, Social Media Specialist, Analytics Specialist |
| Operations | Manager, CI/CD Engineer, Infrastructure Engineer, Platform Engineer, SRE Engineer |
| Orchestrator | Project Manager, Build Engineer, Documentation Engineer |
| Product | Manager, Analyst, Designer, Technical Manager, UX Researcher |
| Quality | Manager, Analyst, Test Automation Engineer, Performance Test Engineer, Security Test Engineer |

Each agent has specialized expertise for new product development workflows.

### session-hooks

Three integrated hook binaries that enhance your Claude Code workflow:

#### 1. voice-announcer
Provides voice announcements for key Claude Code events using text-to-speech.

**Events:**
- **Notification**: Announces when Claude has a question
- **UserPromptSubmit**: Announces when you've submitted a prompt
- **Stop**: Announces when the session stops

**Requirements:**
- `kokoroSay.sh` script in your PATH for voice synthesis

#### 2. session-logger
Logs session summaries to `~/.claude/log/cAudit-YYYY-MM-DD.log` with AI-generated 8-word summaries.

**Log Format:**
```
2025-10-17 00:47:52 projectname feat: add user authentication
```

**Features:**
- Parses JSONL transcripts
- Uses Claude 3.5 Haiku for concise summaries
- Follows Conventional Commits format
- Falls back to basic logging if transcript unavailable

**Requirements:**
- `ANTHROPIC_API_KEY` environment variable (~$0.005 per summary)

#### 3. git-committer
Automatically creates git commits with Conventional Commits messages.

**Behavior:**
- **PostToolUse**: Auto-commits on `Write` and `Edit` tool usage
- **Stop**: Prompts user to commit any remaining uncommitted changes
- Skips files in `.gitignore`
- Skips when in plan mode
- Generates semantic commit messages based on file type and changes

**Commit Message Format:**
```
<type>(<scope>): <subject>
```

**Types:**
- `feat`: New features (Write tool)
- `fix`: Bug fixes (Edit tool with fix keywords)
- `docs`: Documentation (.md, .rst files)
- `test`: Test files
- `chore`: Config files, misc changes
- `refactor`: Code restructuring
- `style`: Formatting changes
- `perf`: Performance improvements

## Installation

### 1. Clone and Build

```bash
cd ~/workspace/x85446
git clone https://github.com/x85446/claudecodetricks.git
cd claudecodetricks

# Install Go dependencies and build all binaries
make deps
make build

# Test the hooks
make test
```

### 2. Register the Marketplace

Before installing plugins, you must register this marketplace with Claude Code.

#### Option A: Manual Registration (Recommended)

Edit `~/.claude/plugins/known_marketplaces.json` and add the `claudecodetricks` entry:

```json
{
  "claudecodetricks": {
    "source": {
      "source": "local",
      "path": "/Users/travis/workspace/x85446/claudecodetricks"
    },
    "installLocation": "/Users/travis/workspace/x85446/claudecodetricks",
    "lastUpdated": "2025-12-15T00:00:00.000Z"
  }
}
```

#### Option B: CLI Registration

```bash
/plugin marketplace add /Users/travis/workspace/x85446/claudecodetricks
```

### 3. Install Plugins

Once the marketplace is registered, install the plugins you want:

#### Option A: CLI Installation

```bash
# Install the agent team
/plugin install new-product-team@claudecodetricks

# Install session hooks
/plugin install session-hooks@claudecodetricks

# Enable/disable plugins
/plugin enable new-product-team@claudecodetricks
/plugin disable session-hooks@claudecodetricks
```

#### Option B: Manual Installation

Edit `~/.claude/settings.json` and add to the `enabledPlugins` section:

```json
{
  "enabledPlugins": {
    "new-product-team@claudecodetricks": true,
    "session-hooks@claudecodetricks": true
  }
}
```

### 4. Configure (for session-hooks)

**Required:**
```bash
# For session-logger AI summaries
export ANTHROPIC_API_KEY="your-api-key-here"
```

**Optional:**
```bash
# For voice-announcer (if not already in PATH)
export PATH="$PATH:/path/to/kokoroSay.sh"
```

### 5. Restart Claude Code

Restart your Claude Code session to load the new hooks.

## Usage

Once installed, the hooks run automatically:

- **Voice announcements** play on Notification, UserPromptSubmit, and Stop events
- **Session logs** are written to `~/.claude/log/cAudit-YYYY-MM-DD.log` on Stop
- **Git commits** are created automatically when you Write or Edit files in git repos

### Manual Git Commit

On Stop, if you have uncommitted changes, git-committer will prompt:
```
You have uncommitted changes.
Suggested commit:
  feat: update component.js

Create commit now? (y/N)
```

## Development

### Directory Structure

```
claudecodetricks/
├── .claude-plugin/
│   └── marketplace.json          # Marketplace definition
├── plugins/
│   ├── new-product-team/         # 41-agent team plugin
│   │   ├── agents/               # Agent definition files
│   │   │   ├── Architecture-*.md
│   │   │   ├── Backend-*.md
│   │   │   ├── Frontend-*.md
│   │   │   ├── Marketing-*.md
│   │   │   ├── Operations-*.md
│   │   │   ├── Orchestrator-*.md
│   │   │   ├── Product-*.md
│   │   │   └── Quality-*.md
│   │   └── team.md               # Team documentation
│   └── session-hooks/
│       └── hooks/                # Built binaries (output)
│           ├── voice-announcer
│           ├── session-logger
│           └── git-committer
├── src/
│   ├── cmd/                      # Binary main packages
│   │   ├── voice-announcer/
│   │   ├── session-logger/
│   │   └── git-committer/
│   ├── internal/                 # Internal packages
│   │   ├── claude/              # Claude API client
│   │   ├── git/                 # Git operations
│   │   └── voice/               # Voice announcements
│   └── pkg/
│       └── hooks/               # Shared types
├── teams/
│   └── agent2clip.sh             # Utility: extract agents to clipboard
├── Makefile
└── README.md
```

### Building

```bash
# Build all binaries
make build

# Build individual binaries
make plugins/session-hooks/hooks/voice-announcer
make plugins/session-hooks/hooks/session-logger
make plugins/session-hooks/hooks/git-committer

# Clean built binaries
make clean

# Update dependencies
make deps
```

### Testing Hooks

Test hooks individually with sample JSON:

```bash
# Test voice-announcer
echo '{"hook_event_name":"Stop","cwd":"/Users/travis/.claude"}' | \
  plugins/session-hooks/hooks/voice-announcer

# Test session-logger
echo '{"hook_event_name":"Stop","transcript_path":"","cwd":"/Users/travis/.claude"}' | \
  plugins/session-hooks/hooks/session-logger

# Test git-committer (requires git repo)
echo '{"hook_event_name":"PostToolUse","tool_name":"Write","cwd":"'$(pwd)'","tool_input":{"file_path":"test.txt"},"permission_mode":"default"}' | \
  plugins/session-hooks/hooks/git-committer
```

## Conventional Commits Reference

Format: `<type>(<scope>): <subject>`

**Subject Rules:**
- ≤ 50 characters
- Imperative mood ("add" not "added")
- No period at end
- Lowercase

**Examples:**
```
feat(auth): add OAuth2 login flow
fix(parser): correct null pointer in validator
docs: update API documentation
test(auth): add login integration tests
chore: update dependencies
refactor(api): restructure endpoint handlers
```

## Troubleshooting

### Voice Announcements Not Working

1. Check if `kokoroSay.sh` is in PATH:
   ```bash
   which kokoroSay.sh
   ```
2. Check stderr logs at `/tmp/claudelog/`

### Session Logger Not Summarizing

1. Verify `ANTHROPIC_API_KEY` is set:
   ```bash
   echo $ANTHROPIC_API_KEY
   ```
2. Check API quota at https://console.anthropic.com
3. Check logs at `~/.claude/log/cAudit-*.log`

### Git Commits Not Working

1. Verify you're in a git repository:
   ```bash
   git status
   ```
2. Check that files aren't in `.gitignore`
3. Verify you're not in plan mode

### Hooks Not Receiving JSON

The Go binaries use proper stdin blocking, unlike bash hooks with timeouts. If hooks aren't receiving JSON:

1. Check Claude Code settings.json has the plugin enabled
2. Verify binary permissions: `chmod +x plugins/session-hooks/hooks/*`
3. Check stderr output: `/tmp/claudelog/`

## Credits

- Marketplace structure inspired by [davila7/claude-code-templates](https://github.com/davila7/claude-code-templates)
- Uses [Anthropic Claude API](https://www.anthropic.com/api) for AI summarization
- Follows [Conventional Commits](https://www.conventionalcommits.org/) specification

## License

MIT
