// SPDX-License-Identifier: MIT

package planner_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/planner"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fencedJSON wraps a JSON object in the ```json fence the planner
// asks for. Used by the happy-path tests to mimic a well-behaved LLM.
func fencedJSON(j string) string {
	return "```json\n" + j + "\n```"
}

func TestGenerate_HappyPath_TwoNodes(t *testing.T) {
	plan := `{
		"name": "research and draft",
		"max_parallel": 2,
		"nodes": [
			{"id": "research", "kind": "loop", "intent": "research topic X", "deps": []},
			{"id": "draft", "kind": "loop", "intent": "draft summary based on research", "deps": ["research"]}
		]
	}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))

	raw, p, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "write me a quick brief on X")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(raw, `"id": "research"`) {
		t.Errorf("rawJSON does not look like the unwrapped plan: %s", raw)
	}
	if !strings.Contains(raw, `"intent"`) || p.Intent == nil || p.Intent.CanonicalIntent != "write me a quick brief on X" {
		t.Errorf("generated plan did not preserve intent metadata: raw=%s intent=%+v", raw, p.Intent)
	}
	if p.Name != "research and draft" {
		t.Errorf("Name = %q", p.Name)
	}
	if len(p.Nodes) != 2 {
		t.Fatalf("Nodes len = %d, want 2", len(p.Nodes))
	}
	if p.Nodes[0].ID != "research" || p.Nodes[1].ID != "draft" {
		t.Errorf("node ids = %v", []string{p.Nodes[0].ID, p.Nodes[1].ID})
	}
	if len(p.Nodes[1].Deps) != 1 || p.Nodes[1].Deps[0] != "research" {
		t.Errorf("draft deps = %v", p.Nodes[1].Deps)
	}
}

func TestGenerate_SingleNodePlan(t *testing.T) {
	plan := `{"name":"trivial","max_parallel":1,"nodes":[{"id":"do_it","kind":"loop","intent":"the thing"}]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, p, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "the thing")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(p.Nodes) != 1 {
		t.Errorf("Nodes len = %d", len(p.Nodes))
	}
}

func TestGenerate_BareJSONWithoutFence(t *testing.T) {
	// Some models (tool-use-trained) skip fences when asked for "just JSON".
	plan := `{"name":"bare","nodes":[{"id":"x","kind":"loop","intent":"do x"}]}`
	prov := mock.New(mock.FinalText(plan))
	_, p, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "do x")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if p.Name != "bare" {
		t.Errorf("Name = %q", p.Name)
	}
}

func TestGenerate_GateNode(t *testing.T) {
	plan := `{
		"nodes": [
			{"id":"propose","kind":"loop","intent":"propose the change"},
			{"id":"approve","kind":"gate","capability":"plan.execute","description":"Approve the proposed change?","deps":["propose"]},
			{"id":"execute","kind":"loop","intent":"apply the change","deps":["approve"]}
		]
	}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, p, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "deploy thing")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if p.Nodes[1].Kind != "gate" {
		t.Errorf("node[1].Kind = %q, want gate", p.Nodes[1].Kind)
	}
	if p.Nodes[1].Description == "" {
		t.Errorf("gate node missing description")
	}
}

func TestGenerate_RejectsMissingProvider(t *testing.T) {
	_, _, err := planner.Generate(context.Background(), planner.Config{}, "x")
	if err == nil || !strings.Contains(err.Error(), "Provider required") {
		t.Errorf("err = %v, want Provider-required", err)
	}
}

func TestGenerate_RejectsEmptyIntent(t *testing.T) {
	prov := mock.New(mock.FinalText("{}"))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "   ")
	if err == nil || !strings.Contains(err.Error(), "intent required") {
		t.Errorf("err = %v, want intent-required", err)
	}
}

func TestGenerate_RejectsEmptyLLMResponse(t *testing.T) {
	prov := mock.New(mock.FinalText(""))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "do thing")
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("err = %v, want empty-response", err)
	}
}

func TestGenerate_RejectsNonJSONResponse(t *testing.T) {
	prov := mock.New(mock.FinalText("hi i'm a friendly model and here's some text"))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "do thing")
	if err == nil {
		t.Fatal("expected error for non-JSON response")
	}
	if !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("err = %v, want not-JSON message", err)
	}
}

func TestGenerate_RejectsUnclosedFence(t *testing.T) {
	prov := mock.New(mock.FinalText("```json\n{\"nodes\":[]}\n"))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "no closing fence") {
		t.Errorf("err = %v, want no-closing-fence", err)
	}
}

func TestGenerate_RejectsEmptyNodes(t *testing.T) {
	prov := mock.New(mock.FinalText(fencedJSON(`{"nodes":[]}`)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "no nodes") {
		t.Errorf("err = %v, want no-nodes", err)
	}
}

func TestGenerate_RejectsDuplicateIDs(t *testing.T) {
	plan := `{"nodes":[
		{"id":"a","kind":"loop","intent":"x"},
		{"id":"a","kind":"loop","intent":"y"}
	]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("err = %v, want duplicate-id", err)
	}
}

func TestGenerate_RejectsUnknownDep(t *testing.T) {
	plan := `{"nodes":[
		{"id":"a","kind":"loop","intent":"x","deps":["ghost"]}
	]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), `dep "ghost" does not exist`) {
		t.Errorf("err = %v, want unknown-dep", err)
	}
}

func TestGenerate_RejectsSelfDep(t *testing.T) {
	plan := `{"nodes":[
		{"id":"a","kind":"loop","intent":"x","deps":["a"]}
	]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "depends on itself") {
		t.Errorf("err = %v, want self-dep", err)
	}
}

func TestGenerate_RejectsCycle(t *testing.T) {
	plan := `{"nodes":[
		{"id":"a","kind":"loop","intent":"x","deps":["b"]},
		{"id":"b","kind":"loop","intent":"y","deps":["a"]}
	]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err = %v, want cycle", err)
	}
}

func TestGenerate_RejectsUnknownKind(t *testing.T) {
	plan := `{"nodes":[{"id":"a","kind":"frobnicate","intent":"x"}]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("err = %v, want unknown-kind", err)
	}
}

func TestGenerate_RejectsEmptyLoopIntent(t *testing.T) {
	plan := `{"nodes":[{"id":"a","kind":"loop","intent":"  "}]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "intent is empty") {
		t.Errorf("err = %v, want intent-empty", err)
	}
}

func TestGenerate_RejectsEmptyGateDescription(t *testing.T) {
	plan := `{"nodes":[
		{"id":"a","kind":"loop","intent":"x"},
		{"id":"g","kind":"gate","description":"","deps":["a"]}
	]}`
	prov := mock.New(mock.FinalText(fencedJSON(plan)))
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: prov}, "x")
	if err == nil || !strings.Contains(err.Error(), "description is empty") {
		t.Errorf("err = %v, want description-empty", err)
	}
}

func TestGenerate_HonorsSystemOverride(t *testing.T) {
	// Verify the SystemOverride field actually replaces the system
	// prompt for the call (caller-injected planner-prompt tuning).
	custom := "you must always emit a one-node plan named ZZZ-CUSTOM"
	prov := mock.New(mock.FinalText(fencedJSON(`{"nodes":[{"id":"a","kind":"loop","intent":"x"}]}`)))
	var seenSystem string
	prov.OnRequest = func(req agent.CompletionRequest) { seenSystem = req.System }
	_, _, err := planner.Generate(context.Background(),
		planner.Config{Provider: prov, SystemOverride: custom},
		"x")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if seenSystem != custom {
		t.Errorf("system = %q, want %q", seenSystem, custom)
	}
}

func TestGenerate_SendsIntentFrameInsteadOfBareUtterance(t *testing.T) {
	plan := `{"nodes":[{"id":"confirm","kind":"gate","description":"Confirm file cleanup scope."}]}`
	cp := &captureProvider{resp: fencedJSON(plan)}
	if _, _, err := planner.Generate(context.Background(), planner.Config{Provider: cp}, "dosyaları temizle"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(cp.gotUserContent, "INTENT_FRAME:\n") {
		t.Fatalf("planner user message should be a formal intent frame: %q", cp.gotUserContent)
	}
	if !strings.Contains(cp.gotUserContent, `"canonical_intent": "dosyaları temizle"`) {
		t.Fatalf("intent frame missing canonical intent: %q", cp.gotUserContent)
	}
	if !strings.Contains(cp.gotUserContent, `"underdetermined": true`) {
		t.Fatalf("intent frame missing ambiguity metadata: %q", cp.gotUserContent)
	}
	if !strings.Contains(cp.gotUserContent, `"harmful_reading"`) {
		t.Fatalf("intent frame missing harmful reading: %q", cp.gotUserContent)
	}
}

func TestGenerateFromIntent_RejectsEmptyFrame(t *testing.T) {
	cp := &captureProvider{resp: fencedJSON(`{"nodes":[{"id":"a","kind":"loop","intent":"x"}]}`)}
	_, _, err := planner.GenerateFromIntent(context.Background(), planner.Config{Provider: cp}, intentmodel.Frame{})
	if err == nil || !strings.Contains(err.Error(), "intent frame required") {
		t.Fatalf("err = %v, want intent-frame-required", err)
	}
}

func TestGenerateFromIntent_UsesProvidedFrame(t *testing.T) {
	plan := `{"nodes":[{"id":"confirm","kind":"gate","description":"Confirm scope before deletion."}]}`
	cp := &captureProvider{resp: fencedJSON(plan)}
	frame := intentmodel.Frame{
		UserUtteranceHash: "abc",
		CanonicalIntent:   "clean archived files",
		HarmfulReading:    "Could delete non-cache archived files.",
		AmbiguityScore:    0.8,
		Underdetermined:   true,
	}
	if _, _, err := planner.GenerateFromIntent(context.Background(), planner.Config{Provider: cp}, frame); err != nil {
		t.Fatalf("GenerateFromIntent: %v", err)
	}
	if !strings.Contains(cp.gotUserContent, `"user_utterance_hash": "abc"`) {
		t.Fatalf("provided frame was not used: %q", cp.gotUserContent)
	}
	if !strings.Contains(cp.gotUserContent, `"harmful_reading": "Could delete non-cache archived files."`) {
		t.Fatalf("provided harmful reading missing: %q", cp.gotUserContent)
	}
}

func TestGenerate_RejectsUnderdeterminedHighRegretLoopWithoutGate(t *testing.T) {
	plan := `{"nodes":[{"id":"delete_files","kind":"loop","intent":"delete files"}]}`
	cp := &captureProvider{resp: fencedJSON(plan)}
	_, _, err := planner.Generate(context.Background(), planner.Config{Provider: cp}, "clean files")
	if err == nil || !strings.Contains(err.Error(), "requires a gate") {
		t.Fatalf("err = %v, want requires-a-gate", err)
	}
}

func TestGenerate_AllowsUnderdeterminedHighRegretLoopBehindGate(t *testing.T) {
	plan := `{"nodes":[
		{"id":"confirm_scope","kind":"gate","description":"Confirm exact cleanup scope before deleting files."},
		{"id":"delete_files","kind":"loop","intent":"delete files","deps":["confirm_scope"]}
	]}`
	cp := &captureProvider{resp: fencedJSON(plan)}
	_, p, err := planner.Generate(context.Background(), planner.Config{Provider: cp}, "clean files")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(p.Nodes) != 2 || p.Nodes[1].Deps[0] != "confirm_scope" {
		t.Fatalf("plan = %+v", p)
	}
}

// captureProvider records the CompletionRequest the planner sends.
type captureProvider struct {
	gotJSONMode    bool
	gotUserContent string
	resp           string
}

func (c *captureProvider) Name() string { return "capture" }
func (c *captureProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	c.gotJSONMode = req.JSONMode
	if len(req.Messages) > 0 {
		c.gotUserContent = req.Messages[0].Content
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: c.resp},
		StopReason: agent.StopEndTurn,
	}, nil
}

// TestGenerate_RequestsJSONMode (M313): plan generation is SPEC-10's canonical
// structured-output case, so the planner asks the provider for JSON mode.
func TestGenerate_RequestsJSONMode(t *testing.T) {
	plan := `{"nodes":[{"id":"a","kind":"loop","intent":"x","deps":[]}]}`
	cp := &captureProvider{resp: fencedJSON(plan)}
	if _, _, err := planner.Generate(context.Background(), planner.Config{Provider: cp}, "do x"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !cp.gotJSONMode {
		t.Error("planner should request JSONMode for structured plan output (SPEC-10)")
	}
}
