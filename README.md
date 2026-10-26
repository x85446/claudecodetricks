# Claude Code Tricks

A Claude Code marketplace providing:
- **Agent Teams**: Pre-configured multi-agent teams for product development
- **Session Hooks**: Voice announcements, AI-powered logging, and automatic git commits
- **Iterate Dashboard**: One page showing every plan across all your projects, optionally always-on

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

## Iterate Dashboard and Conductor

The dashboard and Conductor share **http://localhost:8420/**. All projects shows canonical project folders, queue counts and admission reasons. Each project opens Plans, Automations and Activity. Plans retains Queued, Active and Archived filters and the full goal, requirements, timeline, logs and evidence for each plan.

### Open the dashboard

```bash
make run
```

This builds the application and reuses the managed dashboard on port 8420, or starts that port in the foreground when it is free. An occupied port produces an ownership error; startup does not select another port. A stale installed service reports its build so you can perform a deliberate handover.

### Keep it running

```bash
make serve-install     # atomically install and hand over the managed 8420 service
make serve-status
make serve-uninstall
```

The macOS service starts at login and restarts after a failure. Repeated installation verifies the intended build and one listener. The installer preserves a rollback binary and service definition. Check the active build with:

```bash
curl http://localhost:8420/healthz
iterate-run version
```

Logs live in `~/Library/Logs/iterate-run/serve.log`. The dashboard presents project plans, the dependency graph, recorded runs, archives and reports. Execution labels, positions, release groups and Next come from the same graph as `iterate-run plan graph --json`.

### Keep it on screen

A dedicated browser window can sit on the dashboard permanently, reopening itself if you close it:

```bash
make chrome-install     # opens Chrome on the dashboard, and keeps it open
make chrome-status
make chrome-uninstall
```

This window runs in its own browser profile, so your everyday Chrome — bookmarks, extensions, signed-in accounts — is untouched. It also exposes a debugging port on **9242** for tools that attach to a browser. That is deliberately not Chrome's usual 9222, so nothing that attaches to your normal browser lands here by accident.

To use Chromium or another Chromium-family browser instead:

```bash
make chrome-install CHROME_BIN=/Applications/Chromium.app/Contents/MacOS/Chromium
```

Both services are macOS only. On other systems the commands stop with a clear message rather than half-installing.

### What the dashboard shows

Compact plan rows show status, actual AI assignment, plan name, goal and recorded timing. An unassigned plan stays unassigned. Queue links select the exact counted plans, with waiting and running work reported separately. Each worktree retains its full canonical identity.

Opening a plan shows its report in reading order:

1. The plan header: name, harness, launch command, goal, and a **Needs you** banner when the plan waits on a person.
2. The summary: one elapsed figure, the plan's outcome, requirement and lane counts, a grid with one cell per requirement, and the validation evidence behind each requirement.
3. The execution timeline: the coordinator's lane and each team's lane on one clock axis. Each lane opens to its numbered steps and validations, each linked to its evidence.
4. The breakdowns: time, ownership and handoffs, attempts and rework, usage, tracking coverage, and delivery.
5. For a plan that is not archived, the Conductor activity. Its event log scrolls in its own fixed-height box.

An **On this page** list after the header jumps to each part. Each requirement cell is a link that opens that requirement's evidence, by mouse or keyboard. Every section states whether its data is available, partial or unavailable, and an unknown figure reads as unknown, never as zero. Tool history loads after the page appears; disclosures you opened, the requirement you selected and your place on the page stay as they were when it arrives. Existing `/plan?project=…&name=…` and `/archive?project=…&file=…` bookmarks enter this same project workspace. Archived open observations describe historical records, not a presently running process.

Activity updates every three seconds while the page is visible. Durable events survive reloads and reconnects; configuration drafts retain their values while read-only state refreshes. Session token totals remain separate from subscription allowance percentages.

### Reading a plan report

Each section answers one question and names its unit. A count of requirements is never a count of lanes, and a figure nobody recorded reads **unknown**, not zero.

- **Verified progress**: the plan's outcome, taken from its recorded completion and archive, then requirement counts (complete, tasks implemented, validations passed, and how many of those have observed proof) apart from lane counts (completed, active, blocked, queued). A checked box is not proof. A lane of an ended plan with no recorded end counts as **unknown end**, never as running.
- **Validation evidence**: for each requirement and attempt, the result, command, exit status, time, artifact, source revision and environment. What the validation must show sits in its own collapsed **Acceptance** box, apart from what was observed. Reports copied from the plan log are tagged **imported** with result not observed. When a file the proof covers changes after the proof's revision, the proof reads **stale** and stays visible.
- **Time**: elapsed time split into execution, blocked on a person, paused, waiting on a schedule, queued and unobserved; the parts add up to the elapsed figure exactly. Time spent by lanes in parallel (tool calls, subagents, builds, model turns) is listed apart and never added to elapsed. A long stretch without execution shows its recorded cause, or reads **unknown, not a failure**.
- **Ownership and handoffs**: who holds the plan now (a CLI session or a Conductor assignment), its last heartbeat, when a blocker was detected and cleared, when ownership was released, and what admission is waiting on: automation disabled, schedule, budget or ownership. Each handoff reports the delays from blocker cleared to released to resumed. A stale owner is labelled; reading the report never releases it.
- **Attempts and rework**: attempts by outcome, retries, restarts and reopened requirements, first-pass success and recoveries with their denominators, and what recovery cost in time and tokens.
- **Usage**: what the plan cost, by scope (see below).
- **Tracking coverage**: which sessions, agents and requests were captured, per lane and per source, and whether the record is complete, partial or unavailable. A percentage appears only when the total is known.
- **Delivery**: source commit, pull request, review, CI, merge, install, deploy and runtime verification as separate stages, each with its own revision and time. A green CI never implies a deploy, and a stage with no evidence says so.

### What a run cost in tokens

A plan's **Usage** section reports what the run cost. Plan-attributed usage sits apart from whole-session cost and subscription allowance, and its breakdowns are:

- **By coordinator, team and agent**: one row per lane with requests, input, cache, output and reasoning tokens, and the peak request input.
- **By tool** (estimated): per tool, how much bigger the next request got because the call happened (the real price of a read, paid again every later turn) alongside what the model spent writing the call itself.
- **By wake-up** (estimated): one row per kind of wake-up: a cron or `/loop` firing, a coordinator nudge to a team, something you typed. A firing that arrived mid-turn and never got its own request is counted **coalesced** and cost nothing; one that ran and then made no tool call is **idle**, and those are the expensive ones.

`iterate-run tokens [--plan <name>]` prints the per-lane, per-tool and per-wake-up tables in the terminal.

Numbers come from the session transcripts Claude Code and Codex already write, joined to the hook event log by session and agent id, plus the provider usage the Conductor records for its assignments. A request counts once even when it appears in a parent's transcript, a resumed session or the Conductor's record. Session cost in dollars is Claude Code's own figure for the whole session, which usually covers more than one plan; it is labelled **whole-session cost**, shown apart from plan usage and subscription allowance, and never divided between lanes.

### The run record

Every plan execution has one durable history, the run record, which hooks, `/iterate` and the Conductor append to and every report reads. A plan that ran before its project had a record gets a read-only view rebuilt from its plan file, hook log and Conductor history, labelled **legacy**.

```bash
iterate-run record list                          # recorded runs in this project
iterate-run record show --plan finch --json      # one execution, folded: lifecycle, attempts, evidence, usage, unknowns
iterate-run record show --plan finch --archive 20261001T120000Z-finch-done.md
iterate-run record import --plan finch --dry-run # what a legacy import would record; drop --dry-run to keep it
```

The same reads have make targets: `make record-list`, `make record-show PLAN=finch`, `make record-import PLAN=finch` (a preview unless `APPLY=1`) and `make report-json PLAN=finch HISTORY=1`, which prints the report the plan page renders. `make preview` serves the current build's dashboard on port 8490 beside the one on 8420, with a throwaway Conductor store, so trying a build never admits or changes real work.

### Plan names

Plans are named after animals, drawn from a pool of 1,442 across every letter. Each project works through the alphabet in its own order, and no two plans anywhere on your machine ever get the same name.

If a letter runs out, naming moves to the next letter with room rather than failing. Only a completely exhausted pool is an error, and it tells you how many names remain under each letter.

To see what the next plan in a project will be called without claiming the name:

```bash
iterate-run name peek
# h hamster
```

### Plan states in the terminal

`iterate-run status`, run inside a project, lists every plan with its state before any running units:

```
plan flamingo executing
plan gaur planned
plan quokka queued
```

States are `blocked`, `paused`, `unblocked`, `executing`, `queued` and `planned`, in that order of precedence.

The Claude Code status line carries the same states as one letter per plan after the `⚙️` icon, coloured by state and cased by shape (UPPERCASE for a teamed plan, lowercase for a flat one):

| Colour | Meaning |
|---|---|
| bold green | executing, and something touched a tool inside the last 15 minutes |
| dark green | executing according to the plan file, but nothing has run in 15 minutes — the session stalled or died |
| yellow | planned, not yet approved |
| orange | queued — approved for the nightly tick |
| red | blocked on something only you can supply |
| cyan | unblocked, waiting to be picked back up |
| magenta | paused by you |

The icon lights up as `⚡` while a run is live. A dim trailing letter indicates the next animal-name allocation in the Claude Code status line; it does not indicate execution order. `⏰` after the letters means this project is enrolled in the nightly tick; `⏰ off` means the tick is installed but switched off. Orange and dark green need a 256-colour terminal; on a 16-colour one they appear as bright yellow and plain green.

### iTerm2 execution order

The personal-mac-tools iTerm2 status bar reads `iterate-run plan graph --json`. It shows stable A–Z execution labels in position order, separators between release groups, and `Next` for the next ready plan. State changes preserve positions. Waiting plans show their prerequisite in the popup.

Flags show only the icon and message count at every width. A separate soft-green bar label shows the complete project directory name without abbreviation and yields space when it cannot fit. Each flag popup identifies the clicked pane's full project name and canonical folder path in dark green, with one separator before the inbox actions. See the [status-bar operating guide](https://github.com/x85446/personal-mac-tools/blob/6e38c8f/docs/product/iterm-statusbar.md).

### Execute plans

A bare `/iterate` or `$iterate` selects the sole ready plan when the project has no live execution owner. Several ready plans produce a list. A named plan obeys its unfinished dependencies. The shared `iterate-run plan claim|heartbeat|release|reconcile` contract excludes concurrent executors and preserves checkpoint evidence during recovery.

Stage approved work with `/ip stage <name>` in Claude Code or `$ip stage <name>` in Codex. The external `conductor` service at `~/workspace/tmctech/conductor` runs staged queues and hosts scheduling controls on localhost:8421. It uses the shared graph, ownership and record CLI contracts. Release and release-candidate plans require a direct human invocation.

### Order plans with dependencies

Each plan names the plans it builds on in one line near its top, so you can stage a whole queue and let the conductor run it in the right order:

```
depends: M1-04, M1-05
```

A plan waits until every plan on that line has finished, and explicit graph positions order plans with no unfinished predecessors. When a plan parks on something only you can supply, every plan behind it waits instead of launching and parking in turn, and the dashboard notice names the plan holding them: `caiman holds 7 plans`.

- `/ip <name> depends on <x>` sets the line, `/ip drop dependency <x> from <name>` removes one, and `/ip <name> depends on nothing` marks a plan that needs only what main already holds. Editing the line keeps the plan staged.
- `/ip graph` (or `iterate-run plan graph` in a project) prints the graph: `✔` done, `●` ready, `◌` waiting, `✖` blocked, with what each plan waits on. Add `--format mermaid` for a diagram you can paste into Markdown, `--json` for scripts, or `--check` to list problems such as a cycle or a name that matches no plan.
- The dashboard's **Graph** tab draws the same graph. Dashed arrows still wait, and selecting a plan opens it. In the **Plans** tab, a held plan's badge reads **Waiting** and its row says what it waits on.
- `/ip retrofit` gives existing plans their line. It reads each plan's milestone and its "Depends on" notes, writes what it can derive, settles the rest by reading them, and tells you which `/ip stage` command would queue what is left. `iterate-run plan depends backfill` shows the same proposals without writing anything; add `--apply` to write them.

[docs/plan-graph.md](docs/plan-graph.md) has the full reference.

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
