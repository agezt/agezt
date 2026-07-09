// SPDX-License-Identifier: MIT

// Package testfixtures provides shared test helpers for kernel packages.
// It is internal to the kernel tree and only imported by _test.go files.
package testfixtures

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
)

// WithUsage sets the usage on a CompletionResponse (test helper).
func WithUsage(resp agent.CompletionResponse, usage agent.Usage) agent.CompletionResponse {
	resp.Usage = usage
	return resp
}

// ToolUse constructs a tool-use CompletionResponse (test helper).
func ToolUse(callID, toolName string, input any) agent.CompletionResponse {
	raw, err := json.Marshal(input)
	if err != nil {
		panic("ToolUse: marshal input: " + err.Error())
	}
	return agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{{ID: callID, Name: toolName, Input: raw}},
		},
		StopReason: agent.StopToolUse,
	}
}
