// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// runResponder answers every run with a fixed text.
func runResponder(_ llm.CompletionRequest) llm.CompletionResponse {
	return mock.FinalText("the answer")
}

// runKernel opens a kernel with a scripted provider.
func runKernel(t *testing.T) *runtime.Kernel {
	t.Helper()
	prov := mock.New()
	prov.Responder = runResponder
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "run-model",
		Tools:    map[string]toolapi.Tool{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestRunIsATypedOperation: the run command is app-owned, audited, tenant
// routed and streams — the same wire the native handler had.
func TestRunIsATypedOperation(t *testing.T) {
	wire, exists := commandRegistry[CmdRun]
	if !exists {
		t.Fatal("run is not registered")
	}
	if !wire.AppOwned || wire.ReadOnly {
		t.Fatalf("native wire %+v", wire)
	}
	if !wire.TenantAllowed || !wire.TenantRouted {
		t.Fatalf("a tenant's run must stay on its own kernel: %+v", wire)
	}
	if wire.Streaming != StreamEvents {
		t.Fatalf("streaming = %v, want StreamEvents", wire.Streaming)
	}
	found := 0
	for _, operation := range registeredAppOperations() {
		if operation.Spec().Name == CmdRun {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("run is registered %d times as a typed operation", found)
	}
}

// TestRunDryRunAnswersWithThePlan: the dry run answers with the plan and
// starts no run.
func TestRunDryRunAnswersWithThePlan(t *testing.T) {
	k := runKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	responses := callAppHost(t, s, Request{ID: "q1", Cmd: CmdRun, Token: "primary", Args: map[string]any{
		"intent":  "what is the weather",
		"dry_run": true,
	}})
	var result map[string]any
	for _, resp := range responses {
		switch resp.Type {
		case RespResult:
			result = resp.Result
		case RespEvent:
			t.Fatal("a dry run must not stream events")
		case RespError:
			t.Fatalf("the dry run failed: %s", resp.Error)
		}
	}
	if result == nil {
		t.Fatalf("the dry run answered nothing: %+v", responses)
	}
	if result["dry_run"] != true {
		t.Fatalf("dry_run = %v", result["dry_run"])
	}
	if result["intent"] != "what is the weather" {
		t.Fatalf("intent = %v", result["intent"])
	}
	if result["model"] != "run-model" {
		t.Fatalf("model = %v, want run-model", result["model"])
	}
	if _, ok := result["answer"]; ok {
		t.Fatal("the plan must not carry a run's answer")
	}
}

// TestRunAnswersWithTheRun: a plain run answers with the run's answer and
// correlation, and streams the run's events.
func TestRunAnswersWithTheRun(t *testing.T) {
	k := runKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	responses := callAppHost(t, s, Request{ID: "q1", Cmd: CmdRun, Token: "primary", Args: map[string]any{
		"intent": "say hi",
	}})
	var result map[string]any
	for _, resp := range responses {
		switch resp.Type {
		case RespResult:
			result = resp.Result
		case RespError:
			t.Fatalf("the run failed: %s", resp.Error)
		}
	}
	if result == nil {
		t.Fatalf("the run answered nothing: %+v", responses)
	}
	if result["answer"] != "the answer" {
		t.Fatalf("answer = %v", result["answer"])
	}
	corr, _ := result["correlation_id"].(string)
	if !strings.HasPrefix(corr, "run-") {
		t.Fatalf("correlation_id = %v", result["correlation_id"])
	}
}

// TestRunRefusesAMissingIntent: the run without an intent is a clean error.
func TestRunRefusesAMissingIntent(t *testing.T) {
	k := runKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "q1", Cmd: CmdRun, Token: "primary", Args: map[string]any{}})[0]
	if resp.Type != RespError || !strings.Contains(resp.Error, "args.intent required") {
		t.Fatalf("response = %+v", resp)
	}
}

// TestRunRejectsAMistypedDryRun: a dry_run sent as the string "true" must be
// an error, not a silent execution of the run the operator meant to preview.
func TestRunRejectsAMistypedDryRun(t *testing.T) {
	k := runKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "q1", Cmd: CmdRun, Token: "primary", Args: map[string]any{
		"intent":  "say hi",
		"dry_run": "true",
	}})[0]
	if resp.Type != RespError || !strings.Contains(resp.Error, "args.dry_run must be a boolean") {
		t.Fatalf("response = %+v", resp)
	}
}

// TestRunRejectsAMistypedTenant: the tenant argument is pinned to the
// authorized tenant, so a mistyped value is a usage error.
func TestRunRejectsAMistypedTenant(t *testing.T) {
	k := runKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "q1", Cmd: CmdRun, Token: "primary", Args: map[string]any{
		"intent": "say hi",
		"tenant": 7,
	}})[0]
	if resp.Type != RespError || !strings.Contains(resp.Error, "args.tenant must be a string") {
		t.Fatalf("response = %+v", resp)
	}
}

// TestRunWireShapeIsFlat: the run's terminal answer is a flat object, so the
// CLI reads it the way it always did.
func TestRunWireShapeIsFlat(t *testing.T) {
	k := runKernel(t)
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	responses := callAppHost(t, s, Request{ID: "q1", Cmd: CmdRun, Token: "primary", Args: map[string]any{
		"intent": "say hi",
	}})
	var result map[string]any
	for _, resp := range responses {
		if resp.Type == RespResult {
			result = resp.Result
		}
	}
	if result == nil {
		t.Fatalf("the run answered nothing: %+v", responses)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var flat map[string]any
	if err := json.Unmarshal(raw, &flat); err != nil {
		t.Fatal(err)
	}
	if _, ok := flat["answer"].(string); !ok {
		t.Fatalf("answer = %T", flat["answer"])
	}
}
