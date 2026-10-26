package iterrun

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// acctEnv is one isolated accounting fixture: a temporary HOME holding the
// hook log, Claude Code transcripts and Codex rollouts, a temporary run
// record and Conductor store, and a registered project. Fixtures live under
// testdata/reports/accounting/<scenario>/ as sanitized, shaped copies of
// real executions; {{PROJECT}} in them becomes the project path.
type acctEnv struct {
	t       *testing.T
	home    string
	project string
	records string
}

func newAcctEnv(t *testing.T, scenario string) acctEnv {
	t.Helper()
	home := t.TempDir()
	records := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("ITERATE_RUN_RECORD_DIR", records)
	t.Setenv("ITERATE_LEGACY_RUN_DIR", t.TempDir())
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("testdata", "reports", "accounting", scenario)
	copyFixtureTree(t, filepath.Join(src, "home"), home, project)
	copyFixtureTree(t, filepath.Join(src, "project"), project, project)
	ResetTranscriptCache()
	t.Cleanup(ResetTranscriptCache)
	RegisterProject(project)
	env := acctEnv{t: t, home: home, project: project, records: records}
	if data, err := os.ReadFile(filepath.Join(src, "record.jsonl")); err == nil {
		env.appendRecord(data)
	}
	return env
}

// copyFixtureTree copies a fixture directory, substituting the project
// path.
func copyFixtureTree(t *testing.T, from, to, project string) {
	t.Helper()
	if _, err := os.Stat(from); err != nil {
		return
	}
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, bytes.ReplaceAll(data, []byte("{{PROJECT}}"), []byte(project)), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// appendRecord writes fixture events through the record store, the same
// path every writer uses.
func (e acctEnv) appendRecord(data []byte) {
	e.t.Helper()
	var events []runrecord.Event
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var ev runrecord.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			e.t.Fatalf("fixture event %s: %v", sc.Text(), err)
		}
		ev.ProjectPath = e.project
		events = append(events, ev)
	}
	if _, err := RecordStore().AppendAll(events); err != nil {
		e.t.Fatal(err)
	}
}

// input loads a report input through the same loader the page and API use.
func (e acctEnv) input(plan, archive string, history bool) ReportInput {
	e.t.Helper()
	in, err := LoadReportInput(e.t.Context(), e.project, plan, archive, history, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		e.t.Fatalf("LoadReportInput(%s, %s): %v", plan, archive, err)
	}
	return in
}

// acctAPIReport is the /api/report envelope.
type acctAPIReport struct {
	Source  string     `json:"source"`
	History bool       `json:"history"`
	Report  PlanReport `json:"report"`
}

// api reads GET /api/report for a plan or an archive.
func (e acctEnv) api(plan, archive string, history bool) acctAPIReport {
	e.t.Helper()
	q := url.Values{"project": {e.project}}
	if plan != "" {
		q.Set("plan", plan)
	}
	if archive != "" {
		q.Set("archive", archive)
	}
	if history {
		q.Set("history", "1")
	}
	w := httptest.NewRecorder()
	ReportAPI().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/report?"+q.Encode(), nil))
	if w.Code != http.StatusOK {
		e.t.Fatalf("GET /api/report?%s: %d %s", q.Encode(), w.Code, w.Body)
	}
	var out acctAPIReport
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	return out
}

// snapshot fingerprints every file under the given roots: path, size,
// modification time and content.
func snapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, _ := d.Info()
			data, _ := os.ReadFile(path)
			out[path] = info.ModTime().String() + "\x00" + string(data)
			return nil
		})
	}
	return out
}

// sectionHTML renders the report and returns one section's markup.
func sectionHTML(t *testing.T, rep PlanReport, id string) string {
	t.Helper()
	page := RenderPlanReport(rep)
	start := strings.Index(page, `<section class="report-section" id="`+id+`"`)
	if start < 0 {
		t.Fatalf("section %s missing from the report", id)
	}
	end := strings.Index(page[start:], "</section>")
	if end < 0 {
		t.Fatalf("section %s is not closed", id)
	}
	return page[start : start+end+len("</section>")]
}

func lane(t *testing.T, lanes []CoverageLane, label string) CoverageLane {
	t.Helper()
	for _, l := range lanes {
		if l.Label == label {
			return l
		}
	}
	t.Fatalf("no lane %q in %+v", label, lanes)
	return CoverageLane{}
}

func count(t *testing.T, s CoverageSection, kind string) CoverageCount {
	t.Helper()
	for _, c := range s.Counts {
		if c.Kind == kind {
			return c
		}
	}
	t.Fatalf("no %s count", kind)
	return CoverageCount{}
}

func val(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}
