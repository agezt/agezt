// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRun_ResolutionRejectionsPreserveBatchOrder(t *testing.T) {
	b, j := newTestBus(t)
	tool := &admissionProbe{}
	policyCalls, hooks := 0, 0
	calls := []agent.ToolCall{
		{ID: "missing", Name: "missing", Input: json.RawMessage(`{`)},
		{ID: "invalid", Name: "probe", Input: json.RawMessage(`{}`)},
		{ID: "valid", Name: "probe", Input: json.RawMessage(`{"n":1}`)},
		{ID: "guard", Name: "probe", Input: json.RawMessage(`{"n":1}`)},
	}
	prov := mock.New(agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: calls}, StopReason: agent.StopToolUse}, mock.FinalText("done"))
	_, err := agent.Run(context.Background(), agent.LoopConfig{Provider: prov, Tools: map[string]agent.Tool{"probe": tool}, Bus: b, Actor: "actor", CorrelationID: "corr", MaxIdenticalToolCalls: 1, Policy: func(context.Context, agent.ToolCall) agent.PolicyVerdict {
		policyCalls++
		return agent.PolicyVerdict{Allow: true}
	}, ToolResultHook: func(context.Context, agent.ToolCall, agent.Result) { hooks++ }}, "run")
	if err != nil || policyCalls != 1 || tool.calls.Load() != 1 || hooks != 1 {
		t.Fatalf("error=%v policy=%d backend=%d hooks=%d", err, policyCalls, tool.calls.Load(), hooks)
	}
	var arc []string
	wantOutput := map[string]string{"missing": `tool "missing" is not available`, "invalid": "tool call rejected by schema: $.n is required", "valid": "ok", "guard": "loop guard:"}
	wantTool := map[string]string{"missing": "missing", "invalid": "probe", "valid": "probe", "guard": "probe"}
	if err := j.Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolInvoked && e.Kind != event.KindToolResult {
			return nil
		}
		var p struct {
			CallID       string `json:"call_id"`
			Tool, Output string
			Error        bool
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		arc = append(arc, string(e.Kind)+":"+p.CallID)
		if e.Actor != "actor" || e.CorrelationID != "corr" {
			t.Errorf("lost envelope: %+v", e)
		}
		if e.Kind == event.KindToolResult {
			output, ok := wantOutput[p.CallID]
			if !ok || !strings.HasPrefix(p.Output, output) || p.Error != (p.CallID != "valid") || p.Tool != wantTool[p.CallID] {
				t.Errorf("result=%s", e.Payload)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wantArc := []string{"policy.decision:valid", "tool.invoked:valid", "tool.result:missing", "tool.result:invalid", "tool.result:valid", "tool.result:guard"}
	if !reflect.DeepEqual(arc, wantArc) {
		t.Errorf("arc=%v want=%v", arc, wantArc)
	}
}
