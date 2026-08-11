package main

import (
	"fmt"
	"os"

	"github.com/x85446/claudecodetricks/src/internal/iterrun"
)

// Set via -ldflags at build time (see Makefile). This is the first binary
// in this repo to actually declare these symbols — the Makefile has been
// injecting them into every hook all along, but none declared the vars.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "status":
		statusCmd(os.Args[2:])
	case "hook":
		hookCmd(os.Args[2:])
	case "timeline", "graph":
		timelineCmd(os.Args[2:])
	case "serve":
		serveCmd(os.Args[2:])
	case "purge":
		purgeCmd(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("iterate-run %s (commit %s, built %s)\n", Version, Commit, BuildTime)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `iterate-run — heartbeat/progress wrapper + status/timeline/dashboard for the iterate stack

Usage:
  iterate-run run --plan <name> [--team <name>] --unit <name> -- <command> [args...]
  iterate-run status
  iterate-run hook pre|post          (wired into PreToolUse/PostToolUse in settings.json, not run by hand)
  iterate-run timeline [--plan <name>] [--home <dir>] [scan-dir...]
  iterate-run serve [--port N]       (dashboard at http://localhost:N, default 8420)
  iterate-run purge --plan <name> | --all-completed [--yes] [--force]
  iterate-run version`)
}

// registerCWD best-effort-registers the current directory as a known
// project if it has a plans/ folder — called from every subcommand that
// touches a project directory, so the dashboard and purge index build
// themselves from ordinary use with no separate setup step.
func registerCWD() {
	if cwd, err := os.Getwd(); err == nil {
		iterrun.RegisterProject(cwd)
	}
}

func hookCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "iterate-run: hook requires pre or post")
		os.Exit(2)
	}
	phase := args[0]
	if phase != "pre" && phase != "post" {
		fmt.Fprintln(os.Stderr, "iterate-run: hook phase must be pre or post")
		os.Exit(2)
	}
	// Never fail loudly here: a hook that exits non-zero or hangs can
	// interfere with the real tool call it's observing. Observability
	// must be strictly best-effort.
	iterrun.HandleHook(phase, os.Stdin)
}

func timelineCmd(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
		os.Exit(1)
	}
	registerCWD()

	var plan, home string
	var scanDirs []string
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--plan":
			i++
			plan = args[i]
		case "--home":
			i++
			home = args[i]
		default:
			scanDirs = append(scanDirs, args[i])
		}
		i++
	}

	events, eventsErr := iterrun.ReadEvents()
	labels, labelsErr := iterrun.ReadLabels()

	var rows []iterrun.Row
	if plan != "" {
		// Filesystem mode: build the graph from team logs + iterate-run
		// registry entries already on disk — works for a run already in
		// progress, unlike hook data (which only sees activity from the
		// moment hooks are wired in). Merge in whatever hook-derived
		// activity has accumulated for this plan, if any — same command
		// automatically gets richer once hooks have been live a while.
		if home == "" {
			home = cwd
		} else {
			iterrun.RegisterProject(home)
		}
		rows, err = iterrun.BuildRowsFromFilesystem(plan, home, scanDirs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
			os.Exit(1)
		}
		if eventsErr == nil && labelsErr == nil {
			rows = iterrun.MergeRows(rows, iterrun.BuildRowsFromHookEvents(events, labels, plan, home))
		}
	} else {
		if eventsErr != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", eventsErr)
			os.Exit(1)
		}
		if labelsErr != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", labelsErr)
			os.Exit(1)
		}
		rows = iterrun.BuildRows(events, labels)
	}

	iterrun.PrintTimelineSummary(os.Stdout, rows)

	planSummary := iterrun.PlanSummary{Name: plan}
	if plan != "" {
		if ps, err := iterrun.GetPlanSummary(home, plan); err == nil {
			planSummary = ps
		}
	}
	path, err := iterrun.WriteTimelineHTML(cwd, rows, planSummary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iterate-run: could not write HTML timeline: %v\n", err)
		return
	}
	fmt.Printf("\nHTML timeline written to %s\n", path)
}

func runCmd(args []string) {
	opts := iterrun.Options{}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
		os.Exit(1)
	}
	opts.CWD = cwd
	registerCWD()

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--plan":
			i++
			opts.Plan = args[i]
		case "--team":
			i++
			opts.Team = args[i]
		case "--unit":
			i++
			opts.Unit = args[i]
		case "--":
			opts.Args = args[i+1:]
			i = len(args)
			continue
		default:
			fmt.Fprintf(os.Stderr, "iterate-run: unknown flag %q\n", args[i])
			usage()
			os.Exit(2)
		}
		i++
	}

	if opts.Plan == "" || opts.Unit == "" {
		fmt.Fprintln(os.Stderr, "iterate-run: --plan and --unit are required")
		usage()
		os.Exit(2)
	}

	if err := iterrun.Run(opts); err != nil {
		os.Exit(1)
	}
}

func statusCmd(_ []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
		os.Exit(1)
	}
	registerCWD()
	iterrun.PrintStatus(cwd, os.Stdout)
}

func serveCmd(args []string) {
	port := "8420"
	i := 0
	for i < len(args) {
		if args[i] == "--port" {
			i++
			port = args[i]
		}
		i++
	}
	if err := iterrun.Serve("localhost:" + port); err != nil {
		fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
		os.Exit(1)
	}
}

func purgeCmd(args []string) {
	var plan string
	var allCompleted, yes, force bool
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--plan":
			i++
			plan = args[i]
		case "--all-completed":
			allCompleted = true
		case "--yes":
			yes = true
		case "--force":
			force = true
		default:
			fmt.Fprintf(os.Stderr, "iterate-run: unknown flag %q\n", args[i])
			os.Exit(2)
		}
		i++
	}

	if plan == "" && !allCompleted {
		fmt.Fprintln(os.Stderr, "iterate-run: purge requires --plan <name> or --all-completed")
		os.Exit(2)
	}
	if plan != "" && allCompleted {
		fmt.Fprintln(os.Stderr, "iterate-run: --plan and --all-completed are mutually exclusive")
		os.Exit(2)
	}

	var targets []iterrun.PlanSummary
	if allCompleted {
		eligible, err := iterrun.PurgeEligible()
		if err != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
			os.Exit(1)
		}
		if len(eligible) == 0 {
			fmt.Println("no completed plans found to purge")
			return
		}
		targets = eligible
	} else {
		p, err := iterrun.FindPlan(plan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
			os.Exit(1)
		}
		if !p.IsCompleted() && !force {
			fmt.Fprintf(os.Stderr, "iterate-run: plan %q does not look completed (teams %d/%d done, or no Teams table to check against) — pass --force to purge it anyway\n",
				plan, p.TeamsDone, p.TeamsTotal)
			os.Exit(1)
		}
		targets = []iterrun.PlanSummary{p}
	}

	dryRun := !yes
	for _, t := range targets {
		res, err := iterrun.PurgePlan(t.Name, dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: purging %q: %v\n", t.Name, err)
			continue
		}
		verb := "would remove"
		if !dryRun {
			verb = "removed"
		}
		fmt.Printf("%s: %s %d hook event(s), %d label(s), %d registry/log file(s)\n",
			t.Name, verb, res.EventsRemoved, res.LabelsRemoved, len(res.RegistryFiles))
		for _, f := range res.RegistryFiles {
			fmt.Printf("  %s\n", f)
		}
	}
	if dryRun {
		fmt.Println("\ndry run — nothing deleted. Add --yes to actually purge.")
	}
}
