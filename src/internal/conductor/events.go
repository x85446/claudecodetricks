package conductor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const EventLimit = 10000

func safeID(id string) bool {
	return id != "" && !strings.ContainsAny(id, "/\\\x00") && id != "." && id != ".."
}
func (s *Service) runDir(id string) (string, error) {
	if !safeID(id) {
		return "", errors.New("invalid run identity")
	}
	return filepath.Join(s.Dir, "runs", id), nil
}

type EventPage struct {
	Events       []model.Event `json:"events"`
	Cursor       uint64        `json:"cursor"`
	Expired      bool          `json:"expired"`
	InputTokens  int64         `json:"input_tokens"`
	OutputTokens int64         `json:"output_tokens"`
}

func (s *Service) Events(run string, after uint64, limit int) (EventPage, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	dir, e := s.runDir(run)
	if e != nil {
		return EventPage{}, e
	}
	p := EventPage{Events: []model.Event{}}
	f, e := os.Open(filepath.Join(dir, "events.jsonl"))
	if os.IsNotExist(e) {
		return p, nil
	}
	if e != nil {
		return p, e
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 1<<20)
	sessions := map[string][2]int64{}
	cumulative := map[string][2]int64{}
	hasCumulative := map[string]bool{}
	first := uint64(0)
	for scan.Scan() {
		var ev model.Event
		if e = json.Unmarshal(scan.Bytes(), &ev); e != nil {
			return p, e
		}
		if first == 0 {
			first = ev.Sequence
		}
		if ev.Sequence > after && len(p.Events) < limit {
			p.Events = append(p.Events, ev)
			p.Cursor = ev.Sequence
		}
		key := ev.SessionID
		if key == "" {
			key = ev.AssignmentID
		}
		old := sessions[key]
		if ev.Cumulative {
			old = cumulative[key]
			hasCumulative[key] = true
			if ev.InputTokens > old[0] {
				old[0] = ev.InputTokens
			}
			if ev.OutputTokens > old[1] {
				old[1] = ev.OutputTokens
			}
		} else {
			old[0] += ev.InputTokens
			old[1] += ev.OutputTokens
		}
		if ev.Cumulative {
			cumulative[key] = old
		} else {
			sessions[key] = old
		}
	}
	p.Expired = after > 0 && first > after+1
	for key, n := range cumulative {
		if hasCumulative[key] {
			sessions[key] = n
		}
	}
	for _, n := range sessions {
		p.InputTokens += n[0]
		p.OutputTokens += n[1]
	}
	return p, scan.Err()
}

type eventIndex struct {
	Sequence uint64          `json:"sequence"`
	Size     int64           `json:"size"`
	Seen     map[string]bool `json:"seen"`
}

func (s *Service) RecordEvent(a model.Assignment, event model.Event) error {
	return s.withLock(func() error {
		dir, e := s.runDir(a.RunID)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		path := filepath.Join(dir, "events.jsonl")
		idxPath := filepath.Join(dir, "events-index.json")
		if s.eventIndexes == nil {
			s.eventIndexes = map[string]eventIndex{}
		}
		idx, cached := s.eventIndexes[a.RunID]
		if !cached {
			_ = readJSON(idxPath, &idx)
		}
		if idx.Seen == nil {
			idx.Seen = map[string]bool{}
		}
		info, statErr := os.Stat(path)
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		if info != nil && info.Size() != idx.Size {
			idx = eventIndex{Seen: map[string]bool{}}
			f, x := os.Open(path)
			if x != nil {
				return x
			}
			scan := bufio.NewScanner(f)
			scan.Buffer(make([]byte, 4096), 1<<20)
			for scan.Scan() {
				var old model.Event
				if x = json.Unmarshal(scan.Bytes(), &old); x != nil {
					f.Close()
					return x
				}
				idx.Sequence = old.Sequence
				if old.ProviderID != "" {
					idx.Seen[old.AssignmentID+":"+old.ProviderID] = true
				}
			}
			x = scan.Err()
			f.Close()
			if x != nil {
				return x
			}
			idx.Size = info.Size()
		}
		key := a.ID + ":" + event.ProviderID
		if event.ProviderID != "" && idx.Seen[key] {
			return nil
		}
		event.Version = model.ProtocolVersion
		event.ProjectPath = a.ProjectPath
		event.Plan = a.Plan
		event.RunID = a.RunID
		event.AssignmentID = a.ID
		event.At = s.now()
		event.Sequence = idx.Sequence + 1
		if len(event.Message) > 8192 {
			event.Message = event.Message[:8192] + " [truncated]"
		}
		event.Message = redactPublic(event.Message)
		event.Data = nil
		b, e := json.Marshal(event)
		if e != nil {
			return e
		}
		b = append(b, '\n')
		f, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(b)
		if e == nil {
			e = f.Sync()
		}
		f.Close()
		if e != nil {
			return e
		}
		idx.Sequence = event.Sequence
		idx.Size += int64(len(b))
		if event.ProviderID != "" {
			idx.Seen[key] = true
		}
		if idx.Sequence > EventLimit && idx.Sequence%1000 == 0 {
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) > EventLimit {
				data = []byte(strings.Join(lines[len(lines)-EventLimit:], "\n") + "\n")
				if e = AtomicWrite(path, data); e != nil {
					return e
				}
				idx.Size = int64(len(data))
				idx.Seen = map[string]bool{}
				for _, line := range lines[len(lines)-EventLimit:] {
					var old model.Event
					_ = json.Unmarshal([]byte(line), &old)
					if old.ProviderID != "" {
						idx.Seen[old.AssignmentID+":"+old.ProviderID] = true
					}
				}
			}
		}
		s.eventIndexes[a.RunID] = idx
		if idx.Sequence%128 == 0 {
			return atomicJSON(idxPath, idx)
		}
		return nil
	})
}
func (s *Service) Runs() ([]Lease, error) {
	r, e := s.Runtime()
	if e != nil {
		return nil, e
	}
	out := []Lease{}
	for _, l := range r.Leases {
		out = append(out, l)
	}
	entries, e := os.ReadDir(filepath.Join(s.Dir, "runs"))
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var l Lease
		if readJSON(filepath.Join(s.Dir, "runs", entry.Name(), "run.json"), &l) == nil {
			active := false
			for _, x := range out {
				if x.Assignment.RunID == l.Assignment.RunID {
					active = true
				}
			}
			if !active {
				out = append(out, l)
			}
		}
	}
	return out, nil
}
func validateResult(a model.Assignment, r model.Result) error {
	if r.Version != model.ProtocolVersion || r.AssignmentID != a.ID || r.RunID != a.RunID || r.ProjectPath != a.ProjectPath || r.Plan != a.Plan || r.Step != a.Step || r.SourceDigest != a.SourceDigest {
		return errors.New("assignment result identity or digest mismatch")
	}
	switch r.Outcome {
	case "completed":
		if !r.TaskComplete || !r.ValidationMet || len(r.Evidence) == 0 {
			return errors.New("completion requires task, validation and evidence")
		}
		for _, v := range r.Evidence {
			if strings.TrimSpace(v) == "" {
				return errors.New("empty validation evidence")
			}
		}
	case "retry", "blocked", "partial", "interrupted":
	default:
		return fmt.Errorf("unknown result outcome %q", r.Outcome)
	}
	return nil
}

var publicSecrets = regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/=-]+|(?:sk-|ghp_|github_pat_)[a-z0-9_-]{12,}|(?:api[_-]?key|password|access[_-]?token)\s*[:=]\s*[^\s,;]+`)

func redactPublic(s string) string { return publicSecrets.ReplaceAllString(s, "[redacted]") }
func (s *Service) Observe(assignmentID string, event model.Event) error {
	r, e := s.Runtime()
	if e != nil {
		return e
	}
	lease, ok := r.Leases[assignmentID]
	if !ok {
		return errors.New("active assignment not found")
	}
	if lease.Assignment.Subscription.Adapter == "codex" && strings.HasPrefix(event.ProviderID, "tool:") {
		return nil
	}
	return s.RecordEvent(lease.Assignment, event)
}
