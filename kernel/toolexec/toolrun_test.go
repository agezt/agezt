// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolexec"
)

// fakeTool implements agent.Tool for testing.
type fakeTool struct {
	def    agent.ToolDef
	invoke func(ctx context.Context, input json.RawMessage) (agent.Result, error)
}

func (f *fakeTool) Definition() agent.ToolDef { return f.def }
func (f *fakeTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	return f.invoke(ctx, input)
}

// mockLookup implements toolexec.ToolLookup.
type mockLookup map[string]agent.Tool

func (m mockLookup) LookupTool(name string) (agent.Tool, bool) {
	t, ok := m[name]
	return t, ok
}

// mockPolicy implements toolexec.PolicyChecker with a programmable verdict.
type mockPolicy struct {
	verdict agent.PolicyVerdict
}

func (m *mockPolicy) CheckPolicy(_ context.Context, _ agent.ToolCall) agent.PolicyVerdict {
	return m.verdict
}

// mockEvents implements toolexec.EventPublisher.
type mockEvents struct {
	published []event.Spec
}

func (m *mockEvents) PublishEvent(spec event.Spec) error {
	m.published = append(m.published, spec)
	return nil
}

// mockNoise implements toolexec.NoiseNotifier.
type mockNoise struct {
	calls int
}

func (m *mockNoise) NotifyNoise(_ context.Context, _ agent.ToolCall, _ agent.Result) {
	m.calls++
}

func TestRun_KnownTool_Allowed_Success(t *testing.T) {
	ctx := context.Background()
	tools := mockLookup{
		"greet": &fakeTool{
			def: agent.ToolDef{Name: "greet"},
			invoke: func(_ context.Context, input json.RawMessage) (agent.Result, error) {
				return agent.Result{Output: "hello"}, nil
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
			def: agent.ToolDef{Name: "blocked"},
			invoke: func(_ context.Context, _ json.RawMessage) (agent.Result, error) {
				return agent.Result{}, errors.New("should not be called")
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
}

func TestRun_ToolInvokeError_JournalsAndNoise(t *testing.T) {
	ctx := context.Background()
	tools := mockLookup{
		"flakey": &fakeTool{
			def: agent.ToolDef{Name: "flakey"},
			invoke: func(_ context.Context, _ json.RawMessage) (agent.Result, error) {
				return agent.Result{}, errors.New("internal failure")
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
			def: agent.ToolDef{
				Name:        "strict",
				InputSchema: json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`),
			},
			invoke: func(_ context.Context, _ json.RawMessage) (agent.Result, error) {
				return agent.Result{}, errors.New("should not be called")
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
