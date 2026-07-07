// SPDX-License-Identifier: MIT

package toolname

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestMaps_LongNameLeavesSuffixRoom covers the branch that truncates a
// sanitized base to maxLen-4 so a collision suffix always fits within the cap.
func TestMaps_LongNameLeavesSuffixRoom(t *testing.T) {
	// A 100-char name → Sanitize caps to 64, Maps truncates to maxLen-4 (60).
	long := strings.Repeat("a", 100)
	tools := []agent.ToolDef{{Name: long}}
	fwd, _ := Maps(tools)
	wire := fwd[long]
	if len(wire) != 60 { // maxLen(64) - 4
		t.Fatalf("wire len = %d, want 60 (room for suffix): %q", len(wire), wire)
	}
}

// TestMaps_DuplicateToolName covers the `dup` continue branch: a repeated tool
// name must not produce a second wire mapping.
func TestMaps_DuplicateToolName(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "browser.read"},
		{Name: "browser.read"}, // duplicate → skipped
	}
	fwd, rev := Maps(tools)
	if len(fwd) != 1 {
		t.Fatalf("fwd should have 1 entry, got %d: %v", len(fwd), fwd)
	}
	if fwd["browser.read"] != "browser_read" {
		t.Fatalf("fwd = %v", fwd)
	}
	if rev["browser_read"] != "browser.read" {
		t.Fatalf("rev = %v", rev)
	}
}

// TestMaps_CollisionSuffix covers the injective collision-breaking suffix loop:
// two distinct names that sanitize to the same wire string must get distinct
// wire names.
func TestMaps_CollisionSuffix(t *testing.T) {
	// "a.b" and "a/b" both sanitize to "a_b".
	tools := []agent.ToolDef{
		{Name: "a.b"},
		{Name: "a/b"},
	}
	fwd, rev := Maps(tools)
	w1 := fwd["a.b"]
	w2 := fwd["a/b"]
	if w1 == w2 {
		t.Fatalf("collision not broken: both mapped to %q", w1)
	}
	if w1 != "a_b" {
		t.Fatalf("first name should keep base wire, got %q", w1)
	}
	if w2 != "a_b_2" {
		t.Fatalf("second name should get _2 suffix, got %q", w2)
	}
	// Both changed → both in rev, distinct originals.
	if rev[w1] != "a.b" || rev[w2] != "a/b" {
		t.Fatalf("rev mapping wrong: %v", rev)
	}
}

// TestMaps_NoChanges verifies rev is nil when nothing needs conforming.
func TestMaps_NoChanges(t *testing.T) {
	tools := []agent.ToolDef{{Name: "shell"}, {Name: "web_search"}}
	fwd, rev := Maps(tools)
	if rev != nil {
		t.Fatalf("rev should be nil when no name changed, got %v", rev)
	}
	if fwd["shell"] != "shell" || fwd["web_search"] != "web_search" {
		t.Fatalf("fwd = %v", fwd)
	}
}

// TestWire_NotInMap covers Wire's fallback path: a name absent from fwd is
// sanitized directly (never sent non-conforming).
func TestWire_NotInMap(t *testing.T) {
	// nil map → sanitize directly.
	if got := Wire(nil, "old.tool"); got != "old_tool" {
		t.Fatalf("Wire(nil,...) = %q, want old_tool", got)
	}
	// Non-nil map missing the key → sanitize directly.
	fwd := map[string]string{"present": "present"}
	if got := Wire(fwd, "gone.tool"); got != "gone_tool" {
		t.Fatalf("Wire(missing) = %q, want gone_tool", got)
	}
	// Present key → mapped value.
	if got := Wire(fwd, "present"); got != "present" {
		t.Fatalf("Wire(present) = %q", got)
	}
}

// TestRestoreCalls covers all branches: nil response, empty rev, rename hit,
// and rename miss.
func TestRestoreCalls(t *testing.T) {
	rev := map[string]string{"browser_read": "browser.read"}

	// nil response → no panic, no-op.
	RestoreCalls(nil, rev)

	// empty rev → no-op.
	resp := &agent.CompletionResponse{
		Message: agent.Message{
			ToolCalls: []agent.ToolCall{{Name: "browser_read"}},
		},
	}
	RestoreCalls(resp, nil)
	if resp.Message.ToolCalls[0].Name != "browser_read" {
		t.Fatalf("empty rev should not rename, got %q", resp.Message.ToolCalls[0].Name)
	}

	// rev with a hit → renamed back to original; a miss stays as-is.
	resp2 := &agent.CompletionResponse{
		Message: agent.Message{
			ToolCalls: []agent.ToolCall{
				{Name: "browser_read"}, // in rev → renamed
				{Name: "shell"},        // not in rev → unchanged
			},
		},
	}
	RestoreCalls(resp2, rev)
	if resp2.Message.ToolCalls[0].Name != "browser.read" {
		t.Fatalf("hit should be renamed, got %q", resp2.Message.ToolCalls[0].Name)
	}
	if resp2.Message.ToolCalls[1].Name != "shell" {
		t.Fatalf("miss should be unchanged, got %q", resp2.Message.ToolCalls[1].Name)
	}
}
