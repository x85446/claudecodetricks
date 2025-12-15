package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CommitType represents a Conventional Commits type
type CommitType string

const (
	TypeFeat     CommitType = "feat"
	TypeFix      CommitType = "fix"
	TypeDocs     CommitType = "docs"
	TypeStyle    CommitType = "style"
	TypeRefactor CommitType = "refactor"
	TypeTest     CommitType = "test"
	TypeChore    CommitType = "chore"
	TypePerf     CommitType = "perf"
)

// GenerateCommitMessage generates a Conventional Commits message
func GenerateCommitMessage(toolName, filePath string, toolInput map[string]interface{}) string {
	commitType := determineCommitType(toolName, filePath, toolInput)
	scope := determineScope(filePath)
	subject := determineSubject(toolName, filePath, toolInput)

	// Format: <type>(<scope>): <subject>
	if scope != "" {
		return fmt.Sprintf("%s(%s): %s", commitType, scope, subject)
	}
	return fmt.Sprintf("%s: %s", commitType, subject)
}

// determineCommitType determines the commit type based on tool and file
func determineCommitType(toolName, filePath string, toolInput map[string]interface{}) CommitType {
	ext := filepath.Ext(filePath)
	base := filepath.Base(filePath)
	dir := filepath.Dir(filePath)

	// Documentation files
	if ext == ".md" || ext == ".mdx" || ext == ".rst" || strings.Contains(strings.ToLower(base), "readme") {
		return TypeDocs
	}

	// Test files
	if strings.Contains(base, "test") || strings.Contains(base, "spec") || strings.Contains(dir, "test") {
		return TypeTest
	}

	// Config files
	if isConfigFile(base) {
		return TypeChore
	}

	// For Write tool, it's usually a new feature
	if toolName == "Write" {
		return TypeFeat
	}

	// For Edit tool, analyze the change type
	if toolName == "Edit" {
		if isBugFix(toolInput) {
			return TypeFix
		}
		if isRefactoring(toolInput) {
			return TypeRefactor
		}
		// Default to fix for edits
		return TypeFix
	}

	return TypeChore
}

// determineScope determines the scope from the file path
func determineScope(filePath string) string {
	// Use the directory name or file basename without extension
	dir := filepath.Dir(filePath)
	if dir != "." && dir != "/" {
		parts := strings.Split(dir, string(filepath.Separator))
		// Use the last directory name
		if len(parts) > 0 {
			scope := parts[len(parts)-1]
			// Clean up the scope
			scope = strings.TrimPrefix(scope, ".")
			if scope != "" && len(scope) < 20 {
				return scope
			}
		}
	}

	// Fall back to filename without extension
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if len(name) < 20 {
		return name
	}

	return ""
}

// determineSubject creates the commit subject line
func determineSubject(toolName, filePath string, toolInput map[string]interface{}) string {
	base := filepath.Base(filePath)

	switch toolName {
	case "Write":
		return fmt.Sprintf("add %s", base)
	case "Edit":
		// Try to be more specific about what was edited
		if oldStr, ok := toolInput["old_string"].(string); ok {
			if len(oldStr) > 30 {
				oldStr = oldStr[:30] + "..."
			}
			return fmt.Sprintf("update %s", base)
		}
		return fmt.Sprintf("modify %s", base)
	default:
		return fmt.Sprintf("update %s", base)
	}
}

// isConfigFile checks if the file is a configuration file
func isConfigFile(filename string) bool {
	configFiles := []string{
		"package.json", "package-lock.json",
		"go.mod", "go.sum",
		"Cargo.toml", "Cargo.lock",
		"Makefile", "Dockerfile",
		".gitignore", ".env",
		"tsconfig.json", "webpack.config.js",
		".eslintrc", ".prettierrc",
	}

	for _, cf := range configFiles {
		if filename == cf {
			return true
		}
	}

	return strings.HasPrefix(filename, ".") && (strings.HasSuffix(filename, "rc") || strings.HasSuffix(filename, "config"))
}

// isBugFix analyzes if the edit looks like a bug fix
func isBugFix(toolInput map[string]interface{}) bool {
	oldStr, oldOk := toolInput["old_string"].(string)
	newStr, newOk := toolInput["new_string"].(string)

	if !oldOk || !newOk {
		return false
	}

	// Simple heuristics
	oldLower := strings.ToLower(oldStr)
	newLower := strings.ToLower(newStr)

	// Check for fix-related keywords
	fixKeywords := []string{"fix", "bug", "error", "issue", "problem", "correct"}
	for _, keyword := range fixKeywords {
		if strings.Contains(newLower, keyword) && !strings.Contains(oldLower, keyword) {
			return true
		}
	}

	return false
}

// isRefactoring analyzes if the edit looks like refactoring
func isRefactoring(toolInput map[string]interface{}) bool {
	oldStr, oldOk := toolInput["old_string"].(string)
	newStr, newOk := toolInput["new_string"].(string)

	if !oldOk || !newOk {
		return false
	}

	// If lengths are very different, it's likely refactoring
	if float64(len(newStr)) > float64(len(oldStr))*1.5 || float64(len(oldStr)) > float64(len(newStr))*1.5 {
		return true
	}

	// Check for refactor keywords
	refactorKeywords := []string{"refactor", "restructure", "reorganize", "cleanup"}
	newLower := strings.ToLower(newStr)
	for _, keyword := range refactorKeywords {
		if strings.Contains(newLower, keyword) {
			return true
		}
	}

	return false
}
