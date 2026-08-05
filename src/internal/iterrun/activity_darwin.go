//go:build darwin

package iterrun

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// cpuTicks returns a monotonically increasing measure of CPU time consumed
// by pid, in centiseconds, parsed from `ps -o time=`. macOS has no /proc,
// so this shells out to ps instead — same purpose as the Linux /proc reader:
// a flat reading across samples means the process isn't burning CPU right
// now, which combined with no new output is the real stall signal.
func cpuTicks(pid int) (int64, error) {
	out, err := exec.Command("ps", "-o", "time=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, err
	}
	return parsePSTime(strings.TrimSpace(string(out)))
}

// parsePSTime parses ps's TIME column, which is [[HH:]MM:]SS[.ss].
func parsePSTime(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty ps time")
	}
	parts := strings.Split(s, ":")
	var h, m int64
	var secStr string
	switch len(parts) {
	case 3:
		h, _ = strconv.ParseInt(parts[0], 10, 64)
		m, _ = strconv.ParseInt(parts[1], 10, 64)
		secStr = parts[2]
	case 2:
		m, _ = strconv.ParseInt(parts[0], 10, 64)
		secStr = parts[1]
	default:
		secStr = parts[0]
	}
	sec, err := strconv.ParseFloat(secStr, 64)
	if err != nil {
		return 0, err
	}
	totalCentisec := (h*3600+m*60)*100 + int64(sec*100)
	return totalCentisec, nil
}
