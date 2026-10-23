// SPDX-License-Identifier: MIT

package main

// The composition root's own tests for the port binding. The module's tests
// stand the adapters in; these run the REAL ones — the compat family, the
// offline mock and the Responses adapter — because that is where they are
// bound.

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/plugins/providers/openairesponses"
)

// TestDemoEchoProviderEchoesTheLastUserMessage covers the demo provider's
// behaviour, which now lives with the adapter rather than in the module: the
// last user message is echoed back, and an empty request still answers.
func TestDemoEchoProviderEchoesTheLastUserMessage(t *testing.T) {
	p := providerPorts().DemoEcho()
	if p == nil {
		t.Fatal("DemoEcho returned nil")
	}
	if p.Name() != "mock" {
		t.Errorf("Name() = %q, want mock", p.Name())
	}
	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "first"},
			{Role: llm.RoleAssistant, Content: "answer"},
			{Role: llm.RoleUser, Content: "hello e2e"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := resp.Message.Content; got != "[echo] hello e2e" {
		t.Errorf("content = %q, want %q", got, "[echo] hello e2e")
	}

	// No user message at all → a defined answer, never an empty one.
	empty, err := p.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("Complete (empty): %v", err)
	}
	if empty.Message.Content == "" {
		t.Error("empty request produced an empty answer")
	}
}

// TestProviderPortsBuildARealProvider covers the compat binding end to end: the
// catalog entry goes in, a wire provider comes out, and the model id it was
// constructed with is reported back.
func TestProviderPortsBuildARealProvider(t *testing.T) {
	entry := &catalog.Provider{
		ID: "local-test", NPM: "@ai-sdk/openai-compatible",
		API: "http://localhost:9/v1",
		Models: map[string]*catalog.Model{
			"m-1": {ID: "m-1", Name: "m-1"},
		},
	}
	prov, model, err := providerPorts().Build(entry, "m-1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if prov == nil {
		t.Fatal("Build returned no provider")
	}
	if model != "m-1" {
		t.Errorf("model = %q, want m-1", model)
	}
	if !strings.Contains(prov.Name(), "local-test") {
		t.Errorf("provider name = %q, want it to carry the catalog id", prov.Name())
	}
}

// TestProviderPortsRejectAnEmptyModel pins the adapter's own guard: a wire
// cannot be built without a model id. The module relies on that error to
// refuse a catalog entry with no models.
func TestProviderPortsRejectAnEmptyModel(t *testing.T) {
	entry := &catalog.Provider{
		ID: "local-test", NPM: "@ai-sdk/openai-compatible",
		API: "http://localhost:9/v1",
		Models: map[string]*catalog.Model{
			"m-1": {ID: "m-1", Name: "m-1"},
		},
	}
	if _, _, err := providerPorts().Build(entry, "", func(string) string { return "" }); err == nil {
		t.Error("Build with an empty model id must fail")
	}
}

// TestProviderPortsCatalogHelpers covers the catalog-facing halves of the
// binding: the family gate and the placeholder model. The engine leans on
// both — an unsupported family must never be registered, and the placeholder
// must be a model the entry actually serves.
func TestProviderPortsCatalogHelpers(t *testing.T) {
	entry := &catalog.Provider{
		ID: "local-test", NPM: "@ai-sdk/openai-compatible",
		API: "http://localhost:9/v1",
		Models: map[string]*catalog.Model{
			"m-2": {ID: "m-2", Name: "m-2"},
			"m-1": {ID: "m-1", Name: "m-1"},
		},
	}
	if got := providerPorts().FirstModelID(entry); got != "m-1" {
		t.Errorf("FirstModelID = %q, want m-1 (the lowest id)", got)
	}
	if got := providerPorts().FirstModelID(&catalog.Provider{ID: "empty"}); got != "" {
		t.Errorf("FirstModelID = %q, want empty for an entry with no models", got)
	}
	if !providerPorts().SupportedFamily(catalog.FamilyOpenAICompatible) {
		t.Error("SupportedFamily = false, want true for the openai-compatible family")
	}
	if providerPorts().SupportedFamily(catalog.Family("not-a-family")) {
		t.Error("SupportedFamily = true, want false for an unknown family")
	}
}

// TestProviderPortsChatGPTBaseURL pins that the ChatGPT base URL reaches the
// catalog entry: an empty one would point the entry nowhere.
func TestProviderPortsChatGPTBaseURL(t *testing.T) {
	if got := providerPorts().ChatGPTBaseURL(); got == "" {
		t.Error("ChatGPTBaseURL = empty — the catalog entry would point nowhere")
	}
}

// TestProviderPortsChatGPTDiscoverMapsTheBackendReply covers the mapping of
// the backend's reply into the module's own model row: the fields the module
// routes on (slug, display name, priority, instructions, modalities) must
// survive the crossing, or the catalog entry and the picker describe a model
// that does not exist.
func TestProviderPortsChatGPTDiscoverMapsTheBackendReply(t *testing.T) {
	models := chatGPTModelRows([]openairesponses.ModelInfo{
		{
			Slug: "gpt-x", DisplayName: "GPT-X", Visibility: "list", Priority: 3,
			ContextWindow: 400000, MaxContextWindow: 500000,
			InputModalities: []string{"text", "image"}, BaseInstructions: "instr-x",
		},
		{Slug: "hidden-y", DisplayName: "Hidden", Visibility: "hide", Priority: 9},
	})
	if len(models) != 2 {
		t.Fatalf("models = %d, want 2 (hidden entries are returned too; the module filters them)", len(models))
	}
	x := models[0]
	if x.Slug != "gpt-x" || x.DisplayName != "GPT-X" || x.Priority != 3 {
		t.Errorf("mapped row = %+v, want slug/display name/priority carried over", x)
	}
	if x.BaseInstructions != "instr-x" {
		t.Errorf("BaseInstructions = %q, want instr-x", x.BaseInstructions)
	}
	if x.MaxContextWindow != 500000 || x.ContextWindow != 400000 {
		t.Errorf("context windows = %d/%d, want 500000/400000", x.MaxContextWindow, x.ContextWindow)
	}
	if len(x.InputModalities) != 2 {
		t.Errorf("InputModalities = %v, want two entries", x.InputModalities)
	}
	if models[1].Visibility != "hide" {
		t.Errorf("Visibility = %q, want hide", models[1].Visibility)
	}
}

// TestChatGPTTokenFn exercises both branches of the token function against a
// token source with no tokens: the adapter's 401-retry path (force) and the
// normal path. Neither may panic, and both must report the missing token.
func TestChatGPTTokenFn(t *testing.T) {
	src := &emptyTokenSource{}
	fn := chatGPTTokenFn(src)
	if fn == nil {
		t.Fatal("chatGPTTokenFn returned nil")
	}
	if _, _, err := fn(context.Background(), false); err == nil {
		t.Error("non-force path: want an error when no token is stored")
	}
	if _, _, err := fn(context.Background(), true); err == nil {
		t.Error("force path: want an error when no token is stored")
	}
	if src.forced != 1 {
		t.Errorf("forced refreshes = %d, want 1 (the force path must force)", src.forced)
	}
}

// emptyTokenSource is a token source with nothing stored.
type emptyTokenSource struct {
	forced int
}

func (s *emptyTokenSource) HasTokens() bool { return false }

func (s *emptyTokenSource) Token(context.Context) (string, string, error) {
	return "", "", errNoToken
}

func (s *emptyTokenSource) ForceRefresh(context.Context) (string, string, error) {
	s.forced++
	return "", "", errNoToken
}

func (s *emptyTokenSource) Account() (string, string) { return "", "" }

var errNoToken = errNoTokenType{}

type errNoTokenType struct{}

func (errNoTokenType) Error() string { return "no token stored" }
