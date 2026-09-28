// Package chainguard enforces one rule: a command that matches a
// permissions.allow rule must run as a bare, single command.
//
// A permission allow rule matches a COMMAND, not a pipeline. Chaining an
// allowlisted command with &&, ||, ;, | or a newline means the combined
// invocation no longer matches the rule, so it falls out of permission
// decision-step 1 ("actions matching your allow rules resolve
// immediately") down to step 3 ("everything else goes to the
// classifier") -- and in auto mode the classifier then vetoes exactly the
// operations those rules were written to permit.
//
// Confirmed live: `gh pr merge 23 --merge --delete-branch 2>&1 | tail -3
// && git checkout main` was denied as [Merge Without Review] even though
// Bash(gh pr merge:*) was allowlisted. The same merge, run bare, went
// through instantly.
//
// The guard is deliberately narrow. Chaining is normally good and saves
// round-trips, and for a benign allowlisted command like `cat` the
// classifier approves the pipeline anyway, so blocking that would be pure
// tax. Only commands that are BOTH allowlisted AND expensive to have
// blocked are guarded -- see sensitivePrefixes.
package chainguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// sensitivePrefixes are the command prefixes where losing the allow rule
// actually costs something: the classifier reviews them on their own
// merits and often refuses. A benign allowlisted command (cat, mkdir,
// lsof) is not listed -- chaining those is harmless, so guarding them
// would only cost round-trips.
//
// A prefix is guarded only when the user ALSO has an allow rule for it,
// so this list never invents permission the user did not grant.
var sensitivePrefixes = []string{
	"gh pr merge",
	"gh pr create",
	"gh pr ready",
	"gh release create",
	"git push",
	"git merge",
	"git rebase",
}

// separator finds the shell operators that turn one command into
// several. A quoted or escaped operator is not a separator, so the
// scanner below tracks quoting rather than splitting blindly -- `echo
// "a && b"` is one command and must not be flagged.
var allowRulePattern = regexp.MustCompile(`^Bash\((.+?):?\*?\)$`)

// LoadAllowedBashPrefixes reads every Bash(...) allow rule from the
// user's settings files and returns the command prefixes they grant.
// Reading the real settings is what keeps the guard self-maintaining: a
// rule the user removes stops being guarded, with nothing to edit here.
func LoadAllowedBashPrefixes(home string) []string {
	var prefixes []string
	for _, name := range []string{"settings.json", "settings.local.json"} {
		data, err := os.ReadFile(filepath.Join(home, ".claude", name))
		if err != nil {
			continue
		}
		var parsed struct {
			Permissions struct {
				Allow []string `json:"allow"`
			} `json:"permissions"`
		}
		if json.Unmarshal(data, &parsed) != nil {
			continue
		}
		for _, rule := range parsed.Permissions.Allow {
			if m := allowRulePattern.FindStringSubmatch(rule); m != nil {
				prefixes = append(prefixes, strings.TrimSuffix(strings.TrimSpace(m[1]), ":"))
			}
		}
	}
	return prefixes
}

// SplitTopLevel breaks a shell command on the operators that separate
// commands, ignoring any that are quoted or escaped. It is not a shell
// parser and does not need to be: it only has to decide "is there more
// than one command here", and it errs toward seeing fewer separators, so
// an exotic quoting case under-reports rather than blocking a command
// that was actually fine.
func SplitTopLevel(command string) []string {
	var parts []string
	var current strings.Builder
	var quote byte
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' && i+1 < len(command) {
				current.WriteByte(c)
				i++
				current.WriteByte(command[i])
				continue
			}
			if c == quote {
				quote = 0
			}
			current.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			current.WriteByte(c)
		case c == '\\' && i+1 < len(command):
			current.WriteByte(c)
			i++
			current.WriteByte(command[i])
		case c == ';' || c == '\n':
			parts = append(parts, current.String())
			current.Reset()
		case c == '&' || c == '|':
			// `2>&1` is a redirection, not a separator: only treat & or |
			// as a separator when it starts a command boundary.
			if c == '&' && i > 0 && command[i-1] == '>' {
				current.WriteByte(c)
				continue
			}
			parts = append(parts, current.String())
			current.Reset()
			if i+1 < len(command) && command[i+1] == c {
				i++
			}
		default:
			current.WriteByte(c)
		}
	}
	parts = append(parts, current.String())

	var out []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Offender returns the guarded command found inside a compound command,
// and whether there was one. A single command is always fine, however
// sensitive -- the rule is about chaining, not about the command.
func Offender(command string, allowedPrefixes []string) (string, bool) {
	parts := SplitTopLevel(command)
	if len(parts) < 2 {
		return "", false
	}
	for _, part := range parts {
		for _, sensitive := range sensitivePrefixes {
			if !strings.HasPrefix(part, sensitive) {
				continue
			}
			// Guard it only if the user actually granted it. Without an
			// allow rule there is no rule to lose, so chaining costs
			// nothing and blocking would be noise.
			for _, granted := range allowedPrefixes {
				if strings.HasPrefix(sensitive, granted) || strings.HasPrefix(granted, sensitive) {
					return sensitive, true
				}
			}
		}
	}
	return "", false
}
