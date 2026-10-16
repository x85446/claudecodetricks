package iterrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// LegacyImportReport describes an explicit, revision-checked import. Source
// files remain authoritative for legacy schedule restrictions and run locks.
type LegacyImportReport struct {
	Revision  int64                 `json:"revision"`
	Applied   bool                  `json:"applied"`
	Projects  []LegacyProjectImport `json:"projects"`
	Decisions []string              `json:"decisions"`
}

type LegacyProjectImport struct {
	Path      string   `json:"path"`
	ProjectID string   `json:"project_id,omitempty"`
	Action    string   `json:"action"`
	Enabled   bool     `json:"enabled"`
	Paused    bool     `json:"paused"`
	Review    []string `json:"review,omitempty"`
}

func legacyID(path string) string {
	h := sha256.Sum256([]byte(path))
	return "project-" + hex.EncodeToString(h[:12])
}

// ImportLegacyConductor previews by default and imports only with apply=true.
// Existing projects, subscriptions and automations retain their configuration.
// Provider/account selection requires an explicit configured launch sequence.
func ImportLegacyConductor(s *conductor.Service, paths []string, apply bool) (LegacyImportReport, error) {
	cfg, err := s.Config()
	if err != nil {
		return LegacyImportReport{}, err
	}
	report := LegacyImportReport{Revision: cfg.Revision, Projects: []LegacyProjectImport{}, Decisions: []string{
		"Project identities use canonical folders; existing registrations remain unchanged.",
		"Legacy launch and conductor schedules remain intersecting admission gates.",
		"No provider credentials, allowance observations or overdrive windows are inferred from prose.",
		"Configure a bound subscription and automation before dispatch; import does not start a worker.",
	}}
	if len(paths) == 0 {
		paths, err = ListProjects()
		if err != nil {
			return report, err
		}
	}
	known := map[string]model.Project{}
	for _, p := range cfg.Projects {
		known[p.Path] = p
	}
	changed := false
	backups := map[string][]byte{}
	for _, path := range paths {
		canonical, e := conductor.Canonical(path)
		if e != nil {
			report.Projects = append(report.Projects, LegacyProjectImport{Path: path, Action: "unavailable", Review: []string{e.Error()}})
			continue
		}
		item := LegacyProjectImport{Path: canonical, ProjectID: legacyID(canonical), Action: "register disabled"}
		if old, ok := known[canonical]; ok {
			item.ProjectID = old.ID
			item.Action = "already registered"
			item.Enabled = old.Enabled
			item.Paused = old.Paused
			report.Projects = append(report.Projects, item)
			continue
		}
		cs, exists := ReadConductorState(canonical)
		p := model.Project{ID: item.ProjectID, Name: filepath.Base(canonical), Path: canonical}
		if exists {
			p.Enabled, p.Paused = cs.Enabled, cs.Paused
			item.Action = "import legacy project"
			if cs.Cron != "" {
				item.Review = append(item.Review, "External scheduler identity "+cs.Cron+" requires ownership reconciliation before dispatch.")
				p.Paused = true
			}
			if nightlyLockHeld(canonical) || nightlyLockHeld(path) {
				item.Review = append(item.Review, "An existing nightly worker owns this project; import waits for its checkpoint.")
				p.Paused = true
			}
			item.Review = append(item.Review, "Select an authenticated subscription and launch sequence; legacy AI prose does not define a verified account binding.")
		}
		for _, name := range []string{"conductor.md", "policy.md"} {
			path := filepath.Join(canonical, ".claude", "iterate", name)
			b, e := os.ReadFile(path)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return report, e
			}
			backups[filepath.Join(item.ProjectID, name)] = b
			if name == "conductor.md" {
				fm := legacyFrontmatter(string(b))
				for key, dst := range map[string]*int{"max-attempts": &p.MaxAttempts, "watch-bound": &p.WatchBound, "max-import": &p.MaxImport, "plan-cap-seconds": &p.PlanCapSeconds, "no-progress-seconds": &p.NoProgressSeconds} {
					if raw := fm[key]; raw != "" {
						n, e := strconv.Atoi(raw)
						if e != nil || n < 1 {
							item.Review = append(item.Review, "Review unsupported "+key+" value.")
							p.Paused = true
						} else {
							*dst = n
						}
					}
				}
				for key := range fm {
					if strings.Contains(key, "overdrive") || strings.Contains(key, "budget") || strings.Contains(key, "power-band") {
						item.Review = append(item.Review, "Review legacy "+key+"; use explicit named policy and percent-used window settings.")
						p.Paused = true
					}
				}
			}
		}
		if _, e := ReadLaunchSchedule(canonical); e != nil {
			item.Review = append(item.Review, e.Error())
			p.Paused = true
		}
		if _, e := ReadConductorSchedule(canonical); e != nil {
			item.Review = append(item.Review, e.Error())
			p.Paused = true
		}
		item.Enabled, item.Paused = p.Enabled, p.Paused
		cfg.Projects = append(cfg.Projects, p)
		known[canonical] = p
		changed = true
		report.Projects = append(report.Projects, item)
	}
	// The global nightly opt-out also gates the binary configuration at import.
	// An existing conductor configuration retains its own enabled state.
	if cfg.Revision == 0 && !LoadNightlyConfig().Enabled {
		cfg.Enabled = false
		report.Decisions = append(report.Decisions, "Global nightly opt-out is preserved.")
	}
	if !apply || !changed {
		return report, nil
	}
	backupDir := filepath.Join(s.Dir, "migration-backups")
	for name, b := range backups {
		h := sha256.Sum256(b)
		path := filepath.Join(backupDir, hex.EncodeToString(h[:12]), name)
		if e := conductor.AtomicWrite(path, b); e != nil {
			return report, fmt.Errorf("backup legacy source: %w", e)
		}
	}
	if b, e := os.ReadFile(filepath.Join(s.Dir, "config.json")); e == nil {
		h := sha256.Sum256(b)
		if e = conductor.AtomicWrite(filepath.Join(backupDir, hex.EncodeToString(h[:12]), "config.json"), b); e != nil {
			return report, e
		}
	} else if !os.IsNotExist(e) {
		return report, e
	}
	next, e := s.Apply(report.Revision, cfg)
	if e != nil {
		return report, e
	}
	report.Applied = true
	report.Revision = next.Revision
	data, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		return report, e
	}
	if e = conductor.AtomicWrite(filepath.Join(s.Dir, "legacy-import.json"), append(data, '\n')); e != nil {
		return report, e
	}
	return report, nil
}

func legacyFrontmatter(body string) map[string]string {
	out := map[string]string{}
	inside := false
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "---" {
			if inside {
				break
			}
			inside = true
			continue
		}
		if !inside {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if ok && !strings.HasPrefix(line, " ") {
			out[strings.TrimSpace(key)] = stripComment(strings.TrimSpace(value))
		}
	}
	return out
}
