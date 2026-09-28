// chain-guard is a PreToolUse hook that refuses a Bash call which buries
// an allowlisted, classifier-sensitive command inside a compound command.
//
// The permission system resolves an allow rule against a single command.
// Chained, the rule stops matching and the whole invocation is handed to
// the auto-mode classifier, which blocks the very operation the rule was
// written to permit. The fix is mechanical -- run it bare -- so this is a
// mechanical check rather than a line of documentation somebody has to
// remember.
//
// Wire it as a PreToolUse hook on Bash. It denies only the compound case
// and is silent otherwise; anything it cannot parse it lets through,
// because a guard that blocks good commands is worse than the problem.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/x85446/claudecodetricks/src/internal/chainguard"
)

type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// hookOutput is Claude Code's PreToolUse decision envelope. "deny" stops
// the call and hands reason back to the model, which is exactly what we
// want: the model reads why and reissues the command bare.
type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

func main() {
	var in hookInput
	if json.NewDecoder(os.Stdin).Decode(&in) != nil || in.ToolName != "Bash" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	offender, chained := chainguard.Offender(in.ToolInput.Command, chainguard.LoadAllowedBashPrefixes(home))
	if !chained {
		return
	}

	var out hookOutput
	out.HookSpecificOutput.HookEventName = "PreToolUse"
	out.HookSpecificOutput.PermissionDecision = "deny"
	out.HookSpecificOutput.PermissionDecisionReason = fmt.Sprintf(
		"%q is allowlisted in permissions.allow, but an allow rule matches a single command, not a pipeline. "+
			"Chained with && ; or |, this invocation stops matching the rule and goes to the auto-mode classifier, "+
			"which blocks it. Re-run %q as a bare command on its own, and gather any state in a separate call.",
		offender, offender)
	_ = json.NewEncoder(os.Stdout).Encode(out)
}
