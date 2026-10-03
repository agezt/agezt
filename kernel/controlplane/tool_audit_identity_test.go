// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// Providers and direct callers can reuse call IDs across runs. Denied calls
// have no invocation and must not inherit another run's input or latency.
func TestToolAudit_JoinsWithinCorrelation(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New())
	for _, spec := range []event.Spec{
		{Kind: event.KindToolInvoked, CorrelationID: "a", Payload: map[string]any{"tool": "probe", "call_id": "reused", "input": map[string]any{"marker": "A"}}},
		{Kind: event.KindToolInvoked, CorrelationID: "b", Payload: map[string]any{"tool": "probe", "call_id": "reused", "input": map[string]any{"marker": "B"}}},
		{Kind: event.KindToolResult, CorrelationID: "a", Payload: map[string]any{"tool": "probe", "call_id": "reused", "output": "A", "error": false}},
		{Kind: event.KindToolResult, CorrelationID: "denied", Payload: map[string]any{"tool": "probe", "call_id": "reused", "output": "tool call denied by policy", "error": true}},
		{Kind: event.KindToolResult, CorrelationID: "b", Payload: map[string]any{"tool": "probe", "call_id": "reused", "output": "B", "error": false}},
	} {
		spec.Subject, spec.Actor = "tool", "tool"
		if _, err := k.Bus().Publish(spec); err != nil {
			t.Fatal(err)
		}
	}
	log, err := c.Call(context.Background(), controlplane.CmdToolLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := log["invocations"].([]any)
	if len(rows) != 3 {
		t.Fatalf("rows=%v", rows)
	}
	for _, raw := range rows {
		row := raw.(map[string]any)
		input, _ := row["input"].(string)
		switch row["correlation_id"] {
		case "a":
			if !strings.Contains(input, `"marker":"A"`) {
				t.Errorf("run A borrowed input: %v", row)
			}
		case "b":
			if !strings.Contains(input, `"marker":"B"`) {
				t.Errorf("run B input: %v", row)
			}
		case "denied":
			if input != "" || row["duration_ms"] != float64(0) {
				t.Errorf("denial borrowed invocation: %v", row)
			}
		default:
			t.Errorf("unexpected row=%v", row)
		}
	}
	stats, err := c.Call(context.Background(), controlplane.CmdToolStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	latency, _ := stats["duration_ms"].(map[string]any)
	if latency["count"] != float64(2) {
		t.Fatalf("latency samples=%v; denial has no invocation", latency)
	}
}

func TestToolAudit_SkippedBatchCallHasNoExecutionLatency(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New())
	for _, spec := range []event.Spec{
		{Kind: event.KindToolInvoked, Payload: map[string]any{"tool": "probe", "call_id": "ran", "input": map[string]any{}}},
		{Kind: event.KindToolInvoked, Payload: map[string]any{"tool": "probe", "call_id": "skipped", "input": map[string]any{}}},
		{Kind: event.KindToolResult, Payload: map[string]any{"tool": "probe", "call_id": "ran", "output": "finished", "error": false}},
		{Kind: event.KindToolResult, Payload: map[string]any{"tool": "probe", "call_id": "skipped", "output": "not executed after panic", "error": true, "not_executed": true}},
	} {
		if spec.Kind == event.KindToolResult {
			// A nonzero apparent dispatch span must never become execution
			// latency for a skipped call, even on fast Linux filesystems.
			time.Sleep(2 * time.Millisecond)
		}
		spec.Subject, spec.Actor, spec.CorrelationID = "tool", "ops", "batch"
		if _, err := k.Bus().Publish(spec); err != nil {
			t.Fatal(err)
		}
	}
	log, err := c.Call(context.Background(), controlplane.CmdToolLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := log["invocations"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows=%v", rows)
	}
	for _, raw := range rows {
		row := raw.(map[string]any)
		if row["call_id"] == "skipped" && (row["not_executed"] != true || row["duration_ms"] != float64(0)) {
			t.Errorf("skipped row=%v", row)
		}
	}
	stats, err := c.Call(context.Background(), controlplane.CmdToolStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	latency := stats["duration_ms"].(map[string]any)
	if latency["count"] != float64(1) {
		t.Errorf("latency samples=%v; skipped call never executed", latency)
	}
}
