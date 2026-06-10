// SPDX-License-Identifier: MIT

package runtime_test

// M800 node-library tests: map/filter/switch/merge data plumbing, the error
// port, the code node (sandbox runner), the http node's tool bridging, the
// approval gate, and the subworkflow depth cap.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRunWorkflow_MapFilterSwitchMerge: the pure-data nodes compose — filter
// keeps high scores, map shapes them, switch routes by a payload field, and
// a merge in "all" mode waits for BOTH branches before combining.
func TestRunWorkflow_MapFilterSwitchMerge(t *testing.T) {
	k := openWorkflowKernel(t, mock.New(), &echoTool{})

	saveFlow(t, k, workflow.Workflow{
		Name: "data-pipes",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "keep", Type: workflow.NodeFilter, Config: json.RawMessage(
				`{"items":"{{trigger.payload.items}}","left":"{{item.score}}","op":"gt","right":"50"}`)},
			{ID: "shape", Type: workflow.NodeMap, Config: json.RawMessage(
				`{"items":"{{keep.output}}","template":"{{index}}:{{item.name}}!"}`)},
			{ID: "route", Type: workflow.NodeSwitch, Config: json.RawMessage(
				`{"value":"{{trigger.payload.mode}}","cases":[{"equals":"loud","port":"loud"},{"equals":"quiet","port":"quiet"}]}`)},
			{ID: "yell", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"YELLING"}`)},
			{ID: "hush", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"hushing"}`)},
			{ID: "join", Type: workflow.NodeMerge, Config: json.RawMessage(`{"mode":"all"}`)},
		},
		Edges: []workflow.Edge{
			{From: "start", To: "keep"},
			{From: "keep", To: "shape"},
			{From: "start", To: "route"},
			{From: "route", To: "yell", Port: "loud"},
			{From: "route", To: "hush", Port: "quiet"},
			{From: "shape", To: "join"},
			{From: "yell", To: "join"},
		},
	})

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "data-pipes", map[string]any{
		"mode": "loud",
		"items": []any{
			map[string]any{"name": "a", "score": 80},
			map[string]any{"name": "b", "score": 20},
			map[string]any{"name": "c", "score": 55},
		},
	})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if kept, _ := res.Outputs["keep"].([]any); len(kept) != 2 {
		t.Fatalf("filter kept %v", res.Outputs["keep"])
	}
	shaped, _ := res.Outputs["shape"].([]any)
	if len(shaped) != 2 || shaped[0] != "0:a!" || shaped[1] != "1:c!" {
		t.Fatalf("map output = %v", shaped)
	}
	if _, ran := res.Outputs["hush"]; ran {
		t.Fatal("switch ran the unmatched branch")
	}
	// The merge waited for BOTH inputs (shape and yell) and collected them.
	joined, _ := res.Outputs["join"].([]any)
	if len(joined) != 2 {
		t.Fatalf("merge-all collected %v", joined)
	}
	// Edge order: shape→join first, then yell→join.
	if _, isList := joined[0].([]any); !isList || joined[1] != "YELLING" {
		t.Fatalf("merge inputs out of order: %v", joined)
	}
}

// TestRunWorkflow_ErrorPortBranches: a failing tool with an "error" edge
// keeps the run alive — the message lands in {{call.output.error}} and the
// error branch runs while the default branch never does.
func TestRunWorkflow_ErrorPortBranches(t *testing.T) {
	tool := &echoTool{out: "kaput", isErr: true}
	k := openWorkflowKernel(t, mock.New(), tool)
	k.Edict().SetLevel("echo", edict.LevelAllow)

	saveFlow(t, k, workflow.Workflow{
		Name: "rescued",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"echo","args":{}}`)},
			{ID: "happy", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"all good"}`)},
			{ID: "rescue", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"rescued: {{call.output.error}}"}`)},
		},
		Edges: []workflow.Edge{
			{From: "start", To: "call"},
			{From: "call", To: "happy"},
			{From: "call", To: "rescue", Port: "error"},
		},
	})

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "rescued", nil)
	if err != nil {
		t.Fatalf("error port did not rescue the run: %v", err)
	}
	rescue, _ := res.Outputs["rescue"].(string)
	if !strings.Contains(rescue, "rescued: ") || !strings.Contains(rescue, "kaput") {
		t.Fatalf("rescue output = %q", rescue)
	}
	if _, ran := res.Outputs["happy"]; ran {
		t.Fatal("default branch ran after a failure")
	}
}

// TestRunWorkflow_CodeNode: the code node rides the M794 sandbox runner —
// interpolated input in, stdout out, structured output stays structured.
func TestRunWorkflow_CodeNode(t *testing.T) {
	runner := &stubRunner{out: `{"doubled": 84}`}
	prov := mock.New()
	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     prov,
		ScriptRunner: runner,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	saveFlow(t, k, workflow.Workflow{
		Name: "compute",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "calc", Type: workflow.NodeCode, Config: json.RawMessage(
				`{"language":"python","code":"print(42*2)","input":"{\"n\": {{trigger.payload.n}}}"}`)},
			{ID: "read", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"result={{calc.output.doubled}}"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "calc"}, {From: "calc", To: "read"}},
	})

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "compute", map[string]any{"n": 42})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if runner.lang != "python" || !strings.Contains(runner.input, `"n": 42`) {
		t.Fatalf("runner got %q / %q", runner.lang, runner.input)
	}
	if res.Outputs["read"] != "result=84" {
		t.Fatalf("structured code output lost: %v", res.Outputs["read"])
	}
}

// TestRunWorkflow_HTTPNodeBridgesTool: the http node maps its config onto
// the registered http tool's input and passes the CapHTTPGet policy axis.
func TestRunWorkflow_HTTPNodeBridgesTool(t *testing.T) {
	tool := &echoTool{out: `{"ok":true}`}
	prov := mock.New()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools:    map[string]agent.Tool{"http": tool}, // stands in for the real guarded tool
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	saveFlow(t, k, workflow.Workflow{
		Name: "fetcher",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "get", Type: workflow.NodeHTTP, Config: json.RawMessage(
				`{"method":"get","url":"https://api.example.com/{{trigger.payload.path}}","headers":{"Accept":"application/json"}}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "get"}},
	})

	if _, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "fetcher", map[string]any{"path": "users/1"}); err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if len(tool.inputs) != 1 {
		t.Fatalf("http tool calls = %d", len(tool.inputs))
	}
	got := tool.inputs[0]
	if !strings.Contains(got, `"method":"GET"`) || !strings.Contains(got, "https://api.example.com/users/1") {
		t.Fatalf("http args = %s", got)
	}
}

// TestRunWorkflow_ApprovalGate: the approval node blocks on the HITL
// registry; a grant lets the run continue, a deny fails the node.
func TestRunWorkflow_ApprovalGate(t *testing.T) {
	k := openWorkflowKernel(t, mock.New(), &echoTool{})

	saveFlow(t, k, workflow.Workflow{
		Name: "gated-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "ask", Type: workflow.NodeApproval, Config: json.RawMessage(
				`{"description":"Proceed with {{trigger.payload.what}}?"}`)},
			{ID: "go", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"proceeded ({{ask.output}})"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "ask"}, {From: "ask", To: "go"}},
	})

	// Resolve the pending approval as soon as it appears.
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			for _, req := range k.Approvals().Pending() {
				if req.ToolName == "workflow.approval" {
					_ = k.Approvals().Resolve(req.ID, approval.DecisionGrant, "looks fine", "tester")
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "gated-flow", map[string]any{"what": "the deploy"})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	out, _ := res.Outputs["go"].(string)
	if !strings.Contains(out, "granted by tester") {
		t.Fatalf("approval output = %q", out)
	}
}

// TestRunWorkflow_SubworkflowAndDepthCap: a parent runs a stored child and
// reads its outputs; self-recursive nesting is refused at the cap.
func TestRunWorkflow_SubworkflowAndDepthCap(t *testing.T) {
	k := openWorkflowKernel(t, mock.New(), &echoTool{})

	saveFlow(t, k, workflow.Workflow{
		Name: "child",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "greet", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"hi {{trigger.payload.name}}"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "greet"}},
	})
	saveFlow(t, k, workflow.Workflow{
		Name: "parent",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "sub", Type: workflow.NodeSubflow, Config: json.RawMessage(
				`{"workflow":"child","payload":"{\"name\":\"{{trigger.payload.name}}\"}"}`)},
			{ID: "read", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"child said: {{sub.output.outputs.greet}}"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "sub"}, {From: "sub", To: "read"}},
	})

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "parent", map[string]any{"name": "ersin"})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if res.Outputs["read"] != "child said: hi ersin" {
		t.Fatalf("subworkflow output = %v", res.Outputs["read"])
	}

	// A workflow that calls ITSELF hits the depth cap instead of recursing
	// forever.
	saveFlow(t, k, workflow.Workflow{
		Name: "ouroboros",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "again", Type: workflow.NodeSubflow, Config: json.RawMessage(`{"workflow":"ouroboros"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "again"}},
	})
	if _, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "ouroboros", nil); err == nil ||
		!strings.Contains(err.Error(), "nesting") {
		t.Fatalf("depth cap missing: %v", err)
	}
}

// Silence the unused-import linter when toolforge types are only used by the
// shared stubRunner from scripttool_test.go.
var _ toolforge.Runner = (*stubRunner)(nil)
