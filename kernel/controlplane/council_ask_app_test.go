// SPDX-License-Identifier: MIT

package controlplane

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// councilResponder answers every panel seat and the chair's synthesis with the
// same text, so the deliberation is deterministic.
func councilResponder(_ llm.CompletionRequest) llm.CompletionResponse {
	return mock.FinalText("agreed")
}

func councilKernel(t *testing.T) *runtime.Kernel {
	t.Helper()
	prov := mock.New()
	prov.Responder = councilResponder
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "panel-model",
		Tools:    map[string]toolapi.Tool{},
		CouncilMembers: func() []runtime.CouncilMember {
			return []runtime.CouncilMember{{Seat: "Chair", Model: "model-a"}, {Seat: "Scribe", Model: "model-b"}}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The kernel cannot import the reasoning module itself (archcheck's
	// principle P5), so the composition root builds it and injects it.
	k.SetReasoning(reasoning.New(k.ReasoningDependencies()))
	t.Cleanup(func() { k.Close() })
	return k
}

// TestCouncilAskBindsTheKernel: the panel runs on the primary kernel, the
// answer keeps the legacy wire shape, and the operation is audited.
func TestCouncilAskBindsTheKernel(t *testing.T) {
	k := councilKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "c", Cmd: CmdCouncilAsk, Token: "primary", Args: map[string]any{
		"question": "why?", "rounds": 1, "corr": "client-corr",
	}})[0]
	if resp.Type != RespResult {
		t.Fatalf("response=%+v", resp)
	}
	result := resp.Result
	// The legacy wire: the nine keys, always present.
	keys := make([]string, 0, len(result))
	for key := range result {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"as_of", "brief", "consensus", "correlation_id", "dissent", "members", "opinions", "question", "rounds"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("wire keys=%v, want %v", keys, want)
	}
	if result["correlation_id"] != "client-corr" {
		t.Fatalf("the client correlation is kept: %v", result["correlation_id"])
	}
	if result["question"] != "why?" || result["consensus"] != "agreed" {
		t.Fatalf("the panel answered: %v", result)
	}
	members, ok := result["members"].([]any)
	if !ok || len(members) != 2 {
		t.Fatalf("members=%v", result["members"])
	}
	opinions, ok := result["opinions"].([]any)
	if !ok || len(opinions) == 0 {
		t.Fatalf("opinions=%v", result["opinions"])
	}
	row, ok := opinions[0].(map[string]any)
	if !ok || row["seat"] == "" || row["model"] == "" {
		t.Fatalf("opinion row=%v", opinions[0])
	}
	// The operation is a live, audited, primary-only write.
	if wire, exists := commandRegistry[CmdCouncilAsk]; !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamLive {
		t.Fatalf("native wire %+v", wire)
	}
	var audited []event.Event
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op."+CmdCouncilAsk {
			audited = append(audited, *e)
		}
		return nil
	})
	if len(audited) != 2 || audited[0].Kind != event.KindOpInvoked || audited[1].Kind != event.KindOpCompleted {
		t.Fatalf("audit arc=%v", audited)
	}
}

// TestCouncilAskRejectsAMissingQuestion: the empty question is a clean error.
func TestCouncilAskRejectsAMissingQuestion(t *testing.T) {
	k := councilKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "c", Cmd: CmdCouncilAsk, Token: "primary", Args: map[string]any{}})[0]
	if resp.Type != RespError || !strings.Contains(resp.Error, "args.question required") {
		t.Fatalf("response=%+v", resp)
	}
}

// TestConductorAskBindsTheKernel: the Conductor runs the same loop on the
// primary kernel and keeps the legacy wire shape.
func TestConductorAskBindsTheKernel(t *testing.T) {
	k := councilKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "c", Cmd: CmdConductorAsk, Token: "primary", Args: map[string]any{
		"task": "what is 2+2?", "max_rounds": 1,
	}})[0]
	if resp.Type != RespResult {
		t.Fatalf("response=%+v", resp)
	}
	result := resp.Result
	keys := make([]string, 0, len(result))
	for key := range result {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"answer", "correlation_id", "passed", "plan", "roles", "rounds", "steps", "task"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("wire keys=%v, want %v", keys, want)
	}
	if result["task"] != "what is 2+2?" {
		t.Fatalf("the loop ran the task: %v", result["task"])
	}
	steps, ok := result["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("steps=%v", result["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok || step["role"] == "" || step["model"] == "" {
		t.Fatalf("step row=%v", steps[0])
	}
	if wire, exists := commandRegistry[CmdConductorAsk]; !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamLive {
		t.Fatalf("native wire %+v", wire)
	}
}

// TestCognitionOperationsAreTyped: both commands are app-owned typed
// operations, registered once each.
func TestCognitionOperationsAreTyped(t *testing.T) {
	for _, cmd := range []string{CmdCouncilAsk, CmdConductorAsk} {
		found := 0
		for _, operation := range registeredAppOperations() {
			if operation.Spec().Name == cmd {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("%s is registered %d times as a typed operation", cmd, found)
		}
	}
}
