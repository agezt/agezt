// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/toolexec"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestPolicyAuditMatchesLoop(t *testing.T) {
	for _, allow := range []bool{true, false} {
		name := "deny"
		if allow {
			name = "allow"
		}
		t.Run(name, func(t *testing.T) {
			verdict := agent.PolicyVerdict{
				Allow: allow, Capability: "file.read", Reason: "decision reason", WouldAsk: true, HardDenied: !allow,
				EffectClass: "read_only", AffectedResources: []string{"resource:one", "resource:two"},
				EpistemicAction: "escalate", EpistemicReason: "calibration reason", EpistemicSignals: []string{"temporal_sensitive"},
				EpistemicConfidence: 0.75, FailureMatches: 3, WeightedFailures: 1.25, SchemaHash: "schema-id", InputShape: "shape-id",
				TemporalSensitive: true, NovelTool: true, UntrustedObservation: true, ObservationSources: []string{"web:one"},
				ObservationDirectiveLike: true, ObservationDirectiveMatches: []string{"directive:one"},
			}
			want := map[string]any{
				"tool": "probe", "call_id": "call", "capability": "file.read", "allow": allow, "reason": "decision reason", "would_ask": true, "hard_denied": !allow,
				"effect_class": "read_only", "affected_resources": []string{"resource:one", "resource:two"},
				"epistemic_action": "escalate", "epistemic_reason": "calibration reason", "epistemic_signals": []string{"temporal_sensitive"},
				"epistemic_confidence": 0.75, "failure_matches": 3, "weighted_failures": 1.25, "schema_hash": "schema-id", "input_shape": "shape-id",
				"temporal_sensitive": true, "novel_tool": true, "untrusted_observation": true, "observation_sources": []string{"web:one"}, "directive_like": true, "directive_matches": []string{"directive:one"},
			}
			wantJSON, _ := json.Marshal(want)
			var expected map[string]any
			if err := json.Unmarshal(wantJSON, &expected); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"loop", "direct", "options"} {
				t.Run(path, func(t *testing.T) {
					calls := 0
					tool := &fakeTool{def: toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
						calls++
						return toolapi.Result{Output: "ok"}, nil
					}}
					var payloads []json.RawMessage
					if path == "loop" {
						j, err := journal.Open(t.TempDir(), journal.Options{})
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { j.Close() })
						b := bus.New(j)
						t.Cleanup(b.Close)
						_, err = agent.Run(context.Background(), agent.LoopConfig{Bus: b, Actor: "actor", CorrelationID: "corr", Tools: map[string]toolapi.Tool{"probe": tool},
							Provider: mock.New(llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{Name: "probe", ID: "call", Input: json.RawMessage(`{}`)}}}, StopReason: llm.StopToolUse}, mock.FinalText("done")),
							Policy:   func(context.Context, llm.ToolCall) agent.PolicyVerdict { return verdict },
						}, "run")
						if err != nil {
							t.Fatal(err)
						}
						if err := j.Range(func(e *event.Event) error {
							if e.Kind == event.KindPolicyDecision && e.CorrelationID == "corr" {
								payloads = append(payloads, e.Payload)
							}
							return nil
						}); err != nil {
							t.Fatal(err)
						}
					} else {
						events := &mockEvents{}
						var err error
						if path == "direct" {
							_, err = toolexec.Run(context.Background(), "corr", "call", "probe", json.RawMessage(`{}`), mockLookup{"probe": tool}, &mockPolicy{verdict: verdict}, events, &mockNoise{})
						} else {
							_, err = toolexec.RunWithOptions(context.Background(), "corr", "call", "probe", json.RawMessage(`{}`), mockLookup{"probe": tool}, &mockPolicy{verdict: verdict}, events, &mockNoise{}, toolexec.Options{})
						}
						if (err == nil) != allow {
							t.Fatalf("allow=%v error=%v", allow, err)
						}
						for _, e := range events.published {
							if e.Kind == event.KindPolicyDecision {
								if e.CorrelationID != "corr" || e.Actor != "policy" || e.Subject != "policy" {
									t.Errorf("event identity=%+v", e)
								}
								p, err := json.Marshal(e.Payload)
								if err != nil {
									t.Fatal(err)
								}
								payloads = append(payloads, p)
							}
						}
					}
					if len(payloads) != 1 {
						t.Fatalf("policy records=%d", len(payloads))
					}
					var got map[string]any
					if err := json.Unmarshal(payloads[0], &got); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, expected) {
						t.Errorf("policy payload mismatch: got %s; want %s", payloads[0], wantJSON)
					}
					wantCalls := 0
					if allow {
						wantCalls = 1
					}
					if calls != wantCalls {
						t.Errorf("calls=%d want %d", calls, wantCalls)
					}
				})
			}
		})
	}
}

func TestRunPolicyAuditIncludesZeroDetails(t *testing.T) {
	events := &mockEvents{}
	tool := &fakeTool{def: toolapi.ToolDef{Name: "probe"}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
		return toolapi.Result{Output: "ok"}, nil
	}}
	_, err := toolexec.Run(context.Background(), "corr", "call", "probe", json.RawMessage(`{}`), mockLookup{"probe": tool}, &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}, events, &mockNoise{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.published) != 3 || events.published[0].Kind != event.KindPolicyDecision {
		t.Fatalf("events=%v", events.published)
	}
	p := events.published[0].Payload.(map[string]any)
	if len(p) != 23 {
		t.Fatalf("fields=%d want 23", len(p))
	}
	for _, key := range []string{"affected_resources", "epistemic_signals", "observation_sources", "directive_matches"} {
		data, err := json.Marshal(p[key])
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "null" {
			t.Errorf("%s=%s", key, data)
		}
	}
	for _, key := range []string{"temporal_sensitive", "novel_tool", "untrusted_observation", "directive_like"} {
		if p[key] != false {
			t.Errorf("%s=%v", key, p[key])
		}
	}
}
