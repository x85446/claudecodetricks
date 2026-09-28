package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHeartbeatPathIsShellComputable(t *testing.T) {
	// The status line builds this path with ${DIR//\//-} and nothing
	// else, so the encoding must stay a plain separator substitution --
	// no hash, no escaping, no length cap.
	dir := "/Users/someone/workspace/proj"
	got := filepath.Base(HeartbeatPath(dir))
	if want := "-Users-someone-workspace-proj"; got != want {
		t.Errorf("HeartbeatPath base = %q, want %q", got, want)
	}
	if strings.Contains(got, string(os.PathSeparator)) {
		t.Errorf("heartbeat filename %q still contains a separator", got)
	}
}

func TestHeartbeatRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()

	if _, ok := HeartbeatAge(proj, time.Now()); ok {
		t.Error("a project with no heartbeat reported one")
	}
	if IsLive(proj, time.Now()) {
		t.Error("a project with no heartbeat reported live")
	}

	TouchHeartbeat(proj)
	age, ok := HeartbeatAge(proj, time.Now())
	if !ok {
		t.Fatal("no heartbeat after TouchHeartbeat")
	}
	if age > time.Minute {
		t.Errorf("fresh heartbeat reported age %v", age)
	}
	if !IsLive(proj, time.Now()) {
		t.Error("fresh heartbeat did not report live")
	}
	// The whole point: an old stamp is not live, which is what makes a
	// crashed run stop reading as green.
	if IsLive(proj, time.Now().Add(2*DefaultLiveWindow)) {
		t.Error("a heartbeat older than the window still reported live")
	}
}

func TestLiveWindowOverride(t *testing.T) {
	t.Setenv("ITERATE_LIVE_SECS", "60")
	if got := LiveWindow(); got != time.Minute {
		t.Errorf("LiveWindow = %v, want 1m", got)
	}
	for _, bad := range []string{"", "0", "-5", "abc"} {
		t.Setenv("ITERATE_LIVE_SECS", bad)
		if got := LiveWindow(); got != DefaultLiveWindow {
			t.Errorf("ITERATE_LIVE_SECS=%q gave %v, want the default", bad, got)
		}
	}
}

func TestTouchHeartbeatNeverPanicsOnBadInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	TouchHeartbeat("") // must be a no-op, not a crash: hooks must never fail loudly
}
