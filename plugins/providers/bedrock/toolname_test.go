// SPDX-License-Identifier: MIT

package bedrock

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestEncodeConformsToolNames(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read", Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	body, err := encodeAnthropicOnBedrockRequest("", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, tools, 100, agent.Params{}, nil)
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
