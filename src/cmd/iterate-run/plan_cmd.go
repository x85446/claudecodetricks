package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/iterrun"
	"github.com/x85446/claudecodetricks/src/internal/plangraph"
)

// planFlags holds a `plan` invocation after options-anywhere parsing: the
// words in order and every flag wherever it appeared.
type planFlags struct {
	words  []string
	values map[string]string
	bools  map[string]bool
}

var (
	planValued  = map[string]bool{"format": true, "project": true, "color": true}
	planBoolean = map[string]bool{"check": true, "json": true, "apply": true, "help": true, "no-color": true}
	planShort   = map[string]string{"-C": "project", "-h": "help"}
)

// planInvocation finds the `plan` noun after any plan flags, so
// `iterate-run -C dir plan graph` and `iterate-run plan graph -C dir` route
// the same way.
func planInvocation(args []string) ([]string, bool) {
	for i := 0; i < len(args); i++ {
		v := args[i]
		if v == "--" {
			return nil, false
		}
		if v == "plan" {
			out := append([]string{}, args[:i]...)
			return append(out, args[i+1:]...), true
		}
		if long, ok := planShort[v]; ok {
			if planValued[long] {
				i++
			}
			continue
		}
		if strings.HasPrefix(v, "--") {
			k, _, has := strings.Cut(strings.TrimPrefix(v, "--"), "=")
			if planValued[k] && !has {
				i++
			}
			if !planValued[k] && !planBoolean[k] {
				return nil, false
			}
			continue
		}
		return nil, false
	}
	return nil, false
}

func parsePlanFlags(args []string) (planFlags, error) {
	f := planFlags{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		v := args[i]
		if v == "--" {
			f.words = append(f.words, args[i+1:]...)
			break
		}
		if long, ok := planShort[v]; ok {
			v = "--" + long
		}
		if !strings.HasPrefix(v, "--") {
			if strings.HasPrefix(v, "-") && v != "-" {
				return f, fmt.Errorf("unknown flag %s", v)
			}
			f.words = append(f.words, v)
			continue
		}
		k, val, has := strings.Cut(strings.TrimPrefix(v, "--"), "=")
		switch {
		case planBoolean[k]:
			if has {
				return f, fmt.Errorf("--%s takes no value", k)
			}
			f.bools[k] = true
		case planValued[k]:
			if !has {
				if i+1 >= len(args) {
					return f, fmt.Errorf("--%s needs a value", k)
				}
				i++
				val = args[i]
			}
			f.values[k] = val
		default:
			return f, fmt.Errorf("unknown flag --%s", k)
		}
	}
	return f, nil
}

func planCmd(args []string) {
	os.Exit(runPlanCLI(args, os.Stdout, os.Stderr, time.Now()))
}

// planUsageError prints the three-part error with its next action and
// returns exit code 2.
func planUsageError(diag io.Writer, what, next string) int {
	fmt.Fprintf(diag, "Error: %s\n\n  %s\n", what, next)
	return 2
}

func runPlanCLI(args []string, out, diag io.Writer, now time.Time) int {
	f, err := parsePlanFlags(args)
	if err != nil {
		return planUsageError(diag, err.Error(), "See the flags with: iterate-run plan --help")
	}
	verb := strings.Join(f.words, " ")
	if len(f.words) == 0 || f.bools["help"] {
		switch verb {
		case "graph":
			planGraphHelp(out)
		case "depends", "depends backfill":
			planBackfillHelp(out)
		case "":
			planHelp(out)
		default:
			planHelp(out)
		}
		return 0
	}
	if f.values["color"] != "" && f.values["color"] != "auto" && f.values["color"] != "always" && f.values["color"] != "never" {
		return planUsageError(diag, fmt.Sprintf("--color received %q; it accepts auto, always or never", f.values["color"]), "Run: iterate-run plan "+verb+" --color never")
	}
	project := f.values["project"]
	if project == "" {
		project = "."
	}
	switch verb {
	case "graph":
		for _, k := range []string{"apply"} {
			if f.bools[k] {
				return planUsageError(diag, "--"+k+" does not apply to plan graph; it belongs to plan depends backfill", "Run: iterate-run plan depends backfill --"+k)
			}
		}
		return runPlanGraph(f, project, out, diag)
	case "depends":
		planBackfillHelp(out)
		return 0
	case "depends backfill":
		for _, k := range []string{"check"} {
			if f.bools[k] {
				return planUsageError(diag, "--"+k+" does not apply to plan depends backfill; it belongs to plan graph", "Run: iterate-run plan graph --"+k)
			}
		}
		if f.values["format"] != "" {
			return planUsageError(diag, "--format does not apply to plan depends backfill; it belongs to plan graph", "Use --json for machine output: iterate-run plan depends backfill --json")
		}
		return runPlanBackfill(f, project, out, diag, now)
	}
	next := "See the commands with: iterate-run plan --help"
	if guess := planDidYouMean(f.words[0]); guess != "" {
		next = "Did you mean: iterate-run plan " + guess + "?"
	}
	return planUsageError(diag, fmt.Sprintf("unknown plan command %q", verb), next)
}

func planDidYouMean(word string) string {
	best, dist := "", 3
	for _, cand := range []string{"graph", "depends backfill"} {
		head, _, _ := strings.Cut(cand, " ")
		if d := levenshtein(word, head); d < dist {
			best, dist = cand, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func planProjectError(diag io.Writer, project string, err error) int {
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(diag, "Error: no iterate plans in %s: .claude/iterate/plans does not exist\n\n  Run it from a project, or name one: iterate-run plan graph -C <dir>\n", project)
		return 1
	}
	fmt.Fprintf(diag, "Error: cannot read the plans in %s: %v\n\n  Check the directory's permissions, then run the command again.\n", project, err)
	return 1
}

// planPalette is the resolved color tier for one stream, plus whether the
// locale renders the box-drawing symbols.
type planPalette struct {
	color bool
	utf8  bool
}

func (p planPalette) paint(code, s string) string {
	if !p.color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func newPlanPalette(f planFlags, out io.Writer) planPalette {
	cf := conductorFlags{values: map[string]string{"color": f.values["color"]}, bools: map[string]bool{"no-color": f.bools["no-color"]}}
	locale := strings.ToLower(os.Getenv("LC_ALL") + os.Getenv("LC_CTYPE") + os.Getenv("LANG"))
	return planPalette{color: conductorColor(cf, out), utf8: strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")}
}

func runPlanGraph(f planFlags, project string, out, diag io.Writer) int {
	format := f.values["format"]
	if f.bools["json"] {
		if format != "" && format != "json" {
			return planUsageError(diag, fmt.Sprintf("--json conflicts with --format %s", format), "Pick one: iterate-run plan graph --format json")
		}
		format = "json"
	}
	if format == "" {
		format = "text"
	}
	if format != "text" && format != "mermaid" && format != "json" {
		return planUsageError(diag, fmt.Sprintf("--format received %q; it accepts text, mermaid or json", format), "Run: iterate-run plan graph --format mermaid")
	}
	if _, err := os.Stat(project + "/.claude/iterate/plans"); err != nil {
		return planProjectError(diag, project, err)
	}
	v, err := iterrun.BuildGraphView(project)
	if err != nil {
		return planProjectError(diag, project, err)
	}
	if f.bools["check"] {
		for _, e := range v.Errors {
			fmt.Fprintln(out, e)
		}
		if len(v.Errors) > 0 {
			return 1
		}
		return 0
	}
	switch format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			fmt.Fprintln(diag, "Error:", err)
			return 1
		}
		return 0
	case "mermaid":
		fmt.Fprint(out, v.Mermaid())
		return 0
	}
	pal := newPlanPalette(f, out)
	if len(v.Nodes) == 0 {
		fmt.Fprintf(diag, "no plans in %s\n", v.Project)
		return 0
	}
	for _, row := range v.TextRows() {
		fmt.Fprintln(out, planGraphLine(row, pal))
	}
	if len(v.Errors) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, pal.paint("1", "Errors:"))
		for _, e := range v.Errors {
			fmt.Fprintln(out, "  "+pal.paint("31", "✖")+" "+e)
		}
	}
	counts := map[string]int{}
	live := 0
	for _, n := range v.Nodes {
		if !n.Archived {
			live++
			counts[iterrun.DisplayStatus(n)]++
		}
	}
	fmt.Fprintf(diag, "%d plans: %d ready, %d waiting, %d blocked\n", live, counts[plangraph.StatusReady], counts[plangraph.StatusWaiting], counts[plangraph.StatusBlocked])
	return 0
}

func planGraphLine(row iterrun.GraphRow, pal planPalette) string {
	var b strings.Builder
	vert, tee, elbow := "│  ", "├─ ", "└─ "
	if !pal.utf8 {
		vert, tee, elbow = "|  ", "|- ", `\- `
	}
	for i := 0; i < row.Depth-1; i++ {
		if row.Last[i] {
			b.WriteString("   ")
		} else {
			b.WriteString(vert)
		}
	}
	if row.Depth > 0 {
		if row.Last[row.Depth-1] {
			b.WriteString(elbow)
		} else {
			b.WriteString(tee)
		}
	}
	n := row.Node
	sym, code := "●", ""
	ascii := "*"
	switch iterrun.DisplayStatus(n) {
	case plangraph.StatusDone:
		sym, ascii, code = "✔", "+", "32"
	case plangraph.StatusWaiting:
		sym, ascii, code = "◌", "~", "33"
	case plangraph.StatusBlocked, plangraph.StatusHeld:
		sym, ascii, code = "✖", "x", "31"
	}
	if !pal.utf8 {
		sym = ascii
	}
	b.WriteString(pal.paint(code, sym))
	b.WriteString(" ")
	b.WriteString(pal.paint("1", n.Name))
	if len(n.Milestones) > 0 {
		b.WriteString("  " + pal.paint("2", strings.Join(n.Milestones, ", ")))
	}
	word := n.State
	if n.Archived {
		word = string(n.Outcome)
		if word == "" {
			word = "archived"
		}
	}
	detail := row.Detail
	if detail == word || (n.Archived && detail == plangraph.StatusDone) {
		detail = ""
	}
	b.WriteString("  " + word)
	if detail != "" {
		b.WriteString(" · " + pal.paint(code, detail))
	}
	if len(row.Also) > 0 {
		b.WriteString("  " + pal.paint("2", "also after "+strings.Join(row.Also, ", ")))
	}
	return b.String()
}

func runPlanBackfill(f planFlags, project string, out, diag io.Writer, now time.Time) int {
	rep, err := iterrun.BackfillDepends(project, f.bools["apply"], now)
	if err != nil {
		return planProjectError(diag, project, err)
	}
	if f.bools["json"] {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintln(diag, "Error:", err)
			return 1
		}
		return 0
	}
	pal := newPlanPalette(f, out)
	width := 0
	for _, r := range rep.Rows {
		if r.Action != "none" && r.Action != "unresolved" && len(r.Plan) > width {
			width = len(r.Plan)
		}
	}
	for _, r := range rep.Rows {
		switch r.Action {
		case "none":
			continue
		case "unresolved":
			fmt.Fprintf(out, "%s %s: %s\n", pal.paint("33", "unresolved"), r.Plan, r.Prose)
			continue
		}
		detail := r.Line
		switch r.Action {
		case "skip":
			detail = r.Source
		case "propose", "applied":
			if r.Source != "" {
				detail += "  " + pal.paint("2", "("+r.Source+")")
			}
		}
		fmt.Fprintf(out, "%-10s %-*s  %s\n", r.Action, width, r.Plan, detail)
	}
	proposed, applied := rep.Count("propose"), rep.Count("applied")
	unresolved, skipped, kept := rep.Count("unresolved"), rep.Count("skip"), rep.Count("keep")
	if f.bools["apply"] {
		fmt.Fprintf(diag, "applied %d, unresolved %d, skipped %d, kept %d\n", applied, unresolved, skipped, kept)
	} else {
		fmt.Fprintf(diag, "dry run: %d to propose, %d unresolved, %d skipped, %d kept\n", proposed, unresolved, skipped, kept)
		if proposed > 0 {
			fmt.Fprintf(diag, "  Write them with: iterate-run plan depends backfill --apply -C %s\n", rep.Project)
		}
	}
	if unresolved > 0 {
		fmt.Fprintln(diag, "  Settle each unresolved plan by reading its Depends on, then: /ip <plan> depends on <x>")
	}
	return 0
}

func planHelp(w io.Writer) {
	fmt.Fprintln(w, `iterate-run plan — read and repair the dependency graph between a project's plans

Usage:
  iterate-run plan <command> [flags]

Examples:
  iterate-run plan graph                          # this project's graph as a tree
  iterate-run plan graph --format mermaid         # paste into Markdown
  iterate-run plan graph --check -C ~/work/app    # exit 1 on a cycle or unknown name
  iterate-run plan depends backfill               # dry run: propose depends: lines
  iterate-run plan depends backfill --apply       # write them

Commands:
  graph               print the plan graph: each plan, its state, what it waits on
  depends backfill    derive depends: lines from milestones and Depends on prose

Global flags:
  -C, --project <dir>         project directory (default .)
      --color <when>          auto, always or never (default auto)
      --no-color              same as --color never
  -h, --help                  help for this command

Errors go to stderr with exit 1 (operation failed) or 2 (usage).
See also: iterate-run status, iterate-run conductor status`)
}

func planGraphHelp(w io.Writer) {
	fmt.Fprintln(w, `iterate-run plan graph — print the dependency graph between a project's plans

Usage:
  iterate-run plan graph [flags]

Examples:
  iterate-run plan graph                          # tree: each plan once, under its predecessor
  iterate-run plan graph --format mermaid         # Mermaid flowchart for Markdown
  iterate-run plan graph --json | jq '.nodes[] | select(.status=="waiting")'
  iterate-run plan graph --check                  # one line per error; exit 1 when any

Flags:
      --format <fmt>          text, mermaid or json (default text)
      --json                  same as --format json
      --check                 print graph errors only; exit 1 when there are any

Global flags:
  -C, --project <dir>         project directory (default .)
      --color <when>          auto, always or never (default auto)
      --no-color              same as --color never

A plan is ready when every predecessor on its depends: line is archived done
or landed, waiting while one is unfinished, and blocked when its own depends:
line names an unknown plan, itself, or a cycle.

See also: iterate-run plan depends backfill, iterate-run conductor status`)
}

func planBackfillHelp(w io.Writer) {
	fmt.Fprintln(w, `iterate-run plan depends backfill — derive depends: lines for plans that lack one

Usage:
  iterate-run plan depends backfill [flags]

Examples:
  iterate-run plan depends backfill               # dry run: what it would write
  iterate-run plan depends backfill --apply       # write the lines
  iterate-run plan depends backfill --json -C ~/work/app

Flags:
      --apply                 write the proposed lines (default: dry run)
      --json                  the report as JSON on stdout

Global flags:
  -C, --project <dir>         project directory (default .)
      --color <when>          auto, always or never (default auto)
      --no-color              same as --color never

Sources, merged: the plan's milestone: line in docs/milestones.md (internal ids;
(external) items dropped), and Depends on prose naming (from plan <name>),
(from milestone <id>) or nothing. A plan with a depends: line is kept as
written; a plan with a live run is skipped. Prose naming no plan is listed as
unresolved and needs a reading.

See also: iterate-run plan graph --check`)
}
