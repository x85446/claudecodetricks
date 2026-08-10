//go:build linux

package iterrun

import (
	"os"
	"time"
)

// birthTime has no reliable stdlib-only path on Linux (classic stat has no
// birth field; statx does, but reading it needs raw syscalls this repo
// avoids per its no-external-deps rule). Callers fall back to ModTime.
func birthTime(_ os.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
