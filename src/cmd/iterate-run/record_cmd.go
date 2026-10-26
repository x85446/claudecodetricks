package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/iterrun"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

const recordHelp = `iterate-run record — the durable run record the CLI and Conductor share

Usage:
  iterate-run record list   [--project <dir>] [--plan <name>] [--json]
  iterate-run record show   [<run-id>] [--project <dir>] [--plan <name>] [--archive <file>] [--json]
  iterate-run record append --type <type> [--file <path|->] [--project <dir>] [--plan <name>] [--run <id>] [--json]
  iterate-run record import [--project <dir>] [--plan <name>] [--archive <file>] [--dry-run] [--json]
  iterate-run record types  [--json]

Examples:
  iterate-run record show                                   # this project's current plan
  iterate-run record show --plan finch --archive 20261001T120000Z-finch-done.md --json
  echo '{"lifecycle":{"state":"paused"}}' | iterate-run record append --type run.lifecycle
  iterate-run record import --plan finch --dry-run

Flags:
      --project <dir>   project directory (default: the project holding the working directory)
      --plan <name>     plan codename (default: the project's current plan)
      --archive <file>  one archived execution, by archive filename
      --run <id>        run id; append writes to it instead of the plan's open run
      --type <type>     event type for append (iterate-run record types lists them)
      --file <path|->   event JSON for append (default: stdin)
  -n, --dry-run         import: print what would be recorded, write nothing
      --json            one JSON value on stdout
  -h, --help            this help

Exit codes: 0 success (a duplicate append is success), 1 failure, 2 usage error, 3 not found.
Reads never write. Store: $ITERATE_RUN_RECORD_DIR, else ~/.claude/iterate-run/records.
See docs/run-record.md.
`

type recordFlags struct {
	values map[string]string
	bools  map[string]bool
	words  []string
}

var recordValued = map[string]bool{"project": true, "plan": true, "archive": true, "run": true, "type": true, "file": true}
var recordBoolean = map[string]bool{"json": true, "dry-run": true, "help": true}

// parseRecordFlags accepts every flag in every position; "--" ends flags.
func parseRecordFlags(args []string) (recordFlags, error) {
	f := recordFlags{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			f.words = append(f.words, args[i+1:]...)
			break
		}
		switch a {
		case "-h":
			a = "--help"
		case "-n":
			a = "--dry-run"
		}
		if !strings.HasPrefix(a, "--") {
			if strings.HasPrefix(a, "-") && a != "-" {
				return f, fmt.Errorf("unknown flag %s; run iterate-run record --help", a)
			}
			f.words = append(f.words, a)
			continue
		}
		k, v, has := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		switch {
		case recordBoolean[k]:
			if has {
				return f, fmt.Errorf("flag --%s takes no value", k)
			}
			f.bools[k] = true
		case recordValued[k]:
			if !has {
				if i+1 >= len(args) {
					return f, fmt.Errorf("flag --%s needs a value", k)
				}
				i++
				v = args[i]
			}
			f.values[k] = v
		default:
			return f, fmt.Errorf("unknown flag --%s; run iterate-run record --help", k)
		}
	}
	return f, nil
}

// recordInvocation finds "record" as the first word, after any record flags.
func recordInvocation(args []string) ([]string, bool) {
	for i := 0; i < len(args); i++ {
		v := args[i]
		if v == "--" {
			return nil, false
		}
		if v == "record" {
			out := append([]string{}, args[:i]...)
			return append(out, args[i+1:]...), true
		}
		if strings.HasPrefix(v, "--") {
			k, _, has := strings.Cut(strings.TrimPrefix(v, "--"), "=")
			if recordValued[k] && !has {
				i++
			}
			if !recordValued[k] && !recordBoolean[k] {
				return nil, false
			}
			continue
		}
		if v == "-h" || v == "-n" {
			continue
		}
		return nil, false
	}
	return nil, false
}

func recordCmd(args []string) {
	os.Exit(runRecordCLI(args, os.Stdin, os.Stdout, os.Stderr))
}

func recordFail(out, diag io.Writer, asJSON bool, code int, err error, next string) int {
	if asJSON {
		_ = json.NewEncoder(out).Encode(map[string]any{"error": err.Error(), "exit": code})
		return code
	}
	fmt.Fprintf(diag, "Error: %v\n", err)
	if next != "" {
		fmt.Fprintf(diag, "\n  %s\n", next)
	}
	return code
}

func recordJSON(out io.Writer, v any) {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// runRecordCLI executes `iterate-run record …` and returns the exit code.
func runRecordCLI(args []string, input io.Reader, out, diag io.Writer) int {
	f, err := parseRecordFlags(args)
	asJSON := f.bools["json"]
	if err != nil {
		return recordFail(out, diag, asJSON, 2, err, "Run: iterate-run record --help")
	}
	if f.bools["help"] || len(f.words) == 0 {
		fmt.Fprint(out, recordHelp)
		return 0
	}
	verb, operands := f.words[0], f.words[1:]
	project := f.values["project"]
	if project == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return recordFail(out, diag, asJSON, 1, err, "")
		}
		project = iterrun.ProjectRoot(cwd)
	} else if abs, err := filepath.Abs(project); err == nil {
		project = abs
	}
	plan := f.values["plan"]
	defaultPlan := func() string {
		if plan == "" {
			plan = iterrun.CurrentPlanName(project)
		}
		return plan
	}

	switch verb {
	case "types":
		if asJSON {
			recordJSON(out, map[string]any{"schema_version": runrecord.SchemaVersion, "types": runrecord.KnownTypes,
				"lifecycle_states": runrecord.LifecycleStates, "interval_causes": runrecord.IntervalCauses, "delivery_stages": runrecord.DeliveryStages})
			return 0
		}
		for _, t := range runrecord.KnownTypes {
			fmt.Fprintln(out, t)
		}
		return 0

	case "list":
		filterProject := f.values["project"]
		if filterProject != "" {
			filterProject = project
		}
		runs, err := iterrun.ListRecordRuns(filterProject, plan)
		if err != nil {
			return recordFail(out, diag, asJSON, 1, err, "")
		}
		if asJSON {
			recordJSON(out, map[string]any{"schema_version": runrecord.SchemaVersion, "runs": runs})
			return 0
		}
		for _, r := range runs {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%d events\n", r.RunID, r.State, r.Plan, r.ProjectPath, r.Provenance, r.Events)
		}
		return 0

	case "show":
		if len(operands) > 1 {
			return recordFail(out, diag, asJSON, 2, errors.New("show takes at most one run id"), "Run: iterate-run record show [<run-id>]")
		}
		if len(operands) == 1 {
			run, err := iterrun.FindRecordRun(operands[0])
			if errors.Is(err, runrecord.ErrNotFound) {
				return recordFail(out, diag, asJSON, 3, fmt.Errorf("no recorded run %s", operands[0]), "List runs: iterate-run record list")
			}
			if err != nil {
				return recordFail(out, diag, asJSON, 1, err, "")
			}
			if asJSON {
				recordJSON(out, run)
			} else {
				printRun(out, run)
			}
			return 0
		}
		if f.values["archive"] == "" && defaultPlan() == "" {
			return recordFail(out, diag, asJSON, 2, errors.New("no plan named and the project has no current plan"), "Name one: iterate-run record show --plan <name>")
		}
		rep, err := iterrun.ReadRecordReport(project, plan, f.values["archive"])
		if errors.Is(err, runrecord.ErrNotFound) {
			return recordFail(out, diag, asJSON, 3, fmt.Errorf("no record and no plan or archive file for %q in %s", plan, project), "List recorded runs: iterate-run record list --project "+project)
		}
		if err != nil {
			return recordFail(out, diag, asJSON, 1, err, "")
		}
		if asJSON {
			recordJSON(out, rep)
			return 0
		}
		fmt.Fprintf(out, "source %s\n", rep.Source)
		for _, r := range rep.Runs {
			printRun(out, r)
		}
		return 0

	case "append":
		if len(operands) > 0 {
			return recordFail(out, diag, asJSON, 2, fmt.Errorf("append takes no operands, got %q", operands[0]), "Pass the event as JSON on stdin or with --file")
		}
		if defaultPlan() == "" {
			return recordFail(out, diag, asJSON, 2, errors.New("no plan named and the project has no current plan"), "Name one: iterate-run record append --plan <name> --type <type>")
		}
		var src io.Reader = input
		if path := f.values["file"]; path != "" && path != "-" {
			fh, err := os.Open(path)
			if err != nil {
				return recordFail(out, diag, asJSON, 1, err, "")
			}
			defer fh.Close()
			src = fh
		}
		data, err := io.ReadAll(io.LimitReader(src, 1<<20))
		if err != nil {
			return recordFail(out, diag, asJSON, 1, err, "")
		}
		if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
			return appendRecordBatch(data, f, project, plan, out, diag)
		}
		var e runrecord.Event
		if len(strings.TrimSpace(string(data))) > 0 {
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&e); err != nil {
				return recordFail(out, diag, asJSON, 2, fmt.Errorf("event JSON is invalid: %w", err), "See the fields in docs/run-record.md")
			}
		}
		if t := f.values["type"]; t != "" {
			e.Type = t
		}
		if e.Type == "" {
			return recordFail(out, diag, asJSON, 2, errors.New("no event type given"), "Pass --type <type>; iterate-run record types lists them")
		}
		if err := runrecord.ValidatePayload(e); err != nil {
			return recordFail(out, diag, asJSON, 2, err, "See the event contract in docs/run-record.md")
		}
		e.ProjectPath, e.ProjectKey, e.Plan = project, "", plan
		if e.Source == "" {
			e.Source = runrecord.SourceCLI
		}
		if e.ID == "" {
			e.ID = runrecord.NewEventID()
		}
		store := iterrun.RecordStore()
		var runID string
		var accepted bool
		if id := f.values["run"]; id != "" {
			e.RunID = id
			runID = id
			accepted, err = store.Append(e)
		} else {
			var n int
			marker := ""
			if ps, perr := iterrun.GetPlanSummary(project, plan); perr == nil {
				if t, ok := ps.ExecutingAt(); ok {
					marker = t.UTC().Format(time.RFC3339)
				} else {
					marker = strings.TrimSpace(ps.Executing)
				}
			}
			runID, n, err = store.Observe(runrecord.OpenRequest{
				Key: runrecord.PlanKey{ProjectPath: project, ProjectID: e.ProjectID, Plan: plan}, Marker: marker, Source: e.Source,
				At: e.At, Versions: iterrun.RecordPlanVersions(project, plan), AttemptKey: e.AttemptKey,
			}, func(runrecord.Run) []runrecord.Event { return []runrecord.Event{e} })
			accepted = n == 1
		}
		if err != nil {
			return recordFail(out, diag, asJSON, 1, err, "")
		}
		status := "accepted"
		if !accepted {
			status = "duplicate"
		}
		if asJSON {
			recordJSON(out, map[string]any{"status": status, "accepted": accepted, "event_id": e.ID, "run_id": runID})
		} else {
			fmt.Fprintf(out, "%s %s %s\n", status, e.ID, runID)
		}
		return 0

	case "import":
		if f.values["archive"] == "" && defaultPlan() == "" {
			return recordFail(out, diag, asJSON, 2, errors.New("no plan named and the project has no current plan"), "Name one: iterate-run record import --plan <name>")
		}
		run, n, err := iterrun.ImportLegacyRecord(project, plan, f.values["archive"], f.bools["dry-run"])
		if errors.Is(err, runrecord.ErrNotFound) {
			return recordFail(out, diag, asJSON, 3, fmt.Errorf("no plan or archive file for %q in %s", plan, project), "")
		}
		if err != nil {
			return recordFail(out, diag, asJSON, 1, err, "")
		}
		if asJSON {
			recordJSON(out, map[string]any{"run_id": run.RunID, "accepted": n, "dry_run": f.bools["dry-run"], "run": run})
			return 0
		}
		verb := "imported"
		if f.bools["dry-run"] {
			verb = "would import"
			n = run.Events
		}
		fmt.Fprintf(out, "%s %d events into %s (%s)\n", verb, n, run.RunID, run.Provenance)
		return 0
	}
	return recordFail(out, diag, asJSON, 2, fmt.Errorf("unknown verb %q; verbs are list, show, append, import, types", verb), "Run: iterate-run record --help")
}

func appendRecordBatch(data []byte, f recordFlags, project, plan string, out, diag io.Writer) int {
	asJSON := f.bools["json"]
	if f.values["type"] != "" {
		return recordFail(out, diag, asJSON, 2, errors.New("--type does not apply to a mixed event batch"), "Give every event its type")
	}
	var events []runrecord.Event
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&events); e != nil {
		return recordFail(out, diag, asJSON, 2, e, "Pass one event object or an array of event objects")
	}
	var extra any
	if e := dec.Decode(&extra); e != io.EOF {
		return recordFail(out, diag, asJSON, 2, errors.New("one JSON document required"), "")
	}
	if len(events) == 0 {
		return recordFail(out, diag, asJSON, 2, errors.New("event batch is empty"), "")
	}
	ids := []string{}
	for i := range events {
		e := &events[i]
		if e.Type == "" {
			return recordFail(out, diag, asJSON, 2, fmt.Errorf("event %d has no type", i+1), "")
		}
		if err := runrecord.ValidatePayload(*e); err != nil {
			return recordFail(out, diag, asJSON, 2, err, "")
		}
		e.ProjectPath, e.ProjectKey, e.Plan = project, "", plan
		if e.Source == "" {
			e.Source = runrecord.SourceCLI
		}
		if e.ID == "" {
			e.ID = runrecord.NewEventID()
		}
		ids = append(ids, e.ID)
	}
	store := iterrun.RecordStore()
	runID := f.values["run"]
	accepted := 0
	var err error
	if runID != "" {
		for i := range events {
			events[i].RunID = runID
		}
		accepted, err = store.AppendAll(events)
	} else {
		marker := ""
		if ps, e := iterrun.GetPlanSummary(project, plan); e == nil {
			if at, ok := ps.ExecutingAt(); ok {
				marker = at.UTC().Format(time.RFC3339)
			} else {
				marker = strings.TrimSpace(ps.Executing)
			}
		}
		first := events[0]
		runID, accepted, err = store.Observe(runrecord.OpenRequest{Key: runrecord.PlanKey{ProjectPath: project, ProjectID: first.ProjectID, Plan: plan}, Marker: marker, Source: first.Source, At: first.At, AttemptKey: first.AttemptKey, Versions: iterrun.RecordPlanVersions(project, plan)}, func(runrecord.Run) []runrecord.Event { return events })
	}
	if err != nil {
		return recordFail(out, diag, asJSON, 1, err, "")
	}
	status := "accepted"
	if accepted == 0 {
		status = "duplicate"
	}
	if asJSON {
		recordJSON(out, map[string]any{"status": status, "accepted": accepted > 0, "accepted_count": accepted, "event_ids": ids, "run_id": runID})
	} else {
		fmt.Fprintf(out, "%s %d events %s\n", status, accepted, runID)
	}
	return 0
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}

// printRun is the human summary of one run: one fact per line.
func printRun(out io.Writer, r runrecord.Run) {
	fmt.Fprintf(out, "run %s\tplan %s\tproject %s\n", r.RunID, r.Plan, r.ProjectPath)
	fmt.Fprintf(out, "state %s\tprovenance %s\tevents %d\tduplicates %d\ttorn %v\n", r.State, r.Provenance, r.Events, r.Duplicates, r.Torn)
	fmt.Fprintf(out, "opened %s\tlast %s\tarchive %s\n", fmtTime(r.OpenedAt), fmtTime(r.LastAt), orUnknown(r.ArchiveFile))
	v := r.Versions
	fmt.Fprintf(out, "versions engine=%s executor=%s iterate-run=%s model=%s\n", orUnknown(v.Engine), orUnknown(v.ExecutorVersion), orUnknown(v.IterateRun), orUnknown(v.Model))
	if r.Owner != nil {
		fmt.Fprintf(out, "owner %s %s since %s\n", r.Owner.OwnerKind, r.Owner.OwnerID, r.Owner.AcquiredAt.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(out, "sessions %d\tagents %d\n", len(r.Sessions), len(r.Agents))
	for _, q := range r.Requirements {
		last := "none"
		if n := len(q.Attempts); n > 0 {
			last = q.Attempts[n-1].Outcome
		}
		evidence := len(q.Evidence)
		for _, a := range q.Attempts {
			evidence += len(a.Evidence)
		}
		fmt.Fprintf(out, "requirement %d\tattempts %d\tlast %s\tevidence %d\n", q.Number, len(q.Attempts), last, evidence)
	}
	scopes := make([]string, 0, len(r.Usage.ByScope))
	for s := range r.Usage.ByScope {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)
	for _, s := range scopes {
		t := r.Usage.ByScope[s]
		num := func(p *int64) string {
			if p == nil {
				return "unknown"
			}
			return fmt.Sprint(*p)
		}
		fmt.Fprintf(out, "usage %s\trequests %d\tinput %s\toutput %s\n", s, t.Requests, num(t.InputTokens), num(t.OutputTokens))
	}
	if len(r.Unknown) > 0 {
		fmt.Fprintf(out, "unknown %s\n", strings.Join(r.Unknown, ", "))
	}
}
