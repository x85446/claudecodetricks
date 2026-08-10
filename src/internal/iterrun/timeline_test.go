package iterrun

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRowsGapDetection(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	mk := func(offsetSec int, hook, agentID, toolName, toolUseID string) Event {
		return Event{
			TS: base.Add(time.Duration(offsetSec) * time.Second), Hook: hook,
			AgentID: agentID, ToolName: toolName, ToolUseID: toolUseID,
		}
	}

	events := []Event{
		// coordinator: two calls 10s apart (no notable gap)
		mk(0, "pre", "", "Bash", "c1"),
		mk(2, "post", "", "Bash", "c1"),
		mk(12, "pre", "", "Bash", "c2"),
		mk(14, "post", "", "Bash", "c2"),
		// coordinator: then a 400s (severe) gap before the next call
		mk(414, "pre", "", "Bash", "c3"),
		mk(416, "post", "", "Bash", "c3"),

		// agent "team-x": one call, then a 90s (notable, not severe) gap
		mk(0, "pre", "team-x", "Bash", "a1"),
		mk(1, "post", "team-x", "Bash", "a1"),
		mk(91, "pre", "team-x", "Bash", "a2"),
		mk(93, "post", "team-x", "Bash", "a2"),
	}

	rows := BuildRows(events, map[string]string{"team-x": "badger-app"})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (coordinator + team-x), got %d", len(rows))
	}

	var coord, teamX *Row
	for i := range rows {
		switch rows[i].key {
		case "":
			coord = &rows[i]
		case "team-x":
			teamX = &rows[i]
		}
	}
	if coord == nil || teamX == nil {
		t.Fatalf("missing expected rows: coord=%v teamX=%v", coord, teamX)
	}

	if coord.label != "coordinator" {
		t.Errorf("coordinator label = %q, want %q", coord.label, "coordinator")
	}
	if teamX.label != "badger-app" {
		t.Errorf("team-x label = %q, want %q (label resolution failed)", teamX.label, "badger-app")
	}

	if len(coord.spans) != 3 {
		t.Fatalf("coordinator spans = %d, want 3", len(coord.spans))
	}
	if len(coord.gaps) != 1 {
		t.Fatalf("coordinator gaps = %d, want 1 (only the 400s gap clears the 60s notable threshold)", len(coord.gaps))
	}
	if coord.gaps[0].dur() < SevereGap {
		t.Errorf("coordinator's one gap should be severe (>=5m), got %s", coord.gaps[0].dur())
	}

	if len(teamX.gaps) != 1 {
		t.Fatalf("team-x gaps = %d, want 1", len(teamX.gaps))
	}
	if teamX.gaps[0].dur() < NotableGap || teamX.gaps[0].dur() >= SevereGap {
		t.Errorf("team-x's gap should be notable but not severe, got %s", teamX.gaps[0].dur())
	}

	// Rendering shouldn't error, and the summary should mention the severe gap.
	var buf strings.Builder
	PrintTimelineSummary(&buf, rows)
	out := buf.String()
	if !strings.Contains(out, "SEVERE GAP") {
		t.Errorf("summary output missing SEVERE GAP marker:\n%s", out)
	}
	if !strings.Contains(out, "badger-app") {
		t.Errorf("summary output missing resolved label badger-app:\n%s", out)
	}
}
