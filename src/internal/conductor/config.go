package conductor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/conductor/provider"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func Defaults() model.Config {
	return model.Config{Version: model.SchemaVersion, PowerBands: []model.PowerBand{budget.Universal()}, Subscriptions: provider.Templates(), Projects: []model.Project{}, Automations: []model.Automation{}, TimeWindows: []model.TimeWindow{}}
}
func (s *Service) config() (model.Config, error) {
	var c model.Config
	b, e := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if os.IsNotExist(e) {
		c = Defaults()
		normalize(&c)
		return c, nil
	}
	if e != nil {
		return c, e
	}
	return migrateJSON(b)
}
func (s *Service) Config() (c model.Config, e error) {
	e = s.withLock(func() error { var x error; c, x = s.config(); return x })
	return
}
func normalize(c *model.Config) {
	c.Version = model.SchemaVersion
	if len(c.PowerBands) == 0 {
		c.PowerBands = []model.PowerBand{budget.Universal()}
	}
	fallback := "universal"
	for _, p := range c.PowerBands {
		if p.ID == "universal" {
			fallback = p.ID
		}
	}
	for i := range c.Subscriptions {
		s := &c.Subscriptions[i]
		if s.PowerBandID == "" {
			s.PowerBandID = fallback
		}
		if s.Concurrency == 0 {
			s.Concurrency = 1
		}
		if s.FreshnessSeconds == 0 {
			s.FreshnessSeconds = 300
		}
	}
	for i := range c.Projects {
		p := &c.Projects[i]
		if p.MaxAttempts == 0 {
			p.MaxAttempts = 5
		}
		if p.NoProgressSeconds == 0 {
			p.NoProgressSeconds = 2700
		}
		if p.WatchBound == 0 {
			p.WatchBound = 10
		}
		if p.MaxImport == 0 {
			p.MaxImport = 10
		}
	}
	for i := range c.Automations {
		a := &c.Automations[i]
		if a.RunLimit == "" {
			a.RunLimit = "drain_queue"
		}
		if a.OnWindowEnd == "" {
			a.OnWindowEnd = "finish_checkpoint"
		}
	}
}
func migrateJSON(b []byte) (model.Config, error) {
	var root map[string]json.RawMessage
	var c model.Config
	if e := json.Unmarshal(b, &root); e != nil {
		return c, e
	}
	if e := json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	if c.Version > model.SchemaVersion {
		return c, errors.New("configuration schema is newer than this binary")
	}
	if c.Version < 2 {
		var subs []map[string]json.RawMessage
		_ = json.Unmarshal(root["subscriptions"], &subs)
		for i, m := range subs {
			if i >= len(c.Subscriptions) {
				break
			}
			s := &c.Subscriptions[i]
			var ws []map[string]json.RawMessage
			_ = json.Unmarshal(m["overdrive_windows"], &ws)
			for j, w := range ws {
				if _, has := w["target_used_percent"]; !has {
					var rem float64
					if x, ok := w["target_remaining_percent"]; ok {
						if e := json.Unmarshal(x, &rem); e != nil {
							return c, e
						}
						s.OverdriveWindows[j].TargetUsedPercent = 100 - rem
					}
				}
			}
			for j := range s.Sequences {
				q := &s.Sequences[j]
				for _, sep := range []string{" · ", " - ", ": "} {
					q.Name = strings.TrimPrefix(q.Name, s.Name+sep)
				}
			}
			if raw, ok := m["power_band"]; ok {
				var p model.PowerBand
				if e := json.Unmarshal(raw, &p); e != nil {
					return c, e
				}
				if p.ID == "" {
					p.ID = "migrated-" + s.ID
				}
				if p.Name == "" {
					p.Name = s.Name + " power band"
				}
				found := false
				for _, old := range c.PowerBands {
					if old.ID == p.ID {
						if !reflect.DeepEqual(old, p) {
							return c, fmt.Errorf("conflicting legacy policy %s", p.ID)
						}
						found = true
					}
				}
				if !found {
					c.PowerBands = append(c.PowerBands, p)
				}
				s.PowerBandID = p.ID
			}
		}
	}
	normalize(&c)
	return c, nil
}

// Migrate preserves the source document and writes a validated current-schema snapshot once.
func (s *Service) Migrate() (c model.Config, e error) {
	e = s.withLock(func() error {
		b, x := os.ReadFile(filepath.Join(s.Dir, "config.json"))
		if os.IsNotExist(x) {
			c = Defaults()
			normalize(&c)
			return atomicJSON(filepath.Join(s.Dir, "config.json"), c)
		}
		if x != nil {
			return x
		}
		c, x = migrateJSON(b)
		if x != nil {
			return x
		}
		if x = ValidateConfig(c); x != nil {
			return x
		}
		var old model.Config
		_ = json.Unmarshal(b, &old)
		if old.Version == model.SchemaVersion {
			return nil
		}
		if x = AtomicWrite(filepath.Join(s.Dir, "config.legacy.backup.json"), b); x != nil {
			return x
		}
		c.Revision++
		return atomicJSON(filepath.Join(s.Dir, "config.json"), c)
	})
	return
}
func (s *Service) Apply(expected int64, draft model.Config) (c model.Config, e error) {
	e = s.withLock(func() error {
		old, x := s.config()
		if x != nil {
			return x
		}
		if old.Revision != expected {
			return ErrConflict
		}
		if len(draft.PowerBands) == 0 {
			return errors.New("Universal power band is required")
		}
		if draft.Version > model.SchemaVersion {
			return errors.New("configuration schema is newer than this binary")
		}
		normalize(&draft)
		for i := range draft.Projects {
			p := &draft.Projects[i]
			p.Path, x = Canonical(p.Path)
			if x != nil {
				return fmt.Errorf("projects.%s.path: %w", p.ID, x)
			}
		}
		revisions(&draft, old)
		if x = ValidateConfig(draft); x != nil {
			return x
		}
		draft.Revision = old.Revision + 1
		c = draft
		return atomicJSON(filepath.Join(s.Dir, "config.json"), draft)
	})
	return
}
func revisions(c *model.Config, old model.Config) {
	for i := range c.TimeWindows {
		w := &c.TimeWindows[i]
		w.Revision = 1
		for _, o := range old.TimeWindows {
			if o.ID == w.ID {
				w.Revision = o.Revision
				a, b := *w, o
				a.Name = ""
				b.Name = ""
				a.Enabled = false
				b.Enabled = false
				a.Revision = 0
				b.Revision = 0
				if !reflect.DeepEqual(a, b) {
					w.Revision++
				}
			}
		}
	}
	for i := range c.Subscriptions {
		for j := range c.Subscriptions[i].OverdriveWindows {
			w := &c.Subscriptions[i].OverdriveWindows[j]
			w.Revision = 1
			for _, s := range old.Subscriptions {
				if s.ID != c.Subscriptions[i].ID {
					continue
				}
				for _, o := range s.OverdriveWindows {
					if o.ID == w.ID {
						w.Revision = o.Revision
						a, b := *w, o
						a.Name = ""
						b.Name = ""
						a.Enabled = false
						b.Enabled = false
						a.TargetUsedPercent = 0
						b.TargetUsedPercent = 0
						a.Revision = 0
						b.Revision = 0
						if !reflect.DeepEqual(a, b) {
							w.Revision++
						}
					}
				}
			}
		}
	}
}
func ValidateConfig(c model.Config) error {
	ids := map[string]bool{}
	add := func(id string) error {
		if id == "" || strings.ContainsAny(id, "/\\\x00") {
			return errors.New("nonempty safe stable ID required")
		}
		if ids[id] {
			return fmt.Errorf("duplicate identity %s", id)
		}
		ids[id] = true
		return nil
	}
	pol := map[string]bool{}
	win := map[string]bool{}
	projects := map[string]bool{}
	seq := map[string]bool{}
	paths := map[string]bool{}
	for _, p := range c.PowerBands {
		if e := add(p.ID); e != nil {
			return e
		}
		if e := budget.ValidatePolicy(p); e != nil {
			return fmt.Errorf("power_bands[%s]: %w", p.ID, e)
		}
		if strings.TrimSpace(p.Name) == "" {
			return errors.New("power band name required")
		}
		pol[p.ID] = true
	}
	for _, w := range c.TimeWindows {
		if e := add(w.ID); e != nil {
			return e
		}
		if w.StartMode != "reset_relative" {
			if e := budget.ValidateRecurrence(w.Recurrence); e != nil {
				return fmt.Errorf("time_windows[%s].recurrence: %w", w.ID, e)
			}
		} else if w.HoursBeforeReset <= 0 {
			return fmt.Errorf("time_windows[%s].hours_before_reset: must be positive", w.ID)
		}
		if strings.TrimSpace(w.Name) == "" {
			return errors.New("time window name required")
		}
		win[w.ID] = true
	}
	for _, p := range c.Projects {
		if e := add(p.ID); e != nil {
			return e
		}
		if p.Name == "" || !filepath.IsAbs(p.Path) {
			return errors.New("project name and absolute path required")
		}
		if paths[p.Path] {
			return errors.New("project path already registered")
		}
		paths[p.Path] = true
		if p.WindowID != "" && !win[p.WindowID] {
			return errors.New("referenced project time window missing")
		}
		if p.MaxAttempts < 1 || p.NoProgressSeconds < 1 {
			return errors.New("invalid retry/no-progress limits")
		}
		projects[p.ID] = true
	}
	for _, s := range c.Subscriptions {
		if e := add(s.ID); e != nil {
			return e
		}
		if s.Name == "" || (s.Enabled && s.Executable == "") {
			return errors.New("subscription name and executable required")
		}
		if !pol[s.PowerBandID] {
			return errors.New("referenced power band missing")
		}
		if s.Concurrency < 1 || s.FreshnessSeconds < 1 {
			return errors.New("invalid subscription limits")
		}
		presets := map[string]bool{}
		for _, p := range s.Presets {
			if e := add(p.ID); e != nil {
				return e
			}
			if strings.TrimSpace(p.Name) == "" {
				return errors.New("preset name required")
			}
			presets[p.ID] = true
		}
		for _, q := range s.Sequences {
			if e := add(q.ID); e != nil {
				return e
			}
			if strings.TrimSpace(q.Name) == "" {
				return errors.New("sequence suffix required")
			}
			if q.PresetID != "" && !presets[q.PresetID] {
				return errors.New("referenced permission preset missing")
			}
			if _, e := provider.Compile(s, q, model.Assignment{}); e != nil && (s.Enabled || s.Executable != "") {
				return fmt.Errorf("sequence %s: %w", q.ID, e)
			}
			seq[q.ID] = true
		}
		for _, w := range s.OverdriveWindows {
			if e := add(w.ID); e != nil {
				return e
			}
			if strings.TrimSpace(w.Name) == "" {
				return errors.New("overdrive window name required")
			}
			if e := budget.ValidateOverdrive(w); e != nil {
				return fmt.Errorf("subscriptions[%s].overdrive_windows[%s]: %w", s.ID, w.ID, e)
			}
		}
	}
	if !pol["universal"] {
		return errors.New("Universal power band is required")
	}
	for _, a := range c.Automations {
		if e := add(a.ID); e != nil {
			return e
		}
		if !projects[a.ProjectID] {
			return errors.New("referenced project missing")
		}
		if a.OnlyOverdrive && !a.HonorOverdrive {
			return errors.New("only during overdrive requires honor overdrive")
		}
		if a.Mode != "keep_working" && a.Mode != "scheduled" && a.Mode != "overdrive" {
			return errors.New("invalid automation mode")
		}
		if a.Mode == "overdrive" && (!a.OnlyOverdrive || !a.HonorOverdrive || a.WindowID != "") {
			return errors.New("overdrive mode requires honor and only, without an automation window")
		}
		if a.Mode == "scheduled" && a.WindowID == "" {
			return errors.New("scheduled automation requires a time window")
		}
		if a.WindowID != "" && !win[a.WindowID] {
			return errors.New("referenced time window missing")
		}
		if a.RunLimit != "drain_queue" && a.RunLimit != "one_plan" {
			return errors.New("invalid run limit")
		}
		if a.OnWindowEnd != "finish_checkpoint" && a.OnWindowEnd != "finish_plan" && a.OnWindowEnd != "interrupt" {
			return errors.New("invalid window end action")
		}
		for _, en := range a.Entries {
			if e := add(en.ID); e != nil {
				return e
			}
			if !seq[en.SequenceID] {
				return errors.New("referenced sequence missing")
			}
			if en.MaxBand != model.Green && en.MaxBand != model.Yellow && en.MaxBand != model.Red {
				return errors.New("invalid maximum band")
			}
		}
		if a.Enabled && ((a.Mode == "overdrive" && len(a.Participants) == 0) || (a.Mode != "overdrive" && len(a.Entries) == 0)) {
			return errors.New("enabled automation requires launch sequences")
		}
		participants := map[string]bool{}
		for _, id := range a.Participants {
			if participants[id] {
				return errors.New("duplicate participating sequence")
			}
			participants[id] = true
			if !seq[id] {
				return errors.New("referenced participant missing")
			}
		}
	}
	return nil
}
func DecodeConfig(b []byte) (model.Config, error) {
	if len(b) > 8<<20 {
		return model.Config{}, errors.New("configuration exceeds limit")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var c model.Config
	e := d.Decode(&c)
	if e == nil {
		var extra any
		if x := d.Decode(&extra); x != io.EOF {
			e = errors.New("configuration must contain exactly one JSON value")
		}
	}
	return c, e
}

// ValidateDraft applies the same defaults and path validation as a saved transaction.
func ValidateDraft(draft model.Config) (model.Config, error) {
	if draft.Version > model.SchemaVersion {
		return draft, errors.New("configuration schema is newer than this binary")
	}
	normalize(&draft)
	for i := range draft.Projects {
		path, e := Canonical(draft.Projects[i].Path)
		if e != nil {
			return draft, e
		}
		draft.Projects[i].Path = path
	}
	return draft, ValidateConfig(draft)
}
