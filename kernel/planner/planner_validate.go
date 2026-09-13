// SPDX-License-Identifier: MIT

// Planner: ValidateJSON + parseAndValidate + validateDAG + validateIntentBoundary + hasGateDependency + snippet.
// Code extracted from planner.go during the Day-143 god-file split.
// Public API unchanged.
package planner


import (
	"errors"
	"fmt"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/internal/strutil"
	intentmodel "github.com/agezt/agezt/kernel/intent"
)

func ValidateJSON(raw []byte) (Plan, error) { return parseAndValidate(string(raw)) }

// parseAndValidate decodes the JSON, runs the structural checks
// described in the package doc, and returns the typed Plan.
func parseAndValidate(raw string) (Plan, error) {
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Plan{}, fmt.Errorf("plan JSON parse: %w", err)
	}
	if len(p.Nodes) == 0 {
		return Plan{}, errors.New("plan has no nodes")
	}
	if p.MaxParallel < 0 {
		return Plan{}, fmt.Errorf("max_parallel must be >= 0 (got %d)", p.MaxParallel)
	}

	ids := make(map[string]struct{}, len(p.Nodes))
	for i, n := range p.Nodes {
		if strings.TrimSpace(n.ID) == "" {
			return Plan{}, fmt.Errorf("node[%d]: id is empty", i)
		}
		if _, dup := ids[n.ID]; dup {
			return Plan{}, fmt.Errorf("node[%d]: duplicate id %q", i, n.ID)
		}
		ids[n.ID] = struct{}{}
		switch n.Kind {
		case "loop":
			if strings.TrimSpace(n.Intent) == "" {
				return Plan{}, fmt.Errorf("node %q (loop): intent is empty", n.ID)
			}
		case "gate":
			if strings.TrimSpace(n.Description) == "" {
				return Plan{}, fmt.Errorf("node %q (gate): description is empty", n.ID)
			}
		default:
			return Plan{}, fmt.Errorf("node %q: unknown kind %q (want loop|gate)", n.ID, n.Kind)
		}
	}
	// Dep resolution + cycle check (Kahn-style topological walk).
	if err := validateDAG(p.Nodes, ids); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// validateDAG ensures every dep id exists and there are no cycles.
// We run our own check rather than relying on the scheduler so a
// bad plan from the LLM produces a clear "node X depends on Y
// which doesn't exist" message at the planner boundary.
func validateDAG(nodes []Node, ids map[string]struct{}) error {
	// Reference check.
	for _, n := range nodes {
		for _, d := range n.Deps {
			if d == n.ID {
				return fmt.Errorf("node %q depends on itself", n.ID)
			}
			if _, ok := ids[d]; !ok {
				return fmt.Errorf("node %q: dep %q does not exist", n.ID, d)
			}
		}
	}
	// Cycle check via Kahn (count incoming, repeatedly drop nodes
	// with zero remaining inputs).
	indeg := map[string]int{}
	deps := map[string][]string{}
	for _, n := range nodes {
		indeg[n.ID] = 0
		deps[n.ID] = nil
	}
	for _, n := range nodes {
		for _, d := range n.Deps {
			indeg[n.ID]++
			deps[d] = append(deps[d], n.ID)
		}
	}
	queue := []string{}
	for id, deg := range indeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, downstream := range deps[id] {
			indeg[downstream]--
			if indeg[downstream] == 0 {
				queue = append(queue, downstream)
			}
		}
	}
	if processed != len(nodes) {
		return fmt.Errorf("plan has a cycle (processed %d of %d nodes via topological walk)", processed, len(nodes))
	}
	return nil
}

func validateIntentBoundary(frame intentmodel.Frame, plan Plan) error {
	if !frame.Underdetermined || frame.AmbiguityScore < 0.6 {
		return nil
	}
	nodes := make(map[string]Node, len(plan.Nodes))
	for _, n := range plan.Nodes {
		nodes[n.ID] = n
	}
	for _, n := range plan.Nodes {
		if n.Kind != "loop" {
			continue
		}
		axes := intentmodel.RegretForAction(intentmodel.Action{
			ToolName:    "planner.loop",
			Capability:  "plan.loop",
			EffectClass: "read_only",
			Input:       n.Intent,
		})
		if !intentmodel.RequiresConfirmation(frame, axes) {
			continue
		}
		if !hasGateDependency(n, nodes, map[string]bool{}) {
			return fmt.Errorf("underdetermined intent requires a gate before high-regret loop node %q", n.ID)
		}
	}
	return nil
}

func hasGateDependency(n Node, nodes map[string]Node, seen map[string]bool) bool {
	for _, depID := range n.Deps {
		if seen[depID] {
			continue
		}
		seen[depID] = true
		dep, ok := nodes[depID]
		if !ok {
			continue
		}
		if dep.Kind == "gate" {
			return true
		}
		if hasGateDependency(dep, nodes, seen) {
			return true
		}
	}
	return false
}

// snippet returns the first ~200 chars of s for error messages.
// Truncates so a verbose LLM doesn't dump a multi-kilobyte
// response into the operator's terminal as part of an error.
func snippet(s string) string {
	return strutil.Ellipsis(strings.TrimSpace(s), 200, "...")
}
