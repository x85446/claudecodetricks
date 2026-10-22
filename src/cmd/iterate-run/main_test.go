package main

import (
	"os"
	"testing"
)

// TestMain points the run record at a temporary store, so a test that
// drives the Conductor or a hook never writes the real ~/.claude record.
// A test that needs the HOME-relative default clears the variable itself.
//
// With ITERATE_RUN_RECORD_CLI_HELPER=1 the test binary instead runs
// `iterate-run record` on the arguments after "--" and exits, so a test can
// restart the CLI as a fresh process without building the binary.
func TestMain(m *testing.M) {
	if os.Getenv("ITERATE_RUN_RECORD_CLI_HELPER") == "1" {
		for i, a := range os.Args {
			if a == "--" {
				os.Exit(runRecordCLI(os.Args[i+1:], os.Stdin, os.Stdout, os.Stderr))
			}
		}
		os.Exit(2)
	}
	dir, err := os.MkdirTemp("", "iterate-run-records-")
	if err != nil {
		panic(err)
	}
	os.Setenv("ITERATE_RUN_RECORD_DIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
