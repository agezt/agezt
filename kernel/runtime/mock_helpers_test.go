// SPDX-License-Identifier: MIT

package runtime_test

import (
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/internal/testfixtures"
)

// testWithUsage sets the usage on a CompletionResponse.
// Delegates to kernel/internal/testfixtures for the canonical implementation.
func testWithUsage(resp agent.CompletionResponse, usage agent.Usage) agent.CompletionResponse {
	return testfixtures.WithUsage(resp, usage)
}

// testToolUse constructs a tool-use CompletionResponse.
// Delegates to kernel/internal/testfixtures for the canonical implementation.
func testToolUse(callID, toolName string, input any) agent.CompletionResponse {
	return testfixtures.ToolUse(callID, toolName, input)
}
