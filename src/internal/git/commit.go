package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsGitRepo checks if the given directory is inside a git repository
func IsGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// HasUncommittedChanges checks if there are uncommitted changes
func HasUncommittedChanges(dir string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %w", err)
	}

	return len(bytes.TrimSpace(output)) > 0, nil
}

// StageFiles stages the given files for commit, screening out build output
// first. Screening happens before `git add` deliberately: git hashes a file
// into .git/objects the moment it is added, so a binary caught after staging
// is already in the repository. Rejected paths are added to .gitignore and
// returned so the caller can report them.
func StageFiles(dir string, files []string) ([]Rejection, error) {
	approved, rejected := ScreenFiles(dir, files)

	if len(rejected) > 0 {
		EnsureGitignored(dir, rejected)
		if !IsInGitIgnore(dir, ".gitignore") {
			approved = append(approved, ".gitignore")
		}
	}

	if len(approved) == 0 {
		return rejected, nil
	}

	args := append([]string{"add", "--"}, approved...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	if err := cmd.Run(); err != nil {
		return rejected, fmt.Errorf("failed to stage files: %w", err)
	}

	return rejected, nil
}

// HasStagedChanges reports whether anything is already staged. When something
// is, someone is composing their own commit: an automated commit would land
// their work under a generated message and discard the reasoning they were
// about to write.
func HasStagedChanges(dir string) bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	return cmd.Run() != nil
}

// MidGitOperation reports whether a merge, rebase or cherry-pick is in flight.
func MidGitOperation(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	for _, marker := range []string{"MERGE_HEAD", "REBASE_HEAD", "CHERRY_PICK_HEAD", "rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(gitDir, marker)); err == nil {
			return true
		}
	}
	return false
}

// Commit creates a git commit with the given message
func Commit(dir, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// GetModifiedFiles returns a list of modified files from git status
func GetModifiedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get modified files: %w", err)
	}

	var files []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		// Parse git status output (format: "XY filename")
		file := strings.TrimSpace(line[3:])
		if file != "" {
			files = append(files, file)
		}
	}

	return files, nil
}

// IsInGitIgnore checks if a file is in .gitignore
func IsInGitIgnore(dir, file string) bool {
	cmd := exec.Command("git", "check-ignore", file)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// GetRepoRoot returns the root directory of the git repository
func GetRepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get repo root: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetRelativePath returns the relative path from repo root
func GetRelativePath(repoRoot, filePath string) (string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(repoRoot, absPath)
	if err != nil {
		return "", err
	}

	return relPath, nil
}

// PromptUser prompts the user for y/n input
func PromptUser(message string) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s (y/n): ", message)

	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		return false, err
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}
