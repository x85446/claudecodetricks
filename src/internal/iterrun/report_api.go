package iterrun

import (
	"errors"
	"net/http"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

func registerReportAPI(mux *http.ServeMux) {
	mux.Handle("/api/report", ReportAPI())
}

// ReportAPI serves the plan report model read-only:
//
//	GET /api/report?project=<path>&plan=<name>[&history=1]
//	GET /api/report?project=<path>&archive=<file>[&history=1]
//
// history=1 adds the hook log and transcript usage. No route writes,
// acquires ownership, launches work or alters enrollment.
func ReportAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			recordAPIError(w, http.StatusMethodNotAllowed, "the report API is read-only; use GET")
			return
		}
		q := r.URL.Query()
		project, plan, archive := q.Get("project"), q.Get("plan"), q.Get("archive")
		if project == "" || (plan == "" && archive == "") {
			recordAPIError(w, http.StatusBadRequest, "project and plan (or archive) query parameters are required")
			return
		}
		in, err := LoadReportInput(r.Context(), project, plan, archive, q.Get("history") == "1", time.Now())
		if errors.Is(err, errReportNotFound) {
			recordAPIError(w, http.StatusNotFound, err.Error())
			return
		}
		if err != nil {
			recordAPIError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		recordAPIReply(w, map[string]any{
			"schema_version": runrecord.SchemaVersion,
			"project":        in.Record.ProjectPath,
			"plan":           in.Plan.Name,
			"archive_file":   in.Plan.ArchiveFile,
			"source":         in.Record.Source,
			"history":        in.HistoryLoaded,
			"report":         BuildPlanReport(in),
		})
	})
}
