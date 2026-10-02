// SPDX-License-Identifier: MIT

// Package testfixtures provides shared test helpers for kernel packages.
// It is internal to the kernel tree and only imported by _test.go files.
package testfixtures

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// WithUsage sets the usage on a CompletionResponse (test helper).
func WithUsage(resp llm.CompletionResponse, usage llm.Usage) llm.CompletionResponse {
	resp.Usage = usage
	return resp
}

// ToolUse constructs a tool-use CompletionResponse (test helper).
func ToolUse(callID, toolName string, input any) llm.CompletionResponse {
	raw, err := json.Marshal(input)
	if err != nil {
		panic("ToolUse: marshal input: " + err.Error())
	}
	return llm.CompletionResponse{
		Message: llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{ID: callID, Name: toolName, Input: raw}},
		},
		StopReason: llm.StopToolUse,
	}
}
