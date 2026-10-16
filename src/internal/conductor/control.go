package conductor

import (
	"context"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"strings"
	"time"
)

func (s *Service) Control(ctx context.Context, action, projectID string) error {
	return s.control(ctx, action, projectID, nil)
}
func (s *Service) ControlExpected(ctx context.Context, action, projectID string, revision int64) error {
	return s.control(ctx, action, projectID, &revision)
}
func (s *Service) control(ctx context.Context, action, projectID string, expected *int64) error {
	if action == "kill" {
		var r Runtime
		e := s.withLock(func() error {
			c, x := s.config()
			if x != nil {
				return x
			}
			if expected != nil && c.Revision != *expected {
				return ErrConflict
			}
			r, x = s.runtime()
			if x != nil {
				return x
			}
			for id, l := range r.Leases {
				if projectID != "" && l.Assignment.ProjectID != projectID {
					continue
				}
				l.Interrupt = true
				r.Leases[id] = l
			}
			return s.saveRuntime(&r)
		})
		if e != nil {
			return e
		}
		for id, l := range r.Leases {
			if projectID != "" && l.Assignment.ProjectID != projectID {
				continue
			}
			s.mu.Lock()
			cancel := s.cancels[id]
			s.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			if !processAlive(l.OwnerPID, l.OwnerStart) {
				_ = signalOwned(l.PID, l.ProcessStart, false)
				deadline := time.Now().Add(s.GracePeriod)
				for processAlive(l.PID, l.ProcessStart) && time.Now().Before(deadline) {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(20 * time.Millisecond):
					}
				}
				_ = signalOwned(l.PID, l.ProcessStart, true)
			}
		}
		return s.Reconcile(ctx)
	}
	return s.withLock(func() error {
		c, e := s.config()
		if e != nil {
			return e
		}
		if expected != nil && c.Revision != *expected {
			return ErrConflict
		}
		if projectID == "" {
			switch action {
			case "start", "resume":
				c.Enabled = true
				c.Paused = false
			case "pause":
				c.Paused = true
			case "stop":
				c.Enabled = false
			default:
				return errors.New("unknown lifecycle action")
			}
		} else {
			found := false
			for i := range c.Projects {
				p := &c.Projects[i]
				if p.ID != projectID {
					continue
				}
				found = true
				switch action {
				case "start", "resume":
					p.Enabled = true
					p.Paused = false
				case "pause":
					p.Paused = true
				case "stop":
					p.Enabled = false
				default:
					return errors.New("unknown lifecycle action")
				}
			}
			if !found {
				return errors.New("project not found")
			}
		}
		c.Revision++
		return atomicJSON(s.Dir+"/config.json", c)
	})
}
func (s *Service) Unblock(projectPath, plan, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("unblock reason required")
	}
	return s.withLock(func() error {
		r, e := s.runtime()
		if e != nil {
			return e
		}
		key := PlanID(projectPath, plan)
		p := r.Progress[key]
		p.Parked = false
		p.Attempts = 0
		p.Reason = reason
		p.NextKind = ""
		p.LastProgress = s.now()
		r.Progress[key] = p
		return s.saveRuntime(&r)
	})
}
func (s *Service) AcknowledgeNotice(key string) error {
	return s.withLock(func() error {
		r, e := s.runtime()
		if e != nil {
			return e
		}
		n, ok := r.Notices[key]
		if !ok {
			return errors.New("notice not found")
		}
		n.Acknowledged = true
		r.Notices[key] = n
		return s.saveRuntime(&r)
	})
}

// ImportIssues records read-only forge observations; it never mutates a remote issue.
func (s *Service) ImportIssues(projectID string, items []Intake) (int, error) {
	count := 0
	e := s.withLock(func() error {
		c, e := s.config()
		if e != nil {
			return e
		}
		var p *model.Project
		for i := range c.Projects {
			if c.Projects[i].ID == projectID {
				p = &c.Projects[i]
			}
		}
		if p == nil || !p.IntakeEnabled {
			return errors.New("issue intake disabled")
		}
		r, e := s.runtime()
		if e != nil {
			return e
		}
		for _, item := range items {
			if count >= p.MaxImport {
				break
			}
			if item.Provider != "github" && item.Provider != "gitlab" {
				return errors.New("unsupported issue provider")
			}
			if item.Repository == "" || item.Issue == "" {
				return errors.New("issue source identity required")
			}
			item.Key = fmt.Sprintf("%s:%s:%s", item.Provider, item.Repository, item.Issue)
			item.ProjectID = projectID
			item.State = "pending"
			if _, ok := r.Intake[item.Key]; ok {
				continue
			}
			r.Intake[item.Key] = item
			count++
		}
		return s.saveRuntime(&r)
	})
	return count, e
}
