// SPDX-License-Identifier: MIT

package agent_test

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
)

func testWithUsage(resp agent.CompletionResponse, usage agent.Usage) agent.CompletionResponse {
	resp.Usage = usage
	return resp
}

func testToolUse(callID, toolName string, input any) agent.CompletionResponse {
	raw, err := json.Marshal(input)
	if err != nil {
		panic("testToolUse: marshal input: " + err.Error())
	}
	return agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{{ID: callID, Name: toolName, Input: raw}},
		},
		StopReason: agent.StopToolUse,
	}
}
