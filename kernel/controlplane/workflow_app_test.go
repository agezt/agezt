// SPDX-License-Identifier: MIT
package controlplane

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func workflowAppFixture(t *testing.T) (*runtime.Kernel, *Server, workflow.Workflow, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	provider := mock.New(mock.FinalText(`{"name":"designed","nodes":[{"id":"start","type":"trigger"}]}`), mock.FinalText("owned"))
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	graph, _, err := k.SaveWorkflow("", workflow.Workflow{Name: "owned", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, {ID: "work", Type: workflow.NodeLLM, Config: json.RawMessage(`{"prompt":"owned"}`)}}, Edges: []workflow.Edge{{From: "start", To: "work"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, graph, provider
}
func TestWorkflowNativeMetadataComesFromThirteenTypedAppSpecs(t *testing.T) {
	want := map[string]bool{CmdWorkflowList: true, CmdWorkflowShow: true, CmdWorkflowRuns: true, CmdWorkflowTemplates: true, CmdWorkflowSave: false, CmdWorkflowRestore: false, CmdWorkflowRemove: false, CmdWorkflowSetEnabled: false, CmdWorkflowRun: false, CmdWorkflowDraft: false, CmdWorkflowRefine: false, CmdWorkflowWebhook: false, CmdWorkflowTestNode: false}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "workflow_") {
			continue
		}
		read, known := want[spec.Name]
		wire, exists := commandRegistry[spec.Name]
		if !known || seen[spec.Name] || !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 13 || len(workflowOperations) != 13 {
		t.Fatal(seen, len(workflowOperations))
	}
}
func TestWorkflowAppRequiresActualAuditBeforeAllNineMutations(t *testing.T) {
	for _, cmd := range []string{CmdWorkflowSave, CmdWorkflowRestore, CmdWorkflowRemove, CmdWorkflowSetEnabled, CmdWorkflowRun, CmdWorkflowDraft, CmdWorkflowRefine, CmdWorkflowWebhook, CmdWorkflowTestNode} {
		t.Run(cmd, func(t *testing.T) {
			k, s, graph, provider := workflowAppFixture(t)
			before := k.Workflows().List()
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			out := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"ref": graph.ID, "workflow": graph, "enabled": false, "async": true, "description": "design owned graph", "instruction": "revise owned graph", "node": "work", "payload": "owned", "secret": "wrong"}})
			if len(out) != 1 || out[0].Type != RespError || !strings.Contains(out[0].Error, "journal") || !reflect.DeepEqual(before, k.Workflows().List()) || provider.CallCount() != 0 {
				t.Fatal(cmd, out, before, k.Workflows().List(), provider.CallCount())
			}
		})
	}
}
func TestWorkflowAppReadAndMutationAuditPolicyJoinsLifecycleIdentity(t *testing.T) {
	k, s, graph, provider := workflowAppFixture(t)
	for _, cmd := range []string{CmdWorkflowList, CmdWorkflowShow, CmdWorkflowRuns, CmdWorkflowTemplates} {
		out := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"ref": graph.ID, "with_runs": "YES", "unused": true}})
		if len(out) != 1 || out[0].Type != RespResult {
			t.Fatal(cmd, out)
		}
	}
	invoked := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked && strings.HasPrefix(e.Subject, "op.workflow_") {
			invoked++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 0 || provider.CallCount() != 0 {
		t.Fatal(invoked, provider.CallCount())
	}
	out := callAppHost(t, s, Request{ID: "enable", Cmd: CmdWorkflowSetEnabled, Token: "primary", Args: map[string]any{"ref": graph.ID, "enabled": "TRUE", "correlation_id": "spoof"}})
	if len(out) != 1 || out[0].Type != RespResult {
		t.Fatal(out)
	}
	var begin, end, domain []*event.Event
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op.workflow_set_enabled" {
			if e.Kind == event.KindOpInvoked {
				begin = append(begin, e)
			}
			if e.Kind == event.KindOpCompleted {
				end = append(end, e)
			}
		}
		if e.Kind == event.KindWorkflowUpdated {
			domain = append(domain, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(begin) != 1 || len(end) != 1 || len(domain) != 1 || begin[0].CorrelationID == "" || begin[0].CorrelationID == "spoof" || end[0].CorrelationID != begin[0].CorrelationID || domain[0].CorrelationID != begin[0].CorrelationID {
		t.Fatal(begin, end, domain)
	}
}
func TestWorkflowAppPresenceAdmissionOrderAndCompatForms(t *testing.T) {
	k, s, graph, _ := workflowAppFixture(t)
	for _, test := range []struct {
		value any
		want  bool
	}{{true, true}, {false, false}, {"TRUE", true}, {"1", true}, {" true ", false}, {float64(1), false}, {nil, false}} {
		out := callAppHost(t, s, Request{ID: "enable", Cmd: CmdWorkflowSetEnabled, Token: "primary", Args: map[string]any{"ref": graph.ID, "enabled": test.value}})
		if len(out) != 1 || out[0].Type != RespResult || out[0].Result["workflow"].(map[string]any)["enabled"] != test.want {
			t.Fatal(test, out)
		}
	}
	cases := []struct {
		cmd       string
		args      map[string]any
		errorText string
	}{{CmdWorkflowRestore, map[string]any{"workflow": nil, "reason": true}, "args.reason must be a string"}, {CmdWorkflowRefine, map[string]any{"instruction": " ", "workflow": true}, "args.instruction required"}, {CmdWorkflowRefine, map[string]any{"instruction": "revise", "workflow": "bad", "ref": true}, "args.workflow:"}, {CmdWorkflowTestNode, map[string]any{"workflow": "bad", "node": true, "data": nil}, "args.workflow:"}, {CmdWorkflowTestNode, map[string]any{"workflow": graph, "node": "work", "data": nil}, "args.data must be an object"}, {CmdWorkflowWebhook, map[string]any{"ref": true, "secret": true}, "webhook refused"}}
	for _, test := range cases {
		out := callAppHost(t, s, Request{ID: test.cmd, Cmd: test.cmd, Token: "primary", Args: test.args})
		if len(out) != 1 || out[0].Type != RespError || !strings.Contains(out[0].Error, test.errorText) {
			t.Fatal(test, out)
		}
	}
	for i := 0; i < 25; i++ {
		if _, err := k.Bus().Publish(event.Spec{Subject: "workflow.owned", Kind: event.KindWorkflowStarted, Actor: "fixture", CorrelationID: strings.Repeat("x", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		limit any
		count float64
	}{{"1", 20}, {float64(1.9), 1}, {nil, 20}} {
		out := callAppHost(t, s, Request{ID: "history", Cmd: CmdWorkflowRuns, Token: "primary", Args: map[string]any{"ref": graph.ID, "limit": test.limit}})
		if len(out) != 1 || out[0].Type != RespResult || out[0].Result["count"] != test.count {
			t.Fatal(test, out)
		}
	}
}
func TestWorkflowAppGraphSchemasKeepRawConfigsAndRequiredFullFields(t *testing.T) {
	_, s, graph, _ := workflowAppFixture(t)
	specs := map[string]json.RawMessage{}
	for _, operation := range workflowOperations {
		spec := operation.Spec()
		specs[spec.Name] = spec.OutputSchema
	}
	out := callAppHost(t, s, Request{ID: "show", Cmd: CmdWorkflowShow, Token: "primary", Args: map[string]any{"ref": graph.ID}})
	if len(out) != 1 || out[0].Type != RespResult {
		t.Fatal(out)
	}
	row := out[0].Result
	node := row["workflow"].(map[string]any)["nodes"].([]any)[0].(map[string]any)
	for _, config := range []any{nil, map[string]any{"raw": true}, []any{false, float64(0)}, "raw", float64(3)} {
		node["config"] = config
		raw, _ := json.Marshal(row)
		if err := schema.ValidateJSON(specs[CmdWorkflowShow], raw); err != nil {
			t.Fatal(config, err)
		}
	}
	full := row["workflow"].(map[string]any)
	saved := full["nodes"]
	delete(full, "nodes")
	raw, _ := json.Marshal(row)
	if err := schema.ValidateJSON(specs[CmdWorkflowShow], raw); err == nil {
		t.Fatal("schema omitted required full nodes")
	}
	full["nodes"] = saved
	raw, _ = json.Marshal(map[string]any{"workflow": full, "created": false})
	if err := schema.ValidateJSON(specs[CmdWorkflowSave], raw); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(map[string]any{"workflow": full})
	if err := schema.ValidateJSON(specs[CmdWorkflowSave], raw); err == nil {
		t.Fatal("schema omitted required created:false")
	}
}
