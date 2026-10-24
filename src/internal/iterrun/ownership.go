package iterrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/plangraph"
)

// ExecutionOwner identifies the process that holds a project's execution lease.
type ExecutionOwner struct {
	Plan      string    `json:"plan"`
	Owner     string    `json:"owner"`
	PID       int       `json:"pid"`
	Identity  string    `json:"identity"`
	Heartbeat time.Time `json:"heartbeat"`
	Live      bool      `json:"live"`
}

func ownerField(text, key string) string {
	head, _, _ := strings.Cut(text, "\n## ")
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:[ \t]*([^\r\n]*)`).FindStringSubmatch(head)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func readOwner(path string, now time.Time) (ExecutionOwner, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ExecutionOwner{}, err
	}
	s := string(b)
	o := ExecutionOwner{Plan: strings.TrimSuffix(filepath.Base(path), ".md"), Owner: ownerField(s, "owner"), Identity: ownerField(s, "owner-start")}
	if o.Owner == "" && ownerField(s, "conductor-assignment") != "" {
		o.Owner = "conductor"
	}
	o.PID, _ = strconv.Atoi(ownerField(s, "owner-pid"))
	o.Heartbeat, _ = time.Parse(time.RFC3339, ownerField(s, "heartbeat"))
	if o.Heartbeat.IsZero() {
		o.Heartbeat, _ = time.Parse(time.RFC3339, ownerField(s, "running"))
	}
	if ownerField(s, "running") == "false" {
		return o, nil
	}
	if o.PID > 0 {
		identity := ownerProcessIdentity(o.PID)
		// A matching live process keeps ownership during a delayed heartbeat.
		o.Live = identity != "" && o.Identity != "" && identity == o.Identity
	} else {
		o.Live = !o.Heartbeat.IsZero() && now.Sub(o.Heartbeat) >= 0 && now.Sub(o.Heartbeat) < 90*time.Second
	}
	child, _ := strconv.Atoi(ownerField(s, "owner-child-pid"))
	if child > 0 {
		identity := ownerProcessIdentity(child)
		if identity != "" && identity == ownerField(s, "owner-child-start") {
			o.Live = true
		}
	}
	return o, nil
}

// ProjectExecutionOwner reads live ownership without changing lease evidence.
func ProjectExecutionOwner(project string, now time.Time) (ExecutionOwner, error) {
	paths, err := filepath.Glob(filepath.Join(project, ".claude", "iterate", "plans", "*.md"))
	if err != nil {
		return ExecutionOwner{}, err
	}
	for _, path := range paths {
		o, e := readOwner(path, now)
		if e != nil {
			return o, e
		}
		if o.Live {
			return o, nil
		}
	}
	return ExecutionOwner{}, nil
}

func ownershipLocked(project string, fn func() error) error {
	path := filepath.Join(project, ".claude", "iterate", "execution.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = ownerLockFile(f); err != nil {
		return err
	}
	defer ownerUnlockFile(f)
	return fn()
}

func writeOwnerPlan(path, text string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".owner-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if st, e := os.Stat(path); e == nil {
		_ = f.Chmod(st.Mode().Perm())
	}
	if _, err = f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// ClaimPlan atomically admits one ready plan and excludes every live project owner.
func ClaimPlan(project, name, owner string, pid int, now time.Time) (ExecutionOwner, error) {
	var result ExecutionOwner
	err := ownershipLocked(project, func() error {
		if owner != "cli" && owner != "conductor" {
			return fmt.Errorf("owner must be cli or conductor")
		}
		identity := ownerProcessIdentity(pid)
		if identity == "" {
			return fmt.Errorf("owner pid %d has no observable process identity", pid)
		}
		path, err := safePlanPath(project, name)
		if err != nil {
			return err
		}
		graph, err := plangraph.Load(project)
		if err != nil {
			return err
		}
		node, ok := graph.Node(name)
		if !ok || node.Archived {
			return fmt.Errorf("plan %s not found", name)
		}
		if len(graph.Errors) > 0 {
			return fmt.Errorf("plan graph: %s", strings.Join(graph.Errors, "; "))
		}
		if node.Status != plangraph.StatusReady {
			return fmt.Errorf("plan %s: %s", name, node.Reason)
		}
		held, err := ProjectExecutionOwner(project, now)
		if err != nil {
			return err
		}
		if held.Live {
			return fmt.Errorf("project busy: %s owns %s (pid %d)", held.Owner, held.Plan, held.PID)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(b)
		for k, v := range map[string]string{"owner": owner, "owner-pid": strconv.Itoa(pid), "owner-start": identity, "heartbeat": now.UTC().Format(time.RFC3339), "running": now.UTC().Format(time.RFC3339)} {
			text = setPlanField(text, k, v)
		}
		text = setPlanField(text, "owner-child-pid", "")
		text = setPlanField(text, "owner-child-start", "")
		if owner == "cli" {
			text = setPlanField(text, "conductor-assignment", "")
		}
		if err = writeOwnerPlan(path, text); err != nil {
			return err
		}
		result, err = readOwner(path, now)
		return err
	})
	return result, err
}

// SetPlanOwnerChild retains exclusion while an owned child survives its supervisor.
func SetPlanOwnerChild(project, name string, pid int, identity string) error {
	return ownershipLocked(project, func() error {
		path, e := safePlanPath(project, name)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		text := setPlanField(string(b), "owner-child-pid", strconv.Itoa(pid))
		text = setPlanField(text, "owner-child-start", identity)
		return writeOwnerPlan(path, text)
	})
}

// AttachPlanOwnerChild observes a live child's identity for the heartbeat CLI.
func AttachPlanOwnerChild(project, name string, pid int) error {
	identity := ownerProcessIdentity(pid)
	if identity == "" {
		return fmt.Errorf("child pid %d has no observable process identity", pid)
	}
	return SetPlanOwnerChild(project, name, pid, identity)
}

// UpdatePlanOwnership refreshes, releases, or reconciles the named lease.
func UpdatePlanOwnership(project, name, verb string, now time.Time) error {
	return ownershipLocked(project, func() error {
		paths := []string{}
		if name != "" {
			p, e := safePlanPath(project, name)
			if e != nil {
				return e
			}
			paths = append(paths, p)
		} else {
			paths, _ = filepath.Glob(filepath.Join(project, ".claude", "iterate", "plans", "*.md"))
		}
		for _, path := range paths {
			o, err := readOwner(path, now)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(b)
			if verb == "reconcile" && o.Live {
				continue
			}
			if verb == "reconcile" && ownerField(text, "running") == "false" {
				continue
			}
			if verb == "heartbeat" {
				if !o.Live {
					return fmt.Errorf("plan %s has no live owner", o.Plan)
				}
				text = setPlanField(text, "heartbeat", now.UTC().Format(time.RFC3339))
				text = setPlanField(text, "running", now.UTC().Format(time.RFC3339))
			} else {
				text = setPlanField(text, "running", "false")
				if verb == "release" {
					text = setPlanField(text, "conductor-assignment", "")
				}
				if verb == "reconcile" {
					if assignment := ownerField(text, "conductor-assignment"); assignment != "" {
						text = setPlanField(text, "reconciled-assignment", assignment)
						text = setPlanField(text, "conductor-assignment", "")
					}
				}
				if verb == "reconcile" && ownerField(text, "owner") != "" {
					text = setPlanField(text, "checkpoint-reconciliation", "required: inspect retained evidence before replaying side effects")
					text += "\n- " + now.UTC().Format(time.RFC3339) + " — reconciled dead execution owner; checkpoint evidence retained for diagnosis.\n"
				}
			}
			if err = writeOwnerPlan(path, text); err != nil {
				return err
			}
		}
		return nil
	})
}

func safePlanPath(project, name string) (string, error) {
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return "", errors.New("invalid plan name")
	}
	base := filepath.Join(project, ".claude", "iterate", "plans")
	path := filepath.Join(base, name+".md")
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", e
	}
	canonicalBase, e := filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	if filepath.Dir(resolved) != canonicalBase {
		return "", errors.New("plan resolves outside project plan directory")
	}
	return path, nil
}

func setPlanField(text, key, value string) string {
	boundary := strings.Index(text, "\n## ")
	head, tail := text, ""
	if boundary >= 0 {
		head, tail = text[:boundary], text[boundary:]
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:.*$`)
	if re.MatchString(head) {
		return re.ReplaceAllStringFunc(head, func(string) string { return key + ": " + value }) + tail
	}
	return key + ": " + value + "\n" + head + tail
}
