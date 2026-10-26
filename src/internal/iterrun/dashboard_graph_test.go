package iterrun

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dashboardGraphPlan(name string, lines ...string) string {
	return "name: " + name + "\n" + strings.Join(lines, "\n") + "\n"
}

// TESTMASTER: id=dag-web-1 tier=fast parallel=no
func TestDashboardGraphAPIServesTheCLIDocument(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	RegisterProject(dir)
	os.MkdirAll(filepath.Join(dir, ".claude", "iterate", "plans"), 0700)
	plans := filepath.Join(dir, ".claude", "iterate", "plans")
	for name, body := range map[string]string{
		"alpha.md": dashboardGraphPlan("alpha", "phase: executing", "status: blocked-on-operator: token"),
		"bravo.md": dashboardGraphPlan("bravo", "phase: planned", "depends: alpha"),
	} {
		if e := os.WriteFile(filepath.Join(plans, name), []byte(body), 0o644); e != nil {
			t.Fatal(e)
		}
	}
	RegisterProject(dir)
	want, err := BuildGraphView(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(want)
	canonical, _ := filepath.EvalSymlinks(dir)
	sum := sha256.Sum256([]byte(canonical))
	mux := http.NewServeMux()
	registerDashboardWeb(mux)
	for _, project := range []string{dir, "discovered-" + hex.EncodeToString(sum[:12])} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/graph?project="+url.QueryEscape(project), nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", project, w.Code, w.Body)
		}
		var got GraphView
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		gotJSON, _ := json.Marshal(got)
		if !bytes.Equal(gotJSON, wantJSON) {
			t.Fatalf("%s:\n%s\nwant\n%s", project, gotJSON, wantJSON)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/graph?project=/etc", nil))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `unknown project \"/etc\"`) {
		t.Fatalf("unknown project: %d %s", w.Code, w.Body)
	}
}
