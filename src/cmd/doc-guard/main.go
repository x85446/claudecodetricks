// doc-guard keeps documentation in the present tense.
//
// As a PostToolUse hook on Write and Edit it scans the text just written to
// a documentation file for change narration ("is now", "previously", "we
// renamed", "no longer") and hands every hit back to the model to rewrite.
// The write has already happened, so this never blocks a tool call; it
// prompts a correction. Anything it cannot parse it lets through.
//
// As a command it audits existing files:
//
//	doc-guard scan <file|dir>...   path:line<TAB>phrase<TAB>text, exit 1 on any hit
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/x85446/claudecodetricks/src/internal/docguard"
)

type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath  string `json:"file_path"`
		Content   string `json:"content"`
		NewString string `json:"new_string"`
		Edits     []struct {
			NewString string `json:"new_string"`
		} `json:"edits"`
	} `json:"tool_input"`
}

// hookOutput is Claude Code's PostToolUse envelope: "block" feeds reason
// back to the model as the next thing to act on.
type hookOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

const rule = "Documentation describes the system as it is now; git records how it changed. " +
	"Rewrite each line to state the current behavior as plain fact, or delete it if it exists only to record the change. " +
	"Leave a line only if it describes present behavior (e.g. a timeout after which a lock is no longer held)."

func main() {
	if len(os.Args) > 1 {
		os.Exit(cli(os.Args[1:]))
	}
	var in hookInput
	if json.NewDecoder(os.Stdin).Decode(&in) != nil {
		return
	}
	if in.ToolName != "Write" && in.ToolName != "Edit" && in.ToolName != "MultiEdit" {
		return
	}
	if !docguard.Applies(in.ToolInput.FilePath) {
		return
	}
	parts := []string{in.ToolInput.Content, in.ToolInput.NewString}
	for _, e := range in.ToolInput.Edits {
		parts = append(parts, e.NewString)
	}
	hits := docguard.Scan(strings.Join(parts, "\n"))
	if len(hits) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "doc-guard: %s has change narration.\n", in.ToolInput.FilePath)
	for i, h := range hits {
		if i == 8 {
			fmt.Fprintf(&b, "  … and %d more\n", len(hits)-i)
			break
		}
		fmt.Fprintf(&b, "  %q in: %s\n", h.Phrase, clip(h.Text))
	}
	b.WriteString(rule)
	_ = json.NewEncoder(os.Stdout).Encode(hookOutput{Decision: "block", Reason: b.String()})
}

func cli(args []string) int {
	if args[0] != "scan" || len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: doc-guard scan <file|dir>...   (no arguments: PostToolUse hook on stdin)")
		return 2
	}
	found := 0
	for _, root := range args[1:] {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if n := d.Name(); p != root && (n == ".git" || n == "node_modules" || n == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			if abs, _ := filepath.Abs(p); !docguard.Applies(abs) {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for _, h := range docguard.Scan(string(data)) {
				fmt.Printf("%s:%d\t%s\t%s\n", p, h.Line, h.Phrase, clip(h.Text))
				found++
			}
			return nil
		})
	}
	if found > 0 {
		return 1
	}
	return 0
}

func clip(s string) string {
	if r := []rune(s); len(r) > 160 {
		return string(r[:157]) + "..."
	}
	return s
}
