// SPDX-License-Identifier: MIT

package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func f64(v float64) *float64 { return &v }

// TestEncodeRequest_ParamsUnset (M997): an unset Params must leave the body
// free of any sampling field — the default-preserving contract. With no
// maxTokens / JSON mode / thinking budget either, generationConfig is omitted
// entirely.
func TestEncodeRequest_ParamsUnset(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	body, err := encodeRequest("", msgs, nil, 0, false, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"temperature", "topP", "topK", "stopSequences", "generationConfig"} {
		if strings.Contains(string(body), `"`+k+`"`) {
			t.Fatalf("unset Params leaked %q into body: %s", k, body)
		}
	}
}

// TestEncodeRequest_ParamsSet (M997): set knobs appear INSIDE generationConfig
// (Gemini nests them, unlike OpenAI's top-level placement); nil knobs stay
// absent. Gemini has no seed / penalties, so those Params fields are dropped.
func TestEncodeRequest_ParamsSet(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	p := agent.Params{
		Temperature: f64(0.2),
		TopP:        f64(0.9),
		Stop:        []string{"STOP"},
	}
	body, err := encodeRequest("", msgs, nil, 0, false, 0, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		GenerationConfig *struct {
			Temperature   *float64 `json:"temperature"`
			TopP          *float64 `json:"topP"`
			TopK          *int     `json:"topK"`
			StopSequences []string `json:"stopSequences"`
		} `json:"generationConfig"`
		// Sampling knobs must NOT be promoted to the top level.
		Temperature *float64 `json:"temperature"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Temperature != nil {
		t.Fatalf("temperature leaked to top level; Gemini nests it in generationConfig: %s", body)
	}
	if got.GenerationConfig == nil {
		t.Fatalf("generationConfig missing: %s", body)
	}
	gc := got.GenerationConfig
	if gc.Temperature == nil || *gc.Temperature != 0.2 {
		t.Fatalf("temperature: got %v", gc.Temperature)
	}
	if gc.TopP == nil || *gc.TopP != 0.9 {
		t.Fatalf("topP: got %v", gc.TopP)
	}
	if len(gc.StopSequences) != 1 || gc.StopSequences[0] != "STOP" {
		t.Fatalf("stopSequences: got %v", gc.StopSequences)
	}
	if gc.TopK != nil {
		t.Fatalf("topK should be absent, got %v", *gc.TopK)
	}
}

// TestEncodeRequest_ProviderOptionsMerge (M997): a ProviderOptions["google"]
// object is overlaid onto the wire body; an unset map changes nothing.
func TestEncodeRequest_ProviderOptionsMerge(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	extra := json.RawMessage(`{"cachedContent":"projects/p/locations/l/cachedContents/c"}`)
	body, err := encodeRequest("", msgs, nil, 0, false, 0, agent.Params{}, extra)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		CachedContent string `json:"cachedContent"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.CachedContent == "" {
		t.Fatalf("provider options not merged: %s", body)
	}
}
