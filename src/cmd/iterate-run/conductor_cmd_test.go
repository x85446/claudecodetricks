package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/iterrun"
)

// TESTMASTER: id=indigobird-cli-interspersed-flags tier=? parallel=yes
func TestConductorCLIInterspersedFlags(t *testing.T) {
	dir := t.TempDir()
	inputs := [][]string{{"--json", "conductor", "--store", dir, "config", "show"}, {"conductor", "config", "--json", "show", "--store", dir}, {"conductor", "config", "show", "--store", dir, "--json"}}
	var previous string
	for _, args := range inputs {
		parsed, ok := conductorInvocation(args)
		if !ok {
			t.Fatal("not dispatched", args)
		}
		var out, diag bytes.Buffer
		if code := runConductorCLI(parsed, strings.NewReader(""), &out, &diag); code != 0 {
			t.Fatalf("exit %d: %s", code, &diag)
		}
		if previous != "" && out.String() != previous {
			t.Fatal("flag positions changed output")
		}
		previous = out.String()
		if !json.Valid(out.Bytes()) {
			t.Fatal(out.String())
		}
	}
}

// TESTMASTER: id=indigobird-cli-http-shared-definitions tier=fast parallel=yes
func TestConductorCLIHTTPSharedDefinitions(t *testing.T) {
	store := t.TempDir()
	service := iterrun.NewConductorService(store)
	initial, err := service.Config()
	if err != nil {
		t.Fatal(err)
	}
	sub := initial.Subscriptions[0]
	seq := sub.Sequences[0]
	invoke := func(args []string, input string, want int) json.RawMessage {
		t.Helper()
		var out, diag bytes.Buffer
		args = append(args, "--store", store, "--json")
		code := runConductorCLI(args, strings.NewReader(input), &out, &diag)
		if code != want || diag.Len() != 0 {
			t.Fatalf("%v: exit %d stdout=%s stderr=%s", args, code, &out, &diag)
		}
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatal(err, out.String())
		}
		return env.Data
	}
	args := []string{"sequence", "edit", seq.ID, "--subscription", sub.ID, "--file", "-", "--revision", "0"}
	invoke(args, `{"name":"custom suffix","model":"custom-model","reasoning":"high","args":["--add-dir","folder with spaces","--add-dir","$(touch never)"]}`, 0)
	r := httptest.NewRequest("GET", "http://localhost:8420/api/conductor/config", nil)
	w := httptest.NewRecorder()
	iterrun.ConductorAPI(service).ServeHTTP(w, r)
	var env struct {
		Data model.Config `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || w.Code != 200 {
		t.Fatalf("HTTP config %d %s (%v)", w.Code, w.Body, err)
	}
	if env.Data.Revision != 1 || env.Data.Subscriptions[0].Sequences[0].ID != seq.ID || env.Data.Subscriptions[0].Sequences[0].Name != "custom suffix" {
		t.Fatal("CLI write missing in HTTP", w.Body.String())
	}
	compiled := invoke([]string{"sequence", "preview", seq.ID, "--subscription", sub.ID}, "", 0)
	var preview struct {
		Argv []string `json:"argv"`
	}
	if err := json.Unmarshal(compiled, &preview); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(preview.Argv, "\x00")
	if !strings.Contains(joined, "custom-model") || !strings.Contains(joined, "folder with spaces\x00--add-dir\x00$(touch never)") {
		t.Fatal("preview lost literal argument boundaries", string(compiled))
	}
	// HTTP edits and stale CLI writes share one revision guard.
	env.Data.Subscriptions[0].Name = "Renamed account"
	body, _ := json.Marshal(map[string]any{"expected_revision": 1, "config": env.Data})
	r = httptest.NewRequest("POST", "http://localhost:8420/api/conductor/config", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:8420")
	w = httptest.NewRecorder()
	iterrun.ConductorAPI(service).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	invoke(args, `{"name":"stale"}`, 3)
	read := invoke([]string{"config", "show"}, "", 0)
	var saved model.Config
	if err := json.Unmarshal(read, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 || model.SequenceName(saved.Subscriptions[0], saved.Subscriptions[0].Sequences[0]) != "Renamed account · custom suffix" {
		t.Fatal("shared prefix/revision mismatch", string(read))
	}
	for _, noun := range []string{"project", "subscription", "sequence", "preset", "power-band", "time-window", "overdrive", "automation"} {
		list := []string{noun, "list"}
		if noun == "sequence" || noun == "preset" || noun == "overdrive" {
			list = append(list, "--subscription", sub.ID)
		}
		data := invoke(list, "", 0)
		var records []json.RawMessage
		if err := json.Unmarshal(data, &records); err != nil {
			t.Fatalf("%s list: %s", noun, data)
		}
	}
}

// TESTMASTER: id=indigobird-cli-six-color-cases tier=? parallel=yes
func TestConductorCLISixColorCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY fixture requires POSIX")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("PTY fixture requires python3")
	}
	binary, err := filepath.Abs("../../../bin/iterate-run")
	if err != nil {
		t.Fatal(err)
	}
	clean := []string{}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		switch k {
		case "TERM", "CI", "NO_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE", "ITERATE_RUN_COLOR", "ITERATE_RUN_NO_COLOR":
		default:
			clean = append(clean, v)
		}
	}
	ptyScript := `import os,pty,subprocess,sys
master,slave=pty.openpty()
p=subprocess.Popen(sys.argv[1:],stdin=subprocess.DEVNULL,stdout=slave,stderr=subprocess.PIPE)
os.close(slave)
data=b''
while True:
 try:
  chunk=os.read(master,4096)
  if not chunk:break
  data+=chunk
 except OSError:break
os.close(master)
_,err=p.communicate()
sys.stdout.buffer.write(data)
sys.stderr.buffer.write(err)
sys.exit(p.returncode)
`
	for _, tc := range []struct {
		name       string
		pty, color bool
		env, args  []string
	}{
		{"pipe-auto", false, false, []string{"TERM=xterm-256color"}, nil},
		{"pipe-force", false, true, []string{"TERM=xterm-256color", "FORCE_COLOR=1"}, nil},
		{"no-color-over-force", false, false, []string{"TERM=xterm-256color", "FORCE_COLOR=1", "NO_COLOR=1"}, nil},
		{"explicit-over-no-color", false, true, []string{"TERM=xterm-256color", "NO_COLOR=1"}, []string{"--color=always"}},
		{"terminal-auto", true, true, []string{"TERM=xterm-256color"}, nil},
		{"terminal-dumb", true, false, []string{"TERM=dumb"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"conductor", "--help"}, tc.args...)
			cmd := exec.Command(binary, args...)
			if tc.pty {
				cmd = exec.Command(python, append([]string{"-c", ptyScript, binary}, args...)...)
			}
			cmd.Env = append(append([]string{}, clean...), tc.env...)
			out, e := cmd.CombinedOutput()
			if e != nil {
				t.Fatalf("%v: %s", e, out)
			}
			if bytes.Contains(out, []byte("\x1b[")) != tc.color {
				t.Fatalf("color expected %v: %q", tc.color, out)
			}
			if !bytes.Contains(out, []byte("Usage:")) {
				t.Fatal("plain meaning missing", string(out))
			}
		})
	}
	cmd := exec.Command(binary, "conductor", "config", "show", "--store", t.TempDir(), "--json", "--color=always")
	cmd.Env = append(clean, "FORCE_COLOR=1", "TERM=xterm-256color")
	out, e := cmd.Output()
	if e != nil || !json.Valid(out) || bytes.Contains(out, []byte("\x1b[")) {
		t.Fatalf("JSON purity: %v %s", e, out)
	}
}

// TESTMASTER: id=indigobird-cli-atomic-json-workflow tier=? parallel=yes
func TestConductorCLIAtomicJSONWorkflow(t *testing.T) {
	store := t.TempDir()
	project := t.TempDir()
	var out, diag bytes.Buffer
	body := fmt.Sprintf(`{"id":"project-a","path":%q,"enabled":true}`, project)
	args := []string{"project", "--store", store, "create", "--file", "-", "--revision", "0", "--json"}
	code := runConductorCLI(args, strings.NewReader(body), &out, &diag)
	if code != 0 {
		t.Fatalf("create %d %s %s", code, &out, &diag)
	}
	var env struct {
		OK       bool  `json:"ok"`
		Revision int64 `json:"revision"`
		Data     struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	if e := json.Unmarshal(out.Bytes(), &env); e != nil {
		t.Fatal(e)
	}
	if !env.OK || env.Revision != 1 || env.Data.Enabled {
		t.Fatal(out.String())
	}
	out.Reset()
	diag.Reset()
	code = runConductorCLI(args, strings.NewReader(body), &out, &diag)
	if code != 3 || !json.Valid(out.Bytes()) {
		t.Fatalf("conflict %d: %s", code, &out)
	}
	if diag.Len() != 0 {
		t.Fatal("machine error mixed diagnostics", diag.String())
	}
}

// TESTMASTER: id=indigobird-cli-errors-help tier=? parallel=yes
func TestConductorCLIErrorsAndHelp(t *testing.T) {
	for _, args := range [][]string{{"status", "--revision"}, {"status", "--unknown"}, {"not-a-command"}} {
		var out, diag bytes.Buffer
		args = append([]string{"--store", t.TempDir(), "--json"}, args...)
		if code := runConductorCLI(args, strings.NewReader(""), &out, &diag); code != 2 {
			t.Fatalf("%v code %d: %s", args, code, &out)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatal(out.String())
		}
	}
	var out, diag bytes.Buffer
	if code := runConductorCLI([]string{"sequence", "edit", "--help"}, strings.NewReader(""), &out, &diag); code != 0 || !strings.Contains(out.String(), "Help context: sequence edit") {
		t.Fatal(code, out.String(), diag.String())
	}
	f, e := parseConductorFlags([]string{"sequence", "show", "--", "--literal"})
	if e != nil || len(f.words) != 3 || f.words[2] != "--literal" {
		t.Fatal("terminator lost", f, e)
	}
}

// TESTMASTER: id=indigobird-cli-installed-entrypoint tier=? parallel=yes
func TestConductorCLIBinaryEntrypoint(t *testing.T) {
	binary, e := filepath.Abs("../../../bin/iterate-run")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(binary); e != nil {
		t.Fatal("build iterate-run with make bin/iterate-run before acceptance", e)
	}
	store := t.TempDir()
	cmd := exec.Command(binary, "--json", "--store", store, "conductor", "config", "show")
	b, e := cmd.CombinedOutput()
	if e != nil || !json.Valid(b) {
		t.Fatalf("binary JSON entrypoint %v: %s", e, b)
	}
	cmd = exec.Command(binary, "conductor", "tick", "--store", store, "--json")
	b, e = cmd.CombinedOutput()
	exit := 0
	if e != nil {
		if ee, ok := e.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			t.Fatal(e)
		}
	}
	if exit != 4 || !json.Valid(b) {
		t.Fatalf("gated binary tick exit %d: %s", exit, b)
	}
	cmd = exec.Command(binary, "conductor", "--help", "--color=always")
	b, e = cmd.Output()
	if e != nil || !bytes.Contains(b, []byte("\x1b[")) {
		t.Fatalf("explicit color missing: %v %q", e, b)
	}
	cmd = exec.Command(binary, "conductor", "--help", "--color=never")
	b, e = cmd.Output()
	if e != nil || bytes.Contains(b, []byte("\x1b[")) {
		t.Fatal("plain help contains escapes", e)
	}
}

// TESTMASTER: id=indigobird-cli-project-queue tier=? parallel=yes
func TestConductorCLIProjectQueue(t *testing.T) {
	store := t.TempDir()
	dir := t.TempDir()
	plans := filepath.Join(dir, ".claude", "iterate", "plans")
	if e := os.MkdirAll(plans, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(plans, "queued.md"), []byte("name: queued\nphase: planned\nstatus: queued\n\n## Goal\nQueued fixture\n"), 0600); e != nil {
		t.Fatal(e)
	}
	var out, diag bytes.Buffer
	body := fmt.Sprintf(`{"id":"p","path":%q}`, dir)
	if code := runConductorCLI([]string{"project", "create", "--store", store, "--file", "-", "--revision", "0", "--json"}, strings.NewReader(body), &out, &diag); code != 0 {
		t.Fatal(code, out.String(), diag.String())
	}
	out.Reset()
	if code := runConductorCLI([]string{"project", "show", "p", "--store", store, "--json"}, strings.NewReader(""), &out, &diag); code != 0 {
		t.Fatal(code, out.String(), diag.String())
	}
	var env struct {
		Data struct {
			Queue struct {
				Depth int `json:"depth"`
				Plans []struct {
					Name string `json:"name"`
				} `json:"plans"`
			} `json:"queue"`
		} `json:"data"`
	}
	if e := json.Unmarshal(out.Bytes(), &env); e != nil {
		t.Fatal(e)
	}
	if env.Data.Queue.Depth != 1 || len(env.Data.Queue.Plans) != 1 || env.Data.Queue.Plans[0].Name != "queued" {
		t.Fatal(out.String())
	}
}
