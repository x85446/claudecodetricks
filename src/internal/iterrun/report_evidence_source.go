package iterrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// reportPlanSource is what the progress and evidence sections read from a
// plan's own files beyond ReportInput: each numbered requirement's
// checkboxes, the Status / Log entries, and every validation report and
// terminal line the team logs carry. Reading it never writes.
type reportPlanSource struct {
	// Read is false when the plan file could not be read.
	Read        bool
	Boxes       map[int]boxState
	Steps       map[int]string
	Validations map[int]string
	Teams       map[string]teamMeta
	Log         []planLogEntry
	// Reports holds every ##ITERATE-VALIDATION## line in team-log order,
	// earlier reports included: a later report supersedes an earlier one
	// for the same step without erasing it.
	Reports []teamValidationReport
	// TeamEnds maps a team to "done" or "blocked" from its log's terminal
	// line.
	TeamEnds map[string]string
}

// planLogEntry is one bullet of a plan's Status / Log section.
type planLogEntry struct {
	At      *time.Time
	Text    string
	Line    int    // 1-based line in the plan file
	Context string // the "### " heading the bullet sits under, if any
}

// teamValidationReport is one ##ITERATE-VALIDATION## line: a team's own
// assessment of one validation. It carries no time of its own.
type teamValidationReport struct {
	Team   string
	Step   int
	Status string // met | partial | not-met
	Note   string
	Seq    int // position within the team's log
}

// reportPlanSourceFor reads a plan's source files. Tests replace it.
var reportPlanSourceFor = readReportPlanSource

// evidenceChangedFiles lists the files under project that differ from
// revision. Tests replace it with a fixed change source.
var evidenceChangedFiles = gitChangedFiles

// reportPlanPaths locates the plan file and its teams directory for the
// execution a summary describes.
func reportPlanPaths(p PlanSummary) (planFile, teamsDir string) {
	if p.ProjectDir == "" || p.Name == "" {
		return "", ""
	}
	if p.Archived && p.ArchiveFile != "" {
		if filepath.Base(p.ArchiveFile) != p.ArchiveFile {
			return "", ""
		}
		dir := filepath.Join(p.ProjectDir, ".claude", "iterate", "archive")
		return filepath.Join(dir, p.ArchiveFile), filepath.Join(dir, strings.TrimSuffix(p.ArchiveFile, ".md")+".teams")
	}
	if filepath.Base(p.Name) != p.Name {
		return "", ""
	}
	dir := filepath.Join(p.ProjectDir, ".claude", "iterate", "plans")
	return filepath.Join(dir, p.Name+".md"), filepath.Join(dir, p.Name+".teams")
}

var (
	reLogBullet = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	reLogStamp  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}(?:[T ]\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?)?)\s*(?:[:—–-]\s*)?`)
)

func readReportPlanSource(p PlanSummary) reportPlanSource {
	src := reportPlanSource{TeamEnds: map[string]string{}}
	planFile, teamsDir := reportPlanPaths(p)
	if planFile == "" {
		return src
	}
	data, err := os.ReadFile(planFile)
	if err != nil {
		return src
	}
	src.Read = true
	src.Teams, src.Steps, src.Validations, _, _, src.Boxes = readPlanTeamsStepsAndBoxes(planFile)
	src.Log = parsePlanLog(string(data))

	logs, _ := filepath.Glob(filepath.Join(teamsDir, "*.log.md"))
	sort.Strings(logs)
	for _, lf := range logs {
		team := strings.TrimSuffix(filepath.Base(lf), ".log.md")
		b, err := os.ReadFile(lf)
		if err != nil {
			continue
		}
		src.Reports = append(src.Reports, parseTeamValidationReports(team, string(b))...)
		switch teamTerminalLine.FindString(string(b)) {
		case "TEAM DONE:":
			src.TeamEnds[team] = "done"
		case "TEAM BLOCKED:":
			src.TeamEnds[team] = "blocked"
		}
	}
	return src
}

// parsePlanLog splits a plan's Status / Log section into bullets. A line
// that does not start a bullet continues the one before it.
func parsePlanLog(text string) []planLogEntry {
	var out []planLogEntry
	in := false
	heading := ""
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			switch trimmed {
			case "## Status / Log", "## Status/Log":
				in = true
			default:
				in = false
			}
			heading = ""
			continue
		}
		if !in || trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			heading = strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			continue
		}
		if m := reLogBullet.FindStringSubmatch(line); m != nil {
			e := planLogEntry{Line: i + 1, Context: heading, Text: strings.TrimSpace(m[1])}
			if s := reLogStamp.FindStringSubmatch(e.Text); s != nil {
				if t, ok := parseLogStamp(s[1]); ok {
					e.At = &t
					e.Text = strings.TrimSpace(e.Text[len(s[0]):])
				}
			}
			out = append(out, e)
			continue
		}
		if n := len(out); n > 0 {
			out[n-1].Text += " " + trimmed
		}
	}
	return out
}

func parseLogStamp(s string) (time.Time, bool) {
	s = strings.Replace(s, " ", "T", 1)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04Z07:00", "2006-01-02T15:04Z", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// parseTeamValidationReports reads every ##ITERATE-VALIDATION## line of a
// team log, in order. readValidationMarkers keeps only the last per step;
// evidence needs the earlier ones too.
func parseTeamValidationReports(team, text string) []teamValidationReport {
	var out []teamValidationReport
	for line := range strings.SplitSeq(text, "\n") {
		_, after, found := strings.Cut(line, validationMarker)
		if !found {
			continue
		}
		var m struct {
			Step   int    `json:"step"`
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(after)), &m) != nil || m.Step <= 0 {
			continue
		}
		if m.Status != "met" && m.Status != "partial" && m.Status != "not-met" {
			continue
		}
		out = append(out, teamValidationReport{Team: team, Step: m.Step, Status: m.Status, Note: m.Note, Seq: len(out)})
	}
	return out
}

// logSegment is part of one log entry: the text from a step mention up to
// the next mention, or the entry's text before its first mention.
type logSegment struct {
	Steps []int // nil for text that names no requirement
	Text  string
}

// reStepMention matches "step 4", "Step6", "steps 2-3", "Steps 3, 4, 12"
// and "steps6/12". Only numbers that name a known requirement count.
var reStepMention = regexp.MustCompile(`(?i)\bsteps?\s*(\d{1,3})((?:\s*(?:,|/|&|\band\b|-|–|\bto\b)\s*\d{1,3}\b)*)`)
var reMentionPart = regexp.MustCompile(`(?i)(-|–|\bto\b)?\s*(\d{1,3})`)

// splitLogEntry splits one log entry at each step mention, so a line that
// reports steps 4, 6 and 7 yields three segments, never one note.
func splitLogEntry(text string, known map[int]bool) []logSegment {
	type mention struct {
		start int
		steps []int
	}
	var ms []mention
	for _, loc := range reStepMention.FindAllStringSubmatchIndex(text, -1) {
		steps := mentionSteps(text[loc[2]:loc[3]]+text[loc[4]:loc[5]], known)
		if len(steps) > 0 {
			ms = append(ms, mention{start: loc[0], steps: steps})
		}
	}
	if len(ms) == 0 {
		return []logSegment{{Text: strings.TrimSpace(text)}}
	}
	var out []logSegment
	if pre := strings.TrimSpace(text[:ms[0].start]); pre != "" {
		out = append(out, logSegment{Text: pre})
	}
	for i, m := range ms {
		end := len(text)
		if i+1 < len(ms) {
			end = ms[i+1].start
		}
		out = append(out, logSegment{Steps: m.steps, Text: strings.TrimSpace(text[m.start:end])})
	}
	return out
}

func mentionSteps(list string, known map[int]bool) []int {
	var out []int
	prev := 0
	for _, p := range reMentionPart.FindAllStringSubmatch(list, -1) {
		n, err := strconv.Atoi(p[2])
		if err != nil {
			continue
		}
		if p[1] != "" && prev > 0 && n > prev && n-prev <= 50 {
			for k := prev + 1; k <= n; k++ {
				if known[k] {
					out = append(out, k)
				}
			}
		} else if known[n] {
			out = append(out, n)
		}
		prev = n
	}
	return out
}

// reEvidenceSignal finds words that report a result. Only a leading word
// boundary is required, because some logs run words together
// ("passes24PDFs").
var reEvidenceSignal = regexp.MustCompile(`(?i)\b(pass|fail|done\b|met\b|partial|blocked|green\b|exit|verif|succe|merge|squash-merge)`)

func hasEvidenceSignal(text string) bool { return reEvidenceSignal.MatchString(text) }

// reportedClaim reads the status a segment's first clause states: "step 4
// partial:", "step 13 DONE", "step2 ci.retained.moved passed". It is what
// the prose claims, never an observed result.
func reportedClaim(text string) string {
	clause := text
	if loc := reClauseEnd.FindStringIndex(clause); loc != nil && loc[0] > 0 {
		clause = clause[:loc[0]]
	}
	lc := strings.ToLower(clause)
	switch {
	case reClaimNotMet.MatchString(lc):
		return "not-met"
	case strings.Contains(lc, "partial"):
		return "partial"
	case reClaimMet.MatchString(lc):
		return "met"
	}
	return ""
}

var (
	reClaimNotMet = regexp.MustCompile(`\b(not[- ]met|blocked|failed|fails)\b`)
	reClaimMet    = regexp.MustCompile(`\b(done|met|passed|passes|pass|green|complete|completed)\b`)
	reMergeWord   = regexp.MustCompile(`(?i)merge`)
	// A clause ends at a colon, semicolon or parenthesis, or at a period
	// that ends a sentence; "bake.cleanup.green" stays one word.
	reClauseEnd = regexp.MustCompile(`[:;(]|\.(\s|$)`)
)

// EvidenceFact is one figure pulled out of imported prose so a reader can
// find it without reading the whole line: a test count, an exit status, a
// revision or artifact hash, a merge reference.
type EvidenceFact struct {
	Kind string `json:"kind"` // tests | exit | revision | merge
	Text string `json:"text"`
}

var (
	reFactTests    = regexp.MustCompile(`(?i)(?:^|[^0-9.,])(\d[\d,]*\s*(?:[a-z/+-]{0,30}?\s*)?(?:tests?|cases?))\b`)
	reFactPassFail = regexp.MustCompile(`(?i)\b(\d+)\s*passed\s*[,/]?\s*(\d+)\s*failed\b`)
	reFactFailures = regexp.MustCompile(`(?i)\b(\d+|zero)\s*failures?\b`)
	reFactExit     = regexp.MustCompile(`(?i)\bexit(?:\s*code|\s*status)?\s*[=:]?\s*(\d+)\b`)
	reFactRevision = regexp.MustCompile(`(?i)(?:commit|sha|head|revision|build|as|source|hash)\s*:?\s*([0-9a-f]{7,40})\b`)
	reFactLongHex  = regexp.MustCompile(`\b([0-9a-f]{12,40})\b`)
	reFactPR       = regexp.MustCompile(`(?i)\b(?:PR\s*#?\s*(\d+)|pull/(\d+))`)
	reHasDigit     = regexp.MustCompile(`[0-9]`)
	reHasHexLetter = regexp.MustCompile(`[a-f]`)
)

// evidenceFacts extracts the figures a prose line records. It reads; it
// never infers a result.
func evidenceFacts(text string) []EvidenceFact {
	var out []EvidenceFact
	seen := map[string]bool{}
	add := func(kind, s string) {
		k := kind + "\x00" + s
		if !seen[k] {
			seen[k] = true
			out = append(out, EvidenceFact{Kind: kind, Text: s})
		}
	}
	for _, m := range reFactPassFail.FindAllStringSubmatch(text, -1) {
		add("tests", m[1]+" passed, "+m[2]+" failed")
	}
	for _, m := range reFactTests.FindAllStringSubmatch(text, -1) {
		add("tests", strings.TrimSpace(m[1]))
	}
	for _, m := range reFactFailures.FindAllString(text, -1) {
		add("tests", strings.TrimSpace(m))
	}
	for _, m := range reFactExit.FindAllStringSubmatch(text, -1) {
		add("exit", "exit "+m[1])
	}
	hashes := map[string]bool{}
	for _, m := range reFactRevision.FindAllStringSubmatch(text, -1) {
		if isRevisionLike(m[1]) {
			hashes[strings.ToLower(m[1])] = true
			add("revision", strings.ToLower(m[1]))
		}
	}
	for _, m := range reFactLongHex.FindAllStringSubmatch(text, -1) {
		h := strings.ToLower(m[1])
		if isRevisionLike(h) && !hashes[h] {
			hashes[h] = true
			add("revision", h)
		}
	}
	if reMergeWord.MatchString(text) {
		for _, m := range reFactPR.FindAllStringSubmatch(text, -1) {
			n := m[1]
			if n == "" {
				n = m[2]
			}
			add("merge", "PR "+n)
		}
	}
	return out
}

func isRevisionLike(s string) bool {
	return reHasDigit.MatchString(s) && reHasHexLetter.MatchString(strings.ToLower(s))
}

var reRevisionArg = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// gitChangedFiles lists files under project that differ between revision
// and the working tree, relative to project. GIT_OPTIONAL_LOCKS=0 keeps
// git from refreshing the index, so the read writes nothing.
func gitChangedFiles(project, revision string) ([]string, error) {
	if !reRevisionArg.MatchString(revision) {
		return nil, errors.New("revision is not a commit hash")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", project, "diff", "--name-only", "--no-renames", "--relative", revision, "--")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("revision " + shortRevision(revision) + " is not in this project's history")
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, filepath.ToSlash(l))
		}
	}
	return files, nil
}

func shortRevision(r string) string {
	if len(r) > 12 {
		return r[:12]
	}
	return r
}
