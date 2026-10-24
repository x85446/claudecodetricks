package iterrun

import (
	"os"
	"path/filepath"
	"time"
)

// These read-only JSON projections preserve historical producer documents.
// They contain only fields the run-record legacy reader consumes.
type legacyLeaseDocument struct {
	Started    time.Time `json:"started"`
	Assignment struct {
		ID               string `json:"id"`
		RunID            string `json:"run_id"`
		ProjectPath      string `json:"project_path"`
		Plan, Kind, Team string
		Step             int
		Subscription     struct{ Adapter string }
		Sequence         struct{ Model string }
	} `json:"assignment"`
}
type legacyResultDocument struct {
	Outcome, Blocker string
	ValidationMet    bool `json:"validation_met"`
	Evidence         []string
}
type legacyUsageDocument struct {
	Cumulative   bool
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	SessionID    string `json:"session_id"`
	At           time.Time
}

func legacyRunDirectory() string {
	if p := os.Getenv("ITERATE_LEGACY_RUN_DIR"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "iterate", "conductor")
}
