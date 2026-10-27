// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// A run's CONTEXT is what the run IS: the agent's profile, the provenance of
// its wake, the cost ceiling it may spend, the model chain it runs under and
// the tools it may call.
//
// The APPLICATION owns that — the host that starts a run (the control plane's
// dispatcher, the roster's operator wake) decides WHY the run exists and
// WHICH agent runs it, but it must not have to name the runtime's module to
// say so.
//
// These tests hold the decoration end to end: what the application applies is
// what the run actually carries.

func TestDecorateCarriesTheProfileTheWakeAndTheCeiling(t *testing.T) {
	k := runKernel(t)
	p := agentapi.Profile{Slug: "scout", System: true}
	ctx := Decorate(context.Background(), p, agentapi.WakeContext{Source: "operator", Reason: "escalate"}, 500)

	if got := k.WakeContextSource(ctx); got != "operator" {
		t.Errorf("wake source = %q, want %q", got, "operator")
	}
	if got := k.WakeContextReason(ctx); got != "escalate" {
		t.Errorf("wake reason = %q, want %q", got, "escalate")
	}
	if got := k.MaxCostFromCtx(ctx); got != 500 {
		t.Errorf("cost ceiling = %d, want 500", got)
	}
	if !k.SystemAgentFromCtx(ctx) {
		t.Error("the profile's system flag did not travel with the run")
	}
	if got := toolapi.AgentFromContext(ctx); got != "scout" {
		t.Errorf("acting agent = %q, want %q", got, "scout")
	}
}

func TestDecorateWithoutACeilingLeavesTheRunUncapped(t *testing.T) {
	k := runKernel(t)
	ctx := Decorate(context.Background(), agentapi.Profile{Slug: "scout"}, agentapi.WakeContext{Source: "operator"}, 0)
	if got := k.MaxCostFromCtx(ctx); got != 0 {
		t.Errorf("cost ceiling = %d, want 0 (uncapped)", got)
	}
}

func TestDecorateModelsCarriesTheChainAndItsPrimary(t *testing.T) {
	k := runKernel(t)
	ctx := DecorateModels(context.Background(), []string{"model-a", "model-b"})
	chain := k.ModelChainFromCtx(ctx)
	if len(chain) != 2 || chain[0] != "model-a" || chain[1] != "model-b" {
		t.Errorf("model chain = %v, want [model-a model-b]", chain)
	}
	if got, ok := k.ResolveRunModel(ctx); !ok || got != "model-a" {
		t.Errorf("run model = %q (set=%v), want model-a", got, ok)
	}
}

func TestDecorateModelsWithNoChainLeavesTheRunAlone(t *testing.T) {
	k := runKernel(t)
	ctx := DecorateModels(context.Background(), nil)
	if got := k.ModelChainFromCtx(ctx); got != nil {
		t.Errorf("model chain = %v, want none", got)
	}
	if got, ok := k.ResolveRunModel(ctx); ok {
		t.Errorf("run model = %q, want no override", got)
	}
}

func TestDecorateToolsCarriesTheAllowlist(t *testing.T) {
	ctx := DecorateTools(context.Background(), []string{"shell"})
	got, ok := toolapi.ToolsFrom(ctx)
	if !ok {
		t.Fatal("the allowlist did not travel")
	}
	if len(got) != 1 || got[0] != "shell" {
		t.Errorf("allowlist = %v, want [shell]", got)
	}
}
