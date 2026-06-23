// SPDX-License-Identifier: MIT

package ollama

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
// of any sampling option — the default-preserving contract. With no MaxTokens
// either, the whole `options` object stays omitted.
func TestEncodeRequest_ParamsUnset(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	body, err := encodeRequest("llama3", "", msgs, nil, 0, false, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"options"`) {
		t.Fatalf("unset Params + no MaxTokens must omit options: %s", body)
	}
	for _, k := range []string{"temperature", "top_p", "top_k", "seed", "stop"} {
		if strings.Contains(string(body), `"`+k+`"`) {
			t.Fatalf("unset Params leaked %q into body: %s", k, body)
		}
	}
}

// TestEncodeRequest_ParamsSet (M997): Ollama nests sampling knobs inside the
// `options` map (alongside num_predict), keyed by Ollama's own option names.
func TestEncodeRequest_ParamsSet(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	p := agent.Params{
		Temperature: f64(0.2),
		TopP:        f64(0.9),
		TopK:        iptr(40),
		Seed:        i64(42),
		Stop:        []string{"STOP"},
	}
	body, err := encodeRequest("llama3", "", msgs, nil, 0, false, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Options struct {
			Temperature *float64 `json:"temperature"`
			TopP        *float64 `json:"top_p"`
			TopK        *int     `json:"top_k"`
			Seed        *int64   `json:"seed"`
			Stop        []string `json:"stop"`
		} `json:"options"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Options.Temperature == nil || *got.Options.Temperature != 0.2 {
		t.Fatalf("options.temperature: got %v", got.Options.Temperature)
	}
	if got.Options.TopP == nil || *got.Options.TopP != 0.9 {
		t.Fatalf("options.top_p: got %v", got.Options.TopP)
	}
	if got.Options.TopK == nil || *got.Options.TopK != 40 {
		t.Fatalf("options.top_k: got %v", got.Options.TopK)
	}
	if got.Options.Seed == nil || *got.Options.Seed != 42 {
		t.Fatalf("options.seed: got %v", got.Options.Seed)
	}
	if len(got.Options.Stop) != 1 || got.Options.Stop[0] != "STOP" {
		t.Fatalf("options.stop: got %v", got.Options.Stop)
	}
	// Sampling knobs must NOT leak to the top level of the request object.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"temperature", "top_p", "top_k", "seed", "stop"} {
		if _, ok := top[k]; ok {
			t.Fatalf("%q must stay inside options, not top-level: %s", k, body)
		}
	}
}

// TestEncodeRequest_ProviderOptionsMerge (M997): a ProviderOptions["ollama"]
// object is overlaid onto the wire body; an unset map changes nothing.
func TestEncodeRequest_ProviderOptionsMerge(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	extra := json.RawMessage(`{"keep_alive":"10m"}`)
	body, err := encodeRequest("llama3", "", msgs, nil, 0, false, agent.Params{}, extra)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		KeepAlive string `json:"keep_alive"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.KeepAlive != "10m" {
		t.Fatalf("provider options not merged: %s", body)
	}
}
