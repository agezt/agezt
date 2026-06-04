// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/planner"
)

// cmdPlanVisualize implements `agt plan visualize <file.json>`.
// Renders a plan as a Mermaid `graph TD` block — pasteable into
// GitHub PRs, Notion docs, or any markdown viewer that supports
// Mermaid for human-comprehensible review of plans before they
// hit the daemon.
//
// Pure client-side, same rationale as cmdPlanValidate: visualization
// is a pure function over the plan JSON, no providers / no creds.
//
// Output shape (default):
//
//	```mermaid
//	graph TD
//	  a["loop: do thing"]
//	  b{{"gate: approve?"}}
//	  a --> b
//	```
//
// Loops render as rectangles; gates as the Mermaid hexagon shape
// (`{{...}}`) so reviewers can spot HITL stops at a glance.
//
// With --raw the surrounding ```mermaid fences are dropped, so the
// output composes into a larger document or pipes into `mmdc` (the
// Mermaid CLI) without post-processing.
func cmdPlanVisualize(args []string, stdout, stderr io.Writer) int {
	path := ""
	raw := false
	for _, a := range args {
		switch a {
		case "--raw":
			raw = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s plan visualize <file.json> [--raw]\n", brand.CLI)
			fmt.Fprintf(stdout, "render a plan as a Mermaid graph TD block; paste into a markdown viewer\n")
			fmt.Fprintf(stdout, "  --raw   drop the ```mermaid fences (for piping into mmdc, etc.)\n")
			return 0
		default:
			if path == "" {
				path = a
				continue
			}
			fmt.Fprintf(stderr, "%s plan visualize: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if path == "" {
		fmt.Fprintf(stderr, "%s plan visualize: plan file path required\n", brand.CLI)
		return 2
	}

	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s plan visualize: read %s: %v\n", brand.CLI, path, err)
		return 1
	}
	plan, err := planner.ValidateJSON(body)
	if err != nil {
		// Reject invalid plans up front — rendering Mermaid for a
		// plan that wouldn't execute is misleading. Reviewers might
		// approve a diagram that the daemon would reject at submit
		// time. Fail loudly instead.
		fmt.Fprintf(stderr, "%s plan visualize: %s: %v\n", brand.CLI, path, err)
		return 1
	}

	if !raw {
		fmt.Fprintln(stdout, "```mermaid")
	}
	renderPlanMermaid(stdout, plan)
	if !raw {
		fmt.Fprintln(stdout, "```")
	}
	return 0
}

// renderPlanMermaid emits a `graph TD` block describing the plan.
// Extracted so tests can exercise the rendering without going
// through file I/O.
func renderPlanMermaid(w io.Writer, plan planner.Plan) {
	fmt.Fprintln(w, "graph TD")
	if plan.Name != "" {
		// Mermaid comments — operators viewing the rendered diagram
		// see only the topology, but the raw source carries the plan
		// name as documentation.
		fmt.Fprintf(w, "  %% plan: %s\n", plan.Name)
	}
	if plan.MaxParallel > 0 {
		fmt.Fprintf(w, "  %% max_parallel: %d\n", plan.MaxParallel)
	}

	// Node declarations first, then edges. Mermaid accepts inline
	// declarations on edges, but split form keeps the source diffable
	// when a plan adds/removes a single edge.
	for _, n := range plan.Nodes {
		fmt.Fprintf(w, "  %s%s\n", mermaidNodeID(n.ID), mermaidNodeShape(n))
	}
	for _, n := range plan.Nodes {
		for _, dep := range n.Deps {
			// Edge points from dependency → dependant, matching the
			// scheduler's execution order ("dep must finish before n").
			fmt.Fprintf(w, "  %s --> %s\n", mermaidNodeID(dep), mermaidNodeID(n.ID))
		}
	}
}

// mermaidNodeShape returns the bracket-form for a node based on
// its kind. Loop nodes use rectangles `["..."]`; gate nodes use
// the hexagon shape `{{"..."}}` so HITL stops jump out visually.
// Unknown kinds fall back to rectangles — graceful degradation,
// since validation already accepted the plan.
func mermaidNodeShape(n planner.Node) string {
	label := mermaidLabel(nodeSummary(n))
	switch n.Kind {
	case "gate":
		return "{{\"" + label + "\"}}"
	default:
		return "[\"" + label + "\"]"
	}
}

// nodeSummary produces the one-line label that goes into the node
// shape. Format: `kind: intent-or-description`, trimmed so wide
// plans stay legible.
func nodeSummary(n planner.Node) string {
	body := n.Intent
	if body == "" {
		body = n.Description
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return n.Kind
	}
	return n.Kind + ": " + truncate(body, 59)
}

// mermaidLabel escapes characters that break Mermaid's quoted-label
// syntax. Quotes get HTML-entity-escaped; newlines collapse to
// <br/> which Mermaid renders natively. Backslashes are passed
// through unchanged (Mermaid doesn't interpret them).
func mermaidLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "\r\n", "<br/>")
	s = strings.ReplaceAll(s, "\n", "<br/>")
	return s
}

// mermaidNodeID sanitizes a plan's node ID for Mermaid. Mermaid
// identifiers are word chars only — replace anything else with
// underscores. Most plan IDs are already safe (alphanumeric +
// dashes/underscores), so this is a defensive pass for edge cases.
func mermaidNodeID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}
