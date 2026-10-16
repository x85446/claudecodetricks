// Package conductor owns durable scheduling and bounded execution.
package conductor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

var ErrConflict = errors.New("configuration revision conflict")
var ErrGated = errors.New("admission gated")

type Pair struct {
	Step       int    `json:"step"`
	Team       string `json:"team,omitempty"`
	Task       string `json:"task"`
	Validation string `json:"validation"`
	Digest     string `json:"digest"`
}
type Plan struct {
	Running     bool      `json:"running"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ProjectPath string    `json:"project_path"`
	State       string    `json:"state"`
	Staged      bool      `json:"staged"`
	Blocked     bool      `json:"blocked"`
	Reason      string    `json:"reason,omitempty"`
	Release     bool      `json:"release"`
	Started     time.Time `json:"started"`
	Complete    bool      `json:"complete"`
	Kind        string    `json:"kind,omitempty"`
}
type PlanSource interface {
	List(context.Context, model.Project) ([]Plan, error)
	Pair(context.Context, model.Project, string) (Pair, error)
	Checkpoint(context.Context, model.Assignment, model.Result) error
	Gate(context.Context, model.Project, time.Time) error
}

// OutcomeSource records parking and delivery without granting an AI filesystem authority.
type LeaseSource interface {
	Claim(context.Context, model.Assignment) error
	Heartbeat(context.Context, model.Assignment) error
	Release(context.Context, model.Assignment) error
}
type OutcomeSource interface {
	Outcome(context.Context, model.Assignment, model.Result) error
}
type Service struct {
	IssueReader  func(context.Context, model.Project) ([]Intake, error)
	Dir          string
	Source       PlanSource
	Clock        func() time.Time
	CollectUsage func(context.Context, model.Subscription) (model.Usage, error)
	PollInterval time.Duration
	GracePeriod  time.Duration
	eventIndexes map[string]eventIndex
	mu           sync.Mutex
	cancels      map[string]context.CancelFunc
}

func DefaultDir() string {
	if p := os.Getenv("ITERATE_CONDUCTOR_DIR"); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".claude", "iterate", "conductor")
}
func NewService(dir string, source PlanSource) *Service {
	if dir == "" {
		dir = DefaultDir()
	}
	return &Service{Dir: dir, Source: source, Clock: time.Now, PollInterval: 250 * time.Millisecond, GracePeriod: 2 * time.Second, cancels: map[string]context.CancelFunc{}}
}
func NewID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}
func Canonical(path string) (string, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	p, e = filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	fi, e := os.Stat(p)
	if e != nil {
		return "", e
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("project path is not a directory")
	}
	return filepath.Clean(p), nil
}
func PlanID(path, name string) string { return path + "\x00" + name }
func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) withLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := os.MkdirAll(s.Dir, 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(s.Dir, "store.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = lockFile(f); e != nil {
		return e
	}
	defer unlockFile(f)
	return fn()
}
func readJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if len(b) > 32<<20 {
		return errors.New("state file exceeds 32 MiB limit")
	}
	return json.Unmarshal(b, v)
}
func atomicJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return AtomicWrite(path, append(b, '\n'))
}
func AtomicWrite(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".conductor-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e == nil {
		e = d.Sync()
		d.Close()
	}
	return e
}

type Lease struct {
	Assignment   model.Assignment `json:"assignment"`
	PID          int              `json:"pid"`
	ProcessStart string           `json:"process_start"`
	OwnerPID     int              `json:"owner_pid"`
	OwnerStart   string           `json:"owner_start"`
	Started      time.Time        `json:"started"`
	Status       string           `json:"status"`
	Interrupt    bool             `json:"interrupt"`
	OccurrenceID string           `json:"occurrence_id,omitempty"`
}
type Progress struct {
	ReconcileSequence string          `json:"reconcile_sequence,omitempty"`
	AttemptsByState   map[string]int  `json:"attempts_by_state,omitempty"`
	Attempts          int             `json:"attempts"`
	Signature         string          `json:"signature"`
	LastProgress      time.Time       `json:"last_progress"`
	Started           time.Time       `json:"started"`
	Parked            bool            `json:"parked"`
	Reason            string          `json:"reason"`
	NextKind          string          `json:"next_kind,omitempty"`
	Diagnosed         map[string]bool `json:"diagnosed,omitempty"`
	LastResult        *model.Result   `json:"last_result,omitempty"`
	CompletedSteps    map[int]string  `json:"completed_steps,omitempty"`
}
type Notice struct {
	Key          string    `json:"key"`
	ProjectID    string    `json:"project_id"`
	Message      string    `json:"message"`
	At           time.Time `json:"at"`
	Acknowledged bool      `json:"acknowledged"`
}
type Intake struct {
	Key        string `json:"key"`
	ProjectID  string `json:"project_id"`
	Provider   string `json:"provider"`
	Repository string `json:"repository"`
	Issue      string `json:"issue"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	State      string `json:"state"`
}
type Watch struct {
	Attempts  int    `json:"attempts"`
	Signature string `json:"signature"`
	State     string `json:"state"`
}
type Runtime struct {
	Watches          map[string]Watch       `json:"watches"`
	IntakePoll       map[string]time.Time   `json:"intake_poll"`
	Version          int                    `json:"version"`
	Revision         int64                  `json:"revision"`
	Usage            map[string]model.Usage `json:"usage"`
	Occurrences      []model.Occurrence     `json:"occurrences"`
	Leases           map[string]Lease       `json:"leases"`
	Progress         map[string]Progress    `json:"progress"`
	LastProject      string                 `json:"last_project"`
	LastSubscription string                 `json:"last_subscription"`
	Notices          map[string]Notice      `json:"notices"`
	Intake           map[string]Intake      `json:"intake"`
}

func (s *Service) runtime() (Runtime, error) {
	var r Runtime
	e := readJSON(filepath.Join(s.Dir, "runtime.json"), &r)
	if e != nil && !os.IsNotExist(e) {
		return r, e
	}
	r.Version = model.SchemaVersion
	if r.Watches == nil {
		r.Watches = map[string]Watch{}
	}
	if r.IntakePoll == nil {
		r.IntakePoll = map[string]time.Time{}
	}
	if r.Usage == nil {
		r.Usage = map[string]model.Usage{}
	}
	if r.Leases == nil {
		r.Leases = map[string]Lease{}
	}
	if r.Progress == nil {
		r.Progress = map[string]Progress{}
	}
	if r.Notices == nil {
		r.Notices = map[string]Notice{}
	}
	if r.Intake == nil {
		r.Intake = map[string]Intake{}
	}
	return r, nil
}
func (s *Service) saveRuntime(r *Runtime) error {
	r.Revision++
	return atomicJSON(filepath.Join(s.Dir, "runtime.json"), r)
}
func (s *Service) Runtime() (r Runtime, e error) {
	e = s.withLock(func() error { var x error; r, x = s.runtime(); return x })
	return
}
func fmtPID(pid int) string { return fmt.Sprint(pid) }
