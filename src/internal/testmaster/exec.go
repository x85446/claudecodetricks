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

// killedExit is the exit code a killed test reports on every OS: 128 + SIGKILL,
// what a shell prints for a process killed by signal 9.
const killedExit = 128 + int(syscall.SIGKILL)

// runProc runs argv in dir with stdin closed. The child and everything it
// spawns (make → bash → the test) form one tree, so a timeout or Ctrl-C kills
// all of it, not just the first process: a process group on Unix, a Job
// Object on Windows.
func runProc(ctx context.Context, dir string, env []string, timeout time.Duration, combined bool, argv ...string) proc {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(tctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	tree := prepareTree(cmd)
	defer tree.close()
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if combined {
		cmd.Stderr = &stdout
	} else {
		cmd.Stderr = &stderr
	}
	start := time.Now()
	err := cmd.Start()
	if err == nil {
		if terr := tree.started(); terr != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return proc{Exit: -1, StartErr: terr, Elapsed: time.Since(start)}
		}
		err = cmd.Wait()
	}
	r := proc{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Elapsed: time.Since(start)}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		r.Exit = ee.ExitCode()
		if r.Exit < 0 {
			r.Exit = killedExit
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
