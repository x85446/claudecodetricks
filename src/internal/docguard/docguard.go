// Package docguard finds change narration in documentation: sentences that
// record how the system used to be instead of stating how it is.
//
// A document describes the present. The change itself lives in git, so a
// sentence like "X now reads the key block" or "previously this was prose"
// tells the reader nothing about the system that the sentence "X reads the
// key block" does not, and it ages into noise the moment the next change
// lands. Models write these by reflex when they edit a doc during a change,
// so this is a mechanical check rather than a rule somebody has to remember.
//
// Precision matters more than recall: every hit is handed back to the model
// as work, so the patterns name phrasing that is almost always narration.
// Quoted text and code are skipped, because a rule that lists the banned
// phrases, or an example of output, quotes them on purpose.
package docguard

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Hit is one sentence of change narration.
type Hit struct {
	Line   int    // 1-based line in the scanned text
	Phrase string // the matched phrase
	Text   string // the line, trimmed
}

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bno longer\b`),
	regexp.MustCompile(`(?i)\bpreviously\b`),
	regexp.MustCompile(`(?i)\bused to (be|have|say|show|live|read|write|call|look|run|use|do)\b`),
	regexp.MustCompile(`(?i)\b(was|were|has been|have been|got) (changed|renamed|moved|replaced|removed|updated|switched|rewritten|reworked|dropped|deprecated|migrated|retired)\b`),
	regexp.MustCompile(`(?i)\b(we|i) (changed|renamed|moved|replaced|removed|updated|switched|rewrote|reworked|dropped|migrated|retired)\b`),
	regexp.MustCompile(`(?i)\brenamed (from|to)\b`),
	regexp.MustCompile(`(?i)\bformerly\b`),
	regexp.MustCompile(`(?i)\brecently (added|changed|switched|moved|introduced|renamed|removed|updated|rewritten)\b`),
	regexp.MustCompile(`(?i)\b(is|are) now\b`),
	regexp.MustCompile(`(?i)\bnow (uses|supports|reads|lives|runs|writes|calls|shows|returns|requires|defaults|handles|lets|accepts)\b`),
	regexp.MustCompile(`(?i)\bchanged from\b`),
	regexp.MustCompile(`(?i)\bswitched (from|over)\b`),
	regexp.MustCompile(`(?i)\bas of (v?\d|this (release|version|change|commit|update))`),
	regexp.MustCompile(`(?i)^\s*(>\s*)?(\*\*|__)?(update|updated|changed|new)(\*\*|__)?\s*:`),
	regexp.MustCompile(`(?i)\b(the|an?) (old|previous|earlier|original|prior) (version|behaviou?r|approach|implementation|way|design|format|name|wording|layout|rule)\b`),
	regexp.MustCompile(`(?i)\bsince removed\b`),
	regexp.MustCompile(`(?i)\banymore\b`),
	regexp.MustCompile(`(?i)\bno more\b`),
}

// "no longer than 50 characters" is a length, not a history.
var lengthPhrase = regexp.MustCompile(`(?i)\bno (longer|more) than\b`)

// quoted spans are skipped: "…", “…”, `…`
var quoted = regexp.MustCompile("\"[^\"\n]*\"|“[^”\n]*”|`[^`\n]*`")

// Scan returns every line of text that narrates a change. Fenced code
// blocks, inline code and quoted phrases are ignored.
func Scan(text string) []Hit {
	var hits []Hit
	fence := false
	for i, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		bare := lengthPhrase.ReplaceAllString(quoted.ReplaceAllString(line, " "), " ")
		for _, p := range patterns {
			if m := p.FindString(bare); m != "" {
				hits = append(hits, Hit{Line: i + 1, Phrase: strings.TrimSpace(m), Text: t})
				break
			}
		}
	}
	return hits
}

var docExt = map[string]bool{".md": true, ".mdx": true, ".markdown": true, ".rst": true, ".adoc": true, ".txt": true}

// historyName matches files whose whole job is recording change.
var historyName = regexp.MustCompile(`(?i)^(changelog|changes|history|news|release[-_ ]?notes|releases)\b`)

// Applies reports whether path is documentation that must describe the
// present. Records of change are exempt: changelogs, and the per-project
// state Claude keeps under .claude/ (plans and their logs, archives, notes,
// inbox messages, product state and drift logs, test registries), except
// CLAUDE.md, skills and agent definitions, which are documentation. ADR
// logs are decision records and are exempt too.
func Applies(path string) bool {
	base := filepath.Base(path)
	if !docExt[strings.ToLower(filepath.Ext(base))] && !strings.HasPrefix(strings.ToUpper(base), "README") {
		return false
	}
	if historyName.MatchString(base) {
		return false
	}
	p := filepath.ToSlash(path)
	for _, seg := range []string{"/adr/", "/adrs/", "/decisions/"} {
		if strings.Contains(strings.ToLower(p), seg) {
			return false
		}
	}
	if strings.Contains(p, "/.claude/") || strings.HasPrefix(p, ".claude/") {
		// top-level docs (CLAUDE.md and the files it imports), skills and
		// agent definitions are documentation; every other subdirectory is state
		if filepath.Base(filepath.Dir(p)) == ".claude" || base == "CLAUDE.md" ||
			strings.Contains(p, "/skills/") || strings.Contains(p, "/agents/") {
			return true
		}
		return false
	}
	return true
}
