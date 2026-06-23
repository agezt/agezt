// SPDX-License-Identifier: MIT

package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }

// TestEncodeRequest_ParamsUnset (M997): an unset Params must leave the body
// free of any sampling field — the default-preserving contract.
func TestEncodeRequest_ParamsUnset(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	body, err := encodeRequest("gpt-4o", "", msgs, nil, 0, false, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"temperature", "top_p", "stop", "seed", "frequency_penalty", "presence_penalty", "reasoning_effort"} {
		if strings.Contains(string(body), `"`+k+`"`) {
			t.Fatalf("unset Params leaked %q into body: %s", k, body)
		}
	}
}

// TestEncodeRequest_ParamsSet (M997): set knobs appear with their values; nil
// knobs stay absent.
func TestEncodeRequest_ParamsSet(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	p := agent.Params{
		Temperature:     f64(0.2),
		TopP:            f64(0.9),
		Seed:            i64(42),
		Stop:            []string{"STOP"},
		ReasoningEffort: "high",
	}
	body, err := encodeRequest("gpt-4o", "", msgs, nil, 0, false, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Temperature      *float64 `json:"temperature"`
		TopP             *float64 `json:"top_p"`
		Seed             *int64   `json:"seed"`
		Stop             []string `json:"stop"`
		ReasoningEffort  string   `json:"reasoning_effort"`
		FrequencyPenalty *float64 `json:"frequency_penalty"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Temperature == nil || *got.Temperature != 0.2 {
		t.Fatalf("temperature: got %v", got.Temperature)
	}
	if got.Seed == nil || *got.Seed != 42 {
		t.Fatalf("seed: got %v", got.Seed)
	}
	if len(got.Stop) != 1 || got.Stop[0] != "STOP" {
		t.Fatalf("stop: got %v", got.Stop)
	}
	if got.ReasoningEffort != "high" {
		t.Fatalf("reasoning_effort: got %q", got.ReasoningEffort)
	}
	if got.FrequencyPenalty != nil {
		t.Fatalf("frequency_penalty should be absent, got %v", *got.FrequencyPenalty)
	}
}

// TestEncodeRequest_ProviderOptionsMerge (M997): a ProviderOptions["openai"]
// object is overlaid onto the wire body; an unset map changes nothing.
func TestEncodeRequest_ProviderOptionsMerge(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	extra := json.RawMessage(`{"logprobs":true,"top_logprobs":5}`)
	body, err := encodeRequest("gpt-4o", "", msgs, nil, 0, false, agent.Params{}, extra)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Logprobs    bool `json:"logprobs"`
		TopLogprobs int  `json:"top_logprobs"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Logprobs || got.TopLogprobs != 5 {
		t.Fatalf("provider options not merged: %s", body)
	}
}
