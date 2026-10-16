package conductor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"os/exec"
	"strings"
)

func intakeName(item Intake) string {
	h := sha256.Sum256([]byte(item.Key))
	return "issue-" + hex.EncodeToString(h[:6])
}

// FetchIssues uses the installed forge client for read-only issue intake.
func (s *Service) FetchIssues(ctx context.Context, projectID string) (int, error) {
	c, e := s.Config()
	if e != nil {
		return 0, e
	}
	for _, p := range c.Projects {
		if p.ID != projectID {
			continue
		}
		if !p.IntakeEnabled {
			return 0, errors.New("issue intake disabled")
		}
		read := s.IssueReader
		if read == nil {
			read = readForgeIssues
		}
		items, e := read(ctx, p)
		_ = s.withLock(func() error {
			r, x := s.runtime()
			if x != nil {
				return x
			}
			r.IntakePoll[projectID] = s.now()
			if e != nil {
				key := "intake:" + projectID + ":" + e.Error()
				if _, ok := r.Notices[key]; !ok {
					r.Notices[key] = Notice{Key: key, ProjectID: projectID, Message: e.Error(), At: s.now()}
				}
			}
			return s.saveRuntime(&r)
		})
		if e != nil {
			return 0, e
		}
		return s.ImportIssues(projectID, items)
	}
	return 0, errors.New("project not found")
}
func readForgeIssues(ctx context.Context, p model.Project) ([]Intake, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = p.Path
	b, e := cmd.Output()
	if e != nil {
		return nil, errors.New("issue intake requires an origin remote")
	}
	remote := strings.TrimSpace(string(b))
	forge := ""
	repo := ""
	for _, host := range []string{"github.com", "gitlab.com"} {
		i := strings.Index(remote, host)
		if i >= 0 {
			forge = strings.TrimSuffix(host, ".com")
			repo = strings.TrimLeft(remote[i+len(host):], ":/")
			repo = strings.TrimSuffix(repo, ".git")
		}
	}
	if repo == "" {
		return nil, errors.New("issue intake supports explicit GitHub or GitLab origin")
	}
	limit := fmt.Sprint(p.MaxImport)
	if forge == "github" {
		cmd = exec.CommandContext(ctx, "gh", "issue", "list", "--repo", repo, "--state", "open", "--limit", limit, "--json", "number,title,body")
	} else {
		cmd = exec.CommandContext(ctx, "glab", "issue", "list", "--repo", repo, "--per-page", limit, "--output", "json")
	}
	cmd.Dir = p.Path
	b, e = cmd.Output()
	if e != nil {
		return nil, fmt.Errorf("%s issue read capability unavailable", forge)
	}
	if len(b) > 4<<20 {
		return nil, errors.New("issue response exceeds 4 MiB")
	}
	var rows []struct {
		Number      int    `json:"number"`
		IID         int    `json:"iid"`
		Title       string `json:"title"`
		Body        string `json:"body"`
		Description string `json:"description"`
	}
	if e = json.Unmarshal(b, &rows); e != nil {
		return nil, e
	}
	out := []Intake{}
	for _, row := range rows {
		n := row.Number
		if n == 0 {
			n = row.IID
		}
		body := row.Body
		if body == "" {
			body = row.Description
		}
		out = append(out, Intake{Provider: forge, Repository: repo, Issue: fmt.Sprint(n), Title: row.Title, Body: body})
	}
	return out, nil
}
