package conductor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"strings"
)

func updateWatch(r *Runtime, c model.Config, d Decision) {
	for _, q := range d.Queues {
		parts := []string{fmt.Sprint(c.Revision)}
		human := q.Depth > 0
		unnotified := false
		holds := map[string]int{}
		roots := []string{}
		for _, p := range q.Plans {
			parts = append(parts, p.ID+":"+p.State)
			progress := r.Progress[p.ID]
			heldByOperator := p.Waiting && rootNeedsOperator(*r, q, p.HeldBy)
			if heldByOperator {
				if holds[p.HeldBy] == 0 {
					roots = append(roots, p.HeldBy)
				}
				holds[p.HeldBy]++
			}
			if !p.Blocked && !progress.Parked && !heldByOperator {
				human = false
			}
			if progress.Parked {
				if _, exists := r.Notices[p.ID+":"+progress.Signature]; !exists {
					unnotified = true
				}
			} else if p.Blocked {
				unnotified = true
			}
		}
		for _, x := range d.Candidates {
			if x.Project.ID == q.ProjectID {
				parts = append(parts, x.Automation.ID+":"+x.EntryID+":"+x.Code)
			}
		}
		sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
		signature := hex.EncodeToString(sum[:])
		w := r.Watches[q.ProjectID]
		if w.Signature != signature {
			w = Watch{Signature: signature}
		}
		w.Attempts++
		bound := 10
		for _, p := range c.Projects {
			if p.ID == q.ProjectID {
				bound = p.WatchBound
			}
		}
		w.State = "watching"
		if human || w.Attempts >= bound {
			w.State = "stood_down"
		}
		r.Watches[q.ProjectID] = w
		if human && unnotified {
			key := "human:" + q.ProjectID + ":" + signature
			if _, ok := r.Notices[key]; !ok {
				r.Notices[key] = Notice{Key: key, ProjectID: q.ProjectID, Message: operatorMessage(roots, holds), At: d.At}
			}
		}
	}
}
func watchState(r Runtime, c model.Config, d Decision) string {
	if !c.Enabled || c.Paused {
		return "stood_down"
	}
	allDown := len(d.Queues) > 0
	for _, q := range d.Queues {
		if r.Watches[q.ProjectID].State != "stood_down" {
			allDown = false
		}
	}
	if allDown {
		return "stood_down"
	}
	return "watching"
}

// rootNeedsOperator reports whether the plan holding a waiting branch can
// only move because a human acts: it is blocked or parked, or it is absent
// from the queue (an unstaged plan, a closed archive). A root that is
// running, queued or executing may still finish by itself.
func rootNeedsOperator(r Runtime, q Queue, root string) bool {
	if root == "" {
		return false
	}
	for _, name := range q.RunningPlans {
		if name == root {
			return false
		}
	}
	for _, p := range q.Plans {
		if p.Name != root {
			continue
		}
		if p.Waiting {
			return false
		}
		return p.Blocked || (r.Progress[p.ID].Parked && p.State != "unblocked")
	}
	return true
}

func operatorMessage(roots []string, holds map[string]int) string {
	if len(roots) == 0 {
		return "Queued plans require operator attention"
	}
	parts := make([]string, 0, len(roots))
	for _, root := range roots {
		noun := "plans"
		if holds[root] == 1 {
			noun = "plan"
		}
		parts = append(parts, fmt.Sprintf("%s holds %d %s", root, holds[root], noun))
	}
	return "Queued plans require operator attention: " + strings.Join(parts, "; ")
}
