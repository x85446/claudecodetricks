package iterrun

import (
	"os"
	"testing"
)

// TestMain points the run record at a temporary store, so a test that
// drives hooks or the Conductor never writes the real ~/.claude record.
// A test that needs the HOME-relative default clears the variable itself.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "iterrun-records-")
	if err != nil {
		panic(err)
	}
	os.Setenv("ITERATE_RUN_RECORD_DIR", dir)
	// Coordinator evidence goes to the same temporary store, and the
	// session that happens to run the suite is not a caller in any case.
	os.Setenv("ITERATE_RUN_SESSIONS_DIR", dir+"/sessions")
	for _, k := range []string{"ITERATE_RUN_SESSION", "CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID", "CLAUDE_PID"} {
		os.Unsetenv(k)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
