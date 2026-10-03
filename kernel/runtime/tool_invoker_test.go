// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolexec"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"sync/atomic"
	"testing"
)

type observedInvoker struct {
	delegate toolapi.Invoker
	calls    *atomic.Int32
}

func TestToolInvokerFactoryIsBoundPerKernel(t *testing.T) {
	var factories, calls atomic.Int32
	tool := &workflowAuditTool{name: "probe"}
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"probe": tool}, NewToolInvoker: func(deps toolexec.Dependencies) toolapi.Invoker {
		factories.Add(1)
		return &observedInvoker{delegate: toolexec.NewInvoker(deps), calls: &calls}
	}}
	primary, err := runtime.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.Close() })
	cfg.BaseDir = t.TempDir()
	cfg.TenantID = "tenant"
	tenant, err := runtime.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tenant.Close() })
	tenant.Edict().SetLevel(edict.CapIntrospect, edict.LevelDeny)
	if _, err := primary.RunTool(context.Background(), "same-corr", "same-call", "probe", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.RunTool(context.Background(), "same-corr", "same-call", "probe", json.RawMessage(`{}`)); err == nil {
		t.Fatal("tenant policy leaked primary allow")
	}
	if factories.Load() != 2 || calls.Load() != 2 || tool.calls != 1 {
		t.Fatalf("factories=%d port=%d tool=%d", factories.Load(), calls.Load(), tool.calls)
	}
	for name, k := range map[string]*runtime.Kernel{"primary": primary, "tenant": tenant} {
		found := 0
		if err := k.Journal().Range(func(e *event.Event) error {
			if e.Kind != event.KindPolicyDecision || e.CorrelationID != "same-corr" {
				return nil
			}
			found++
			var p struct{ Allow bool }
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return err
			}
			if p.Allow != (name == "primary") {
				t.Errorf("%s policy=%s", name, e.Payload)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Errorf("%s decisions=%d", name, found)
		}
	}
}

func TestOpenRejectsNilToolInvokerAndReleasesStores(t *testing.T) {
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), NewToolInvoker: func(toolexec.Dependencies) toolapi.Invoker { return nil }}
	if k, err := runtime.Open(cfg); k != nil || err == nil || !strings.Contains(err.Error(), "factory returned nil") {
		if k != nil {
			k.Close()
		}
		t.Fatalf("kernel=%v error=%v", k, err)
	}
	cfg.NewToolInvoker = nil
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("standalone reopen: %v", err)
	}
	k.Close()
}

func (s *observedInvoker) Invoke(ctx context.Context, call toolapi.Invocation) (toolapi.Result, error) {
	s.calls.Add(1)
	return s.delegate.Invoke(ctx, call)
}

func TestInjectedToolInvokerPreservesPaths(t *testing.T) {
	for _, path := range []string{"direct", "workflow", "canvas", "code"} {
		t.Run(path, func(t *testing.T) {
			var factoryCalls, portCalls atomic.Int32
			full := strings.Repeat("DATA", 5000)
			k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"dump": offloadAuditTool{full}}, ScriptRunner: &stubRunner{out: full}, NewToolInvoker: func(deps toolexec.Dependencies) toolapi.Invoker {
				factoryCalls.Add(1)
				return &observedInvoker{delegate: toolexec.NewInvoker(deps), calls: &portCalls}
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			ctx := context.Background()
			const corr = "offload-run"
			if path == "direct" {
				res, err := k.RunTool(ctx, corr, "call", "dump", json.RawMessage(`{}`))
				if err != nil || res.Output != full {
					t.Fatalf("caller result bytes=%d error=%v", len(res.Output), err)
				}
			} else {
				node := workflow.Node{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"dump","args":{}}`)}
				if path == "code" {
					node.Type = workflow.NodeCode
					node.Config = json.RawMessage(`{"language":"python","code":"print(42)"}`)
				}
				w := workflow.Workflow{Name: "offload-flow", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, node}, Edges: []workflow.Edge{{From: "start", To: "call"}}}
				if path == "canvas" {
					res, err := k.TestWorkflowNode(ctx, corr, w, "call", nil, nil)
					if err != nil || res.Output != full {
						t.Fatalf("canvas output=%v error=%v", res.Output, err)
					}
				} else {
					saveFlow(t, k, w)
					res, err := k.RunWorkflow(ctx, corr, w.Name, nil)
					if err != nil || res.Outputs["call"] != full {
						t.Fatalf("workflow error=%v", err)
					}
				}
			}
			if factoryCalls.Load() != 1 || portCalls.Load() != 1 {
				t.Fatalf("factory=%d port=%d", factoryCalls.Load(), portCalls.Load())
			}
			found := 0
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Kind != event.KindToolResult || e.CorrelationID != corr {
					return nil
				}
				found++
				var p struct {
					Output      string
					RawRef      string `json:"raw_ref"`
					OutputBytes int    `json:"output_bytes"`
				}
				if err := json.Unmarshal(e.Payload, &p); err != nil {
					return err
				}
				if p.RawRef == "" || p.OutputBytes != len(full) || len(p.Output) >= len(full) {
					t.Errorf("audit bytes=%d ref=%q full_bytes=%d", len(p.Output), p.RawRef, p.OutputBytes)
					return nil
				}
				stored, err := k.Artifacts().Get(p.RawRef)
				if err != nil || string(stored) != full {
					t.Errorf("artifact bytes=%d error=%v", len(stored), err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if found != 1 {
				t.Errorf("terminal records=%d", found)
			}
		})
	}
}
