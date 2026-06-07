// SPDX-License-Identifier: MIT

package bedrock

import (
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestDecodeMistralOnBedrock_HardcodesAssistantRole pins M484: the canonical
// response role must be assistant even when the Bedrock-Mistral backend omits
// message.role (common for OpenAI-shaped backends). A blank/other role would
// misclassify the turn for downstream role switches.
func TestDecodeMistralOnBedrock_HardcodesAssistantRole(t *testing.T) {
	// Response body with NO "role" field on the message.
	body := []byte(`{"choices":[{"message":{"content":"hi there"},"finish_reason":"stop"}]}`)
	resp, err := decodeMistralOnBedrockResponse(body, "mistral.test")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Message.Role != agent.RoleAssistant {
		t.Errorf("role = %q, want %q (must be hard-coded, not taken from the wire)", resp.Message.Role, agent.RoleAssistant)
	}
	if resp.Message.Content != "hi there" {
		t.Errorf("content = %q", resp.Message.Content)
	}
}
