//go:build linux

package iterrun

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// cpuTicks returns a monotonically increasing measure of CPU time consumed
// by pid (utime+stime, in clock ticks, from /proc/<pid>/stat field 14+15).
// A flat reading across samples means the process isn't burning CPU right
// now — combined with no new output, that's the real stall signal.
func cpuTicks(pid int) (int64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// Fields after the comm field (which may itself contain spaces/parens)
	// are space-separated; split on the last ')' to skip past comm safely.
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 || idx+2 >= len(s) {
		return 0, fmt.Errorf("unexpected /proc/%d/stat format", pid)
	}
	fields := strings.Fields(s[idx+2:])
	// After comm: field 3 is state (index 0 here), ... utime is field 14
	// overall, which is index 11 in this post-comm slice; stime is index 12.
	if len(fields) < 13 {
		return 0, fmt.Errorf("short /proc/%d/stat", pid)
	}
	utime, err := strconv.ParseInt(fields[11], 10, 64)
	if err != nil {
		return 0, err
	}
	stime, err := strconv.ParseInt(fields[12], 10, 64)
	if err != nil {
		return 0, err
	}
	return utime + stime, nil
}
