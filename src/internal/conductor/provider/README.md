# Provider adapters

`Compile` returns executable plus ordered literal argv. The supervisor executes these tokens directly, sets the assignment working directory, and uses `ExecutionEnv` for built-in subscription adapters. `CommandPreview` renders the same argv for display. Model and reasoning controls compile into provider flags; permission presets and additional arguments retain their order and repeated values. Reserved working-directory, prompt/result transport and account-provider switches fail before execution.

Codex uses JSONL and `--output-last-message` for the result file. Claude uses stream-json; the assignment prompt instructs the child to write its result destination. `ExtractResult` also exposes a terminal JSON candidate for supervisor validation. An exit code or terminal event never validates an assignment. The supervisor checks result identity, source digest, task/validation status and evidence before advancing a plan.

`Decode` drains records incrementally, bounds each record to 1 MiB, and emits diagnostics for malformed or unknown events. Malformed streams return an error after draining. Tool input/output bodies are omitted from public activity. Allowance observations and execution-token totals have separate event kinds. The supervisor supplies durable event identity, retention, redaction and cumulative usage reconciliation.

## Subscription allowance

`CollectUsage` calls the installed Codex app-server's read-only account and rate-limit RPCs. Authenticated account identity produces a stable provider-prefixed hash for both account and pool. An unbound subscription can discover this identity; dispatch requires an explicit matching binding. A missing secondary window is explicitly unsupported only when an otherwise valid provider response omits it. Provider embargo remains separate from measured percentages.

Claude allowance collection uses the installed `codexbar usage --provider claude --source cli --format json --no-credits --no-color` collector. Claude auth status checks before and after collection establish a stable claude.ai account. The selected executable must match the default Claude executable that CodexBar invokes. The collector's timestamp and resets remain authoritative. Missing CodexBar, failed authentication or an account change is explicit. Live stream rate-limit observations use `NormalizeClaudeUsage` with the verified process account scope.

The existing statusline contract exposes `rate_limits.five_hour` and `rate_limits.seven_day`, with `used_percentage` and `resets_at`. Its `/tmp/sessiondata` snapshot contains session identity and capture time but lacks account identity, so it cannot authorize a configured subscription.

A custom `usage_command` is a literal executable/argument array returning one `model.Usage` JSON object. Its subscription, account and pool must match the selected binding. Source and observation timestamp are required. Token totals cannot replace allowance percentages.

## Agent templates

Templates are disabled, editable starting points. Registry presence does not prove installation, authentication, allowance support or result-contract readiness. Codex and Claude have native stream/result adapters. Other seeded agent definitions use `prompt_file`: the compiler appends the supervisor-owned result destination, identity and evidence requirements to their `{prompt}` argument. The `argv_file` transport for wrappers requires standalone `{prompt}` and `{result}` tokens; `{cwd}` is optional. All transports require a validated result file and authenticated fresh allowance before dispatch. Model and reasoning choices for generic adapters use literal arguments. The `jsonl` adapter accepts bounded normalized events; the `generic` adapter relies on the result file and diagnostic process output.

| Template | Noninteractive invocation | Reference |
|---|---|---|
| Codex | `exec --json --output-last-message` | Installed `codex exec --help`; [official example](https://developers.openai.com/cookbook/examples/codex/build_iterative_repair_loops_with_codex) |
| Claude Code | `--print --output-format stream-json --verbose` | Installed `claude --help` |
| Gemini CLI | `-p` | [Headless mode](https://google-gemini.github.io/gemini-cli/docs/cli/headless.html) |
| Kimi CLI | `--print -p` | [Print mode](https://moonshotai.github.io/kimi-cli/en/customization/print-mode.html) |
| OpenCode | `run` | Installed `opencode run --help`; [project](https://github.com/anomalyco/opencode) |
| GitHub Copilot CLI | `-p` | [Programmatic reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-programmatic-reference) |
| Aider | `--message` | Installed `aider --help`; [Scripting](https://aider.chat/docs/scripting.html) |
| Goose | `run --text` | [CLI reference](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/goose-cli-commands.md) |
| Custom CLI | User-defined executable and literal arguments | Explicit generic result contract |

No template installs a CLI, purchases access or enables an API fallback. Built-in subscription environments omit inherited API-key and third-party provider override variables. Optional agents without verified subscription authorization remain unavailable for dispatch.
