// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type offloadAuditTool struct{ output string }

func (p offloadAuditTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: "dump", InputSchema: json.RawMessage(`{"type":"object"}`), Capability: toolapi.ToolCapability{Name: "introspect"}}
}
func (p offloadAuditTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: p.output}, nil
}

func TestSharedToolAuditOffloadsLargeOutput(t *testing.T) {
	for _, path := range []string{"direct", "workflow", "canvas", "code"} {
		t.Run(path, func(t *testing.T) {
			full := strings.Repeat("DATA", 5000)
			k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"dump": offloadAuditTool{full}}, ScriptRunner: &stubRunner{out: full}})
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

func TestSharedToolAuditUsesConfiguredThreshold(t *testing.T) {
	for _, threshold := range []int{0, 16, 4096} {
		t.Run(strconv.Itoa(threshold), func(t *testing.T) {
			full := strings.Repeat("x", 4096)
			k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"dump": offloadAuditTool{full}}, ArtifactThreshold: threshold})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			res, err := k.RunTool(context.Background(), "threshold", "call", "dump", json.RawMessage(`{}`))
			if err != nil || res.Output != full {
				t.Fatal("caller output changed")
			}
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Kind != event.KindToolResult || e.CorrelationID != "threshold" {
					return nil
				}
				var p struct {
					RawRef string `json:"raw_ref"`
				}
				if err := json.Unmarshal(e.Payload, &p); err != nil {
					return err
				}
				if (p.RawRef != "") != (threshold == 16) {
					t.Errorf("threshold=%d ref=%q", threshold, p.RawRef)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
