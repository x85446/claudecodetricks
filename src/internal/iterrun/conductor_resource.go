package iterrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
)

var ErrConductorNotFound = errors.New("definition not found; reload the collection and choose an existing ID")

var conductorCollections = map[string]string{"project": "projects", "subscription": "subscriptions", "sequence": "sequences", "preset": "presets", "power-band": "power_bands", "time-window": "time_windows", "overdrive": "overdrive_windows", "automation": "automations", "automation-attachment": "automation_attachments"}

// ConductorResource uses the same full-document validation and atomic transaction as config apply.
func ConductorResource(s *conductor.Service, noun, verb, id, parent string, body []byte, expected *int64, dryRun bool) (any, int64, error) {
	c, err := s.Config()
	if err != nil {
		return nil, 0, err
	}
	key, ok := conductorCollections[noun]
	if !ok {
		return nil, c.Revision, fmt.Errorf("unknown resource %q", noun)
	}
	raw, _ := json.Marshal(c)
	root := map[string]any{}
	_ = json.Unmarshal(raw, &root)
	owner := root
	if noun == "sequence" || noun == "preset" || noun == "overdrive" {
		found := false
		subs, _ := root["subscriptions"].([]any)
		for _, v := range subs {
			sub := v.(map[string]any)
			if sub["id"] == parent {
				owner = sub
				found = true
				break
			}
		}
		if !found {
			return nil, c.Revision, fmt.Errorf("subscription %q: %w", parent, ErrConductorNotFound)
		}
	}
	items, _ := owner[key].([]any)
	if items == nil {
		items = []any{}
	}
	idx := -1
	for i, v := range items {
		m := v.(map[string]any)
		if m["id"] == id {
			idx = i
			break
		}
	}
	if verb == "list" {
		return items, c.Revision, nil
	}
	if verb == "show" {
		if idx < 0 {
			return nil, c.Revision, ErrConductorNotFound
		}
		if noun == "project" {
			d, e := s.Evaluate(context.Background(), conductor.EvaluateRequest{ProjectID: id})
			if e != nil {
				return nil, c.Revision, e
			}
			m := items[idx].(map[string]any)
			for _, q := range d.Queues {
				if q.ProjectID == id {
					m["queue"] = q
				}
			}
			m["candidates"] = d.Candidates
			return m, c.Revision, nil
		}
		return items[idx], c.Revision, nil
	}
	if expected == nil {
		return nil, c.Revision, errors.New("expected revision required; read config show and pass --revision")
	}
	if *expected != c.Revision {
		return nil, c.Revision, conductor.ErrConflict
	}
	var result any
	switch verb {
	case "create", "edit", "duplicate":
		m := map[string]any{}
		if verb != "create" {
			if idx < 0 {
				return nil, c.Revision, ErrConductorNotFound
			}
			b, _ := json.Marshal(items[idx])
			_ = json.Unmarshal(b, &m)
		}
		if len(body) > 0 {
			var patch map[string]any
			if err = json.Unmarshal(body, &patch); err != nil {
				return nil, c.Revision, err
			}
			for k, v := range patch {
				m[k] = v
			}
		}
		if verb == "edit" {
			m["id"] = id
		} else {
			if verb == "duplicate" || m["id"] == nil || m["id"] == "" {
				m["id"] = conductor.NewID(noun)
			}
		}
		if verb == "duplicate" {
			if len(body) == 0 {
				if n, ok := m["name"].(string); ok {
					m["name"] = n + " copy"
				}
			}
			if noun == "subscription" {
				remapSubscriptionIDs(m)
				m["enabled"] = false
			}
			if noun == "automation" {
				if entries, ok := m["entries"].([]any); ok {
					for _, en := range entries {
						en.(map[string]any)["id"] = conductor.NewID("entry")
					}
				}
			}
		}
		if noun == "project" {
			path, _ := m["path"].(string)
			canonical, e := conductor.Canonical(path)
			if path == "" || e != nil {
				return nil, c.Revision, errors.New("project.path: choose an existing accessible directory")
			}
			m["path"] = canonical
			if m["name"] == nil || m["name"] == "" {
				m["name"] = filepath.Base(canonical)
			}
			if verb != "edit" {
				m["enabled"] = false
			}
		}
		if noun == "overdrive" && m["target_used_percent"] == nil {
			return nil, c.Revision, errors.New("overdrive.target_used_percent: enter a used percentage between 0 and 100")
		}
		if verb == "edit" {
			items[idx] = m
		} else {
			items = append(items, m)
		}
		result = m
	case "delete":
		if idx < 0 {
			return nil, c.Revision, ErrConductorNotFound
		}
		if noun == "power-band" && id == "universal" {
			return nil, c.Revision, errors.New("Universal power band is the default; edit it instead of deleting it")
		}
		result = items[idx]
		items = append(items[:idx], items[idx+1:]...)
	default:
		return nil, c.Revision, fmt.Errorf("unknown resource action %q", verb)
	}
	owner[key] = items
	b, _ := json.Marshal(root)
	draft, err := conductor.DecodeConfig(b)
	if err != nil {
		return nil, c.Revision, err
	}
	if dryRun {
		if draft, err = conductor.ValidateDraft(draft); err != nil {
			return nil, c.Revision, err
		}
		return map[string]any{"dry_run": true, "definition": result, "config": draft}, c.Revision, nil
	}
	saved, err := s.Apply(*expected, draft)
	if err != nil {
		return nil, c.Revision, err
	}
	return result, saved.Revision, nil
}

func remapSubscriptionIDs(sub map[string]any) {
	ids := map[string]string{}
	for _, key := range []string{"presets", "sequences", "overdrive_windows"} {
		items, _ := sub[key].([]any)
		for _, v := range items {
			m := v.(map[string]any)
			old, _ := m["id"].(string)
			fresh := conductor.NewID(strings.TrimSuffix(key, "s"))
			ids[old] = fresh
			m["id"] = fresh
		}
	}
	items, _ := sub["sequences"].([]any)
	for _, v := range items {
		m := v.(map[string]any)
		old, _ := m["preset_id"].(string)
		if n := ids[old]; n != "" {
			m["preset_id"] = n
		}
	}
}
