// SPDX-License-Identifier: MIT

package bedrock

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func f64(v float64) *float64 { return &v }

// TestAnthropicOnBedrock_ParamsUnset (M997): an unset Params must leave the
// anthropic-on-bedrock body free of any sampling field — the default-preserving
// contract every adapter upholds.
func TestAnthropicOnBedrock_ParamsUnset(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	body, err := encodeAnthropicOnBedrockRequest("", msgs, nil, 100, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"temperature", "top_p", "top_k", "stop_sequences", "thinking"} {
		if strings.Contains(string(body), `"`+k+`"`) {
			t.Fatalf("unset Params leaked %q into body: %s", k, body)
		}
	}
}

// TestAnthropicOnBedrock_ParamsSet (M997): set sampling knobs appear with their
// values; a ReasoningEffort turns on the thinking block.
func TestAnthropicOnBedrock_ParamsSet(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	p := agent.Params{
		Temperature:     f64(0.2),
		TopP:            f64(0.9),
		Stop:            []string{"STOP"},
		ReasoningEffort: "high",
	}
	body, err := encodeAnthropicOnBedrockRequest("", msgs, nil, 4096, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Temperature   *float64 `json:"temperature"`
		TopP          *float64 `json:"top_p"`
		StopSequences []string `json:"stop_sequences"`
		Thinking      *struct {
			Type         string `json:"type"`
			BudgetTokens int    `json:"budget_tokens"`
		} `json:"thinking"`
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
	if got.Thinking == nil || got.Thinking.Type != "enabled" || got.Thinking.BudgetTokens < MinThinkingBudget {
		t.Fatalf("thinking: got %+v", got.Thinking)
	}
}

// TestAnthropicOnBedrock_ProviderOptionsMerge (M997): a ProviderOptions["bedrock"]
// object is overlaid onto the wire body; an unset map changes nothing.
func TestAnthropicOnBedrock_ProviderOptionsMerge(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	extra := json.RawMessage(`{"anthropic_beta":["interleaved-thinking-2025-05-14"]}`)
	body, err := encodeAnthropicOnBedrockRequest("", msgs, nil, 100, agent.Params{}, extra)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		AnthropicBeta []string `json:"anthropic_beta"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.AnthropicBeta) != 1 || got.AnthropicBeta[0] != "interleaved-thinking-2025-05-14" {
		t.Fatalf("provider options not merged: %s", body)
	}
}
