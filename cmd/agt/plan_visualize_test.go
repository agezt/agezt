// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/planner"
)

// TestRenderPlanMermaid_LoopOnly covers the simplest case: a
// plan with only loop nodes, no deps. Output must start with
// `graph TD`, declare each node with the rectangle shape, and
// emit no edge lines.
func TestRenderPlanMermaid_LoopOnly(t *testing.T) {
	plan := planner.Plan{
		Name: "smoke",
		Nodes: []planner.Node{
			{ID: "a", Kind: "loop", Intent: "fetch users"},
			{ID: "b", Kind: "loop", Intent: "compute totals"},
		},
	}
	var buf bytes.Buffer
	renderPlanMermaid(&buf, plan)
	out := buf.String()

	if !strings.HasPrefix(out, "graph TD\n") {
		t.Errorf("output should start with `graph TD`; got: %q", out)
	}
	if !strings.Contains(out, `a["loop: fetch users"]`) {
		t.Errorf("missing node a; got: %q", out)
	}
	if !strings.Contains(out, `b["loop: compute totals"]`) {
		t.Errorf("missing node b; got: %q", out)
	}
	if strings.Contains(out, "-->") {
		t.Errorf("no deps configured but found edge arrow; got: %q", out)
	}
}

// TestRenderPlanMermaid_GateGetsHexagonShape — gates must use
// the {{...}} shape so HITL stops visually stand out from
// loop rectangles.
func TestRenderPlanMermaid_GateGetsHexagonShape(t *testing.T) {
	plan := planner.Plan{
		Nodes: []planner.Node{
			{ID: "a", Kind: "loop", Intent: "draft"},
			{ID: "b", Kind: "gate", Description: "approve draft?", Deps: []string{"a"}},
		},
	}
	var buf bytes.Buffer
	renderPlanMermaid(&buf, plan)
	out := buf.String()

	if !strings.Contains(out, `b{{"gate: approve draft?"}}`) {
		t.Errorf("gate should render with {{...}} shape; got: %q", out)
	}
	if !strings.Contains(out, "a --> b") {
		t.Errorf("dep edge a→b missing; got: %q", out)
	}
}

// TestMermaidLabel_EscapesQuotes — a plan intent containing
// double quotes would break Mermaid's quoted-label syntax;
// they must be HTML-entity-escaped.
func TestMermaidLabel_EscapesQuotes(t *testing.T) {
	got := mermaidLabel(`he said "hi"`)
	if strings.Contains(got, `"`) {
		t.Errorf("raw quote leaked into label: %q", got)
	}
	if !strings.Contains(got, "&quot;") {
		t.Errorf("quote not escaped to &quot;: %q", got)
	}
}

// TestMermaidLabel_BreaksNewlines — newlines in an intent must
// become <br/> tags so Mermaid renders them as line breaks
// instead of treating them as syntax.
func TestMermaidLabel_BreaksNewlines(t *testing.T) {
	got := mermaidLabel("first\nsecond")
	if strings.Contains(got, "\n") {
		t.Errorf("raw newline leaked: %q", got)
	}
	if !strings.Contains(got, "<br/>") {
		t.Errorf("missing <br/>: %q", got)
	}
}

// TestMermaidNodeID_SanitizesUnsafeChars — plan IDs with dots,
// slashes, or other non-word characters must be normalized to
// underscores so Mermaid accepts them as identifiers.
func TestMermaidNodeID_SanitizesUnsafeChars(t *testing.T) {
	cases := map[string]string{
		"abc":          "abc",
		"node-1":       "node_1",
		"foo.bar":      "foo_bar",
		"a/b/c":        "a_b_c",
		"plan_step_42": "plan_step_42",
		"":             "_",
	}
	for in, want := range cases {
		got := mermaidNodeID(in)
		if got != want {
			t.Errorf("mermaidNodeID(%q) = %q want %q", in, got, want)
		}
	}
}

// TestNodeSummary_TruncatesLongIntents — operators reviewing a
// rendered DAG should see manageable labels; ~60 chars keeps
// even wide graphs legible.
func TestNodeSummary_TruncatesLongIntents(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := nodeSummary(planner.Node{Kind: "loop", Intent: long})
	if len(got) > 80 {
		t.Errorf("nodeSummary not truncated: len=%d", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix; got: %q", got)
	}
}

// TestCmdPlanVisualize_RoundtripsValidPlan exercises the full
// file → render path on a valid plan. End-to-end smoke test.
func TestCmdPlanVisualize_RoundtripsValidPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	planJSON := `{
		"name": "test",
		"nodes": [
			{"id": "a", "kind": "loop", "intent": "do thing"},
			{"id": "b", "kind": "gate", "description": "approve?", "deps": ["a"]}
		]
	}`
	if err := os.WriteFile(path, []byte(planJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := cmdPlanVisualize([]string{path}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	s := out.String()
	for _, want := range []string{"```mermaid", "graph TD", `a["loop:`, `b{{"gate:`, "a --> b", "```\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q; got %q", want, s)
		}
	}
}

// TestCmdPlanVisualize_RawDropsFences — --raw mode should omit
// the surrounding ```mermaid fences so the output can be piped
// into the Mermaid CLI without post-processing.
func TestCmdPlanVisualize_RawDropsFences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	planJSON := `{"nodes":[{"id":"a","kind":"loop","intent":"x"}]}`
	if err := os.WriteFile(path, []byte(planJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := cmdPlanVisualize([]string{path, "--raw"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if strings.Contains(out.String(), "```") {
		t.Errorf("--raw should drop fences; got: %q", out.String())
	}
	if !strings.HasPrefix(out.String(), "graph TD") {
		t.Errorf("--raw output should start with `graph TD`; got: %q", out.String())
	}
}

// TestCmdPlanVisualize_RejectsInvalidPlan — visualizing an
// invalid plan would mislead reviewers ("I see the diagram, so
// it must execute"). Must surface the same validation errors
// `agt plan validate` does, with exit code 1.
func TestCmdPlanVisualize_RejectsInvalidPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	// Cycle: a depends on b, b depends on a.
	if err := os.WriteFile(path, []byte(`{"nodes":[
		{"id":"a","kind":"loop","intent":"x","deps":["b"]},
		{"id":"b","kind":"loop","intent":"y","deps":["a"]}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := cmdPlanVisualize([]string{path}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit=%d want 1", code)
	}
	if !strings.Contains(errOut.String(), "cycle") {
		t.Errorf("stderr should mention cycle; got %q", errOut.String())
	}
}

// TestCmdPlanVisualize_RejectsMissingPath — basic usage guard.
func TestCmdPlanVisualize_RejectsMissingPath(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdPlanVisualize(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "plan file path required") {
		t.Errorf("stderr missing path-required note; got %q", errOut.String())
	}
}
