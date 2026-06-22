// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
)

// conductorFakeProvider returns role-appropriate text so a Conduct run completes:
// the verifier critique passes, everything else echoes a short answer.
type conductorFakeProvider struct{}

func (conductorFakeProvider) Name() string { return "conductor-cp-fake" }
func (conductorFakeProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	text := "ok"
	if strings.Contains(strings.ToLower(req.System), "verifier") {
		text = "PASS: looks correct"
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: text},
		StopReason: agent.StopEndTurn,
	}, nil
}

func TestConductor_RolesAndAskViaControlPlane(t *testing.T) {
	_, _, client, _ := startPairWithConfig(t, runtime.Config{
		Provider: conductorFakeProvider{},
		Tools:    map[string]agent.Tool{},
		CouncilMembers: func() []runtime.CouncilMember {
			return []runtime.CouncilMember{
				{Seat: "A", Model: "model-a"}, {Seat: "B", Model: "model-b"}, {Seat: "C", Model: "model-c"},
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// conductor_roles previews the auto-filled role→model mapping.
	roles, err := client.Call(ctx, controlplane.CmdConductorRoles, nil)
	if err != nil {
		t.Fatalf("CmdConductorRoles: %v", err)
	}
	if roles["thinker"] != "model-a" || roles["worker"] != "model-b" || roles["verifier"] != "model-c" {
		t.Errorf("roles = %v, want a/b/c", roles)
	}

	// conductor_ask runs the loop and returns the answer + transcript.
	res, err := client.Call(ctx, controlplane.CmdConductorAsk, map[string]any{
		"task": "what is 2+2?", "max_rounds": 1,
	})
	if err != nil {
		t.Fatalf("CmdConductorAsk: %v", err)
	}
	if passed, _ := res["passed"].(bool); !passed {
		t.Errorf("passed = %v, want true; res=%v", res["passed"], res)
	}
	if ans, _ := res["answer"].(string); ans == "" {
		t.Error("answer should not be empty")
	}
	steps, ok := res["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("steps missing/empty: %T %v", res["steps"], res["steps"])
	}
	if res["correlation_id"] == "" {
		t.Error("correlation_id should be set")
	}

	// Missing task → a clean error, not a panic.
	if _, err := client.Call(ctx, controlplane.CmdConductorAsk, map[string]any{}); err == nil {
		t.Error("empty task should error")
	}
}
