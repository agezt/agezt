// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// echoTool records its input and returns a canned output — the workflow
// engine's tool-node test double.
type echoTool struct {
	mu     sync.Mutex
	inputs []string
	out    string
	isErr  bool
}

func (t *echoTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t *echoTool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	t.mu.Lock()
	t.inputs = append(t.inputs, string(raw))
	t.mu.Unlock()
	return agent.Result{Output: t.out, IsError: t.isErr}, nil
}

func openWorkflowKernel(t *testing.T, prov agent.Provider, tool *echoTool) *runtime.Kernel {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools:    map[string]agent.Tool{"echo": tool},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

func saveFlow(t *testing.T, k *runtime.Kernel, w workflow.Workflow) {
	t.Helper()
	if _, _, err := k.SaveWorkflow("", w); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
}

func TestRunWorkflow_LLMNodeUsesContextModelWhenUnset(t *testing.T) {
	prov := mock.New(mock.FinalText("done"))
	var llmReq agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { llmReq = r }
	k := openWorkflowKernel(t, prov, &echoTool{})
	saveFlow(t, k, workflow.Workflow{
		Name: "scheduled-model-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "brief", Type: workflow.NodeLLM, Config: json.RawMessage(`{"prompt":"run"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "brief"}},
	})

	_, err := k.RunWorkflow(runtime.WithModel(context.Background(), "schedule-model"), k.NewCorrelation(), "scheduled-model-flow", nil)
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if llmReq.Model != "schedule-model" {
		t.Fatalf("llm model = %q, want schedule-model", llmReq.Model)
	}
}

func TestRunWorkflow_LLMNodeExplicitModelWinsOverContextModel(t *testing.T) {
	prov := mock.New(mock.FinalText("done"))
	var llmReq agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { llmReq = r }
	k := openWorkflowKernel(t, prov, &echoTool{})
	saveFlow(t, k, workflow.Workflow{
		Name: "node-model-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "brief", Type: workflow.NodeLLM, Config: json.RawMessage(`{"prompt":"run","model":"node-model"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "brief"}},
	})

	_, err := k.RunWorkflow(runtime.WithModel(context.Background(), "schedule-model"), k.NewCorrelation(), "node-model-flow", nil)
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if llmReq.Model != "node-model" {
		t.Fatalf("llm model = %q, want node-model", llmReq.Model)
	}
}

func TestRunWorkflow_JournalsRunnerProvenance(t *testing.T) {
	k := openWorkflowKernel(t, mock.New(mock.FinalText("unused")), &echoTool{})
	saveFlow(t, k, workflow.Workflow{
		Name: "agent-owned-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "shape", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"ok"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "shape"}},
	})
	ctx := runtime.WithAgentIdent(context.Background(), "ops", 0)
	ctx = runtime.WithWakeContext(ctx, runtime.WakeContext{Source: "schedule", ScheduleID: "sch-1", ParentCorrelation: "corr-parent"})
	corr := k.NewCorrelation()
	if _, err := k.RunWorkflow(ctx, corr, "agent-owned-flow", nil); err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	var started map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "workflow.agent-owned-flow" || e.Kind != event.KindWorkflowStarted || e.CorrelationID != corr {
			return nil
		}
		_ = json.Unmarshal(e.Payload, &started)
		return nil
	})
	if started["source"] != "schedule" || started["runner"] != "agent" || started["agent"] != "ops" ||
		started["schedule_id"] != "sch-1" || started["parent_correlation_id"] != "corr-parent" {
		t.Fatalf("workflow start provenance = %+v", started)
	}
}

// TestRunWorkflow_LinearDataFlow is the engine e2e: trigger payload flows
// through templates into a tool node, the tool's JSON output is parsed so a
// transform can reach INTO it, and an llm node sees the transformed value in
// its prompt — with the journal carrying the whole started→node…→completed arc.
func TestRunWorkflow_LinearDataFlow(t *testing.T) {
	prov := mock.New(mock.FinalText("looks sunny"))
	var llmReq agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { llmReq = r }
	tool := &echoTool{out: `{"temp": 28, "sky": "clear"}`}
	k := openWorkflowKernel(t, prov, tool)
	k.Edict().SetLevel("echo", edict.LevelAllow) // the policy-gate test covers the deny path

	saveFlow(t, k, workflow.Workflow{
		Name: "weather-brief",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "fetch", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"echo","args":{"city":"{{trigger.payload.city}}"}}`)},
			{ID: "shape", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"temp is {{fetch.output.temp}} and sky {{fetch.output.sky}}"}`)},
			{ID: "brief", Type: workflow.NodeLLM, Config: json.RawMessage(`{"prompt":"one line: {{shape.output}}","system":"you brief weather"}`)},
		},
		Edges: []workflow.Edge{
			{From: "start", To: "fetch"},
			{From: "fetch", To: "shape"},
			{From: "shape", To: "brief"},
		},
	})

	// Watch the journal arc.
	var mu sync.Mutex
	kinds := []string{}
	sub, err := k.Bus().Subscribe("workflow.weather-brief", 32)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range sub.C {
			mu.Lock()
			kinds = append(kinds, string(ev.Kind))
			done := ev.Kind == event.KindWorkflowCompleted || ev.Kind == event.KindWorkflowFailed
			mu.Unlock()
			if done {
				return
			}
		}
	}()

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "weather-brief",
		map[string]any{"city": "izmir"})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	<-done

	if got := strings.Join(res.Executed, ","); got != "start,fetch,shape,brief" {
		t.Fatalf("executed = %s", got)
	}
	// Trigger payload reached the tool through the template.
	if len(tool.inputs) != 1 || !strings.Contains(tool.inputs[0], `"izmir"`) {
		t.Fatalf("tool args = %v", tool.inputs)
	}
	// The tool's JSON output was parsed and reached the llm via the transform.
	if !strings.Contains(llmReq.Messages[0].Content, "temp is 28 and sky clear") {
		t.Fatalf("llm prompt = %q", llmReq.Messages[0].Content)
	}
	if llmReq.System != "you brief weather" || llmReq.TaskType != "workflow" {
		t.Fatalf("llm system/tasktype = %q/%q", llmReq.System, llmReq.TaskType)
	}
	if res.Outputs["brief"] != "looks sunny" {
		t.Fatalf("llm output = %v", res.Outputs["brief"])
	}
	mu.Lock()
	arc := strings.Join(kinds, ",")
	mu.Unlock()
	if !strings.HasPrefix(arc, "workflow.started") || !strings.HasSuffix(arc, "workflow.completed") ||
		strings.Count(arc, "workflow.node") != 4 {
		t.Fatalf("journal arc = %s", arc)
	}
}

func TestRunWorkflow_PipelineNodeChainsTypedToolSteps(t *testing.T) {
	tool := &echoTool{out: `{"temp":28,"sky":"clear"}`}
	k := openWorkflowKernel(t, mock.New(), tool)
	k.Edict().SetLevel("echo", edict.LevelAllow)

	saveFlow(t, k, workflow.Workflow{
		Name: "typed-pipeline",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "pipe", Type: workflow.NodePipeline, Config: json.RawMessage(`{
				"steps":[
					{
						"id":"fetch",
						"tool":"echo",
						"args":{"city":"{{trigger.payload.city}}","kind":"fetch"},
						"output_schema":{"type":"object","properties":{"temp":{"type":"number"},"sky":{"type":"string"}},"required":["temp"]}
					},
					{
						"id":"shape",
						"tool":"echo",
						"args":{"temp":"{{steps.fetch.output.temp}}","kind":"shape"}
					}
				]
			}`)},
			{ID: "read", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"last={{pipe.output.last.temp}} fetch={{pipe.output.steps.fetch.output.sky}}"}`)},
		},
		Edges: []workflow.Edge{
			{From: "start", To: "pipe"},
			{From: "pipe", To: "read"},
		},
	})

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "typed-pipeline", map[string]any{"city": "izmir"})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if got := res.Outputs["read"]; got != "last=28 fetch=clear" {
		t.Fatalf("pipeline downstream output = %v", got)
	}
	tool.mu.Lock()
	inputs := append([]string(nil), tool.inputs...)
	tool.mu.Unlock()
	if len(inputs) != 2 {
		t.Fatalf("tool inputs = %v, want two pipeline steps", inputs)
	}
	if !strings.Contains(inputs[0], `"izmir"`) || !strings.Contains(inputs[0], `"fetch"`) {
		t.Fatalf("first step args = %s", inputs[0])
	}
	if !strings.Contains(inputs[1], `"temp":"28"`) || !strings.Contains(inputs[1], `"shape"`) {
		t.Fatalf("second step args = %s", inputs[1])
	}
}

func TestRunWorkflow_PipelineOutputSchemaFailure(t *testing.T) {
	tool := &echoTool{out: `{"temp":"hot"}`}
	k := openWorkflowKernel(t, mock.New(), tool)
	k.Edict().SetLevel("echo", edict.LevelAllow)

	saveFlow(t, k, workflow.Workflow{
		Name: "bad-pipeline",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "pipe", Type: workflow.NodePipeline, Config: json.RawMessage(`{
				"steps":[{"id":"fetch","tool":"echo","args":{},"output_schema":{"type":"object","properties":{"temp":{"type":"number"}},"required":["temp"]}}]
			}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "pipe"}},
	})

	_, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "bad-pipeline", nil)
	if err == nil || !strings.Contains(err.Error(), "output rejected by schema") {
		t.Fatalf("pipeline schema failure = %v", err)
	}
}

// TestRunWorkflow_ConditionBranching: only the matching port's branch runs.
func TestRunWorkflow_ConditionBranching(t *testing.T) {
	tool := &echoTool{out: "ran"}
	k := openWorkflowKernel(t, mock.New(), tool)

	saveFlow(t, k, workflow.Workflow{
		Name: "branchy",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "check", Type: workflow.NodeCondition, Config: json.RawMessage(`{"left":"{{trigger.payload.n}}","op":"gt","right":"10"}`)},
			{ID: "big", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"BIG"}`)},
			{ID: "small", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"SMALL"}`)},
		},
		Edges: []workflow.Edge{
			{From: "start", To: "check"},
			{From: "check", To: "big", Port: "true"},
			{From: "check", To: "small", Port: "false"},
		},
	})

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "branchy", map[string]any{"n": 42})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if res.Outputs["big"] != "BIG" {
		t.Fatalf("true branch did not run: %v", res.Outputs)
	}
	if _, ran := res.Outputs["small"]; ran {
		t.Fatal("false branch ran on a true condition")
	}

	res, err = k.RunWorkflow(context.Background(), k.NewCorrelation(), "branchy", map[string]any{"n": 3})
	if err != nil {
		t.Fatalf("RunWorkflow(small): %v", err)
	}
	if res.Outputs["small"] != "SMALL" {
		t.Fatalf("false branch did not run: %v", res.Outputs)
	}
	if _, ran := res.Outputs["big"]; ran {
		t.Fatal("true branch ran on a false condition")
	}
}

// TestRunWorkflow_PolicyGatesToolNodes: a tool whose capability is unknown to
// the policy engine is default-denied — exactly like an agent-loop call.
func TestRunWorkflow_PolicyGatesToolNodes(t *testing.T) {
	tool := &echoTool{out: "never"}
	k := openWorkflowKernel(t, mock.New(), tool)

	saveFlow(t, k, workflow.Workflow{
		Name: "gated",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			// "echo" is not a known capability → Capability("echo") → default-deny.
			{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"echo","args":{}}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "call"}},
	})

	_, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "gated", nil)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("policy did not gate the tool node: %v", err)
	}
	if len(tool.inputs) != 0 {
		t.Fatal("denied tool was still invoked")
	}
}

func TestRunWorkflow_AgentToolDenyGatesToolNodes(t *testing.T) {
	tool := &echoTool{out: "never"}
	k := openWorkflowKernel(t, mock.New(), tool)
	k.Edict().SetLevel("echo", edict.LevelAllow)

	saveFlow(t, k, workflow.Workflow{
		Name: "agent-denied",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"echo","args":{}}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "call"}},
	})

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug:     "guarded",
		ToolDeny: []string{"echo"},
	})
	_, err := k.RunWorkflow(ctx, k.NewCorrelation(), "agent-denied", nil)
	if err == nil || !strings.Contains(err.Error(), "agent tool denylist") {
		t.Fatalf("agent tool deny did not gate workflow tool node: %v", err)
	}
	if len(tool.inputs) != 0 {
		t.Fatal("agent-denied workflow tool was still invoked")
	}
}

// TestRunWorkflow_ToolFailureFailsRun: an IsError tool result fails the run
// and journals workflow.failed (error branching is M800).
func TestRunWorkflow_ToolFailureFailsRun(t *testing.T) {
	tool := &echoTool{out: "boom", isErr: true}
	k := openWorkflowKernel(t, mock.New(), tool)
	// Allow the echo capability so we hit the FAILURE path, not the policy.
	k.Edict().SetLevel("echo", edict.LevelAllow)

	saveFlow(t, k, workflow.Workflow{
		Name: "failing",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"echo","args":{}}`)},
			{ID: "after", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"unreachable"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "call"}, {From: "call", To: "after"}},
	})

	res, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "failing", nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("tool failure not surfaced: %v", err)
	}
	if _, ran := res.Outputs["after"]; ran {
		t.Fatal("downstream node ran after a failure")
	}
}
