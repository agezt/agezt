// SPDX-License-Identifier: MIT

package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// Gemini rejects dotted function names; the encoded request must carry the
// conformed name, never the raw "browser.read".
func TestEncodeConformsToolNames(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read", Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	body, err := encodeRequest("", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, tools, 100, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "browser.read") {
		t.Fatalf("raw dotted tool name leaked: %s", body)
	}
	if !strings.Contains(string(body), "browser_read") {
		t.Fatalf("conformed name missing: %s", body)
	}
}
