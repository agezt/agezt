// SPDX-License-Identifier: MIT

package google

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_ToolResultControlBytesValidJSON pins M481: a tool result whose
// content contains control bytes (ESC \x1b from ANSI color, a NUL, etc.) must still
// encode to VALID JSON. The old strconv.Quote path emitted Go-only \xNN escapes
// that are invalid JSON, failing the whole request encode and wedging the agent
// loop on Gemini.
func TestEncodeRequest_ToolResultControlBytesValidJSON(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: "run it"},
		{Role: agent.RoleTool, ToolCallID: "call-0", Content: "ANSI:\x1b[31mred\x1b[0m and NUL:\x00 done"},
	}
	body, err := encodeRequest("", msgs, nil, 0, false, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest with control-byte tool result: %v", err)
	}
	if !json.Valid(body) {
		t.Fatalf("encoded request is not valid JSON:\n%s", body)
	}
}
