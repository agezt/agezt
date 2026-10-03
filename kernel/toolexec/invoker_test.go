// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolexec"
)

func TestInvokerPortPreservesPipeline(t *testing.T) {
	for name, newInvoker := range map[string]toolexec.Factory{"compatibility": toolexec.NewInvoker, "app": apptools.NewInvoker} {
		for _, mode := range []string{"registry", "local", "deny", "invoke-error", "audit-error"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				full := strings.Repeat("x", 512)
				calls := 0
				cause := errors.New("typed invoke cause")
				auditCause := errors.New("audit unavailable")
				tool := &fakeTool{def: toolapi.ToolDef{Name: "probe", Capability: toolapi.ToolCapability{Name: "introspect"}}, invoke: func(ctx context.Context, args json.RawMessage) (toolapi.Result, error) {
					calls++
					if toolapi.CorrelationFromContext(ctx) != "corr" || string(args) != `{"x":1}` {
						t.Error("context/input changed")
					}
					if mode == "invoke-error" {
						return toolapi.Result{}, cause
					}
					return toolapi.Result{Output: full}, nil
				}}
				lookup := mockLookup{"probe": tool}
				policy := &mockPolicy{verdict: policyapi.PolicyVerdict{Allow: mode != "deny", Reason: "denied", Capability: "introspect"}}
				events := &mockEvents{}
				noise := &fullOutputNoise{}
				store := &offloadStore{}
				call := toolapi.Invocation{CorrelationID: "corr", CallID: "call", Name: "probe", Input: json.RawMessage(`{"x":1}`), Artifacts: store, ArtifactThreshold: 64}
				if mode == "local" {
					call.Lookup = lookup
					lookup = mockLookup{}
				}
				if mode == "audit-error" {
					events.failKind = event.KindToolResult
					events.failure = auditCause
				}
				invoker := newInvoker(toolexec.Dependencies{Tools: lookup, Policy: policy, Events: events, Noise: noise})
				res, err := invoker.Invoke(context.Background(), call)
				switch mode {
				case "deny":
					if err == nil || calls != 0 || len(events.published) != 2 || noise.calls != 0 {
						t.Fatalf("denial err=%v calls=%d events=%d noise=%d", err, calls, len(events.published), noise.calls)
					}
				case "invoke-error":
					if !errors.Is(err, cause) || calls != 1 {
						t.Fatalf("typed cause lost: %v", err)
					}
				case "audit-error":
					if !errors.Is(err, auditCause) || calls != 1 {
						t.Fatalf("audit cause lost: %v", err)
					}
				default:
					if err != nil || res.Output != full || calls != 1 || noise.calls != 1 || noise.result.Output != full {
						t.Fatalf("result=%+v err=%v calls=%d noise=%+v", res, err, calls, noise)
					}
					payload := events.published[2].Payload.(map[string]any)
					if payload["raw_ref"] != "blob" || payload["output_bytes"] != len(full) || payload["call_id"] != "call" || events.published[2].CorrelationID != "corr" || string(store.data) != full {
						t.Errorf("terminal=%v artifact bytes=%d", payload, len(store.data))
					}
					call.ArtifactThreshold = len(full)
					call.CallID = "second"
					if _, err := invoker.Invoke(context.Background(), call); err != nil {
						t.Fatal(err)
					}
					payload = events.published[5].Payload.(map[string]any)
					if payload["output"] != full || payload["raw_ref"] != nil || store.calls != 1 || payload["call_id"] != "second" {
						t.Errorf("per-call threshold/identity lost: %v puts=%d", payload, store.calls)
					}
				}
				if policy.definition.Name != "probe" || policy.correlation != "corr" {
					t.Errorf("policy metadata=%+v correlation=%s", policy.definition, policy.correlation)
				}
			})
		}
	}
}

type discardAudit struct{}

func (discardAudit) PublishEvent(event.Spec) error { return nil }

// This measures pipeline/port overhead with mock policy and no journal I/O,
// not the runtime policy engine, real tools or durable storage latency.
func BenchmarkInvokerPort(b *testing.B) {
	for name, newInvoker := range map[string]toolexec.Factory{"compatibility": toolexec.NewInvoker, "app": apptools.NewInvoker} {
		b.Run(name, func(b *testing.B) {
			tool := &fakeTool{def: toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
				return toolapi.Result{Output: "ok"}, nil
			}}
			s := newInvoker(toolexec.Dependencies{Tools: mockLookup{"probe": tool}, Policy: &mockPolicy{verdict: policyapi.PolicyVerdict{Allow: true}}, Events: discardAudit{}, Noise: &mockNoise{}})
			call := toolapi.Invocation{Name: "probe", CallID: "call", CorrelationID: "corr", Input: json.RawMessage(`{}`)}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.Invoke(ctx, call); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
