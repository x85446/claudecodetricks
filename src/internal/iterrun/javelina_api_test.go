package iterrun

import (
	"encoding/json"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"strings"
	"testing"
)

// TESTMASTER: id=javelina-policy-preview-api tier=? parallel=yes
func TestJavelinaPreviewAPI(t *testing.T) {
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	c.Subscriptions = nil
	c, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	h := ConductorAPI(s)
	for _, day := range []float64{0, .0001, 7} {
		w := conductorRequest(t, h, "POST", "policy-preview", "http://localhost:8420", "localhost:8420", budget.PolicySample{Policy: c.PowerBands[0], Day: day, Hour: 2.55, ShortUsed: 51})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		for _, key := range []string{"allowed", "subscription", "overdrive", "eligib"} {
			if strings.Contains(w.Body.String(), key) {
				t.Fatal("impure preview", w.Body)
			}
		}
	}
	saved, _ := s.Config()
	if saved.Revision != c.Revision {
		t.Fatal("preview saved config")
	}
	c.Projects = []model.Project{{ID: "p", Name: "Sample", Path: t.TempDir()}}
	c.Automations = []model.Automation{{ID: "a", Name: "Shared", Mode: "keep_working"}}
	c.Attachments = []model.AutomationAttachment{{ID: "l", DefinitionID: "a", ProjectID: "p"}}
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	c.Automations = nil
	w := conductorRequest(t, h, "POST", "config", "http://localhost:8420", "localhost:8420", map[string]any{"expected_revision": c.Revision, "config": c})
	if w.Code != 400 {
		t.Fatal("referenced delete", w.Code, w.Body)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !strings.Contains(w.Body.String(), "detach") {
		t.Fatal("actionable error missing", out)
	}
}
