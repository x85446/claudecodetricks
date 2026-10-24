package iterrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// TESTMASTER: id=nene-record-baseline tier=? parallel=no
func TestNeneRecordBaseline(t *testing.T) {
	destination := os.Getenv("NENE_BASELINE_DEST")
	lcEnv(t)
	project := lcProject(t, "goldfinch")
	start := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	clock := lcUseClock(t, start)
	writeRecordPlan(t, project, "goldfinch", lcExec)
	lcAttempt(t, clock, project, "goldfinch", 1, "nene-baseline", start, start.Add(time.Minute), 1000, 100,
		model.Result{Outcome: "completed", ValidationMet: true, Evidence: []string{"fixture receipt read back"}})
	runs, err := RecordStore().Runs(project, "goldfinch")
	if err != nil || len(runs) != 1 {
		t.Fatalf("fixture record: %v %v", runs, err)
	}
	b, err := json.MarshalIndent(runs[0], "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if destination != "" {
		if err := os.WriteFile(filepath.Join(destination, "record-golden.json"), b, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, "record-golden-command.txt"), []byte("NENE_BASELINE_DEST=<evidence directory> testmaster suite run iterrun.TestNeneRecordBaseline\nThe current in-process RecordAssignment writer produces this isolated, fixed-clock fold.\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
