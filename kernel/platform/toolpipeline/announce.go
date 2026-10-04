// SPDX-License-Identifier: MIT

package toolpipeline

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
)

// Announce records invocation admission before execution. Callers decide when
// a call is eligible (after policy and any memo lookup) and stamp their own
// journal envelope. The mandatory publisher's error is returned unchanged.
// No executor is accepted: the loop may finish admission of the whole batch
// before performing any effects.
func Announce(call llm.ToolCall, publish func(event.Kind, map[string]any) error) error {
	return publish(event.KindToolInvoked, map[string]any{
		"tool":    call.Name,
		"call_id": call.ID,
		"input":   call.Input,
	})
}
