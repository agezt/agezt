// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// flakyTool fails its first failUntil invocations, then succeeds.
type flakyTool struct {
	mu        sync.Mutex
	calls     int
	failUntil int
	block     bool // when set, every call blocks until ctx is done
}

func (t *flakyTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "flaky", Description: "flaky", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t *flakyTool) Invoke(ctx context.Context, _ json.RawMessage) (agent.Result, error) {
	t.mu.Lock()
	t.calls++
	n := t.calls
	block := t.block
	t.mu.Unlock()
	if block {
		<-ctx.Done()
		return agent.Result{}, ctx.Err()
	}
	if n <= t.failUntil {
		return agent.Result{Output: "transient boom", IsError: true}, nil
	}
	return agent.Result{Output: `{"ok":true}`, IsError: false}, nil
}

func (t *flakyTool) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

func openReliabilityKernel(t *testing.T, tool agent.Tool) *runtime.Kernel {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
		Tools:    map[string]agent.Tool{"flaky": tool},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	k.Edict().SetLevel("flaky", edict.LevelAllow)
	return k
}

// watchNodeEvents collects workflow.node payloads for one workflow.
func watchNodeEvents(t *testing.T, k *runtime.Kernel, name string) func() []map[string]any {
	t.Helper()
	sub, err := k.Bus().Subscribe("workflow."+name, 64)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	var mu sync.Mutex
	var payloads []map[string]any
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range sub.C {
			if ev.Kind == event.KindWorkflowNode {
				var p map[string]any
				_ = json.Unmarshal(ev.Payload, &p)
				mu.Lock()
				payloads = append(payloads, p)
				mu.Unlock()
			}
			if ev.Kind == event.KindWorkflowCompleted || ev.Kind == event.KindWorkflowFailed {
				return
			}
		}
	}()
	return func() []map[string]any {
		sub.Cancel()
		<-done
		mu.Lock()
		defer mu.Unlock()
		return payloads
	}
}

// TestWorkflowRetry_TransientFailureRecovers: retries=2 absorbs two transient
// failures; the journal records attempts=3 and the run completes.
func TestWorkflowRetry_TransientFailureRecovers(t *testing.T) {
	tool := &flakyTool{failUntil: 2}
	k := openReliabilityKernel(t, tool)
	if _, _, err := k.SaveWorkflow("", workflow.Workflow{
		Name: "retry-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "call", Type: workflow.NodeTool, Retries: 2,
				Config: json.RawMessage(`{"tool":"flaky","args":{}}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "call"}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	collect := watchNodeEvents(t, k, "retry-flow")

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "retry-flow", nil)
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if tool.callCount() != 3 {
		t.Fatalf("tool calls = %d, want 3", tool.callCount())
	}
	out, _ := res.Outputs["call"].(map[string]any)
	if out["ok"] != true {
		t.Fatalf("output = %v", res.Outputs["call"])
	}
	var callEv map[string]any
	for _, p := range collect() {
		if p["node"] == "call" {
			callEv = p
		}
	}
	if callEv["attempts"] != float64(3) || callEv["ok"] != true {
		t.Fatalf("call event = %v", callEv)
	}
	if !strings.Contains(callEv["output"].(string), `"ok":true`) {
		t.Fatalf("output snippet = %v", callEv["output"])
	}
}

// TestWorkflowRetry_ExhaustionFailsThenErrorPortRescues: without enough
// retries the node fails; an error branch still rescues AFTER exhaustion.
func TestWorkflowRetry_ExhaustionFailsThenErrorPortRescues(t *testing.T) {
	tool := &flakyTool{failUntil: 99}
	k := openReliabilityKernel(t, tool)
	if _, _, err := k.SaveWorkflow("", workflow.Workflow{
		Name: "rescue-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "call", Type: workflow.NodeTool, Retries: 1,
				Config: json.RawMessage(`{"tool":"flaky","args":{}}`)},
			{ID: "rescue", Type: workflow.NodeTransform,
				Config: json.RawMessage(`{"template":"saved: {{call.output.error}}"}`)},
		},
		Edges: []workflow.Edge{
			{From: "start", To: "call"},
			{From: "call", To: "rescue", Port: "error"},
		},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "rescue-flow", nil)
	if err != nil {
		t.Fatalf("RunWorkflow (rescued) err: %v", err)
	}
	if tool.callCount() != 2 { // 1 + retries(1), error port fires only after exhaustion
		t.Fatalf("tool calls = %d, want 2", tool.callCount())
	}
	rescue, _ := res.Outputs["rescue"].(string)
	if !strings.Contains(rescue, "saved:") || !strings.Contains(rescue, "transient boom") {
		t.Fatalf("rescue output = %q", rescue)
	}
}

// TestWorkflowTimeout_BoundsOneAttempt: a blocking tool with timeout_sec=1
// fails fast with a named timeout error instead of eating the run's 15m.
func TestWorkflowTimeout_BoundsOneAttempt(t *testing.T) {
	tool := &flakyTool{block: true}
	k := openReliabilityKernel(t, tool)
	if _, _, err := k.SaveWorkflow("", workflow.Workflow{
		Name: "timeout-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "call", Type: workflow.NodeTool, TimeoutSec: 1,
				Config: json.RawMessage(`{"tool":"flaky","args":{}}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "call"}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	startAt := time.Now()
	_, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "timeout-flow", nil)
	if err == nil || !strings.Contains(err.Error(), "node timeout after 1s") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(startAt); elapsed > 10*time.Second {
		t.Fatalf("timeout did not bound the attempt: %v", elapsed)
	}
}

// TestWorkflowValidate_ReliabilityBounds: the validator refuses nonsense.
func TestWorkflowValidate_ReliabilityBounds(t *testing.T) {
	base := func(mut func(*workflow.Node)) workflow.Workflow {
		n := workflow.Node{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"flaky"}`)}
		mut(&n)
		return workflow.Workflow{
			Name:  "x",
			Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, n},
			Edges: []workflow.Edge{{From: "start", To: "call"}},
		}
	}
	for _, tc := range []struct {
		mut  func(*workflow.Node)
		want string
	}{
		{func(n *workflow.Node) { n.Retries = 6 }, "retries must be 0..5"},
		{func(n *workflow.Node) { n.TimeoutSec = 601 }, "timeout_sec must be 0..600"},
		{func(n *workflow.Node) { n.RetryDelaySec = 61 }, "retry_delay_sec must be 0..60"},
		{func(n *workflow.Node) {
			n.Type = workflow.NodeTransform
			n.Config = json.RawMessage(`{"template":"t"}`)
			n.Retries = 1
		}, "failable"},
	} {
		if err := workflow.Validate(base(tc.mut)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %q, got %v", tc.want, err)
		}
	}
	// Legal settings pass.
	ok := base(func(n *workflow.Node) { n.Retries = 2; n.RetryDelaySec = 1; n.TimeoutSec = 30 })
	if err := workflow.Validate(ok); err != nil {
		t.Fatalf("legal reliability rejected: %v", err)
	}
}
