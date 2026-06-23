// SPDX-License-Identifier: MIT

package cohere

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }
func iptr(v int) *int        { return &v }

// TestEncodeRequest_ParamsUnset (M997): an unset Params must leave the body free
// of any sampling field — the default-preserving contract.
func TestEncodeRequest_ParamsUnset(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	body, err := encodeRequest("command-r", "", msgs, nil, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"temperature", `"p"`, `"k"`, "seed", "stop_sequences", "frequency_penalty", "presence_penalty"} {
		needle := k
		if !strings.HasPrefix(k, `"`) {
			needle = `"` + k + `"`
		}
		if strings.Contains(string(body), needle) {
			t.Fatalf("unset Params leaked %s into body: %s", needle, body)
		}
	}
}

// TestEncodeRequest_ParamsSet (M997): Cohere v2/chat carries sampling knobs at
// the top level, spelling top_p as `p`, top_k as `k`, and the stop list as
// `stop_sequences`.
func TestEncodeRequest_ParamsSet(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	p := agent.Params{
		Temperature:      f64(0.2),
		TopP:             f64(0.9),
		TopK:             iptr(40),
		Seed:             i64(42),
		Stop:             []string{"STOP"},
		FrequencyPenalty: f64(0.5),
	}
	body, err := encodeRequest("command-r", "", msgs, nil, 0, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Temperature      *float64 `json:"temperature"`
		P                *float64 `json:"p"`
		K                *int     `json:"k"`
		Seed             *int64   `json:"seed"`
		StopSequences    []string `json:"stop_sequences"`
		FrequencyPenalty *float64 `json:"frequency_penalty"`
		PresencePenalty  *float64 `json:"presence_penalty"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Temperature == nil || *got.Temperature != 0.2 {
		t.Fatalf("temperature: got %v", got.Temperature)
	}
	if got.P == nil || *got.P != 0.9 {
		t.Fatalf("p (top_p): got %v", got.P)
	}
	if got.K == nil || *got.K != 40 {
		t.Fatalf("k (top_k): got %v", got.K)
	}
	if got.Seed == nil || *got.Seed != 42 {
		t.Fatalf("seed: got %v", got.Seed)
	}
	if len(got.StopSequences) != 1 || got.StopSequences[0] != "STOP" {
		t.Fatalf("stop_sequences: got %v", got.StopSequences)
	}
	if got.FrequencyPenalty == nil || *got.FrequencyPenalty != 0.5 {
		t.Fatalf("frequency_penalty: got %v", got.FrequencyPenalty)
	}
	if got.PresencePenalty != nil {
		t.Fatalf("presence_penalty should be absent, got %v", *got.PresencePenalty)
	}
}

// TestEncodeRequest_ProviderOptionsMerge (M997): a ProviderOptions["cohere"]
// object is overlaid onto the wire body; an unset map changes nothing.
func TestEncodeRequest_ProviderOptionsMerge(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	extra := json.RawMessage(`{"safety_mode":"CONTEXTUAL"}`)
	body, err := encodeRequest("command-r", "", msgs, nil, 0, agent.Params{}, extra)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		SafetyMode string `json:"safety_mode"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.SafetyMode != "CONTEXTUAL" {
		t.Fatalf("provider options not merged: %s", body)
	}
}
