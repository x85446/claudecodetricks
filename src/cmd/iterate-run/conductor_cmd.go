package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/conductor/provider"
	"github.com/x85446/claudecodetricks/src/internal/iterrun"
)

type conductorFlags struct {
	values map[string]string
	bools  map[string]bool
	words  []string
}

func parseConductorFlags(args []string) (conductorFlags, error) {
	f := conductorFlags{values: map[string]string{}, bools: map[string]bool{}}
	for _, v := range args {
		if v == "--" {
			break
		}
		if v == "--json" {
			f.bools["json"] = true
		}
	}
	boolean := map[string]bool{"json": true, "help": true, "dry-run": true, "worker": true, "legacy": true, "apply": true, "no-color": true, "yes": true, "no-input": true, "bind": true}
	valued := map[string]bool{"store": true, "config": true, "file": true, "revision": true, "project": true, "subscription": true, "sequence": true, "id": true, "name": true, "path": true, "color": true, "after": true, "limit": true, "run": true, "automation": true}
	for i := 0; i < len(args); i++ {
		v := args[i]
		if v == "--" {
			f.words = append(f.words, args[i+1:]...)
			break
		}
		if v == "-h" {
			v = "--help"
		}
		if !strings.HasPrefix(v, "--") {
			if strings.HasPrefix(v, "-") && v != "-" {
				return f, fmt.Errorf("unknown flag %s; use --help", v)
			}
			f.words = append(f.words, v)
			continue
		}
		k, val, has := strings.Cut(strings.TrimPrefix(v, "--"), "=")
		if boolean[k] {
			if has {
				b, e := strconv.ParseBool(val)
				if e != nil {
					return f, fmt.Errorf("--%s expects true or false", k)
				}
				f.bools[k] = b
			} else {
				f.bools[k] = true
			}
			continue
		}
		if !valued[k] {
			return f, fmt.Errorf("unknown flag --%s; use --help", k)
		}
		if !has {
			i++
			if i == len(args) || strings.HasPrefix(args[i], "--") {
				return f, fmt.Errorf("--%s requires a value", k)
			}
			val = args[i]
		}
		f.values[k] = val
	}
	if c := f.values["color"]; c != "" && c != "always" && c != "never" && c != "auto" {
		return f, errors.New("--color accepts auto, always or never")
	}
	return f, nil
}

// conductorInvocation permits global flags before the command while preserving wrapper arguments.
func conductorInvocation(args []string) ([]string, bool) {
	for i, v := range args {
		if v == "--" {
			return nil, false
		}
		if v == "conductor" {
			out := append([]string{}, args[:i]...)
			out = append(out, args[i+1:]...)
			return out, true
		}
		if !strings.HasPrefix(v, "-") {
			if i > 0 && (args[i-1] == "--store" || args[i-1] == "--config" || args[i-1] == "--color") {
				continue
			}
			return nil, false
		}
	}
	return nil, false
}
func conductorCmd(args []string) { os.Exit(runConductorCLI(args, os.Stdin, os.Stdout, os.Stderr)) }
func runConductorCLI(args []string, input io.Reader, output, diagnostics io.Writer) int {
	f, err := parseConductorFlags(args)
	if err != nil {
		return conductorCLIError(output, diagnostics, f.bools["json"], 0, err, 2)
	}
	if len(f.words) == 0 || f.bools["help"] {
		helpOut := output
		var colored bytes.Buffer
		if !f.bools["json"] && conductorColor(f, output) {
			helpOut = &colored
		}
		conductorHelp(helpOut, f.words)
		if helpOut != output {
			text := colored.String()
			first, rest, _ := strings.Cut(text, "\n")
			fmt.Fprintf(output, "\x1b[1;36m%s\x1b[0m\n%s", first, rest)
		}
		return 0
	}
	if f.words[0] == "completion" {
		shell := "bash"
		if len(f.words) > 1 {
			shell = f.words[1]
		}
		return conductorCompletion(output, diagnostics, shell)
	}
	if err := validateConductorCommand(f); err != nil {
		return conductorCLIError(output, diagnostics, f.bools["json"], 0, err, 2)
	}
	dir := f.values["store"]
	if dir == "" {
		dir = f.values["config"]
	}
	s := iterrun.NewConductorService(dir)
	c, err := s.Config()
	if err != nil {
		return conductorCLIError(output, diagnostics, f.bools["json"], 0, err, 1)
	}
	revision := c.Revision
	var expected *int64
	if value, ok := f.values["revision"]; ok {
		n, e := strconv.ParseInt(value, 10, 64)
		if e != nil || n < 0 {
			return conductorCLIError(output, diagnostics, f.bools["json"], revision, errors.New("--revision requires a nonnegative integer"), 2)
		}
		expected = &n
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	read := func() ([]byte, error) {
		path := f.values["file"]
		if path == "" {
			return nil, errors.New("--file <path|-> is required")
		}
		var r io.Reader = input
		if path != "-" {
			file, e := os.Open(path)
			if e != nil {
				return nil, e
			}
			defer file.Close()
			r = file
		}
		b, e := io.ReadAll(io.LimitReader(r, (8<<20)+1))
		if len(b) > 8<<20 {
			return nil, errors.New("input exceeds 8 MiB")
		}
		return b, e
	}
	requireRevision := func() error {
		if expected == nil {
			return errors.New("--revision is required; read conductor config show first")
		}
		if *expected != revision {
			return conductor.ErrConflict
		}
		return nil
	}
	var data any
	code := 1
	verb := f.words[0]
	project := f.values["project"]
	switch verb {
	case "status", "explain":
		data, err = s.Evaluate(ctx, conductor.EvaluateRequest{ProjectID: project})
	case "simulate":
		var b []byte
		b, err = read()
		if err == nil {
			var req conductor.EvaluateRequest
			err = json.Unmarshal(b, &req)
			if err == nil {
				data, err = s.Evaluate(ctx, req)
			}
		}
	case "tick":
		if f.bools["dry-run"] {
			data, err = s.Evaluate(ctx, conductor.EvaluateRequest{ProjectID: project})
		} else if f.bools["worker"] {
			data, err = iterrun.RunConductorWorker(ctx, s, project)
		} else {
			var result conductor.TickResult
			result, err = s.TickProject(ctx, project)
			data = result
			if err == nil && result.Assignment == nil {
				code = 4
				err = fmt.Errorf("no assignment admitted (%s); inspect conductor explain for gate reasons", result.State)
			}
		}
	case "start", "resume", "pause", "stop", "kill":
		if err = requireRevision(); err == nil {
			if f.bools["dry-run"] {
				data = map[string]any{"dry_run": true, "action": verb, "project_id": project}
			} else {
				err = s.ControlExpected(ctx, verb, project, *expected)
				if err == nil && (verb == "start" || verb == "resume") {
					var pid int
					pid, err = iterrun.StartConductorWorker(s.Dir, project)
					data = map[string]any{"pid": pid, "action": verb}
				} else {
					data = map[string]any{"action": verb}
				}
			}
		}
	case "migrate":
		if f.bools["legacy"] {
			var paths []string
			if project != "" {
				paths = []string{project}
			}
			data, err = iterrun.ImportLegacyConductor(s, paths, f.bools["apply"] && !f.bools["dry-run"])
		} else if f.bools["dry-run"] {
			data = c
		} else {
			data, err = s.Migrate()
		}
	case "config":
		action := "show"
		if len(f.words) > 1 {
			action = f.words[1]
		}
		switch action {
		case "show":
			data = c
		case "apply", "validate":
			var b []byte
			b, err = read()
			if err == nil {
				var draft model.Config
				draft, err = conductor.DecodeConfig(b)
				if err == nil {
					if action == "validate" || f.bools["dry-run"] {
						draft, err = conductor.ValidateDraft(draft)
						data = map[string]any{"valid": err == nil, "config": draft}
					} else if err = requireRevision(); err == nil {
						data, err = s.Apply(*expected, draft)
					}
				}
			}
		default:
			err = fmt.Errorf("unknown config action %q; use show, validate or apply", action)
			code = 2
		}
	case "usage":
		action := "show"
		if len(f.words) > 1 {
			action = f.words[1]
		}
		id := f.values["subscription"]
		switch action {
		case "show", "list":
			var rt conductor.Runtime
			rt, err = s.Runtime()
			data = rt.Usage
			if id != "" {
				u, ok := rt.Usage[id]
				if !ok {
					err = iterrun.ErrConductorNotFound
				}
				data = u
			}
		case "refresh":
			code = 4
			if err = requireRevision(); err == nil {
				if f.bools["dry-run"] {
					data = map[string]any{"dry_run": true, "subscription_id": id}
				} else {
					data, err = iterrun.RefreshConductorUsage(ctx, s, id, *expected, f.bools["bind"])
				}
			}
		default:
			err = errors.New("usage action must be show or refresh")
			code = 2
		}
	case "run":
		action := "list"
		if len(f.words) > 1 {
			action = f.words[1]
		}
		if action != "list" && action != "show" {
			err = errors.New("run action must be list or show")
			code = 2
			break
		}
		var runs []map[string]any
		runs, err = iterrun.ConductorRuns(s, project, "")
		data = runs
		if action == "show" {
			id := f.values["id"]
			if len(f.words) > 2 {
				id = f.words[2]
			}
			found := false
			for _, r := range runs {
				if r["run_id"] == id {
					data = r
					found = true
				}
			}
			if !found {
				err = iterrun.ErrConductorNotFound
			}
		}

	case "event":
		if len(f.words) > 1 && f.words[1] != "list" && f.words[1] != "show" {
			err = errors.New("event action must be list or show")
			code = 2
			break
		}
		id := f.values["run"]
		if id == "" && len(f.words) > 2 {
			id = f.words[2]
		}
		var after uint64
		after, err = strconv.ParseUint("0"+f.values["after"], 10, 64)
		limit := 100
		if v := f.values["limit"]; v != "" {
			limit, err = strconv.Atoi(v)
		}
		if id == "" || limit < 1 || limit > 500 {
			err = errors.New("event requires --run ID and --limit between 1 and 500")
			code = 2
		}
		if err != nil {
			code = 2
		}
		if err == nil {
			data, err = iterrun.ConductorEvents(s, id, after, limit, project, "")
		}
	case "templates":
		data = provider.Templates()
	default:
		allowed := map[string]bool{"project": true, "subscription": true, "sequence": true, "preset": true, "power-band": true, "time-window": true, "overdrive": true, "automation": true}
		if !allowed[verb] {
			err = fmt.Errorf("unknown conductor command %q; use --help", verb)
			code = 2
			break
		}
		if len(f.words) == 1 {
			conductorHelp(output, f.words)
			return 0
		}
		action := "list"
		if len(f.words) > 1 {
			action = f.words[1]
		}
		id := f.values["id"]
		if len(f.words) > 2 {
			id = f.words[2]
		}
		parent := f.values["subscription"]
		actions := map[string]bool{"list": true, "show": true, "create": true, "edit": true, "duplicate": true, "delete": true, "probe": true, "preview": true}
		if !actions[action] {
			err = fmt.Errorf("unknown %s action %q; use list, show, create, edit, duplicate or delete", verb, action)
			code = 2
			break
		}
		if len(f.words) > 3 || (action != "list" && action != "create" && id == "") {
			err = errors.New("one resource ID is required for this action")
			code = 2
			break
		}
		if action == "preview" && verb == "sequence" {
			found := false
			for _, sub := range c.Subscriptions {
				if parent != "" && sub.ID != parent {
					continue
				}
				for _, q := range sub.Sequences {
					if q.ID == id {
						var argv []string
						argv, err = provider.Compile(sub, q, model.Assignment{})
						data = map[string]any{"argv": argv, "command": provider.CommandPreview(argv)}
						found = true
					}
				}
			}
			if !found {
				err = iterrun.ErrConductorNotFound
			}
			break
		}

		if action == "probe" && verb == "subscription" {
			found := false
			for _, sub := range c.Subscriptions {
				if sub.ID == id {
					data = provider.Probe(ctx, sub)
					found = true
				}
			}
			if !found {
				err = iterrun.ErrConductorNotFound
			}
			break
		}
		var b []byte
		if action == "create" || action == "edit" || action == "duplicate" {
			if f.values["file"] != "" {
				b, err = read()
			} else if verb == "project" && action == "create" {
				b, _ = json.Marshal(map[string]string{"name": f.values["name"], "path": f.values["path"]})
			} else if action != "duplicate" {
				err = errors.New("--file <path|-> is required")
				code = 2
			}
			if err != nil {
				break
			}
		}
		data, revision, err = iterrun.ConductorResource(s, verb, action, id, parent, b, expected, f.bools["dry-run"])
	}
	if err != nil {
		if errors.Is(err, conductor.ErrConflict) {
			code = 3
		} else if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "requires") {
			code = 2
		} else if errors.Is(err, iterrun.ErrConductorNotFound) {
			code = 4
		}
		return conductorCLIError(output, diagnostics, f.bools["json"], revision, err, code)
	}
	if next, e := s.Config(); e == nil {
		revision = next.Revision
	}
	enc := json.NewEncoder(output)
	enc.SetIndent("", "  ")
	if f.bools["json"] {
		err = enc.Encode(iterrun.ConductorResponse(data, revision, nil))
	} else {
		err = enc.Encode(data)
	}
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		return 1
	}
	return 0
}
func conductorCLIError(out, diag io.Writer, jsonMode bool, rev int64, err error, code int) int {
	if jsonMode {
		_ = json.NewEncoder(out).Encode(iterrun.ConductorResponse(nil, rev, err))
	} else {
		fmt.Fprintf(diag, "Error: %s\n\nRead iterate-run conductor --help; reload configuration before retrying a write.\n", err)
	}
	return code
}
func conductorHelp(w io.Writer, words []string) {
	fmt.Fprintln(w, `iterate-run conductor — control queued plans through the shared scheduler

Usage: iterate-run [global flags] conductor <noun> <verb> [ID] [flags anywhere]
       iterate-run conductor <status|explain|simulate|tick|start|stop|pause|resume|kill>

Resources: project, subscription, sequence, preset, power-band, time-window,
           overdrive, automation
Actions:   list, show, create, edit, duplicate, delete
Other:     config show|validate|apply; usage show|refresh; run list;
           event list --run ID; templates; migrate; completion bash|zsh|fish

Flags:
  --json                    Stable schema_version/ok/revision/data/error envelope
  --store DIR, --config DIR  Store directory (ITERATE_CONDUCTOR_DIR, then user default)
  --project ID              Restrict lifecycle/queue operations to a project
  --subscription ID         Parent for sequence, preset and overdrive operations
  --file PATH|-             JSON definition, full configuration or simulation request
  --revision N              Expected revision for writes; conflicts do not overwrite
  --dry-run                 Validate/describe without writing or launching
  --legacy [--apply]        Preview legacy import; --apply commits it
  --bind                    Bind authentic account/pool during usage refresh
  --color auto|always|never  Terminal color preference; JSON remains uncolored
  --help, -h                Show help

Examples:
  iterate-run conductor config show --json
  iterate-run conductor project create --path /workspace/app --revision 0
  iterate-run conductor sequence edit SEQUENCE --subscription ACCOUNT --file sequence.json --revision 2
  iterate-run conductor config apply --file config.json --revision 3
  iterate-run conductor simulate --file sample.json --json
  iterate-run conductor usage refresh --subscription ACCOUNT --bind --revision 4

Registrations start disabled. start/resume enable admission and launch a detached
worker. pause/stop finish the active checkpoint; kill interrupts owned work.
Exit codes: 0 success/read-only wait, 1 operation failed, 2 usage, 3 revision
conflict, 4 unavailable. -- terminates option parsing.`)
	if len(words) > 0 {
		fmt.Fprintf(w, "\nHelp context: %s\n", strings.Join(words, " "))
	}
}
func conductorCompletion(out, diag io.Writer, shell string) int {
	words := "conductor status explain simulate tick start stop pause resume kill migrate project subscription sequence preset power-band time-window overdrive automation usage run event config list show create edit duplicate delete apply validate refresh templates"
	switch shell {
	case "bash":
		fmt.Fprintf(out, "complete -W '%s' iterate-run\n", words)
	case "zsh":
		fmt.Fprintf(out, "#compdef iterate-run\n_arguments '*:command:(%s)'\n", words)
	case "fish":
		fmt.Fprintf(out, "complete -c iterate-run -f -a '%s'\n", words)
	default:
		fmt.Fprintln(diag, "completion accepts bash, zsh or fish")
		return 2
	}
	return 0
}

func conductorColor(f conductorFlags, out io.Writer) bool {
	if f.bools["no-color"] {
		return false
	}
	switch f.values["color"] {
	case "always":
		return true
	case "never":
		return false
	}
	if os.Getenv("ITERATE_RUN_NO_COLOR") != "" {
		return false
	}
	if os.Getenv("ITERATE_RUN_COLOR") == "always" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || (os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0") {
		return true
	}
	if os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" || os.Getenv("CI") != "" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	st, e := file.Stat()
	return e == nil && st.Mode()&os.ModeCharDevice != 0
}

func validateConductorCommand(f conductorFlags) error {
	if len(f.words) == 0 {
		return nil
	}
	noun := f.words[0]
	allowed := map[string]bool{"json": true, "help": true, "store": true, "config": true, "dry-run": true, "color": true, "no-color": true, "yes": true, "no-input": true}
	add := func(keys ...string) {
		for _, k := range keys {
			allowed[k] = true
		}
	}
	maxWords := 1
	switch noun {
	case "status", "explain":
		add("project")
	case "simulate":
		add("file")
	case "tick":
		add("project", "worker")
	case "start", "resume", "pause", "stop", "kill":
		add("project", "revision")
	case "migrate":
		add("legacy", "apply", "project")
	case "config":
		maxWords = 2
		add("file", "revision")
	case "usage":
		maxWords = 2
		add("subscription", "revision", "bind")
	case "run":
		maxWords = 3
		add("project", "id")
	case "event":
		maxWords = 3
		add("project", "id", "run", "after", "limit")
	case "templates":
	default:
		maxWords = 3
		add("id", "file", "revision")
		if noun == "sequence" || noun == "preset" || noun == "overdrive" {
			add("subscription")
		}
		if noun == "project" {
			add("path", "name")
		}
	}
	if len(f.words) > maxWords {
		return fmt.Errorf("%s received extra operands; use conductor %s --help", noun, noun)
	}
	for key := range f.values {
		if !allowed[key] {
			return fmt.Errorf("--%s does not apply to conductor %s; use that command's --help", key, noun)
		}
	}
	for key, value := range f.bools {
		if value && !allowed[key] {
			return fmt.Errorf("--%s does not apply to conductor %s", key, noun)
		}
	}
	return nil
}
