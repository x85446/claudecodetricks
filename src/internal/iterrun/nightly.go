package iterrun

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Package iterrun, nightly: the launchd-driven tick that runs enrolled
// projects' staged plans while nobody is at the keyboard.
//
// The design is stateless on purpose. A LaunchAgent fires `iterate-run
// nightly tick` every few minutes; each tick walks the registered projects
// and, for every one that is enrolled, allowed by its schedules, and not
// already being driven, spawns one detached `claude -p "/iterate-conductor
// run"` in that project. The conductor does the rest — picks the queued
// plan, lets /iterate drive it, handles the ending — and the next tick
// resumes whatever is still executing. Nothing here remembers a run
// between ticks except the files the run itself leaves behind: the plan,
// its heartbeat, the per-project lock, and the result JSON.
//
// launchd, never cron: the gui domain has the user's Keychain (claude's
// OAuth token lives there), runs a missed interval on wake, and needs no
// Full Disk Access grant for ~/workspace. ProcessType is Standard, not
// Background — confirmed live: under Background the Bun runtime logged 44s
// of event-loop stalls, the Keychain read timed out, and claude fell back
// to a stale credentials file and reported "OAuth session expired".

const (
	NightlyLabel        = "com.x85446.iterate-nightly"
	nightlyDefaultEvery = 10 * time.Minute
	nightlyLiveSecsEnv  = "ITERATE_LIVE_SECS"
	nightlyDefaultLive  = 900 * time.Second
)

// conductorPrompt is what every child runs. Never `--bare`: the hooks are
// what stamp the heartbeat the next tick reads.
var conductorArgs = []string{"-p", "/iterate-conductor run", "--permission-mode", "auto", "--output-format", "json"}

// NightlyConfig is ~/.claude/iterate-run/nightly.json — the global switch.
type NightlyConfig struct {
	Enabled bool   `json:"enabled"`
	Every   string `json:"every,omitempty"`
	Updated string `json:"updated,omitempty"`
}

func NightlyConfigPath() string { return filepath.Join(StoreDir(), "nightly.json") }
func NightlyLockDir() string    { return filepath.Join(StoreDir(), "nightly") }
func NightlyLogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".claude", "log", "iterate-nightly")
}
func NightlyPlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "Library", "LaunchAgents", NightlyLabel+".plist")
}

// LoadNightlyConfig reads the switch. No file means enabled: installing
// the agent is the opt-in, `nightly off` is the explicit opt-out.
func LoadNightlyConfig() NightlyConfig {
	cfg := NightlyConfig{Enabled: true}
	data, err := os.ReadFile(NightlyConfigPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func SaveNightlyConfig(cfg NightlyConfig) error {
	cfg.Updated = time.Now().UTC().Format(time.RFC3339)
	if err := os.MkdirAll(StoreDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(NightlyConfigPath(), append(data, '\n'), 0o644)
}

// projectKeyName is the per-project file stem shared by the heartbeat,
// the lock and the result record: the absolute path with separators
// turned into dashes, same encoding as HeartbeatPath.
func projectKeyName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return strings.ReplaceAll(abs, string(os.PathSeparator), "-")
}

func nightlyLockPath(dir string) string {
	return filepath.Join(NightlyLockDir(), projectKeyName(dir)+".lock")
}

func nightlyLastRunPath(dir string) string {
	return filepath.Join(NightlyLockDir(), projectKeyName(dir)+".json")
}

// liveWindow is how fresh a heartbeat must be for the project to count as
// driven by someone else — the same ITERATE_LIVE_SECS the status line uses.
func liveWindow() time.Duration {
	if v := os.Getenv(nightlyLiveSecsEnv); v != "" {
		if secs, err := time.ParseDuration(v + "s"); err == nil && secs > 0 {
			return secs
		}
	}
	return nightlyDefaultLive
}

// TickDecision is one project's verdict for one tick.
type TickDecision struct {
	Dir    string
	Launch bool
	Reason string // skip reason, or "" when launching
	Next   time.Time
}

func (d TickDecision) String() string {
	short := filepath.Base(d.Dir)
	if d.Launch {
		return short + ": launch"
	}
	s := short + ": skip: " + d.Reason
	if !d.Next.IsZero() {
		s += ", next window " + d.Next.Local().Format("2006-01-02 15:04")
	}
	return s
}

// hasStartablePlan reports whether anything in plans/ could be launched
// unattended: a queued plan, an unblocked one, or an executing one whose
// run died (its heartbeat is stale — the caller has already checked that).
// A bare planned plan is never startable unattended; a blocked or paused
// one is waiting on a human.
func hasStartablePlan(dir string) bool {
	plans, err := ListPlans(dir)
	if err != nil {
		return false
	}
	for _, p := range plans {
		switch p.StateLabel() {
		case "queued", "unblocked", "executing":
			return true
		}
	}
	return false
}

// decideProject applies the skip order the plan fixed: not enrolled,
// conductor disabled or paused, nothing startable, heartbeat live, lock
// held, outside either schedule — then launch. The global switch is the
// tick's own first check, before any project is looked at.
func decideProject(dir string, now time.Time, live time.Duration) TickDecision {
	d := TickDecision{Dir: dir}
	cs, ok := ReadConductorState(dir)
	if !ok || cs.TickSource != "launchd" {
		d.Reason = "not enrolled"
		return d
	}
	if !cs.Enabled {
		d.Reason = "conductor disabled"
		return d
	}
	if cs.Paused {
		d.Reason = "conductor paused"
		return d
	}
	if !hasStartablePlan(dir) {
		d.Reason = "nothing queued"
		return d
	}
	if st, err := os.Stat(HeartbeatPath(dir)); err == nil && now.Sub(st.ModTime()) <= live {
		d.Reason = "live"
		return d
	}
	if nightlyLockHeld(dir) {
		d.Reason = "running"
		return d
	}
	launch, err := ReadLaunchSchedule(dir)
	if err != nil {
		d.Reason = "outside launch-schedule (" + err.Error() + ")"
		return d
	}
	conductor, err := ReadConductorSchedule(dir)
	if err != nil {
		d.Reason = "outside conductor-schedule (" + err.Error() + ")"
		return d
	}
	if ok, rule, next := AllowedAll(now, launch, conductor); !ok {
		name, why, _ := strings.Cut(rule, ": ")
		d.Reason = "outside " + name + " (" + why + ")"
		d.Next = next
		return d
	}
	d.Launch = true
	return d
}

// nightlyLockHeld tries the project's lock without blocking: held means a
// previous tick's child is still running there. The lock is advisory
// (flock), released by the OS the moment that child exits or dies, so a
// killed run never leaves a stale lock behind.
func nightlyLockHeld(dir string) bool {
	unlock, err := tryNightlyLock(dir)
	if err != nil {
		return true
	}
	unlock()
	return false
}

func tryNightlyLock(dir string) (unlock func(), err error) {
	if err := os.MkdirAll(NightlyLockDir(), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(nightlyLockPath(dir), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// appendStatus adds one timestamped line to the nightly status log.
func appendStatus(line string) {
	if err := os.MkdirAll(NightlyLogDir(), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(NightlyLogDir(), "status.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), line)
}

// enrolledProjects is every registered project whose conductor.md names
// launchd as its tick — the only ones a tick reports on, so the status log
// is not 27 lines of "not enrolled" every ten minutes.
func enrolledProjects() []string {
	known, _ := ListProjects()
	var out []string
	for _, dir := range known {
		if cs, ok := ReadConductorState(dir); ok && cs.TickSource == "launchd" {
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}

// Tick is `iterate-run nightly tick [--dry-run]`. It decides every enrolled
// project, launches a detached runner for each launchable one, and exits —
// it never waits on a child, so launchd's next interval fires on time even
// while one project's run takes hours. Output is machine-first: one line
// per enrolled project, nothing when there is nothing to say.
func Tick(w io.Writer, dryRun bool) error {
	now := time.Now()
	if !LoadNightlyConfig().Enabled {
		if dryRun {
			fmt.Fprintln(w, "disabled")
		} else {
			appendStatus("disabled")
		}
		return nil
	}
	live := liveWindow()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	for _, dir := range enrolledProjects() {
		d := decideProject(dir, now, live)
		if dryRun {
			if d.Launch {
				fmt.Fprintf(w, "would launch %s\n", dir)
			} else {
				fmt.Fprintln(w, d.String())
			}
			continue
		}
		if !d.Launch {
			appendStatus(d.String())
			continue
		}
		if err := spawnRunner(self, dir); err != nil {
			appendStatus(filepath.Base(dir) + ": launch failed: " + err.Error())
			continue
		}
		appendStatus(filepath.Base(dir) + ": launched")
	}
	return nil
}

// spawnRunner starts `iterate-run nightly run-one <dir>` in its own
// session so the tick can exit while the conductor keeps running.
func spawnRunner(self, dir string) error {
	cmd := exec.Command(self, "nightly", "run-one", dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// LastRun is the per-project record `nightly status` reads.
type LastRun struct {
	Dir      string `json:"dir"`
	Launched string `json:"launched"`
	Finished string `json:"finished,omitempty"`
	Exit     *int   `json:"exit,omitempty"`
	Result   string `json:"result,omitempty"`
	IsError  *bool  `json:"is_error,omitempty"`
}

func saveLastRun(lr LastRun) {
	data, err := json.MarshalIndent(lr, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(nightlyLastRunPath(lr.Dir), append(data, '\n'), 0o644)
}

func loadLastRun(dir string) (LastRun, bool) {
	data, err := os.ReadFile(nightlyLastRunPath(dir))
	if err != nil {
		return LastRun{}, false
	}
	var lr LastRun
	if json.Unmarshal(data, &lr) != nil {
		return LastRun{}, false
	}
	return lr, true
}

// RunOne is the detached runner: take the project's lock, spawn the
// conductor, wait, record. A lock already held means a sibling runner won
// the race — exit quietly, that is the normal outcome of two ticks
// overlapping.
func RunOne(dir string) error {
	unlock, err := tryNightlyLock(dir)
	if err != nil {
		return nil
	}
	defer unlock()
	claude, err := exec.LookPath("claude")
	if err != nil {
		appendStatus(filepath.Base(dir) + ": launch failed: claude not on PATH")
		return err
	}
	stamp := time.Now().UTC()
	logDir := filepath.Join(NightlyLogDir(), projectKeyName(dir))
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(logDir, stamp.Format("20060102T150405Z")+".json")
	errPath := strings.TrimSuffix(outPath, ".json") + ".err"
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	errf, err := os.Create(errPath)
	if err != nil {
		return err
	}
	defer errf.Close()

	lr := LastRun{Dir: dir, Launched: stamp.Format(time.RFC3339), Result: outPath}
	saveLastRun(lr)

	cmd := exec.Command(claude, conductorArgs...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = out, errf
	cmd.Stdin = nil
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		code = -1
		if ee, ok := runErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	lr.Exit = &code
	lr.Finished = time.Now().UTC().Format(time.RFC3339)
	if data, err := os.ReadFile(outPath); err == nil {
		var res struct {
			IsError bool `json:"is_error"`
		}
		if json.Unmarshal(data, &res) == nil {
			lr.IsError = &res.IsError
		}
	}
	saveLastRun(lr)
	appendStatus(fmt.Sprintf("%s: launched %s → exit %d (%s)", filepath.Base(dir), lr.Launched, code, time.Since(stamp).Round(time.Second)))
	return runErr
}

// Enroll marks a project as ticked by launchd: `tick-source: launchd` in
// its conductor.md frontmatter, the file created with `enabled: true`
// when absent. Also registers the project so the tick can find it.
func Enroll(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	it := filepath.Join(abs, ".claude", "iterate")
	if err := os.MkdirAll(filepath.Join(it, "plans"), 0o755); err != nil {
		return err
	}
	RegisterProject(abs)
	path := filepath.Join(it, "conductor.md")
	data, err := os.ReadFile(path)
	if err != nil {
		body := "---\nenabled: true\npaused: false\ncron:\ntick-source: launchd\ncurrent:\ntick: working\n---\n\n# Conductor — " + filepath.Base(abs) +
			"\n\nEnrolled in the nightly launchd tick by `iterate-run nightly enroll` on " + time.Now().UTC().Format("2006-01-02") + ".\n\n## Sweep log\n"
		return os.WriteFile(path, []byte(body), 0o644)
	}
	return os.WriteFile(path, []byte(setFrontmatterKey(string(data), "tick-source", "launchd")), 0o644)
}

// Withdraw removes the project's `tick-source:` line; the conductor goes
// back to ticking itself (or to nothing, if it was never started).
func Withdraw(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(abs, ".claude", "iterate", "conductor.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return os.WriteFile(path, []byte(setFrontmatterKey(string(data), "tick-source", "")), 0o644)
}

// setFrontmatterKey sets (or, with an empty value, removes) one `key:` line
// inside the file's `---` block, adding the block when the file has none.
func setFrontmatterKey(content, key, value string) string {
	lines := strings.Split(content, "\n")
	start, end := -1, -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "---" {
			if start < 0 {
				start = i
				continue
			}
			end = i
			break
		}
	}
	if start < 0 || end < 0 {
		block := "---\n"
		if value != "" {
			block += key + ": " + value + "\n"
		}
		return block + "---\n" + content
	}
	var out []string
	done := false
	for i, l := range lines {
		if i > start && i < end && strings.HasPrefix(l, key+":") {
			if value != "" && !done {
				out = append(out, key+": "+value)
			}
			done = true
			continue
		}
		if i == end && !done && value != "" {
			out = append(out, key+": "+value)
			done = true
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// PlistXML renders the LaunchAgent. program is the iterate-run binary to
// run; every is the StartInterval; path is the PATH the tick and its
// claude children see (launchd gives almost none).
func PlistXML(program string, every time.Duration, path string) string {
	home, _ := os.UserHomeDir()
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array>
    <string>%s</string><string>nightly</string><string>tick</string>
  </array>
  <key>StartInterval</key><integer>%d</integer>
  <key>RunAtLoad</key><false/>
  <key>ProcessType</key><string>Standard</string>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>%s</string>
    <key>HOME</key><string>%s</string>
    <key>LANG</key><string>en_US.UTF-8</string>
    <key>TERM</key><string>dumb</string>
  </dict>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, NightlyLabel, program, int(every.Seconds()), path, home,
		filepath.Join(NightlyLogDir(), "launchd.out"), filepath.Join(NightlyLogDir(), "launchd.err"))
}

// tickPath is the PATH baked into the plist: wherever claude and
// iterate-run live right now, then the usual places.
func tickPath() string {
	seen := map[string]bool{}
	var parts []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			parts = append(parts, p)
		}
	}
	if c, err := exec.LookPath("claude"); err == nil {
		add(filepath.Dir(c))
	}
	if self, err := os.Executable(); err == nil {
		add(filepath.Dir(self))
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, "go", "bin"), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"} {
		add(p)
	}
	return strings.Join(parts, ":")
}

func launchctl(args ...string) ([]byte, error) {
	return exec.Command("launchctl", args...).CombinedOutput()
}

func guiDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// Install writes the plist, (re)bootstraps it, and records the interval.
func Install(every time.Duration) error {
	if every <= 0 {
		every = nightlyDefaultEvery
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	if err := os.MkdirAll(NightlyLogDir(), 0o755); err != nil {
		return err
	}
	plist := NightlyPlistPath()
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plist, []byte(PlistXML(self, every, tickPath())), 0o644); err != nil {
		return err
	}
	_, _ = launchctl("bootout", guiDomain()+"/"+NightlyLabel)
	if out, err := launchctl("bootstrap", guiDomain(), plist); err != nil {
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
	}
	cfg := LoadNightlyConfig()
	cfg.Every = every.String()
	return SaveNightlyConfig(cfg)
}

// Uninstall boots the agent out and removes the plist. Enrollments and
// the switch are left as they are — reinstalling picks them straight up.
func Uninstall() error {
	_, _ = launchctl("bootout", guiDomain()+"/"+NightlyLabel)
	err := os.Remove(NightlyPlistPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Installed reports whether launchd currently has the agent loaded.
func Installed() bool {
	_, err := launchctl("print", guiDomain()+"/"+NightlyLabel)
	return err == nil
}

// Status is `iterate-run nightly status`: one fact per line.
func Status(w io.Writer) {
	cfg := LoadNightlyConfig()
	if _, err := os.Stat(NightlyPlistPath()); err != nil {
		fmt.Fprintln(w, "not installed")
	} else {
		loaded := "not loaded"
		if Installed() {
			loaded = "loaded"
		}
		every := cfg.Every
		if every == "" {
			every = nightlyDefaultEvery.String()
		}
		fmt.Fprintf(w, "installed\t%s\tevery %s\t%s\n", NightlyPlistPath(), every, loaded)
	}
	fmt.Fprintf(w, "enabled\t%v\n", cfg.Enabled)
	now := time.Now()
	for _, dir := range enrolledProjects() {
		last := "never"
		if lr, ok := loadLastRun(dir); ok {
			last = "launched " + lr.Launched
			if lr.Exit != nil {
				last += fmt.Sprintf(" exit %d", *lr.Exit)
			} else {
				last += " running"
			}
		}
		d := decideProject(dir, now, liveWindow())
		next := "now"
		if !d.Launch {
			next = d.Reason
			if !d.Next.IsZero() {
				next += " until " + d.Next.Local().Format("2006-01-02 15:04")
			}
		}
		fmt.Fprintf(w, "project\t%s\t%s\t%s\n", dir, last, next)
	}
}
