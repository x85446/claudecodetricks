package iterrun

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

func conductorRequest(t *testing.T, h http.Handler, method, path, origin, host string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://localhost:8420/api/conductor/"+path, bytes.NewReader(b))
	r.Host = host
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// TESTMASTER: id=indigobird-api-revision-security tier=? parallel=yes
func TestConductorAPIRevisionSecurity(t *testing.T) {
	s := NewConductorService(t.TempDir())
	h := ConductorAPI(s)
	c, e := s.Config()
	if e != nil {
		t.Fatal(e)
	}
	body := map[string]any{"expected_revision": c.Revision, "config": c}
	for _, tc := range []struct {
		origin, host string
		want         int
	}{{"http://evil.test", "localhost:8420", 403}, {"http://localhost:8420", "evil.test:8420", 403}, {"http://localhost:8420", "localhost:8420", 200}} {
		w := conductorRequest(t, h, "POST", "config", tc.origin, tc.host, body)
		if w.Code != tc.want {
			t.Fatalf("origin %s host %s status %d: %s", tc.origin, tc.host, w.Code, w.Body)
		}
	}
	w := conductorRequest(t, h, "POST", "config", "http://localhost:8420", "localhost:8420", body)
	if w.Code != 409 {
		t.Fatalf("stale revision wrote: %d %s", w.Code, w.Body)
	}
	w = conductorRequest(t, h, "POST", "config", "http://localhost:8420", "localhost:8420", map[string]any{"config": c})
	if w.Code != 400 {
		t.Fatal("missing revision accepted")
	}
	r := httptest.NewRequest("POST", "http://localhost:8420/api/conductor/config", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Content-Type", "text/plain")
	out := httptest.NewRecorder()
	h.ServeHTTP(out, r)
	if out.Code != 415 {
		t.Fatal("non JSON write accepted")
	}
	saved, _ := s.Config()
	if saved.Revision != 1 {
		t.Fatalf("security failures mutated revision %d", saved.Revision)
	}
}

// TESTMASTER: id=indigobird-resource-atomic-onboarding tier=? parallel=yes
func TestConductorResourceAtomicOnboarding(t *testing.T) {
	s := NewConductorService(t.TempDir())
	dir := t.TempDir()
	rev := int64(0)
	b, _ := json.Marshal(map[string]any{"id": "p", "path": dir, "enabled": true})
	_, revision, e := ConductorResource(s, "project", "create", "", "", b, &rev, false)
	if e != nil {
		t.Fatal(e)
	}
	if revision != 1 {
		t.Fatal(revision)
	}
	c, _ := s.Config()
	if c.Projects[0].Enabled {
		t.Fatal("registration armed live work")
	}
	canonical, _ := conductor.Canonical(dir)
	if c.Projects[0].Path != canonical {
		t.Fatal(c.Projects[0].Path)
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatal("registration created project files")
	}
	rev = revision
	_, _, e = ConductorResource(s, "project", "create", "", "", b, &rev, false)
	if e == nil {
		t.Fatal("duplicate path allowed")
	}
	_, _, e = ConductorResource(s, "power-band", "delete", "universal", "", nil, &rev, false)
	if e == nil {
		t.Fatal("deleted universal")
	}
	after, _ := s.Config()
	if after.Revision != revision || len(after.Projects) != 1 {
		t.Fatal("rejected writes changed config")
	}
}

// TESTMASTER: id=indigobird-resource-reference-drafts tier=? parallel=yes
func TestConductorResourceReferenceDrafts(t *testing.T) {
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	sub := c.Subscriptions[0]
	if len(sub.Sequences) == 0 {
		t.Fatal("template sequence absent")
	}
	q := sub.Sequences[0]
	c.Projects = []model.Project{{ID: "project-test", Name: "test", Path: t.TempDir()}}
	c.Automations = []model.Automation{{ID: "rule-test", Name: "test", ProjectID: "project-test", Mode: "keep_working", Entries: []model.Entry{{ID: "row-test", SequenceID: q.ID, MaxBand: model.Green}}}}
	c, e := s.Apply(0, c)
	if e != nil {
		t.Fatal(e)
	}
	rev := c.Revision
	_, _, e = ConductorResource(s, "sequence", "delete", q.ID, sub.ID, nil, &rev, false)
	if e == nil {
		t.Fatal("deleted referenced sequence")
	}
	_, _, e = ConductorResource(s, "sequence", "edit", q.ID, sub.ID, []byte(`{"name":"draft"}`), &rev, true)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := s.Config()
	if after.Revision != rev || after.Subscriptions[0].Sequences[0].Name != q.Name {
		t.Fatal("dry run mutated config")
	}
	_, _, e = ConductorResource(s, "sequence", "edit", q.ID, sub.ID, []byte(`{"unknown":true}`), &rev, false)
	if e == nil {
		t.Fatal("unknown fields silently dropped")
	}
}

// TESTMASTER: id=indigobird-api-simulation-pure tier=? parallel=yes
func TestConductorAPISimulationPure(t *testing.T) {
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	c.Projects = []model.Project{{ID: "project", Name: "empty", Path: t.TempDir()}}
	c, e := s.Apply(0, c)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	h := ConductorAPI(s)
	for _, path := range []string{"status", "usage", "runtime", "runs", "templates"} {
		w := conductorRequest(t, h, "GET", path, "", "localhost:8420", nil)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	w := conductorRequest(t, h, "POST", "evaluate", "http://localhost:8420", "localhost:8420", map[string]any{"project_id": "project", "queue_available": true})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("simulation saved config")
	}
	rt, e := s.Runtime()
	if e != nil || rt.Revision != 0 || len(rt.Leases) != 0 {
		t.Fatal("simulation acquired runtime ownership", e)
	}
}

// TESTMASTER: id=indigobird-api-private-run-projection tier=? parallel=yes
func TestConductorAPIPrivateRunProjection(t *testing.T) {
	l := conductor.Lease{Assignment: model.Assignment{RunID: "r", Prompt: "private prompt", ResultPath: "private path", Subscription: model.Subscription{ID: "s", Name: "account", Args: []string{"secret"}}, Sequence: model.Sequence{ID: "q", Name: "build"}}}
	b, _ := json.Marshal(publicRun(l))
	for _, private := range []string{"private prompt", "private path", "secret"} {
		if bytes.Contains(b, []byte(private)) {
			t.Fatal("private launch fields exposed")
		}
	}
	if !bytes.Contains(b, []byte(`"sequence_name":"account · build"`)) {
		t.Fatal(string(b))
	}
}
