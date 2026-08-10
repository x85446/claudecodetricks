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
	case "version", "--version", "-v":
		fmt.Printf("iterate-run %s (commit %s, built %s)\n", Version, Commit, BuildTime)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `iterate-run — heartbeat/progress wrapper + status/timeline view for the iterate stack

Usage:
  iterate-run run --plan <name> [--team <name>] --unit <name> -- <command> [args...]
  iterate-run status
  iterate-run hook pre|post          (wired into PreToolUse/PostToolUse in settings.json, not run by hand)
  iterate-run timeline               (prints a text summary + writes .claude/iterate/timeline.html)
  iterate-run version`)
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

	var rows []iterrun.Row
	if plan != "" {
		// Filesystem mode: build the graph from team logs + iterate-run
		// registry entries already on disk — works for a run already in
		// progress, unlike the hook path below (which only sees activity
		// from the moment hooks are wired in).
		if home == "" {
			home = cwd
		}
		rows, err = iterrun.BuildRowsFromFilesystem(plan, home, scanDirs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
			os.Exit(1)
		}
	} else {
		events, err := iterrun.ReadEvents(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
			os.Exit(1)
		}
		labels, err := iterrun.ReadLabels(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iterate-run: %v\n", err)
			os.Exit(1)
		}
		rows = iterrun.BuildRows(events, labels)
	}

	iterrun.PrintTimelineSummary(os.Stdout, rows)

	path, err := iterrun.WriteTimelineHTML(cwd, rows)
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
	iterrun.PrintStatus(cwd, os.Stdout)
}
