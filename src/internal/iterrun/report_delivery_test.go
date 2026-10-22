package iterrun

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

const (
	revA  = "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"
	revB  = "b2c3d4e5f60718293a4b5c6d7e8f901234567890"
	revM  = "c3d4e5f60718293a4b5c6d7e8f90123456789012"
	revM2 = "d4e5f60718293a4b5c6d7e8f9012345678901234"
)

func stage(t *testing.T, s DeliverySection, name string) DeliveryStage {
	t.Helper()
	for _, st := range s.Stages {
		if st.Stage == name {
			return st
		}
	}
	t.Fatalf("no stage %s", name)
	return DeliveryStage{}
}

// TESTMASTER: id=kr-delivery-stages tier=fast parallel=no
//
// Tested, merged, installed and runtime-verified evidence recorded at
// different revisions: every stage carries its own result, revision and
// time, and proof for an older revision is stale rather than silently
// counted.
func TestKrDeliveryStages(t *testing.T) {
	env := newAcctEnv(t, "delivery")

	d := BuildPlanReport(env.input("godwit", "", false)).Delivery
	if d.State != stateComplete || len(d.Outstanding) != 0 {
		t.Fatalf("godwit delivery %s, outstanding %+v", d.State, d.Outstanding)
	}
	want := map[string]string{"source_commit": revA, "ci": revA, "merge": revM, "install": revM, "runtime_verification": revM}
	for name, rev := range want {
		st := stage(t, d, name)
		if st.State != "passed" || st.Latest == nil || st.Latest.Revision != rev || st.Latest.At.IsZero() {
			t.Errorf("%s: %+v, want passed at %s with a time", name, st.Latest, rev)
		}
	}
	if st := stage(t, d, "install"); st.Latest.Target != "cypressMini" || st.Latest.ArtifactPath != "bin/iterate-run" {
		t.Errorf("install target/artifact %+v", st.Latest)
	}
	if st := stage(t, d, "deploy"); st.State != "not_recorded" || st.Latest != nil {
		t.Errorf("deploy %+v: no deployment was recorded", st)
	}
	if st := stage(t, d, "pr"); st.Latest.URL == "" || st.Latest.Ref != "PR 41" {
		t.Errorf("pr %+v", st.Latest)
	}

	s := BuildPlanReport(env.input("stilt", "", false)).Delivery
	if s.State != statePartial {
		t.Fatalf("stilt delivery %s, want partial", s.State)
	}
	ci := stage(t, s, "ci")
	if ci.State != "stale" || !strings.Contains(ci.Stale, shortRev(revA)) || !strings.Contains(ci.Stale, shortRev(revB)) {
		t.Errorf("CI tested %s, a later commit %s exists: %+v", revA[:12], revB[:12], ci)
	}
	rv := stage(t, s, "runtime_verification")
	if rv.State != "stale" || !strings.Contains(rv.Stale, shortRev(revM2)) || rv.Latest.Revision != revM {
		t.Errorf("runtime proof at %s predates the reinstall at %s: %+v", revM[:12], revM2[:12], rv)
	}
	if inst := stage(t, s, "install"); inst.State != "passed" || inst.Latest.Revision != revM2 || len(inst.Observed) != 2 {
		t.Errorf("install keeps both observations, latest %s: %+v", revM2[:12], inst)
	}
	dep := stage(t, s, "deploy")
	if dep.State != "pending" || len(dep.Intended) != 1 || len(dep.Observed) != 0 {
		t.Errorf("a pending deploy is intended, not observed proof: %+v", dep)
	}
	var gates []string
	for _, g := range s.Outstanding {
		gates = append(gates, g.Stage+":"+g.State)
	}
	if !reflect.DeepEqual(gates, []string{"ci:stale", "deploy:pending", "runtime_verification:stale"}) {
		t.Errorf("outstanding gates %v", gates)
	}

	html := sectionHTML(t, BuildPlanReport(env.input("stilt", "", false)), "report-delivery")
	for _, want := range []string{"Tested (CI)", "Merged", "Installed", "Runtime verified", "stale", shortRev(revM2), "Proof required", "Outstanding gates", "pending"} {
		if !strings.Contains(html, want) {
			t.Errorf("delivery HTML missing %q", want)
		}
	}
	got := env.api("godwit", "", false)
	if got.Report.Delivery.State != stateComplete || stage(t, got.Report.Delivery, "merge").Latest.Revision != revM {
		t.Errorf("API delivery %+v", got.Report.Delivery.State)
	}
}

// TESTMASTER: id=kr-delivery-unknown tier=fast parallel=no
//
// Green CI with no deployment evidence leaves installation, deployment and
// runtime proof unknown; plan-file prose about a merge stays an imported
// note; and reading the report through the API or the page changes no file
// in the record or the project and offers no action.
func TestKrDeliveryUnknown(t *testing.T) {
	env := newAcctEnv(t, "delivery")
	before := snapshot(t, env.records, env.project)

	rep := BuildPlanReport(env.input("curlew", "", true))
	d := rep.Delivery
	if stage(t, d, "ci").State != "passed" {
		t.Fatalf("ci %+v", stage(t, d, "ci"))
	}
	for _, name := range []string{"merge", "install", "deploy", "runtime_verification"} {
		if st := stage(t, d, name); st.State != "not_recorded" || st.Latest != nil {
			t.Errorf("%s %+v: green CI must not imply it", name, st)
		}
	}
	if d.State != statePartial || !strings.Contains(d.Summary, "Runtime verification is not recorded") {
		t.Errorf("delivery %s %q", d.State, d.Summary)
	}
	html := sectionHTML(t, rep, "report-delivery")
	if !strings.Contains(html, "No evidence recorded") || !strings.Contains(html, "Unknown, not failed") {
		t.Error("missing stages must say their evidence is unknown")
	}
	if strings.Contains(html, `href="javascript:`) || !strings.Contains(html, "javascript:alert(1)") {
		t.Error("a non-http link must render as text, never as a link")
	}
	for _, id := range []string{"report-delivery", "report-usage", "report-coverage"} {
		sec := sectionHTML(t, rep, id)
		for _, action := range []string{"<form", "<button", "method=", "onclick"} {
			if strings.Contains(sec, action) {
				t.Errorf("%s offers an action (%s); evidence display is read-only", id, action)
			}
		}
	}

	// Reads change nothing, and the API refuses anything but GET.
	_ = env.api("curlew", "", true)
	_ = env.api("godwit", "", false)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		q := url.Values{"project": {env.project}, "plan": {"curlew"}}
		ReportAPI().ServeHTTP(w, httptest.NewRequest(method, "/api/report?"+q.Encode(), nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/report answered %d", method, w.Code)
		}
	}
	after := snapshot(t, env.records, env.project)
	if !reflect.DeepEqual(before, after) {
		for k := range after {
			if before[k] != after[k] {
				t.Errorf("report read changed %s", k)
			}
		}
		for k := range before {
			if _, ok := after[k]; !ok {
				t.Errorf("report read removed %s", k)
			}
		}
	}

	// Legacy: the plan-file prose about PR28 is a note, not a merge.
	gem := newAcctEnv(t, "gemsbok")
	g := BuildPlanReport(gem.input("", "20261020T094038Z-gemsbok.md", false)).Delivery
	if g.State != stateUnavailable || stage(t, g, "merge").State != "not_recorded" {
		t.Errorf("legacy delivery %s merge %s", g.State, stage(t, g, "merge").State)
	}
	if len(g.Imported) != 1 || !strings.Contains(g.Imported[0].Text, "merged PR28") || !strings.HasPrefix(g.Imported[0].Provenance, "legacy:") {
		t.Errorf("imported notes %+v", g.Imported)
	}
}
