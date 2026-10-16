package iterrun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/conductor/provider"
)

type ConductorError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Remedy  string `json:"remedy"`
}
type ConductorEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	OK            bool            `json:"ok"`
	Revision      int64           `json:"revision"`
	Data          any             `json:"data,omitempty"`
	Error         *ConductorError `json:"error,omitempty"`
}

func ConductorResponse(data any, revision int64, err error) ConductorEnvelope {
	out := ConductorEnvelope{SchemaVersion: model.SchemaVersion, OK: err == nil, Revision: revision, Data: data}
	if err != nil {
		code := "validation"
		remedy := "Check the definition and retry."
		if errors.Is(err, conductor.ErrConflict) {
			code = "revision_conflict"
			remedy = "Reload configuration, reconcile your draft, and retry with its revision."
		}
		if errors.Is(err, ErrConductorNotFound) {
			code = "not_found"
		}
		out.Data = nil
		out.Error = &ConductorError{Code: code, Message: err.Error(), Remedy: remedy}
		if field, _, ok := strings.Cut(err.Error(), ": "); ok && (strings.ContainsAny(field, ".[") && !strings.Contains(field, " ") || field == "preset_id" || field == "adapter" || field == "result_transport") {
			out.Error.Field = field
		}
	}
	return out
}
func conductorReply(w http.ResponseWriter, data any, revision int64, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	status := http.StatusOK
	if err != nil {
		status = http.StatusBadRequest
		if errors.Is(err, conductor.ErrConflict) {
			status = http.StatusConflict
		}
		if errors.Is(err, ErrConductorNotFound) {
			status = http.StatusNotFound
		}
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ConductorResponse(data, revision, err))
}
func conductorDecode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, (8<<20)+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("one JSON document is required")
	}
	return nil
}
func conductorSameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") == ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return u.Scheme == scheme && u.Host == r.Host && u.User == nil && u.Path == ""
}
func registerConductorAPI(mux *http.ServeMux) {
	mux.Handle("/api/conductor/", ConductorAPI(NewConductorService("")))
}

// ConductorAPI binds transport validation to the shared service without owning worker lifetimes.
func ConductorAPI(s *conductor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !conductorLocalHost(r.Host) {
			conductorHTTPError(w, http.StatusForbidden, "localhost Host required")
			return
		}
		if r.Method == "POST" && strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]) != "application/json" {
			conductorHTTPError(w, http.StatusUnsupportedMediaType, "Content-Type application/json required")
			return
		}
		if r.Method != "GET" && r.Method != "POST" {
			w.Header().Set("Allow", "GET, POST")
			conductorHTTPError(w, 405, "method not allowed")
			return
		}
		if r.Method == "POST" && !conductorSameOrigin(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(403)
			_ = json.NewEncoder(w).Encode(ConductorResponse(nil, 0, errors.New("cross-origin writes are forbidden")))
			return
		}
		c, err := s.Config()
		if err != nil {
			conductorReply(w, nil, 0, err)
			return
		}
		revision := c.Revision
		path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/conductor/"), "/")
		var data any
		get := func() bool {
			if r.Method != "GET" {
				err = errors.New("GET required")
				return false
			}
			return true
		}
		post := func() bool {
			if r.Method != "POST" {
				err = errors.New("POST required")
				return false
			}
			return true
		}
		switch path {
		case "config":
			if r.Method == "GET" {
				data = c
			} else {
				var req struct {
					ExpectedRevision *int64       `json:"expected_revision"`
					Config           model.Config `json:"config"`
				}
				if err = conductorDecode(r, &req); err == nil {
					if req.ExpectedRevision == nil {
						err = errors.New("expected_revision required")
					} else {
						var saved model.Config
						saved, err = s.Apply(*req.ExpectedRevision, req.Config)
						data = saved
						if err == nil {
							revision = saved.Revision
						}
					}
				}
			}
		case "status":
			if get() {
				data, err = s.Status(r.Context())
			}
		case "visibility":
			if get() {
				data, err = conductorVisibility(r.Context(), s, r.URL.Query().Get("project"))
			}
		case "runtime":
			if get() {
				var rt conductor.Runtime
				rt, err = s.Runtime()
				data = map[string]any{"revision": rt.Revision, "usage": rt.Usage, "occurrences": rt.Occurrences, "notices": rt.Notices, "leases": publicRunsMap(rt.Leases)}
			}
		case "runs":
			if get() {
				var runs []conductor.Lease
				runs, err = s.Runs()
				filtered := []map[string]any{}
				for _, run := range runs {
					a := run.Assignment
					if id := r.URL.Query().Get("project_id"); id != "" && a.ProjectID != id {
						continue
					}
					if p := r.URL.Query().Get("project"); p != "" && a.ProjectPath != p {
						continue
					}
					if p := r.URL.Query().Get("plan"); p != "" && a.Plan != p {
						continue
					}
					filtered = append(filtered, publicRun(run))
				}
				data = filtered
			}
		case "events":
			if get() {
				run := r.URL.Query().Get("run_id")
				if run == "" {
					run = r.URL.Query().Get("run")
				}
				after, e := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
				if r.URL.Query().Get("after") != "" && e != nil {
					err = errors.New("after must be a nonnegative cursor")
					break
				}
				limit := 100
				if v := r.URL.Query().Get("limit"); v != "" {
					limit, e = strconv.Atoi(v)
					if e != nil || limit < 1 || limit > 500 {
						err = errors.New("limit must be between 1 and 500")
						break
					}
				}
				var page conductor.EventPage
				page, err = ConductorEvents(s, run, after, limit, r.URL.Query().Get("project_id"), r.URL.Query().Get("plan"))
				if page.Cursor == 0 {
					page.Cursor = after
				}
				data = page
			}
		case "templates":
			if get() {
				data = provider.Templates()
			}
		case "evaluate":
			if post() {
				var req conductor.EvaluateRequest
				if err = conductorDecode(r, &req); err == nil {
					data, err = s.Evaluate(r.Context(), req)
				}
			}
		case "budget-preview":
			if post() {
				var req conductor.BudgetPreviewRequest
				if err = conductorDecode(r, &req); err == nil {
					data, err = s.BudgetPreview(r.Context(), req)
				}
			}
		case "overdrive-conversion":
			if post() {
				var req struct {
					SubscriptionID string `json:"subscription_id"`
					WindowID       string `json:"window_id"`
				}
				if err = conductorDecode(r, &req); err == nil {
					data, err = s.PreviewOverdriveConversion(r.Context(), req.SubscriptionID, req.WindowID)
				}
			}
		case "overdrive-options":
			if r.Method == "GET" {
				data, err = s.OverdriveOptions(r.Context(), r.URL.Query().Get("automation_id"))
			} else {
				var req struct {
					Automation model.Automation `json:"automation"`
				}
				if err = conductorDecode(r, &req); err == nil {
					options, e := s.OverdriveOptionsFor(r.Context(), req.Automation)
					err = e
					if err == nil {
						outside, available, e := s.OutsideOverdriveTime(r.Context(), req.Automation)
						err = e
						data = map[string]any{"options": options, "outside_time": outside, "outside_available": available}
					}
				}
			}
		case "probe", "preview":
			if post() {
				var req struct {
					Subscription   model.Subscription `json:"subscription"`
					Sequence       model.Sequence     `json:"sequence"`
					SubscriptionID string             `json:"subscription_id"`
					SequenceID     string             `json:"sequence_id"`
				}
				if err = conductorDecode(r, &req); err != nil {
					break
				}
				if req.SubscriptionID != "" {
					found := false
					for _, v := range c.Subscriptions {
						if v.ID == req.SubscriptionID {
							req.Subscription = v
							found = true
						}
					}
					if !found {
						err = ErrConductorNotFound
						break
					}
				}
				if req.SequenceID != "" {
					found := false
					for _, v := range req.Subscription.Sequences {
						if v.ID == req.SequenceID {
							req.Sequence = v
							found = true
						}
					}
					if !found {
						err = ErrConductorNotFound
						break
					}
				}
				if path == "probe" {
					ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
					data = provider.Probe(ctx, req.Subscription)
					cancel()
				} else {
					var argv []string
					argv, err = provider.Compile(req.Subscription, req.Sequence, model.Assignment{})
					data = map[string]any{"argv": argv, "command": provider.CommandPreview(argv)}
				}
			}
		case "control", "tick", "usage":
			if path == "usage" && r.Method == "GET" {
				var rt conductor.Runtime
				rt, err = s.Runtime()
				data = map[string]any{"usage": rt.Usage}
				break
			}
			if !post() {
				break
			}
			var req struct {
				ExpectedRevision *int64 `json:"expected_revision"`
				Action           string `json:"action"`
				ProjectID        string `json:"project_id"`
				SubscriptionID   string `json:"subscription_id"`
				Bind             bool   `json:"bind"`
			}
			if err = conductorDecode(r, &req); err != nil {
				break
			}
			if req.ExpectedRevision == nil {
				err = errors.New("expected_revision required")
				break
			}
			if *req.ExpectedRevision != c.Revision {
				err = conductor.ErrConflict
				break
			}
			switch path {
			case "tick":
				var pid int
				pid, err = StartConductorWorker(s.Dir, req.ProjectID)
				data = map[string]any{"pid": pid, "state": "worker_started"}
			case "usage":
				data, err = RefreshConductorUsage(r.Context(), s, req.SubscriptionID, *req.ExpectedRevision, req.Bind)
			case "control":
				err = s.ControlExpected(r.Context(), req.Action, req.ProjectID, *req.ExpectedRevision)
				if err == nil && (req.Action == "start" || req.Action == "resume") {
					var pid int
					pid, err = StartConductorWorker(s.Dir, req.ProjectID)
					data = map[string]any{"pid": pid, "action": req.Action}
				} else {
					data = map[string]any{"action": req.Action}
				}
			}
			if next, e := s.Config(); e == nil {
				revision = next.Revision
			}
		default:
			parts := strings.Split(path, "/")
			noun := parts[0]
			if _, ok := conductorCollections[noun]; !ok {
				err = ErrConductorNotFound
				break
			}
			id := ""
			if len(parts) > 1 {
				id = parts[1]
			}
			if len(parts) > 2 {
				err = ErrConductorNotFound
				break
			}
			if r.Method == "GET" {
				verb := "list"
				if id != "" {
					verb = "show"
				}
				data, revision, err = ConductorResource(s, noun, verb, id, r.URL.Query().Get("subscription_id"), nil, nil, false)
			} else {
				var req struct {
					ExpectedRevision *int64          `json:"expected_revision"`
					Action           string          `json:"action"`
					SubscriptionID   string          `json:"subscription_id"`
					Definition       json.RawMessage `json:"definition"`
					DryRun           bool            `json:"dry_run"`
				}
				if err = conductorDecode(r, &req); err == nil {
					data, revision, err = ConductorResource(s, noun, req.Action, id, req.SubscriptionID, req.Definition, req.ExpectedRevision, req.DryRun)
				}
			}
		}
		conductorReply(w, data, revision, err)
	})
}
func publicRun(l conductor.Lease) map[string]any {
	a := l.Assignment
	return map[string]any{"run_id": a.RunID, "assignment_id": a.ID, "project_id": a.ProjectID, "project_path": a.ProjectPath, "plan": a.Plan, "step": a.Step, "kind": a.Kind, "subscription_id": a.Subscription.ID, "sequence_id": a.Sequence.ID, "sequence_name": model.SequenceName(a.Subscription, a.Sequence), "status": l.Status, "started": l.Started, "pid": l.PID, "interrupt": l.Interrupt, "occurrence_id": l.OccurrenceID, "assignment": map[string]any{"run_id": a.RunID, "id": a.ID, "project_id": a.ProjectID, "project_path": a.ProjectPath, "plan": a.Plan, "step": a.Step, "kind": a.Kind, "subscription": map[string]any{"id": a.Subscription.ID, "name": a.Subscription.Name}, "sequence": map[string]any{"id": a.Sequence.ID, "name": a.Sequence.Name}}}
}
func publicRunsMap(ls map[string]conductor.Lease) []map[string]any {
	out := []map[string]any{}
	for _, l := range ls {
		out = append(out, publicRun(l))
	}
	return out
}

func conductorLocalHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

func conductorHTTPError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ConductorResponse(nil, 0, errors.New(message)))
}

// ConductorRuns exposes stable public identities without prompt or launch internals.
func ConductorRuns(s *conductor.Service, project, plan string) ([]map[string]any, error) {
	runs, err := s.Runs()
	out := []map[string]any{}
	if err != nil {
		return out, err
	}
	for _, run := range runs {
		a := run.Assignment
		if project != "" && a.ProjectID != project && a.ProjectPath != project {
			continue
		}
		if plan != "" && a.Plan != plan {
			continue
		}
		out = append(out, publicRun(run))
	}
	return out, nil
}
func ConductorEvents(s *conductor.Service, id string, after uint64, limit int, project, plan string) (conductor.EventPage, error) {
	runs, err := ConductorRuns(s, project, plan)
	if err != nil {
		return conductor.EventPage{}, err
	}
	found := false
	for _, run := range runs {
		if run["run_id"] == id {
			found = true
		}
	}
	if !found {
		return conductor.EventPage{}, ErrConductorNotFound
	}
	page, err := s.Events(id, after, limit)
	if page.Cursor == 0 {
		page.Cursor = after
	}
	return page, err
}
