// Package testmaster runs a project's registered tests outside the model's
// context and reports only what failed. The registry at
// .claude/testmaster/registry.json is the database: one entry per test, with
// how to run it (kind + identity), how long it takes (measured), and its last
// result. Passing tests leave no trace beyond a count; each failure leaves a
// log under .claude/testmaster/failures/ for the AI to read when it needs to.
package testmaster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Tier boundaries, from the TESTMASTER contract: fast ≤10s, standard ≤2min.
const (
	FastMs     = 10_000
	StandardMs = 120_000
)

// StateDir is the project-relative home of every TESTMASTER file.
const StateDir = ".claude/testmaster"

// Kinds the runner knows how to batch and discover. "shell" runs any command
// and is how every other language gets in.
var Kinds = []string{"go", "pytest", "cargo", "shell"}

// Source is a place discovery looks for tests: a toolchain at a directory.
type Source struct {
	Kind string `json:"kind"`
	Dir  string `json:"dir"`
}

// LastRun is the whole batch's wall-clock and counts from the latest run.
type LastRun struct {
	TS        string `json:"ts"`
	Selection string `json:"selection"`
	Ms        int64  `json:"ms"`
	Passed    int    `json:"passed"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
	Missing   int    `json:"missing"`
	NotRun    int    `json:"not_run"`
}

// Entry is one test. Kind decides which identity fields matter:
// go → pkg+name, pytest → nodeid, cargo → pkg+target+name, shell → cmd.
// Dir (project-relative, default ".") is where the runner is invoked.
type Entry struct {
	Kind         string   `json:"kind,omitempty"`
	Name         string   `json:"name,omitempty"`
	Pkg          string   `json:"pkg,omitempty"`
	Target       string   `json:"target,omitempty"`
	NodeID       string   `json:"nodeid,omitempty"`
	Dir          string   `json:"dir,omitempty"`
	Cmd          string   `json:"cmd,omitempty"`
	Args         []string `json:"args,omitempty"`
	File         string   `json:"file,omitempty"`
	Blocking     bool     `json:"blocking,omitempty"`
	ParallelSafe *bool    `json:"parallel_safe,omitempty"`
	Runs         int      `json:"runs"`
	AvgMs        *int64   `json:"avg_ms"`
	LastMs       *int64   `json:"last_ms"`
	Tier         string   `json:"tier"`
	LastResult   string   `json:"last_result,omitempty"`
	Updated      string   `json:"updated,omitempty"`

	extra map[string]json.RawMessage
}

// Registry is registry.json. Unknown fields, at the top and in every entry,
// survive a load/save round trip untouched.
type Registry struct {
	Schema  int                 `json:"schema"`
	Updated string              `json:"updated,omitempty"`
	Runners map[string][]string `json:"runners,omitempty"`
	Sources []Source            `json:"sources,omitempty"`
	LastRun *LastRun            `json:"last_run,omitempty"`
	Tests   map[string]*Entry   `json:"tests"`

	extra map[string]json.RawMessage
}

func jsonKeys(v any) []string {
	t := reflect.TypeOf(v)
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if name, _, _ := strings.Cut(tag, ","); name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

func splitExtra(b []byte, known []string) (map[string]json.RawMessage, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	for _, k := range known {
		delete(all, k)
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}

func mergeExtra(b []byte, extra map[string]json.RawMessage) ([]byte, error) {
	if len(extra) == 0 {
		return b, nil
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	for k, v := range extra {
		if _, ok := all[k]; !ok {
			all[k] = v
		}
	}
	return marshal(all, "")
}

// marshal encodes without HTML escaping, so a command like `a && b` stays
// readable in the file instead of becoming `a \u0026\u0026 b`.
func marshal(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

type plainEntry Entry

func (e *Entry) UnmarshalJSON(b []byte) error {
	var p plainEntry
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	extra, err := splitExtra(b, jsonKeys(plainEntry{}))
	if err != nil {
		return err
	}
	*e = Entry(p)
	e.extra = extra
	return nil
}

func (e Entry) MarshalJSON() ([]byte, error) {
	b, err := marshal(plainEntry(e), "")
	if err != nil {
		return nil, err
	}
	return mergeExtra(b, e.extra)
}

type plainRegistry Registry

func (r *Registry) UnmarshalJSON(b []byte) error {
	var p plainRegistry
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	extra, err := splitExtra(b, jsonKeys(plainRegistry{}))
	if err != nil {
		return err
	}
	*r = Registry(p)
	r.extra = extra
	return nil
}

func (r Registry) MarshalJSON() ([]byte, error) {
	b, err := marshal(plainRegistry(r), "")
	if err != nil {
		return nil, err
	}
	return mergeExtra(b, r.extra)
}

// normalize fills Kind on entries written before the runner existed: a
// legacy `runner: make` or any non-`go test` command is a shell test, and a
// `go test` command with pkg+name is a go test.
func (e *Entry) normalize() {
	if e.Kind != "" {
		return
	}
	var legacy string
	if raw, ok := e.extra["runner"]; ok {
		_ = json.Unmarshal(raw, &legacy)
	}
	switch {
	case legacy != "make" && strings.HasPrefix(e.Cmd, "go test") && e.Pkg != "" && e.Name != "":
		e.Kind = "go"
	case e.Cmd != "":
		e.Kind = "shell"
	default:
		return
	}
	delete(e.extra, "runner")
}

// Parallel reports whether the test may share the machine with others.
// Unset means yes: the header's parallel=no is the declared exception.
func (e *Entry) Parallel() bool { return e.ParallelSafe == nil || *e.ParallelSafe }

// Measured reports whether the entry has a real timing behind its tier.
func (e *Entry) Measured() bool {
	return e.Runs > 0 && e.AvgMs != nil && (e.Tier == "fast" || e.Tier == "standard" || e.Tier == "slow")
}

// TierFor derives a tier from a measured average.
func TierFor(avgMs int64) string {
	switch {
	case avgMs <= FastMs:
		return "fast"
	case avgMs <= StandardMs:
		return "standard"
	default:
		return "slow"
	}
}

// Project is one repository's TESTMASTER state.
type Project struct {
	Root   string
	Reg    *Registry
	exists bool
}

// Find locates the project for a working directory: the nearest ancestor
// holding a registry, else the git root, else the directory itself.
func Find(start string) (*Project, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	for dir := abs; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, StateDir, "registry.json")); err == nil {
			return Open(dir)
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	root := abs
	if out, err := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	}
	return Open(root)
}

// Open loads the registry at root, or starts an empty one. The root is
// resolved through symlinks so it compares equal to the paths toolchains
// report (macOS's /var is /private/var).
func Open(root string) (*Project, error) {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	p := &Project{Root: root, Reg: &Registry{Schema: 1, Tests: map[string]*Entry{}}}
	b, err := os.ReadFile(p.path(StateDir, "registry.json"))
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, p.Reg); err != nil {
		return nil, fmt.Errorf("%s: %w", p.Rel(p.path(StateDir, "registry.json")), err)
	}
	if p.Reg.Tests == nil {
		p.Reg.Tests = map[string]*Entry{}
	}
	for _, e := range p.Reg.Tests {
		e.normalize()
	}
	p.exists = true
	return p, nil
}

// Exists reports whether a registry was on disk when the project opened.
func (p *Project) Exists() bool { return p.exists }

func (p *Project) path(parts ...string) string {
	return filepath.Join(append([]string{p.Root}, parts...)...)
}

// Rel renders a path for output: relative to the project root.
func (p *Project) Rel(path string) string {
	if r, err := filepath.Rel(p.Root, path); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return path
}

// DirOf resolves an entry's working directory.
func (p *Project) DirOf(e *Entry) string {
	if e.Dir == "" || e.Dir == "." {
		return p.Root
	}
	if filepath.IsAbs(e.Dir) {
		return e.Dir
	}
	return p.path(e.Dir)
}

// Save writes registry.json atomically.
func (p *Project) Save() error {
	p.Reg.Updated = now()
	b, err := marshal(p.Reg, "  ")
	if err != nil {
		return err
	}
	return p.writeState("registry.json", b)
}

// writeState replaces a file under StateDir atomically.
func (p *Project) writeState(name string, b []byte) error {
	dir := p.path(StateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if name == "registry.json" {
		p.exists = true
	}
	return nil
}

// FailureDir holds one log per currently failing test, plus the run lock.
// It ignores itself in git: everything in it is local and transient.
func (p *Project) FailureDir() string { return p.path(StateDir, "failures") }

func (p *Project) ensureFailureDir() error {
	dir := p.FailureDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gi := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gi); errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(gi, []byte("*\n"), 0o644)
	}
	return nil
}

// Lock takes the project's run lock so two runs never interleave registry
// writes. The kernel drops it when the process exits, however it exits.
func (p *Project) Lock() (func(), error) {
	if err := p.ensureFailureDir(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(p.FailureDir(), ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another testmaster process holds %s", p.Rel(f.Name()))
	}
	return func() { f.Close() }, nil
}

// LogPath is where a test's latest failure output lives.
func (p *Project) LogPath(id string) string {
	return filepath.Join(p.FailureDir(), safeName(id)+".log")
}

func safeName(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// IDs returns every registered id, sorted.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.Tests))
	for id := range r.Tests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Runner returns the argv prefix for a kind: the registry's override, else
// the built-in default.
func (r *Registry) Runner(kind string, def ...string) []string {
	if v, ok := r.Runners[kind]; ok && len(v) > 0 {
		return append([]string(nil), v...)
	}
	return append([]string(nil), def...)
}

func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }
