// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/event"
)

// TokenText returns the streamed text delta carried by an llm.token event, and
// false for any other event kind (or an empty delta). Use it inside a RunStream
// callback to render the answer as the model generates it:
//
//	c.RunStream(ctx, intent, func(ev *sdk.Event) {
//		if txt, ok := sdk.TokenText(ev); ok {
//			fmt.Print(txt)
//		}
//	})
func TokenText(ev *Event) (string, bool) {
	if ev == nil || ev.Kind != event.KindLLMToken {
		return "", false
	}
	var p struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(ev.Payload, &p) != nil || p.Text == "" {
		return "", false
	}
	return p.Text, true
}

// ToolCall returns the tool name from a tool.invoked event, and false for any
// other kind. Use it to show which tool the agent is reaching for.
func ToolCall(ev *Event) (string, bool) {
	if ev == nil || ev.Kind != event.KindToolInvoked {
		return "", false
	}
	var p struct {
		Tool string `json:"tool"`
	}
	if json.Unmarshal(ev.Payload, &p) != nil || p.Tool == "" {
		return "", false
	}
	return p.Tool, true
}

// IsTerminal reports whether ev is a run's terminal event — task.completed or
// task.failed. After it fires no more run events follow.
func IsTerminal(ev *Event) bool {
	return ev != nil && (ev.Kind == event.KindTaskCompleted || ev.Kind == event.KindTaskFailed)
}
