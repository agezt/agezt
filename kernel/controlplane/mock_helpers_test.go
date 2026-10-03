// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/internal/testfixtures"
)

// testWithUsage sets the usage on a CompletionResponse.
// Delegates to kernel/internal/testfixtures for the canonical implementation.
func testWithUsage(resp llm.CompletionResponse, usage llm.Usage) llm.CompletionResponse {
	return testfixtures.WithUsage(resp, usage)
}

// testToolUse constructs a tool-use CompletionResponse.
// Delegates to kernel/internal/testfixtures for the canonical implementation.
func testToolUse(callID, toolName string, input any) llm.CompletionResponse {
	return testfixtures.ToolUse(callID, toolName, input)
}
