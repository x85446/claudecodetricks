package testmaster

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// catalog.json belongs to /testmaster-catalog. The binary reads it to join
// cases to registered tests, and writes only the validation stamps.

// readCatalog decodes catalog.json keeping numbers and unknown fields as
// they are. A project without a catalog returns nil, nil.
func (p *Project) readCatalog() (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(p.Root, StateDir, "catalog.json"))
	if err != nil {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var cat map[string]any
	if err := dec.Decode(&cat); err != nil {
		return nil, fmt.Errorf("catalog.json: %w", err)
	}
	return cat, nil
}

// catalogCases returns every case object, in catalog order.
func catalogCases(cat map[string]any) []map[string]any {
	var out []map[string]any
	reqs, _ := cat["requirements"].([]any)
	for _, r := range reqs {
		req, _ := r.(map[string]any)
		cases, _ := req["cases"].([]any)
		for _, c := range cases {
			if cs, ok := c.(map[string]any); ok {
				out = append(out, cs)
			}
		}
	}
	return out
}

// caseTests returns the registry ids a case's `test` names: one id, or a
// list when several tests back the case together.
func caseTests(cs map[string]any) []string {
	switch v := cs["test"].(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []any:
		var out []string
		for _, t := range v {
			if s, ok := t.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// Unlinked is a catalog case whose `test` names a test the registry does not
// have: the test is unregistered, deleted, or the link is not a registry id.
type Unlinked struct {
	Case, Test string
}

// Unlinked lists every catalog link that names no registered test, in
// catalog order.
func (p *Project) Unlinked() ([]Unlinked, error) {
	cat, err := p.readCatalog()
	if cat == nil {
		return nil, err
	}
	var out []Unlinked
	for _, cs := range catalogCases(cat) {
		id, _ := cs["id"].(string)
		for _, t := range caseTests(cs) {
			if p.Reg.Tests[t] == nil {
				out = append(out, Unlinked{Case: id, Test: t})
			}
		}
	}
	return out, nil
}
