package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/conductor/provider"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fixtureSource struct {
	mu          sync.Mutex
	plans       []Plan
	done        map[string]int
	gated       bool
	checkpoints int
}

func (f *fixtureSource) List(_ context.Context, p model.Project) ([]Plan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Plan{}, f.plans...), nil
}
func (f *fixtureSource) Pair(_ context.Context, p model.Project, name string) (Pair, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	step := f.done[PlanID(p.Path, name)] + 1
	if step > 2 {
		return Pair{Digest: "delivery"}, nil
	}
	return Pair{Step: step, Task: "implement fixture", Validation: "verify fixture", Digest: fmt.Sprint("digest-", step)}, nil
}
func (f *fixtureSource) Checkpoint(_ context.Context, a model.Assignment, r model.Result) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := PlanID(a.ProjectPath, a.Plan)
	if f.done[key] < a.Step {
		f.done[key] = a.Step
		f.checkpoints++
	}
	return nil
}
func (f *fixtureSource) Gate(context.Context, model.Project, time.Time) error {
	if f.gated {
		return errors.New("manual owner")
	}
	return nil
}
func (f *fixtureSource) Outcome(_ context.Context, a model.Assignment, r model.Result) error {
	if a.Kind == "delivery" && r.Outcome == "completed" {
		f.mu.Lock()
		defer f.mu.Unlock()
		for i := range f.plans {
			if f.plans[i].Name == a.Plan {
				f.plans[i].Complete = true
			}
		}
	}
	return nil
}
func fixture(t *testing.T, behavior string) (*Service, *fixtureSource, model.Config) {
	t.Helper()
	dir := t.TempDir()
	project := t.TempDir()
	script := filepath.Join(dir, "agent.py")
	source := `import sys,json,re,time,threading,os
prompt,result=sys.argv[1:3]
m=re.search(r'identity fields in this template: (\{.*?\})\.',prompt)
r=json.loads(m.group(1))
behavior=` + fmt.Sprintf("%q", behavior) + `
if behavior=='contract':
 for instruction in ('contract expansion, parity repair, or regression', 'continue instead of requesting a human decision', 'a secret no agent has, physical access, or an external party', 'Do not edit plan checkboxes or start another pair'):
  assert instruction in prompt, instruction
 print(json.dumps({'type':'activity','kind':'activity','provider_id':'contract-policy','message':'bounded contract-expansion policy received'}),flush=True)
if behavior=='sleep': time.sleep(10)
if behavior=='plain': print('human-readable agent log')
if behavior in ('descendant','holdtree'):
 import subprocess
 child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(20)'])
 with open(result+'.child','w') as f:f.write(str(child.pid))
if behavior in ('hold','holdtree'):
 with open('uncommitted.txt','w') as f:f.write('owned fixture edit\n')
 print(json.dumps({'type':'activity','kind':'activity','provider_id':'fixture-ready','message':'waiting at checkpoint'}),flush=True)
 while not os.path.exists(result+'.continue'): time.sleep(.01)
 if behavior=='holdtree': child.terminate();child.wait()
if behavior=='burst':
 def err():
  for i in range(1500): print('e'*1024,file=sys.stderr)
 th=threading.Thread(target=err);th.start()
 for i in range(1500): print(json.dumps({'type':'activity','kind':'activity','provider_id':str(i),'message':'x'*1024}))
 th.join()
if behavior=='bad': r['assignment_id']='wrong'
if behavior=='partial': r.update(outcome='partial',blocker='unfinished work',evidence=['validated partial work'])
elif behavior=='retry': r.update(outcome='retry',blocker='fixed failure')
else: r.update(outcome='completed',task_complete=True,validation_met=True,evidence=['fixture verified'])
if behavior!='missing':
 with open(result,'w') as f: json.dump(r,f)
`
	if e := os.WriteFile(script, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("python fixture interpreter unavailable")
	}
	src := &fixtureSource{plans: []Plan{{Name: "alpha", Staged: true, State: "queued"}, {Name: "beta", Staged: true, State: "queued"}}, done: map[string]int{}}
	s := NewService(filepath.Join(dir, "store"), src)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	s.PollInterval = 20 * time.Millisecond
	s.GracePeriod = 40 * time.Millisecond
	c := model.Config{Version: 2, Enabled: true, PowerBands: []model.PowerBand{budget.Universal()}, Projects: []model.Project{{ID: "project", Name: "fixture", Path: project, Enabled: true}}, Subscriptions: []model.Subscription{{ID: "account", Name: "Fixture", Enabled: true, Adapter: "jsonl", ResultTransport: "argv_file", Executable: python, Args: []string{script, "{prompt}", "{result}"}, PoolID: "pool", AccountID: "identity", Sequences: []model.Sequence{{ID: "sequence", Name: "build"}}}}, Automations: []model.Automation{{ID: "automation", Name: "always", ProjectID: "project", Enabled: true, Mode: "keep_working", Entries: []model.Entry{{ID: "entry", SequenceID: "sequence", MaxBand: model.Red}}}}}
	c, e = s.Apply(0, c)
	if e != nil {
		t.Fatal(e)
	}
	usage := func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
		return model.Usage{SubscriptionID: sub.ID, PoolID: sub.PoolID, AccountID: sub.AccountID, Source: "fixture authentic scope", ObservedAt: s.now(), Supported: true, Weekly: &model.Allowance{UsedPercent: 10, StartsAt: now.Add(-4 * 24 * time.Hour), ResetsAt: now.Add(3 * 24 * time.Hour), ResetTrusted: true, Epoch: "epoch"}, Short: &model.Allowance{UsedPercent: 10, StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "short"}}, nil
	}
	s.CollectUsage = usage
	u, _ := usage(context.Background(), c.Subscriptions[0])
	if e = s.PutUsage(u); e != nil {
		t.Fatal(e)
	}
	return s, src, c
}

// TESTMASTER: id=indigobird-authority-shared tier=fast parallel=yes
func TestIndigobirdConfiguration(t *testing.T) {
	s, _, c := fixture(t, "")
	other := NewService(s.Dir, s.Source)
	next := c
	next.Projects[0].Name = "edited"
	saved, e := other.Apply(c.Revision, next)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(c.Revision, c); !errors.Is(e, ErrConflict) {
		t.Fatalf("conflict %v", e)
	}
	reloaded, e := s.Config()
	if e != nil || reloaded.Revision != saved.Revision || reloaded.Projects[0].Name != "edited" {
		t.Fatal(reloaded, e)
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	bad := reloaded
	bad.Subscriptions[0].PowerBandID = "missing"
	if _, e = s.Apply(reloaded.Revision, bad); e == nil {
		t.Fatal("accepted broken ref")
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if string(before) != string(after) {
		t.Fatal("failed save mutated config")
	}
	_ = os.WriteFile(filepath.Join(s.Dir, ".conductor-interrupted"), []byte("partial"), 0600)
	again, e := other.Config()
	if e != nil || again.Revision != reloaded.Revision {
		t.Fatal("interrupted temp affected config")
	}
}

// TESTMASTER: id=indigobird-design-sync-used-migration tier=fast parallel=yes
func TestIndigobirdMigration(t *testing.T) {
	s, _, c := fixture(t, "")
	c.Version = 1
	c.Subscriptions[0].Sequences[0].Name = "Fixture · build"
	b, _ := json.Marshal(c)
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	subs := raw["subscriptions"].([]any)
	subs[0].(map[string]any)["overdrive_windows"] = []any{map[string]any{"id": "over", "name": "target", "enabled": false, "target_remaining_percent": 40, "recurrence": map[string]any{"days": []int{1}, "start": "00:00", "end": "00:00", "timezone": "UTC", "every_weeks": 1}, "end_mode": "recurrence"}}
	b, _ = json.Marshal(raw)
	if e := AtomicWrite(filepath.Join(s.Dir, "config.json"), b); e != nil {
		t.Fatal(e)
	}
	m, e := s.Migrate()
	if e != nil {
		t.Fatal(e)
	}
	if m.Subscriptions[0].OverdriveWindows[0].TargetUsedPercent != 60 || m.Subscriptions[0].Sequences[0].Name != "build" {
		t.Fatal(m)
	}
	n, e := s.Migrate()
	if e != nil || n.Revision != m.Revision {
		t.Fatal(n, e)
	}
}

// TESTMASTER: id=indigobird-ordered-profiles-fallback tier=fast parallel=yes
func TestIndigobirdAdmission(t *testing.T) {
	s, src, c := fixture(t, "")
	c.Automations[0].Entries = []model.Entry{{ID: "first", SequenceID: "sequence", MaxBand: model.Green}, {ID: "second", SequenceID: "sequence", MaxBand: model.Yellow}}
	c, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	used := 65.
	d, e := s.Evaluate(context.Background(), EvaluateRequest{Samples: map[string]SampleAccount{"account": {WeeklyUsed: &used, Availability: "ready"}}})
	if e != nil || d.Selected == nil || d.Selected.EntryID != "second" {
		t.Fatalf("fallback: %+v %v", d, e)
	}
	src.gated = true
	d, e = s.Status(context.Background())
	if e != nil || d.Selected != nil {
		t.Fatal("manual gate", d, e)
	}
	src.gated = false
	c.Projects[0].Enabled = false
	_, _ = s.Apply(c.Revision, c)
	d, _ = s.Status(context.Background())
	if d.Queues[0].Depth != 2 || d.Selected != nil {
		t.Fatal("disabled queue lost", d)
	}
}

// TESTMASTER: id=indigobird-execution-boundary tier=fast parallel=yes
func TestIndigobirdSupervisor(t *testing.T) {
	s, src, _ := fixture(t, "")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		out, e := s.Tick(ctx)
		if e != nil || out.Result == nil || out.Result.Outcome != "completed" {
			t.Fatalf("tick %d: %+v %v", i, out, e)
		}
	}
	if src.checkpoints != 2 {
		t.Fatal("did not bound pair", src.checkpoints)
	}
	runs, e := s.Runs()
	if e != nil || len(runs) != 3 {
		t.Fatal(runs, e)
	}
	d, _ := s.Status(ctx)
	if d.Queues[0].Depth != 1 {
		t.Fatal("completion queue", d)
	}
}

// TESTMASTER: id=indigobird-execution-invalid-result tier=fast parallel=yes
func TestIndigobirdInvalidResults(t *testing.T) {
	for _, behavior := range []string{"bad", "missing"} {
		t.Run(behavior, func(t *testing.T) {
			s, src, _ := fixture(t, behavior)
			out, e := s.Tick(context.Background())
			if e == nil || out.Result.Outcome != "retry" || src.checkpoints != 0 {
				t.Fatal(out, e)
			}
			r, _ := s.Runtime()
			if len(r.Leases) != 0 {
				t.Fatal("lease retained")
			}
			if r.Progress[PlanID(out.Assignment.ProjectPath, "alpha")].NextKind != "diagnosis" {
				t.Fatal("no bounded diagnosis")
			}
		})
	}
}

// TESTMASTER: id=indigobird-stream-drain tier=standard parallel=yes
func TestIndigobirdStreamDrain(t *testing.T) {
	s, src, _ := fixture(t, "burst")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, e := s.Tick(ctx)
	if e != nil || src.checkpoints != 1 {
		t.Fatal(out, e)
	}
	page, e := s.Events(out.Assignment.RunID, 0, 500)
	if e != nil || len(page.Events) != 500 {
		t.Fatal(len(page.Events), e)
	}
	if page.Cursor == 0 {
		t.Fatal("missing cursor")
	}
	ev := model.Event{ProviderID: "stable", Kind: "tokens", InputTokens: 100, OutputTokens: 2, Cumulative: true, SessionID: "session"}
	if e = s.RecordEvent(*out.Assignment, ev); e != nil {
		t.Fatal(e)
	}
	_ = s.RecordEvent(*out.Assignment, ev)
	ev.ProviderID = "next"
	ev.InputTokens = 110
	_ = s.RecordEvent(*out.Assignment, ev)
	page, e = s.Events(out.Assignment.RunID, 0, 500)
	if e != nil || page.InputTokens != 110 {
		t.Fatal("token duplicate", page.InputTokens, e)
	}
}

// TESTMASTER: id=indigobird-window-controls-interrupt tier=fast parallel=yes
func TestIndigobirdInterruptAndConcurrency(t *testing.T) {
	s, src, c := fixture(t, "sleep")
	c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "short", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "12:00", End: "12:01", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "window"
	c.Automations[0].OnWindowEnd = "interrupt"
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	var seconds atomic.Int64
	base := s.now()
	s.Clock = func() time.Time { return base.Add(time.Duration(seconds.Load()) * time.Second) }
	done := make(chan error, 1)
	go func() { _, e := s.Tick(context.Background()); done <- e }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := s.Runtime()
		active := false
		for _, l := range r.Leases {
			if l.PID > 0 {
				active = true
			}
		}
		if active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	other := NewService(s.Dir, src)
	other.Clock = s.Clock
	other.CollectUsage = s.CollectUsage
	out, e := other.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("duplicate admission", out, e)
	}
	seconds.Store(61)
	select {
	case e = <-done:
		if e == nil {
			t.Fatal("interrupt claimed success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("interrupt failed")
	}
	r, _ := s.Runtime()
	if len(r.Leases) != 0 || src.checkpoints != 0 {
		t.Fatal("unclean interruption", r)
	}
}

// TESTMASTER: id=indigobird-stream-failure tier=fast parallel=yes
func TestIndigobirdIdleAndIntakeDedupe(t *testing.T) {
	s, src, c := fixture(t, "")
	src.plans = nil
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal(out, e)
	}
	c.Projects[0].IntakeEnabled = true
	_, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	items := []Intake{{Provider: "github", Repository: "fixture/repo", Issue: "1", Title: "repair"}}
	n, e := s.ImportIssues("project", items)
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	n, e = s.ImportIssues("project", items)
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	r, _ := s.Runtime()
	if len(r.Intake) != 1 {
		t.Fatal(r)
	}
}

// TESTMASTER: id=indigobird-window-controls-occurrence tier=fast parallel=yes
func TestIndigobirdOnePlan(t *testing.T) {
	s, src, c := fixture(t, "")
	c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "all", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "00:00", End: "00:00", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "window"
	c.Automations[0].RunLimit = "one_plan"
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		if _, e = s.Tick(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil || src.checkpoints != 2 {
		t.Fatal("second plan claimed", out, e)
	}
}

// TESTMASTER: id=indigobird-execution-recovery tier=fast parallel=yes
func TestIndigobirdRecovery(t *testing.T) {
	s, src, _ := fixture(t, "")
	out, e := s.Tick(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	a := *out.Assignment
	r, _ := s.Runtime()
	r.Leases[a.ID] = Lease{Assignment: a, OwnerPID: 999999, OwnerStart: "gone"}
	if e = s.withLock(func() error { return s.saveRuntime(&r) }); e != nil {
		t.Fatal(e)
	}
	if e = s.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	if src.checkpoints != 1 {
		t.Fatal("checkpoint replayed")
	}
	for _, run := range []string{"../escape", "a/b"} {
		if _, e = s.Events(run, 0, 10); e == nil {
			t.Fatal("unsafe run path")
		}
	}
	if !strings.Contains(a.Task, "fixture") {
		t.Fatal("task snapshot missing")
	}
}

// TESTMASTER: id=indigobird-window-controls-finish-plan tier=fast parallel=yes
func TestIndigobirdWindowEndModes(t *testing.T) {
	for _, mode := range []string{"finish_checkpoint", "finish_plan"} {
		t.Run(mode, func(t *testing.T) {
			s, _, c := fixture(t, "")
			c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "minute", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "12:00", End: "12:01", Timezone: "UTC", EveryWeeks: 1}}}
			c.Automations[0].Mode = "scheduled"
			c.Automations[0].WindowID = "window"
			c.Automations[0].OnWindowEnd = mode
			_, e := s.Apply(c.Revision, c)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Tick(context.Background()); e != nil {
				t.Fatal(e)
			}
			base := s.now()
			s.Clock = func() time.Time { return base.Add(2 * time.Minute) }
			out, e := s.Tick(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			if mode == "finish_checkpoint" && out.Assignment != nil {
				t.Fatal("checkpoint launched beyond end")
			}
			if mode == "finish_plan" {
				if out.Assignment == nil || out.Assignment.Plan != "alpha" {
					t.Fatal("continuation missing")
				}
				if _, e = s.Tick(context.Background()); e != nil {
					t.Fatal(e)
				}
				out, e = s.Tick(context.Background())
				if e != nil || out.Assignment != nil {
					t.Fatal("continuation claimed another plan", out, e)
				}
			}
		})
	}
}

// TESTMASTER: id=indigobird-execution-retry tier=fast parallel=yes
func TestIndigobirdRetryParking(t *testing.T) {
	s, src, _ := fixture(t, "retry")
	for i := 0; i < 5; i++ {
		out, e := s.Tick(context.Background())
		if e != nil || out.Assignment == nil {
			t.Fatal("retry cycle", i, out, e)
		}
	}
	r, _ := s.Runtime()
	if len(r.Notices) != 1 || src.checkpoints != 0 {
		t.Fatal("parking notice", r)
	}
	out, e := s.Tick(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if out.Assignment != nil && out.Assignment.Plan == "alpha" {
		t.Fatal("parked plan retried")
	}
	r, _ = s.Runtime()
	if len(r.Notices) != 1 {
		t.Fatal("duplicate notice")
	}
}

// TESTMASTER: id=indigobird-window-controls-drain tier=fast parallel=yes
func TestIndigobirdDrainAndPoolIdentity(t *testing.T) {
	s, src, c := fixture(t, "")
	c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "day", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "00:00", End: "00:00", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "window"
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 6; i++ {
		if _, e = s.Tick(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	if src.checkpoints != 4 {
		t.Fatal("queue not drained", src.checkpoints)
	}
	d, _ := s.Status(context.Background())
	if d.Queues[0].Depth != 0 {
		t.Fatal(d)
	}
	u, _ := s.CollectUsage(context.Background(), c.Subscriptions[0])
	u.AccountID = "other"
	if e = s.PutUsage(u); e == nil {
		t.Fatal("mismatched identity admitted")
	}
}

// TESTMASTER: id=indigobird-authority-no-ai tier=fast parallel=yes
func TestIndigobirdLifecycleAndDraft(t *testing.T) {
	s, src, c := fixture(t, "")
	if e := s.ControlExpected(context.Background(), "pause", "", c.Revision-1); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e := s.ControlExpected(context.Background(), "pause", "", c.Revision); e != nil {
		t.Fatal(e)
	}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal(out, e)
	}
	if e = s.Control(context.Background(), "resume", ""); e != nil {
		t.Fatal(e)
	}
	src.plans = nil
	yes := true
	d, e := s.Evaluate(context.Background(), EvaluateRequest{ProjectID: "project", AutomationID: "automation", QueueAvailable: &yes})
	if e != nil || d.Selected == nil || d.Selected.Plan.Name != "sample-plan" {
		t.Fatal(d, e)
	}
	live, _ := s.Status(context.Background())
	if live.Queues[0].Depth != 0 {
		t.Fatal("sample persisted")
	}
	c, _ = s.Config()
	c.PowerBands = nil
	if _, e = s.Apply(c.Revision, c); e == nil {
		t.Fatal("deleted Universal accepted")
	}
}

// TESTMASTER: id=indigobird-execution-intake tier=fast parallel=yes
func TestIndigobirdBoundedIntake(t *testing.T) {
	s, src, c := fixture(t, "")
	src.plans = nil
	c.Projects[0].IntakeEnabled = true
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	s.IssueReader = func(context.Context, model.Project) ([]Intake, error) {
		calls++
		return []Intake{{Provider: "github", Repository: "fixture/repo", Issue: "1", Title: "fix fixture"}}, nil
	}
	n, e := s.FetchIssues(context.Background(), "project")
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment == nil || out.Assignment.Kind != "intake" || !strings.Contains(out.Assignment.Task, "Do not queue") {
		t.Fatal(out, e)
	}
	n, e = s.FetchIssues(context.Background(), "project")
	if e != nil || n != 0 || calls != 2 {
		t.Fatal(n, e)
	}
	out, e = s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("intake repeated", out, e)
	}
}

// TESTMASTER: id=indigobird-projects-watch tier=fast parallel=yes
func TestIndigobirdWatchAndFreshness(t *testing.T) {
	s, src, c := fixture(t, "")
	src.plans = nil
	c.Projects[0].WatchBound = 2
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		out, e := s.Tick(context.Background())
		if e != nil || out.Assignment != nil {
			t.Fatal(out, e)
		}
	}
	d, _ := s.Status(context.Background())
	if d.State != "stood_down" {
		t.Fatal(d.State)
	}
	src.plans = []Plan{{Name: "new", Staged: true, State: "queued"}}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment == nil {
		t.Fatal("new staged work did not reactivate", out, e)
	}
	s.CollectUsage = func(context.Context, model.Subscription) (model.Usage, error) {
		return model.Usage{}, errors.New("provider unavailable")
	}
	out, e = s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("old usage reused after failure", out, e)
	}
}

// TESTMASTER: id=indigobird-ordered-profiles-readiness tier=fast parallel=yes
func TestIndigobirdMissingExecutableFallback(t *testing.T) {
	s, _, c := fixture(t, "")
	sub := c.Subscriptions[0]
	sub.ID = "missing"
	sub.Name = "Missing"
	sub.Executable = "/missing/conductor-fixture-cli"
	sub.Sequences = []model.Sequence{{ID: "missing-sequence", Name: "build"}}
	c.Subscriptions = append(c.Subscriptions, sub)
	c.Automations[0].Entries = []model.Entry{{ID: "missing-first", SequenceID: "missing-sequence", MaxBand: model.Red}, {ID: "available-second", SequenceID: "sequence", MaxBand: model.Red}}
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := s.CollectUsage(context.Background(), sub)
	if e = s.PutUsage(u); e != nil {
		t.Fatal(e)
	}
	d, e := s.Status(context.Background())
	if e != nil || d.Selected == nil || d.Selected.EntryID != "available-second" {
		t.Fatal(d, e)
	}
	if d.Candidates[0].Code != "executable_unavailable" {
		t.Fatal(d.Candidates[0])
	}
}

// TESTMASTER: id=indigobird-stream-failure tier=fast parallel=yes
func TestIndigobirdStorageFailure(t *testing.T) {
	s, _, _ := fixture(t, "")
	a := model.Assignment{ID: "a", RunID: "storage", ProjectPath: "/fixture", Plan: "p"}
	dir, _ := s.runDir(a.RunID)
	if e := os.MkdirAll(filepath.Join(dir, "events.jsonl"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := s.RecordEvent(a, model.Event{Kind: "activity"}); e == nil {
		t.Fatal("storage failure ignored")
	}
}

// TESTMASTER: id=indigobird-execution-partial tier=fast parallel=yes
func TestIndigobirdPartialDelivery(t *testing.T) {
	s, _, _ := fixture(t, "partial")
	out, e := s.Tick(context.Background())
	if e != nil || out.Result.Outcome != "partial" {
		t.Fatal(out, e)
	}
	r, _ := s.Runtime()
	p := r.Progress[PlanID(out.Assignment.ProjectPath, "alpha")]
	if p.Parked || p.NextKind != "delivery" {
		t.Fatal("partial not routed to bounded delivery", p)
	}
	out, e = s.Tick(context.Background())
	if e != nil || out.Assignment.Kind != "delivery" || !strings.Contains(out.Assignment.Task, "Land only") {
		t.Fatal(out, e)
	}
	r, _ = s.Runtime()
	p = r.Progress[PlanID(out.Assignment.ProjectPath, "alpha")]
	if !p.Parked {
		t.Fatal("partial delivery did not retain unfinished parked work")
	}
}

// TESTMASTER: id=indigobird-projects-fairness tier=fast parallel=yes
func TestIndigobirdFairProjects(t *testing.T) {
	s, _, c := fixture(t, "")
	for _, id := range []string{"project-b", "project-c"} {
		p := c.Projects[0]
		p.ID = id
		p.Path = t.TempDir()
		c.Projects = append(c.Projects, p)
		a := c.Automations[0]
		a.ID = "automation-" + id
		a.ProjectID = id
		a.Entries = []model.Entry{{ID: "entry-" + id, SequenceID: "sequence", MaxBand: model.Red}}
		c.Automations = append(c.Automations, a)
	}
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		out, e := s.Tick(context.Background())
		if e != nil || out.Assignment == nil {
			t.Fatal(out, e)
		}
		seen[out.Assignment.ProjectID] = true
	}
	if len(seen) != 3 {
		t.Fatal("project starvation", seen)
	}
}

// TESTMASTER: id=indigobird-execution-process-tree tier=fast parallel=yes
func TestIndigobirdOwnedDescendantCleanup(t *testing.T) {
	s, src, _ := fixture(t, "descendant")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := s.Tick(ctx)
	if e == nil || out.Result.Outcome == "completed" || src.checkpoints != 0 {
		t.Fatal("background descendants accepted", out, e)
	}
	r, _ := s.Runtime()
	if len(r.Leases) != 0 {
		t.Fatal("ownership not released after cleanup")
	}
	runs, _ := s.Runs()
	for _, run := range runs {
		if processAlive(run.PID, run.ProcessStart) {
			t.Fatal("owned leader survived")
		}
	}
}

// TESTMASTER: id=indigobird-providers-generic-stream tier=fast parallel=yes
func TestIndigobirdGenericDiagnostics(t *testing.T) {
	s, src, c := fixture(t, "plain")
	c.Subscriptions[0].Adapter = "generic"
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.Tick(context.Background())
	if e != nil || src.checkpoints != 1 {
		t.Fatal("generic diagnostic output blocked valid result", out, e)
	}
}

// TESTMASTER: id=indigobird-stream-redaction tier=fast parallel=yes
func TestIndigobirdEventRedactionAndRecovery(t *testing.T) {
	s, _, _ := fixture(t, "")
	a := model.Assignment{ID: "a", RunID: "recover-events", ProjectPath: "/fixture", Plan: "p"}
	ev := model.Event{Kind: "activity", ProviderID: "same", Message: "Bearer abcdefghijklmnop token api_key=secretpass", Data: json.RawMessage(`{"secret":"hidden"}`)}
	if e := s.RecordEvent(a, ev); e != nil {
		t.Fatal(e)
	}
	other := NewService(s.Dir, s.Source)
	if e := other.RecordEvent(a, ev); e != nil {
		t.Fatal(e)
	}
	page, e := other.Events(a.RunID, 0, 100)
	if e != nil || len(page.Events) != 1 || strings.Contains(page.Events[0].Message, "secretpass") || page.Events[0].Data != nil {
		t.Fatal(page, e)
	}
}

// TESTMASTER: id=indigobird-settings-subscription-prefix tier=fast parallel=yes
func TestIndigobirdConfigReferences(t *testing.T) {
	s, _, c := fixture(t, "")
	c.Subscriptions[0].Name = "Tiger"
	c.Subscriptions[0].Sequences[0].Name = "custom suffix"
	saved, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	if model.SequenceName(saved.Subscriptions[0], saved.Subscriptions[0].Sequences[0]) != "Tiger · custom suffix" {
		t.Fatal("derived prefix")
	}
	saved.Subscriptions[0].Sequences = nil
	if _, e = s.Apply(saved.Revision, saved); e == nil {
		t.Fatal("referenced sequence deleted")
	}
	saved, _ = s.Config()
	saved.Automations[0].OnlyOverdrive = true
	saved.Automations[0].HonorOverdrive = false
	if _, e = s.Apply(saved.Revision, saved); e == nil {
		t.Fatal("contradictory override")
	}
	if _, e = DecodeConfig([]byte(`{} {}`)); e == nil {
		t.Fatal("trailing JSON accepted")
	}
}

// TESTMASTER: id=indigobird-stream-hook-dedupe tier=fast parallel=yes
func TestIndigobirdClaudeHookStreamDedupe(t *testing.T) {
	s, _, _ := fixture(t, "")
	a := model.Assignment{ID: "claude-assignment", RunID: "claude-events", ProjectPath: "/fixture", Plan: "p", Subscription: model.Subscription{Adapter: "claude"}}
	r, _ := s.Runtime()
	r.Leases[a.ID] = Lease{Assignment: a}
	if e := s.withLock(func() error { return s.saveRuntime(&r) }); e != nil {
		t.Fatal(e)
	}
	if e := s.Observe(a.ID, model.Event{ProviderID: "tool:one:start", Kind: "tool", SessionID: "session"}); e != nil {
		t.Fatal(e)
	}
	raw := `{"type":"assistant","uuid":"message","session_id":"session","message":{"usage":{"input_tokens":10,"output_tokens":3},"content":[{"type":"tool_use","id":"one","name":"Bash","input":{"secret":"hidden"}},{"type":"tool_use","id":"two","name":"Read"}]}}
{"type":"user","session_id":"session","message":{"content":[{"type":"tool_result","tool_use_id":"one","content":"secret result"}]}}
`
	if e := provider.Decode(context.Background(), "claude", strings.NewReader(raw), func(ev model.Event) error { return s.RecordEvent(a, ev) }); e != nil {
		t.Fatal(e)
	}
	if e := s.Observe(a.ID, model.Event{ProviderID: "tool:one:end", Kind: "tool", SessionID: "session"}); e != nil {
		t.Fatal(e)
	}
	page, e := s.Events(a.RunID, 0, 100)
	if e != nil || len(page.Events) != 4 || page.InputTokens != 10 {
		t.Fatal("hook stream dedupe lost usage", page, e)
	}
	_ = s.RecordEvent(a, model.Event{ProviderID: "delta", Kind: "usage", SessionID: "session", InputTokens: 10})
	page, e = s.Events(a.RunID, 0, 100)
	if e != nil || page.InputTokens != 10 {
		t.Fatal("cumulative and delta double counted", page, e)
	}
}

// TESTMASTER: id=indigobird-execution-orphan-isolation tier=fast parallel=yes
func TestIndigobirdOrphanIsolation(t *testing.T) {
	s, src, _ := fixture(t, "")
	cmd := exec.Command("sleep", "30")
	processGroup(cmd)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	identity := processIdentity(cmd.Process.Pid)
	t.Cleanup(func() { _ = signalOwned(cmd.Process.Pid, identity, true); _ = cmd.Wait() })
	a := model.Assignment{ID: "orphan", RunID: "orphan-run", ProjectID: "other-project", ProjectPath: t.TempDir(), Plan: "orphan-plan", Subscription: model.Subscription{PoolID: "other-pool"}}
	r, _ := s.Runtime()
	r.Leases[a.ID] = Lease{Assignment: a, PID: cmd.Process.Pid, ProcessStart: identity, OwnerPID: 999999, OwnerStart: "gone"}
	if e := s.withLock(func() error { return s.saveRuntime(&r) }); e != nil {
		t.Fatal(e)
	}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment == nil || src.checkpoints != 1 {
		t.Fatal("orphan blocked unrelated queue", out, e)
	}
	r, _ = s.Runtime()
	if r.Leases[a.ID].Status != "orphaned" || len(r.Notices) != 1 || !processAlive(cmd.Process.Pid, identity) {
		t.Fatal("orphan ownership lost")
	}
}

// TESTMASTER: id=indigobird-band-simulator-sample-epoch tier=fast parallel=yes
func TestIndigobirdSampleEpochAndDaySeven(t *testing.T) {
	s, _, c := fixture(t, "")
	c.Subscriptions[0].TypicalReset.Timezone = "America/Chicago"
	_, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := s.Runtime()
	originalReset := before.Usage["account"].Weekly.ResetsAt
	now := s.now().AddDate(0, 0, -21)
	used := 30.
	sample := SampleAccount{WeeklyUsed: &used, Availability: "ready"}
	d, e := s.BudgetPreview(context.Background(), BudgetPreviewRequest{SubscriptionID: "account", Now: &now, Sample: &sample, MaxBand: model.Red})
	if e != nil || !d.HardAllowed {
		t.Fatal("sample retained live epoch", d, e)
	}
	r := before
	applySamples(c, &r, now, map[string]SampleAccount{"account": sample})
	oldReset := originalReset.In(time.UTC)
	newReset := r.Usage["account"].Weekly.ResetsAt.In(time.UTC)
	if oldReset.Weekday() != newReset.Weekday() || oldReset.Hour() != newReset.Hour() {
		t.Fatal("reset clock shifted")
	}
	day := 7.
	sample.Day = &day
	d, e = s.BudgetPreview(context.Background(), BudgetPreviewRequest{SubscriptionID: "account", Sample: &sample, MaxBand: model.Red})
	if e != nil || !d.HardAllowed {
		t.Fatal("day seven malformed", d, e)
	}
	after, _ := s.Runtime()
	if !after.Usage["account"].Weekly.ResetsAt.Equal(originalReset) {
		t.Fatal("sample changed live epoch")
	}
	used = 100
	d, e = s.BudgetPreview(context.Background(), BudgetPreviewRequest{SubscriptionID: "account", Sample: &sample, MaxBand: model.Red})
	if e != nil || d.HardAllowed {
		t.Fatal("sample exhaustion hidden", d, e)
	}
}

// TESTMASTER: id=indigobird-ordered-profiles-contention tier=fast parallel=yes
func TestIndigobirdSeparateProcessLease(t *testing.T) {
	if dir := os.Getenv("INDIGOBIRD_LEASE_CHILD"); dir != "" {
		src := &fixtureSource{plans: []Plan{{Name: "alpha", Staged: true, State: "queued"}}, done: map[string]int{}}
		s := NewService(dir, src)
		s.Clock = func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }
		s.PollInterval = 10 * time.Millisecond
		s.GracePeriod = 30 * time.Millisecond
		s.CollectUsage = func(_ context.Context, sub model.Subscription) (model.Usage, error) {
			r, e := s.Runtime()
			return r.Usage[sub.ID], e
		}
		id := os.Getenv("INDIGOBIRD_LEASE_CHILD_ID")
		if e := os.WriteFile(filepath.Join(dir, "ready-"+id), nil, 0600); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(4 * time.Second)
		for {
			if _, e := os.Stat(filepath.Join(dir, "start")); e == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("start barrier expired")
			}
			time.Sleep(5 * time.Millisecond)
		}
		out, e := s.TickProject(context.Background(), "project")
		report := struct {
			Owned bool
			Error string
		}{Owned: out.Assignment != nil}
		if e != nil {
			report.Error = e.Error()
		}
		if e := atomicJSON(filepath.Join(dir, "report-"+id), report); e != nil {
			t.Fatal(e)
		}
		return
	}
	s, _, _ := fixture(t, "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmds := make([]*exec.Cmd, 2)
	for i := range cmds {
		id := fmt.Sprint(i)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIndigobirdSeparateProcessLease$")
		cmd.Env = append(os.Environ(), "INDIGOBIRD_LEASE_CHILD="+s.Dir, "INDIGOBIRD_LEASE_CHILD_ID="+id)
		log, e := os.Create(filepath.Join(s.Dir, "child-"+id+".log"))
		if e != nil {
			t.Fatal(e)
		}
		defer log.Close()
		cmd.Stdout = log
		cmd.Stderr = log
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		cmds[i] = cmd
	}
	defer func() {
		_ = s.Control(context.Background(), "kill", "project")
		for _, cmd := range cmds {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
	}()
	waitUntil(t, 3*time.Second, func() bool {
		for i := range cmds {
			if _, e := os.Stat(filepath.Join(s.Dir, "ready-"+fmt.Sprint(i))); e != nil {
				return false
			}
		}
		return true
	})
	if e := os.WriteFile(filepath.Join(s.Dir, "start"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	var active Lease
	waitUntil(t, 3*time.Second, func() bool {
		r, e := s.Runtime()
		if e != nil {
			return false
		}
		if len(r.Leases) != 1 {
			return false
		}
		for _, l := range r.Leases {
			if l.PID > 0 {
				active = l
				return true
			}
		}
		return false
	})
	waitUntil(t, 3*time.Second, func() bool {
		for i := range cmds {
			if _, e := os.Stat(filepath.Join(s.Dir, "report-"+fmt.Sprint(i))); e == nil {
				return true
			}
		}
		return false
	})
	r, _ := s.Runtime()
	if len(r.Leases) != 1 {
		t.Fatal("racing workers lost exclusive ownership")
	}
	if active.OwnerPID == os.Getpid() {
		t.Fatal("worker did not run in another OS process")
	}
	if e := s.Control(context.Background(), "kill", "project"); e != nil {
		t.Fatal(e)
	}
	owned := 0
	for i, cmd := range cmds {
		if e := cmd.Wait(); e != nil {
			b, _ := os.ReadFile(filepath.Join(s.Dir, "child-"+fmt.Sprint(i)+".log"))
			t.Fatal(e, string(b))
		}
		var report struct {
			Owned bool
			Error string
		}
		b, e := os.ReadFile(filepath.Join(s.Dir, "report-"+fmt.Sprint(i)))
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(b, &report); e != nil {
			t.Fatal(e)
		}
		if report.Owned {
			owned++
			if report.Error == "" {
				t.Fatal("interrupted owner reported success")
			}
		} else if report.Error != "" {
			t.Fatal(report.Error)
		}
	}
	r, _ = s.Runtime()
	if owned != 1 || len(r.Leases) != 0 || processAlive(active.PID, active.ProcessStart) {
		t.Fatal("cross-process exclusion or cleanup failed", owned, r.Leases)
	}
}

func waitUntil(t *testing.T, limit time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("bounded condition did not become true")
}

// TESTMASTER: id=indigobird-window-controls-finish-plan tier=fast parallel=yes
func TestIndigobirdFinishPlanHardGates(t *testing.T) {
	for _, gate := range []string{"global_paused", "global_disabled", "project_paused", "project_disabled", "subscription_disabled", "rule_disabled", "weekly_exhausted", "short_exhausted", "project_window", "rule_deleted"} {
		t.Run(gate, func(t *testing.T) {
			s, src, c := fixture(t, "")
			src.plans = src.plans[:1]
			c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "minute", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "12:00", End: "12:01", Timezone: "UTC", EveryWeeks: 1}}}
			c.Automations[0].Mode = "scheduled"
			c.Automations[0].WindowID = "window"
			c.Automations[0].OnWindowEnd = "finish_plan"
			var e error
			c, e = s.Apply(c.Revision, c)
			if e != nil {
				t.Fatal(e)
			}
			first, e := s.Tick(context.Background())
			if e != nil || first.Assignment == nil {
				t.Fatal(first, e)
			}
			base := s.now()
			s.Clock = func() time.Time { return base.Add(2 * time.Minute) }
			switch gate {
			case "global_paused":
				c.Paused = true
			case "global_disabled":
				c.Enabled = false
			case "project_paused":
				c.Projects[0].Paused = true
			case "project_disabled":
				c.Projects[0].Enabled = false
			case "subscription_disabled":
				c.Subscriptions[0].Enabled = false
			case "rule_disabled":
				c.Automations[0].Enabled = false
			case "project_window":
				c.Projects[0].WindowID = "window"
			case "rule_deleted":
				c.Automations = nil
			case "weekly_exhausted", "short_exhausted":
				collect := s.CollectUsage
				s.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
					u, e := collect(ctx, sub)
					if gate == "weekly_exhausted" {
						u.Weekly.UsedPercent = 100
					} else {
						u.Short.UsedPercent = 100
					}
					return u, e
				}
			}
			if _, e = s.Apply(c.Revision, c); e != nil {
				t.Fatal(e)
			}
			out, e := s.Tick(context.Background())
			if e != nil || out.Assignment != nil || src.checkpoints != 1 {
				t.Fatal("finish-plan bypassed hard gate", gate, out, e, src.checkpoints)
			}
			restarted := NewService(s.Dir, src)
			restarted.Clock = s.Clock
			d, e := restarted.Evaluate(context.Background(), EvaluateRequest{})
			if e != nil || d.Selected != nil {
				t.Fatal("restart bypassed hard gate", gate, d.Selected, e)
			}
		})
	}
}

// TESTMASTER: id=indigobird-execution-recovery tier=fast parallel=yes
func TestIndigobirdNoProgressTimeout(t *testing.T) {
	s, src, c := fixture(t, "sleep")
	src.plans = src.plans[:1]
	c.Projects[0].NoProgressSeconds = 60
	if _, e := s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	var elapsed atomic.Int64
	base := s.now()
	s.Clock = func() time.Time { return base.Add(time.Duration(elapsed.Load()) * time.Second) }
	done := make(chan error, 1)
	go func() { _, e := s.Tick(context.Background()); done <- e }()
	defer s.Control(context.Background(), "kill", "project")
	var active Lease
	waitUntil(t, 3*time.Second, func() bool {
		r, _ := s.Runtime()
		for _, l := range r.Leases {
			if l.PID > 0 {
				active = l
				return true
			}
		}
		return false
	})
	elapsed.Store(61)
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("no-progress timeout reported success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no-progress deadline did not stop child")
	}
	r, _ := s.Runtime()
	pg := r.Progress[PlanID(c.Projects[0].Path, "alpha")]
	if !pg.Parked || pg.Attempts != 1 || len(r.Notices) != 1 || len(r.Leases) != 0 || src.checkpoints != 0 || processAlive(active.PID, active.ProcessStart) {
		t.Fatal("timeout did not park/release safely", pg, r.Notices)
	}
	for i := 0; i < 2; i++ {
		out, e := s.Tick(context.Background())
		if e != nil || out.Assignment != nil {
			t.Fatal("parked timeout relaunched", out, e)
		}
	}
	r, _ = s.Runtime()
	if len(r.Notices) != 1 {
		t.Fatal("timeout notification duplicated")
	}
}

// TESTMASTER: id=indigobird-project-rules-checkpoint tier=fast parallel=yes
func TestIndigobirdCheckpointProfileHandoff(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(fmt.Sprint("interrupted=", interrupted), func(t *testing.T) {
			behavior := ""
			if interrupted {
				behavior = "sleep"
			}
			s, src, c := fixture(t, behavior)
			src.plans = src.plans[:1]
			c.Subscriptions[0].Sequences = append(c.Subscriptions[0].Sequences, model.Sequence{ID: "other", Name: "other"})
			var e error
			c, e = s.Apply(c.Revision, c)
			if e != nil {
				t.Fatal(e)
			}
			if interrupted {
				done := make(chan error, 1)
				go func() { _, e := s.Tick(context.Background()); done <- e }()
				defer s.Control(context.Background(), "kill", "project")
				waitUntil(t, 3*time.Second, func() bool {
					r, _ := s.Runtime()
					for _, l := range r.Leases {
						if l.PID > 0 {
							return true
						}
					}
					return false
				})
				if e = s.Control(context.Background(), "kill", "project"); e != nil {
					t.Fatal(e)
				}
				select {
				case e = <-done:
					if e == nil {
						t.Fatal("interruption succeeded")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("interrupt timeout")
				}
				script := c.Subscriptions[0].Args[0]
				b, e := os.ReadFile(script)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(script, []byte(strings.Replace(string(b), `behavior="sleep"`, `behavior=""`, 1)), 0600); e != nil {
					t.Fatal(e)
				}
			} else {
				first, e := s.Tick(context.Background())
				if e != nil || first.Assignment == nil || first.Assignment.Sequence.ID != "sequence" {
					t.Fatal(first, e)
				}
			}
			c.Automations[0].Entries = []model.Entry{{ID: "new", SequenceID: "other", MaxBand: model.Red}, {ID: "old", SequenceID: "sequence", MaxBand: model.Red}}
			if _, e = s.Apply(c.Revision, c); e != nil {
				t.Fatal(e)
			}
			restarted := NewService(s.Dir, src)
			restarted.Clock = s.Clock
			restarted.CollectUsage = s.CollectUsage
			if interrupted {
				d, e := restarted.Evaluate(context.Background(), EvaluateRequest{})
				if e != nil || d.Selected == nil || d.Selected.Sequence.ID != "sequence" {
					t.Fatal("interrupted checkpoint handed off prematurely", d.Selected, e)
				}
				out, e := restarted.Tick(context.Background())
				if e != nil || out.Assignment == nil || out.Assignment.Kind != "diagnosis" || out.Assignment.Sequence.ID != "sequence" || src.checkpoints != 0 {
					t.Fatal("original sequence did not reconcile", out, e)
				}
			}
			out, e := restarted.Tick(context.Background())
			wantStep := 2
			if interrupted {
				wantStep = 1
			}
			if e != nil || out.Assignment == nil || out.Assignment.Sequence.ID != "other" || out.Assignment.Step != wantStep {
				t.Fatal("checkpoint handoff failed", out, e)
			}
		})
	}
}

// TESTMASTER: id=indigobird-window-controls-occurrence tier=fast parallel=yes
func TestIndigobirdDSTOccurrenceRestart(t *testing.T) {
	s, src, c := fixture(t, "")
	current := time.Date(2026, 11, 1, 6, 15, 0, 0, time.UTC)
	s.Clock = func() time.Time { return current }
	collect := s.CollectUsage
	s.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
		u, e := collect(ctx, sub)
		u.Weekly.StartsAt = current.Add(-24 * time.Hour)
		u.Weekly.ResetsAt = current.Add(6 * 24 * time.Hour)
		u.Short.StartsAt = current.Add(-time.Hour)
		u.Short.ResetsAt = current.Add(4 * time.Hour)
		return u, e
	}
	c.TimeWindows = []model.TimeWindow{{ID: "dst", Name: "Repeated Sunday hour", Enabled: true, Recurrence: model.Recurrence{Days: []int{7}, Start: "01:00", End: "01:30", Timezone: "America/Chicago", EveryWeeks: 1}}}
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "dst"
	c.Automations[0].RunLimit = "one_plan"
	if _, e := s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	var occurrence string
	for i := 0; i < 3; i++ {
		restarted := NewService(s.Dir, src)
		restarted.Clock = s.Clock
		restarted.CollectUsage = s.CollectUsage
		out, e := restarted.Tick(context.Background())
		if e != nil || out.Assignment == nil || out.Assignment.Plan != "alpha" {
			t.Fatal("first occurrence did not complete claimed plan", i, out, e)
		}
		r, e := restarted.Runtime()
		if e != nil || len(r.Occurrences) != 1 {
			t.Fatal(r.Occurrences, e)
		}
		if i == 0 {
			occurrence = r.Occurrences[0].ID
		}
		if r.Occurrences[0].ID != occurrence {
			t.Fatal("restart duplicated occurrence")
		}
	}
	current = current.Add(time.Hour)
	restarted := NewService(s.Dir, src)
	restarted.Clock = s.Clock
	restarted.CollectUsage = s.CollectUsage
	out, e := restarted.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("repeated local hour claimed a second plan", out, e)
	}
	r, _ := restarted.Runtime()
	if len(r.Occurrences) != 1 || r.Occurrences[0].ID != occurrence || !r.Occurrences[0].Completed {
		t.Fatal("completed claim lost across repeated hour", r.Occurrences)
	}
	current = current.AddDate(0, 0, 7)
	out, e = restarted.Tick(context.Background())
	if e != nil || out.Assignment == nil || out.Assignment.Plan != "beta" {
		t.Fatal("next weekly occurrence did not admit next plan", out, e)
	}
	r, _ = restarted.Runtime()
	if len(r.Occurrences) != 2 {
		t.Fatal("new weekly occurrence was not durable", r.Occurrences)
	}
}

// TESTMASTER: id=indigobird-window-controls-checkpoint tier=fast parallel=yes
func TestIndigobirdInFlightCheckpointWindow(t *testing.T) {
	s, src, c := fixture(t, "hold")
	c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "minute", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "12:00", End: "12:01", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "window"
	c.Automations[0].OnWindowEnd = "finish_checkpoint"
	var e error
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	var elapsed atomic.Int64
	base := s.now()
	s.Clock = func() time.Time { return base.Add(time.Duration(elapsed.Load()) * time.Second) }
	type tick struct {
		out TickResult
		err error
	}
	done := make(chan tick, 1)
	go func() { out, e := s.Tick(context.Background()); done <- tick{out, e} }()
	defer s.Control(context.Background(), "kill", "project")
	var active Lease
	waitUntil(t, 3*time.Second, func() bool {
		r, _ := s.Runtime()
		for _, l := range r.Leases {
			if l.PID > 0 {
				active = l
				_, e := os.Stat(filepath.Join(c.Projects[0].Path, "uncommitted.txt"))
				return e == nil
			}
		}
		return false
	})
	c.Subscriptions[0].Sequences = append(c.Subscriptions[0].Sequences, model.Sequence{ID: "other", Name: "other"})
	c.Automations[0].Entries = []model.Entry{{ID: "other-entry", SequenceID: "other", MaxBand: model.Red}}
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	elapsed.Store(61)
	waitUntil(t, 3*time.Second, func() bool {
		r, _ := s.Runtime()
		return len(r.Occurrences) == 1 && r.Occurrences[0].ClosingAction == "finish_checkpoint"
	})
	r, _ := s.Runtime()
	l := r.Leases[active.Assignment.ID]
	if !processAlive(l.PID, l.ProcessStart) || l.Assignment.ConfigRevision == c.Revision || l.Assignment.Sequence.ID != "sequence" {
		t.Fatal("active snapshot changed or was stopped")
	}
	if e = os.WriteFile(active.Assignment.ResultPath+".continue", nil, 0600); e != nil {
		t.Fatal(e)
	}
	select {
	case result := <-done:
		if result.err != nil || result.out.Result == nil || result.out.Result.Outcome != "completed" {
			t.Fatal(result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("allowed checkpoint did not complete")
	}
	if src.checkpoints != 1 {
		t.Fatal("valid in-flight checkpoint was not recorded")
	}
	restarted := NewService(s.Dir, src)
	restarted.Clock = s.Clock
	restarted.CollectUsage = s.CollectUsage
	out, e := restarted.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("closed occurrence launched successor", out, e)
	}
	current, e := restarted.Config()
	if e != nil || current.Revision != c.Revision || current.Automations[0].Entries[0].SequenceID != "other" {
		t.Fatal("next snapshot lost saved edit", current, e)
	}
}

// TESTMASTER: id=indigobird-window-controls-one-plan tier=fast parallel=yes
func TestIndigobirdOnePlanRetryPark(t *testing.T) {
	for _, mode := range []string{"finish_checkpoint", "finish_plan"} {
		t.Run(mode, func(t *testing.T) {
			s, _, c := fixture(t, "retry")
			c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "minute", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "12:00", End: "12:01", Timezone: "UTC", EveryWeeks: 1}}}
			c.Automations[0].Mode = "scheduled"
			c.Automations[0].WindowID = "window"
			c.Automations[0].RunLimit = "one_plan"
			c.Automations[0].OnWindowEnd = mode
			if _, e := s.Apply(c.Revision, c); e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 5; i++ {
				out, e := s.Tick(context.Background())
				if e != nil || out.Assignment == nil || out.Assignment.Plan != "alpha" {
					t.Fatal("retry escaped claim", i, out, e)
				}
				if i == 0 && mode == "finish_plan" {
					base := s.now()
					s.Clock = func() time.Time { return base.Add(2 * time.Minute) }
				}
			}
			r, _ := s.Runtime()
			if !r.Progress[PlanID(c.Projects[0].Path, "alpha")].Parked {
				t.Fatal("retry did not park")
			}
			restarted := NewService(s.Dir, s.Source)
			restarted.Clock = s.Clock
			restarted.CollectUsage = s.CollectUsage
			for i := 0; i < 2; i++ {
				out, e := restarted.Tick(context.Background())
				if e != nil || out.Assignment != nil {
					t.Fatal("parking reopened one-plan slot", out, e)
				}
			}
			r, _ = restarted.Runtime()
			if len(r.Occurrences) != 1 || r.Occurrences[0].ClaimedPlan != PlanID(c.Projects[0].Path, "alpha") {
				t.Fatal("parked claim changed", r.Occurrences)
			}
		})
	}
}

// TESTMASTER: id=indigobird-projects-queue tier=fast parallel=yes
func TestIndigobirdMixedQueuePriority(t *testing.T) {
	s, src, c := fixture(t, "")
	base := s.now()
	src.plans = []Plan{{Name: "recent", Staged: true, State: "queued", Started: base}, {Name: "old", Staged: true, State: "queued", Started: base.Add(-24 * time.Hour)}, {Name: "executing", State: "executing", Started: base}, {Name: "unblocked", State: "unblocked", Started: base}, {Name: "draft", State: "planned", Started: base.Add(-48 * time.Hour)}, {Name: "done", Staged: true, Complete: true}, {Name: "release", Staged: true, Release: true}}
	for _, want := range []string{"unblocked", "executing", "old", "recent"} {
		d, e := s.Evaluate(context.Background(), EvaluateRequest{})
		if e != nil || d.Selected == nil || d.Selected.Plan.Name != want {
			t.Fatal("mixed queue priority", want, d.Selected, e)
		}
		if d.Queues[0].Running != 0 || d.Queues[0].Depth != 4-src.checkpoints {
			t.Fatal("mixed queue count", d.Queues)
		}
		out, e := s.Tick(context.Background())
		if e != nil || out.Assignment == nil || out.Assignment.Plan != want {
			t.Fatal("tick diverged from queue", out, e)
		}
		src.mu.Lock()
		for i := range src.plans {
			if src.plans[i].Name == want {
				src.plans[i].Complete = true
			}
		}
		src.mu.Unlock()
	}
	d, e := s.Evaluate(context.Background(), EvaluateRequest{})
	if e != nil || d.Selected != nil || d.Queues[0].Depth != 0 {
		t.Fatal("draft/completed/release work appeared queued", d, e)
	}
	if _, e = s.TickProject(context.Background(), c.Projects[0].ID); e != nil {
		t.Fatal(e)
	}
}

// TESTMASTER: id=indigobird-execution-controls tier=fast parallel=yes
func TestIndigobirdDisableAndKillSafety(t *testing.T) {
	for _, action := range []string{"stop", "pause", "kill", "window"} {
		t.Run(action, func(t *testing.T) {
			s, src, c := fixture(t, "holdtree")
			if action == "window" {
				c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "minute", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "12:00", End: "12:01", Timezone: "UTC", EveryWeeks: 1}}}
				c.Automations[0].Mode = "scheduled"
				c.Automations[0].WindowID = "window"
				c.Automations[0].OnWindowEnd = "interrupt"
				var e error
				c, e = s.Apply(c.Revision, c)
				if e != nil {
					t.Fatal(e)
				}
			}
			python := c.Subscriptions[0].Executable
			sentinel := exec.Command(python, "-c", "import time;time.sleep(20)")
			if e := sentinel.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { _ = sentinel.Process.Kill(); _ = sentinel.Wait() }()
			sentinelStart := processIdentity(sentinel.Process.Pid)
			var elapsed atomic.Int64
			base := s.now()
			s.Clock = func() time.Time { return base.Add(time.Duration(elapsed.Load()) * time.Second) }
			type tick struct {
				out TickResult
				err error
			}
			done := make(chan tick, 1)
			go func() { out, e := s.Tick(context.Background()); done <- tick{out, e} }()
			defer s.Control(context.Background(), "kill", "project")
			var active Lease
			var child int
			waitUntil(t, 3*time.Second, func() bool {
				r, _ := s.Runtime()
				for _, l := range r.Leases {
					if l.PID > 0 {
						active = l
						b, e := os.ReadFile(l.Assignment.ResultPath + ".child")
						if e == nil {
							_, _ = fmt.Sscan(string(b), &child)
							_, e = os.Stat(filepath.Join(c.Projects[0].Path, "uncommitted.txt"))
							return child > 0 && e == nil
						}
					}
				}
				return false
			})
			childStart := processIdentity(child)
			if action == "window" {
				elapsed.Store(61)
			} else {
				if e := s.Control(context.Background(), action, "project"); e != nil {
					t.Fatal(e)
				}
			}
			if action == "stop" || action == "pause" {
				if !processAlive(active.PID, active.ProcessStart) {
					t.Fatal("graceful control killed checkpoint")
				}
				if e := os.WriteFile(active.Assignment.ResultPath+".continue", nil, 0600); e != nil {
					t.Fatal(e)
				}
			}
			select {
			case result := <-done:
				if action == "stop" || action == "pause" {
					if result.err != nil || src.checkpoints != 1 {
						t.Fatal("graceful checkpoint lost", result.err, src.checkpoints)
					}
				} else if result.err == nil || src.checkpoints != 0 {
					t.Fatal("forced interruption claimed completion", result.err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("control did not settle")
			}
			if !processAlive(sentinel.Process.Pid, sentinelStart) {
				t.Fatal("unrelated process was signaled")
			}
			if processAlive(active.PID, active.ProcessStart) || processAlive(child, childStart) {
				t.Fatal("owned process survived cleanup")
			}
			data, e := os.ReadFile(filepath.Join(c.Projects[0].Path, "uncommitted.txt"))
			if e != nil || string(data) != "owned fixture edit\n" {
				t.Fatal("uncommitted edit lost", string(data), e)
			}
			r, _ := s.Runtime()
			if len(r.Leases) != 0 {
				t.Fatal("control retained finished lease")
			}
			if action == "stop" || action == "pause" {
				out, e := s.Tick(context.Background())
				if e != nil || out.Assignment != nil {
					t.Fatal("graceful control admitted successor", out, e)
				}
			} else {
				pg := r.Progress[PlanID(c.Projects[0].Path, "alpha")]
				if pg.ReconcileSequence != "sequence" || pg.NextKind != "diagnosis" {
					t.Fatal("interrupted checkpoint not resumable", pg)
				}
			}
		})
	}
}

// TESTMASTER: id=indigobird-design-sync-used-migration tier=fast parallel=yes
func TestIndigobirdMigrationTargetsAndInterruptedWrite(t *testing.T) {
	s, _, c := fixture(t, "")
	c.Version = 1
	c.Subscriptions[0].Sequences = []model.Sequence{{ID: "sequence", Name: "Fixture · build"}, {ID: "suffix", Name: "already suffix"}, {ID: "custom", Name: "Other · custom"}}
	rawBytes, _ := json.Marshal(c)
	var raw map[string]any
	if e := json.Unmarshal(rawBytes, &raw); e != nil {
		t.Fatal(e)
	}
	sub := raw["subscriptions"].([]any)[0].(map[string]any)
	windows := []any{}
	for i, remaining := range []float64{0, 40, 39} {
		windows = append(windows, map[string]any{"id": fmt.Sprint("window-", i), "name": fmt.Sprint("Window ", i), "enabled": i != 1, "target_remaining_percent": remaining, "recurrence": map[string]any{"days": []int{1}, "start": "00:00", "end": "00:00", "timezone": "UTC", "every_weeks": 1}, "end_mode": "recurrence"})
	}
	sub["overdrive_windows"] = windows
	legacy, _ := json.Marshal(raw)
	if e := AtomicWrite(filepath.Join(s.Dir, "config.json"), legacy); e != nil {
		t.Fatal(e)
	}
	r, _ := s.Runtime()
	r.Occurrences = []model.Occurrence{{ID: "completed-occurrence", WindowID: "window-0", Completed: true, ClaimedPlan: PlanID(c.Projects[0].Path, "alpha")}}
	if e := s.withLock(func() error { return s.saveRuntime(&r) }); e != nil {
		t.Fatal(e)
	}
	runtimeBefore, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
	backup := filepath.Join(s.Dir, "config.legacy.backup.json")
	if e := os.Mkdir(backup, 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Migrate(); e == nil {
		t.Fatal("replacement fault did not abort migration")
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if string(before) != string(legacy) {
		t.Fatal("failed migration replaced prior revision")
	}
	if e := os.WriteFile(filepath.Join(s.Dir, ".conductor-interrupted-migration"), []byte(`{"version":2,"subscriptions":`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(backup); e != nil {
		t.Fatal(e)
	}
	restarted := NewService(s.Dir, s.Source)
	migrated, e := restarted.Migrate()
	if e != nil {
		t.Fatal(e)
	}
	for i, w := range migrated.Subscriptions[0].OverdriveWindows {
		if w.TargetUsedPercent != []float64{100, 60, 61}[i] || w.Enabled != (i != 1) {
			t.Fatal("legacy target or enable flag changed", w)
		}
	}
	for i, q := range migrated.Subscriptions[0].Sequences {
		if q.Name != []string{"build", "already suffix", "Other · custom"}[i] {
			t.Fatal("suffix migration changed custom name", q)
		}
	}
	saved, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	again, e := restarted.Migrate()
	if e != nil || again.Revision != migrated.Revision {
		t.Fatal("repeat migration changed revision", e)
	}
	final, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if string(final) != string(saved) || strings.Contains(string(final), "target_remaining_percent") {
		t.Fatal("migration did not stabilize explicit used units")
	}
	restored, _ := os.ReadFile(backup)
	if string(restored) != string(legacy) {
		t.Fatal("legacy backup lost source bytes")
	}
	runtimeAfter, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
	if string(runtimeAfter) != string(runtimeBefore) {
		t.Fatal("migration changed occurrence completion or runtime history")
	}
	if e = ValidateConfig(again); e != nil {
		t.Fatal("migration broke references", e)
	}
}

func overdriveFixture(t *testing.T, behavior string) (*Service, *fixtureSource, model.Config) {
	s, src, c := fixture(t, behavior)
	for i := range c.PowerBands[0].GreenCurve {
		c.PowerBands[0].GreenCurve[i] = 25
		c.PowerBands[0].RedCurve[i] = 75
	}
	c.Subscriptions[0].OverdriveEnabled = true
	c.Subscriptions[0].OverdriveWindows = []model.OverdriveWindow{{ID: "over", Name: "Friday capacity", Enabled: true, TargetUsedPercent: 100, EndMode: "recurrence", Recurrence: model.Recurrence{Days: []int{5}, Start: "11:00", End: "13:00", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Entries[0].MaxBand = model.Green
	c.Automations[0].HonorOverdrive = true
	return s, src, c
}

// TESTMASTER: id=indigobird-rule-overdrive-honor tier=fast parallel=yes
func TestIndigobirdOverdriveTickMatrix(t *testing.T) {
	for _, tc := range []struct {
		name string
		used float64
		want bool
	}{
		{"honor-yellow", 50, true}, {"honor-red", 80, true}, {"optout-green", 10, true}, {"optout-yellow", 50, false}, {"optout-red", 80, false},
		{"only-before", 10, false}, {"only-inside", 10, true}, {"only-after", 10, false}, {"target-below", 59, true}, {"target-at", 60, false}, {"target-above", 61, false}, {"normal-after-target", 61, true}, {"zero-target", 0, false},
		{"project-disabled", 80, false}, {"window-disabled", 80, false}, {"subscription-disabled", 80, false}, {"project-window", 80, false}, {"short-exhausted", 80, false}, {"weekly-exhausted", 100, false}, {"stale", 80, false}, {"unknown", 80, false}, {"outside", 80, false}, {"no-work", 80, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, src, c := overdriveFixture(t, "")
			used := tc.used
			if strings.HasPrefix(tc.name, "optout") {
				c.Automations[0].HonorOverdrive = false
			}
			if strings.HasPrefix(tc.name, "only") || strings.HasPrefix(tc.name, "target") || tc.name == "zero-target" {
				c.Automations[0].OnlyOverdrive = true
			}
			switch tc.name {
			case "only-before", "only-after", "outside":
				c.Subscriptions[0].OverdriveWindows[0].Recurrence.Start = "13:00"
				c.Subscriptions[0].OverdriveWindows[0].Recurrence.End = "14:00"
				if tc.name == "only-after" {
					c.Subscriptions[0].OverdriveWindows[0].Recurrence.Start = "10:00"
					c.Subscriptions[0].OverdriveWindows[0].Recurrence.End = "11:00"
				}
			case "project-disabled":
				c.Projects[0].Enabled = false
			case "subscription-disabled":
				c.Subscriptions[0].Enabled = false
			case "window-disabled":
				c.Subscriptions[0].OverdriveWindows[0].Enabled = false
			case "project-window":
				c.TimeWindows = []model.TimeWindow{{ID: "closed", Name: "Closed", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "13:00", End: "14:00", Timezone: "UTC", EveryWeeks: 1}}}
				c.Projects[0].WindowID = "closed"
			case "target-below", "target-at", "target-above", "normal-after-target":
				c.Subscriptions[0].OverdriveWindows[0].TargetUsedPercent = 60
				if tc.name == "normal-after-target" {
					c.Automations[0].Entries[0].MaxBand = model.Red
				}
			case "zero-target":
				c.Subscriptions[0].OverdriveWindows[0].TargetUsedPercent = 0
			case "no-work":
				src.plans = nil
			case "stale":
				base := s.now()
				s.Clock = func() time.Time { return base.Add(2 * time.Hour) }
				c.Subscriptions[0].OverdriveWindows[0].Recurrence.End = "18:00"
			}
			collect := s.CollectUsage
			s.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
				u, e := collect(ctx, sub)
				u.Weekly.UsedPercent = used
				switch tc.name {
				case "short-exhausted":
					u.Short.UsedPercent = 100
				case "stale":
					u.ObservedAt = s.now().Add(-time.Hour)
				case "unknown":
					u.Weekly = nil
				}
				return u, e
			}
			if _, e := s.Apply(c.Revision, c); e != nil {
				t.Fatal(e)
			}
			restarted := NewService(s.Dir, src)
			restarted.Clock = s.Clock
			restarted.CollectUsage = s.CollectUsage
			out, e := restarted.Tick(context.Background())
			if e != nil || (out.Assignment != nil) != tc.want {
				t.Fatal(tc.name, "eligibility", out, e)
			}
			if tc.want && src.checkpoints != 1 {
				t.Fatal("admission did not execute real checkpoint")
			}
			if !tc.want && src.checkpoints != 0 {
				t.Fatal("denied case spawned work")
			}
			if tc.name == "normal-after-target" && out.Assignment.Budget.OverdriveActive {
				t.Fatal("met target remained override funded")
			}
			if !tc.want && tc.name != "no-work" && len(out.Decision.Candidates) > 0 && out.Decision.Candidates[0].Reason == "" {
				t.Fatal("missing blocked explanation")
			}
		})
	}
}

// TESTMASTER: id=indigobird-rule-overdrive-completion tier=fast parallel=yes
func TestIndigobirdOverdriveCheckpointRetirement(t *testing.T) {
	for _, action := range []string{"target", "end", "reset", "disable", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, src, c := overdriveFixture(t, "hold")
			c.Automations[0].OnlyOverdrive = true
			c.Subscriptions[0].OverdriveWindows[0].TargetUsedPercent = 60
			var e error
			c, e = s.Apply(c.Revision, c)
			if e != nil {
				t.Fatal(e)
			}
			usage, _ := s.CollectUsage(context.Background(), c.Subscriptions[0])
			s.CollectUsage = func(context.Context, model.Subscription) (model.Usage, error) { return usage, nil }
			type tick struct {
				out TickResult
				err error
			}
			done := make(chan tick, 1)
			go func() { out, e := s.Tick(context.Background()); done <- tick{out, e} }()
			defer s.Control(context.Background(), "kill", "project")
			var active Lease
			waitUntil(t, 3*time.Second, func() bool {
				r, _ := s.Runtime()
				for _, l := range r.Leases {
					if l.PID > 0 {
						active = l
						_, e := os.Stat(filepath.Join(c.Projects[0].Path, "uncommitted.txt"))
						return e == nil
					}
				}
				return false
			})
			switch action {
			case "target":
				usage.Weekly.UsedPercent = 61
			case "end":
				c.Subscriptions[0].OverdriveWindows[0].Recurrence.End = "12:00"
			case "reset":
				usage.Weekly.Epoch = "new-epoch"
				usage.Weekly.StartsAt = s.now()
				usage.Weekly.ResetsAt = s.now().Add(7 * 24 * time.Hour)
				usage.Weekly.UsedPercent = 0
			case "disable":
				c.Subscriptions[0].OverdriveWindows[0].Enabled = false
			case "delete":
				c.Subscriptions[0].OverdriveWindows = nil
			}
			if action == "end" || action == "disable" || action == "delete" {
				if _, e = s.Apply(c.Revision, c); e != nil {
					t.Fatal(e)
				}
			}
			if e = os.WriteFile(active.Assignment.ResultPath+".continue", nil, 0600); e != nil {
				t.Fatal(e)
			}
			select {
			case result := <-done:
				if result.err != nil || result.out.Result == nil || result.out.Result.Outcome != "completed" {
					t.Fatal(result)
				}
				windows := result.out.Assignment.Subscription.OverdriveWindows
				if len(windows) != 1 || !windows[0].Enabled || windows[0].Recurrence.End != "13:00" {
					t.Fatal("active overdrive snapshot changed with saved edit", windows)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("active overdrive checkpoint did not finish")
			}
			restarted := NewService(s.Dir, src)
			restarted.Clock = s.Clock
			restarted.CollectUsage = s.CollectUsage
			out, e := restarted.Tick(context.Background())
			if e != nil || out.Assignment != nil || src.checkpoints != 1 {
				t.Fatal("retired overdrive admitted successor", action, out, e)
			}
			if action == "target" || action == "reset" {
				usage.Weekly.UsedPercent = 0
				out, e = restarted.Tick(context.Background())
				if e != nil || out.Assignment != nil {
					t.Fatal("telemetry correction reopened completed occurrence", out, e)
				}
			}
		})
	}
}

// TESTMASTER: id=indigobird-execution-cross-band tier=fast parallel=yes
func TestIndigobirdCrossBandFallbackAndShortRecovery(t *testing.T) {
	s, src, c := overdriveFixture(t, "")
	c.Subscriptions[0].OverdriveEnabled = false
	other := c.Subscriptions[0]
	other.ID = "second"
	other.Name = "Second"
	other.AccountID = "identity-2"
	other.PoolID = "pool-2"
	other.OverdriveWindows = nil
	other.Sequences = []model.Sequence{{ID: "sequence-2", Name: "build"}}
	third := other
	third.ID = "third"
	third.Name = "Third"
	third.AccountID = "identity-3"
	third.PoolID = "pool-3"
	third.Sequences = []model.Sequence{{ID: "sequence-3", Name: "build"}}
	c.Subscriptions = append(c.Subscriptions, other, third)
	c.Automations[0].Entries = append(c.Automations[0].Entries, model.Entry{ID: "second-entry", SequenceID: "sequence-2", MaxBand: model.Green}, model.Entry{ID: "third-entry", SequenceID: "sequence-3", MaxBand: model.Green})
	var e error
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	used := map[string]float64{"account": 10, "second": 10, "third": 10}
	short := map[string]float64{}
	collect := s.CollectUsage
	s.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
		u, e := collect(ctx, sub)
		u.Weekly.UsedPercent = used[sub.ID]
		u.Short.UsedPercent = short[sub.ID]
		return u, e
	}
	for i, want := range []string{"account", "second", "third", "account"} {
		if i == 1 {
			used["account"] = 50
		}
		if i == 2 {
			used["second"] = 80
		}
		if i == 3 {
			used["account"] = 10
		}
		restarted := NewService(s.Dir, src)
		restarted.Clock = s.Clock
		restarted.CollectUsage = s.CollectUsage
		out, e := restarted.Tick(context.Background())
		if e != nil || out.Assignment == nil || out.Assignment.Subscription.ID != want {
			t.Fatal("cross-band order", i, want, out, e)
		}
	}
	for id := range used {
		used[id] = 80
		short[id] = 100
	}
	c.Subscriptions[0].OverdriveEnabled = true
	c.Automations[0].OnlyOverdrive = true
	c.Automations[0].Entries = c.Automations[0].Entries[:1]
	if _, e = s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("short hard limit bypassed during overdrive", out, e)
	}
	short["account"] = 0
	out, e = s.Tick(context.Background())
	if e != nil || out.Assignment == nil || out.Assignment.Subscription.ID != "account" || !out.Assignment.Budget.OverdriveActive {
		t.Fatal("fresh short quota did not resume useful work", out, e)
	}
}

// TESTMASTER: id=indigobird-automation-sample-gates tier=fast parallel=yes
func TestIndigobirdSampleQueueOverride(t *testing.T) {
	for _, state := range []string{"completed", "running", "blocked", "draft"} {
		t.Run(state, func(t *testing.T) {
			s, src, _ := fixture(t, "")
			p := Plan{Name: "existing", Staged: true, State: "queued"}
			switch state {
			case "completed":
				p.Complete = true
			case "running":
				p.Running = true
			case "blocked":
				p.Blocked = true
			case "draft":
				p.Staged = false
				p.State = "planned"
			}
			src.plans = []Plan{p}
			before, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
			queued, busy := true, false
			d, e := s.Evaluate(context.Background(), EvaluateRequest{QueueAvailable: &queued, ProjectBusy: &busy})
			if e != nil || d.Selected == nil || d.Selected.Plan.Name != "sample-plan" {
				t.Fatal("sample availability did not create an eligible sample", state, d.Selected, e)
			}
			d, e = s.Evaluate(context.Background(), EvaluateRequest{})
			if e != nil || d.Selected != nil {
				t.Fatal("sample leaked into live queue", d.Selected, e)
			}
			after, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
			if string(before) != string(after) || len(src.plans) != 1 || src.plans[0].Name != "existing" {
				t.Fatal("sample changed persistent/source state")
			}
		})
	}
}

// TESTMASTER: id=indigobird-engine-policy-shared tier=fast parallel=yes
func TestIndigobirdSharedPoolAdmission(t *testing.T) {
	s, src, c := fixture(t, "hold")
	other := c.Subscriptions[0]
	other.ID = "alias"
	other.Name = "Alias"
	other.Sequences = []model.Sequence{{ID: "alias-sequence", Name: "build"}}
	script := filepath.Join(t.TempDir(), "agent.py")
	body, e := os.ReadFile(other.Args[0])
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(script, []byte(strings.Replace(string(body), `behavior="hold"`, `behavior=""`, 1)), 0600); e != nil {
		t.Fatal(e)
	}
	other.Args = []string{script, "{prompt}", "{result}"}
	c.Subscriptions = append(c.Subscriptions, other)
	c.Projects = append(c.Projects, model.Project{ID: "other-project", Name: "Other", Path: t.TempDir(), Enabled: true})
	c.Automations = append(c.Automations, model.Automation{ID: "other-rule", Name: "Other", ProjectID: "other-project", Enabled: true, Mode: "keep_working", Entries: []model.Entry{{ID: "alias-entry", SequenceID: "alias-sequence", MaxBand: model.Red}}})
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := s.TickProject(context.Background(), "project"); done <- e }()
	defer s.Control(context.Background(), "kill", "project")
	var active Lease
	waitUntil(t, 3*time.Second, func() bool {
		r, _ := s.Runtime()
		for _, l := range r.Leases {
			if l.PID > 0 {
				active = l
				return true
			}
		}
		return false
	})
	separate := NewService(s.Dir, src)
	separate.Clock = s.Clock
	separate.CollectUsage = s.CollectUsage
	out, e := separate.TickProject(context.Background(), "other-project")
	if e != nil || out.Assignment != nil {
		t.Fatal("same pool dispatched concurrently through another subscription", out, e)
	}
	if len(out.Decision.Candidates) == 0 || out.Decision.Candidates[0].Code != "account_busy" {
		t.Fatal("not blocked by shared allowance pool", out.Decision.Candidates)
	}
	c.Subscriptions[1].PoolID = "independent"
	c.Subscriptions[1].AccountID = "other-account"
	if _, e = separate.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	out, e = separate.TickProject(context.Background(), "other-project")
	if e != nil || out.Assignment == nil || out.Assignment.Subscription.ID != "alias" {
		t.Fatal("independent pool did not proceed", out, e)
	}
	if e = os.WriteFile(active.Assignment.ResultPath+".continue", nil, 0600); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shared owner did not settle")
	}
	if src.checkpoints != 2 {
		t.Fatal("pool isolation lost checkpoints", src.checkpoints)
	}
}

func twoSubscriptions(c *model.Config) {
	other := c.Subscriptions[0]
	other.ID = "second"
	other.Name = "Second"
	other.AccountID = "second-identity"
	other.PoolID = "second-pool"
	other.Sequences = []model.Sequence{{ID: "second-sequence", Name: "build"}}
	other.OverdriveWindows = nil
	c.Subscriptions = append(c.Subscriptions, other)
}

// TESTMASTER: id=indigobird-project-rules-routing tier=fast parallel=yes
func TestIndigobirdDistinctRuleWindows(t *testing.T) {
	s, src, c := fixture(t, "")
	twoSubscriptions(&c)
	c.TimeWindows = []model.TimeWindow{{ID: "early", Name: "Early", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "11:00", End: "13:00", Timezone: "UTC", EveryWeeks: 1}}, {ID: "late", Name: "Late", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "13:00", End: "15:00", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "early"
	second := c.Automations[0]
	second.ID = "second-rule"
	second.Name = "Second route"
	second.WindowID = "late"
	second.Entries = []model.Entry{{ID: "second-entry", SequenceID: "second-sequence", MaxBand: model.Red}}
	c.Automations = append(c.Automations, second)
	if _, e := s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	first, e := s.Tick(context.Background())
	if e != nil || first.Assignment == nil || first.Assignment.Subscription.ID != "account" || first.Assignment.Step != 1 {
		t.Fatal("early route", first, e)
	}
	base := s.now()
	s.Clock = func() time.Time { return base.Add(2 * time.Hour) }
	restarted := NewService(s.Dir, src)
	restarted.Clock = s.Clock
	restarted.CollectUsage = s.CollectUsage
	next, e := restarted.Tick(context.Background())
	if e != nil || next.Assignment == nil || next.Assignment.Subscription.ID != "second" || next.Assignment.Step != 2 || next.Assignment.Automation.ID != "second-rule" {
		t.Fatal("late route", next, e)
	}
	if src.checkpoints != 2 {
		t.Fatal("window handoff repeated pair")
	}
	s.Clock = func() time.Time { return base.Add(3 * time.Hour) }
	restarted.Clock = s.Clock
	closed, e := restarted.Tick(context.Background())
	if e != nil || closed.Assignment != nil {
		t.Fatal("exclusive scheduled end dispatched", closed, e)
	}
	c, e = restarted.Config()
	if e != nil {
		t.Fatal(e)
	}
	c.Automations[1].Mode = "keep_working"
	c.Automations[1].WindowID = ""
	if _, e = restarted.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	anytime, e := restarted.Tick(context.Background())
	if e != nil || anytime.Assignment == nil || anytime.Assignment.Subscription.ID != "second" {
		t.Fatal("same ordered profile did not run anytime", anytime, e)
	}
}

// TESTMASTER: id=indigobird-project-rules-overlap tier=fast parallel=yes
func TestIndigobirdRuleOverlapSelection(t *testing.T) {
	for _, criterion := range []string{"priority", "reset", "id"} {
		t.Run(criterion, func(t *testing.T) {
			s, src, c := fixture(t, "hold")
			twoSubscriptions(&c)
			second := c.Automations[0]
			second.ID = "a-rule"
			second.Name = "Other"
			second.Entries = []model.Entry{{ID: "second-entry", SequenceID: "second-sequence", MaxBand: model.Red}}
			c.Automations[0].ID = "z-rule"
			c.Automations = append(c.Automations, second)
			want := "a-rule"
			if criterion == "priority" {
				c.Automations[0].Priority = -1
				want = "z-rule"
			}
			collect := s.CollectUsage
			s.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
				u, e := collect(ctx, sub)
				if criterion == "reset" && sub.ID == "account" {
					u.Weekly.ResetsAt = s.now().Add(2 * 24 * time.Hour)
				}
				return u, e
			}
			if criterion == "reset" {
				want = "z-rule"
			}
			if _, e := s.Apply(c.Revision, c); e != nil {
				t.Fatal(e)
			}
			type tick struct {
				out TickResult
				err error
			}
			done := make(chan tick, 1)
			go func() { out, e := s.Tick(context.Background()); done <- tick{out, e} }()
			defer s.Control(context.Background(), "kill", "project")
			var active Lease
			waitUntil(t, 3*time.Second, func() bool {
				r, _ := s.Runtime()
				for _, l := range r.Leases {
					if l.PID > 0 {
						active = l
						return true
					}
				}
				return false
			})
			if active.Assignment.Automation.ID != want {
				t.Fatal("wrong overlapping rule winner", criterion, active.Assignment.Automation.ID, want)
			}
			other := NewService(s.Dir, src)
			other.Clock = s.Clock
			other.CollectUsage = s.CollectUsage
			duplicate, e := other.Tick(context.Background())
			if e != nil || duplicate.Assignment != nil {
				t.Fatal("overlapping rules double dispatched", duplicate, e)
			}
			if e = os.WriteFile(active.Assignment.ResultPath+".continue", nil, 0600); e != nil {
				t.Fatal(e)
			}
			select {
			case result := <-done:
				if result.err != nil || src.checkpoints != 1 {
					t.Fatal("overlap checkpoint", result.err, src.checkpoints)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("overlap owner did not finish")
			}
		})
	}
}

// TESTMASTER: id=indigobird-ordered-profiles-overdrive tier=fast parallel=yes
func TestIndigobirdUnorderedOverdriveSelection(t *testing.T) {
	for _, scenario := range []string{"nearest-trusted", "unknown-later", "fair-tie", "window-transition"} {
		t.Run(scenario, func(t *testing.T) {
			s, src, c := overdriveFixture(t, "")
			twoSubscriptions(&c)
			c.Subscriptions[1].OverdriveEnabled = true
			w := c.Subscriptions[0].OverdriveWindows[0]
			w.ID = "second-over"
			w.Name = "Second capacity"
			c.Subscriptions[1].OverdriveWindows = []model.OverdriveWindow{w}
			c.Automations[0].Mode = "overdrive"
			c.Automations[0].OnlyOverdrive = true
			c.Automations[0].Entries = nil
			c.Automations[0].Participants = []string{"second-sequence", "sequence"}
			if scenario == "window-transition" {
				c.Subscriptions[1].OverdriveWindows[0].Recurrence.Start = "13:00"
				c.Subscriptions[1].OverdriveWindows[0].Recurrence.End = "15:00"
			}
			collect := s.CollectUsage
			s.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
				u, e := collect(ctx, sub)
				if scenario == "nearest-trusted" && sub.ID == "second" {
					u.Weekly.ResetsAt = s.now().Add(2 * 24 * time.Hour)
				}
				if scenario == "unknown-later" && sub.ID == "account" {
					u.Weekly.ResetTrusted = false
					u.Weekly.ResetsAt = s.now().Add(time.Hour)
				}
				return u, e
			}
			if _, e := s.Apply(c.Revision, c); e != nil {
				t.Fatal(e)
			}
			wants := []string{"second"}
			if scenario == "fair-tie" {
				wants = []string{"account", "second", "account"}
			}
			if scenario == "window-transition" {
				wants = []string{"account", "second"}
			}
			for i, want := range wants {
				if scenario == "window-transition" && i == 1 {
					base := s.now()
					s.Clock = func() time.Time { return base.Add(2 * time.Hour) }
				}
				restarted := NewService(s.Dir, src)
				restarted.PollInterval = 10 * time.Millisecond
				restarted.Clock = s.Clock
				restarted.CollectUsage = s.CollectUsage
				out, e := restarted.Tick(context.Background())
				if e != nil || out.Assignment == nil || out.Assignment.Subscription.ID != want || !out.Assignment.Budget.OverdriveActive {
					t.Fatal("unordered overdrive winner", scenario, i, want, out, e)
				}
			}
		})
	}
}

// TESTMASTER: id=indigobird-engine-policy-metadata tier=fast parallel=yes
func TestIndigobirdResetMetadataPrecedence(t *testing.T) {
	s, src, c := overdriveFixture(t, "")
	c.Subscriptions[0].Tier = "Custom enterprise allowance"
	c.Subscriptions[0].TypicalReset = model.ResetSchedule{Day: 6, Time: "00:00", Timezone: "UTC"}
	c.Subscriptions[0].OverdriveWindows[0].StartMode = "reset_relative"
	c.Subscriptions[0].OverdriveWindows[0].EndMode = "next_reset"
	c.Subscriptions[0].OverdriveWindows[0].HoursBeforeReset = 24
	c.Automations[0].OnlyOverdrive = true
	if _, e := s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	restarted := NewService(s.Dir, src)
	restarted.Clock = s.Clock
	restarted.CollectUsage = s.CollectUsage
	loaded, e := restarted.Config()
	if e != nil || loaded.Subscriptions[0].Tier != c.Subscriptions[0].Tier || loaded.Subscriptions[0].TypicalReset != c.Subscriptions[0].TypicalReset {
		t.Fatal("metadata persistence", loaded, e)
	}
	out, e := restarted.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("typical Saturday reset overrode trusted Monday reset", out, e)
	}
	collect := s.CollectUsage
	trusted := false
	restarted.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
		u, e := collect(ctx, sub)
		u.Weekly.ResetsAt = s.now().Add(12 * time.Hour)
		u.Weekly.ResetTrusted = trusted
		return u, e
	}
	out, e = restarted.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatal("estimated reset fabricated capacity", out, e)
	}
	trusted = true
	out, e = restarted.Tick(context.Background())
	if e != nil || out.Assignment == nil || !out.Assignment.Budget.OverdriveActive || src.checkpoints != 1 {
		t.Fatal("trusted live reset did not open actual window", out, e)
	}
}

// TESTMASTER: id=indigobird-project-rules-scope tier=fast parallel=yes
func TestIndigobirdOverdriveProjectScope(t *testing.T) {
	s, src, c := overdriveFixture(t, "")
	twoSubscriptions(&c)
	c.Subscriptions[1].OverdriveEnabled = false
	c.TimeWindows = []model.TimeWindow{{ID: "closed", Name: "Closed", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "13:00", End: "14:00", Timezone: "UTC", EveryWeeks: 1}}}
	for _, id := range []string{"second-project", "closed-project", "other-engine"} {
		p := model.Project{ID: id, Name: id, Path: t.TempDir(), Enabled: true}
		if id == "closed-project" {
			p.WindowID = "closed"
		}
		c.Projects = append(c.Projects, p)
		a := c.Automations[0]
		a.ID = id + "-rule"
		a.Name = id
		a.ProjectID = id
		sequence := "sequence"
		if id == "other-engine" {
			sequence = "second-sequence"
		}
		a.Entries = []model.Entry{{ID: id + "-entry", SequenceID: sequence, MaxBand: model.Green}}
		c.Automations = append(c.Automations, a)
	}
	collect := s.CollectUsage
	s.CollectUsage = func(ctx context.Context, sub model.Subscription) (model.Usage, error) {
		u, e := collect(ctx, sub)
		u.Weekly.UsedPercent = 80
		return u, e
	}
	var e error
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"project", "second-project", "closed-project", "other-engine"} {
		out, e := s.TickProject(context.Background(), id)
		want := id == "project" || id == "second-project"
		if e != nil || (out.Assignment != nil) != want {
			t.Fatal("overdrive scope", id, out, e)
		}
		if want && !out.Assignment.Budget.OverdriveActive {
			t.Fatal("shared engine rule did not inherit override")
		}
	}
	for i := range c.PowerBands[0].GreenCurve {
		c.PowerBands[0].GreenCurve[i] = 90
		c.PowerBands[0].RedCurve[i] = 95
	}
	c.Subscriptions[0].OverdriveEnabled = false
	if _, e = s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	out, e := s.TickProject(context.Background(), "other-engine")
	if e != nil || out.Assignment == nil || out.Assignment.Budget.OverdriveActive {
		t.Fatal("universal edit did not reach independent subscription", out, e)
	}
	if src.checkpoints != 3 {
		t.Fatal("scope gates ran unexpected work", src.checkpoints)
	}
}

// TESTMASTER: id=indigobird-projects-approval tier=fast parallel=yes
func TestIndigobirdUnapprovedPlanGates(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{{Name: "draft", State: "planned"}, {Name: "paused", State: "paused", Staged: true, Blocked: true, Reason: "Paused"}, {Name: "operator", State: "blocked", Staged: true, Blocked: true, Reason: "Operator approval required"}}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil || src.checkpoints != 0 {
		t.Fatal("unapproved plan started", out, e)
	}
	if len(out.Decision.Candidates) != 2 {
		t.Fatal("blocked plans lost explanations", out.Decision.Candidates)
	}
	for _, row := range out.Decision.Candidates {
		if row.Code != "plan_blocked" {
			t.Fatal("approval gates were bypassed or masked", row)
		}
	}
}

// TESTMASTER: id=contract-3 tier=fast parallel=yes
func TestIndigobirdBoundedContractExpansion(t *testing.T) {
	s, source, _ := fixture(t, "contract")
	out, err := s.Tick(context.Background())
	if err != nil || out.Assignment == nil || out.Result == nil || out.Result.Outcome != "completed" || source.checkpoints != 1 {
		t.Fatalf("real executor did not receive bounded autonomous repair policy: %+v %v", out, err)
	}
}
