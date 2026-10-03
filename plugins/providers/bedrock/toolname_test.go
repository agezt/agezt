// SPDX-License-Identifier: MIT

package bedrock

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

func TestEncodeConformsToolNames(t *testing.T) {
	tools := []toolapi.ToolDef{{Name: "browser.read", Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	body, err := encodeAnthropicOnBedrockRequest("", []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, tools, 100, llm.Params{}, nil)
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
