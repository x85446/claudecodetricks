package testmaster

import (
	"context"
	"fmt"
	"time"
)

// Status values a test can end a run with.
const (
	Pass        = "pass"
	Fail        = "fail"
	Skip        = "skip"
	Timeout     = "timeout"
	Missing     = "missing"     // the runner has no such test: the registry is stale
	NotRun      = "not-run"     // a blocking test failed first
	Interrupted = "interrupted" // Ctrl-C or the parent context ended
)

// Result is one test's outcome. Output is kept only for failures.
type Result struct {
	ID     string
	Status string
	Ms     int64
	Timed  bool   // Ms is a real measurement of this test
	Output []byte // what the AI reads in the failure log
	Repro  string // the one command that reruns just this test
	Exit   int
	Note   string // one-line cause, e.g. "build failed"
	Group  string // failures sharing one cause collapse to one report line
}

func (r Result) failed() bool { return r.Status == Fail || r.Status == Timeout }

// Unit is one invocation: a package of go tests, a pytest batch, one shell
// command. Every id in IDs gets exactly one Result back.
type Unit struct {
	Kind    string
	IDs     []string
	Display string
	Run     func(ctx context.Context) []Result
}

// adapter knows one toolchain.
type adapter interface {
	// plan batches entries of this kind into units.
	plan(p *Project, ids []string, floor time.Duration) []Unit
	// repro is the command that reruns exactly one test.
	repro(p *Project, e *Entry) string
	// discover lists every test the toolchain sees under a source.
	discover(ctx context.Context, p *Project, src Source) ([]Found, error)
	// identity is the key that says two entries are the same test.
	identity(e *Entry) string
}

// Found is a test discovery saw, with the id it proposes and a longer
// fallback for when that id already names a different test.
type Found struct {
	ID    string
	AltID string
	Entry Entry
}

var adapters = map[string]adapter{
	"go":     goAdapter{},
	"pytest": pytestAdapter{},
	"cargo":  cargoAdapter{},
	"shell":  shellAdapter{},
}

func adapterFor(kind string) (adapter, error) {
	if a, ok := adapters[kind]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("unknown kind %q (known: go, pytest, cargo, shell)", kind)
}

// fillMissing guarantees every id in a unit has a result.
func fillMissing(ids []string, got []Result, status, note string, out []byte) []Result {
	seen := map[string]bool{}
	for _, r := range got {
		seen[r.ID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			got = append(got, Result{ID: id, Status: status, Note: note, Output: out})
		}
	}
	return got
}
