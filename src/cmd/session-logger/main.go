package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/claude"
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

	// Only process Stop events
	if hookInput.HookEventName != "Stop" {
		os.Exit(0)
	}

	// Get project name from CWD
	projectName := filepath.Base(hookInput.CWD)

	// Generate log file path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home dir: %v\n", err)
		os.Exit(0)
	}

	logDir := filepath.Join(homeDir, ".claude", "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating log dir: %v\n", err)
		os.Exit(0)
	}

	now := time.Now()
	logFile := filepath.Join(logDir, fmt.Sprintf("cAudit-%s.log", now.Format("2006-01-02")))

	// Generate summary
	summary := generateSummary(hookInput.TranscriptPath, projectName)

	// Write log entry
	timestamp := now.Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("%s %s %s\n", timestamp, projectName, summary)

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log file: %v\n", err)
		os.Exit(0)
	}
	defer f.Close()

	if _, err := f.WriteString(logEntry); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to log: %v\n", err)
	}

	os.Exit(0)
}

func generateSummary(transcriptPath, projectName string) string {
	// Check if transcript exists
	if transcriptPath == "" || !fileExists(transcriptPath) {
		return fmt.Sprintf("session in %s", projectName)
	}

	// Try to generate AI summary
	summary, err := claude.SummarizeTranscript(transcriptPath, projectName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating summary: %v\n", err)
		return fmt.Sprintf("session work on %s", projectName)
	}

	return summary
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
