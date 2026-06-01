// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func renderDiff(ops []diffOp) string {
	var b strings.Builder
	for _, op := range ops {
		b.WriteByte(op.sign)
		b.WriteByte(' ')
		b.WriteString(op.text)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestLineDiff(t *testing.T) {
	// Identical → all context, no +/-.
	id := lineDiff([]string{"a", "b"}, []string{"a", "b"})
	for _, op := range id {
		if op.sign != ' ' {
			t.Errorf("identical inputs produced a change: %q", renderDiff(id))
			break
		}
	}

	// A middle line changed: keep a/c as context, -b +B.
	ops := lineDiff([]string{"a", "b", "c"}, []string{"a", "B", "c"})
	out := renderDiff(ops)
	if !strings.Contains(out, "- b") || !strings.Contains(out, "+ B") {
		t.Errorf("changed-line diff wrong:\n%s", out)
	}
	if !strings.Contains(out, "  a") || !strings.Contains(out, "  c") {
		t.Errorf("context lines missing:\n%s", out)
	}

	// Pure addition.
	add := lineDiff([]string{"a"}, []string{"a", "b"})
	if !strings.Contains(renderDiff(add), "+ b") {
		t.Errorf("addition not detected: %q", renderDiff(add))
	}

	// Pure removal.
	rm := lineDiff([]string{"a", "b"}, []string{"a"})
	if !strings.Contains(renderDiff(rm), "- b") {
		t.Errorf("removal not detected: %q", renderDiff(rm))
	}

	// Empty → empty.
	if d := lineDiff(nil, nil); len(d) != 0 {
		t.Errorf("nil/nil should be empty, got %v", d)
	}
}

func TestSplitLines(t *testing.T) {
	if got := splitLines(""); got != nil {
		t.Errorf("empty → nil, got %v", got)
	}
	got := splitLines("a\r\nb\nc")
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("splitLines normalisation wrong: %v", got)
	}
}
