package iterrun

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/plangraph"
)

// BackfillRow is one plan's outcome in a depends: backfill.
type BackfillRow struct {
	Plan   string   `json:"plan"`
	Action string   `json:"action"` // propose | applied | keep | skip | unresolved | none
	Line   string   `json:"line,omitempty"`
	Refs   []string `json:"refs,omitempty"`
	Source string   `json:"source,omitempty"`
	Prose  string   `json:"prose,omitempty"`
	File   string   `json:"file"`
}

// BackfillReport is the whole backfill of one project.
type BackfillReport struct {
	Project string        `json:"project"`
	Apply   bool          `json:"apply"`
	Rows    []BackfillRow `json:"rows"`
}

// Count returns how many rows carry the action.
func (r BackfillReport) Count(action string) int {
	n := 0
	for _, row := range r.Rows {
		if row.Action == action {
			n++
		}
	}
	return n
}

var (
	reMilestoneID       = regexp.MustCompile(`\b[A-Z][A-Z0-9]*\d+-\d+\b`)
	reMilestoneHeading  = regexp.MustCompile(`^#{2,4}\s+([A-Z][A-Z0-9]*\d+-\d+)\b`)
	reDependsOnLine     = regexp.MustCompile(`^\s*(?:[-*]\s+)?\*{0,2}Depends on:\*{0,2}\s*(.*)$`)
	reFromPlan          = regexp.MustCompile(`\((?:from )?plans? ([^)]+)\)`)
	reFromMilestone     = regexp.MustCompile(`\(from milestones? ([^)]+)\)`)
	rePlanName          = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	reNothing           = regexp.MustCompile(`(?i)^(nothing|none|—|-)\b`)
	reRunningKey        = regexp.MustCompile(`(?m)^running:[ \t]*([^\r\n]*)`)
	reConductorOwnerKey = regexp.MustCompile(`(?m)^conductor-assignment:[ \t]*([^\r\n]*)`)
)

// milestoneDepends reads docs/milestones.md: each `### <ID> — …` section's
// `**Depends on:**` line, internal ids only. An `(external)` item names no
// plan and is dropped. A section whose line reads "nothing" maps to none.
func milestoneDepends(projectDir string) map[string][]string {
	b, err := os.ReadFile(filepath.Join(projectDir, "docs", "milestones.md"))
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	current := ""
	for _, line := range strings.Split(string(b), "\n") {
		if m := reMilestoneHeading.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if current == "" {
			continue
		}
		m := reDependsOnLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value := strings.TrimSpace(m[1])
		refs := []string{}
		for _, item := range strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == ',' }) {
			if strings.Contains(item, "(external)") {
				continue
			}
			for _, id := range reMilestoneID.FindAllString(item, -1) {
				if id != current && !slices.Contains(refs, id) {
					refs = append(refs, id)
				}
			}
		}
		if _, seen := out[current]; !seen {
			out[current] = refs
		}
		current = ""
	}
	return out
}

// planIsLive reports a plan another session or the conductor is running
// right now: a fresh heartbeat, `running: true`, or a conductor assignment.
func planIsLive(text string, now time.Time) bool {
	if m := reConductorOwnerKey.FindStringSubmatch(text); m != nil && strings.TrimSpace(m[1]) != "" {
		return true
	}
	if m := reRunningKey.FindStringSubmatch(text); m != nil {
		v := strings.TrimSpace(m[1])
		if v == "true" {
			return true
		}
		if at, err := time.Parse(time.RFC3339, v); err == nil && now.Sub(at) < 90*time.Second {
			return true
		}
	}
	return false
}

// keyBlockEnd returns the index of the first "## " heading line, or the
// line count when there is none.
func keyBlockEnd(lines []string) int {
	for i, l := range lines {
		if strings.HasPrefix(l, "## ") {
			return i
		}
	}
	return len(lines)
}

// deriveDepends reads a plan's dependency sources: its milestone's line in
// docs/milestones.md, and its `Depends on:` prose naming `(from plan X)`,
// `(plan X)`, `(from milestone M…)` or "nothing". It returns the references, whether
// the plan declared it depends on nothing, the source description, and
// the prose it could not resolve.
func deriveDepends(name string, milestones []string, lines []string, doc map[string][]string) (refs []string, none bool, sources []string, unresolved string) {
	add := func(r string) {
		if r != "" && r != name && !slices.Contains(milestones, r) && !slices.Contains(refs, r) {
			refs = append(refs, r)
		}
	}
	docNone := len(milestones) > 0
	for _, milestone := range milestones {
		ids, ok := doc[milestone]
		if !ok {
			docNone = false
			continue
		}
		for _, id := range ids {
			add(id)
		}
		if len(ids) > 0 {
			docNone = false
		}
		sources = append(sources, "docs/milestones.md "+milestone)
	}
	if docNone && len(sources) > 0 {
		none = true
	}
	end := keyBlockEnd(lines)
	prose := []string{}
	for _, l := range lines[end:] {
		m := reDependsOnLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[1])
		found := false
		for _, fm := range reFromPlan.FindAllStringSubmatch(text, -1) {
			for _, part := range strings.FieldsFunc(fm[1], func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
				part = strings.Trim(part, "`'\"")
				if part == "and" || part == "" {
					continue
				}
				if rePlanName.MatchString(part) {
					add(part)
					found = true
				}
			}
		}
		for _, fm := range reFromMilestone.FindAllStringSubmatch(text, -1) {
			for _, id := range reMilestoneID.FindAllString(fm[1], -1) {
				add(id)
				found = true
			}
		}
		if found {
			if !slices.Contains(sources, "Depends on (from …)") {
				sources = append(sources, "Depends on (from …)")
			}
			continue
		}
		if reNothing.MatchString(text) {
			none = true
			if !slices.Contains(sources, "Depends on: nothing") {
				sources = append(sources, "Depends on: nothing")
			}
			continue
		}
		prose = append(prose, text)
	}
	if len(refs) > 0 {
		none = false
	}
	if len(refs) == 0 && !none && len(prose) > 0 {
		unresolved = strings.Join(prose, " / ")
	}
	return refs, none, sources, unresolved
}

// BackfillDepends derives a `depends:` line for every live plan that lacks
// one. Without apply it changes nothing; with apply it inserts the line
// into each plan's key block by atomic replace, leaving every other byte as
// it was. A plan that already has a depends: line is kept as written, and a
// live plan is skipped.
func BackfillDepends(projectDir string, apply bool, now time.Time) (BackfillReport, error) {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return BackfillReport{}, err
	}
	rep := BackfillReport{Project: abs, Apply: apply, Rows: []BackfillRow{}}
	dir := filepath.Join(abs, ".claude", "iterate", "plans")
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return rep, err
	}
	if _, err := os.Stat(dir); err != nil {
		return rep, err
	}
	sort.Strings(files)
	doc := milestoneDepends(abs)
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return rep, err
		}
		text := string(raw)
		lines := strings.Split(text, "\n")
		end := keyBlockEnd(lines)
		var kb plangraph.KeyBlock
		for _, l := range lines[:end] {
			kb.Apply(l)
		}
		name := kb.Name
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		row := BackfillRow{Plan: name, File: path}
		switch {
		case kb.Declared:
			row.Action = "keep"
			for _, l := range lines[:end] {
				if strings.HasPrefix(l, "depends:") {
					row.Line = strings.TrimRight(l, "\r")
				}
			}
		case planIsLive(text, now):
			row.Action, row.Source = "skip", "live run"
		default:
			refs, none, sources, unresolved := deriveDepends(name, kb.Milestones, lines, doc)
			row.Refs, row.Source = refs, strings.Join(sources, "; ")
			switch {
			case len(refs) > 0:
				row.Line = "depends: " + strings.Join(refs, ", ")
			case none:
				row.Line = "depends: none"
			case unresolved != "":
				row.Action, row.Prose = "unresolved", unresolved
			default:
				row.Action = "none"
			}
			if row.Line != "" {
				row.Action = "propose"
				if apply {
					if err := insertKeyLine(path, raw, row.Line); err != nil {
						return rep, err
					}
					row.Action = "applied"
				}
			}
		}
		rep.Rows = append(rep.Rows, row)
	}
	return rep, nil
}

// insertKeyLine writes line into the plan's key block — after its
// milestone: line when it has one, else after the last key line — and
// replaces the file atomically with its mode kept.
func insertKeyLine(path string, raw []byte, line string) error {
	lines := strings.Split(string(raw), "\n")
	end := keyBlockEnd(lines)
	at := -1
	for i := 0; i < end; i++ {
		if strings.HasPrefix(lines[i], "milestone:") {
			at = i
		}
	}
	if at < 0 {
		for i := 0; i < end; i++ {
			if reKeyLine.MatchString(lines[i]) {
				at = i
			}
		}
	}
	out := make([]string, 0, len(lines)+1)
	if at < 0 {
		out = append(out, line)
		out = append(out, lines...)
	} else {
		out = append(out, lines[:at+1]...)
		out = append(out, line)
		out = append(out, lines[at+1:]...)
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(strings.Join(out, "\n")); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

var reKeyLine = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*:[ \t]`)
