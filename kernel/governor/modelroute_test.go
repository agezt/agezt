// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// Per-request model routing (SPEC-15 §1): a request naming a model is routed to
// the provider that serves it, even when that provider is not the primary.
func TestModelRoute_HoistsServingProvider(t *testing.T) {
	b, _ := newBus(t)
	r := governor.NewRegistry()
	alpha := &fakeProvider{name: "alpha", resp: okResp("model-a", 1, 1)}
	beta := &fakeProvider{name: "beta", resp: okResp("model-b", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "alpha", Provider: alpha, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
		&governor.ProviderInfo{Name: "beta", Provider: beta, AuthMode: governor.AuthAPIKey, Models: []string{"model-b"}},
	)
	g, err := governor.New(governor.Config{Registry: r, Bus: b})
	if err != nil {
		t.Fatal(err)
	}

	// A request for model-b must hit beta (the provider that serves it), not the
	// primary alpha.
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "model-b"}); err != nil {
		t.Fatal(err)
	}
	if beta.calls.Load() != 1 || alpha.calls.Load() != 0 {
		t.Errorf("model-b routing: alpha=%d beta=%d (want 0,1)", alpha.calls.Load(), beta.calls.Load())
	}

	// A request for model-a hits alpha (also the primary).
	alpha.calls.Store(0)
	beta.calls.Store(0)
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "model-a"}); err != nil {
		t.Fatal(err)
	}
	if alpha.calls.Load() != 1 || beta.calls.Load() != 0 {
		t.Errorf("model-a routing: alpha=%d beta=%d (want 1,0)", alpha.calls.Load(), beta.calls.Load())
	}
}

func TestModelRoute_UnknownModelKeepsDefaultOrder(t *testing.T) {
	b, _ := newBus(t)
	r := governor.NewRegistry()
	alpha := &fakeProvider{name: "alpha", resp: okResp("model-a", 1, 1)}
	beta := &fakeProvider{name: "beta", resp: okResp("model-b", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "alpha", Provider: alpha, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
		&governor.ProviderInfo{Name: "beta", Provider: beta, AuthMode: governor.AuthAPIKey, Models: []string{"model-b"}},
	)
	g, _ := governor.New(governor.Config{Registry: r, Bus: b})

	// A model no provider declares → default order (primary alpha runs).
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "mystery-model"}); err != nil {
		t.Fatal(err)
	}
	if alpha.calls.Load() != 1 {
		t.Errorf("unknown model should run on primary alpha, got alpha=%d beta=%d", alpha.calls.Load(), beta.calls.Load())
	}
}

func TestProviderInfo_Serves(t *testing.T) {
	p := &governor.ProviderInfo{Models: []string{"gpt-4o", "gpt-4o-mini"}}
	if !p.Serves("gpt-4o") || !p.Serves("gpt-4o-mini") {
		t.Error("Serves should be true for listed models")
	}
	if p.Serves("claude") || p.Serves("") {
		t.Error("Serves should be false for unlisted/empty")
	}
}
