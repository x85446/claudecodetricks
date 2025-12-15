package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/x85446/claudecodetricks/src/internal/git"
	"github.com/x85446/claudecodetricks/src/pkg/hooks"
)

func main() {
	// Read JSON from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(0)
	}

	// Parse hook input
	var hookInput hooks.HookInput
	if err := json.Unmarshal(input, &hookInput); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(0)
	}

	// Handle based on event type
	switch hookInput.HookEventName {
	case "PostToolUse":
		handlePostToolUse(&hookInput)
	case "Stop":
		handleStop(&hookInput)
	}

	os.Exit(0)
}

func handlePostToolUse(hookInput *hooks.HookInput) {
	// Only process Write and Edit tools
	if hookInput.ToolName != "Write" && hookInput.ToolName != "Edit" {
		return
	}

	// Skip in plan mode
	if hookInput.PermissionMode == "plan" {
		return
	}

	// Check if we're in a git repo
	if !git.IsGitRepo(hookInput.CWD) {
		return
	}

	// Get the file path from tool input
	filePath, ok := hookInput.ToolInput["file_path"].(string)
	if !ok || filePath == "" {
		return
	}

	// Check if file is in gitignore
	if git.IsInGitIgnore(hookInput.CWD, filePath) {
		return
	}

	// Generate conventional commit message
	message := git.GenerateCommitMessage(hookInput.ToolName, filePath, hookInput.ToolInput)

	// Truncate message to 50 chars
	if len(message) > 50 {
		message = message[:47] + "..."
	}

	// Stage and commit the file
	if err := git.StageFiles(hookInput.CWD, []string{filePath}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stage file: %v\n", err)
		return
	}

	if err := git.Commit(hookInput.CWD, message); err != nil {
		// Might fail if no changes, that's okay
		return
	}

	fmt.Fprintf(os.Stderr, "Auto-committed: %s\n", message)
}

func handleStop(hookInput *hooks.HookInput) {
	// Check if we're in a git repo
	if !git.IsGitRepo(hookInput.CWD) {
		return
	}

	// Check for uncommitted changes
	hasChanges, err := git.HasUncommittedChanges(hookInput.CWD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to check git status: %v\n", err)
		return
	}

	if !hasChanges {
		return
	}

	// Get modified files
	files, err := git.GetModifiedFiles(hookInput.CWD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get modified files: %v\n", err)
		return
	}

	// Generate suggested commit message from files
	projectName := filepath.Base(hookInput.CWD)
	message := generateStopCommitMessage(files, projectName)

	// Prompt user
	fmt.Fprintf(os.Stderr, "\nYou have uncommitted changes.\n")
	fmt.Fprintf(os.Stderr, "Suggested commit:\n  %s\n\n", message)

	response, err := git.PromptUser("Create commit now?")
	if err != nil || !response {
		return
	}

	// Stage all files
	if err := git.StageFiles(hookInput.CWD, files); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stage files: %v\n", err)
		return
	}

	// Commit
	if err := git.Commit(hookInput.CWD, message); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to commit: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Committed: %s\n", message)
}

func generateStopCommitMessage(files []string, projectName string) string {
	if len(files) == 0 {
		return fmt.Sprintf("chore: session work on %s", projectName)
	}

	// Use the first file to determine the commit message
	firstFile := files[0]

	// Simple heuristic based on file type
	ext := filepath.Ext(firstFile)
	base := filepath.Base(firstFile)

	switch ext {
	case ".md", ".mdx":
		return fmt.Sprintf("docs: update %s", base)
	case ".go", ".js", ".ts", ".py", ".java":
		if len(files) == 1 {
			return fmt.Sprintf("feat: update %s", base)
		}
		return fmt.Sprintf("feat: update %d files", len(files))
	default:
		if len(files) == 1 {
			return fmt.Sprintf("chore: update %s", base)
		}
		return fmt.Sprintf("chore: update %d files", len(files))
	}
}
