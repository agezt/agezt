// SPDX-License-Identifier: MIT

package vertex

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_ToolResultControlBytesValidJSON pins M483 (same class as M481
// for google): a tool result with control bytes (ANSI \x1b, NUL) must encode to
// VALID JSON. The old strconv.Quote path produced invalid JSON and wedged the loop.
func TestEncodeRequest_ToolResultControlBytesValidJSON(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: "run it"},
		{Role: agent.RoleTool, ToolCallID: "call-0", Content: "ANSI:\x1b[31mred\x1b[0m NUL:\x00 done"},
	}
	body, err := encodeRequest("", msgs, nil, 0, false, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest with control-byte tool result: %v", err)
	}
	if !json.Valid(body) {
		t.Fatalf("encoded request is not valid JSON:\n%s", body)
	}
}
