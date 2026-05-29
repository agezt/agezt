// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/kernel/governor"
)

// TestParseTaskRoutesEnv_Basic covers the happy-path of the env
// spec format the daemon uses to plumb AGEZT_TASK_ROUTES into
// Governor.Config.
func TestParseTaskRoutesEnv_Basic(t *testing.T) {
	routes, err := governor.ParseTaskRoutesEnv("plan=anthropic;code=anthropic,openai;embed=ollama")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := routes["plan"]; len(got) != 1 || got[0] != "anthropic" {
		t.Errorf("plan = %v, want [anthropic]", got)
	}
	if got := routes["code"]; len(got) != 2 || got[0] != "anthropic" || got[1] != "openai" {
		t.Errorf("code = %v, want [anthropic openai]", got)
	}
	if got := routes["embed"]; len(got) != 1 || got[0] != "ollama" {
		t.Errorf("embed = %v, want [ollama]", got)
	}
	if len(routes) != 3 {
		t.Errorf("got %d entries, want 3", len(routes))
	}
}

// TestParseTaskRoutesEnv_Whitespace verifies the parser tolerates
// the kind of whitespace operators sprinkle in shell quotes.
func TestParseTaskRoutesEnv_Whitespace(t *testing.T) {
	routes, err := governor.ParseTaskRoutesEnv("  plan = anthropic , openai ;  code= openai  ")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := routes["plan"]; len(got) != 2 || got[0] != "anthropic" || got[1] != "openai" {
		t.Errorf("plan = %v, want [anthropic openai]", got)
	}
	if got := routes["code"]; len(got) != 1 || got[0] != "openai" {
		t.Errorf("code = %v, want [openai]", got)
	}
}

// TestParseTaskRoutesEnv_Empty handles edge cases that should
// produce a nil / empty map, not an error.
func TestParseTaskRoutesEnv_Empty(t *testing.T) {
	cases := []string{"", "   ", ";", "  ;  ;  "}
	for _, c := range cases {
		routes, err := governor.ParseTaskRoutesEnv(c)
		if err != nil {
			t.Errorf("ParseTaskRoutesEnv(%q): %v", c, err)
			continue
		}
		if len(routes) != 0 {
			t.Errorf("ParseTaskRoutesEnv(%q) = %v, want empty", c, routes)
		}
	}
}

// TestParseTaskRoutesEnv_BadEntry verifies the parser returns an
// error for syntactically bad entries (no '='), so operator
// misconfiguration surfaces at daemon startup rather than silently.
func TestParseTaskRoutesEnv_BadEntry(t *testing.T) {
	cases := []string{
		"plan",            // missing '='
		"plan=foo;bare",   // second entry malformed
		"=foo",            // empty key
		"   =foo",         // whitespace-only key
	}
	for _, c := range cases {
		_, err := governor.ParseTaskRoutesEnv(c)
		if err == nil {
			t.Errorf("ParseTaskRoutesEnv(%q): expected error, got nil", c)
		}
	}
}

// TestParseTaskRoutesEnv_EmptyValueDeletes verifies that an entry
// like `plan=` (or `plan=  ,  `) removes any prior route for
// `plan`. Useful for operators overriding a parent-scope env var
// (e.g. systemd drop-ins) without setting a new value.
func TestParseTaskRoutesEnv_EmptyValueDeletes(t *testing.T) {
	routes, err := governor.ParseTaskRoutesEnv("plan=anthropic;plan=")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := routes["plan"]; ok {
		t.Errorf("plan still present after empty-value override: %v", routes)
	}
}

// TestGovernor_TaskRouteHoistsPreferredProvider is the core
// behaviour test: when TaskRoutes names a provider for a given
// TaskType, that provider runs first even if the default
// subscription-first ordering would have put another provider
// ahead.
func TestGovernor_TaskRouteHoistsPreferredProvider(t *testing.T) {
	// Default ordering: subscription before api-key. Without a
	// task route, `sub` would be tried first.
	r := governor.NewRegistry()
	sub := &fakeProvider{name: "sub", resp: okResp("m1", 10, 5)}
	api := &fakeProvider{name: "api", resp: okResp("m2", 10, 5)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "sub", Provider: sub, AuthMode: governor.AuthSubscription},
		&governor.ProviderInfo{Name: "api", Provider: api, AuthMode: governor.AuthAPIKey},
	)

	g, err := governor.New(governor.Config{
		Registry: r,
		TaskRoutes: governor.TaskRoutes{
			"plan": []string{"api"}, // hoist api-key provider for plan
		},
	})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}

	// Plan task → api should win.
	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "m2",
		TaskType: "plan",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("plan Complete: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "from m2") {
		t.Errorf("plan task got %q, expected to come from api/m2", resp.Message.Content)
	}
	if api.calls.Load() != 1 {
		t.Errorf("api calls = %d, want 1", api.calls.Load())
	}
	if sub.calls.Load() != 0 {
		t.Errorf("sub calls = %d, want 0 (task-route should hoist api)", sub.calls.Load())
	}

	// Non-plan task (default behaviour) → sub should win.
	api.calls.Store(0)
	sub.calls.Store(0)
	resp, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model: "m1",
		// No TaskType → default subscription-first ordering.
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("default Complete: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "from m1") {
		t.Errorf("default task got %q, expected sub/m1", resp.Message.Content)
	}
	if sub.calls.Load() != 1 {
		t.Errorf("sub calls = %d, want 1 (default routing)", sub.calls.Load())
	}
}

// TestGovernor_TaskRouteFallsThroughOnFailure verifies that when
// the hoisted preferred provider fails, the chain continues
// through the remaining (un-hoisted) providers in default order —
// i.e. task-routes don't compromise the fallback story.
func TestGovernor_TaskRouteFallsThroughOnFailure(t *testing.T) {
	r := governor.NewRegistry()
	hoisted := &fakeProvider{name: "h", err: errors.New("upstream down")}
	other := &fakeProvider{name: "o", resp: okResp("model-o", 5, 5)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "h", Provider: hoisted, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "o", Provider: other, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:   r,
		TaskRoutes: governor.TaskRoutes{"plan": []string{"h"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "model-o",
		TaskType: "plan",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if hoisted.calls.Load() != 1 {
		t.Errorf("hoisted calls = %d, want 1", hoisted.calls.Load())
	}
	if other.calls.Load() != 1 {
		t.Errorf("other calls = %d, want 1 (fallback after hoisted failed)", other.calls.Load())
	}
	if !strings.Contains(resp.Message.Content, "from model-o") {
		t.Errorf("got %q, want fallback response from 'o'", resp.Message.Content)
	}
}

// TestGovernor_TaskRouteIgnoresUnknownProvider verifies that
// listing a provider that isn't registered degrades gracefully to
// the default chain rather than failing. Operators editing the env
// var won't take down the daemon by typo.
func TestGovernor_TaskRouteIgnoresUnknownProvider(t *testing.T) {
	r := governor.NewRegistry()
	real := &fakeProvider{name: "real", resp: okResp("m", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "real", Provider: real, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:   r,
		TaskRoutes: governor.TaskRoutes{"plan": []string{"ghost", "phantom"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "m",
		TaskType: "plan",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if real.calls.Load() != 1 {
		t.Errorf("real calls = %d, want 1 (default chain after unknown preferred providers)", real.calls.Load())
	}
}

// TestGovernor_TaskRoutePreservesOrderInList verifies that when
// the route lists multiple registered providers, they are tried
// in the order listed (not registry-insertion or auth-tier order).
func TestGovernor_TaskRoutePreservesOrderInList(t *testing.T) {
	r := governor.NewRegistry()
	// 3 providers; subscription-first ordering would put 'sub' first.
	a := &fakeProvider{name: "a", err: errors.New("a down")}
	b := &fakeProvider{name: "b", err: errors.New("b down")}
	sub := &fakeProvider{name: "sub", resp: okResp("ms", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "a", Provider: a, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "b", Provider: b, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "sub", Provider: sub, AuthMode: governor.AuthSubscription},
	)
	g, err := governor.New(governor.Config{
		Registry:   r,
		TaskRoutes: governor.TaskRoutes{"code": []string{"a", "b"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "ms",
		TaskType: "code",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	// Expected: a tried (fails), b tried (fails), sub tried (succeeds).
	if a.calls.Load() != 1 {
		t.Errorf("a calls = %d, want 1", a.calls.Load())
	}
	if b.calls.Load() != 1 {
		t.Errorf("b calls = %d, want 1", b.calls.Load())
	}
	if sub.calls.Load() != 1 {
		t.Errorf("sub calls = %d, want 1", sub.calls.Load())
	}
}

// TestGovernor_TaskRouteOnlyAppliesToMatchingType verifies that
// a route configured for "plan" doesn't bleed into "code" tasks.
func TestGovernor_TaskRouteOnlyAppliesToMatchingType(t *testing.T) {
	r := governor.NewRegistry()
	sub := &fakeProvider{name: "sub", resp: okResp("ms", 1, 1)}
	api := &fakeProvider{name: "api", resp: okResp("ma", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "sub", Provider: sub, AuthMode: governor.AuthSubscription},
		&governor.ProviderInfo{Name: "api", Provider: api, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:   r,
		TaskRoutes: governor.TaskRoutes{"plan": []string{"api"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// "code" is NOT in the routes → default subscription-first applies.
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "ms",
		TaskType: "code",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if sub.calls.Load() != 1 {
		t.Errorf("sub calls = %d, want 1 (code is unrouted; subscription-first wins)", sub.calls.Load())
	}
	if api.calls.Load() != 0 {
		t.Errorf("api calls = %d, want 0 (plan-route should not apply to code)", api.calls.Load())
	}
}
