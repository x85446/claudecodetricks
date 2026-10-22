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
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
