// Package provider compiles literal CLI arguments and normalizes provider observations.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"os/exec"
	"strings"
	"time"
)

// Compile returns the executable and literal arguments. It never invokes a shell.
func Compile(s model.Subscription, q model.Sequence, a model.Assignment) ([]string, error) {
	if s.Executable == "" {
		return nil, fmt.Errorf("executable: required")
	}
	args := append([]string{}, s.Args...)
	if q.PresetID != "" {
		found := false
		for _, p := range s.Presets {
			if p.ID == q.PresetID {
				args = append(args, p.Args...)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("preset_id: unknown preset %q", q.PresetID)
		}
	}
	args = append(args, q.Args...)
	if err := ValidateArgs(s.Adapter, args); err != nil {
		return nil, err
	}
	prompt := a.Prompt
	if prompt == "" {
		prompt = "<assignment prompt>"
	}
	result := a.ResultPath
	if result == "" {
		result = "<assignment result path>"
	}
	cwd := a.ProjectPath
	if cwd == "" {
		cwd = "<project path>"
	}
	cmd := []string{s.Executable}
	switch s.Adapter {
	case "codex":
		cmd = append(cmd, "exec", "--json", "--color", "never", "--cd", cwd, "--output-last-message", result)
		if q.Model != "" {
			cmd = append(cmd, "--model", q.Model)
		}
		if q.Reasoning != "" {
			cmd = append(cmd, "-c", "model_reasoning_effort="+quoteTOML(q.Reasoning))
		}
		cmd = append(cmd, args...)
		cmd = append(cmd, "--", prompt)
	case "claude", "claude-code":
		cmd = append(cmd, "--print", "--output-format", "stream-json", "--verbose", "--include-partial-messages")
		if q.Model != "" {
			cmd = append(cmd, "--model", q.Model)
		}
		if q.Reasoning != "" {
			cmd = append(cmd, "--effort", q.Reasoning)
		}
		cmd = append(cmd, args...)
		cmd = append(cmd, "--", prompt)
	case "generic", "jsonl", "custom":
		transport := s.ResultTransport
		if transport == "" {
			transport = "argv_file"
		}
		if transport != "argv_file" && transport != "prompt_file" {
			return nil, fmt.Errorf("result_transport: use argv_file or prompt_file")
		}
		if transport == "prompt_file" {
			prompt = AssignmentPrompt(a)
		}
		if q.Model != "" || q.Reasoning != "" {
			return nil, fmt.Errorf("model/reasoning: this adapter requires explicit literal arguments")
		}
		hasPrompt, hasResult := false, false
		for _, v := range args {
			switch v {
			case "{prompt}":
				v = prompt
				hasPrompt = true
			case "{result}":
				v = result
				hasResult = true
			case "{cwd}":
				v = cwd
			}
			cmd = append(cmd, v)
		}
		if !hasPrompt || (transport == "argv_file" && !hasResult) {
			return nil, fmt.Errorf("args: custom adapters require standalone {prompt} and {result} tokens")
		}
	default:
		return nil, fmt.Errorf("adapter: unsupported %q; use generic with explicit result contract", s.Adapter)
	}
	return cmd, nil
}
func quoteTOML(v string) string { return fmt.Sprintf("%q", v) }

// ValidateArgs rejects supervisor-owned transport arguments before dispatch.
func ValidateArgs(adapter string, args []string) error {
	for i, v := range args {
		if strings.IndexByte(v, 0) >= 0 {
			return fmt.Errorf("args[%d]: NUL is invalid", i)
		}
		key := strings.SplitN(v, "=", 2)[0]
		if key == "--" {
			return fmt.Errorf("args[%d]: argument terminator is reserved", i)
		}
		reserved := false
		switch adapter {
		case "codex":
			for _, r := range []string{"--json", "--cd", "-C", "--output-last-message", "-o", "--output-schema", "--color", "--worktree", "--profile", "-p", "--oss", "--local-provider", "--model", "-m"} {
				if key == r || ((r == "-C" || r == "-o") && strings.HasPrefix(v, r)) {
					reserved = true
				}
			}
			if key == "-c" || key == "--config" || strings.HasPrefix(v, "-c") {
				cv := ""
				if key == "-c" || key == "--config" {
					if strings.Contains(v, "=") {
						cv = strings.SplitN(v, "=", 2)[1]
					} else if i+1 < len(args) {
						cv = args[i+1]
					}
				} else {
					cv = strings.TrimPrefix(v, "-c")
				}
				for _, r := range []string{"model_provider", "model_providers", "cli_auth_credentials_store", "forced_login_method", "model_reasoning_effort"} {
					if strings.HasPrefix(cv, r+"=") || strings.HasPrefix(cv, r+".") {
						reserved = true
					}
				}
			}
		case "claude", "claude-code":
			for _, r := range []string{"--print", "-p", "--output-format", "--input-format", "--json-schema", "--resume", "-r", "--continue", "-c", "--session-id", "--fork-session", "--verbose", "--include-partial-messages", "--model", "--effort", "--fallback-model", "--bare", "--setting-sources"} {
				if key == r {
					reserved = true
				}
			}
		}
		if reserved {
			return fmt.Errorf("args[%d]: %s is reserved for the supervisor or structured controls", i, key)
		}
	}
	return nil
}

// CommandPreview quotes only for display; dispatch uses Compile's original tokens.
func CommandPreview(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = "'" + strings.ReplaceAll(a, "'", "'\"'\"'") + "'"
	}
	return strings.Join(out, " ")
}

// ExecutionEnv prevents an inherited API key from replacing subscription authentication.
func ExecutionEnv(adapter string, env []string) []string {
	if adapter != "codex" && adapter != "claude" && adapter != "claude-code" {
		return append([]string{}, env...)
	}
	deny := map[string]bool{"OPENAI_API_KEY": true, "CODEX_API_KEY": true, "OPENAI_BASE_URL": true, "ANTHROPIC_API_KEY": true, "ANTHROPIC_AUTH_TOKEN": true, "ANTHROPIC_BASE_URL": true, "CLAUDE_CODE_USE_BEDROCK": true, "CLAUDE_CODE_USE_VERTEX": true, "CLAUDE_CODE_USE_FOUNDRY": true, "CLAUDECODE": true}
	out := []string{}
	for _, v := range env {
		if !deny[strings.SplitN(v, "=", 2)[0]] {
			out = append(out, v)
		}
	}
	return out
}

// Probe checks executable help without submitting a model request or claiming authentication.
func Probe(ctx context.Context, s model.Subscription) model.Readiness {
	r := model.Readiness{Executable: s.Executable}
	path, err := exec.LookPath(s.Executable)
	if err != nil {
		r.Reason = "Executable unavailable"
		return r
	}
	r.Executable = path
	r.Installed = true
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	args := []string{"--help"}
	if s.Adapter == "codex" {
		args = []string{"exec", "--help"}
	}
	b, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		r.Reason = "Executable help failed"
		return r
	}
	switch s.Adapter {
	case "codex":
		r.Stream = strings.Contains(string(b), "--json")
		r.ResultContract = strings.Contains(string(b), "--output-last-message")
		r.Telemetry = true
	case "claude", "claude-code":
		r.Stream = strings.Contains(string(b), "stream-json")
		r.ResultContract = r.Stream
		_, collectorErr := exec.LookPath("codexbar")
		r.Telemetry = len(s.UsageCommand) > 0 || collectorErr == nil
	case "generic", "jsonl", "custom":
		_, e := Compile(s, model.Sequence{}, model.Assignment{})
		r.ResultContract = e == nil
		r.Stream = s.Adapter == "jsonl"
		r.Telemetry = len(s.UsageCommand) > 0
	default:
		r.Reason = "Adapter unsupported"
		return r
	}
	r.Reason = "Capabilities inspected; authentication, fresh usage and assignment result still require validation"
	return r
}

// Templates are disabled editable starting points, never proof of account readiness.
func Templates() []model.Subscription {
	defs := []struct {
		id, name, exe, adapter string
		args                   []string
	}{
		{"codex", "Codex", "codex", "codex", nil}, {"claude", "Claude Code", "claude", "claude", nil},
		{"gemini", "Gemini CLI", "gemini", "generic", []string{"-p", "{prompt}"}},
		{"kimi", "Kimi CLI", "kimi", "generic", []string{"--print", "-p", "{prompt}"}},
		{"opencode", "OpenCode", "opencode", "generic", []string{"run", "{prompt}"}},
		{"copilot", "GitHub Copilot CLI", "copilot", "generic", []string{"-p", "{prompt}"}},
		{"aider", "Aider", "aider", "generic", []string{"--message", "{prompt}"}},
		{"goose", "Goose", "goose", "generic", []string{"run", "--text", "{prompt}"}},
		{"custom", "Custom CLI", "", "generic", []string{"{prompt}", "{result}"}},
	}
	out := []model.Subscription{}
	for _, d := range defs {
		s := model.Subscription{ID: d.id, Name: d.name, Executable: d.exe, Adapter: d.adapter, Args: d.args, PoolID: d.id + "-account", PowerBandID: "universal", Concurrency: 1, FreshnessSeconds: 300, TypicalReset: model.ResetSchedule{Timezone: "America/Chicago"}}
		s.Sequences = []model.Sequence{{ID: d.id + "-build", Name: "build"}}
		if d.adapter == "generic" && d.id != "custom" {
			s.ResultTransport = "prompt_file"
		}
		if d.adapter == "codex" {
			s.Presets = []model.Preset{{ID: "codex-standard", Name: "Workspace", Args: []string{"--sandbox", "workspace-write"}}, {ID: "codex-yolo", Name: "YOLO", Args: []string{"--dangerously-bypass-approvals-and-sandbox"}}}
			s.Sequences[0].PresetID = "codex-standard"
		}
		if d.adapter == "claude" {
			s.Presets = []model.Preset{{ID: "claude-standard", Name: "Accept edits", Args: []string{"--permission-mode", "acceptEdits"}}, {ID: "claude-yolo", Name: "YOLO", Args: []string{"--dangerously-skip-permissions"}}}
			s.Sequences[0].PresetID = "claude-standard"
		}
		out = append(out, s)
	}
	return out
}

// AssignmentPrompt appends the supervisor-owned result-file protocol. The child
// reports evidence for the actual task; it cannot claim completion by exit alone.
func AssignmentPrompt(a model.Assignment) string {
	prompt := a.Prompt
	if prompt == "" {
		prompt = "<assignment prompt>"
	}
	destination := a.ResultPath
	if destination == "" {
		destination = "<assignment result path>"
	}
	identity := model.Result{Version: model.ProtocolVersion, AssignmentID: a.ID, RunID: a.RunID, ProjectPath: a.ProjectPath, Plan: a.Plan, Step: a.Step, SourceDigest: a.SourceDigest, Outcome: "partial", Evidence: []string{"Concrete verification command and observed result"}}
	b, _ := json.Marshal(identity)
	transport := "write one JSON object to " + destination + "."
	if a.Subscription.Adapter == "codex" {
		transport = "return exactly one JSON object as your final message, without markdown fences or surrounding text. The supervisor-owned CLI output transport saves that final message to " + destination + "; do not write the result file through a tool."
	}
	return prompt + "\n\nAssignment result contract: " + transport + " Preserve all identity fields in this template: " + string(b) + ". The evidence field must be an array of plain strings, never objects. The task_complete and validation_met fields must be JSON booleans. Preserve all field names and identity value types exactly. Set task_complete and validation_met only after doing the assigned work and its required verification. Record concrete evidence. Use outcome completed only when both are true; otherwise use blocked, retry, partial, or interrupted and explain blocker. This is one bounded assignment; do not advance another pair or claim whole-plan completion."
}
