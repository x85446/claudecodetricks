// Package plangraph resolves the dependency graph between one project's
// iterate plans.
//
// A plan names its predecessors in its key block, the lines above the first
// "## " heading:
//
//	depends: caiman, M1-03
//	depends: none
//
// Each reference is a plan name or a milestone id carried by some plan's
// `milestone:` key. An edge is satisfied only when its predecessor finished:
// archived as `-done`, archived as `-landed-rolled-to-<new>` (its green work
// landed), or still live with `phase: complete|done`. An edge to a plan
// archived as `-rolled-to-<new>` follows the roll to `<new>`. Every other
// predecessor state leaves the edge unsatisfied, and the dependent waits.
//
// The graph is computed whenever it is read and is never written into a
// plan, so a waiting state cannot go stale.
package plangraph

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Outcome is how an archived plan ended, read from its archive filename.
type Outcome string

const (
	OutcomeNone    Outcome = ""         // live plan
	OutcomeDone    Outcome = "done"     // <ts>-<name>-done.md
	OutcomeLanded  Outcome = "landed"   // <ts>-<name>-landed-rolled-to-<new>.md
	OutcomeRolled  Outcome = "rolled"   // <ts>-<name>-rolled-to-<new>.md
	OutcomeClosed  Outcome = "closed"   // <ts>-<name>-closed.md
	OutcomeUnknown Outcome = "archived" // any other archive suffix
)

// Graph status of a live plan.
const (
	StatusReady   = "ready"   // every edge satisfied
	StatusWaiting = "waiting" // at least one predecessor is unfinished
	StatusBlocked = "blocked" // the plan's own edges are in error
	StatusDone    = "done"    // an archived or complete predecessor shown for context
	StatusHeld    = "held"    // an archived predecessor that does not satisfy (closed, unknown)
)

// Input is one plan as read from disk.
type Input struct {
	Name      string
	Milestone string
	Depends   []string // references as written
	Declared  bool     // a depends: line exists, including "depends: none"
	State     string   // live: StateOf(phase, status); archived: "archived"
	Archived  bool
	Outcome   Outcome
	RolledTo  string
	Started   string
	File      string
}

// Edge is one resolved dependency.
type Edge struct {
	Ref       string `json:"ref"`              // as written in depends:
	Target    string `json:"target,omitempty"` // the plan that satisfies or holds it, after following rolls
	Satisfied bool   `json:"satisfied"`
	Reason    string `json:"reason,omitempty"` // why an unsatisfied edge holds
}

// Node is one plan in the resolved graph.
type Node struct {
	Name      string   `json:"name"`
	Milestone string   `json:"milestone,omitempty"`
	State     string   `json:"state"`
	Status    string   `json:"status"`
	Archived  bool     `json:"archived"`
	Outcome   Outcome  `json:"outcome,omitempty"`
	Declared  bool     `json:"declared"`
	Depends   []string `json:"depends"`
	Edges     []Edge   `json:"edges"`
	WaitingOn []string `json:"waiting_on"`
	HeldBy    string   `json:"held_by,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Errors    []string `json:"errors"`
	Depth     int      `json:"depth"`
	Started   string   `json:"started,omitempty"`
}

// Graph is a project's resolved plan graph: every live plan, plus each
// archived plan a live edge reaches.
type Graph struct {
	Nodes  []Node   `json:"nodes"`
	Errors []string `json:"errors"`
	index  map[string]int
}

// Node returns the named node.
func (g Graph) Node(name string) (Node, bool) {
	if i, ok := g.index[name]; ok {
		return g.Nodes[i], true
	}
	return Node{}, false
}

// Live returns the live plans in graph order.
func (g Graph) Live() []Node {
	out := []Node{}
	for _, n := range g.Nodes {
		if !n.Archived {
			out = append(out, n)
		}
	}
	return out
}

// Ready returns the live plans whose edges are all satisfied, in admission
// order.
func (g Graph) Ready() []Node {
	out := []Node{}
	for _, n := range g.Nodes {
		if !n.Archived && n.Status == StatusReady {
			out = append(out, n)
		}
	}
	return out
}

// Less orders plans for admission: graph depth, then milestone id, then
// Started, then name. A plan with a milestone sorts before one without.
func Less(a, b Node) bool {
	if a.Depth != b.Depth {
		return a.Depth < b.Depth
	}
	if a.Milestone != b.Milestone {
		if a.Milestone == "" || b.Milestone == "" {
			return a.Milestone != ""
		}
		return MilestoneLess(a.Milestone, b.Milestone)
	}
	if a.Started != b.Started {
		return a.Started < b.Started
	}
	return a.Name < b.Name
}

var reMilestoneParts = regexp.MustCompile(`\d+|\D+`)

// MilestoneLess compares milestone ids with their numbers read as numbers,
// so M1-10 follows M1-09 and M10-01 follows M9-10.
func MilestoneLess(a, b string) bool {
	pa, pb := reMilestoneParts.FindAllString(a, -1), reMilestoneParts.FindAllString(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] == pb[i] {
			continue
		}
		da, db := isDigits(pa[i]), isDigits(pb[i])
		if da && db {
			na, nb := strings.TrimLeft(pa[i], "0"), strings.TrimLeft(pb[i], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			return na < nb
		}
		return pa[i] < pb[i]
	}
	return len(pa) < len(pb)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// ParseDepends splits a depends: value into references. "none", "—" and
// "-" declare no edges.
func ParseDepends(value string) (refs []string, none bool) {
	v := strings.TrimSpace(value)
	switch strings.ToLower(v) {
	case "", "none", "—", "-", "–":
		return nil, true
	}
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' }) {
		part = strings.Trim(part, "`'\"")
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		refs = append(refs, part)
	}
	return refs, len(refs) == 0
}

// StateOf is the one-word state of a live plan, in the precedence the status
// line colours it: a status arm (blocked, paused, unblocked) beats the
// phase, executing beats queued, and queued is a planned plan carrying an
// approval.
func StateOf(phase, status string) string {
	switch {
	case strings.HasPrefix(status, "blocked"), strings.HasPrefix(status, "awaiting-human-gate"):
		return "blocked"
	case strings.HasPrefix(status, "paused"):
		return "paused"
	case strings.HasPrefix(status, "unblocked"):
		return "unblocked"
	case phase == "executing":
		return "executing"
	case strings.HasPrefix(status, "queued"):
		return "queued"
	case phase == "" || phase == "planned":
		return "planned"
	}
	return phase
}

var (
	reLandedName = regexp.MustCompile(`^(.+)-landed-rolled-to-([A-Za-z0-9_]+)$`)
	reRolledName = regexp.MustCompile(`^(.+)-rolled-to-([A-Za-z0-9_]+)$`)
	reDoneName   = regexp.MustCompile(`^(.+)-(done|closed)$`)
	stampPattern = `(?:\d{8}T\d{6}Z|\d{4}-\d{2}-\d{2}T\d{6}Z|\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}Z|\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`
	reStampName  = regexp.MustCompile(`^` + stampPattern + `-(.+)$`)
	reStampOnly  = regexp.MustCompile(`^` + stampPattern + `$`)
)

// ParseArchiveName reads a plan's name and outcome from its archive filename,
// whatever its timestamp prefix: 20261013T014252Z-, 2026-10-10T060037Z- and
// 2026-10-09T05-24-17Z- all occur. A bare timestamp (a replaced plan's old
// state) names no plan and returns ok=false.
func ParseArchiveName(file string) (name string, outcome Outcome, rolledTo string, ok bool) {
	base := strings.TrimSuffix(filepath.Base(file), ".md")
	if reStampOnly.MatchString(base) {
		return "", "", "", false
	}
	rest := base
	if m := reStampName.FindStringSubmatch(base); m != nil {
		rest = m[1]
	}
	switch {
	case reLandedName.MatchString(rest):
		m := reLandedName.FindStringSubmatch(rest)
		rest, outcome, rolledTo = m[1], OutcomeLanded, m[2]
	case reRolledName.MatchString(rest):
		m := reRolledName.FindStringSubmatch(rest)
		rest, outcome, rolledTo = m[1], OutcomeRolled, m[2]
	case reDoneName.MatchString(rest):
		m := reDoneName.FindStringSubmatch(rest)
		rest = m[1]
		if m[2] == "done" {
			outcome = OutcomeDone
		} else {
			outcome = OutcomeClosed
		}
	default:
		outcome = OutcomeUnknown
	}
	if i := strings.LastIndex(rest, "-"); i >= 0 {
		rest = rest[i+1:]
	}
	if rest == "" {
		return "", "", "", false
	}
	return rest, outcome, rolledTo, true
}

// KeyBlock is the subset of a plan's key block the graph reads.
type KeyBlock struct {
	Name      string
	Phase     string
	Status    string
	Started   string
	Milestone string
	Depends   []string
	Declared  bool
}

var reKey = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):[ \t]*(.*)$`)

// ReadKeyBlock reads the key lines above a plan's first "## " heading.
func ReadKeyBlock(path string) (KeyBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return KeyBlock{}, err
	}
	defer f.Close()
	var kb KeyBlock
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			break
		}
		kb.Apply(line)
	}
	return kb, sc.Err()
}

// Apply reads one key-block line into kb; lines that are not keys it knows
// are ignored.
func (kb *KeyBlock) Apply(line string) {
	m := reKey.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if m == nil {
		return
	}
	v := strings.TrimSpace(m[2])
	switch m[1] {
	case "name":
		kb.Name = v
	case "phase":
		kb.Phase = v
	case "status":
		kb.Status = v
	case "Started":
		kb.Started = v
	case "milestone":
		kb.Milestone = v
	case "depends":
		kb.Depends, _ = ParseDepends(v)
		kb.Declared = true
	}
}

// Load reads a project's live plans and archive and resolves the graph.
func Load(projectDir string) (Graph, error) {
	in, err := Inputs(projectDir)
	if err != nil {
		return Graph{}, err
	}
	return Resolve(in), nil
}

// Inputs reads a project's live plans and its archive as graph inputs.
func Inputs(projectDir string) ([]Input, error) {
	base := filepath.Join(projectDir, ".claude", "iterate")
	live, err := filepath.Glob(filepath.Join(base, "plans", "*.md"))
	if err != nil {
		return nil, err
	}
	in := []Input{}
	for _, path := range live {
		kb, err := ReadKeyBlock(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		if kb.Name != "" {
			name = kb.Name
		}
		in = append(in, Input{Name: name, Milestone: kb.Milestone, Depends: kb.Depends, Declared: kb.Declared, State: StateOf(kb.Phase, kb.Status), Started: kb.Started, File: path})
	}
	archived, err := filepath.Glob(filepath.Join(base, "archive", "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(archived)
	for _, path := range archived {
		name, outcome, rolledTo, ok := ParseArchiveName(path)
		if !ok {
			continue
		}
		kb, err := ReadKeyBlock(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if kb.Name != "" {
			name = kb.Name
		}
		in = append(in, Input{Name: name, Milestone: kb.Milestone, Depends: kb.Depends, Declared: kb.Declared, State: "archived", Archived: true, Outcome: outcome, RolledTo: rolledTo, Started: kb.Started, File: path})
	}
	return in, nil
}

type resolver struct {
	live      map[string]*Input
	archived  map[string]*Input // newest archive per name
	milestone map[string][]string
}

// Resolve builds the graph from inputs. A live plan shadows an archived one
// of the same name, and the newest archive of a name wins.
func Resolve(inputs []Input) Graph {
	r := resolver{live: map[string]*Input{}, archived: map[string]*Input{}, milestone: map[string][]string{}}
	order := []string{}
	for i := range inputs {
		in := &inputs[i]
		if in.Name == "" {
			continue
		}
		if in.Archived {
			if prev, ok := r.archived[in.Name]; !ok || filepath.Base(prev.File) < filepath.Base(in.File) {
				r.archived[in.Name] = in
			}
			continue
		}
		if _, ok := r.live[in.Name]; !ok {
			order = append(order, in.Name)
		}
		r.live[in.Name] = in
	}
	for name, in := range r.live {
		if in.Milestone != "" {
			r.milestone[in.Milestone] = append(r.milestone[in.Milestone], name)
		}
	}
	for name, in := range r.archived {
		if _, shadowed := r.live[name]; shadowed {
			continue
		}
		if in.Milestone != "" {
			r.milestone[in.Milestone] = append(r.milestone[in.Milestone], name)
		}
	}
	for id := range r.milestone {
		sort.Strings(r.milestone[id])
	}
	sort.Strings(order)

	g := Graph{Nodes: []Node{}, Errors: []string{}, index: map[string]int{}}
	add := func(n Node) int {
		if i, ok := g.index[n.Name]; ok {
			return i
		}
		g.Nodes = append(g.Nodes, n)
		g.index[n.Name] = len(g.Nodes) - 1
		return len(g.Nodes) - 1
	}
	for _, name := range order {
		in := r.live[name]
		n := Node{Name: name, Milestone: in.Milestone, State: in.State, Declared: in.Declared, Depends: append([]string{}, in.Depends...), Edges: []Edge{}, WaitingOn: []string{}, Errors: []string{}, Started: in.Started}
		for _, ref := range in.Depends {
			target, err := r.lookup(ref)
			if err != nil {
				n.Errors = append(n.Errors, err.Error())
				continue
			}
			if target == name {
				n.Errors = append(n.Errors, fmt.Sprintf("%s depends on itself", name))
				continue
			}
			e := r.edge(ref, target)
			if e.Target == name {
				n.Errors = append(n.Errors, fmt.Sprintf("%s depends on itself through a roll", name))
				continue
			}
			n.Edges = append(n.Edges, e)
		}
		add(n)
	}
	// Archived predecessors that a live edge reaches, shown for context.
	for _, name := range order {
		n := g.Nodes[g.index[name]]
		for _, e := range n.Edges {
			for _, hop := range r.chain(e.Ref) {
				if _, ok := r.live[hop]; ok {
					continue
				}
				a, ok := r.archived[hop]
				if !ok {
					continue
				}
				status := StatusDone
				if a.Outcome != OutcomeDone && a.Outcome != OutcomeLanded {
					status = StatusHeld
				}
				add(Node{Name: hop, Milestone: a.Milestone, State: "archived", Status: status, Archived: true, Outcome: a.Outcome, Declared: a.Declared, Depends: []string{}, Edges: []Edge{}, WaitingOn: []string{}, Errors: []string{}, Started: a.Started})
			}
		}
	}
	cyclic := g.markCycles()
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Archived {
			continue
		}
		for _, e := range n.Edges {
			if !e.Satisfied && !slices.Contains(n.WaitingOn, e.Target) {
				n.WaitingOn = append(n.WaitingOn, e.Target)
			}
		}
		switch {
		case len(n.Errors) > 0:
			n.Status = StatusBlocked
			n.Reason = strings.Join(n.Errors, "; ")
		case len(n.WaitingOn) > 0:
			n.Status = StatusWaiting
			n.Reason = waitingReason(*n)
		case isComplete(n.State):
			n.Status = StatusDone
		default:
			n.Status = StatusReady
		}
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Status == StatusWaiting {
			n.HeldBy = g.root(n.Name)
		}
	}
	g.depths(cyclic)
	for _, n := range g.Nodes {
		for _, msg := range n.Errors {
			if !slices.Contains(g.Errors, msg) {
				g.Errors = append(g.Errors, msg)
			}
		}
	}
	sort.SliceStable(g.Nodes, func(i, j int) bool {
		a, b := g.Nodes[i], g.Nodes[j]
		if a.Archived != b.Archived {
			return a.Archived
		}
		return Less(a, b)
	})
	for i, n := range g.Nodes {
		g.index[n.Name] = i
	}
	return g
}

func waitingReason(n Node) string {
	parts := []string{}
	for _, e := range n.Edges {
		if e.Satisfied {
			continue
		}
		p := e.Target
		if e.Reason != "" {
			p += " (" + e.Reason + ")"
		}
		if !slices.Contains(parts, p) {
			parts = append(parts, p)
		}
	}
	return "waiting on " + strings.Join(parts, ", ")
}

func isComplete(state string) bool { return state == "complete" || state == "done" }

// lookup resolves a reference to a plan name: an exact plan name first, then
// a milestone id. A milestone carried by a roll's source and its successor
// resolves to the source, whose archive outcome decides the edge.
func (r resolver) lookup(ref string) (string, error) {
	if _, ok := r.live[ref]; ok {
		return ref, nil
	}
	if _, ok := r.archived[ref]; ok {
		return ref, nil
	}
	cands := r.milestone[ref]
	if len(cands) == 0 {
		return "", fmt.Errorf("unknown dependency %s", ref)
	}
	successors := map[string]bool{}
	for _, c := range cands {
		if a, ok := r.archived[c]; ok && a.RolledTo != "" {
			successors[a.RolledTo] = true
		}
	}
	kept := []string{}
	for _, c := range cands {
		if !successors[c] {
			kept = append(kept, c)
		}
	}
	if len(kept) == 1 {
		return kept[0], nil
	}
	return "", fmt.Errorf("ambiguous milestone %s: carried by %s", ref, strings.Join(kept, " and "))
}

// edge decides one reference's satisfaction, following -rolled-to- archives
// to their successor.
func (r resolver) edge(ref, target string) Edge {
	seen := map[string]bool{}
	for {
		if seen[target] {
			return Edge{Ref: ref, Target: target, Reason: "roll loop"}
		}
		seen[target] = true
		if in, ok := r.live[target]; ok {
			if isComplete(in.State) {
				return Edge{Ref: ref, Target: target, Satisfied: true}
			}
			return Edge{Ref: ref, Target: target, Reason: in.State}
		}
		a, ok := r.archived[target]
		if !ok {
			return Edge{Ref: ref, Target: target, Reason: "rolled to a plan that does not exist"}
		}
		switch a.Outcome {
		case OutcomeDone, OutcomeLanded:
			return Edge{Ref: ref, Target: target, Satisfied: true}
		case OutcomeRolled:
			target = a.RolledTo
		case OutcomeClosed:
			return Edge{Ref: ref, Target: target, Reason: "closed unfinished"}
		default:
			return Edge{Ref: ref, Target: target, Reason: "archived unfinished"}
		}
	}
}

// chain lists every plan an edge passes through, rolls included.
func (r resolver) chain(ref string) []string {
	target, err := r.lookup(ref)
	if err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for !seen[target] {
		seen[target] = true
		out = append(out, target)
		a, ok := r.archived[target]
		if _, live := r.live[target]; live || !ok || a.Outcome != OutcomeRolled {
			break
		}
		target = a.RolledTo
	}
	return out
}

// markCycles records an error on every live plan in a dependency cycle,
// naming the cycle, and returns the cycle members.
func (g *Graph) markCycles() map[string]bool {
	members := map[string]bool{}
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	stack := []string{}
	var visit func(string)
	visit = func(name string) {
		i, ok := g.index[name]
		if !ok || g.Nodes[i].Archived {
			return
		}
		color[name] = grey
		stack = append(stack, name)
		for _, e := range g.Nodes[i].Edges {
			t := e.Target
			switch color[t] {
			case white:
				visit(t)
			case grey:
				start := 0
				for k, s := range stack {
					if s == t {
						start = k
					}
				}
				cycle := append(append([]string{}, stack[start:]...), t)
				msg := "cycle " + strings.Join(cycle, " → ")
				for _, member := range stack[start:] {
					members[member] = true
					m := &g.Nodes[g.index[member]]
					if !slices.Contains(m.Errors, msg) {
						m.Errors = append(m.Errors, msg)
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[name] = black
	}
	names := []string{}
	for _, n := range g.Nodes {
		if !n.Archived {
			names = append(names, n.Name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if color[name] == white {
			visit(name)
		}
	}
	return members
}

// root follows a waiting plan's first unfinished predecessor until it reaches
// a plan that is not itself waiting: the plan holding the whole branch.
func (g Graph) root(name string) string {
	seen := map[string]bool{}
	cur := name
	for !seen[cur] {
		seen[cur] = true
		n, ok := g.Node(cur)
		if !ok || n.Archived || n.Status != StatusWaiting || len(n.WaitingOn) == 0 {
			return cur
		}
		cur = n.WaitingOn[0]
	}
	return cur
}

// depths sets each node's depth: 0 with no edges, else one more than its
// deepest predecessor. Cycle members stay at 0.
func (g *Graph) depths(cyclic map[string]bool) {
	memo := map[string]int{}
	for name := range cyclic {
		memo[name] = 0
	}
	onPath := map[string]bool{}
	var depth func(string) int
	depth = func(name string) int {
		if d, ok := memo[name]; ok {
			return d
		}
		i, ok := g.index[name]
		if !ok || onPath[name] {
			return 0
		}
		onPath[name] = true
		d := 0
		for _, e := range g.Nodes[i].Edges {
			if _, known := g.index[e.Target]; !known {
				continue
			}
			if v := depth(e.Target) + 1; v > d {
				d = v
			}
		}
		onPath[name] = false
		memo[name] = d
		return d
	}
	for i := range g.Nodes {
		g.Nodes[i].Depth = depth(g.Nodes[i].Name)
	}
}
