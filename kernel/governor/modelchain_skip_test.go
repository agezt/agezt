// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// M955: a model fallback chain entry that NO registered provider serves (and
// that every provider's catalog model list rules out) must be SKIPPED — the
// walk advances to the next model without dispatching a doomed request to the
// primary. This is the fix for the glm-5.1→deepseek fallback storm: glm-5.1 was
// served only by an (unregistered) zhipu provider, so it fell through to the
// deepseek primary and 400'd once per provider in the chain.
func TestModelChain_SkipsUnservableModel(t *testing.T) {
	b, _ := newBus(t)
	r := governor.NewRegistry()
	// trap is the primary (registered first) and 400s on anything — it stands in
	// for deepseek receiving "glm-5.1". It declares its own model list, so the
	// guard knows it cannot serve the mystery model.
	trap := &fakeProvider{name: "trap", err: errors.New("HTTP 400 invalid_request_error")}
	echo := &fakeProvider{name: "echo", resp: okResp("model-a", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "trap", Provider: trap, AuthMode: governor.AuthAPIKey, Models: []string{"model-trap"}},
		&governor.ProviderInfo{Name: "echo", Provider: echo, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
	)
	g, err := governor.New(governor.Config{
		Registry:        r,
		Bus:             b,
		TaskModelChains: governor.TaskModelChains{"chat": {"mystery", "model-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{TaskType: "chat"})
	if err != nil {
		t.Fatalf("chain should succeed on model-a after skipping mystery: %v", err)
	}
	if resp == nil || resp.Usage.Model != "model-a" {
		t.Fatalf("want answer from model-a, got %+v", resp)
	}
	if trap.calls.Load() != 0 {
		t.Errorf("trap (the 400 primary) must NOT be called for the unservable model, got %d calls", trap.calls.Load())
	}
	if echo.calls.Load() != 1 {
		t.Errorf("echo (serves model-a) should be called once, got %d", echo.calls.Load())
	}
}

// When EVERY model in the chain is unservable, Complete fails clean with
// ErrModelUnservable instead of misrouting to a provider that 400s.
func TestModelChain_AllUnservableErrorsClean(t *testing.T) {
	b, _ := newBus(t)
	r := governor.NewRegistry()
	trap := &fakeProvider{name: "trap", err: errors.New("HTTP 400")}
	echo := &fakeProvider{name: "echo", resp: okResp("model-a", 1, 1)}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "trap", Provider: trap, AuthMode: governor.AuthAPIKey, Models: []string{"model-trap"}},
		&governor.ProviderInfo{Name: "echo", Provider: echo, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
	)
	g, _ := governor.New(governor.Config{
		Registry:        r,
		Bus:             b,
		TaskModelChains: governor.TaskModelChains{"chat": {"mystery-1", "mystery-2"}},
	})

	_, err := g.Complete(context.Background(), agent.CompletionRequest{TaskType: "chat"})
	if !errors.Is(err, governor.ErrModelUnservable) {
		t.Fatalf("want ErrModelUnservable, got %v", err)
	}
	if trap.calls.Load() != 0 || echo.calls.Load() != 0 {
		t.Errorf("no provider should be dispatched for all-unservable chain: trap=%d echo=%d", trap.calls.Load(), echo.calls.Load())
	}
}

// An unknown-coverage provider (empty Models — e.g. the mock/echo fallback in
// keyless mode) preserves the legacy fall-through: an unlisted model is NOT
// skipped, because that provider might accept it.
func TestModelChain_UnknownCoverageNotSkipped(t *testing.T) {
	b, _ := newBus(t)
	r := governor.NewRegistry()
	mock := &fakeProvider{name: "mock", resp: okResp("mystery", 1, 1)}
	mustRegister(t, r,
		// Empty Models = unknown coverage.
		&governor.ProviderInfo{Name: "mock", Provider: mock, AuthMode: governor.AuthLocal},
	)
	g, _ := governor.New(governor.Config{
		Registry:        r,
		Bus:             b,
		TaskModelChains: governor.TaskModelChains{"chat": {"mystery"}},
	})

	if _, err := g.Complete(context.Background(), agent.CompletionRequest{TaskType: "chat"}); err != nil {
		t.Fatalf("unknown-coverage provider should still be tried: %v", err)
	}
	if mock.calls.Load() != 1 {
		t.Errorf("mock (empty Models) should be called once, got %d", mock.calls.Load())
	}
}
