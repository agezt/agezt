// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type changingObservationTool struct {
	outputs []string
	calls   int
}

func (t *changingObservationTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "watch", Description: "observe state", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t *changingObservationTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	if t.calls >= len(t.outputs) {
		return agent.Result{Output: t.outputs[len(t.outputs)-1]}, nil
	}
	out := t.outputs[t.calls]
	t.calls++
	return agent.Result{Output: out}, nil
}

func TestDiffObservation_LineDelta(t *testing.T) {
	got, ok := agent.DiffObservation("alpha\nbeta", "alpha\ngamma")
	if !ok {
		t.Fatal("DiffObservation returned ok=false")
	}
	for _, want := range []string{"1 added", "1 removed", "+ gamma", "- beta"} {
		if !strings.Contains(got, want) {
			t.Fatalf("delta %q missing %q", got, want)
		}
	}
}

func TestRun_ObservationDeltasFeedsDeltaButJournalsRaw(t *testing.T) {
	b, j := newTestBus(t)
	tool := &changingObservationTool{outputs: []string{"alpha\nbeta", "alpha\nbeta\ngamma"}}
	var lastToolContent string
	prov := mock.New(
		testToolUse("c1", "watch", map[string]any{}),
		testToolUse("c2", "watch", map[string]any{}),
		mock.FinalText("done"),
	)
	prov.OnRequest = func(req agent.CompletionRequest) {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == agent.RoleTool {
				lastToolContent = req.Messages[i].Content
				return
			}
		}
	}
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:          prov,
		Tools:             map[string]agent.Tool{"watch": tool},
		ObservationDeltas: true,
		Bus:               b,
		Actor:             "agent-observe",
		CorrelationID:     "corr-observe",
	}, "watch state twice"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(lastToolContent, "observation delta") || !strings.Contains(lastToolContent, "+ gamma") {
		t.Fatalf("last tool message = %q, want delta with added gamma", lastToolContent)
	}

	var sawDeltaPayload bool
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != event.KindToolResult {
			return nil
		}
		var p struct {
			Output           string `json:"output"`
			ObservationDelta bool   `json:"observation_delta"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		if p.ObservationDelta {
			sawDeltaPayload = true
			if p.Output != "alpha\nbeta\ngamma" {
				t.Fatalf("journal output = %q, want raw second observation", p.Output)
			}
		}
		return nil
	})
	if !sawDeltaPayload {
		t.Fatal("tool.result did not mark the delta observation")
	}
}
