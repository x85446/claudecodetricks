//go:build darwin

package iterrun

import (
	"os"
	"syscall"
	"time"
)

// birthTime returns the file's creation time on macOS, where it's a real,
// reliably-populated stat field — the best available proxy for "when did
// this team actually start" absent an explicit dispatch timestamp.
func birthTime(fi os.FileInfo) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	bt := st.Birthtimespec
	return time.Unix(bt.Sec, bt.Nsec), true
}
