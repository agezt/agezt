// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRun_PolicyAuditRetainsSharedDetails(t *testing.T) {
	b, j := newTestBus(t)
	tool := &countingTool{}
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Bus: b, Actor: "actor", CorrelationID: "policy-corr", Tools: map[string]agent.Tool{"echo": tool},
		Provider: mock.New(testToolUse("policy-call", "echo", map[string]any{}), mock.FinalText("done")),
		Policy: func(context.Context, agent.ToolCall) agent.PolicyVerdict {
			return agent.PolicyVerdict{Allow: true, Capability: "file.read", Reason: "allowed", AffectedResources: []string{"file:one"}, EpistemicConfidence: 0.75, ObservationSources: []string{"web:one"}, ObservationDirectiveLike: true}
		},
	}, "run")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	if err := j.Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision {
			return nil
		}
		found++
		if e.CorrelationID != "policy-corr" {
			t.Errorf("correlation=%s", e.CorrelationID)
		}
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return err
		}
		if len(payload) != 23 || payload["tool"] != "echo" || payload["call_id"] != "policy-call" || payload["epistemic_confidence"] != 0.75 || payload["directive_like"] != true {
			t.Errorf("payload=%s", e.Payload)
		}
		if resources, ok := payload["affected_resources"].([]any); !ok || len(resources) != 1 || resources[0] != "file:one" {
			t.Errorf("resources=%v", payload["affected_resources"])
		}
		if sources, ok := payload["observation_sources"].([]any); !ok || len(sources) != 1 || sources[0] != "web:one" {
			t.Errorf("sources=%v", payload["observation_sources"])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if found != 1 || tool.calls != 1 {
		t.Fatalf("records=%d calls=%d", found, tool.calls)
	}
}
