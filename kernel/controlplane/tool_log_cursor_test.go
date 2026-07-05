// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestToolLog_CursorPagination walks /api/tool_log across two pages using the
// shared journal cursor (A2). It seeds more tool calls than one page holds,
// pulls page 1 with a small limit, feeds the returned next_cursor into page 2,
// and asserts the pages are contiguous and non-overlapping — the exact
// contract journal.KeepBeforeCursor / NextCursor provide. The cursor codec
// itself is unit-tested in kernel/journal/cursor_test.go; this is the
// endpoint-level integration proof.
func TestToolLog_CursorPagination(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Seed 5 tool.result events (newest last). Each call_id is unique so we can
	// track which rows a page returned by their tool name.
	const total = 5
	for i := 0; i < total; i++ {
		id := "call-" + string(rune('a'+i))
		toolResult(k, id, "tool-"+string(rune('a'+i)), "out", false)
	}

	// Page 1: limit 2 → newest 2, plus a next_cursor because the page is full.
	p1, err := c.Call(context.Background(), controlplane.CmdToolLog, map[string]any{"limit": float64(2)})
	if err != nil {
		t.Fatalf("page 1 Call: %v", err)
	}
	p1rows, _ := p1["invocations"].([]any)
	if len(p1rows) != 2 {
		t.Fatalf("page 1 rows = %d want 2", len(p1rows))
	}
	cursor, _ := p1["next_cursor"].(string)
	if cursor == "" {
		t.Fatalf("page 1 next_cursor empty; want a token because the page was full")
	}

	// Page 2: same limit, carrying the cursor → the next 2 older rows.
	p2, err := c.Call(context.Background(), controlplane.CmdToolLog, map[string]any{"limit": float64(2), "cursor": cursor})
	if err != nil {
		t.Fatalf("page 2 Call: %v", err)
	}
	p2rows, _ := p2["invocations"].([]any)
	if len(p2rows) != 2 {
		t.Fatalf("page 2 rows = %d want 2", len(p2rows))
	}

	// No overlap: the cursor must exclude every row already emitted on page 1.
	seen := map[string]bool{}
	for _, r := range p1rows {
		m, _ := r.(map[string]any)
		seen[toolName(m)] = true
	}
	for _, r := range p2rows {
		m, _ := r.(map[string]any)
		if seen[toolName(m)] {
			t.Fatalf("page 2 re-emitted a page-1 row: %q (cursor filter failed)", toolName(m))
		}
	}

	// Page 3: the last (5th) row, then a terminal page with no next_cursor.
	c2, _ := p2["next_cursor"].(string)
	if c2 == "" {
		t.Fatalf("page 2 next_cursor empty; 1 row remains, want a token")
	}
	p3, err := c.Call(context.Background(), controlplane.CmdToolLog, map[string]any{"limit": float64(2), "cursor": c2})
	if err != nil {
		t.Fatalf("page 3 Call: %v", err)
	}
	p3rows, _ := p3["invocations"].([]any)
	if len(p3rows) != 1 {
		t.Fatalf("page 3 rows = %d want 1 (the last remaining row)", len(p3rows))
	}
	// A short page is terminal: no next_cursor.
	if tok, _ := p3["next_cursor"].(string); tok != "" {
		t.Fatalf("page 3 next_cursor = %q, want empty (short page is terminal)", tok)
	}
}

// TestToolLog_MalformedCursorFallsBack — a bad cursor must not error; it falls
// back to the newest page (journal.DecodeCursor returns ok=false).
func TestToolLog_MalformedCursorFallsBack(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolResult(k, "call-1", "shell", "ok", false)
	toolResult(k, "call-2", "http", "ok", false)

	res, err := c.Call(context.Background(), controlplane.CmdToolLog, map[string]any{"cursor": "not-a-valid-cursor"})
	if err != nil {
		t.Fatalf("Call with bad cursor errored: %v", err)
	}
	rows, _ := res["invocations"].([]any)
	if len(rows) != 2 {
		t.Fatalf("bad cursor rows = %d want 2 (should fall back to newest page)", len(rows))
	}
}

func toolName(m map[string]any) string {
	s, _ := m["tool"].(string)
	return s
}
