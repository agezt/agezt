// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestSharedToolPolicyAuditRetainsDetails(t *testing.T) {
	for _, path := range []string{"direct", "workflow", "canvas", "code"} {
		for _, deny := range []bool{false, true} {
			name := path + "/allow"
			if deny {
				name = path + "/deny"
			}
			t.Run(name, func(t *testing.T) {
				tool := &workflowAuditTool{name: "policyprobe", definition: &toolapi.ToolDef{Name: "policyprobe", InputSchema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}}}`), Capability: toolapi.ToolCapability{Name: "introspect"}, Effect: toolapi.ToolEffect{Class: toolapi.EffectReadOnly, Confidence: 0.75, AffectedResources: []string{"resource:one"}}}}
				runner := &stubRunner{out: "ok"}
				k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"policyprobe": tool}, ScriptRunner: runner, PromptInjectionGuard: runtime.PromptInjectionOff})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { k.Close() })
				if deny {
					cap := edict.CapIntrospect
					if path == "code" {
						cap = edict.CapCodeExec
					}
					k.Edict().SetLevel(cap, edict.LevelDeny)
				}
				ctx := agent.WithUntrustedObservationTaint(context.Background(), agent.UntrustedObservationTaint{Sources: []string{"web:one"}, DirectiveLike: true, Matches: []string{"directive:one"}})
				const corr = "policy-details"
				if path == "direct" {
					_, err = k.RunTool(ctx, corr, "call", "policyprobe", json.RawMessage(`{"target":"latest release"}`))
				} else {
					node := workflow.Node{ID: "call", Type: workflow.NodeTool, Config: json.RawMessage(`{"tool":"policyprobe","args":{"target":"latest release"}}`)}
					if path == "code" {
						node.Type = workflow.NodeCode
						node.Config = json.RawMessage(`{"language":"python","code":"print(42)"}`)
					}
					w := workflow.Workflow{Name: "policy-flow", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, node}, Edges: []workflow.Edge{{From: "start", To: "call"}}}
					if path == "canvas" {
						_, err = k.TestWorkflowNode(ctx, corr, w, "call", nil, nil)
					} else {
						saveFlow(t, k, w)
						_, err = k.RunWorkflow(ctx, corr, w.Name, nil)
					}
				}
				if (err != nil) != deny {
					t.Fatalf("deny=%v error=%v", deny, err)
				}
				if deny && tool.calls != 0 {
					t.Fatal("denied tool executed")
				}
				found := 0
				if err := k.Journal().Range(func(e *event.Event) error {
					if e.Kind != event.KindPolicyDecision || e.CorrelationID != corr {
						return nil
					}
					found++
					var p map[string]any
					if err := json.Unmarshal(e.Payload, &p); err != nil {
						return err
					}
					if len(p) != 23 {
						t.Errorf("policy fields=%d want 23; payload=%s", len(p), e.Payload)
					}
					if p["allow"] != !deny || p["call_id"] == "" {
						t.Errorf("decision/identity=%s", e.Payload)
					}
					for _, key := range []string{"epistemic_action", "epistemic_reason", "schema_hash", "input_shape"} {
						if value, ok := p[key].(string); !ok || value == "" {
							t.Errorf("missing %s: %s", key, e.Payload)
						}
					}
					if p["untrusted_observation"] != true || p["directive_like"] != true {
						t.Errorf("taint=%s", e.Payload)
					}
					for key, want := range map[string]string{"observation_sources": "web:one", "directive_matches": "directive:one"} {
						if list, ok := p[key].([]any); !ok || len(list) != 1 || list[0] != want {
							t.Errorf("%s=%v", key, p[key])
						}
					}
					if path != "code" {
						if list, ok := p["affected_resources"].([]any); !ok || len(list) != 1 || list[0] != "resource:one" {
							t.Errorf("resources=%v", p["affected_resources"])
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if found != 1 {
					t.Errorf("policy records=%d", found)
				}
			})
		}
	}
}
