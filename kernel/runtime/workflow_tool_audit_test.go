// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type workflowAuditTool struct {
	name, mode, corr string
	calls            int
	definition       *toolapi.ToolDef
}

func TestRunTool_ApprovalCarriesRunIdentity(t *testing.T) {
	for _, path := range []string{"direct", "canvas"} {
		t.Run(path, func(t *testing.T) {
			var invoked int32
			k, reg := newApprovalKernel(t, mock.New(), &invoked, 5*time.Second)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			ctx = runtime.WithAgentProfile(ctx, roster.Profile{Slug: "ops"})
			done := make(chan error, 1)
			go func() {
				var err error
				if path == "direct" {
					_, err = k.RunTool(ctx, "approval-corr", "call", "approvalprobe", json.RawMessage(`{}`))
				} else {
					w := workflow.Workflow{Name: "approval-flow", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, {ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"approvalprobe","args":{}}`)}}, Edges: []workflow.Edge{{From: "start", To: "call"}}}
					_, err = k.TestWorkflowNode(ctx, "approval-corr", w, "call", nil, nil)
				}
				done <- err
			}()
			req := waitForPending(t, reg)
			if req.CorrelationID != "approval-corr" || req.Actor != "ops" {
				t.Errorf("approval identity: correlation=%q actor=%q", req.CorrelationID, req.Actor)
			}
			if atomic.LoadInt32(&invoked) != 0 {
				t.Error("tool ran before approval")
			}
			if err := reg.Resolve(req.ID, approval.DecisionGrant, "test grant", "operator"); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("approved tool did not finish")
			}
			if atomic.LoadInt32(&invoked) != 1 {
				t.Error("approved tool did not run exactly once")
			}
		})
	}
}

func (p *workflowAuditTool) Definition() toolapi.ToolDef {
	if p.definition != nil {
		return *p.definition
	}
	return toolapi.ToolDef{Name: p.name, InputSchema: json.RawMessage(`{"type":"object"}`), Capability: toolapi.ToolCapability{Name: "introspect"}}
}

func TestRunTool_UsesDeclaredCapabilityAxis(t *testing.T) {
	for _, path := range []string{"direct", "workflow"} {
		t.Run(path, func(t *testing.T) {
			tool := &workflowAuditTool{name: "axis", definition: &toolapi.ToolDef{Name: "axis", InputSchema: json.RawMessage(`{"type":"object"}`),
				Capability: toolapi.ToolCapability{Name: "file.read", Field: "op", ByValue: map[string]string{"write": "file.write"}}}}
			k, err := runtime.Open(runtime.Config{NewToolInvoker: apptools.NewInvoker, BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"axis": tool}, ToolCapabilities: map[string]string{"axis": "file.read"}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			k.Edict().SetLevel(edict.CapFileWrite, edict.LevelDeny)
			if path == "direct" {
				_, err = k.RunTool(context.Background(), "axis-corr", "write", "axis", json.RawMessage(`{"op":"write"}`))
			} else {
				saveFlow(t, k, workflow.Workflow{Name: "axis-flow", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, {ID: "write", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"axis","args":{"op":"write"}}`)}}, Edges: []workflow.Edge{{From: "start", To: "write"}}})
				_, err = k.RunWorkflow(context.Background(), "axis-corr", "axis-flow", nil)
			}
			if err == nil || tool.calls != 0 {
				t.Fatalf("declared write axis was not denied: error=%v calls=%d", err, tool.calls)
			}
			// The default-allowed read axis still works.
			if _, err := k.RunTool(context.Background(), "axis-read", "read", "axis", json.RawMessage(`{"op":"read"}`)); err != nil {
				t.Fatalf("read: %v", err)
			}
		})
	}
}

func TestRunTool_DynamicLookup(t *testing.T) {
	for _, kind := range []string{"forge", "mcp"} {
		for _, denied := range []bool{false, true} {
			name := kind + "/allow"
			if denied {
				name = kind + "/deny"
			}
			t.Run(name, func(t *testing.T) {
				var k *runtime.Kernel
				var toolName string
				var didInvoke func() bool
				if kind == "forge" {
					runner := &stubRunner{}
					k = openForgeKernel(t, mock.New(), runner)
					promoteEcho(t, k, runner)
					runner.out = "done"
					before := runner.calls
					didInvoke = func() bool { return runner.calls > before }
					toolName = "forge_echo"
				} else {
					conn := &fakeMCPConn{tools: []mcp.ToolDef{{Name: "greet"}}, out: "done"}
					k, _ = openMCPKernel(t, mock.New(), conn)
					if _, err := k.AddMCPServer("", mcp.Server{Name: "fake", Command: "unused"}); err != nil {
						t.Fatal(err)
					}
					if _, _, err := k.AttachMCPServer(context.Background(), "", "fake"); err != nil {
						t.Fatal(err)
					}
					didInvoke = func() bool { return conn.lastTool != "" }
					toolName = "mcp_fake_greet"
				}
				ctx := context.Background()
				if denied {
					ctx = runtime.WithAgentProfile(ctx, roster.Profile{Slug: "guarded", ToolDeny: []string{toolName}})
				}
				result, err := k.RunTool(ctx, "dynamic-corr", "dynamic-call", toolName, json.RawMessage(`{}`))
				if denied {
					if err == nil || !strings.Contains(err.Error(), "agent tool denylist") || didInvoke() {
						t.Fatalf("denied dynamic tool: error=%v invoked=%v", err, didInvoke())
					}
				} else if err != nil || result.Output != "done" || !didInvoke() {
					t.Fatalf("dynamic tool: result=%+v error=%v invoked=%v", result, err, didInvoke())
				}
				counts := map[event.Kind]int{}
				if err := k.Journal().Range(func(e *event.Event) error {
					if e.CorrelationID == "dynamic-corr" {
						counts[e.Kind]++
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				wantInvoked := 1
				if denied {
					wantInvoked = 0
				}
				if counts[event.KindPolicyDecision] != 1 || counts[event.KindToolResult] != 1 || counts[event.KindToolInvoked] != wantInvoked {
					t.Fatalf("dynamic audit=%v", counts)
				}
			})
		}
	}
}
func (p *workflowAuditTool) Invoke(ctx context.Context, _ json.RawMessage) (toolapi.Result, error) {
	p.calls++
	p.corr = toolapi.CorrelationFromContext(ctx)
	if p.mode == "retry" && p.calls == 1 {
		return toolapi.Result{}, errors.New("retry me")
	}
	if p.mode == "invoke-error" {
		return toolapi.Result{}, errors.New("invoke failure")
	}
	if p.mode == "error-result" {
		return toolapi.Result{Output: "reported failure", IsError: true}, nil
	}
	return toolapi.Result{Output: `{"ok":true}`}, nil
}

func TestWorkflowToolAudit_RetryHasDistinctCallIDs(t *testing.T) {
	tool := &workflowAuditTool{name: "audit", mode: "retry"}
	k, err := runtime.Open(runtime.Config{NewToolInvoker: apptools.NewInvoker, BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"audit": tool}, ToolCapabilities: map[string]string{"audit": "introspect"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	saveFlow(t, k, workflow.Workflow{Name: "retry-audit", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, {ID: "call", Type: workflow.NodeTool, Retries: 1, Config: json.RawMessage(`{"tool":"audit","args":{}}`)}}, Edges: []workflow.Edge{{From: "start", To: "call"}}})
	if _, err := k.RunWorkflow(context.Background(), "retry-corr", "retry-audit", nil); err != nil {
		t.Fatal(err)
	}
	calls := map[string]map[event.Kind]int{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "retry-corr" || (e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolInvoked && e.Kind != event.KindToolResult) {
			return nil
		}
		var p struct {
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if calls[p.CallID] == nil {
			calls[p.CallID] = map[event.Kind]int{}
		}
		calls[p.CallID][e.Kind]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if tool.calls != 2 || len(calls) != 2 {
		t.Fatalf("invocations=%d audit groups=%v; attempts need distinct identities", tool.calls, calls)
	}
	for id, counts := range calls {
		if counts[event.KindPolicyDecision] != 1 || counts[event.KindToolInvoked] != 1 || counts[event.KindToolResult] != 1 {
			t.Errorf("incomplete attempt %s: %v", id, counts)
		}
	}
}

// Run actual graph, pipeline, HTTP and canvas-node paths; inspect the durable
// tool/policy arc rather than a mocked publisher or a helper's reachability.
func TestWorkflowToolAudit(t *testing.T) {
	for _, path := range []string{"workflow", "canvas", "pipeline", "http"} {
		for _, mode := range []string{"allow", "deny", "invoke-error", "error-result"} {
			t.Run(path+"/"+mode, func(t *testing.T) {
				tool := &workflowAuditTool{name: "audit", mode: mode}
				if path == "http" {
					tool.name = "http"
				}
				k, err := runtime.Open(runtime.Config{NewToolInvoker: apptools.NewInvoker, BaseDir: t.TempDir(), Provider: mock.New(),
					Tools: map[string]toolapi.Tool{tool.name: tool}, ToolCapabilities: map[string]string{tool.name: "introspect"}})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { k.Close() })
				if mode == "deny" {
					k.Edict().SetLevel(edict.CapIntrospect, edict.LevelDeny)
				}
				node := workflow.Node{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"audit","args":{}}`)}
				if path == "pipeline" {
					node.Type = workflow.NodePipeline
					node.Config = json.RawMessage(`{"steps":[{"id":"step","tool":"audit","args":{}}]}`)
				}
				if path == "http" {
					node.Type = workflow.NodeHTTP
					node.Config = json.RawMessage(`{"method":"GET","url":"https://example.invalid/"}`)
				}
				w := workflow.Workflow{Name: "audit-flow", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, node}, Edges: []workflow.Edge{{From: "start", To: "call"}}}
				const corr = "workflow-audit"
				if path == "canvas" {
					_, err = k.TestWorkflowNode(context.Background(), corr, w, "call", nil, nil)
				} else {
					saveFlow(t, k, w)
					_, err = k.RunWorkflow(context.Background(), corr, w.Name, nil)
				}
				if (err != nil) != (mode != "allow") {
					t.Fatalf("error=%v mode=%s", err, mode)
				}
				wantCalls := 1
				if mode == "deny" {
					wantCalls = 0
				}
				if tool.calls != wantCalls {
					t.Fatalf("tool calls=%d want %d", tool.calls, wantCalls)
				}
				if tool.calls > 0 && tool.corr != corr {
					t.Errorf("tool correlation=%q want %q", tool.corr, corr)
				}
				callID := "wf-call"
				if path == "pipeline" {
					callID += "-step"
				}
				var arc []event.Kind
				var auditID string
				if err := k.Journal().Range(func(e *event.Event) error {
					if e.CorrelationID != corr {
						return nil
					}
					if e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolInvoked && e.Kind != event.KindToolResult {
						return nil
					}
					var p struct {
						Tool       string
						CallID     string `json:"call_id"`
						Capability string
						Allow      bool
						Error      bool
						Output     string
					}
					if err := json.Unmarshal(e.Payload, &p); err != nil {
						return err
					}
					if auditID == "" {
						auditID = p.CallID
					}
					if p.Tool != tool.name || p.CallID != auditID || !strings.HasPrefix(p.CallID, callID+"-") || len(p.CallID) != len(callID)+27 {
						t.Errorf("audit identity=%+v", p)
					}
					if e.Kind == event.KindPolicyDecision && (p.Capability != "introspect" || p.Allow != (mode != "deny")) {
						t.Errorf("decision=%+v", p)
					}
					if e.Kind == event.KindToolResult {
						if p.Error != (mode != "allow") || p.Output == "" {
							t.Errorf("terminal result=%+v", p)
						}
						if mode == "deny" && !strings.Contains(p.Output, "denied by policy") {
							t.Errorf("denial output=%q", p.Output)
						}
					}
					arc = append(arc, e.Kind)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				want := []event.Kind{event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult}
				if mode == "deny" {
					want = []event.Kind{event.KindPolicyDecision, event.KindToolResult}
				}
				if len(arc) != len(want) {
					t.Fatalf("audit arc=%v want %v", arc, want)
				}
				for i := range want {
					if arc[i] != want[i] {
						t.Errorf("audit arc=%v want %v", arc, want)
						break
					}
				}
			})
		}
	}
}
