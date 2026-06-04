// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// bigTool returns a fixed, large output so the loop's artifact offload triggers.
type bigTool struct{ out string }

func (b bigTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "dump", Description: "emit a large blob", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (b bigTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: b.out}, nil
}

// TestRun_OffloadsLargeToolOutput: with an artifact store configured, a tool
// output larger than the threshold is offloaded — the journaled tool.result
// carries a preview + raw_ref + output_bytes (not the full bytes), and the full
// output is retrievable from the store by that ref.
func TestRun_OffloadsLargeToolOutput(t *testing.T) {
	b, j := newTestBus(t)
	store, err := artifact.Open(t.TempDir())
	if err != nil {
		t.Fatalf("artifact.Open: %v", err)
	}
	big := strings.Repeat("DATA", 5000) // 20 KB, well over the 8 KiB default

	prov := mock.New(
		mock.ToolUse("call-1", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"dump": bigTool{out: big}},
		Bus:           b,
		Actor:         "agent-1",
		CorrelationID: "corr-offload",
		Artifacts:     store,
	}, "dump a big blob"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Find the tool.result event and inspect its payload.
	var payload struct {
		Output      string `json:"output"`
		RawRef      string `json:"raw_ref"`
		OutputBytes int    `json:"output_bytes"`
	}
	found := false
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindToolResult {
			_ = json.Unmarshal(e.Payload, &payload)
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("no tool.result event journaled")
	}
	if payload.RawRef == "" {
		t.Fatal("tool.result should carry a raw_ref for the offloaded output")
	}
	if payload.OutputBytes != len(big) {
		t.Errorf("output_bytes = %d, want full length %d", payload.OutputBytes, len(big))
	}
	if len(payload.Output) >= len(big) {
		t.Errorf("event output should be a preview (%d chars), not the full %d bytes", len(payload.Output), len(big))
	}
	// The full output is recoverable from the store by the ref.
	got, err := store.Get(payload.RawRef)
	if err != nil {
		t.Fatalf("store.Get(%s): %v", payload.RawRef, err)
	}
	if string(got) != big {
		t.Errorf("stored artifact != original output (%d vs %d bytes)", len(got), len(big))
	}
}

// TestRun_SmallToolOutputStaysInline: a small output is journaled inline, no
// raw_ref — the offload must not change ordinary results.
func TestRun_SmallToolOutputStaysInline(t *testing.T) {
	b, j := newTestBus(t)
	store, _ := artifact.Open(t.TempDir())

	prov := mock.New(
		mock.ToolUse("call-1", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"dump": bigTool{out: "small result"}},
		Bus:           b,
		Actor:         "agent-1",
		CorrelationID: "corr-inline",
		Artifacts:     store,
	}, "dump"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var payload struct {
		Output string `json:"output"`
		RawRef string `json:"raw_ref"`
	}
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindToolResult {
			_ = json.Unmarshal(e.Payload, &payload)
		}
		return nil
	})
	if payload.RawRef != "" {
		t.Errorf("small output should NOT be offloaded, got raw_ref %q", payload.RawRef)
	}
	if payload.Output != "small result" {
		t.Errorf("small output should be inline, got %q", payload.Output)
	}
}
