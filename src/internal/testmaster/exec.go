package testmaster

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// proc is one finished child process.
type proc struct {
	Stdout      []byte
	Stderr      []byte // empty when combined: everything lands in Stdout
	Exit        int
	Elapsed     time.Duration
	TimedOut    bool
	Interrupted bool
	StartErr    error
}

// Failed reports whether the process did not exit 0 for any reason.
func (r proc) Failed() bool { return r.Exit != 0 || r.TimedOut || r.Interrupted || r.StartErr != nil }

// runProc runs argv in dir with stdin closed. The child gets its own process
// group so a timeout or Ctrl-C kills everything it spawned (make → bash →
// the test), not just the first process.
func runProc(ctx context.Context, dir string, env []string, timeout time.Duration, combined bool, argv ...string) proc {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(tctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if combined {
		cmd.Stderr = &stdout
	} else {
		cmd.Stderr = &stderr
	}
	start := time.Now()
	err := cmd.Run()
	r := proc{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Elapsed: time.Since(start)}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		r.Exit = ee.ExitCode()
		if r.Exit < 0 {
			r.Exit = 128 + int(syscall.SIGKILL)
		}
	default:
		if cmd.ProcessState == nil {
			r.StartErr = err
			r.Exit = -1
		} else {
			r.Exit = cmd.ProcessState.ExitCode()
		}
	}
	if ctx.Err() != nil {
		r.Interrupted = true
	} else if tctx.Err() != nil {
		r.TimedOut = true
	}
	return r
}

// shellQuote renders argv as a copy-pasteable command line.
func shellQuote(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && strings.IndexFunc(a, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:@+,%", r))
		}) < 0 {
			out[i] = a
			continue
		}
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}

// withDir prefixes a repro command with a cd when the test runs elsewhere.
func withDir(dir, cmd string) string {
	if dir == "" || dir == "." {
		return cmd
	}
	return "cd " + shellQuote([]string{dir}) + " && " + cmd
}

// unitTimeout gives each invocation room for what it has been measured to
// need: three times the summed average, never less than the floor.
func unitTimeout(floor time.Duration, entries []*Entry) time.Duration {
	var sum int64
	for _, e := range entries {
		if e.AvgMs != nil {
			sum += *e.AvgMs
		}
	}
	if d := 3 * time.Duration(sum) * time.Millisecond; d > floor {
		return d
	}
	return floor
}
