// SPDX-License-Identifier: MIT

package toolpipeline

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// Settle records one terminal result through the mandatory caller publisher.
// Callers own outcome classification, output representation and extra metadata.
// The metadata map is copied; call identity and result fields remain authoritative.
// Hooks, model messages and batch error aggregation stay outside this phase.
func Settle(call llm.ToolCall, result toolapi.Result, fields map[string]any, publish func(event.Kind, map[string]any) error) error {
	payload := make(map[string]any, len(fields)+4)
	for key, value := range fields {
		payload[key] = value
	}
	payload["tool"] = call.Name
	payload["call_id"] = call.ID
	payload["output"] = result.Output
	payload["error"] = result.IsError
	return publish(event.KindToolResult, payload)
}
