package iterrun

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// StallTicks is how many consecutive quiet heartbeat ticks (no new output,
// no CPU movement) it takes to declare a run stalled. Fixed at 6 ticks of
// the 10s HeartbeatInterval — 60s of genuine inactivity, not a normal gap
// between a tool's own sparse progress lines.
const StallTicks = 6

// HeartbeatInterval is how often the wrapper samples activity and rewrites
// its registry entry. This is a plain ticker in the wrapper process itself,
// not an LLM-driven poll, so a short interval costs nothing.
const HeartbeatInterval = 10 * time.Second

const progressMarker = "##ITERATE-PROGRESS##"

// progressPayload is the optional structured line a tool the AI wrote can
// emit for exact (not guessed) progress reporting.
type progressPayload struct {
	Done    *int     `json:"done"`
	Total   *int     `json:"total"`
	Pct     *float64 `json:"pct"`
	Message string   `json:"message"`
}

// Options configures one iterate-run invocation.
type Options struct {
	Plan string
	Team string
	Unit string
	CWD  string
	Args []string // the wrapped command and its args
	Env  []string // additional env vars for the child, KEY=VALUE
}

// activityState is shared between the output-reading goroutine and the
// heartbeat ticker under a mutex — the only cross-goroutine state here.
type activityState struct {
	mu          sync.Mutex
	newOutput   bool
	lastMessage string
	done        *int
	total       *int
	pct         *float64
}

func (a *activityState) noteRawLine(line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.newOutput = true
	a.lastMessage = line
}

func (a *activityState) noteProgress(p progressPayload) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.newOutput = true
	if p.Message != "" {
		a.lastMessage = p.Message
	}
	if p.Done != nil {
		a.done = p.Done
	}
	if p.Total != nil {
		a.total = p.Total
	}
	if p.Pct != nil {
		a.pct = p.Pct
	}
}

// snapshotAndClear returns whether new output arrived since the last call,
// and clears the flag — this is the per-tick edge detection.
func (a *activityState) snapshotAndClear() (hadOutput bool, message string, done, total *int, pct *float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	hadOutput = a.newOutput
	a.newOutput = false
	return hadOutput, a.lastMessage, a.done, a.total, a.pct
}

// Run executes the wrapped command, streaming progress/heartbeat into a
// registry entry, and prints to its OWN stdout only on state transitions
// worth waking an agent for (start is silent; stall, recovery, and exit are
// not) — routine ticks stay in the registry file, never on stdout, so
// something watching this process's stdout (e.g. the Monitor tool) isn't
// flooded with noise.
func Run(opts Options) error {
	if opts.Unit == "" {
		return fmt.Errorf("--unit is required")
	}
	if len(opts.Args) == 0 {
		return fmt.Errorf("no command given after --")
	}
	dir := RegistryDir(opts.CWD)

	cmd := exec.Command(opts.Args[0], opts.Args[1:]...)
	cmd.Dir = opts.CWD
	cmd.Env = append(os.Environ(), opts.Env...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // interleave; both are just "output" for progress purposes

	logPath := entryPath(dir, &Entry{Plan: opts.Plan, Team: opts.Team, Unit: opts.Unit})
	logPath = strings.TrimSuffix(logPath, ".json") + ".log"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	state := &activityState{}

	if err := cmd.Start(); err != nil {
		return err
	}

	entry := &Entry{
		Plan:          opts.Plan,
		Team:          opts.Team,
		Unit:          opts.Unit,
		Command:       strings.Join(opts.Args, " "),
		PID:           cmd.Process.Pid,
		Started:       time.Now().UTC(),
		LastHeartbeat: time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
		Status:        StatusProgressing,
	}
	_ = entry.Write(dir)

	// Reader goroutine: tee every line to the raw log, and specially parse
	// our own standardized progress marker when a tool (or an AI-written
	// script) emits one.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(logFile, "%s %s\n", time.Now().UTC().Format(time.RFC3339), line)
			if _, after, found := strings.Cut(line, progressMarker); found {
				payload := strings.TrimSpace(after)
				var p progressPayload
				if json.Unmarshal([]byte(payload), &p) == nil {
					state.noteProgress(p)
					continue
				}
			}
			state.noteRawLine(line)
		}
	}()

	lastCPU, cpuErr := cpuTicks(cmd.Process.Pid)
	quietTicks := 0
	wasStalled := false

	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

tickLoop:
	for {
		select {
		case <-ticker.C:
			hadOutput, msg, done, total, pct := state.snapshotAndClear()

			cpuMoved := false
			if cur, err := cpuTicks(cmd.Process.Pid); err == nil {
				if cpuErr == nil && cur > lastCPU {
					cpuMoved = true
				}
				lastCPU, cpuErr = cur, nil
			}

			now := time.Now().UTC()
			entry.LastHeartbeat = now
			if msg != "" {
				entry.LastMessage = msg
			}
			entry.Done, entry.Total, entry.Pct = done, total, pct

			if hadOutput || cpuMoved {
				entry.LastActivity = now
				quietTicks = 0
				entry.Status = StatusProgressing
				if wasStalled {
					fmt.Printf("RESUMED %s: activity detected again\n", unitLabel(opts))
					wasStalled = false
				}
			} else {
				quietTicks++
				if quietTicks >= StallTicks {
					entry.Status = StatusStalled
					if !wasStalled {
						fmt.Printf("ALERT stalled %s: no activity for %s (last: %q)\n",
							unitLabel(opts), time.Duration(quietTicks)*HeartbeatInterval, entry.LastMessage)
						wasStalled = true
					}
				} else {
					entry.Status = StatusQuiet
				}
			}
			_ = entry.Write(dir)

		case err := <-waitCh:
			<-readerDone // make sure the last lines got teed before we finalize
			hadOutput, msg, done, total, pct := state.snapshotAndClear()
			if msg != "" {
				entry.LastMessage = msg
			}
			entry.Done, entry.Total, entry.Pct = done, total, pct
			_ = hadOutput

			now := time.Now().UTC()
			entry.Finished = &now
			code := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					code = exitErr.ExitCode()
				} else {
					code = -1
				}
			}
			entry.ExitCode = &code
			if code == 0 {
				entry.Status = StatusDone
				fmt.Printf("DONE %s exit=0\n", unitLabel(opts))
			} else {
				entry.Status = StatusFailed
				fmt.Printf("FAILED %s exit=%d\n", unitLabel(opts), code)
			}
			_ = entry.Write(dir)
			break tickLoop
		}
	}

	if entry.ExitCode != nil && *entry.ExitCode != 0 {
		return fmt.Errorf("command exited %d", *entry.ExitCode)
	}
	return nil
}

func unitLabel(opts Options) string {
	if opts.Team != "" {
		return fmt.Sprintf("%s/%s/%s", opts.Plan, opts.Team, opts.Unit)
	}
	return fmt.Sprintf("%s/%s", opts.Plan, opts.Unit)
}
