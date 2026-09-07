package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// MaxAutoCommitBytes is the size above which a file is never auto-committed.
const MaxAutoCommitBytes = 5 * 1024 * 1024

// artifactDirs are directories whose contents are build output by definition.
// bin/, build/, dist/ and vendor/ are deliberately absent: repos legitimately
// keep committable source in them, and the content checks below catch real
// binaries wherever they live.
var artifactDirs = map[string]bool{
	"node_modules": true, "target": true, "__pycache__": true,
	".venv": true, "venv": true, ".next": true, ".parcel-cache": true,
	"DerivedData": true, ".gradle": true, "Pods": true,
	".pytest_cache": true, ".mypy_cache": true, ".tox": true,
	"coverage": true, ".nyc_output": true,
}

var binaryExts = map[string]bool{
	".o": true, ".a": true, ".so": true, ".dylib": true, ".dll": true,
	".exe": true, ".lib": true, ".obj": true, ".class": true, ".jar": true,
	".war": true, ".pyc": true, ".pyo": true, ".wasm": true, ".bin": true,
	".app": true, ".dmg": true, ".pkg": true, ".deb": true, ".rpm": true,
	".msi": true, ".iso": true, ".img": true, ".qcow2": true, ".zip": true,
	".tar": true, ".gz": true, ".bz2": true, ".xz": true, ".7z": true,
	".rar": true, ".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
	".mp3": true, ".wav": true, ".flac": true, ".psd": true, ".sketch": true,
	".blend": true, ".db": true, ".sqlite": true, ".sqlite3": true,
}

// binaryMagic identifies executables and archives regardless of their name.
var binaryMagic = [][]byte{
	{0x7f, 'E', 'L', 'F'},
	{0xcf, 0xfa, 0xed, 0xfe},
	{0xce, 0xfa, 0xed, 0xfe},
	{0xca, 0xfe, 0xba, 0xbe},
	{'M', 'Z'},
	{'!', '<', 'a', 'r', 'c', 'h', '>'},
	{'P', 'K', 0x03, 0x04},
}

// gitignoreHeader marks the block this package manages in a .gitignore.
const gitignoreHeader = "# --- auto-commit guard: build output kept out of git ---"

// Rejection is a file that must not be committed, and why.
type Rejection struct {
	Path   string
	Reason string
}

// ScreenPath reports why a path must not be auto-committed, or "" when it is
// fine. It rejects only on positive evidence so ordinary source is never
// silently withheld from a commit.
func ScreenPath(dir, rel string) string {
	full := filepath.Join(dir, rel)

	parts := strings.Split(filepath.ToSlash(rel), "/")
	scan := parts
	if info, err := os.Stat(full); err != nil || !info.IsDir() {
		if len(parts) > 0 {
			scan = parts[:len(parts)-1]
		}
	}
	for _, part := range scan {
		if artifactDirs[part] {
			return fmt.Sprintf("artifact dir (%s/)", part)
		}
	}

	if binaryExts[strings.ToLower(filepath.Ext(rel))] {
		return "binary file type"
	}

	info, err := os.Stat(full)
	if err != nil {
		return "" // deleted or unreadable: let git handle it normally
	}
	if info.Size() > MaxAutoCommitBytes {
		return fmt.Sprintf("oversized (%d MB)", info.Size()/(1024*1024))
	}

	f, err := os.Open(full)
	if err != nil {
		return ""
	}
	defer f.Close()

	head := make([]byte, 8192)
	n, _ := f.Read(head)
	head = head[:n]

	for _, magic := range binaryMagic {
		if bytes.HasPrefix(head, magic) {
			return "executable/archive"
		}
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return "binary content"
	}
	return ""
}

// ScreenFiles partitions paths into those safe to stage and those rejected.
func ScreenFiles(dir string, files []string) ([]string, []Rejection) {
	var approved []string
	var rejected []Rejection
	for _, f := range files {
		if reason := ScreenPath(dir, f); reason != "" {
			rejected = append(rejected, Rejection{Path: f, Reason: reason})
			continue
		}
		approved = append(approved, f)
	}
	return approved, rejected
}

// EnsureGitignored appends rejected untracked paths to .gitignore so they stop
// being re-offered on every cycle. It returns the patterns it added and the
// rejected paths that are already tracked — .gitignore does nothing for those,
// and removing them from the index is not a call an automated hook may make.
func EnsureGitignored(dir string, rejected []Rejection) (added []string, tracked []string) {
	var patterns []string
	for _, r := range rejected {
		if IsInGitIgnore(dir, r.Path) {
			continue
		}
		if isTracked(dir, r.Path) {
			tracked = append(tracked, r.Path)
			continue
		}
		pattern := r.Path
		if strings.HasPrefix(r.Reason, "artifact dir") {
			if open := strings.Index(r.Reason, "("); open >= 0 {
				pattern = strings.TrimSuffix(r.Reason[open+1:], ")")
			}
		}
		if !slices.Contains(patterns, pattern) {
			patterns = append(patterns, pattern)
		}
	}
	if len(patterns) == 0 {
		return nil, tracked
	}

	path := filepath.Join(dir, ".gitignore")
	existing, _ := os.ReadFile(path)
	lines := strings.Split(string(existing), "\n")

	var fresh []string
	for _, p := range patterns {
		if !slices.Contains(lines, p) {
			fresh = append(fresh, p)
		}
	}
	if len(fresh) == 0 {
		return nil, tracked
	}

	var block strings.Builder
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		block.WriteString("\n")
	}
	if !strings.Contains(string(existing), gitignoreHeader) {
		block.WriteString("\n" + gitignoreHeader + "\n")
	}
	block.WriteString(strings.Join(fresh, "\n") + "\n")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, tracked
	}
	defer f.Close()
	if _, err := f.WriteString(block.String()); err != nil {
		return nil, tracked
	}
	return fresh, tracked
}

func isTracked(dir, file string) bool {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", file)
	cmd.Dir = dir
	return cmd.Run() == nil
}
