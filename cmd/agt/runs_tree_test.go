// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// row is a minimal runs-list row for the tree renderer.
func row(corr, parent string) map[string]any {
	return map[string]any{
		"correlation_id":     corr,
		"parent_correlation": parent,
		"intent":             corr,
		"status":             "completed",
	}
}

// TestRenderRunsTree_HierarchyAndIndent — sub-agent runs nest under their
// lead in DFS order, each level indented two more spaces; a run whose parent
// is absent from the set renders as a root (M43).
func TestRenderRunsTree_HierarchyAndIndent(t *testing.T) {
	rows := []any{
		row("p1", ""),        // root
		row("c1", "p1"),      // child of p1
		row("c2", "p1"),      // child of p1
		row("g1", "c1"),      // grandchild (child of c1)
		row("o1", "missing"), // parent not present → treated as a root
	}
	var buf bytes.Buffer
	renderRunsTree(&buf, rows)
	out := buf.String()

	// Indent of each correlation's header line (leading spaces before the id).
	indentOf := func(corr string) int {
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimLeft(line, " ") == corr {
				return len(line) - len(strings.TrimLeft(line, " "))
			}
		}
		t.Fatalf("correlation %q not found in tree output:\n%s", corr, out)
		return -1
	}
	if got := indentOf("p1"); got != 2 {
		t.Errorf("p1 indent = %d want 2 (root)", got)
	}
	if got := indentOf("c1"); got != 4 {
		t.Errorf("c1 indent = %d want 4 (child)", got)
	}
	if got := indentOf("g1"); got != 6 {
		t.Errorf("g1 indent = %d want 6 (grandchild)", got)
	}
	if got := indentOf("o1"); got != 2 {
		t.Errorf("o1 indent = %d want 2 (orphan → root)", got)
	}

	// DFS order: c1's whole subtree (g1) precedes its sibling c2.
	if strings.Index(out, "g1") > strings.Index(out, "c2") {
		t.Errorf("DFS order wrong: g1 (c1's child) should precede c2\n%s", out)
	}
	if strings.Index(out, "p1") > strings.Index(out, "c1") {
		t.Errorf("root p1 should precede its child c1\n%s", out)
	}
}
