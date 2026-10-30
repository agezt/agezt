// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// researchResponder answers the planner and the synthesis with the same text,
// so the run is deterministic.
func researchResponder(_ llm.CompletionRequest) llm.CompletionResponse {
	return mock.FinalText("the answer")
}

func researchKernel(t *testing.T) *runtime.Kernel {
	t.Helper()
	prov := mock.New()
	prov.Responder = researchResponder
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "research-model",
		Tools:    map[string]toolapi.Tool{},
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

// TestResearchAskBindsTheKernel: the harness runs on the primary kernel, the
// answer keeps the legacy wire shape, and the operation is audited.
func TestResearchAskBindsTheKernel(t *testing.T) {
	k := researchKernel(t)
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "c", Cmd: CmdResearchAsk, Token: "primary", Args: map[string]any{
		"question": "why?", "corr": "client-corr",
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
	want := []string{"cited_sources", "claims", "confidence", "correlation_id", "markdown", "notes", "question", "sources", "sub_questions", "verified"}
	if len(keys) != len(want) {
		t.Fatalf("wire keys=%v, want %v", keys, want)
	}
	for i, key := range want {
		if keys[i] != key {
			t.Fatalf("wire keys=%v, want %v", keys, want)
		}
	}
	if result["correlation_id"] != "client-corr" {
		t.Fatalf("the client correlation is kept: %v", result["correlation_id"])
	}
	if result["question"] != "why?" {
		t.Fatalf("the harness ran the question: %v", result["question"])
	}
	if _, exists := commandRegistry[CmdResearchAsk]; !exists {
		t.Fatal("research_ask is not registered")
	}
	wire := commandRegistry[CmdResearchAsk]
	if !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamLive {
		t.Fatalf("native wire %+v", wire)
	}
	var audited []event.Event
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op."+CmdResearchAsk {
			audited = append(audited, *e)
		}
		return nil
	})
	if len(audited) != 2 || audited[0].Kind != event.KindOpInvoked || audited[1].Kind != event.KindOpCompleted {
		t.Fatalf("audit arc=%v", audited)
	}
}

// TestResearchAskRejectsAMissingQuestion: the empty question is a clean error.
func TestResearchAskRejectsAMissingQuestion(t *testing.T) {
	k := researchKernel(t)
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "c", Cmd: CmdResearchAsk, Token: "primary", Args: map[string]any{}})[0]
	if resp.Type != RespError || resp.Error == "" {
		t.Fatalf("response=%+v", resp)
	}
}
