package iterrun

import (
	"context"
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

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// The launchd timer calls the shared binary supervisor. Detached workers own
// bounded assignments independently of the timer and web process lifetimes.
// launchd supplies the GUI user's Keychain and a provider-capable PATH.

const (
	NightlyLabel        = "com.x85446.iterate-nightly"
	nightlyDefaultEvery = 10 * time.Minute
	nightlyLiveSecsEnv  = "ITERATE_LIVE_SECS"
	nightlyDefaultLive  = 900 * time.Second
)

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
// one is waiting on a human, and a release or release-candidate plan waits
// for a typed /iterate.
func hasStartablePlan(dir string) bool {
	plans, err := ListPlans(dir)
	if err != nil {
		return false
	}
	for _, p := range plans {
		if p.TypedOnly {
			continue
		}
		switch p.StateLabel() {
		case "queued", "unblocked", "executing":
			return true
		}
	}
	return false
}

// decideProject applies the skip order: not enrolled, conductor disabled
// or paused, nothing startable, lock held (a tick's child is running),
// heartbeat live (another session is working here), outside either
// schedule — then launch. The global switch is the
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
	if nightlyLockHeld(dir) {
		d.Reason = "running"
		return d
	}
	if st, err := os.Stat(HeartbeatPath(dir)); err == nil && now.Sub(st.ModTime()) <= live && !ownTrail(dir, st.ModTime()) {
		d.Reason = "live"
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

// ownTrail reports whether a heartbeat is the trail of this project's last
// tick child rather than a session working now. The child's hooks stamp the
// heartbeat until it exits, and its run record's Finished is written after
// that, so a stamp no later than Finished is the child's own. Counting it
// as live skipped every tick inside the live window after a run ended: one
// filemaster step per 16–24 minutes, the rest idle. A stamp after Finished
// is another session, and a run with no Finished (killed before recording)
// leaves the window to decide.
func ownTrail(dir string, beat time.Time) bool {
	lr, ok := loadLastRun(dir)
	if !ok || lr.Finished == "" {
		return false
	}
	fin, err := time.Parse(time.RFC3339, lr.Finished)
	if err != nil {
		return false
	}
	return !beat.After(fin.Add(2 * time.Second)) // Finished is whole seconds
}

// nightlyLockHeld tries the project's lock without blocking: held means a
// previous tick's child is still running there. The lock is advisory
// (flock), released by the OS the moment that child exits or dies, so a
// killed run never leaves a stale lock behind.
func nightlyLockHeld(dir string) bool {
	f, err := os.OpenFile(nightlyLockPath(dir), os.O_RDWR, 0)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
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

// Tick evaluates the shared configuration and starts at most one detached
// binary worker. The supervisor resolves every project and allowance lease.
func Tick(w io.Writer, dryRun bool) error {
	if !LoadNightlyConfig().Enabled {
		if dryRun {
			fmt.Fprintln(w, "disabled")
		} else {
			appendStatus("disabled")
		}
		return nil
	}
	s := NewConductorService("")
	if _, err := os.Stat(filepath.Join(s.Dir, "config.json")); os.IsNotExist(err) {
		fmt.Fprintln(w, "conductor configuration required; preview with iterate-run conductor migrate --legacy")
		return nil
	} else if err != nil {
		return err
	}
	decision, err := s.Status(context.Background())
	if err != nil {
		return err
	}
	if dryRun {
		return json.NewEncoder(w).Encode(decision)
	}
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	if !cfg.Enabled || cfg.Paused {
		appendStatus("conductor disabled or paused")
		return nil
	}
	// A worker also refreshes telemetry before deciding admission. A stale
	// snapshot cannot prevent the next authentic observation from being read.
	if len(cfg.Projects) == 0 {
		return nil
	}
	pid, err := StartConductorWorker(s.Dir, "")
	if err != nil {
		return err
	}
	appendStatus(fmt.Sprintf("binary worker %d", pid))
	return nil
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

// RunOne preserves the scheduled entrypoint and delegates a configured
// project's bounded work to the same supervisor used by CLI and web clients.
func RunOne(dir string) error {
	canonical, err := conductor.Canonical(dir)
	if err != nil {
		return err
	}
	s := NewConductorService("")
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	for _, p := range cfg.Projects {
		if p.Path == canonical {
			_, err = s.TickProject(context.Background(), p.ID)
			return err
		}
	}
	return fmt.Errorf("project is not configured; use conductor project add or conductor migrate --legacy")
}

// Enroll registers a disabled project without creating repository files.
// Account bindings and automations remain explicit configuration choices.
func Enroll(dir string) error {
	canonical, err := conductor.Canonical(dir)
	if err != nil {
		return err
	}
	s := NewConductorService("")
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	for _, p := range cfg.Projects {
		if p.Path == canonical {
			return nil
		}
	}
	cfg.Projects = append(cfg.Projects, model.Project{ID: legacyID(canonical), Name: filepath.Base(canonical), Path: canonical})
	_, err = s.Apply(cfg.Revision, cfg)
	if err != nil {
		return err
	}
	RegisterProject(canonical)
	return nil
}

// Withdraw disables a registered project while retaining its queue and rules.
func Withdraw(dir string) error {
	canonical, err := conductor.Canonical(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	s := NewConductorService("")
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	for i, p := range cfg.Projects {
		if p.Path == canonical {
			if !p.Enabled {
				return nil
			}
			cfg.Projects[i].Enabled = false
			_, err = s.Apply(cfg.Revision, cfg)
			return err
		}
	}
	return nil
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
	s := NewConductorService("")
	decision, err := s.Status(context.Background())
	if err != nil {
		fmt.Fprintf(w, "conductor error\t%s\n", err)
		return
	}
	fmt.Fprintf(w, "conductor\t%s\trevision %d\n", decision.State, decision.Revision)
	for _, q := range decision.Queues {
		fmt.Fprintf(w, "project\t%s\tqueued %d\tready %d\twaiting %d\trunning %d\n", q.ProjectID, q.Depth, q.Ready, q.Waiting, q.Running)
	}
}
