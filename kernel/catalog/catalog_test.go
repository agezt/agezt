// SPDX-License-Identifier: MIT

package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

// ---- fixture ----

// fixtureAPI is a minimal models.dev-shaped catalog used across tests.
// Two providers: an Anthropic-family one with priced models and an
// OpenAI-compatible one (Upstage).
const fixtureAPI = `{
  "anthropic": {
    "id": "anthropic",
    "name": "Anthropic",
    "env": ["ANTHROPIC_API_KEY"],
    "npm": "@ai-sdk/anthropic",
    "api": "https://api.anthropic.com",
    "doc": "https://docs.anthropic.com",
    "models": {
      "claude-opus-4-5": {
        "id": "claude-opus-4-5",
        "name": "Claude Opus 4.5",
        "family": "claude-opus",
        "tool_call": true,
        "modalities": {"input":["text","image"], "output":["text"]},
        "limit": {"context": 200000, "output": 64000},
        "cost": {"input": 5, "output": 25}
      },
      "claude-haiku-4-5": {
        "id": "claude-haiku-4-5",
        "name": "Claude Haiku 4.5",
        "family": "claude-haiku",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 200000, "output": 8192},
        "cost": {"input": 0.8, "output": 4}
      }
    }
  },
  "upstage": {
    "id": "upstage",
    "name": "Upstage",
    "env": ["UPSTAGE_API_KEY"],
    "npm": "@ai-sdk/openai-compatible",
    "api": "https://api.upstage.ai/v1/solar",
    "models": {
      "solar-mini": {
        "id": "solar-mini", "name": "solar-mini", "family": "solar-mini",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 0.15, "output": 0.15}
      }
    }
  }
}`

// ---- parse / family ----

func TestParseAPIFile(t *testing.T) {
	c, err := catalog.ParseAPIFile([]byte(fixtureAPI))
	if err != nil {
		t.Fatalf("ParseAPIFile: %v", err)
	}
	if len(c.Providers) != 2 {
		t.Fatalf("providers=%d want 2", len(c.Providers))
	}
	ant := c.Providers["anthropic"]
	if ant == nil {
		t.Fatal("missing anthropic provider")
	}
	if ant.Name != "Anthropic" || ant.API != "https://api.anthropic.com" {
		t.Errorf("anthropic basic fields: %+v", ant)
	}
	if len(ant.Models) != 2 {
		t.Errorf("anthropic models=%d want 2", len(ant.Models))
	}
}

func TestFamilyFromNPM(t *testing.T) {
	cases := map[string]catalog.Family{
		"@ai-sdk/anthropic":               catalog.FamilyAnthropic,
		"anthropic":                       catalog.FamilyAnthropic,
		"@ai-sdk/openai":                  catalog.FamilyOpenAI,
		"@ai-sdk/openai-compatible":       catalog.FamilyOpenAICompatible,
		"@ai-sdk/ollama":                  catalog.FamilyOllama,
		"@ai-sdk/google":                  catalog.FamilyGoogle,
		"@ai-sdk/google-generative-ai":    catalog.FamilyGoogle,
		"@ai-sdk/google-vertex":           catalog.FamilyGoogleVertex,
		"@ai-sdk/google-vertex/anthropic": catalog.FamilyGoogleVertex,
		// First-party AI SDK packages whose wire dialect is OpenAI
		// Chat Completions. Catalog-driven compat treats them all the
		// same — only base URL + env-var differ.
		"@ai-sdk/groq":                catalog.FamilyOpenAICompatible,
		"@ai-sdk/xai":                 catalog.FamilyOpenAICompatible,
		"@ai-sdk/cerebras":            catalog.FamilyOpenAICompatible,
		"@ai-sdk/togetherai":          catalog.FamilyOpenAICompatible,
		"@ai-sdk/deepinfra":           catalog.FamilyOpenAICompatible,
		"@ai-sdk/perplexity":          catalog.FamilyOpenAICompatible,
		"@ai-sdk/fireworks":           catalog.FamilyOpenAICompatible,
		"@ai-sdk/deepseek":            catalog.FamilyOpenAICompatible,
		"@openrouter/ai-sdk-provider": catalog.FamilyOpenAICompatible,
		"":                            catalog.FamilyUnknown,
		"random-sdk":                  catalog.FamilyUnknown,
	}
	for in, want := range cases {
		if got := catalog.FamilyFromNPM(in); got != want {
			t.Errorf("FamilyFromNPM(%q)=%q want %q", in, got, want)
		}
	}
}

// ---- pricing conversion ----

func TestCostMicrocentsConversion(t *testing.T) {
	c := &catalog.Cost{Input: 5, Output: 25} // claude-opus-4-5
	if got := c.InputMicrocentsPerMTok(); got != 5_000_000_000 {
		t.Errorf("input mc=%d want 5_000_000_000 ($5/MTok)", got)
	}
	if got := c.OutputMicrocentsPerMTok(); got != 25_000_000_000 {
		t.Errorf("output mc=%d want 25_000_000_000 ($25/MTok)", got)
	}
}

func TestCostNilIsZero(t *testing.T) {
	var c *catalog.Cost
	if c.InputMicrocentsPerMTok() != 0 || c.OutputMicrocentsPerMTok() != 0 {
		t.Error("nil Cost must be 0")
	}
}

// ---- credential check ----

func TestHasCredentials(t *testing.T) {
	p := &catalog.Provider{Env: []string{"FAKE_API_KEY", "ALT_KEY"}}
	lookup := func(name string) string {
		if name == "ALT_KEY" {
			return "x"
		}
		return ""
	}
	if !p.HasCredentials(lookup) {
		t.Error("should be true when any env var is set")
	}
	if p.HasCredentials(func(string) string { return "" }) {
		t.Error("should be false when none is set")
	}
	if !((&catalog.Provider{}).HasCredentials(func(string) string { return "" })) {
		t.Error("no env list means local provider, always credentialed")
	}
}

// ---- FindModel ----

func TestFindModel_QualifiedID(t *testing.T) {
	c, _ := catalog.ParseAPIFile([]byte(fixtureAPI))
	p, m := c.FindModel("anthropic/claude-opus-4-5")
	if p == nil || m == nil {
		t.Fatal("FindModel returned nil for qualified id")
	}
	if p.ID != "anthropic" || m.ID != "claude-opus-4-5" {
		t.Errorf("got %s/%s", p.ID, m.ID)
	}
}

func TestFindModel_BareID(t *testing.T) {
	c, _ := catalog.ParseAPIFile([]byte(fixtureAPI))
	p, m := c.FindModel("solar-mini")
	if p == nil || p.ID != "upstage" || m.ID != "solar-mini" {
		t.Errorf("got %+v / %+v", p, m)
	}
}

func TestFindModel_Missing(t *testing.T) {
	c, _ := catalog.ParseAPIFile([]byte(fixtureAPI))
	if p, m := c.FindModel("ghost-model"); p != nil || m != nil {
		t.Error("expected nil for missing model")
	}
	if p, m := c.FindModel(""); p != nil || m != nil {
		t.Error("empty model id should be nil")
	}
}

// ---- merge precedence ----

func TestMerge_LocalOverridesAPI(t *testing.T) {
	api, _ := catalog.ParseAPIFile([]byte(fixtureAPI))
	custom, _ := catalog.ParseAPIFile([]byte(`{
      "anthropic": {
        "id": "anthropic", "name": "Anthropic (custom)",
        "models": {"claude-opus-4-5": {"id":"claude-opus-4-5","cost":{"input":99,"output":99}}}
      },
      "new-provider": {"id":"new-provider","name":"Brand New"}
    }`))
	api.Merge(custom)
	if api.Providers["anthropic"].Name != "Anthropic (custom)" {
		t.Errorf("custom name didn't win: %q", api.Providers["anthropic"].Name)
	}
	// Custom model price wins.
	if api.Providers["anthropic"].Models["claude-opus-4-5"].Cost.Input != 99 {
		t.Errorf("custom price didn't win")
	}
	// API-only models survive (haiku not in custom).
	if _, ok := api.Providers["anthropic"].Models["claude-haiku-4-5"]; !ok {
		t.Error("api-only model lost during merge")
	}
	// New provider added.
	if _, ok := api.Providers["new-provider"]; !ok {
		t.Error("custom-only provider not added")
	}
}

// ---- store roundtrip ----

func TestStore_LoadEmpty(t *testing.T) {
	s := catalog.NewStore(t.TempDir())
	c, err := s.Load()
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(c.Providers) != 0 {
		t.Errorf("expected empty catalog, got %d providers", len(c.Providers))
	}
}

func TestStore_SaveAPIThenLoad(t *testing.T) {
	dir := t.TempDir()
	s := catalog.NewStore(dir)
	if err := s.SaveAPI([]byte(fixtureAPI), "test-fixture"); err != nil {
		t.Fatalf("SaveAPI: %v", err)
	}
	// File exists.
	if _, err := readFile(filepath.Join(dir, catalog.FileAPI)); err != nil {
		t.Fatalf("api.json missing: %v", err)
	}
	// Meta has sync info.
	meta, err := s.LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.APISourceURL != "test-fixture" {
		t.Errorf("meta source=%q", meta.APISourceURL)
	}
	if meta.ProviderCount != 2 {
		t.Errorf("meta provider_count=%d want 2", meta.ProviderCount)
	}
	// Round-trip.
	c, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Providers) != 2 {
		t.Errorf("loaded providers=%d want 2", len(c.Providers))
	}
}

func TestStore_CustomOverridesAPIOnLoad(t *testing.T) {
	dir := t.TempDir()
	s := catalog.NewStore(dir)
	if err := s.SaveAPI([]byte(fixtureAPI), "test"); err != nil {
		t.Fatal(err)
	}
	// Write a custom.json that overrides anthropic's name.
	custom := `{"anthropic":{"id":"anthropic","name":"Custom Name Wins"}}`
	if err := writeFile(filepath.Join(dir, catalog.FileCustom), []byte(custom)); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Load()
	if c.Providers["anthropic"].Name != "Custom Name Wins" {
		t.Errorf("custom didn't override: %q", c.Providers["anthropic"].Name)
	}
	// API models still there (custom didn't list them).
	if len(c.Providers["anthropic"].Models) != 2 {
		t.Errorf("models lost when custom overrode provider: %d", len(c.Providers["anthropic"].Models))
	}
}

// ---- syncer ----

func TestSyncer_FetchesAndParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fixtureAPI))
	}))
	defer srv.Close()

	syncer := catalog.NewSyncer()
	syncer.URL = srv.URL

	raw, cat, res, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !strings.Contains(string(raw), "anthropic") {
		t.Error("raw missing anthropic")
	}
	if res.ProviderCount != 2 {
		t.Errorf("res.ProviderCount=%d", res.ProviderCount)
	}
	if res.ModelCount != 3 {
		t.Errorf("res.ModelCount=%d want 3", res.ModelCount)
	}
	if cat.Providers["upstage"].API == "" {
		t.Error("api field not preserved")
	}
}

func TestSyncer_RejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	syncer := catalog.NewSyncer()
	syncer.URL = srv.URL
	if _, _, _, err := syncer.Sync(context.Background()); err == nil {
		t.Error("expected error for 502")
	}
}

// ---- discovery ----

func TestDiscoverOllama_ParsesTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{
					"name":  "llama3.2:latest",
					"model": "llama3.2:latest",
					"size":  1234567890,
					"details": map[string]any{
						"family":   "llama",
						"families": []string{"llama"},
					},
				},
				{
					"name":  "qwen2.5:7b",
					"model": "qwen2.5:7b",
					"size":  98765432,
					"details": map[string]any{
						"family": "qwen2",
					},
				},
			},
		})
	}))
	defer srv.Close()

	cat, err := catalog.DiscoverOllama(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DiscoverOllama: %v", err)
	}
	p, ok := cat.Providers[catalog.OllamaProviderID]
	if !ok {
		t.Fatal("synthesised provider missing")
	}
	if p.NPM != "@ai-sdk/ollama" {
		t.Errorf("NPM=%q", p.NPM)
	}
	if len(p.Models) != 2 {
		t.Errorf("models=%d", len(p.Models))
	}
	if p.Models["llama3.2:latest"].Cost != nil {
		t.Error("discovered models must have nil cost (local/free)")
	}
}

func TestDiscoverOllama_AbsentReturnsError(t *testing.T) {
	// localhost with no listener → connection refused; should err.
	_, err := catalog.DiscoverOllama(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Error("expected error when ollama is absent")
	}
}

// ---- helpers ----

func readFile(p string) ([]byte, error)     { return os.ReadFile(p) }
func writeFile(p string, data []byte) error { return os.WriteFile(p, data, 0o644) }

func TestModel_SupportsModality(t *testing.T) {
	m := &catalog.Model{
		ID: "m1",
		Modalities: catalog.Modalities{
			Input:  []string{"text", "image"},
			Output: []string{"text"},
		},
	}
	if !m.SupportsModality("input", "image") {
		t.Error("input image should be supported")
	}
	if !m.SupportsModality("input", "IMAGE") { // case-insensitive
		t.Error("modality match should be case-insensitive")
	}
	if m.SupportsModality("output", "image") {
		t.Error("image is not an output modality here")
	}
	if m.SupportsModality("sideways", "text") {
		t.Error("unknown io must return false")
	}
	if !m.SupportsVision() {
		t.Error("a model with image input is vision-capable")
	}
	textOnly := &catalog.Model{ID: "t", Modalities: catalog.Modalities{Input: []string{"text"}}}
	if textOnly.SupportsVision() {
		t.Error("text-only model is not vision-capable")
	}
	// "vision" spelling is also honoured.
	visAlt := &catalog.Model{ID: "v", Modalities: catalog.Modalities{Input: []string{"vision"}}}
	if !visAlt.SupportsVision() {
		t.Error("the 'vision' spelling should count")
	}
}

func TestModel_AgentWarnings(t *testing.T) {
	// Tool-capable, large context → no warnings.
	good := &catalog.Model{ID: "good", ToolCall: true, Limit: catalog.Limit{Context: 200000}}
	if w := good.AgentWarnings(); len(w) != 0 {
		t.Errorf("tool-capable model should have no warnings; got %v", w)
	}
	// No tool-use → warned.
	noTools := &catalog.Model{ID: "noTools", ToolCall: false, Limit: catalog.Limit{Context: 32768}}
	w := noTools.AgentWarnings()
	if len(w) != 1 || !strings.Contains(w[0], "tool-use") {
		t.Errorf("non-tool model should warn about tool-use; got %v", w)
	}
	// No tool-use AND tiny context → two warnings.
	weak := &catalog.Model{ID: "weak", ToolCall: false, Limit: catalog.Limit{Context: 4096}}
	if w := weak.AgentWarnings(); len(w) != 2 {
		t.Errorf("weak model should have 2 warnings; got %v", w)
	}
	// Tool-capable but tiny context → one (context) warning.
	smallCtx := &catalog.Model{ID: "small", ToolCall: true, Limit: catalog.Limit{Context: 2048}}
	w = smallCtx.AgentWarnings()
	if len(w) != 1 || !strings.Contains(w[0], "context") {
		t.Errorf("small-context model should warn about context; got %v", w)
	}
}
