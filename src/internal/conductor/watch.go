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
		for _, p := range q.Plans {
			parts = append(parts, p.ID+":"+p.State)
			progress := r.Progress[p.ID]
			if !p.Blocked && !progress.Parked {
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
				r.Notices[key] = Notice{Key: key, ProjectID: q.ProjectID, Message: "Queued plans require operator attention", At: d.At}
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
