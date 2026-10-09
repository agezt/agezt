// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

func (s *Observations) Log(_ context.Context, in LogInput) (LogOutput, error) {
	// One row per tool.result (the always-present event: a policy-denied call
	// emits a result but no tool.invoked). A first-seen tool.invoked stashes the
	// call's input by (correlation_id, call_id) so the row can show what the agent asked for; since
	// the journal is in order, the invoked event precedes its result and the maps
	// (closure state across decode calls) are already populated when we reach it.
	//
	// tool.invoked events are stashed even when outside the since_ms window
	// (an invoked can precede the cutoff its result falls inside) — safe
	// because ProjectValues runs decode on every event and applies the
	// cutoff to decoded ROWS only, and rows only come from tool.result.
	inputs := map[toolInvocationKey]string{}   // (run, call) → input preview
	invokedTS := map[toolInvocationKey]int64{} // (run, call) → invocation timestamp
	output, err := journalview.ProjectValues(s.journal, in.Page, func(e *event.Event) (LogItem, bool) {
		switch e.Kind {
		case event.KindToolInvoked:
			id, input := decodeToolInvoked(e.Payload)
			if id != "" {
				key := toolInvocationKey{e.CorrelationID, id}
				inputs[key] = input
				invokedTS[key] = e.TSUnixMS
			}
			return LogItem{}, false
		case event.KindToolResult:
			decoded := decodeToolResult(e.Payload)
			key := toolInvocationKey{e.CorrelationID, decoded.callID}
			if in.Tool != "" && decoded.tool != in.Tool {
				return LogItem{}, false
			}
			if in.ErrorsOnly && !decoded.isError {
				return LogItem{}, false
			}
			// Latency (M71) joins the call's invoked→result span within the run. A
			// policy-denied call has no tool.invoked, so it has no latency (0).
			var dur int64
			if it, ok := invokedTS[key]; ok && !decoded.notExecuted && e.TSUnixMS >= it {
				dur = e.TSUnixMS - it
			}
			if in.SlowMS > 0 && dur < in.SlowMS {
				return LogItem{}, false // M73: faster than the latency floor (or unmeasurable)
			}
			row := LogItem{Actor: e.Actor, CorrelationID: e.CorrelationID, Tool: decoded.tool, CallID: decoded.callID, Input: inputs[key], Output: decoded.output, Error: decoded.isError, DurationMS: dur, ObservationTrust: decoded.observationTrust, ObservationSource: decoded.observationSource, DirectiveLike: decoded.directiveLike, DirectiveMatches: decoded.directiveMatches, Seq: e.Seq, TSUnixMS: e.TSUnixMS, NotExecuted: decoded.notExecuted}
			return row, true
		default:
			return LogItem{}, false
		}
	})
	if err != nil {
		return LogOutput{}, err
	}
	return LogOutput{Invocations: output.Rows, Count: output.Count, NextCursor: output.NextCursor}, nil

}
