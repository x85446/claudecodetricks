package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/x85446/claudecodetricks/src/pkg/hooks"
)

// SummarizeTranscript reads a transcript file and generates an 8-word summary
func SummarizeTranscript(transcriptPath, projectName string) (string, error) {
	// Read and parse transcript
	toolCalls, err := extractToolCalls(transcriptPath)
	if err != nil {
		return fmt.Sprintf("session work on %s", projectName), nil
	}

	if len(toolCalls) == 0 {
		return fmt.Sprintf("session in %s", projectName), nil
	}

	// Create Claude client
	client, err := NewClient()
	if err != nil {
		return fmt.Sprintf("session work on %s", projectName), nil
	}

	// Build prompt
	prompt := buildSummaryPrompt(toolCalls, projectName)

	// Call Claude API
	summary, err := client.SendMessage(prompt, 30)
	if err != nil {
		return fmt.Sprintf("session work on %s", projectName), nil
	}

	// Clean and validate summary
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return fmt.Sprintf("session work on %s", projectName), nil
	}

	return summary, nil
}

// extractToolCalls parses the transcript and extracts significant tool calls
func extractToolCalls(transcriptPath string) ([]string, error) {
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var toolCalls []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var entry hooks.TranscriptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		// Look for tool use in content blocks
		for _, block := range entry.Content {
			if block.Type == "tool_use" && block.ToolUse != nil {
				toolCall := formatToolCall(block.ToolUse.Name, block.ToolUse.Input)
				toolCalls = append(toolCalls, toolCall)
			}
		}
	}

	return toolCalls, scanner.Err()
}

// formatToolCall formats a tool call for the summary prompt
func formatToolCall(toolName string, input map[string]interface{}) string {
	switch toolName {
	case "Write":
		if filePath, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("Write: %s", filepath.Base(filePath))
		}
	case "Edit":
		if filePath, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("Edit: %s", filepath.Base(filePath))
		}
	case "Read":
		if filePath, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("Read: %s", filepath.Base(filePath))
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			// Truncate long commands
			if len(cmd) > 50 {
				cmd = cmd[:50] + "..."
			}
			return fmt.Sprintf("Bash: %s", cmd)
		}
	}
	return fmt.Sprintf("%s", toolName)
}

// buildSummaryPrompt creates the prompt for Claude to summarize the session
func buildSummaryPrompt(toolCalls []string, projectName string) string {
	toolSummary := strings.Join(toolCalls, "\n")

	// Limit to last 20 tool calls to avoid huge prompts
	lines := strings.Split(toolSummary, "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
		toolSummary = strings.Join(lines, "\n")
	}

	return fmt.Sprintf(`Summarize this Claude Code session in exactly 8 words using Conventional Commits format.

Format: <type>: <subject>

Available types:
- feat: New functionality or files
- fix: Bug fixes or corrections
- docs: Documentation changes
- refactor: Code restructuring
- test: Test-related changes
- chore: Config, tooling, maintenance

Rules:
- Exactly 8 words maximum
- Use imperative mood (add, fix, update, not added, fixed, updated)
- Be specific about what was accomplished
- No period at the end

Project: %s

Tool calls:
%s

Summary (8 words):`, projectName, toolSummary)
}
