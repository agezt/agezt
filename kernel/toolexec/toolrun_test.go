// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/policyctx"
	"github.com/agezt/agezt/kernel/toolexec"
)

// fakeTool implements toolapi.Tool for testing.
type fakeTool struct {
	def    toolapi.ToolDef
	invoke func(ctx context.Context, input json.RawMessage) (toolapi.Result, error)
}

func TestRun_TerminalAuditFailurePreservesCause(t *testing.T) {
	for _, denied := range []bool{false, true} {
		name := "invoke-error"
		if denied {
			name = "deny"
		}
		t.Run(name, func(t *testing.T) {
			calls := 0
			invokeFailure := errors.New("invoke failure")
			auditFailure := errors.New("result audit unavailable")
			tool := &fakeTool{def: toolapi.ToolDef{Name: "probe"}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
				calls++
				return toolapi.Result{}, invokeFailure
			}}
			_, err := toolexec.Run(context.Background(), "corr", "call", "probe", json.RawMessage(`{}`), mockLookup{"probe": tool}, &mockPolicy{verdict: agent.PolicyVerdict{Allow: !denied, Reason: "test denial"}}, &mockEvents{failKind: event.KindToolResult, failure: auditFailure}, &mockNoise{})
			if !errors.Is(err, auditFailure) {
				t.Fatalf("audit error lost: %v", err)
			}
			if denied {
				if calls != 0 || !strings.Contains(err.Error(), "refused") {
					t.Fatalf("denial lost: calls=%d error=%v", calls, err)
				}
			} else if calls != 1 || !errors.Is(err, invokeFailure) {
				t.Fatalf("invoke error lost: calls=%d error=%v", calls, err)
			}
		})
	}
}

func (f *fakeTool) Definition() toolapi.ToolDef { return f.def }
func (f *fakeTool) Invoke(ctx context.Context, input json.RawMessage) (toolapi.Result, error) {
	return f.invoke(ctx, input)
}

// mockLookup implements toolexec.ToolLookup.
type mockLookup map[string]toolapi.Tool

func (m mockLookup) LookupTool(name string) (toolapi.Tool, bool) {
	t, ok := m[name]
	return t, ok
}

// mockPolicy implements toolexec.PolicyChecker with a programmable verdict.
type mockPolicy struct {
	verdict     agent.PolicyVerdict
	definition  toolapi.ToolDef
	correlation string
}

func (m *mockPolicy) CheckPolicy(ctx context.Context, _ llm.ToolCall) agent.PolicyVerdict {
	m.definition, _ = agent.PolicyToolDefFromContext(ctx)
	m.correlation = toolapi.CorrelationFromContext(ctx)
	return m.verdict
}

// mockEvents implements toolexec.EventPublisher.
type mockEvents struct {
	published []event.Spec
	failKind  event.Kind
	failure   error
}

func (m *mockEvents) PublishEvent(spec event.Spec) error {
	if spec.Kind == m.failKind {
		return m.failure
	}
	m.published = append(m.published, spec)
	return nil
}

func TestRun_UsesResolvedPolicyDefinition(t *testing.T) {
	def := toolapi.ToolDef{Name: "greet", Capability: toolapi.ToolCapability{Name: "introspect"}}
	tool := &fakeTool{def: def, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
		return toolapi.Result{Output: "ok"}, nil
	}}
	ctx := policyctx.WithPolicyToolDef(context.Background(), toolapi.ToolDef{Name: "caller-spoof", Capability: toolapi.ToolCapability{Name: "provider.call"}})
	policy := &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}
	if _, err := toolexec.Run(ctx, "corr", "call", "greet", json.RawMessage(`{}`), mockLookup{"greet": tool}, policy, &mockEvents{}, &mockNoise{}); err != nil {
		t.Fatal(err)
	}
	if policy.definition.Name != def.Name || policy.definition.Capability.Name != def.Capability.Name || policy.correlation != "corr" {
		t.Fatalf("policy definition=%+v correlation=%q", policy.definition, policy.correlation)
	}
}

func TestRun_AuditFailure(t *testing.T) {
	for _, kind := range []event.Kind{event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult} {
		t.Run(string(kind), func(t *testing.T) {
			calls := 0
			tool := &fakeTool{def: toolapi.ToolDef{Name: "greet"}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
				calls++
				return toolapi.Result{Output: "ok"}, nil
			}}
			failure := errors.New("audit unavailable")
			_, err := toolexec.Run(context.Background(), "corr", "call", "greet", json.RawMessage(`{}`), mockLookup{"greet": tool}, &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}, &mockEvents{failKind: kind, failure: failure}, &mockNoise{})
			if !errors.Is(err, failure) {
				t.Fatalf("error=%v want audit failure", err)
			}
			wantCalls := 0
			if kind == event.KindToolResult {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want %d", calls, wantCalls)
			}
		})
	}
}

func TestRun_PanicIsAudited(t *testing.T) {
	tool := &fakeTool{def: toolapi.ToolDef{Name: "panic"}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) { panic("tool failure") }}
	events := &mockEvents{}
	noise := &mockNoise{}
	_, err := toolexec.Run(context.Background(), "corr", "call", "panic", json.RawMessage(`{}`), mockLookup{"panic": tool}, &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}, events, noise)
	if err == nil || len(events.published) != 3 || events.published[2].Kind != event.KindToolResult || noise.calls != 1 {
		t.Fatalf("error=%v events=%v noise=%d", err, events.published, noise.calls)
	}
	if p := events.published[2].Payload.(map[string]any); p["error"] != true {
		t.Fatalf("result=%v", p)
	}
}

// mockNoise implements toolexec.NoiseNotifier.
type mockNoise struct {
	calls int
}

func (m *mockNoise) NotifyNoise(_ context.Context, _ llm.ToolCall, _ toolapi.Result) {
	m.calls++
}

func TestRun_KnownTool_Allowed_Success(t *testing.T) {
	ctx := context.Background()
	tools := mockLookup{
		"greet": &fakeTool{
			def: toolapi.ToolDef{Name: "greet"},
			invoke: func(_ context.Context, input json.RawMessage) (toolapi.Result, error) {
				return toolapi.Result{Output: "hello"}, nil
			},
		},
	}
	policy := &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}
	events := &mockEvents{}
	noise := &mockNoise{}

	result, err := toolexec.Run(ctx, "corr-1", "call-1", "greet", json.RawMessage(`{}`), tools, policy, events, noise)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Output != "hello" {
		t.Fatalf("got output %q, want %q", result.Output, "hello")
	}
	// Should have published: policy decision + tool.invoked + tool.result = 3 events.
	if len(events.published) != 3 {
		t.Fatalf("got %d events, want 3", len(events.published))
	}
	if noise.calls != 1 {
		t.Fatalf("noise notifier called %d times, want 1", noise.calls)
	}
}

func TestRun_UnknownTool_Error(t *testing.T) {
	ctx := context.Background()
	tools := mockLookup{}
	policy := &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}
	events := &mockEvents{}
	noise := &mockNoise{}

	_, err := toolexec.Run(ctx, "corr-2", "call-2", "nonexistent", json.RawMessage(`{}`), tools, policy, events, noise)
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if len(events.published) != 0 {
		t.Fatalf("expected 0 events for unknown tool, got %d", len(events.published))
	}
}

func TestRun_DeniedByPolicy_Error(t *testing.T) {
	ctx := context.Background()
	tools := mockLookup{
		"blocked": &fakeTool{
			def: toolapi.ToolDef{Name: "blocked"},
			invoke: func(_ context.Context, _ json.RawMessage) (toolapi.Result, error) {
				return toolapi.Result{}, errors.New("should not be called")
			},
		},
	}
	policy := &mockPolicy{verdict: agent.PolicyVerdict{Allow: false, Reason: "test denial"}}
	events := &mockEvents{}
	noise := &mockNoise{}

	_, err := toolexec.Run(ctx, "corr-3", "call-3", "blocked", json.RawMessage(`{}`), tools, policy, events, noise)
	if err == nil {
		t.Fatal("expected policy denial error, got nil")
	}
	if noise.calls != 0 {
		t.Fatalf("noise notifier should not be called on denial, got %d calls", noise.calls)
	}
	if len(events.published) != 2 || events.published[0].Kind != event.KindPolicyDecision || events.published[1].Kind != event.KindToolResult {
		t.Fatalf("denial audit=%v; want policy.decision then tool.result", events.published)
	}
}

func TestRun_ToolInvokeError_JournalsAndNoise(t *testing.T) {
	ctx := context.Background()
	tools := mockLookup{
		"flakey": &fakeTool{
			def: toolapi.ToolDef{Name: "flakey"},
			invoke: func(_ context.Context, _ json.RawMessage) (toolapi.Result, error) {
				return toolapi.Result{}, errors.New("internal failure")
			},
		},
	}
	policy := &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}
	events := &mockEvents{}
	noise := &mockNoise{}

	_, err := toolexec.Run(ctx, "corr-4", "call-4", "flakey", json.RawMessage(`{}`), tools, policy, events, noise)
	if err == nil {
		t.Fatal("expected tool error, got nil")
	}
	// Despite the error, the error result event + noise notification should fire.
	if len(events.published) != 3 {
		t.Fatalf("expected 3 events on error (policy + invoked + result), got %d", len(events.published))
	}
	if noise.calls != 1 {
		t.Fatalf("expected 1 noise notification on error, got %d", noise.calls)
	}
}

func TestRun_InputSchemaRejection(t *testing.T) {
	ctx := context.Background()
	tools := mockLookup{
		"strict": &fakeTool{
			def: toolapi.ToolDef{
				Name:        "strict",
				InputSchema: json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`),
			},
			invoke: func(_ context.Context, _ json.RawMessage) (toolapi.Result, error) {
				return toolapi.Result{}, errors.New("should not be called")
			},
		},
	}
	policy := &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}
	events := &mockEvents{}
	noise := &mockNoise{}

	// Missing required "name" field.
	_, err := toolexec.Run(ctx, "corr-5", "call-5", "strict", json.RawMessage(`{}`), tools, policy, events, noise)
	if err == nil {
		t.Fatal("expected schema rejection error, got nil")
	}
	if len(events.published) != 0 {
		t.Fatalf("expected 0 events for schema rejection, got %d", len(events.published))
	}
}
