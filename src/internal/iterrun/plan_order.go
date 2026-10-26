package iterrun

import (
	"fmt"
	"os"
	"strings"

	"github.com/x85446/claudecodetricks/src/internal/plangraph"
)

// SetPlanOrder validates the complete live sequence before writing positions.
func SetPlanOrder(project string, names []string) error {
	return ownershipLocked(project, func() error {
		inputs, e := plangraph.Inputs(project)
		if e != nil {
			return e
		}
		positions := map[string]int{}
		for i, name := range names {
			if positions[name] != 0 {
				return fmt.Errorf("duplicate plan %s", name)
			}
			positions[name] = i + 1
		}
		count := 0
		original := map[string]string{}
		for i := range inputs {
			in := &inputs[i]
			if in.Archived {
				continue
			}
			count++
			pos := positions[in.Name]
			if pos == 0 {
				return fmt.Errorf("execution order omits %s", in.Name)
			}
			in.Position = pos
			in.Label = ""
			b, err := os.ReadFile(in.File)
			if err != nil {
				return err
			}
			original[in.File] = string(b)
		}
		if count != len(names) {
			return fmt.Errorf("order must name every live plan exactly once")
		}
		graph := plangraph.Resolve(inputs)
		if len(graph.Errors) > 0 {
			return fmt.Errorf("execution order: %s", strings.Join(graph.Errors, "; "))
		}
		written := []string{}
		for _, in := range inputs {
			if in.Archived {
				continue
			}
			text := setPlanField(original[in.File], "position", fmt.Sprint(in.Position))
			text = setPlanField(text, "label", string(rune('A'+in.Position-1)))
			if e = writeOwnerPlan(in.File, text); e != nil {
				for _, path := range written {
					_ = writeOwnerPlan(path, original[path])
				}
				return e
			}
			written = append(written, in.File)
		}
		return nil
	})
}
