---
name: tui-runner
description: Use when the user invokes /tui-runner to install, configure, or manage the terminal-mcp MCP server for headless terminal emulation in Claude Code.
disable-model-invocation: true
argument-hint: [install|status|reinstall]
---

## What This Skill Does

Installs and configures the `terminal-mcp` MCP server so Claude Code can interact with a headless terminal emulator. Handles Node.js installation, cross-platform detection (macOS/Linux), source builds from the x85446 fork, and idempotent MCP registration using absolute paths.

Source: `git@github.com:x85446/terminal-mcp.git`

## Steps

### Step 1: Detect OS

Run `uname -s` to determine the platform:
- `Darwin` → macOS
- `Linux` → Linux

Store the result for package manager selection in later steps.

### Step 2: Check and install Node.js

Check if Node.js 18+ is available:

```bash
node --version 2>/dev/null
```

If **node is missing or version < 18**:

**macOS:**
```bash
which brew && brew install node
```

**Linux:**
```bash
if [ -d "$HOME/.nvm" ]; then
  export NVM_DIR="$HOME/.nvm"
  [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"
  nvm install 18
  nvm use 18
else
  curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash
  export NVM_DIR="$HOME/.nvm"
  [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"
  nvm install 18
  nvm use 18
fi
```

After install, verify:
```bash
node --version
npm --version
```

If install fails, stop and report the error. Do not continue without Node.js 18+.

### Step 3: Check and install terminal-mcp from source

The install location is `~/.local/share/terminal-mcp/`. Check if it's already built:

```bash
ls ~/.local/share/terminal-mcp/dist/index.js 2>/dev/null
```

If **not installed** (or if user requested `reinstall`):

```bash
mkdir -p ~/.local/share
git clone git@github.com:x85446/terminal-mcp.git ~/.local/share/terminal-mcp
cd ~/.local/share/terminal-mcp
npm install
npm run build
```

If the repo already exists and user requested `reinstall`:
```bash
cd ~/.local/share/terminal-mcp
git pull
npm install
npm run build
```

Verify the build produced the entry point:
```bash
ls -la ~/.local/share/terminal-mcp/dist/index.js
```

### Step 4: Resolve absolute paths

This is critical — Claude Code fails to spawn MCPs when it can't resolve paths. Get absolute paths:

```bash
NODE_PATH=$(which node)
echo "Node: $NODE_PATH"

MCP_ENTRY="$HOME/.local/share/terminal-mcp/dist/index.js"
echo "MCP entry: $MCP_ENTRY"
```

If `which node` returns nothing, check common locations:
- `/usr/bin/node`
- `/usr/local/bin/node`
- `$HOME/.nvm/versions/node/*/bin/node`
- `/opt/homebrew/bin/node`

Both paths must be absolute. Store them for Step 6.

### Step 5: Check if MCP is already registered

Check for an existing `terminal` MCP configuration:

```bash
# Check project .mcp.json
cat .mcp.json 2>/dev/null | grep -q '"terminal"'

# Check project-level settings
cat .claude/settings.local.json 2>/dev/null | grep -q '"terminal"'

# Check user-level settings
cat ~/.claude/settings.json 2>/dev/null | grep -q '"terminal"'
```

If **already registered** and user did NOT request `reinstall`:
- Report: "terminal-mcp is already configured. Use `/tui-runner reinstall` to reconfigure."
- Skip to Step 7 (verify).

### Step 6: Register the MCP with Claude Code

**Critical: Use absolute path to `node` as the command, and the absolute path to `dist/index.js` as the first arg.** Do NOT use the `terminal-mcp` wrapper script — it relies on PATH and shebang resolution which fails inside Claude Code's subprocess environment.

Write or update `.mcp.json` in the project root:

```json
{
  "mcpServers": {
    "terminal": {
      "command": "/usr/bin/node",
      "args": [
        "/home/USER/.local/share/terminal-mcp/dist/index.js",
        "--headless",
        "--cols", "120",
        "--rows", "40"
      ]
    }
  }
}
```

Replace `/usr/bin/node` with the actual `$NODE_PATH` and `/home/USER/` with the actual `$HOME` from Step 4.

**Why `--headless`:** This mode embeds the PTY internally and serves MCP over stdio in a single process. It works in Docker containers, CI/CD, and environments without a TTY — which is exactly how Claude Code spawns MCPs.

**Alternative: `claude mcp add`** (if available):
```bash
claude mcp add terminal -- "$NODE_PATH" "$MCP_ENTRY" --headless --cols 120 --rows 40
```

### Step 7: Verify

Confirm the MCP is registered and the entry point exists:

```bash
# Check the config
cat .mcp.json 2>/dev/null

# Verify entry point is executable
node -e "require('$HOME/.local/share/terminal-mcp/dist/index.js')" 2>&1 | head -5
```

Report the final status to the user:
- Node.js version and path
- terminal-mcp install location
- MCP registration status and config file location
- Note: "Use `--headless` mode — ideal for Docker/container environments."

## Handling `$ARGUMENTS`

| Argument | Behavior |
|----------|----------|
| (none) or `install` | Run Steps 1-7 (idempotent — skips what's already done) |
| `status` | Run Steps 2, 3, 5 as checks only, report what's installed and configured |
| `reinstall` | Force reinstall: git pull + npm install + rebuild + re-register MCP |

## Available MCP Tools (reference)

Once installed, these tools become available to Claude Code:

| Tool | Description |
|------|-------------|
| `type` | Send text input to the terminal |
| `sendKey` | Send special keys (Enter, Tab, Ctrl+C, arrows, etc.) |
| `getContent` | Read the terminal buffer as plain text |
| `takeScreenshot` | Capture terminal state with cursor position (text, ansi, or png format) |
| `startRecording` | Begin recording session (asciicast v2 format) |
| `stopRecording` | Stop and finalize a recording |

## Important Notes

- **Always use absolute paths** when registering the MCP. The command must be the absolute path to `node`, and the first arg must be the absolute path to `dist/index.js`. This is the #1 cause of MCP spawn failures in Claude Code.
- **Always use `--headless`** in Docker/container environments. Without it, terminal-mcp tries a socket-based architecture that requires a separate TTY process.
- **Install from source**, not npm. This is the x85446 fork, not the published npm package.
- **Idempotent by default.** Running `/tui-runner` multiple times is safe.
- **Cross-platform.** Detects macOS vs Linux for package manager selection (brew vs nvm).
- **Node 18+ required.** terminal-mcp depends on Node.js 18+ features and node-pty native module.
- **No sudo.** All installations use user-level paths. If npm needs sudo, fix prefix first:
  ```bash
  mkdir -p ~/.npm-global
  npm config set prefix '~/.npm-global'
  export PATH="$HOME/.npm-global/bin:$PATH"
  ```
- **If MCP still won't spawn**, check these in order:
  1. Run `node ~/.local/share/terminal-mcp/dist/index.js --headless` manually — does it start?
  2. Check `/tmp/claudelog/` for error output from the MCP process.
  3. Verify node-pty compiled: `ls ~/.local/share/terminal-mcp/node_modules/node-pty/build/Release/pty.node`
  4. Rebuild native modules: `cd ~/.local/share/terminal-mcp && npm rebuild`
