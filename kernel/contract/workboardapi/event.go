// SPDX-License-Identifier: MIT

package workboardapi

// The board's ANSWERS that both sides name: the refusal a host gives when it
// holds no board, and the shape of the event the board publishes.
//
// The payload lives here because it is a ROW both sides speak: the board
// publishes it, the host's tools read it, and the agent-facing surface shows
// it. One row, one home — the contract's.

import "errors"

// ErrNoStore is the answer the board gives when the host holds no board.
//
// It is the contract's row because the host may not name the module that
// keeps the board, and the module may not name the host: the refusal has to
// be something both can hold.
var ErrNoStore = errors.New("workboard: the task board is not available on this daemon (no workboard store)")

// TaskEventPayload builds the payload the board publishes for a task.
//
// The extra fields are the ones a particular action adds (the agent that
// claimed it, the reason it was blocked, the sweep that reclaimed it). An
// empty value is left out: a payload says what is, not what is blank.
func TaskEventPayload(t Task, action string, extra map[string]any) map[string]any {
	payload := map[string]any{
		"id":       t.ID,
		"title":    t.Title,
		"status":   string(t.Status),
		"priority": t.Priority,
		"assignee": t.Assignee,
		"tenant":   t.Tenant,
		"action":   action,
	}
	for key, val := range extra {
		if val != "" {
			payload[key] = val
		}
	}
	return payload
}
