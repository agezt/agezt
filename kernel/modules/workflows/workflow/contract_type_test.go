// SPDX-License-Identifier: MIT

package workflow

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workflowapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the
// CONTRACT'S row, not a copy of it. A copy would drift — the module would
// then keep a shape the contract no longer describes, and the agent-facing
// tool (which speaks the contract) would no longer fit it.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Workflow", Workflow{}, workflowapi.Workflow{}},
		{"Node", Node{}, workflowapi.Node{}},
		{"Edge", Edge{}, workflowapi.Edge{}},
		{"TriggerConfig", TriggerConfig{}, workflowapi.TriggerConfig{}},
		{"ToolConfig", ToolConfig{}, workflowapi.ToolConfig{}},
		{"LLMConfig", LLMConfig{}, workflowapi.LLMConfig{}},
		{"ConditionConfig", ConditionConfig{}, workflowapi.ConditionConfig{}},
		{"TransformConfig", TransformConfig{}, workflowapi.TransformConfig{}},
		{"DelayConfig", DelayConfig{}, workflowapi.DelayConfig{}},
		{"HTTPConfig", HTTPConfig{}, workflowapi.HTTPConfig{}},
		{"CodeConfig", CodeConfig{}, workflowapi.CodeConfig{}},
		{"MapConfig", MapConfig{}, workflowapi.MapConfig{}},
		{"FilterConfig", FilterConfig{}, workflowapi.FilterConfig{}},
		{"SwitchCase", SwitchCase{}, workflowapi.SwitchCase{}},
		{"SwitchConfig", SwitchConfig{}, workflowapi.SwitchConfig{}},
		{"MergeConfig", MergeConfig{}, workflowapi.MergeConfig{}},
		{"ApprovalConfig", ApprovalConfig{}, workflowapi.ApprovalConfig{}},
		{"SubflowConfig", SubflowConfig{}, workflowapi.SubflowConfig{}},
		{"PipelineConfig", PipelineConfig{}, workflowapi.PipelineConfig{}},
		{"PipelineStepConfig", PipelineStepConfig{}, workflowapi.PipelineStepConfig{}},
		{"RunResult", RunResult{}, workflowapi.RunResult{}},
		{"NodeTestResult", NodeTestResult{}, workflowapi.NodeTestResult{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheNodeTypesAreTheContracts: the node type is what a stored graph
// says it is, so the module and the contract must spell them the same way.
func TestTheNodeTypesAreTheContracts(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   string
		contract string
	}{
		{"NodeTrigger", NodeTrigger, workflowapi.NodeTrigger},
		{"NodeTool", NodeTool, workflowapi.NodeTool},
		{"NodeLLM", NodeLLM, workflowapi.NodeLLM},
		{"NodeCondition", NodeCondition, workflowapi.NodeCondition},
		{"NodeTransform", NodeTransform, workflowapi.NodeTransform},
		{"NodeDelay", NodeDelay, workflowapi.NodeDelay},
		{"NodeHTTP", NodeHTTP, workflowapi.NodeHTTP},
		{"NodeCode", NodeCode, workflowapi.NodeCode},
		{"NodeMap", NodeMap, workflowapi.NodeMap},
		{"NodeFilter", NodeFilter, workflowapi.NodeFilter},
		{"NodeSwitch", NodeSwitch, workflowapi.NodeSwitch},
		{"NodeMerge", NodeMerge, workflowapi.NodeMerge},
		{"NodeApproval", NodeApproval, workflowapi.NodeApproval},
		{"NodeSubflow", NodeSubflow, workflowapi.NodeSubflow},
		{"NodePipeline", NodePipeline, workflowapi.NodePipeline},
	} {
		if c.module != c.contract {
			t.Errorf("%s = %q, want the contract's %q", c.name, c.module, c.contract)
		}
	}
}

// TestTheRuleIsTheContracts: the module's delegation and the contract's
// must agree on every verdict — and on the message, because a rule that
// refuses for the wrong reason is a rule nobody can debug.
func TestTheRuleIsTheContracts(t *testing.T) {
	cases := []Workflow{
		{Name: "ok", Nodes: []Node{{ID: "start", Type: NodeTrigger}}},
		{Name: "Bad Name", Nodes: []Node{{ID: "start", Type: NodeTrigger}}},
		{Name: "x"},
		{Name: "x", Nodes: []Node{{ID: "a", Type: NodeTool}}},
		{Name: "x", Nodes: []Node{{ID: "start", Type: NodeTrigger}, {ID: "t2", Type: NodeTrigger}}},
		{Name: "x", Nodes: []Node{{ID: "start", Type: NodeTrigger}, {ID: "Bad ID", Type: NodeTool, Config: json.RawMessage(`{"tool":"x"}`)}}},
		{Name: "x", Nodes: []Node{{ID: "start", Type: NodeTrigger}, {ID: "a", Type: "spaceship"}}},
		{Name: "x", Nodes: []Node{{ID: "start", Type: NodeTrigger}}, Edges: []Edge{{From: "start", To: "ghost"}}},
		{Name: "x", Nodes: []Node{{ID: "start", Type: NodeTrigger}, {ID: "a", Type: NodeTool, Config: json.RawMessage(`{"tool":"x"}`)}}, Edges: []Edge{{From: "a", To: "a"}}},
		{Name: "x", Nodes: []Node{{ID: "start", Type: NodeTrigger}, {ID: "a", Type: NodeTool, Config: json.RawMessage(`{}`)}}},
		{Name: "x", Nodes: []Node{{ID: "start", Type: NodeTrigger}, {ID: "a", Type: NodeDelay, Config: json.RawMessage(`{"seconds":99999}`)}}},
		{Name: "x", Nodes: []Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron"}`)}}},
		{Name: "x", Nodes: []Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","daily_at":"25:99"}`)}}},
		{Name: "x", Nodes: []Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"event"}`)}}},
	}
	for _, w := range cases {
		gotErr, wantErr := Validate(w), workflowapi.Validate(w)
		if (gotErr == nil) != (wantErr == nil) {
			t.Errorf("the module and the contract disagree on %+v: %v vs %v", w, gotErr, wantErr)
			continue
		}
		if gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Errorf("the module's error %q differs from the contract's %q", gotErr, wantErr)
		}
	}
}

// TestTheStoreIsTheContractsPort: the module's store answers the contract's
// read port, so the agent-facing tool can be handed the library without ever
// naming this module.
func TestTheStoreIsTheContractsPort(t *testing.T) {
	var _ workflowapi.Store = (*Store)(nil)
}

// TestTheNotFoundErrorIsTheContracts: the error the store returns for an
// unknown workflow is the contract's — one error, one spelling.
func TestTheNotFoundErrorIsTheContracts(t *testing.T) {
	if !errors.Is(ErrNotFound, workflowapi.ErrNotFound) {
		t.Errorf("the module's ErrNotFound %v is not the contract's %v", ErrNotFound, workflowapi.ErrNotFound)
	}
}
