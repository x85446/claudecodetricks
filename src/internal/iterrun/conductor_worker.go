package iterrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
)

// StartConductorWorker starts a detached binary client. HTTP lifetimes do not own assignments.
func StartConductorWorker(storeDir, projectID string) (int, error) {
	if storeDir == "" {
		storeDir = conductor.DefaultDir()
	}
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	if err = os.MkdirAll(storeDir, 0700); err != nil {
		return 0, err
	}
	log, err := os.OpenFile(filepath.Join(storeDir, "worker.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, err
	}
	defer log.Close()
	args := []string{"conductor", "tick", "--store", storeDir, "--worker", "--json"}
	if projectID != "" {
		args = append(args, "--project", projectID)
	}
	cmd := exec.Command(exe, args...)
	detachConductorWorker(cmd)
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	if err = cmd.Process.Release(); err != nil {
		return pid, err
	}
	return pid, nil
}

// RunConductorWorker runs bounded checkpoints until admission closes or the queue is empty.
func RunConductorWorker(ctx context.Context, s *conductor.Service, projectID string) (conductor.TickResult, error) {
	var out conductor.TickResult
	for count := 0; count < 1000; count++ {
		var err error
		out, err = s.TickProject(ctx, projectID)
		if err != nil || out.Assignment == nil {
			return out, err
		}
		if out.Result == nil || out.Result.Outcome != "completed" {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return out, fmt.Errorf("worker checkpoint bound reached; start another tick to continue")
}

// conductorWakeLoop revisits enabled queues without requiring an open browser.
// Detached workers retain execution ownership across web-service restarts.
func conductorWakeLoop(ctx context.Context, s *conductor.Service, start func(string, string) (int, error)) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := wakeConductor(ctx, s, start); err != nil {
			fmt.Fprintf(os.Stderr, "conductor wake: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func wakeConductor(ctx context.Context, s *conductor.Service, start func(string, string) (int, error)) error {
	c, err := s.Config()
	if err != nil || !c.Enabled || c.Paused {
		return err
	}
	if err = s.Reconcile(ctx); err != nil {
		return err
	}
	d, err := s.Status(ctx)
	if err != nil {
		return err
	}
	r, err := s.Runtime()
	if err != nil {
		return err
	}
	busy := map[string]bool{}
	for _, lease := range r.Leases {
		busy[lease.Assignment.ProjectID] = true
	}
	depth := map[string]int{}
	for _, queue := range d.Queues {
		depth[queue.ProjectID] = queue.Depth
	}
	for _, project := range c.Projects {
		if project.Enabled && !project.Paused && !busy[project.ID] && (depth[project.ID] > 0 || project.IntakeEnabled) {
			_, err = start(s.Dir, "")
			return err
		}
	}
	return nil
}
