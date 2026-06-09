// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// modelAwareProvider succeeds only for models in `ok`; everything else errors,
// letting a test simulate a model being "down" on a provider. It echoes req.Model
// in the response usage so the test can see which model actually served.
type modelAwareProvider struct {
	name  string
	ok    map[string]bool
	calls atomic.Int64
}

func (p *modelAwareProvider) Name() string { return p.name }
func (p *modelAwareProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.calls.Add(1)
	if !p.ok[req.Model] {
		return nil, fmt.Errorf("%s: model %q unavailable", p.name, req.Model)
	}
	return okResp(req.Model, 1, 1), nil
}

// A task with a model chain falls back MODEL→MODEL: when the primary model's
// whole attempt fails (every provider for it errors), the governor retries with
// the next model in the chain (which routes to its own provider).
func TestTaskModelChain_FallsBackToNextModel(t *testing.T) {
	b, _ := newBus(t)
	r := governor.NewRegistry()
	// alpha is routed for model-a but model-a is "down" (ok is empty → errors).
	alpha := &modelAwareProvider{name: "alpha", ok: map[string]bool{}}
	// beta serves model-b and it works.
	beta := &modelAwareProvider{name: "beta", ok: map[string]bool{"model-b": true}}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "alpha", Provider: alpha, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
		&governor.ProviderInfo{Name: "beta", Provider: beta, AuthMode: governor.AuthAPIKey, Models: []string{"model-b"}},
	)
	g, err := governor.New(governor.Config{
		Registry:        r,
		Bus:             b,
		TaskModelChains: governor.TaskModelChains{"chat": {"model-a", "model-b"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{TaskType: "chat"})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Usage.Model != "model-b" {
		t.Fatalf("expected fallback to model-b, served %q", resp.Usage.Model)
	}
	if beta.calls.Load() == 0 {
		t.Error("beta (model-b) should have been called on fallback")
	}
	if alpha.calls.Load() == 0 {
		t.Error("alpha (model-a) should have been tried first")
	}
}

// The first model that works wins — later chain models aren't tried.
func TestTaskModelChain_PrimaryWinsStops(t *testing.T) {
	r := governor.NewRegistry()
	alpha := &modelAwareProvider{name: "alpha", ok: map[string]bool{"model-a": true}}
	beta := &modelAwareProvider{name: "beta", ok: map[string]bool{"model-b": true}}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "alpha", Provider: alpha, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
		&governor.ProviderInfo{Name: "beta", Provider: beta, AuthMode: governor.AuthAPIKey, Models: []string{"model-b"}},
	)
	g, _ := governor.New(governor.Config{
		Registry:        r,
		TaskModelChains: governor.TaskModelChains{"chat": {"model-a", "model-b"}},
	})
	resp, err := g.Complete(context.Background(), agent.CompletionRequest{TaskType: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.Model != "model-a" {
		t.Fatalf("primary model-a should have served, got %q", resp.Usage.Model)
	}
	if beta.calls.Load() != 0 {
		t.Errorf("beta should not be called when model-a works (got %d)", beta.calls.Load())
	}
}

// No chain for the task → the request's model is used as-is (pre-M703 behaviour).
func TestTaskModelChain_NoChainUsesRequestModel(t *testing.T) {
	r := governor.NewRegistry()
	alpha := &modelAwareProvider{name: "alpha", ok: map[string]bool{"model-a": true}}
	mustRegister(t, r, &governor.ProviderInfo{Name: "alpha", Provider: alpha, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}})
	g, _ := governor.New(governor.Config{Registry: r}) // no chains
	resp, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "model-a", TaskType: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.Model != "model-a" {
		t.Fatalf("no-chain should use request model, got %q", resp.Usage.Model)
	}
}

// SetTaskModelChains swaps the chains at runtime (hot-reload path).
func TestTaskModelChain_SetHotSwaps(t *testing.T) {
	r := governor.NewRegistry()
	alpha := &modelAwareProvider{name: "alpha", ok: map[string]bool{}} // model-a down
	beta := &modelAwareProvider{name: "beta", ok: map[string]bool{"model-b": true}}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "alpha", Provider: alpha, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
		&governor.ProviderInfo{Name: "beta", Provider: beta, AuthMode: governor.AuthAPIKey, Models: []string{"model-b"}},
	)
	g, _ := governor.New(governor.Config{Registry: r})

	// No chain yet: a "chat" request with model-a fails (alpha down, beta won't serve model-a).
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "model-a", TaskType: "chat"}); err == nil {
		t.Fatal("expected failure before chain configured")
	}

	// Configure a chain live → the same task now falls back to model-b.
	g.SetTaskModelChains(map[string][]string{"chat": {"model-a", "model-b"}})
	if got := g.TaskModelChainsView()["chat"]; len(got) != 2 {
		t.Fatalf("view after set: %v", got)
	}
	resp, err := g.Complete(context.Background(), agent.CompletionRequest{TaskType: "chat"})
	if err != nil || resp.Usage.Model != "model-b" {
		t.Fatalf("after hot-swap, expected model-b: resp=%v err=%v", resp, err)
	}
}

func TestParseTaskModelChainsEnv(t *testing.T) {
	got, err := governor.ParseTaskModelChainsEnv("chat=a,b , c ;code=x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got["chat"]) != 3 || got["chat"][0] != "a" || got["chat"][2] != "c" {
		t.Errorf("chat chain: %v", got["chat"])
	}
	if len(got["code"]) != 1 || got["code"][0] != "x" {
		t.Errorf("code chain: %v", got["code"])
	}
	if _, err := governor.ParseTaskModelChainsEnv("bad-entry"); err == nil {
		t.Error("expected error on entry missing '='")
	}
}
