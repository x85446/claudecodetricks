package conductor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/conductor/provider"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type TickResult struct {
	Decision   Decision          `json:"decision"`
	Assignment *model.Assignment `json:"assignment,omitempty"`
	Result     *model.Result     `json:"result,omitempty"`
	State      string            `json:"state"`
}

func (s *Service) Tick(ctx context.Context) (TickResult, error) { return s.TickProject(ctx, "") }
func (s *Service) TickProject(ctx context.Context, projectID string) (out TickResult, err error) {
	if s.Source == nil {
		return out, errors.New("plan source required")
	}
	if err = s.Reconcile(ctx); err != nil {
		return out, err
	}
	config, xerr := s.Config()
	if xerr != nil {
		return out, xerr
	}
	if config.Enabled && !config.Paused {
		runtime, x := s.Runtime()
		if x != nil {
			return out, x
		}
		for _, p := range config.Projects {
			if p.Enabled && !p.Paused && p.IntakeEnabled && (projectID == "" || projectID == p.ID) && s.now().Sub(runtime.IntakePoll[p.ID]) >= time.Minute {
				_, _ = s.FetchIssues(ctx, p.ID)
			}
		}
		for _, sub := range config.Subscriptions {
			if !sub.Enabled || sub.AccountID == "" || sub.PoolID == "" {
				continue
			}
			collect := s.CollectUsage
			if collect == nil {
				collect = provider.CollectUsage
			}
			u, x := collect(ctx, sub)
			if x != nil {
				u = model.Usage{SubscriptionID: sub.ID, PoolID: sub.PoolID, AccountID: sub.AccountID, ObservedAt: s.now(), Source: "collector", Reason: x.Error()}
			}
			if x = s.PutUsage(u); x != nil {
				return out, x
			}
		}
	}
	var a model.Assignment
	err = s.withLock(func() error {
		c, e := s.config()
		if e != nil {
			return e
		}
		r, e := s.runtime()
		if e != nil {
			return e
		}
		d, e := s.evaluate(ctx, c, r, s.now(), EvaluateRequest{ProjectID: projectID})
		if e != nil {
			return e
		}
		out.Decision = d
		r.Occurrences = d.Occurrences
		if d.Selected == nil {
			updateWatch(&r, c, d)
			out.State = watchState(r, c, d)
			return s.saveRuntime(&r)
		}
		x := *d.Selected
		r.Watches[x.Project.ID] = Watch{State: "working"}
		var pair Pair
		if x.Plan.Kind == "intake" {
			for _, item := range r.Intake {
				if item.ProjectID == x.Project.ID && intakeName(item) == x.Plan.Name {
					pair = Pair{Task: "Triage this issue into one unstaged iterate plan at .claude/iterate/plans/" + x.Plan.Name + ".md. Use numbered task/validation pairs and phase: planned. Do not queue, execute, close, comment or label the issue. Include source provenance " + item.Key + ".\nIssue: " + item.Title + "\n" + item.Body, Validation: "Verify the saved plan has paired steps and validations, source provenance, phase planned, and no queued or executing status.", Digest: item.Key}
					break
				}
			}
			if pair.Digest == "" {
				return errors.New("intake source disappeared")
			}
		} else {
			pair, e = s.Source.Pair(ctx, x.Project, x.Plan.Name)
			if e != nil {
				return e
			}
		}
		pg := r.Progress[x.Plan.ID]
		if x.Plan.State == "unblocked" {
			// Explicit recovery starts a fresh attempt budget; parked time is not execution time.
			pg.Parked = false
			pg.Attempts = 0
			pg.AttemptsByState = nil
			pg.LastProgress = s.now()
		}
		kind := "pair"
		if pg.NextKind != "" {
			kind = pg.NextKind
		}
		if x.Plan.Complete {
			kind = "delivery"
		}
		if x.Plan.Kind != "" {
			kind = x.Plan.Kind
		}
		if pair.Step == 0 && kind == "pair" {
			kind = "delivery"
		}
		if kind == "diagnosis" {
			pair.Task = "Diagnose the failing pair and propose one bounded recovery. Do not repeat delivery or change plan checkboxes. Last outcome: " + pg.Reason
			pair.Validation = "Record concrete observations and a safe next action in result evidence."
		}
		if kind == "delivery" {
			pair.Task = "Verify final delivery of this plan using its documented delivery policy. Preserve branch and validation evidence. Perform only authorized delivery actions and return their durable references."
			pair.Validation = "Verify required delivery evidence and report completed only when every delivery requirement is met."
			if pg.LastResult != nil && pg.LastResult.Outcome == "partial" {
				pair.Task = "Land only the completed validated portion of this plan using its authorized delivery policy. Preserve unfinished work and branch history. Return outcome partial with concrete landing references; do not archive or claim whole-plan completion."
				pair.Validation = "Verify partial landing references and identify the unfinished work and human gate."
			}
		}
		a = model.Assignment{Version: model.ProtocolVersion, ID: NewID("assignment"), RunID: NewID("run"), ProjectID: x.Project.ID, ProjectPath: x.Project.Path, Plan: x.Plan.Name, Step: pair.Step, Team: pair.Team, Kind: kind, SourceDigest: pair.Digest, Task: pair.Task, Validation: pair.Validation, ConfigRevision: c.Revision, Subscription: x.Subscription, Sequence: x.Sequence, Automation: x.Automation, Budget: x.Budget, CreatedAt: s.now()}
		dir, _ := s.runDir(a.RunID)
		a.ResultPath = filepath.Join(dir, "result.json")
		a.Prompt = "Read .claude/iterate/plans/" + a.Plan + ".md for the goal, constraints, accepted decisions and branch policy. Do not invoke the whole iterate/conductor skill.\nAssignment kind: " + kind + "\nTask: " + a.Task + "\nRequired validation: " + a.Validation + "\nWork only on this bounded assignment. Do not edit plan checkboxes or start another pair. Preserve unrelated changes. Resolve implementation choices within the assignment yourself. A required API or contract expansion, parity repair, or regression in this assigned work is work to fix and validate; continue instead of requesting a human decision. Escalate only for a secret no agent has, physical access, or an external party."
		if a.Subscription.ResultTransport != "prompt_file" {
			a.Prompt = provider.AssignmentPrompt(a)
		}
		if _, e = provider.Compile(a.Subscription, a.Sequence, a); e != nil {
			return e
		}
		l := Lease{Assignment: a, OwnerPID: os.Getpid(), OwnerStart: processIdentity(os.Getpid()), Started: s.now(), Status: "claimed"}
		if x.Occurrence != nil {
			o := *x.Occurrence
			if o.ClaimedPlan == "" || a.Automation.RunLimit == "drain_queue" {
				o.ClaimedPlan = x.Plan.ID
			}
			l.OccurrenceID = o.ID
			r.Occurrences = mergeOccurrences(r.Occurrences, []model.Occurrence{o})
		}
		r.Leases[a.ID] = l
		r.LastProject = x.Project.ID
		r.LastSubscription = x.Subscription.ID
		if pg.Started.IsZero() {
			pg.Started = s.now()
			pg.LastProgress = s.now()
		} else if previous := pg.LastResult; previous != nil && previous.Outcome == "completed" && pg.CompletedSteps[previous.Step] == previous.SourceDigest && (a.Step != previous.Step || a.Kind == "delivery") {
			// A new checkpoint starts its no-progress budget after any gated idle interval.
			pg.LastProgress = s.now()
		}
		r.Progress[x.Plan.ID] = pg
		if e = atomicJSON(filepath.Join(dir, "run.json"), l); e != nil {
			return e
		}
		if e = s.saveRuntime(&r); e != nil {
			return e
		}
		if source, ok := s.Source.(LeaseSource); ok && a.Kind != "intake" {
			if e = source.Claim(ctx, a); e != nil {
				l.Status = "admission_failed"
				_ = atomicJSON(filepath.Join(dir, "run.json"), l)
				_ = source.Release(ctx, a)
				delete(r.Leases, a.ID)
				if saveErr := s.saveRuntime(&r); saveErr != nil {
					return saveErr
				}
				return e
			}
		}
		out.Assignment = &a
		out.State = "running"
		return nil
	})
	if err != nil || out.Assignment == nil {
		return out, err
	}
	result, runErr := s.execute(ctx, a)
	dir, _ := s.runDir(a.RunID)
	_ = readJSON(filepath.Join(dir, "validated-result.json"), &result)
	out.Result = &result
	out.State = result.Outcome
	return out, runErr
}
func (s *Service) execute(ctx context.Context, a model.Assignment) (model.Result, error) {
	result := model.Result{Version: model.ProtocolVersion, AssignmentID: a.ID, RunID: a.RunID, ProjectPath: a.ProjectPath, Plan: a.Plan, Step: a.Step, SourceDigest: a.SourceDigest, Outcome: "interrupted"}
	argv, e := provider.Compile(a.Subscription, a.Sequence, a)
	if e != nil {
		return result, s.finish(ctx, a, result, e)
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	s.cancels[a.ID] = cancel
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.cancels, a.ID); s.mu.Unlock() }()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = a.ProjectPath
	cmd.Env = provider.ExecutionEnv(a.Subscription.Adapter, os.Environ())
	cmd.Env = append(cmd.Env, "ITERATE_CONDUCTOR_DIR="+s.Dir, "ITERATE_CONDUCTOR_ASSIGNMENT_ID="+a.ID, "ITERATE_CONDUCTOR_RUN_ID="+a.RunID)
	processGroup(cmd)
	stdout, outWriter, e := os.Pipe()
	if e != nil {
		return result, s.finish(ctx, a, result, e)
	}
	defer stdout.Close()
	defer outWriter.Close()
	stderr, errWriter, e := os.Pipe()
	if e != nil {
		return result, s.finish(ctx, a, result, e)
	}
	defer stderr.Close()
	defer errWriter.Close()
	cmd.Stdout = outWriter
	cmd.Stderr = errWriter
	dir, _ := s.runDir(a.RunID)
	outLog, e := os.OpenFile(filepath.Join(dir, "stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return result, s.finish(ctx, a, result, e)
	}
	defer outLog.Close()
	errLog, e := os.OpenFile(filepath.Join(dir, "stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return result, s.finish(ctx, a, result, e)
	}
	defer errLog.Close()
	if e = cmd.Start(); e != nil {
		return result, s.finish(ctx, a, result, e)
	}
	_ = outWriter.Close()
	_ = errWriter.Close()
	identity := processIdentity(cmd.Process.Pid)
	e = s.withLock(func() error {
		r, x := s.runtime()
		if x != nil {
			return x
		}
		l := r.Leases[a.ID]
		l.PID = cmd.Process.Pid
		l.ProcessStart = identity
		l.Status = "running"
		r.Leases[a.ID] = l
		if x = atomicJSON(filepath.Join(dir, "run.json"), l); x != nil {
			return x
		}
		return s.saveRuntime(&r)
	})
	if e != nil {
		_ = signalOwned(cmd.Process.Pid, identity, true)
		_ = cmd.Wait()
		return result, s.finish(ctx, a, result, e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	var candidate *model.Result
	var candidateMu sync.Mutex
	wg.Add(2)
	go func() {
		defer wg.Done()
		reader := io.TeeReader(stdout, &boundedLog{w: outLog, max: 64 << 20})
		if a.Subscription.Adapter == "generic" || a.Subscription.Adapter == "custom" {
			_, err := io.Copy(io.Discard, reader)
			if err != nil {
				errs <- err
				_, _ = io.Copy(io.Discard, stdout)
			}
			return
		}
		err := provider.Decode(context.Background(), a.Subscription.Adapter, reader, func(ev model.Event) error {
			if ev.Kind == "allowance" && (a.Subscription.Adapter == "claude" || a.Subscription.Adapter == "claude-code") {
				if u, x := provider.NormalizeClaudeUsage(a.Subscription, ev.Data, s.now(), a.Subscription.AccountID); x == nil {
					if x = s.PutUsage(u); x != nil {
						return x
					}
				}
			}
			if r, x := provider.ExtractResult(ev); x == nil {
				candidateMu.Lock()
				candidate = &r
				candidateMu.Unlock()
			}
			return s.RecordEvent(a, ev)
		})
		if err != nil {
			errs <- err
			_, _ = io.Copy(io.Discard, stdout)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := io.Copy(&boundedLog{w: errLog, max: 64 << 20}, stderr)
		if err != nil {
			errs <- err
			_, _ = io.Copy(io.Discard, stderr)
		}
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	interval := s.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	interrupted := false
	var waitErr, errorCapture error
	running := true
	leaderDone, streamsDone := false, false
	for running {
		select {
		case waitErr = <-done:
			leaderDone = true
			done = nil
			if n := terminateRemainingGroup(cmd.Process.Pid); n > 0 && errorCapture == nil {
				errorCapture = errors.New("executor left running descendants; owned process group cleaned")
			}
			running = !(leaderDone && streamsDone)
		case <-drained:
			streamsDone = true
			drained = nil
			running = !(leaderDone && streamsDone)
		case err := <-errs:
			if errorCapture == nil {
				errorCapture = err
			}
			cancel()
		case <-childCtx.Done():
			if !interrupted {
				interrupted = true
				_ = signalOwned(cmd.Process.Pid, identity, false)
				grace := s.GracePeriod
				if grace <= 0 {
					grace = 2 * time.Second
				}
				time.AfterFunc(grace, func() { _ = signalOwned(cmd.Process.Pid, identity, true) })
			}
		case <-ticker.C:
			stop, x := s.mustInterrupt(a)
			if x != nil {
				errorCapture = x
				cancel()
			}
			if stop {
				cancel()
			}
		}
	}
	close(errs)
	for x := range errs {
		if errorCapture == nil {
			errorCapture = x
		}
	}
	_ = outLog.Sync()
	_ = errLog.Sync()
	if errorCapture != nil {
		e = errorCapture
	} else if interrupted {
		e = errors.New("owned assignment interrupted")
	} else if waitErr != nil {
		e = waitErr
	}
	if e == nil {
		b, x := os.ReadFile(a.ResultPath)
		if x == nil {
			if len(b) > 1<<20 {
				x = errors.New("result exceeds 1 MiB")
			} else {
				x = json.Unmarshal(b, &result)
			}
		}
		if x != nil {
			candidateMu.Lock()
			if candidate != nil {
				result = *candidate
				x = nil
			}
			candidateMu.Unlock()
		}
		if x != nil {
			e = fmt.Errorf("assignment result unavailable: %w", x)
		} else {
			e = validateResult(a, result)
		}
	}
	if e != nil {
		result.Outcome = "retry"
		result.TaskComplete = false
		result.ValidationMet = false
		result.Blocker = e.Error()
		if interrupted {
			result.Outcome = "interrupted"
		}
	}
	return result, s.finish(context.Background(), a, result, e)
}

type boundedLog struct {
	w   io.Writer
	n   int64
	max int64
}

func (w *boundedLog) Write(p []byte) (int, error) {
	if w.n+int64(len(p)) > w.max {
		return 0, errors.New("run log exceeds 64 MiB retention bound")
	}
	n, e := w.w.Write(p)
	w.n += int64(n)
	return n, e
}
func (s *Service) mustInterrupt(a model.Assignment) (bool, error) {
	stop := false
	if source, ok := s.Source.(LeaseSource); ok && a.Kind != "intake" {
		if e := source.Heartbeat(context.Background(), a); e != nil {
			return true, e
		}
	}
	e := s.withLock(func() error {
		r, e := s.runtime()
		if e != nil {
			return e
		}
		l, ok := r.Leases[a.ID]
		if !ok {
			return errors.New("assignment lease missing")
		}
		if l.Interrupt {
			stop = true
		}
		c, e := s.config()
		if e != nil {
			return e
		}
		for _, p := range c.Projects {
			if p.ID == a.ProjectID && p.NoProgressSeconds > 0 {
				pg := r.Progress[PlanID(a.ProjectPath, a.Plan)]
				if s.now().Sub(pg.LastProgress) >= time.Duration(p.NoProgressSeconds)*time.Second {
					stop = true
				}
			}
			if p.ID == a.ProjectID && p.PlanCapSeconds > 0 {
				pg := r.Progress[PlanID(a.ProjectPath, a.Plan)]
				if s.now().Sub(pg.Started) >= time.Duration(p.PlanCapSeconds)*time.Second {
					stop = true
				}
			}
		}
		if a.Automation.Mode == "scheduled" && l.OccurrenceID != "" {
			for i, o := range r.Occurrences {
				if o.ID == l.OccurrenceID && !s.now().Before(o.End) {
					r.Occurrences[i].ClosingAction = a.Automation.OnWindowEnd
					if a.Automation.OnWindowEnd == "interrupt" {
						stop = true
					}
					return s.saveRuntime(&r)
				}
			}
		}
		return nil
	})
	return stop, e
}
func (s *Service) finish(ctx context.Context, a model.Assignment, result model.Result, cause error) error {
	dir, _ := s.runDir(a.RunID)
	if e := atomicJSON(filepath.Join(dir, "validated-result.json"), result); e != nil {
		return e
	}
	if e := s.RecordEvent(a, model.Event{Kind: "assignment_result", ProviderID: "result:" + a.ID, Message: result.Outcome}); e != nil {
		return e
	}
	return s.withLock(func() error {
		r, e := s.runtime()
		if e != nil {
			return e
		}
		l, exists := r.Leases[a.ID]
		if !exists {
			return nil
		}
		if l.PID > 0 && processAlive(l.PID, l.ProcessStart) {
			return errors.New("cannot release ownership while child is alive")
		}
		key := PlanID(a.ProjectPath, a.Plan)
		pg := r.Progress[key]
		if pg.CompletedSteps == nil {
			pg.CompletedSteps = map[int]string{}
		}
		if pg.Diagnosed == nil {
			pg.Diagnosed = map[string]bool{}
		}
		if result.Outcome == "completed" && cause == nil {
			if e = validateResult(a, result); e != nil {
				return e
			}
			if a.Kind == "pair" {
				if pg.CompletedSteps[a.Step] != a.SourceDigest {
					if e = s.Source.Checkpoint(ctx, a, result); e != nil {
						result.Outcome = "retry"
						result.Blocker = "checkpoint reconciliation: " + e.Error()
						cause = e
					} else {
						pg.CompletedSteps[a.Step] = a.SourceDigest
						pg.Attempts = 0
						pg.NextKind = ""
						pg.LastProgress = s.now()
					}
				}
			} else if a.Kind == "diagnosis" {
				pg.Diagnosed[pg.Signature] = true
				pg.NextKind = ""
				pg.ReconcileSequence = ""
			} else if handler, ok := s.Source.(OutcomeSource); ok {
				if e = handler.Outcome(ctx, a, result); e != nil {
					result.Outcome = "blocked"
					result.Blocker = e.Error()
				}
			} else {
				result.Outcome = "blocked"
				result.Blocker = "delivery or intake source handler unavailable"
			}
		}
		if result.Outcome != "completed" {
			sum := sha256.Sum256([]byte(a.SourceDigest + result.Blocker))
			signature := hex.EncodeToString(sum[:])
			if pg.AttemptsByState == nil {
				pg.AttemptsByState = map[string]int{}
			}
			pg.AttemptsByState[signature]++
			pg.Attempts = pg.AttemptsByState[signature]
			if a.Kind == "diagnosis" {
				pg.Diagnosed[signature] = true
				pg.NextKind = ""
			}
			if pg.Signature != signature {
				pg.Signature = signature
			}
			pg.Reason = result.Blocker
			max := 5
			timeout := 2700
			for _, p := range mustConfig(s).Projects {
				if p.ID == a.ProjectID {
					max = p.MaxAttempts
					timeout = p.NoProgressSeconds
				}
			}
			if result.Outcome == "interrupted" {
				pg.ReconcileSequence = a.Sequence.ID
				pg.NextKind = "diagnosis"
			}
			pendingPartial := result.Outcome == "partial" && a.Kind == "pair" && len(result.Evidence) > 0
			if pendingPartial {
				pg.NextKind = "delivery"
				pg.Parked = false
			} else if result.Outcome == "blocked" || result.Outcome == "partial" || pg.Attempts >= max || s.now().Sub(pg.LastProgress) >= time.Duration(timeout)*time.Second {
				pg.Parked = true
				pg.NextKind = ""
				noticeKey := key + ":" + signature
				if _, ok := r.Notices[noticeKey]; !ok {
					r.Notices[noticeKey] = Notice{Key: noticeKey, ProjectID: a.ProjectID, Message: pg.Reason, At: s.now()}
				}
				if h, ok := s.Source.(OutcomeSource); ok {
					if e = h.Outcome(ctx, a, result); e != nil {
						return e
					}
				}
			} else if result.Outcome != "interrupted" && !pg.Diagnosed[signature] {
				pg.NextKind = "diagnosis"
			}
		}
		if a.Kind == "intake" {
			for key, item := range r.Intake {
				if item.ProjectID == a.ProjectID && intakeName(item) == a.Plan {
					if result.Outcome == "completed" {
						item.State = "completed"
					} else if pg.Parked {
						item.State = "parked"
					}
					r.Intake[key] = item
				}
			}
		}
		if e = atomicJSON(filepath.Join(dir, "validated-result.json"), result); e != nil {
			return e
		}
		pg.LastResult = &result
		r.Progress[key] = pg
		for i, o := range r.Occurrences {
			if o.ClaimedPlan == key && (a.Automation.RunLimit == "one_plan" || o.ClosingAction != "") && (pg.Parked || a.Kind == "delivery" && result.Outcome == "completed") {
				r.Occurrences[i].Completed = true
			}
			if o.ID == l.OccurrenceID && !s.now().Before(o.End) && o.ClosingAction == "" {
				r.Occurrences[i].ClosingAction = a.Automation.OnWindowEnd
			}
		}
		if source, ok := s.Source.(LeaseSource); ok && a.Kind != "intake" {
			if e = source.Release(ctx, a); e != nil {
				return e
			}
		}
		delete(r.Leases, a.ID)
		l.Status = result.Outcome
		if e = atomicJSON(filepath.Join(dir, "run.json"), l); e != nil {
			return e
		}
		if e = s.saveRuntime(&r); e != nil {
			return e
		}
		return cause
	})
}
func mustConfig(s *Service) model.Config { c, _ := s.config(); return c }

// Reconcile never replays an uncertain child or delivery side effect.
func (s *Service) Reconcile(ctx context.Context) error {
	r, e := s.Runtime()
	if e != nil {
		return e
	}
	for _, l := range r.Leases {
		if processAlive(l.OwnerPID, l.OwnerStart) {
			continue
		}
		if processAlive(l.PID, l.ProcessStart) {
			if e = s.withLock(func() error {
				current, x := s.runtime()
				if x != nil {
					return x
				}
				lease, ok := current.Leases[l.Assignment.ID]
				if !ok {
					return nil
				}
				lease.Status = "orphaned"
				current.Leases[l.Assignment.ID] = lease
				key := "orphan:" + l.Assignment.ID
				if _, ok := current.Notices[key]; !ok {
					current.Notices[key] = Notice{Key: key, ProjectID: l.Assignment.ProjectID, Message: "Owned child remains alive without its worker; explicit kill or observation required", At: s.now()}
				}
				return s.saveRuntime(&current)
			}); e != nil {
				return e
			}
			continue
		}
		var result model.Result
		dir, _ := s.runDir(l.Assignment.RunID)
		e = readJSON(filepath.Join(dir, "validated-result.json"), &result)
		if e != nil {
			e = readJSON(l.Assignment.ResultPath, &result)
		}
		if e == nil {
			e = validateResult(l.Assignment, result)
		}
		if e != nil {
			a := l.Assignment
			result = model.Result{Version: 1, AssignmentID: a.ID, RunID: a.RunID, ProjectPath: a.ProjectPath, Plan: a.Plan, Step: a.Step, SourceDigest: a.SourceDigest, Outcome: "interrupted", Blocker: "worker ended without validated checkpoint; reconcile before retry"}
			if a.Kind == "delivery" {
				result.Outcome = "blocked"
				result.Blocker = "ambiguous delivery requires operator observation"
			}
		}
		if x := s.finish(ctx, l.Assignment, result, nil); x != nil {
			return x
		}
	}
	return nil
}

// ResolveWindow exposes the same recurrence evaluator to clients.
func ResolveWindow(w model.TimeWindow, u *model.Allowance, now time.Time) (model.Occurrence, bool, error) {
	return budget.ResolveTimeWindow(w, u, now)
}
