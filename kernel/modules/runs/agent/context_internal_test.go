// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"testing"
)

// TestContextSize_ByRole verifies the SPEC-10 §3.5 context-size accounting:
// content length plus tool-call argument JSON, summed and grouped by role, with
// image attachments excluded (separate modality).
func TestContextSize_ByRole(t *testing.T) {
	const system = "system-prompt" // 13
	msgs := []Message{
		{Role: RoleUser, Content: "the task"}, // 8
		{Role: RoleAssistant, Content: "ok", ToolCalls: []ToolCall{{Input: json.RawMessage(`{"x":1}`)}}}, // 2 + 7 = 9
		{Role: RoleTool, Content: "tool-output-here"},                                                    // 16
		{Role: RoleUser, Content: "more", Images: []string{"data:image/png;base64,AAAA"}},                // 4 (image excluded)
	}
	total, byRole := contextSize(system, msgs)

	wantByRole := map[string]int{"system": 13, "user": 12, "assistant": 9, "tool": 16}
	for role, want := range wantByRole {
		if byRole[role] != want {
			t.Errorf("byRole[%q] = %d, want %d", role, byRole[role], want)
		}
	}
	if len(byRole) != len(wantByRole) {
		t.Errorf("byRole has %d roles, want %d: %v", len(byRole), len(wantByRole), byRole)
	}
	wantTotal := 13 + 12 + 9 + 16
	if total != wantTotal {
		t.Errorf("total = %d, want %d (image bytes must be excluded)", total, wantTotal)
	}
}

// TestContextSize_Empty: no system + no messages → zero total, empty (non-nil) map.
func TestContextSize_Empty(t *testing.T) {
	total, byRole := contextSize("", nil)
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if byRole == nil {
		t.Error("byRole should be a non-nil empty map")
	}
	if len(byRole) != 0 {
		t.Errorf("byRole should be empty, got %v", byRole)
	}
}
