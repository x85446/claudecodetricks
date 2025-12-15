package hooks

// HookInput represents the JSON structure that Claude Code sends to hooks via stdin
type HookInput struct {
	SessionID      string                 `json:"session_id"`
	TranscriptPath string                 `json:"transcript_path"`
	CWD            string                 `json:"cwd"`
	PermissionMode string                 `json:"permission_mode"`
	HookEventName  string                 `json:"hook_event_name"`
	ToolName       string                 `json:"tool_name,omitempty"`
	ToolInput      map[string]interface{} `json:"tool_input,omitempty"`
	ToolResponse   map[string]interface{} `json:"tool_response,omitempty"`
}

// TranscriptEntry represents a single line in the .jsonl transcript file
type TranscriptEntry struct {
	Type      string                 `json:"type"`
	Timestamp string                 `json:"timestamp,omitempty"`
	Role      string                 `json:"role,omitempty"`
	Content   []ContentBlock         `json:"content,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	ToolName  string                 `json:"tool_name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Output    interface{}            `json:"output,omitempty"`
}

// ContentBlock represents a block of content in a transcript message
type ContentBlock struct {
	Type    string                 `json:"type"`
	Text    string                 `json:"text,omitempty"`
	ToolUse *ToolUseBlock          `json:"tool_use,omitempty"`
	Input   map[string]interface{} `json:"input,omitempty"`
}

// ToolUseBlock represents a tool use within a content block
type ToolUseBlock struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}
