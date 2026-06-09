// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRoutingActivity_SeparatesModelChainFallbacks proves the M706 observability
// split: a model-chain fallback (scope="model-chain") surfaces under the routing
// activity for its task type and as model_fallbacks in status, while a provider
// fallback stays a provider_fallback — the two dimensions are no longer conflated.
func TestRoutingActivity_SeparatesModelChainFallbacks(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// A per-task model-chain fallback (M703/M706 shape).
	k.Bus().Publish(event.Spec{
		Subject: "governor.fallback", Kind: event.KindProviderFallback, Actor: "governor",
		Payload: map[string]any{
			"failed_model": "deepseek-chat", "next_model": "gpt-4o",
			"reason": "deepseek: 503", "scope": "model-chain", "task_type": "chat",
		},
	})
	// A provider-level fallback (M280 shape) — different dimension.
	k.Bus().Publish(event.Spec{
		Subject: "governor.fallback", Kind: event.KindProviderFallback, Actor: "governor",
		Payload: map[string]any{"failed": "openai", "next": "mock", "reason": "429"},
	})

	// routing_get.activity attributes the model-chain fallback to "chat".
	res, err := c.Call(context.Background(), controlplane.CmdRoutingGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	activity, _ := res["activity"].(map[string]any)
	chat, _ := activity["chat"].(map[string]any)
	if chat == nil {
		t.Fatalf("routing activity missing chat: %v", activity)
	}
	if n, _ := chat["fallbacks"].(float64); n != 1 {
		t.Errorf("chat fallbacks = %v, want 1", chat["fallbacks"])
	}
	if chat["last_failed"] != "deepseek-chat" || chat["last_next"] != "gpt-4o" {
		t.Errorf("chat last hop = %v→%v, want deepseek-chat→gpt-4o", chat["last_failed"], chat["last_next"])
	}
	// The provider fallback is NOT misfiled under a task.
	if len(activity) != 1 {
		t.Errorf("activity should hold only the model-chain fallback, got %v", activity)
	}

	// status splits the two counts.
	st, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	prov, _ := st["provider_fallbacks"].(map[string]any)
	model, _ := st["model_fallbacks"].(map[string]any)
	if prov == nil || model == nil {
		t.Fatalf("status missing fallback dimensions: %v", st)
	}
	if pc, _ := prov["count"].(float64); pc != 1 {
		t.Errorf("provider_fallbacks count = %v, want 1 (provider scope only)", prov["count"])
	}
	if mc, _ := model["count"].(float64); mc != 1 {
		t.Errorf("model_fallbacks count = %v, want 1 (model-chain scope only)", model["count"])
	}
}
