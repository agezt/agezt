// SPDX-License-Identifier: MIT

package vertex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func f64(v float64) *float64 { return &v }

// TestEncodeRequest_ParamsUnset (M997): an unset Params must leave the Gemini
// body free of any sampling field — the default-preserving contract. With no
// maxTokens / JSON mode / thinking budget either, generationConfig is omitted.
func TestEncodeRequest_ParamsUnset(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	body, err := encodeRequest("", msgs, nil, 0, false, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"temperature", "topP", "topK", "stopSequences", "generationConfig"} {
		if strings.Contains(string(body), `"`+k+`"`) {
			t.Fatalf("unset Params leaked %q into Gemini body: %s", k, body)
		}
	}
}

// TestEncodeRequest_ParamsSet (M997): set knobs appear INSIDE generationConfig
// for the Gemini-on-Vertex dialect (nested, not top-level); nil knobs absent.
func TestEncodeRequest_ParamsSet(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	p := agent.Params{
		Temperature: f64(0.2),
		Stop:        []string{"STOP"},
	}
	body, err := encodeRequest("", msgs, nil, 0, false, 0, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		GenerationConfig *struct {
			Temperature   *float64 `json:"temperature"`
			StopSequences []string `json:"stopSequences"`
			TopP          *float64 `json:"topP"`
		} `json:"generationConfig"`
		Temperature *float64 `json:"temperature"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Temperature != nil {
		t.Fatalf("temperature leaked to top level; Gemini nests it: %s", body)
	}
	if got.GenerationConfig == nil {
		t.Fatalf("generationConfig missing: %s", body)
	}
	gc := got.GenerationConfig
	if gc.Temperature == nil || *gc.Temperature != 0.2 {
		t.Fatalf("temperature: got %v", gc.Temperature)
	}
	if len(gc.StopSequences) != 1 || gc.StopSequences[0] != "STOP" {
		t.Fatalf("stopSequences: got %v", gc.StopSequences)
	}
	if gc.TopP != nil {
		t.Fatalf("topP should be absent, got %v", *gc.TopP)
	}
}

// TestEncodeRequest_ProviderOptionsMerge (M997): a ProviderOptions["vertex"]
// object is overlaid onto the Gemini wire body.
func TestEncodeRequest_ProviderOptionsMerge(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	extra := json.RawMessage(`{"labels":{"team":"agezt"}}`)
	body, err := encodeRequest("", msgs, nil, 0, false, 0, agent.Params{}, extra)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Labels["team"] != "agezt" {
		t.Fatalf("provider options not merged: %s", body)
	}
}

// TestEncodeAnthropicOnVertex_Params (M997): the Anthropic-on-Vertex dialect
// places sampling knobs at the TOP level (stop_sequences / temperature), unlike
// the nested Gemini path, and ProviderOptions merge works there too.
func TestEncodeAnthropicOnVertex_Params(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}

	// Unset Params ⇒ no sampling fields.
	body, err := encodeAnthropicOnVertexRequest("", msgs, nil, 1024, 0, false, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"temperature", "top_p", "top_k", "stop_sequences"} {
		if strings.Contains(string(body), `"`+k+`"`) {
			t.Fatalf("unset Params leaked %q into anthropic body: %s", k, body)
		}
	}

	// Set Temperature + Stop ⇒ top-level fields.
	p := agent.Params{Temperature: f64(0.2), Stop: []string{"STOP"}}
	extra := json.RawMessage(`{"metadata":{"user_id":"u1"}}`)
	body, err = encodeAnthropicOnVertexRequest("", msgs, nil, 1024, 0, false, p, extra)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Temperature   *float64 `json:"temperature"`
		StopSequences []string `json:"stop_sequences"`
		Metadata      struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Temperature == nil || *got.Temperature != 0.2 {
		t.Fatalf("temperature: got %v", got.Temperature)
	}
	if len(got.StopSequences) != 1 || got.StopSequences[0] != "STOP" {
		t.Fatalf("stop_sequences: got %v", got.StopSequences)
	}
	if got.Metadata.UserID != "u1" {
		t.Fatalf("provider options not merged: %s", body)
	}
}
