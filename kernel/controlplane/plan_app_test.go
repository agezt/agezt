// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// planResponder answers the plan's loop node with a fixed text.
func planResponder(_ llm.CompletionRequest) llm.CompletionResponse {
	return mock.FinalText("the answer")
}

func planKernel(t *testing.T) *runtime.Kernel {
	t.Helper()
	prov := mock.New()
	prov.Responder = planResponder
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "plan-model",
		Tools:    map[string]toolapi.Tool{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestPlanRunsOnTheSharedDispatcher: the plan command runs on the primary
// kernel, streams the plan's events and answers with the plan id and the node
// outputs.
func TestPlanRunsOnTheSharedDispatcher(t *testing.T) {
	k := planKernel(t)
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	s.token = "primary"
	plan := `{"nodes":[{"id":"a","kind":"loop","intent":"say hi"}]}`
	responses := callAppHost(t, s, Request{ID: "q1", Cmd: CmdPlan, Token: "primary", Args: map[string]any{
		"plan_json": plan,
	}})
	var events int
	var result map[string]any
	for _, resp := range responses {
		switch resp.Type {
		case RespEvent:
			events++
			if resp.Event == nil {
				t.Fatal("an event frame carries no event")
			}
		case RespResult:
			result = resp.Result
		case RespError:
			t.Fatalf("the plan failed: %s", resp.Error)
		}
	}
	if events == 0 {
		t.Fatal("the plan streamed no events")
	}
	if result == nil {
		t.Fatalf("the plan answered nothing: %+v", responses)
	}
	if result["plan_id"] != "plan-1" {
		t.Fatalf("plan_id=%v", result["plan_id"])
	}
	outputs, ok := result["node_outputs"].(map[string]any)
	if !ok {
		t.Fatalf("node_outputs=%T", result["node_outputs"])
	}
	if _, ok := outputs["a"]; !ok {
		t.Fatalf("the node's output is missing: %v", outputs)
	}
}

// TestPlanRefusesTheMissingPlan: the plan command without a plan is a clean
// error.
func TestPlanRefusesTheMissingPlan(t *testing.T) {
	k := planKernel(t)
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "q1", Cmd: CmdPlan, Token: "primary", Args: map[string]any{}})[0]
	if resp.Type != RespError || !strings.Contains(resp.Error, "args.plan_json required") {
		t.Fatalf("response=%+v", resp)
	}
}

// TestPlanIsATypedOperation: the plan command is app-owned, primary-only and
// streams.
func TestPlanIsATypedOperation(t *testing.T) {
	wire, exists := commandRegistry[CmdPlan]
	if !exists {
		t.Fatal("plan is not registered")
	}
	if !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamEvents {
		t.Fatalf("native wire %+v", wire)
	}
	found := 0
	for _, operation := range registeredAppOperations() {
		if operation.Spec().Name == CmdPlan {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("plan is registered %d times as a typed operation", found)
	}
}
