package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// AccountScope hashes a provider's authenticated identity without storing credentials.
func AccountScope(provider, identity string) string {
	h := sha256.Sum256([]byte(identity))
	return provider + ":" + hex.EncodeToString(h[:16])
}

// CollectUsage performs read-only provider discovery or a configured literal
// command returning model.Usage. It never substitutes session tokens for quota.
func CollectUsage(ctx context.Context, s model.Subscription) (model.Usage, error) {
	u := model.Usage{SubscriptionID: s.ID, PoolID: s.PoolID, AccountID: s.AccountID, Source: "unsupported", ObservedAt: time.Now().UTC(), Reason: "No authenticated allowance collector configured"}
	if len(s.UsageCommand) > 0 {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, s.UsageCommand[0], s.UsageCommand[1:]...)
		cmd.Env = ExecutionEnv(s.Adapter, os.Environ())
		var out limitedBuffer
		out.limit = MaxRecordBytes
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return u, errors.New("usage_command failed; inspect the collector independently")
		}
		if err := json.Unmarshal(out.Bytes(), &u); err != nil {
			return u, errors.New("usage_command returned invalid usage JSON")
		}
		if err := ValidateUsage(s, u); err != nil {
			return u, err
		}
		return u, nil
	}
	if s.Adapter == "codex" {
		return collectCodex(ctx, s)
	}
	if s.Adapter == "claude" || s.Adapter == "claude-code" {
		return collectClaude(ctx, s)
	}
	return u, nil
}
func ValidateUsage(s model.Subscription, u model.Usage) error {
	if u.SubscriptionID != s.ID || u.PoolID != s.PoolID || u.AccountID == "" || s.AccountID == "" || u.AccountID != s.AccountID {
		return errors.New("usage scope does not match the bound subscription/account/pool")
	}
	if u.ObservedAt.IsZero() || u.Source == "" {
		return errors.New("usage source and observed_at required")
	}
	if u.ObservedAt.After(time.Now().Add(time.Minute)) {
		return errors.New("usage timestamp is in the future")
	}
	for _, p := range []*model.Allowance{u.Weekly, u.Short} {
		if p != nil {
			if math.IsNaN(p.UsedPercent) || math.IsInf(p.UsedPercent, 0) || p.UsedPercent < 0 || p.UsedPercent > 100 {
				return errors.New("usage percent outside 0..100")
			}
			if p.ResetTrusted && (p.ResetsAt.IsZero() || p.Epoch == "") {
				return errors.New("trusted reset requires timestamp and epoch")
			}
		}
	}
	for _, p := range u.Models {
		if p.UsedPercent < 0 || p.UsedPercent > 100 || math.IsNaN(p.UsedPercent) || math.IsInf(p.UsedPercent, 0) {
			return errors.New("model usage percent outside 0..100")
		}
	}
	return nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("provider output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func collectCodex(ctx context.Context, s model.Subscription) (model.Usage, error) {
	u := model.Usage{SubscriptionID: s.ID, Source: "codex app-server account/rateLimits/read", ObservedAt: time.Now().UTC()}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Executable, "app-server", "--stdio")
	cmd.Env = ExecutionEnv(s.Adapter, os.Environ())
	in, err := cmd.StdinPipe()
	if err != nil {
		return u, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return u, err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return u, fmt.Errorf("usage collector unavailable: %w", err)
	}
	defer func() { in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	send := func(id int, method string, params any) error {
		return json.NewEncoder(in).Encode(map[string]any{"id": id, "method": method, "params": params})
	}
	if err = send(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "iterate_conductor", "version": "1"}}); err != nil {
		return u, err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 4096), MaxRecordBytes)
	authenticated := false
	for sc.Scan() {
		var m map[string]json.RawMessage
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		id := number(m, "id")
		if len(m["error"]) > 0 {
			return u, errors.New("Codex allowance RPC unavailable or authentication required")
		}
		switch id {
		case 1:
			_ = json.NewEncoder(in).Encode(map[string]any{"method": "initialized"})
			if err = send(2, "account/read", map[string]any{}); err != nil {
				return u, err
			}
		case 2:
			a := object(object(m, "result"), "account")
			authenticated = str(a, "type") == "chatgpt"
			if !authenticated {
				return u, errors.New("Codex subscription authentication required; API-key fallback is disabled")
			}
			if err = send(3, "account/rateLimits/read", map[string]any{}); err != nil {
				return u, err
			}
		case 3:
			if !authenticated {
				return u, errors.New("Codex account not authenticated")
			}
			return NormalizeCodexUsage(s, m["result"], time.Now().UTC())
		}
	}
	if err := ctx.Err(); err != nil {
		return u, err
	}
	if err := sc.Err(); err != nil {
		return u, err
	}
	return u, errors.New("Codex collector ended before allowance response")
}

// NormalizeCodexUsage binds the actual reported account; discovery returns its
// scope even when an unbound configuration cannot yet pass admission.
func NormalizeCodexUsage(s model.Subscription, raw []byte, at time.Time) (model.Usage, error) {
	var v map[string]json.RawMessage
	if err := json.Unmarshal(raw, &v); err != nil {
		return model.Usage{}, err
	}
	u := model.Usage{SubscriptionID: s.ID, Source: "codex app-server account/rateLimits/read", ObservedAt: at}
	account := str(v, "accountId")
	if account == "" {
		return u, errors.New("Codex response has no authenticated account scope")
	}
	u.AccountID = AccountScope("codex", account)
	u.PoolID = u.AccountID
	if s.AccountID != "" && s.AccountID != u.AccountID {
		return u, errors.New("Codex active account differs from subscription binding")
	}
	if s.PoolID != "" && s.AccountID != "" && s.PoolID != u.PoolID {
		return u, errors.New("Codex pool must match authenticated shared account scope")
	}
	limits := object(v, "rateLimits")
	var ordinary *bool
	_ = json.Unmarshal(v["ordinaryUsageAllowed"], &ordinary)
	var spend bool
	_ = json.Unmarshal(limits["spendControlReached"], &spend)
	u.HardBlocked = (ordinary != nil && !*ordinary) || spend || (str(limits, "rateLimitReachedType") != "")
	if u.HardBlocked {
		u.Reason = "Provider disallows ordinary subscription usage"
	}
	if len(limits) == 0 {
		return u, errors.New("Codex allowance is absent")
	}
	for _, key := range []string{"primary", "secondary"} {
		p, minutes, err := codexAllowance(limits[key])
		if err != nil {
			return u, err
		}
		if p == nil {
			continue
		}
		if minutes == 10080 {
			u.Weekly = p
		} else if minutes == 300 {
			u.Short = p
		} else {
			return u, fmt.Errorf("unsupported Codex allowance duration %d minutes", minutes)
		}
	}
	if u.Weekly == nil {
		return u, errors.New("Codex weekly allowance is unavailable")
	}
	u.ShortUnsupported = u.Short == nil
	u.Supported = true
	pools := object(v, "rateLimitsByLimitId")
	u.Models = map[string]model.Allowance{}
	for name, pool := range pools {
		if name == str(limits, "limitId") {
			continue
		}
		var p map[string]json.RawMessage
		_ = json.Unmarshal(pool, &p)
		for _, key := range []string{"primary", "secondary"} {
			a, _, err := codexAllowance(p[key])
			if err != nil {
				return u, err
			}
			if a != nil {
				u.Models[name+":"+key] = *a
			}
		}
	}
	return u, nil
}
func codexAllowance(raw json.RawMessage) (*model.Allowance, int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, 0, nil
	}
	var p struct {
		Used    *float64 `json:"usedPercent"`
		Minutes int64    `json:"windowDurationMins"`
		Reset   int64    `json:"resetsAt"`
	}
	if json.Unmarshal(raw, &p) != nil || p.Used == nil || *p.Used < 0 || *p.Used > 100 || p.Minutes <= 0 || p.Reset <= 0 {
		return nil, 0, errors.New("malformed Codex allowance")
	}
	reset := time.Unix(p.Reset, 0).UTC()
	return &model.Allowance{UsedPercent: *p.Used, StartsAt: reset.Add(-time.Duration(p.Minutes) * time.Minute), ResetsAt: reset, ResetTrusted: true, Epoch: strconv.FormatInt(p.Reset, 10)}, p.Minutes, nil
}

// NormalizeClaudeUsage accepts a live rate_limit_event plus the account scope
// established by that process's authenticated Claude session, not a global cache.
func NormalizeClaudeUsage(s model.Subscription, raw []byte, at time.Time, accountID string) (model.Usage, error) {
	u := model.Usage{SubscriptionID: s.ID, PoolID: accountID, AccountID: accountID, ObservedAt: at, Source: "claude stream-json rate_limit_event"}
	if accountID == "" || s.AccountID == "" || accountID != s.AccountID || s.PoolID != accountID {
		return u, errors.New("Claude observation requires matching authenticated account scope")
	}
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil {
		return u, errors.New("malformed Claude allowance")
	}
	u.HardBlocked = str(v, "status") == "rejected"
	if u.HardBlocked {
		u.Reason = "Claude reports a hard rate limit"
	}
	windows := object(v, "unifiedWindows")
	if len(windows) == 0 {
		return u, errors.New("Claude event has no utilization windows")
	}
	for key, raw := range windows {
		var p struct {
			Used  *float64 `json:"utilization"`
			Reset int64    `json:"resetsAt"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Used == nil || *p.Used < 0 || *p.Used > 1 || p.Reset <= 0 {
			return u, errors.New("malformed Claude utilization window")
		}
		d := 7 * 24 * time.Hour
		if key == "five_hour" {
			d = 5 * time.Hour
		}
		reset := time.Unix(p.Reset, 0).UTC()
		a := model.Allowance{UsedPercent: *p.Used * 100, StartsAt: reset.Add(-d), ResetsAt: reset, ResetTrusted: true, Epoch: strconv.FormatInt(p.Reset, 10)}
		switch key {
		case "five_hour":
			u.Short = &a
		case "seven_day":
			u.Weekly = &a
		default:
			if strings.HasPrefix(key, "seven_day_") {
				if u.Models == nil {
					u.Models = map[string]model.Allowance{}
				}
				u.Models[strings.TrimPrefix(key, "seven_day_")] = a
			}
		}
	}
	u.Supported = u.Weekly != nil && u.Short != nil
	if !u.Supported {
		u.Reason = "Claude weekly or five-hour utilization missing"
	}
	return u, nil
}

// ClaudeAccountScope checks subscription authentication through the CLI. API
// credentials are removed and the organization/user identity is hashed locally.
func ClaudeAccountScope(ctx context.Context, executable string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "auth", "status")
	cmd.Env = ExecutionEnv("claude", os.Environ())
	b, err := cmd.Output()
	if err != nil {
		return "", errors.New("Claude subscription authentication unavailable")
	}
	var a struct {
		LoggedIn bool   `json:"loggedIn"`
		Method   string `json:"authMethod"`
		Org      string `json:"orgId"`
		Email    string `json:"email"`
	}
	if json.Unmarshal(b, &a) != nil || !a.LoggedIn || a.Method != "claude.ai" || a.Org == "" || a.Email == "" {
		return "", errors.New("Claude requires authenticated claude.ai account identity")
	}
	return AccountScope("claude", a.Org+":"+a.Email), nil
}
func collectClaude(ctx context.Context, s model.Subscription) (model.Usage, error) {
	u := model.Usage{SubscriptionID: s.ID, Source: "CodexBar Claude CLI usage", ObservedAt: time.Now().UTC()}
	collector, err := exec.LookPath("codexbar")
	if err != nil {
		u.Reason = "CodexBar unavailable; configure an account-scoped usage_command"
		return u, nil
	}
	scope, err := ClaudeAccountScope(ctx, s.Executable)
	if err != nil {
		return u, err
	}
	u.AccountID = scope
	u.PoolID = scope
	if s.AccountID != "" && (s.AccountID != scope || s.PoolID != scope) {
		return u, errors.New("Claude active account differs from subscription binding")
	}
	// CodexBar's CLI source invokes the installed default Claude executable.
	selected, e1 := exec.LookPath(s.Executable)
	installed, e2 := exec.LookPath("claude")
	if e1 != nil || e2 != nil || selected != installed {
		return u, errors.New("CodexBar collector requires the selected default Claude executable; use usage_command for custom executables")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, collector, "usage", "--provider", "claude", "--source", "cli", "--format", "json", "--no-credits", "--no-color")
	cmd.Env = ExecutionEnv("claude", os.Environ())
	var out limitedBuffer
	out.limit = MaxRecordBytes
	cmd.Stdout = &out
	if err = cmd.Run(); err != nil {
		return u, errors.New("Claude CLI usage collector failed")
	}
	after, err := ClaudeAccountScope(ctx, s.Executable)
	if err != nil || after != scope {
		return u, errors.New("Claude account changed while collecting allowance")
	}
	return NormalizeClaudeCollector(s, out.Bytes(), scope)
}

// NormalizeClaudeCollector reuses the installed collector's percentage/reset
// contract with an independently authenticated, stable account scope.
func NormalizeClaudeCollector(s model.Subscription, raw []byte, scope string) (model.Usage, error) {
	u := model.Usage{SubscriptionID: s.ID, AccountID: scope, PoolID: scope, Source: "CodexBar Claude CLI usage"}
	if scope == "" || (s.AccountID != "" && (s.AccountID != scope || s.PoolID != scope)) {
		return u, errors.New("Claude collector account binding mismatch")
	}
	var rows []struct {
		Provider string          `json:"provider"`
		Source   string          `json:"source"`
		Error    json.RawMessage `json:"error"`
		Usage    struct {
			Updated   time.Time       `json:"updatedAt"`
			Primary   json.RawMessage `json:"primary"`
			Secondary json.RawMessage `json:"secondary"`
			Tertiary  json.RawMessage `json:"tertiary"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &rows) != nil || len(rows) != 1 || rows[0].Provider != "claude" || len(rows[0].Error) > 0 {
		return u, errors.New("malformed Claude collector response")
	}
	u.ObservedAt = rows[0].Usage.Updated
	if u.ObservedAt.IsZero() || u.ObservedAt.After(time.Now().Add(time.Minute)) {
		return u, errors.New("Claude collector timestamp unavailable or invalid")
	}
	for _, raw := range []json.RawMessage{rows[0].Usage.Primary, rows[0].Usage.Secondary} {
		var p struct {
			Used    *float64  `json:"usedPercent"`
			Reset   time.Time `json:"resetsAt"`
			Minutes int       `json:"windowMinutes"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Used == nil || *p.Used < 0 || *p.Used > 100 || p.Reset.IsZero() {
			return u, errors.New("malformed Claude allowance window")
		}
		a := model.Allowance{UsedPercent: *p.Used, StartsAt: p.Reset.Add(-time.Duration(p.Minutes) * time.Minute), ResetsAt: p.Reset, ResetTrusted: true, Epoch: p.Reset.UTC().Format(time.RFC3339)}
		switch p.Minutes {
		case 300:
			u.Short = &a
		case 10080:
			u.Weekly = &a
		default:
			return u, errors.New("unsupported Claude allowance duration")
		}
	}
	if u.Short == nil || u.Weekly == nil {
		return u, errors.New("Claude weekly or five-hour allowance absent")
	}
	u.Supported = true
	return u, nil
}
