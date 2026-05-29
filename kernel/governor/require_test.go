// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// TestGovernor_TaskRouteRequire_RestrictsChain: when require is
// set, ONLY the listed provider is tried. Even on its failure,
// the chain doesn't fall through to others — that's the hard-pin
// promise.
func TestGovernor_TaskRouteRequire_RestrictsChain(t *testing.T) {
	r := governor.NewRegistry()
	pinned := &fakeProvider{name: "pinned", err: errors.New("upstream down")}
	other := &fakeProvider{name: "other", resp: okResp("ok", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "pinned", Provider: pinned, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "other", Provider: other, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:          r,
		TaskRouteRequires: governor.TaskRouteRequires{"embed": []string{"pinned"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "x",
		TaskType: "embed",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error — pinned provider failed and no fallback allowed")
	}
	if other.calls.Load() != 0 {
		t.Errorf("other was called %d times — hard-pin should have prevented fallback", other.calls.Load())
	}
	if pinned.calls.Load() != 1 {
		t.Errorf("pinned called %d times, want 1", pinned.calls.Load())
	}
}

// TestGovernor_TaskRouteRequire_AllowsSuccess: pinned provider
// succeeds → caller gets the response normally.
func TestGovernor_TaskRouteRequire_AllowsSuccess(t *testing.T) {
	r := governor.NewRegistry()
	pinned := &fakeProvider{name: "pinned", resp: okResp("model-x", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "pinned", Provider: pinned, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:          r,
		TaskRouteRequires: governor.TaskRouteRequires{"embed": []string{"pinned"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "model-x",
		TaskType: "embed",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "from model-x") {
		t.Errorf("unexpected response: %q", resp.Message.Content)
	}
}

// TestGovernor_TaskRouteRequire_NoMatchingTaskType: when the
// task type isn't in requires, normal routing applies.
func TestGovernor_TaskRouteRequire_NoMatchingTaskType(t *testing.T) {
	r := governor.NewRegistry()
	sub := &fakeProvider{name: "sub", resp: okResp("sub-m", 1, 1)}
	api := &fakeProvider{name: "api", resp: okResp("api-m", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "sub", Provider: sub, AuthMode: governor.AuthSubscription},
		&governor.ProviderInfo{Name: "api", Provider: api, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:          r,
		TaskRouteRequires: governor.TaskRouteRequires{"embed": []string{"api"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// "code" task: not in requires → default subscription-first applies.
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "sub-m",
		TaskType: "code",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if sub.calls.Load() != 1 {
		t.Errorf("sub calls = %d, want 1 (default routing, requires didn't match)", sub.calls.Load())
	}
}

// TestGovernor_TaskRouteRequire_UnregisteredProviderFailsClosed:
// if every required provider is unregistered (typo, missing
// creds at startup), the call FAILS rather than silently falling
// through to other registered providers. This is the whole point
// of "require" vs. "prefer."
func TestGovernor_TaskRouteRequire_UnregisteredProviderFailsClosed(t *testing.T) {
	r := governor.NewRegistry()
	other := &fakeProvider{name: "other", resp: okResp("ok", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "other", Provider: other, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:          r,
		TaskRouteRequires: governor.TaskRouteRequires{"embed": []string{"ghost", "phantom"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "x",
		TaskType: "embed",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected fail-closed when all required providers unregistered")
	}
	if other.calls.Load() != 0 {
		t.Errorf("other called %d times — fail-closed semantics violated", other.calls.Load())
	}
}

// TestGovernor_TaskRouteRequire_TakesPrecedenceOverTaskRoutes:
// when both are set for the same task type, requires wins.
func TestGovernor_TaskRouteRequire_TakesPrecedenceOverTaskRoutes(t *testing.T) {
	r := governor.NewRegistry()
	preferred := &fakeProvider{name: "soft", resp: okResp("soft", 1, 1)}
	required := &fakeProvider{name: "hard", resp: okResp("hard", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "soft", Provider: preferred, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "hard", Provider: required, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:          r,
		TaskRoutes:        governor.TaskRoutes{"plan": []string{"soft"}},
		TaskRouteRequires: governor.TaskRouteRequires{"plan": []string{"hard"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "x",
		TaskType: "plan",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "from hard") {
		t.Errorf("unexpected response %q — TaskRouteRequires should have won over TaskRoutes", resp.Message.Content)
	}
	if preferred.calls.Load() != 0 {
		t.Errorf("soft provider called %d times — hard pin should have excluded it", preferred.calls.Load())
	}
}
