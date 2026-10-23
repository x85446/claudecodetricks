package iterrun

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/x85446/claudecodetricks/src/internal/plangraph"
)

// GraphEdge is one dependency drawn from predecessor to dependent.
type GraphEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Satisfied bool   `json:"satisfied"`
	Reason    string `json:"reason,omitempty"`
	Implied   bool   `json:"implied,omitempty"` // another predecessor already reaches it; drawings omit it
}

// GraphView is the one document `iterate-run plan graph --format json` and
// `GET /api/graph` both serve.
type GraphView struct {
	Project string           `json:"project"`
	Nodes   []plangraph.Node `json:"nodes"`
	Edges   []GraphEdge      `json:"edges"`
	Errors  []string         `json:"errors"`
}

// BuildGraphView resolves a project's plan graph into its serving shape.
func BuildGraphView(projectDir string) (GraphView, error) {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return GraphView{}, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	g, err := ProjectGraph(abs)
	if err != nil {
		return GraphView{}, err
	}
	v := GraphView{Project: abs, Nodes: g.Nodes, Edges: []GraphEdge{}, Errors: g.Errors}
	for _, n := range g.Nodes {
		for _, e := range n.Edges {
			v.Edges = append(v.Edges, GraphEdge{From: e.Target, To: n.Name, Satisfied: e.Satisfied, Reason: e.Reason, Implied: e.Implied})
		}
	}
	return v, nil
}

// Mermaid renders the graph as a Mermaid flowchart: one node per plan,
// archived predecessors included as done, one arrow per edge, dashed while
// the edge is unsatisfied.
func (v GraphView) Mermaid() string {
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	id := map[string]string{}
	for i, n := range v.Nodes {
		id[n.Name] = fmt.Sprintf("n%d", i)
		label := n.Name
		if len(n.Milestones) > 0 {
			label += " · " + strings.Join(n.Milestones, ", ")
		}
		label += " · " + nodeWord(n)
		fmt.Fprintf(&b, "  %s[\"%s\"]:::%s\n", id[n.Name], strings.ReplaceAll(label, `"`, "'"), mermaidClass(n))
	}
	for _, e := range v.Edges {
		if e.Implied {
			continue
		}
		from, ok1 := id[e.From]
		to, ok2 := id[e.To]
		if !ok1 || !ok2 {
			continue
		}
		arrow := "-->"
		if !e.Satisfied {
			arrow = "-.->"
		}
		fmt.Fprintf(&b, "  %s %s %s\n", from, arrow, to)
	}
	b.WriteString("  classDef done fill:#e6f4ea,stroke:#1e8e3e,color:#0d3818\n")
	b.WriteString("  classDef ready fill:#e8f0fe,stroke:#1a73e8,color:#0b2a5c\n")
	b.WriteString("  classDef waiting fill:#fef7e0,stroke:#b06000,color:#4a2800\n")
	b.WriteString("  classDef blocked fill:#fce8e6,stroke:#c5221f,color:#5c0f0d\n")
	return b.String()
}

// DisplayStatus is the status a view draws: the graph status, except that
// a plan whose own edges are met but which is itself blocked or paused
// reads as blocked.
func DisplayStatus(n plangraph.Node) string {
	if n.Status == plangraph.StatusReady && (n.State == "blocked" || n.State == "paused") {
		return plangraph.StatusBlocked
	}
	return n.Status
}

func mermaidClass(n plangraph.Node) string {
	switch DisplayStatus(n) {
	case plangraph.StatusDone:
		return "done"
	case plangraph.StatusWaiting:
		return "waiting"
	case plangraph.StatusBlocked, plangraph.StatusHeld:
		return "blocked"
	}
	return "ready"
}

// nodeWord is the one-word summary a row or label shows: the plan's own
// state, or the archive outcome for an archived predecessor.
func nodeWord(n plangraph.Node) string {
	if n.Archived {
		switch n.Outcome {
		case plangraph.OutcomeDone:
			return "done"
		case plangraph.OutcomeLanded:
			return "landed"
		case plangraph.OutcomeClosed:
			return "closed"
		case plangraph.OutcomeRolled:
			return "rolled"
		}
		return "archived"
	}
	return n.State
}

// GraphRow is one line of the text graph.
type GraphRow struct {
	Depth  int
	Last   []bool // per ancestor level: was that ancestor the last child
	Node   plangraph.Node
	Also   []string // further predecessors of a join
	Detail string
}

// TextRows lays the graph out as a tree: each plan once, nested under its
// first predecessor, roots in graph order.
func (v GraphView) TextRows() []GraphRow {
	byName := map[string]plangraph.Node{}
	for _, n := range v.Nodes {
		byName[n.Name] = n
	}
	parent := map[string]string{}
	children := map[string][]string{}
	also := map[string][]string{}
	for _, n := range v.Nodes {
		for _, e := range n.Edges {
			if _, ok := byName[e.Target]; !ok || e.Implied {
				continue
			}
			if _, has := parent[n.Name]; !has {
				parent[n.Name] = e.Target
				children[e.Target] = append(children[e.Target], n.Name)
			} else if e.Target != parent[n.Name] {
				also[n.Name] = append(also[n.Name], e.Target)
			}
		}
	}
	rows := []GraphRow{}
	seen := map[string]bool{}
	var walk func(name string, depth int, last []bool)
	walk = func(name string, depth int, last []bool) {
		if seen[name] {
			return
		}
		seen[name] = true
		n := byName[name]
		rows = append(rows, GraphRow{Depth: depth, Last: append([]bool{}, last...), Node: n, Also: also[name], Detail: rowDetail(n)})
		kids := children[name]
		for i, k := range kids {
			walk(k, depth+1, append(append([]bool{}, last...), i == len(kids)-1))
		}
	}
	for _, n := range v.Nodes {
		if _, has := parent[n.Name]; !has {
			walk(n.Name, 0, nil)
		}
	}
	// A cycle has no root; print what is left so every plan appears once.
	for _, n := range v.Nodes {
		walk(n.Name, 0, nil)
	}
	return rows
}

func rowDetail(n plangraph.Node) string {
	switch n.Status {
	case plangraph.StatusWaiting, plangraph.StatusBlocked:
		return n.Reason
	case plangraph.StatusHeld:
		return "does not satisfy its dependents"
	}
	if DisplayStatus(n) != n.Status {
		return ""
	}
	return n.Status
}
