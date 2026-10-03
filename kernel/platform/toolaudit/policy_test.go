// SPDX-License-Identifier: MIT

package toolaudit_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/platform/toolaudit"
)

// Existing callbacks and verdicts retain exact type identity after the move.
var _ agent.Policy = policyapi.Policy(nil)
var _ policyapi.Policy = agent.Policy(nil)
var _ agent.PolicyVerdict = policyapi.PolicyVerdict{}

func TestPolicyDecisionPayloadPreservesRepresentation(t *testing.T) {
	v := policyapi.PolicyVerdict{
		Allow: true, Capability: "file.read", Reason: "decision reason", WouldAsk: true, HardDenied: true,
		EffectClass: "read_only", AffectedResources: []string{"resource:one", "resource:two"},
		EpistemicAction: "escalate", EpistemicReason: "calibration reason", EpistemicSignals: []string{"temporal_sensitive"},
		EpistemicConfidence: 0.75, FailureMatches: 3, WeightedFailures: 1.25, SchemaHash: "schema-id", InputShape: "shape-id",
		TemporalSensitive: true, NovelTool: true, UntrustedObservation: true, ObservationSources: []string{"web:one"},
		ObservationDirectiveLike: true, ObservationDirectiveMatches: []string{"directive:one"},
	}
	const wantJSON = `{"tool":"probe","call_id":"call","capability":"file.read","allow":true,"reason":"decision reason","would_ask":true,"hard_denied":true,"effect_class":"read_only","affected_resources":["resource:one","resource:two"],"epistemic_action":"escalate","epistemic_reason":"calibration reason","epistemic_signals":["temporal_sensitive"],"epistemic_confidence":0.75,"failure_matches":3,"weighted_failures":1.25,"schema_hash":"schema-id","input_shape":"shape-id","temporal_sensitive":true,"novel_tool":true,"untrusted_observation":true,"observation_sources":["web:one"],"directive_like":true,"directive_matches":["directive:one"]}`
	var want map[string]any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatal(err)
	}
	call := llm.ToolCall{Name: "probe", ID: "call", Input: json.RawMessage(`{"private":"input"}`)}
	got := toolaudit.PolicyDecisionPayload(call, v)
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("got %s; want %s", encoded, wantJSON)
	}
	got["call_id"] = "other"
	if again := toolaudit.PolicyDecisionPayload(call, v); again["call_id"] != "call" {
		t.Fatal("payload maps shared across calls")
	}
}

func TestPolicyDecisionPayloadPreservesZeroValues(t *testing.T) {
	got := toolaudit.PolicyDecisionPayload(llm.ToolCall{}, policyapi.PolicyVerdict{})
	if len(got) != 23 {
		t.Fatalf("fields=%d want 23", len(got))
	}
	for _, key := range []string{"affected_resources", "epistemic_signals", "observation_sources", "directive_matches"} {
		encoded, err := json.Marshal(got[key])
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != "null" {
			t.Errorf("%s=%s want null", key, encoded)
		}
	}
	for _, key := range []string{"allow", "would_ask", "hard_denied", "temporal_sensitive", "novel_tool", "untrusted_observation", "directive_like"} {
		if got[key] != false {
			t.Errorf("%s=%v want false", key, got[key])
		}
	}
}
