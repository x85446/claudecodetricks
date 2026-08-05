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
	case "version", "--version", "-v":
		fmt.Printf("iterate-run %s (commit %s, built %s)\n", Version, Commit, BuildTime)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `iterate-run — heartbeat/progress wrapper + status view for the iterate stack

Usage:
  iterate-run run --plan <name> [--team <name>] --unit <name> -- <command> [args...]
  iterate-run status
  iterate-run version`)
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
