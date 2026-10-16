package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func script(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "executor")
	if e := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0700); e != nil {
		t.Fatal(e)
	}
	return p
}

// TESTMASTER: id=indigobird-editable-profiles-argv tier=fast parallel=yes
func TestCompileLiteralExecutor(t *testing.T) {
	for _, adapter := range []string{"codex", "claude"} {
		t.Run(adapter, func(t *testing.T) {
			exe := script(t, "printf '%s\\0' \"$@\"\n")
			s := model.Subscription{Executable: exe, Adapter: adapter, Presets: []model.Preset{{ID: "custom", Args: []string{"--tag", "one", "--tag", "a b", "--literal", "$(touch /tmp/no-conductor-test)"}}}}
			q := model.Sequence{PresetID: "custom", Args: []string{"--another", "'quoted'"}}
			argv, e := Compile(s, q, model.Assignment{Prompt: "literal prompt", ProjectPath: "/project with spaces", ResultPath: "/result with spaces"})
			if e != nil {
				t.Fatal(e)
			}
			out, e := exec.Command(argv[0], argv[1:]...).Output()
			if e != nil {
				t.Fatal(e)
			}
			got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
			if !reflect.DeepEqual(got, argv[1:]) {
				t.Fatalf("argv mismatch: %#v", got)
			}
			if !strings.Contains(CommandPreview(argv), "'a b'") {
				t.Fatal("preview loses literal token")
			}
		})
	}
}

// TESTMASTER: id=indigobird-editable-profiles-conflict tier=fast parallel=yes
func TestReservedTransport(t *testing.T) {
	for _, tc := range []struct {
		adapter string
		args    []string
	}{{"codex", []string{"--json"}}, {"codex", []string{"--cd=/else"}}, {"codex", []string{"-oelse"}}, {"codex", []string{"--config", "model_provider=paid"}}, {"claude", []string{"--output-format", "text"}}, {"claude", []string{"--bare"}}, {"claude", []string{"--resume", "existing"}}, {"codex", []string{"--"}}} {
		if e := ValidateArgs(tc.adapter, tc.args); e == nil {
			t.Fatalf("accepted reserved %#v", tc)
		}
	}
	if e := ValidateArgs("codex", []string{"--sandbox", "workspace-write", "-c", "features.web_search=true", "--enable", "x", "--enable", "y"}); e != nil {
		t.Fatal(e)
	}
}

// TESTMASTER: id=indigobird-profiles-dispatch tier=fast parallel=yes
func TestCustomResultContract(t *testing.T) {
	s := model.Subscription{Executable: script(t, "printf '%s' \"$1\" > \"$2\"\n"), Adapter: "generic", Args: []string{"{prompt}", "{result}"}}
	result := filepath.Join(t.TempDir(), "result.json")
	argv, e := Compile(s, model.Sequence{}, model.Assignment{Prompt: `{"outcome":"completed"}`, ResultPath: result})
	if e != nil {
		t.Fatal(e)
	}
	if e := exec.Command(argv[0], argv[1:]...).Run(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(result)
	if e != nil || string(b) != `{"outcome":"completed"}` {
		t.Fatalf("literal custom contract: %s %v", b, e)
	}
	s.Args = []string{"{prompt}"}
	if _, e = Compile(s, model.Sequence{}, model.Assignment{}); e == nil {
		t.Fatal("missing result contract accepted")
	}
}

// TESTMASTER: id=indigobird-engines-catalog tier=fast parallel=yes
func TestReadinessTemplates(t *testing.T) {
	templates := Templates()
	if len(templates) != 9 {
		t.Fatal(len(templates))
	}
	for _, s := range templates {
		if s.Enabled {
			t.Fatal("template dispatch enabled")
		}
	}
	s := model.Subscription{Executable: script(t, "echo '--json --output-last-message'\n"), Adapter: "codex"}
	r := Probe(context.Background(), s)
	if !r.Installed || !r.Stream || !r.ResultContract {
		t.Fatalf("%+v", r)
	}
	s.Executable = filepath.Join(t.TempDir(), "absent")
	if Probe(context.Background(), s).Installed {
		t.Fatal("claims absent CLI installed")
	}
}

// TESTMASTER: id=indigobird-profiles-permissions tier=fast parallel=yes
func TestSubscriptionEnvironment(t *testing.T) {
	got := ExecutionEnv("claude", []string{"PATH=/bin", "ANTHROPIC_API_KEY=secret", "OPENAI_API_KEY=secret", "CLAUDE_CODE_USE_BEDROCK=1", "CLAUDECODE=1", "HOME=/home"})
	if !reflect.DeepEqual(got, []string{"PATH=/bin", "HOME=/home"}) {
		t.Fatal(got)
	}
}

type fragmented struct{ io.Reader }

func (f fragmented) Read(p []byte) (int, error) {
	if len(p) > 3 {
		p = p[:3]
	}
	return f.Reader.Read(p)
}

// TESTMASTER: id=indigobird-stream-decode tier=fast parallel=yes
func TestFragmentedProviderStreams(t *testing.T) {
	for _, adapter := range []string{"codex", "claude"} {
		t.Run(adapter, func(t *testing.T) {
			b, e := os.ReadFile("testdata/" + adapter + ".jsonl")
			if e != nil {
				t.Fatal(e)
			}
			var events []model.Event
			e = Decode(context.Background(), adapter, fragmented{bytes.NewReader(b)}, func(ev model.Event) error { events = append(events, ev); return nil })
			if e != nil {
				t.Fatal(e)
			}
			terminal, result := false, false
			for _, ev := range events {
				if ev.Kind == "terminal" {
					terminal = true
				}
				if ev.Kind == "result" {
					r, e := ExtractResult(ev)
					if e != nil || r.AssignmentID != "probe" || !r.ValidationMet {
						t.Fatal(r, e)
					}
					result = true
				}
				if strings.Contains(string(ev.Data), "mcpServers") {
					t.Fatal("raw tool payload exposed")
				}
			}
			if !terminal || !result {
				t.Fatalf("terminal=%v result=%v events=%+v", terminal, result, events)
			}
		})
	}
}

// TESTMASTER: id=indigobird-stream-malformed tier=fast parallel=yes
func TestMalformedStreamDrain(t *testing.T) {
	for _, text := range []string{"{bad}\n{\"type\":\"future.kind\"}\n", strings.Repeat("x", MaxRecordBytes+1) + "\n{\"type\":\"turn.completed\"}"} {
		n := 0
		e := Decode(context.Background(), "codex", strings.NewReader(text), func(ev model.Event) error { n++; return nil })
		if e == nil || n != 2 {
			t.Fatalf("error=%v count=%d", e, n)
		}
	}
	sentinel := errors.New("storage full")
	if e := Decode(context.Background(), "codex", strings.NewReader("{\"type\":\"turn.started\"}\n"), func(model.Event) error { return sentinel }); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
}

// TESTMASTER: id=indigobird-stream-terminal tier=fast parallel=yes
func TestTerminalIsNotAssignmentSuccess(t *testing.T) {
	for _, tc := range []struct{ a, text string }{{"codex", `{"type":"turn.completed","usage":{"input_tokens":99,"output_tokens":3}}`}, {"claude", `{"type":"result","subtype":"success","is_error":false,"result":"looks good"}`}} {
		var events []model.Event
		e := Decode(context.Background(), tc.a, strings.NewReader(tc.text), func(ev model.Event) error { events = append(events, ev); return nil })
		if e != nil {
			t.Fatal(e)
		}
		for _, ev := range events {
			if ev.Kind == "result" {
				t.Fatal("invented assignment result")
			}
		}
	}
}

// TESTMASTER: id=indigobird-bands-separate tier=fast parallel=yes
func TestAuthenticatedAllowanceNormalization(t *testing.T) {
	s := model.Subscription{ID: "s"}
	raw := []byte(`{"accountId":"real-account","rateLimits":{"limitId":"codex","primary":{"usedPercent":13,"windowDurationMins":10080,"resetsAt":1792698036},"secondary":null}}`)
	u, e := NormalizeCodexUsage(s, raw, time.Now())
	if e != nil || u.Weekly.UsedPercent != 13 || !u.ShortUnsupported || u.Short != nil || u.AccountID == "" {
		t.Fatal(u, e)
	}
	s.AccountID = u.AccountID
	s.PoolID = u.PoolID
	if e = ValidateUsage(s, u); e != nil {
		t.Fatal(e)
	}
	s.AccountID = "other"
	if _, e = NormalizeCodexUsage(s, raw, time.Now()); e == nil {
		t.Fatal("account mismatch accepted")
	}
	c := model.Subscription{ID: "c", AccountID: "claude:scope", PoolID: "claude:scope"}
	u, e = NormalizeClaudeUsage(c, []byte(`{"unifiedWindows":{"five_hour":{"utilization":0.05,"resetsAt":1792196400},"seven_day":{"utilization":0.1,"resetsAt":1792728000}}}`), time.Now(), c.AccountID)
	if e != nil || u.Short.UsedPercent != 5 || u.Weekly.UsedPercent != 10 {
		t.Fatal(u, e)
	}
}

// TESTMASTER: id=indigobird-engines-unavailable tier=fast parallel=yes
func TestUsageCommandScope(t *testing.T) {
	u := model.Usage{SubscriptionID: "s", PoolID: "pool", AccountID: "account", Source: "fixture", ObservedAt: time.Now(), Supported: true, Weekly: &model.Allowance{UsedPercent: 42}}
	b, _ := json.Marshal(u)
	path := filepath.Join(t.TempDir(), "usage.json")
	_ = os.WriteFile(path, b, 0600)
	s := model.Subscription{ID: "s", AccountID: "account", PoolID: "pool", UsageCommand: []string{"cat", path}}
	got, e := CollectUsage(context.Background(), s)
	if e != nil || got.Weekly.UsedPercent != 42 {
		t.Fatal(got, e)
	}
	s.AccountID = "other"
	if _, e = CollectUsage(context.Background(), s); e == nil {
		t.Fatal("mismatched account accepted")
	}
	s = model.Subscription{ID: "custom", Adapter: "generic"}
	got, e = CollectUsage(context.Background(), s)
	if e != nil || got.Supported || got.Reason == "" {
		t.Fatal(got, e)
	}
}

// TESTMASTER: id=indigobird-bands-separate tier=fast parallel=yes
func TestProviderEmbargoPreservesUsage(t *testing.T) {
	u, e := NormalizeCodexUsage(model.Subscription{ID: "s"}, []byte(`{"accountId":"scope","ordinaryUsageAllowed":false,"rateLimits":{"primary":{"usedPercent":12,"windowDurationMins":10080,"resetsAt":1792698036}}}`), time.Now())
	if e != nil || !u.HardBlocked || u.Weekly.UsedPercent != 12 {
		t.Fatal(u, e)
	}
}

// TESTMASTER: id=indigobird-bands-separate tier=fast parallel=yes
func TestClaudeCollectorAuthenticContract(t *testing.T) {
	raw := []byte(`[{"provider":"claude","source":"claude","usage":{"updatedAt":"2026-10-16T20:24:10Z","primary":{"usedPercent":5,"windowMinutes":300,"resetsAt":"2026-10-17T00:20:00Z"},"secondary":{"usedPercent":10,"windowMinutes":10080,"resetsAt":"2026-10-23T04:00:00Z"}}}]`)
	s := model.Subscription{ID: "claude", AccountID: "claude:scope", PoolID: "claude:scope"}
	u, e := NormalizeClaudeCollector(s, raw, s.AccountID)
	if e != nil || !u.Supported || u.Weekly.UsedPercent != 10 || u.Short.UsedPercent != 5 || !u.Weekly.ResetTrusted {
		t.Fatal(u, e)
	}
	if _, e = NormalizeClaudeCollector(s, raw, "claude:other"); e == nil {
		t.Fatal("foreign identity accepted")
	}
}

// TESTMASTER: id=indigobird-engines-catalog tier=fast parallel=yes
func TestAgentPromptFileTransport(t *testing.T) {
	for _, s := range Templates() {
		if s.ResultTransport != "prompt_file" {
			continue
		}
		a := model.Assignment{ID: "assignment", RunID: "run", ProjectPath: "/tmp/project", Plan: "plan", Step: 1, SourceDigest: "digest", Prompt: "Perform pair", ResultPath: "/tmp/result.json"}
		argv, e := Compile(s, s.Sequences[0], a)
		if e != nil {
			t.Fatalf("%s: %v", s.Name, e)
		}
		found := false
		for _, arg := range argv {
			if strings.Contains(arg, "/tmp/result.json") && strings.Contains(arg, `"assignment_id":"assignment"`) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no result protocol", s.Name)
		}
	}
	s := model.Subscription{Executable: "custom", Adapter: "generic", ResultTransport: "unknown", Args: []string{"{prompt}"}}
	if _, e := Compile(s, model.Sequence{}, model.Assignment{}); e == nil {
		t.Fatal("unknown transport accepted")
	}
}

// TESTMASTER: id=indigobird-provider-codex-result-transport tier=fast parallel=yes
func TestCodexFinalResultTransport(t *testing.T) {
	a := model.Assignment{ID: "assignment", RunID: "run", ProjectPath: "/project", Plan: "plan", ResultPath: "/state/result.json", Subscription: model.Subscription{Adapter: "codex"}}
	prompt := AssignmentPrompt(a)
	if !strings.Contains(prompt, "return exactly one JSON object as your final message") || !strings.Contains(prompt, "do not write the result file through a tool") || !strings.Contains(prompt, `"assignment_id":"assignment"`) {
		t.Fatal(prompt)
	}
	a.Subscription.Adapter = "claude"
	prompt = AssignmentPrompt(a)
	if !strings.Contains(prompt, "write one JSON object to /state/result.json") {
		t.Fatal(prompt)
	}
}

// TESTMASTER: id=indigobird-editable-profiles-argv tier=fast parallel=yes
func TestCompilePromptSeparator(t *testing.T) {
	for _, adapter := range []string{"claude", "claude-code", "codex"} {
		t.Run(adapter, func(t *testing.T) {
			exe := script(t, "while [ \"$#\" -gt 0 ]; do if [ \"$1\" = -- ]; then shift; [ \"$#\" -eq 1 ] || exit 2; printf '%s' \"$1\"; exit 0; fi; shift; done; exit 3\n")
			args := []string{"--tools", "Read,Write,Bash"}
			if adapter == "codex" {
				args = []string{"--enable", "fixture"}
			}
			s := model.Subscription{Executable: exe, Adapter: adapter, Args: args}
			prompt := "--assignment starts with a dash and contains spaces"
			argv, e := Compile(s, model.Sequence{}, model.Assignment{Prompt: prompt})
			if e != nil {
				t.Fatal(e)
			}
			out, e := exec.Command(argv[0], argv[1:]...).Output()
			if e != nil || string(out) != prompt {
				t.Fatal("owned prompt is not a single positional argument after user options", string(out), e)
			}
		})
	}
}
