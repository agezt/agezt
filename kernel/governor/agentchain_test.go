// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
)

// TestPerRequestModelChain_FallsBack: a request carrying its own ModelChain
// (M787 — a named agent's fallbacks) walks it model→model on retryable
// failure, journaling agent-chain-scoped fallback events.
func TestPerRequestModelChain_FallsBack(t *testing.T) {
	b, j := newBus(t)
	r := governor.NewRegistry()
	alpha := &modelAwareProvider{name: "alpha", ok: map[string]bool{}}
	beta := &modelAwareProvider{name: "beta", ok: map[string]bool{"model-b": true}}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "alpha", Provider: alpha, AuthMode: governor.AuthAPIKey, Models: []string{"model-a"}},
		&governor.ProviderInfo{Name: "beta", Provider: beta, AuthMode: governor.AuthAPIKey, Models: []string{"model-b"}},
	)
	g, err := governor.New(governor.Config{Registry: r, Bus: b})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		ModelChain: []string{"model-a", "model-b"},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Usage.Model != "model-b" {
		t.Fatalf("expected fallback to model-b, served %q", resp.Usage.Model)
	}

	// The fallback event is scoped "agent-chain", distinguishable from the
	// per-task chain's events.
	var sawScope string
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != event.KindProviderFallback {
			return nil
		}
		var p struct {
			Scope string `json:"scope"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		sawScope = p.Scope
		return nil
	})
	if sawScope != "agent-chain" {
		t.Errorf("fallback scope = %q, want agent-chain", sawScope)
	}
}

// TestPerRequestModelChain_WinsOverTaskChain: the identity's chain beats the
// task type's configured chain — the more specific routing wins.
func TestPerRequestModelChain_WinsOverTaskChain(t *testing.T) {
	b, _ := newBus(t)
	r := governor.NewRegistry()
	prov := &modelAwareProvider{name: "p", ok: map[string]bool{"agent-model": true, "task-model": true}}
	mustRegister(t, r, &governor.ProviderInfo{
		Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey,
		Models: []string{"agent-model", "task-model"},
	})
	g, err := governor.New(governor.Config{
		Registry:        r,
		Bus:             b,
		TaskModelChains: governor.TaskModelChains{"chat": {"task-model"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		TaskType:   "chat",
		ModelChain: []string{"agent-model"},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Usage.Model != "agent-model" {
		t.Fatalf("served %q, want the per-request chain's agent-model", resp.Usage.Model)
	}
}
