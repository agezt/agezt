// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
)

// TestRateLimitLog_CursorPagination proves that the route registered in
// webui.go (A2 Phase 2) is reachable, accepts a cursor query arg, and that
// the journal.Cursor filter + NextCursor emit work for the rate_limit_log
// source. Sends 3 stream calls through a 1-per-min governor to seed ≥2
// throttle events, then paginates with limit 1 to assert a multi-page walk.
func TestRateLimitLog_CursorPagination(t *testing.T) {
	gov := rateLimitedGovernor(t, 1) // 1 admit/min; throttles every subsequent call
	k, _, c, _ := startPair(t, gov)
	gov.SetBus(k.Bus())

	// Three stream calls → at least two "rate.limited" events land in the journal.
	for i := 0; i < 3; i++ {
		_, _ = c.Stream(context.Background(), controlplane.CmdRun, map[string]any{"intent": "x"}, nil)
	}

	// Page 1 (limit 1) → exactly one row + a non-empty next_cursor.
	p1, err := c.Call(context.Background(), controlplane.CmdRateLimitLog,
		map[string]any{"limit": float64(1)})
	if err != nil {
		t.Fatalf("page 1 Call: %v", err)
	}
	rows, _ := p1["throttles"].([]any)
	if len(rows) != 1 {
		t.Fatalf("page 1 rows = %d want 1", len(rows))
	}
	cursor, _ := p1["next_cursor"].(string)
	if cursor == "" {
		t.Fatalf("page 1 next_cursor empty; want a token because the page is full")
	}

	// Page 2 → page 1's same row must NOT reappear (cursor filter works).
	p2, err := c.Call(context.Background(), controlplane.CmdRateLimitLog,
		map[string]any{"limit": float64(1), "cursor": cursor})
	if err != nil {
		t.Fatalf("page 2 Call: %v", err)
	}
	rows2, _ := p2["throttles"].([]any)
	if len(rows2) != 1 {
		t.Fatalf("page 2 rows = %d want 1", len(rows2))
	}
	// Page 2 must be a *different* row from page 1 (cursor filter advanced).
	// Re-emitting the same row means the cursor filter didn't advance. Compare
	// the row map directly: at minimum, ts_unix_ms or some other field must
	// differ; here we require the entire map to differ.
	fa, _ := rows[0].(map[string]any)
	fb, _ := rows2[0].(map[string]any)
	if fmt.Sprint(fa) == fmt.Sprint(fb) {
		t.Fatalf("page 2 re-emitted the exact same row as page 1 (cursor filter failed); %v", fa)
	}
}

// TestRateLimitLog_MalformedCursorFallsBack — a bad cursor must not error;
// the handler should ignore it and return the newest page (journal.DecodeCursor
// returns ok=false on parse error).
func TestRateLimitLog_MalformedCursorFallsBack(t *testing.T) {
	gov := rateLimitedGovernor(t, 1)
	k, _, c, _ := startPair(t, gov)
	gov.SetBus(k.Bus())
	for i := 0; i < 2; i++ {
		_, _ = c.Stream(context.Background(), controlplane.CmdRun, map[string]any{"intent": "x"}, nil)
	}

	res, err := c.Call(context.Background(), controlplane.CmdRateLimitLog,
		map[string]any{"cursor": "definitely-not-valid", "limit": float64(10)})
	if err != nil {
		t.Fatalf("Call with bad cursor errored: %v", err)
	}
	rows, _ := res["throttles"].([]any)
	if len(rows) < 1 {
		t.Fatalf("bad cursor rows = %d, want at least 1 (fallback to newest page)", len(rows))
	}
}
