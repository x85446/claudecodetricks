package iterrun

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

func registerRecordAPI(mux *http.ServeMux) {
	mux.Handle("/api/record", RecordAPI())
	mux.Handle("/api/record/", RecordAPI())
}

// RecordAPI serves the run record read-only:
//
//	GET /api/record?project=<path>&plan=<name>[&archive=<file>]  one plan's report
//	GET /api/record?run=<run-id>                                 one run
//	GET /api/record/runs[?project=<path>][&plan=<name>]          run listing
//
// No route writes, acquires ownership or launches work.
func RecordAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			recordAPIError(w, http.StatusMethodNotAllowed, "the run record API is read-only; use GET")
			return
		}
		q := r.URL.Query()
		switch r.URL.Path {
		case "/api/record/runs":
			runs, err := ListRecordRuns(q.Get("project"), q.Get("plan"))
			if err != nil {
				recordAPIError(w, http.StatusBadRequest, err.Error())
				return
			}
			recordAPIReply(w, map[string]any{"schema_version": runrecord.SchemaVersion, "runs": runs})
		case "/api/record", "/api/record/":
			if id := q.Get("run"); id != "" {
				run, err := FindRecordRun(id)
				if errors.Is(err, runrecord.ErrNotFound) {
					recordAPIError(w, http.StatusNotFound, "no recorded run "+id)
					return
				}
				if err != nil {
					recordAPIError(w, http.StatusBadRequest, err.Error())
					return
				}
				recordAPIReply(w, run)
				return
			}
			project, plan, archive := q.Get("project"), q.Get("plan"), q.Get("archive")
			if project == "" || (plan == "" && archive == "") {
				recordAPIError(w, http.StatusBadRequest, "project and plan (or archive) query parameters are required")
				return
			}
			rep, err := ReadRecordReport(project, plan, archive)
			if errors.Is(err, runrecord.ErrNotFound) {
				recordAPIError(w, http.StatusNotFound, "no record or plan source for "+plan+" in "+project)
				return
			}
			if err != nil {
				recordAPIError(w, http.StatusBadRequest, err.Error())
				return
			}
			recordAPIReply(w, rep)
		default:
			recordAPIError(w, http.StatusNotFound, "unknown run record route")
		}
	})
}

func recordAPIReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func recordAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
