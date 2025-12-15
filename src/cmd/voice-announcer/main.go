package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/x85446/claudecodetricks/src/internal/voice"
	"github.com/x85446/claudecodetricks/src/pkg/hooks"
)

func main() {
	// Read JSON from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(0) // Don't block the hook
	}

	// Parse hook input
	var hookInput hooks.HookInput
	if err := json.Unmarshal(input, &hookInput); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(0)
	}

	// Get project name from CWD
	projectName := filepath.Base(hookInput.CWD)

	// Generate message based on hook event
	message := generateMessage(hookInput.HookEventName, projectName)
	if message == "" {
		// Unknown event, don't announce
		os.Exit(0)
	}

	// Send voice announcement
	if err := voice.Announce(projectName, message); err != nil {
		fmt.Fprintf(os.Stderr, "Voice announcement failed: %v\n", err)
	}

	os.Exit(0)
}

func generateMessage(eventName, projectName string) string {
	switch eventName {
	case "Notification":
		return fmt.Sprintf("Claude has a question in %s", projectName)
	case "UserPromptSubmit":
		return fmt.Sprintf("%s has a question for you", projectName)
	case "Stop":
		return fmt.Sprintf("Project %s stopped. Waiting for your command", projectName)
	default:
		return ""
	}
}
