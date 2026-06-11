// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// multiToolUse scripts one assistant turn that fans out n calls to the same
// tool, with call IDs c0..c(n-1).
func multiToolUse(toolName string, n int) agent.CompletionResponse {
	calls := make([]agent.ToolCall, 0, n)
	for i := range n {
		calls = append(calls, agent.ToolCall{
			ID:    "c" + string(rune('0'+i)),
			Name:  toolName,
			Input: json.RawMessage(`{}`),
		})
	}
	return agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, ToolCalls: calls},
		StopReason: agent.StopToolUse,
	}
}

// barrierTool only returns once `want` invocations are in flight at the same
// time — so the test FAILS (tool reports no-overlap) unless the loop truly
// executes the turn's calls concurrently.
type barrierTool struct {
	want    int32
	arrived atomic.Int32
	release chan struct{}
	once    sync.Once
}

func newBarrierTool(want int) *barrierTool {
	return &barrierTool{want: int32(want), release: make(chan struct{})}
}

func (b *barrierTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "barrier", Description: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (b *barrierTool) Invoke(ctx context.Context, _ json.RawMessage) (agent.Result, error) {
	if b.arrived.Add(1) >= b.want {
		b.once.Do(func() { close(b.release) })
	}
	select {
	case <-b.release:
		return agent.Result{Output: "ok"}, nil
	case <-time.After(3 * time.Second):
		return agent.Result{Output: "no overlap", IsError: true}, nil
	case <-ctx.Done():
		return agent.Result{}, ctx.Err()
	}
}

// concTool records the maximum number of concurrent invocations it observed.
type concTool struct {
	mu       sync.Mutex
	cur, max int
}

func (c *concTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "conc", Description: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (c *concTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	c.mu.Lock()
	c.cur++
	if c.cur > c.max {
		c.max = c.cur
	}
	c.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	c.mu.Lock()
	c.cur--
	c.mu.Unlock()
	return agent.Result{Output: "done"}, nil
}

// TestRun_ParallelToolDispatch_Overlap (M880): three tool calls issued in ONE
// assistant turn execute concurrently. The barrier tool deadlocks (well,
// times out into an error result) unless all three are in flight together, so
// a regression to sequential dispatch turns the results into errors.
func TestRun_ParallelToolDispatch_Overlap(t *testing.T) {
	b, j := newTestBus(t)
	tool := newBarrierTool(3)
	prov := mock.New(multiToolUse("barrier", 3), mock.FinalText("all done"))

	got, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Tools:         map[string]agent.Tool{"barrier": tool},
		Actor:         "agent-par",
		CorrelationID: "corr-par",
	}, "fan out")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "all done" {
		t.Errorf("answer = %q, want %q", got, "all done")
	}

	// Every result must be the barrier's success output — and journaled in
	// the ORIGINAL call order (c0, c1, c2) despite concurrent execution.
	var ids []string
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != event.KindToolResult {
			return nil
		}
		var p struct {
			CallID string `json:"call_id"`
			Output string `json:"output"`
			Error  bool   `json:"error"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("unmarshal tool.result: %v", err)
		}
		if p.Error || p.Output != "ok" {
			t.Errorf("tool.result %s: output=%q error=%v — calls did not overlap", p.CallID, p.Output, p.Error)
		}
		ids = append(ids, p.CallID)
		return nil
	})
	want := []string{"c0", "c1", "c2"}
	if len(ids) != len(want) {
		t.Fatalf("tool.result events = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("tool.result order: got %v, want %v", ids, want)
			break
		}
	}
}

// TestRun_ParallelToolDispatch_CapBoundsConcurrency: MaxParallelTools caps how
// many of a turn's calls run at once, and a negative value forces the
// historical strictly-sequential behaviour.
func TestRun_ParallelToolDispatch_CapBoundsConcurrency(t *testing.T) {
	t.Run("cap=2", func(t *testing.T) {
		b, _ := newTestBus(t)
		tool := &concTool{}
		prov := mock.New(multiToolUse("conc", 4), mock.FinalText("done"))
		if _, err := agent.Run(context.Background(), agent.LoopConfig{
			Provider: prov, Bus: b,
			Tools:            map[string]agent.Tool{"conc": tool},
			MaxParallelTools: 2,
			Actor:            "agent-cap", CorrelationID: "corr-cap",
		}, "go"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if tool.max > 2 {
			t.Errorf("observed concurrency %d, cap 2", tool.max)
		}
	})
	t.Run("negative=sequential", func(t *testing.T) {
		b, _ := newTestBus(t)
		tool := &concTool{}
		prov := mock.New(multiToolUse("conc", 3), mock.FinalText("done"))
		if _, err := agent.Run(context.Background(), agent.LoopConfig{
			Provider: prov, Bus: b,
			Tools:            map[string]agent.Tool{"conc": tool},
			MaxParallelTools: -1,
			Actor:            "agent-seq", CorrelationID: "corr-seq",
		}, "go"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if tool.max != 1 {
			t.Errorf("observed concurrency %d, want strictly sequential (1)", tool.max)
		}
	})
}

// panicTool panics on invoke — exercising the per-goroutine panic firewall of
// the parallel dispatch path (a goroutine panic would otherwise crash the
// whole process, not just fail the run).
type panicTool struct{}

func (panicTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "boom", Description: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (panicTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	panic("kaboom")
}

func TestRun_ParallelToolDispatch_PanicFailsRunNotProcess(t *testing.T) {
	b, _ := newTestBus(t)
	prov := mock.New(multiToolUse("boom", 2), mock.FinalText("unreachable"))
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b,
		Tools: map[string]agent.Tool{"boom": panicTool{}},
		Actor: "agent-boom", CorrelationID: "corr-boom",
	}, "go")
	if err == nil {
		t.Fatal("Run succeeded, want panic-derived error")
	}
}
