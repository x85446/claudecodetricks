// Package model defines the shared conductor configuration and execution protocol.
package model

import (
	"encoding/json"
	"time"
)

const SchemaVersion = 3
const ProtocolVersion = 1

type Band string

const (
	Green  Band = "green"
	Yellow Band = "yellow"
	Red    Band = "red"
)

type Config struct {
	Version       int                    `json:"version"`
	Revision      int64                  `json:"revision"`
	Enabled       bool                   `json:"enabled"`
	Paused        bool                   `json:"paused"`
	Projects      []Project              `json:"projects"`
	Subscriptions []Subscription         `json:"subscriptions"`
	PowerBands    []PowerBand            `json:"power_bands"`
	TimeWindows   []TimeWindow           `json:"time_windows"`
	Attachments   []AutomationAttachment `json:"automation_attachments"`
	Automations   []Automation           `json:"automations"`
}

type Project struct {
	Tags              []string `json:"tags,omitempty"`
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Path              string   `json:"path"`
	Enabled           bool     `json:"enabled"`
	Paused            bool     `json:"paused"`
	WindowID          string   `json:"window_id,omitempty"`
	MaxAttempts       int      `json:"max_attempts"`
	NoProgressSeconds int      `json:"no_progress_seconds"`
	PlanCapSeconds    int      `json:"plan_cap_seconds,omitempty"`
	WatchBound        int      `json:"watch_bound"`
	IntakeEnabled     bool     `json:"intake_enabled"`
	MaxImport         int      `json:"max_import"`
}

type Subscription struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Enabled          bool              `json:"enabled"`
	Adapter          string            `json:"adapter"`
	ResultTransport  string            `json:"result_transport,omitempty"`
	Executable       string            `json:"executable"`
	Args             []string          `json:"args,omitempty"`
	PoolID           string            `json:"pool_id"`
	AccountID        string            `json:"account_id,omitempty"`
	Tier             string            `json:"tier,omitempty"`
	PowerBandID      string            `json:"power_band_id"`
	Concurrency      int               `json:"concurrency"`
	FreshnessSeconds int               `json:"freshness_seconds"`
	TypicalReset     ResetSchedule     `json:"typical_reset"`
	UsageCommand     []string          `json:"usage_command,omitempty"`
	Sequences        []Sequence        `json:"sequences"`
	Presets          []Preset          `json:"presets"`
	OverdriveEnabled bool              `json:"overdrive_enabled"`
	OverdriveWindows []OverdriveWindow `json:"overdrive_windows"`
}

type ResetSchedule struct {
	Day      int    `json:"day"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
}

type Sequence struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Model     string   `json:"model,omitempty"`
	Reasoning string   `json:"reasoning,omitempty"`
	PresetID  string   `json:"preset_id,omitempty"`
	Args      []string `json:"args,omitempty"`
}

func SequenceName(s Subscription, q Sequence) string { return s.Name + " · " + q.Name }

type Preset struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Args []string `json:"args"`
}

type PowerBand struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	GreenCurve      []float64 `json:"green_curve"`
	RedCurve        []float64 `json:"red_curve"`
	ShortGreenCurve []float64 `json:"short_green_curve,omitempty"`
	ShortRedCurve   []float64 `json:"short_red_curve,omitempty"`
	ShortGreen      float64   `json:"short_green"`
	ShortRed        float64   `json:"short_red"`
	AllowedBands    []Band    `json:"allowed_bands"`
}

type Recurrence struct {
	AllDay     bool   `json:"all_day,omitempty"`
	Days       []int  `json:"days"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Timezone   string `json:"timezone"`
	EveryWeeks int    `json:"every_weeks"`
	AnchorDate string `json:"anchor_date,omitempty"`
}

type TimeWindow struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Enabled          bool       `json:"enabled"`
	Revision         int64      `json:"revision"`
	StartMode        string     `json:"start_mode"`
	HoursBeforeReset float64    `json:"hours_before_reset,omitempty"`
	Recurrence       Recurrence `json:"recurrence"`
}

type OverdriveWindow struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Enabled           bool       `json:"enabled"`
	Revision          int64      `json:"revision"`
	Recurrence        Recurrence `json:"recurrence"`
	StartMode         string     `json:"start_mode"`
	HoursBeforeReset  float64    `json:"hours_before_reset,omitempty"`
	EndMode           string     `json:"end_mode"`
	EndDay            int        `json:"end_day,omitempty"`
	EndTime           string     `json:"end_time,omitempty"`
	TargetUsedPercent float64    `json:"target_used_percent"`
}

type AutomationAttachment struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definition_id"`
	ProjectID    string `json:"project_id"`
	Enabled      bool   `json:"enabled"`
	Priority     int    `json:"priority"`
}

// EffectiveAutomations composes shared rules with project-local activation and identity.
func EffectiveAutomations(c Config) []Automation {
	out := []Automation{}
	for _, a := range c.Automations {
		if a.ProjectID != "" {
			out = append(out, a)
			continue
		}
		for _, link := range c.Attachments {
			if link.DefinitionID == a.ID {
				x := a
				x.DefinitionID = a.ID
				x.ID = link.ID
				x.ProjectID = link.ProjectID
				x.Enabled = link.Enabled
				x.Priority = link.Priority
				out = append(out, x)
			}
		}
	}
	return out
}

type Automation struct {
	DefinitionID   string   `json:"definition_id,omitempty"`
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	ProjectID      string   `json:"project_id"`
	Enabled        bool     `json:"enabled"`
	Priority       int      `json:"priority"`
	Mode           string   `json:"mode"`
	Entries        []Entry  `json:"entries"`
	Participants   []string `json:"participants"`
	WindowID       string   `json:"window_id,omitempty"`
	HonorOverdrive bool     `json:"honor_engine_overdrive"`
	OnlyOverdrive  bool     `json:"only_during_overdrive"`
	RunLimit       string   `json:"run_limit"`
	OnWindowEnd    string   `json:"on_window_end"`
}

type Entry struct {
	ID         string `json:"id"`
	SequenceID string `json:"sequence_id"`
	MaxBand    Band   `json:"max_band"`
}

// Usage is an account-scoped allowance observation, separate from execution tokens.
type Usage struct {
	SubscriptionID   string               `json:"subscription_id"`
	PoolID           string               `json:"pool_id"`
	AccountID        string               `json:"account_id,omitempty"`
	Source           string               `json:"source"`
	ObservedAt       time.Time            `json:"observed_at"`
	Supported        bool                 `json:"supported"`
	HardBlocked      bool                 `json:"hard_blocked"`
	Reason           string               `json:"reason,omitempty"`
	Weekly           *Allowance           `json:"weekly,omitempty"`
	Short            *Allowance           `json:"short,omitempty"`
	ShortUnsupported bool                 `json:"short_unsupported"`
	Models           map[string]Allowance `json:"models,omitempty"`
}

type Allowance struct {
	UsedPercent  float64   `json:"used_percent"`
	StartsAt     time.Time `json:"starts_at"`
	ResetsAt     time.Time `json:"resets_at"`
	ResetTrusted bool      `json:"reset_trusted"`
	Epoch        string    `json:"epoch"`
}

type Occurrence struct {
	ID             string    `json:"id"`
	WindowID       string    `json:"window_id"`
	SubscriptionID string    `json:"subscription_id,omitempty"`
	AutomationID   string    `json:"automation_id,omitempty"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	Epoch          string    `json:"epoch,omitempty"`
	Completed      bool      `json:"completed"`
	ClaimedPlan    string    `json:"claimed_plan,omitempty"`
	ClosingAction  string    `json:"closing_action,omitempty"`
}

type BudgetDecision struct {
	Allowed           bool         `json:"allowed"`
	HardAllowed       bool         `json:"hard_allowed"`
	Band              Band         `json:"band"`
	WeeklyBand        Band         `json:"weekly_band"`
	ShortBand         Band         `json:"short_band"`
	Reason            string       `json:"reason"`
	Code              string       `json:"code"`
	OverdriveActive   bool         `json:"overdrive_active"`
	TargetUsedPercent float64      `json:"target_used_percent"`
	WindowIDs         []string     `json:"window_ids,omitempty"`
	Occurrences       []Occurrence `json:"occurrences,omitempty"`
	GreenEnd          float64      `json:"green_end"`
	RedStart          float64      `json:"red_start"`
	WeeklyUsed        *float64     `json:"weekly_used,omitempty"`
	ShortUsed         *float64     `json:"short_used,omitempty"`
}

type Readiness struct {
	Installed      bool   `json:"installed"`
	Executable     string `json:"executable"`
	Stream         bool   `json:"stream"`
	Telemetry      bool   `json:"telemetry"`
	ResultContract bool   `json:"result_contract"`
	Reason         string `json:"reason"`
}

type Assignment struct {
	Version        int            `json:"version"`
	ID             string         `json:"id"`
	RunID          string         `json:"run_id"`
	ProjectID      string         `json:"project_id"`
	ProjectPath    string         `json:"project_path"`
	Plan           string         `json:"plan"`
	Step           int            `json:"step"`
	Team           string         `json:"team,omitempty"`
	Kind           string         `json:"kind"`
	SourceDigest   string         `json:"source_digest"`
	Task           string         `json:"task"`
	Validation     string         `json:"validation"`
	Prompt         string         `json:"prompt"`
	ResultPath     string         `json:"result_path"`
	ConfigRevision int64          `json:"config_revision"`
	Subscription   Subscription   `json:"subscription"`
	Sequence       Sequence       `json:"sequence"`
	Automation     Automation     `json:"automation"`
	Budget         BudgetDecision `json:"budget"`
	CreatedAt      time.Time      `json:"created_at"`
}

type Result struct {
	Version       int      `json:"version"`
	AssignmentID  string   `json:"assignment_id"`
	RunID         string   `json:"run_id"`
	ProjectPath   string   `json:"project_path"`
	Plan          string   `json:"plan"`
	Step          int      `json:"step"`
	SourceDigest  string   `json:"source_digest"`
	Outcome       string   `json:"outcome"`
	TaskComplete  bool     `json:"task_complete"`
	ValidationMet bool     `json:"validation_met"`
	Evidence      []string `json:"evidence"`
	Blocker       string   `json:"blocker,omitempty"`
}

type Event struct {
	Version      int             `json:"version"`
	ProjectPath  string          `json:"project_path"`
	Plan         string          `json:"plan"`
	RunID        string          `json:"run_id"`
	AssignmentID string          `json:"assignment_id"`
	Sequence     uint64          `json:"sequence"`
	At           time.Time       `json:"at"`
	Kind         string          `json:"kind"`
	ProviderID   string          `json:"provider_id,omitempty"`
	SessionID    string          `json:"session_id,omitempty"`
	Message      string          `json:"message,omitempty"`
	InputTokens  int64           `json:"input_tokens,omitempty"`
	OutputTokens int64           `json:"output_tokens,omitempty"`
	Cumulative   bool            `json:"cumulative,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
}
