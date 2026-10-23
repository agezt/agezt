// SPDX-License-Identifier: MIT

package catalogapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// rowKeys reads a row's own wire keys from its tags — so a test is checked
// against the DECLARATION and not against a copy of it that could drift.
func rowKeys(t *testing.T, v any) map[string]bool {
	t.Helper()
	rt := reflect.TypeOf(v)
	keys := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		name := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		keys[name] = true
	}
	return keys
}

// TestTheProviderWireShape: the catalog is fetched from models.dev and
// UNMARSHALLED DIRECTLY — the field names are models.dev's — and it is
// persisted as api.json. So the wire is a contract of its own: a renamed
// field would silently drop every provider's models, and the catalog would
// come back empty. The exact key set is pinned here.
func TestTheProviderWireShape(t *testing.T) {
	p := Provider{
		ID:   "anthropic",
		Name: "Anthropic",
		Env:  []string{"ANTHROPIC_API_KEY"},
		NPM:  "@ai-sdk/anthropic",
		API:  "https://api.anthropic.com",
		Doc:  "https://docs.anthropic.com",
		Models: map[string]*Model{
			"claude": {
				ID: "claude", Name: "Claude", Family: "anthropic",
				Attachment: true, Reasoning: true, ToolCall: true,
				StrictToolArgs: true, SchemaConstrainedDecoding: true,
				GrammarConstrainedDecoding: true,
				Knowledge:                  "2025-01",
				Release:                    "2025-01-01",
				Modalities:                 Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
				OpenWeight:                 true,
				Limit:                      Limit{Context: 200000, Output: 64000, Input: 200000},
				Cost:                       &Cost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75},
			},
		},
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []string{"id", "name", "env", "npm", "api", "doc", "models"}
	keys := rowKeys(t, Provider{})
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("the provider wire lost the key %q: %s", k, raw)
		}
		if !keys[k] {
			t.Errorf("the key %q is not a key of the provider row", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the provider wire grew a key: %s", raw)
	}
	for k := range got {
		if !keys[k] {
			t.Errorf("the provider wire carries the key %q, which the row does not declare", k)
		}
	}

	// The model's own keys.
	modelRaw, err := json.Marshal(p.Models["claude"])
	if err != nil {
		t.Fatalf("marshal model: %v", err)
	}
	var model map[string]any
	if err := json.Unmarshal(modelRaw, &model); err != nil {
		t.Fatalf("unmarshal model: %v", err)
	}
	modelWant := []string{
		"id", "name", "family", "attachment", "reasoning", "tool_call",
		"strict_tool_args", "schema_constrained_decoding",
		"grammar_constrained_decoding", "knowledge", "release_date",
		"modalities", "open_weights", "limit", "cost",
	}
	modelKeys := rowKeys(t, Model{})
	for _, k := range modelWant {
		if _, ok := model[k]; !ok {
			t.Errorf("the model wire lost the key %q: %s", k, modelRaw)
		}
		if !modelKeys[k] {
			t.Errorf("the key %q is not a key of the model row", k)
		}
	}
	if len(model) != len(modelWant) {
		t.Errorf("the model wire grew a key: %s", modelRaw)
	}
	for k := range model {
		if !modelKeys[k] {
			t.Errorf("the model wire carries the key %q, which the row does not declare", k)
		}
	}
	if model["release_date"] != "2025-01-01" {
		t.Errorf("release_date = %v, want 2025-01-01 (models.dev's key, not release)", model["release_date"])
	}
	if model["knowledge"] != "2025-01" {
		t.Errorf("knowledge = %v, want 2025-01", model["knowledge"])
	}
}

// TestTheMinimalWireKeepsTheAlwaysFields: the fields without omitempty stay
// on the wire even when they are empty.
func TestTheMinimalWireKeepsTheAlwaysFields(t *testing.T) {
	raw, err := json.Marshal(Provider{ID: "ollama", Name: "Ollama"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("the minimal provider wire = %s, want only id and name", raw)
	}

	// A model's cost is a pointer: it is omitted when there is none (free and
	// local models), and the cost's own prices are always on the wire.
	costRaw, err := json.Marshal(&Cost{})
	if err != nil {
		t.Fatalf("marshal cost: %v", err)
	}
	var cost map[string]any
	if err := json.Unmarshal(costRaw, &cost); err != nil {
		t.Fatalf("unmarshal cost: %v", err)
	}
	for _, k := range []string{"input", "output"} {
		if _, ok := cost[k]; !ok {
			t.Errorf("the cost wire lost the key %q: %s", k, costRaw)
		}
	}
	if len(cost) != 2 {
		t.Errorf("the cost wire = %s, want only input and output", costRaw)
	}
}

// TestFamilyFromNPMIsTheMap: the family is what picks the adapter, and an
// unrecognised package must fall to "unknown" rather than guess — a wrong
// guess sends the request down the wrong wire.
func TestFamilyFromNPMIsTheMap(t *testing.T) {
	cases := map[string]Family{
		"@ai-sdk/anthropic":           FamilyAnthropic,
		"@ai-sdk/openai":              FamilyOpenAI,
		"@ai-sdk/openai-compatible":   FamilyOpenAICompatible,
		"@openrouter/ai-sdk-provider": FamilyOpenAICompatible,
		"@ai-sdk/groq":                FamilyOpenAICompatible,
		"@ai-sdk/deepseek":            FamilyOpenAICompatible,
		"@ai-sdk/google":              FamilyGoogle,
		"@ai-sdk/google-vertex":       FamilyGoogleVertex,
		"@ai-sdk/ollama":              FamilyOllama,
		"@ai-sdk/mistral":             FamilyMistral,
		"@ai-sdk/cohere":              FamilyCohere,
		"@ai-sdk/amazon-bedrock":      FamilyAWSBedrock,
		"@ai-sdk/azure":               FamilyAzure,
		"@ai-sdk/something-new":       FamilyUnknown,
		"":                            FamilyUnknown,
		"  @ai-sdk/anthropic  ":       FamilyAnthropic,
		"@AI-SDK/Anthropic":           FamilyAnthropic,
	}
	for npm, want := range cases {
		if got := FamilyFromNPM(npm); got != want {
			t.Errorf("FamilyFromNPM(%q) = %q, want %q", npm, got, want)
		}
	}
	if got := (&Provider{NPM: "@ai-sdk/anthropic"}).Family(); got != FamilyAnthropic {
		t.Errorf("Provider.Family() = %q, want %q", got, FamilyAnthropic)
	}
}

// TestTheNativeJSONModeFamilies: the families that have a native structured
// output switch are a fixed list — a change sends a JSON-mode request down a
// family that ignores it.
func TestTheNativeJSONModeFamilies(t *testing.T) {
	for _, f := range []Family{FamilyOpenAI, FamilyOpenAICompatible, FamilyMistral, FamilyAzure, FamilyGoogle, FamilyGoogleVertex, FamilyOllama} {
		if !FamilySupportsNativeJSONMode(f) {
			t.Errorf("family %q must support the native JSON mode", f)
		}
	}
	for _, f := range []Family{FamilyAnthropic, FamilyCohere, FamilyAWSBedrock, FamilyUnknown} {
		if FamilySupportsNativeJSONMode(f) {
			t.Errorf("family %q must not claim a native JSON mode", f)
		}
	}
}

// TestTheModelHelpers: the model's own rules — the modalities it accepts,
// whether it sees images, whether it caches prompts, whether its tool
// arguments are constrained, and the advisories an operator should see. They
// travel with the row, so they are measured where the row is.
func TestTheModelHelpers(t *testing.T) {
	m := &Model{
		ID:         "claude",
		ToolCall:   true,
		Modalities: Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
		Limit:      Limit{Context: 200000},
		Cost:       &Cost{Input: 3, Output: 15, CacheRead: 0.3},
	}
	if !m.SupportsModality("input", "text") {
		t.Error("the model must accept text input")
	}
	if !m.SupportsModality("INPUT", "IMAGE") {
		t.Error("the modality match must be case-insensitive on both sides")
	}
	if m.SupportsModality("output", "image") {
		t.Error("the model must not accept image output")
	}
	if m.SupportsModality("sideways", "text") {
		t.Error("an unknown io must not match")
	}
	if !m.SupportsVision() {
		t.Error("the model accepts image input")
	}
	// "vision" is the other spelling catalogs use.
	m.Modalities.Input = []string{"vision"}
	if !m.SupportsVision() {
		t.Error("the model accepts 'vision' input")
	}
	if !m.SupportsPromptCache() {
		t.Error("a model with a cache-read price caches prompts")
	}
	m.Cost = nil
	if m.SupportsPromptCache() {
		t.Error("a model with no cost block does not cache prompts")
	}

	// The tool-argument constraint is advertised, never assumed: a plain
	// tool-use model does not imply schema enforcement.
	if m.SupportsStrictToolArgs() {
		t.Error("a model that advertises no constraint must not claim one")
	}
	m.StrictToolArgs = true
	if !m.SupportsStrictToolArgs() {
		t.Error("a model that advertises strict tool args must claim it")
	}
	m.StrictToolArgs = false
	m.SchemaConstrainedDecoding = true
	if !m.SupportsStrictToolArgs() {
		t.Error("schema-constrained decoding is a constraint too")
	}
	// A model that cannot call tools cannot be constrained either.
	m.SchemaConstrainedDecoding = false
	m.ToolCall = false
	if m.SupportsStrictToolArgs() {
		t.Error("a model without tool use cannot claim constrained tool args")
	}

	// The advisories: the tool-use headline and the small-context warning.
	warnings := m.AgentWarnings()
	if len(warnings) == 0 {
		t.Error("a model without tool use must carry the headline advisory")
	}
	m.ToolCall = true
	m.Limit.Context = 4096
	warnings = m.AgentWarnings()
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "context window") {
			found = true
		}
	}
	if !found {
		t.Errorf("a small context window must be surfaced: %v", warnings)
	}
	m.Limit.Context = 200000
	if got := m.AgentWarnings(); len(got) != 0 {
		t.Errorf("a healthy model must carry no advisories: %v", got)
	}
}

// TestTheCredentialNames: the vault key shape is read by the credential
// store and by the governor, so the rules are measured here — the scoped key
// first, the bare env var second, and nothing for an empty env.
func TestTheCredentialNames(t *testing.T) {
	if got := ProviderCredentialName("anthropic", "ANTHROPIC_API_KEY"); got != "provider:anthropic:ANTHROPIC_API_KEY" {
		t.Errorf("ProviderCredentialName = %q", got)
	}
	if got := ProviderCredentialName("", "ANTHROPIC_API_KEY"); got != "ANTHROPIC_API_KEY" {
		t.Errorf("a provider-less credential name = %q, want the bare env var", got)
	}
	if got := ProviderCredentialName("anthropic", ""); got != "" {
		t.Errorf("an empty env = %q, want empty", got)
	}
	if !IsProviderCredentialName("provider:anthropic:ANTHROPIC_API_KEY") {
		t.Error("a scoped key must be recognized")
	}
	if IsProviderCredentialName("ANTHROPIC_API_KEY") {
		t.Error("a bare env var must not be recognized as a scoped key")
	}

	names := ProviderCredentialLookupNames("anthropic", "ANTHROPIC_API_KEY")
	if len(names) != 2 || names[0] != "provider:anthropic:ANTHROPIC_API_KEY" || names[1] != "ANTHROPIC_API_KEY" {
		t.Errorf("the lookup order = %v, want the scoped key first and the bare env var second", names)
	}
	if got := ProviderCredentialLookupNames("anthropic", ""); got != nil {
		t.Errorf("an empty env must look up nothing, got %v", got)
	}
}

// TestTheCostConversion: the price conversion is a unit rule (1 USD = 10^9
// microcents) and the billing reads it.
func TestTheCostConversion(t *testing.T) {
	c := &Cost{Input: 5, Output: 15, CacheRead: 0.5, CacheWrite: 6.25}
	if got := c.InputMicrocentsPerMTok(); got != 5_000_000_000 {
		t.Errorf("InputMicrocentsPerMTok = %d, want 5000000000", got)
	}
	if got := c.OutputMicrocentsPerMTok(); got != 15_000_000_000 {
		t.Errorf("OutputMicrocentsPerMTok = %d, want 15000000000", got)
	}
	if got := c.CacheReadMicrocentsPerMTok(); got != 500_000_000 {
		t.Errorf("CacheReadMicrocentsPerMTok = %d, want 500000000", got)
	}
	if got := c.CacheWriteMicrocentsPerMTok(); got != 6_250_000_000 {
		t.Errorf("CacheWriteMicrocentsPerMTok = %d, want 6250000000", got)
	}
	var nilCost *Cost
	if nilCost.InputMicrocentsPerMTok() != 0 {
		t.Error("a nil cost must convert to zero")
	}
}
