// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// wireEchoTool is the tool-node double for the wire round-trip.
type wireEchoTool struct{ last string }

func (t *wireEchoTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t *wireEchoTool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	t.last = string(raw)
	return agent.Result{Output: `{"status":"done"}`}, nil
}

// TestWorkflow_WireRoundTrip drives the full operator pipeline over the
// wire: save (validated) → list → show (full graph) → run with a payload
// (templates resolve, outputs come back) → disable → remove. Exactly what
// `agt workflow` and the console canvas speak.
func TestWorkflow_WireRoundTrip(t *testing.T) {
	tool := &wireEchoTool{}
	k, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: mock.New(mock.FinalText("unused")),
		Tools:    map[string]agent.Tool{"echo": tool},
	})
	k.Edict().SetLevel("echo", edict.LevelAllow)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	graph := map[string]any{
		"name":        "wire-flow",
		"description": "round-trip demo",
		"nodes": []any{
			map[string]any{"id": "start", "type": "trigger"},
			map[string]any{"id": "call", "type": "tool",
				"config": map[string]any{"tool": "echo", "args": map[string]any{"who": "{{trigger.payload.who}}"}}},
			map[string]any{"id": "shape", "type": "transform",
				"config": map[string]any{"template": "status={{call.output.status}}"}},
		},
		"edges": []any{
			map[string]any{"from": "start", "to": "call"},
			map[string]any{"from": "call", "to": "shape"},
		},
	}

	// Save (create).
	res, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": graph})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if created, _ := res["created"].(bool); !created {
		t.Fatalf("created = %v", res["created"])
	}

	// An invalid graph is refused with the validator's reason.
	bad := map[string]any{"name": "bad-flow", "nodes": []any{map[string]any{"id": "a", "type": "tool", "config": map[string]any{"tool": "x"}}}}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": bad}); err == nil ||
		!strings.Contains(err.Error(), "trigger") {
		t.Fatalf("invalid save: %v", err)
	}

	// List is light (no nodes); show carries the full graph.
	res, err = c.Call(ctx, controlplane.CmdWorkflowList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	items, _ := res["workflows"].([]any)
	if len(items) != 1 {
		t.Fatalf("list count = %d", len(items))
	}
	if first, _ := items[0].(map[string]any); first["nodes"] != nil {
		t.Fatal("list leaked the graph body")
	}
	res, err = c.Call(ctx, controlplane.CmdWorkflowShow, map[string]any{"ref": "wire-flow"})
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	wf, _ := res["workflow"].(map[string]any)
	if nodes, _ := wf["nodes"].([]any); len(nodes) != 3 {
		t.Fatalf("show graph = %v", wf["nodes"])
	}

	// Run with a payload: the template reaches the tool; outputs return.
	res, err = c.Call(ctx, controlplane.CmdWorkflowRun, map[string]any{
		"ref": "wire-flow", "payload": map[string]any{"who": "ersin"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(tool.last, `"ersin"`) {
		t.Fatalf("payload did not reach the tool: %q", tool.last)
	}
	outputs, _ := res["outputs"].(map[string]any)
	if outputs["shape"] != "status=done" {
		t.Fatalf("outputs = %v", outputs)
	}
	if executed, _ := res["executed"].([]any); len(executed) != 3 {
		t.Fatalf("executed = %v", res["executed"])
	}

	// Run history (M806): the journal folds into per-run arcs, newest first.
	firstCorr, _ := res["correlation_id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdWorkflowRun, map[string]any{
		"ref": "wire-flow", "payload": map[string]any{"who": "ikinci"},
	}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	res, err = c.Call(ctx, controlplane.CmdWorkflowRuns, map[string]any{"ref": "wire-flow"})
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	runs, _ := res["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("runs count = %d", len(runs))
	}
	newest, _ := runs[0].(map[string]any)
	oldest, _ := runs[1].(map[string]any)
	if newest["status"] != "completed" || oldest["status"] != "completed" {
		t.Fatalf("run statuses = %v / %v", newest["status"], oldest["status"])
	}
	if oldest["correlation_id"] != firstCorr {
		t.Fatalf("ordering: oldest corr = %v want %v", oldest["correlation_id"], firstCorr)
	}
	if evs, _ := newest["node_events"].([]any); len(evs) != 3 {
		t.Fatalf("node_events = %v", newest["node_events"])
	}
	if started, _ := newest["started_ms"].(float64); started <= 0 {
		t.Fatalf("started_ms = %v", newest["started_ms"])
	}
	if _, ok := newest["finished_ms"].(float64); !ok {
		t.Fatal("finished_ms missing on a completed run")
	}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowRuns, map[string]any{"ref": "ghost"}); err == nil {
		t.Fatal("runs for an unknown workflow accepted")
	}

	// Async run (M810): accepted immediately, the arc completes on the bus,
	// a typo'd ref is still a synchronous honest error.
	asyncSub, err := k.Bus().Subscribe("workflow.wire-flow", 16)
	if err != nil {
		t.Fatalf("async subscribe: %v", err)
	}
	res, err = c.Call(ctx, controlplane.CmdWorkflowRun, map[string]any{
		"ref": "wire-flow", "async": true, "payload": map[string]any{"who": "async"},
	})
	if err != nil {
		t.Fatalf("async run: %v", err)
	}
	if res["accepted"] != true || res["async"] != true || res["correlation_id"] == "" {
		t.Fatalf("async result = %v", res)
	}
	asyncDone := false
	asyncDeadline := time.After(10 * time.Second)
	for !asyncDone {
		select {
		case ev := <-asyncSub.C:
			asyncDone = string(ev.Kind) == "workflow.completed"
		case <-asyncDeadline:
			t.Fatal("async run never completed")
		}
	}
	asyncSub.Cancel()
	if _, err := c.Call(ctx, controlplane.CmdWorkflowRun, map[string]any{"ref": "ghost", "async": true}); err == nil {
		t.Fatal("async run of unknown workflow accepted")
	}

	// Webhook fire (M809): right secret accepted (async run completes),
	// wrong secret / disabled / unknown all refuse UNIFORMLY.
	hookGraph := map[string]any{
		"name": "hooked",
		"nodes": []any{
			map[string]any{"id": "start", "type": "trigger",
				"config": map[string]any{"kind": "webhook", "secret": "s3cret-string-12"}},
			map[string]any{"id": "echo_it", "type": "transform",
				"config": map[string]any{"template": "got {{trigger.payload.body.x}}"}},
		},
		"edges": []any{map[string]any{"from": "start", "to": "echo_it"}},
	}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": hookGraph}); err != nil {
		t.Fatalf("save hooked: %v", err)
	}
	// A short secret is refused at validation time.
	short := map[string]any{"name": "shorty", "nodes": []any{map[string]any{
		"id": "start", "type": "trigger", "config": map[string]any{"kind": "webhook", "secret": "tiny"}}}}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSave, map[string]any{"workflow": short}); err == nil ||
		!strings.Contains(err.Error(), "secret") {
		t.Fatalf("short secret: %v", err)
	}
	// Watch for the async run's completion.
	sub, err := k.Bus().Subscribe("workflow.hooked", 16)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()
	res, err = c.Call(ctx, controlplane.CmdWorkflowWebhook, map[string]any{
		"ref": "hooked", "secret": "s3cret-string-12",
		"payload": map[string]any{"kind": "webhook", "body": map[string]any{"x": "dis"}},
	})
	if err != nil {
		t.Fatalf("webhook fire: %v", err)
	}
	if res["accepted"] != true || res["correlation_id"] == "" {
		t.Fatalf("webhook result = %v", res)
	}
	deadline := time.After(10 * time.Second)
	completed := false
	for !completed {
		select {
		case ev := <-sub.C:
			completed = string(ev.Kind) == "workflow.completed"
		case <-deadline:
			t.Fatal("async webhook run never completed")
		}
	}
	// Uniform refusals: wrong secret, unknown name, and disabled.
	for _, args := range []map[string]any{
		{"ref": "hooked", "secret": "wrong-secret-123"},
		{"ref": "ghost", "secret": "s3cret-string-12"},
	} {
		if _, err := c.Call(ctx, controlplane.CmdWorkflowWebhook, args); err == nil ||
			!strings.Contains(err.Error(), "webhook refused") {
			t.Fatalf("args %v: %v", args, err)
		}
	}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSetEnabled, map[string]any{"ref": "hooked", "enabled": false}); err != nil {
		t.Fatalf("disable hooked: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowWebhook, map[string]any{
		"ref": "hooked", "secret": "s3cret-string-12"}); err == nil || !strings.Contains(err.Error(), "webhook refused") {
		t.Fatalf("disabled webhook accepted: %v", err)
	}

	// Disable → remove → ghost refs are honest errors.
	if _, err := c.Call(ctx, controlplane.CmdWorkflowSetEnabled, map[string]any{"ref": "wire-flow", "enabled": false}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	res, err = c.Call(ctx, controlplane.CmdWorkflowRemove, map[string]any{"ref": "wire-flow"})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed, _ := res["removed"].(bool); !removed {
		t.Fatal("remove reported false")
	}
	if _, err := c.Call(ctx, controlplane.CmdWorkflowRun, map[string]any{"ref": "wire-flow"}); err == nil {
		t.Fatal("running a removed workflow accepted")
	}
}
