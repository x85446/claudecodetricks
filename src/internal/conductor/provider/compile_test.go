package provider

import (
	"context"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TESTMASTER: id=javelina-program-detection tier=? parallel=yes
func TestJavelinaDetect(t *testing.T) {
	missing := Detect(model.Subscription{Executable: "/missing/javelina-program"})
	if missing.Installed || missing.Reason == "" {
		t.Fatal(missing)
	}
	found := Detect(model.Subscription{Executable: "sh"})
	if !found.Installed || !filepath.IsAbs(found.Executable) {
		t.Fatal(found)
	}
}

// TESTMASTER: id=javelina-collector-remedy tier=? parallel=no
func TestJavelinaCollectorRemedies(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	standard := filepath.Join(dir, "claude")
	custom := filepath.Join(dir, "other-claude")
	script := []byte("#!/bin/sh\nprintf 'stream-json\\n'\n")
	for _, path := range []string{standard, custom} {
		if e := os.WriteFile(path, script, 0700); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct{ program, reason string }{{"claude", "CodexBar unavailable"}, {custom, "Custom Claude"}} {
		r := Probe(context.Background(), model.Subscription{Adapter: "claude", Executable: tc.program})
		if r.Telemetry || !strings.Contains(r.Reason, tc.reason) {
			t.Fatal(r)
		}
	}
	r := Probe(context.Background(), model.Subscription{Adapter: "claude", Executable: custom, UsageCommand: []string{"collector"}})
	if !r.Telemetry {
		t.Fatal(r)
	}
}
