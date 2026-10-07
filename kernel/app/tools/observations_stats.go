// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

func (s *Observations) Stats(_ context.Context, in StatsInput) (StatsOutput, error) {
	var total, errored int
	type toolAgg struct {
		calls, errors      int
		durSum, durSamples int64 // M75: per-tool latency, to show which TOOL is slow
	}
	byTool := map[string]*toolAgg{}
	invokedTS := map[toolInvocationKey]int64{} // (run, call) → invocation timestamp
	durations := make([]int64, 0)              // per-call latency, for the distribution (M71)
	// Failure-mode breakdown (M79): bucket error outputs by their message so an
	// operator sees WHAT is failing (denied / not-available / timeout / …), not
	// just how many — the tool analogue of runs stats' failed_by_reason.
	errorsByMessage := map[string]int{}
	if err := s.journal.Range(func(e *event.Event) error {
		if e.Kind == event.KindToolInvoked {
			if id, _ := decodeToolInvoked(e.Payload); id != "" {
				invokedTS[toolInvocationKey{e.CorrelationID, id}] = e.TSUnixMS
			}
			return nil
		}
		if e.Kind != event.KindToolResult {
			return nil
		}
		if in.CutoffMS > 0 && e.TSUnixMS < in.CutoffMS {
			return nil
		}
		decoded := decodeToolResult(e.Payload)
		if in.Tool != "" && decoded.tool != in.Tool {
			return nil
		}
		tool := decoded.tool
		if tool == "" {
			tool = "unknown"
		}
		total++
		agg := byTool[tool]
		if agg == nil {
			agg = &toolAgg{}
			byTool[tool] = agg
		}
		agg.calls++
		if decoded.isError {
			errored++
			agg.errors++
			msg := decoded.output // already whitespace-collapsed + capped by decodeToolResult
			if msg == "" {
				msg = "(no message)"
			}
			errorsByMessage[msg]++
		}
		if it, ok := invokedTS[toolInvocationKey{e.CorrelationID, decoded.callID}]; ok && !decoded.notExecuted && e.TSUnixMS >= it {
			d := e.TSUnixMS - it
			durations = append(durations, d)
			agg.durSum += d
			agg.durSamples++
		}
		return nil
	}); err != nil {
		return StatsOutput{}, err
	}

	errorRate := 0.0
	if total > 0 {
		errorRate = float64(errored) / float64(total)
	}
	byToolOut := make(map[string]ToolSummary, len(byTool))
	for tool, agg := range byTool {
		entry := ToolSummary{Calls: agg.calls, Errors: agg.errors}
		if agg.durSamples > 0 {
			avg := agg.durSum / agg.durSamples
			entry.AvgMS = &avg // M75: per-tool mean latency
		}
		byToolOut[tool] = entry
	}
	// Latency distribution (M71) over calls with a joinable invoked→result span,
	// reusing the nearest-rank durationStats so it reads like runs stats' block.
	dstats := journalview.SummarizeDurations(durations)

	return StatsOutput{
		Total:           total,
		Errored:         errored,
		ErrorRate:       errorRate,
		ByTool:          byToolOut,
		Tools:           len(byTool),
		WindowMS:        in.WindowMS,
		ErrorsByMessage: errorsByMessage, // M79: failure-mode breakdown
		DurationMS:      LatencySummary{Count: len(durations), DurationStats: dstats},
	}, nil
}
