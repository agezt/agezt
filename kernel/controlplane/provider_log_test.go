// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestProviderLog_RoutingAndFallbacks — `agt provider log` folds routing.decision
// + provider.fallback newest-first, and --fallbacks keeps only fallbacks (M89).
func TestProviderLog_RoutingAndFallbacks(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	k.Bus().Publish(event.Spec{
		Subject: "governor.route", Kind: event.KindRoutingDecision, Actor: "governor",
		Payload: map[string]any{"primary": "openai", "chain": []string{"openai", "mock"}, "task_type": "chat"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "governor.fallback", Kind: event.KindProviderFallback, Actor: "governor",
		Payload: map[string]any{"failed": "openai", "next": "mock", "reason": "429 rate limited"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdProviderLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := res["events"].([]any)
	if len(all) != 2 {
		t.Fatalf("events = %d want 2", len(all))
	}
	// Newest first: the fallback.
	first, _ := all[0].(map[string]any)
	if first["kind"] != "fallback" || first["failed"] != "openai" || first["next"] != "mock" {
		t.Errorf("newest = %v want fallback openai→mock", first)
	}

	// --fallbacks → just the fallback.
	fres, err := c.Call(context.Background(), controlplane.CmdProviderLog,
		map[string]any{"fallbacks": true})
	if err != nil {
		t.Fatal(err)
	}
	fb, _ := fres["events"].([]any)
	if len(fb) != 1 {
		t.Fatalf("--fallbacks = %d want 1", len(fb))
	}
	m, _ := fb[0].(map[string]any)
	if m["kind"] != "fallback" {
		t.Errorf("--fallbacks returned %v", m["kind"])
	}
}

// TestProviderStats_Aggregates — `agt provider stats` counts routed + fallbacks,
// computes the fallback rate, and breaks both down by provider (M90).
func TestProviderStats_Aggregates(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	route := func(primary string) {
		k.Bus().Publish(event.Spec{
			Subject: "governor.route", Kind: event.KindRoutingDecision, Actor: "governor",
			Payload: map[string]any{"primary": primary, "chain": []string{primary, "mock"}},
		})
	}
	fb := func(failed string) {
		k.Bus().Publish(event.Spec{
			Subject: "governor.fallback", Kind: event.KindProviderFallback, Actor: "governor",
			Payload: map[string]any{"failed": failed, "next": "mock", "reason": "err"},
		})
	}
	route("openai")
	fb("openai")
	route("openai")
	route("anthropic")

	res, err := c.Call(context.Background(), controlplane.CmdProviderStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := res["routed"].(float64); r != 3 {
		t.Errorf("routed = %v want 3", res["routed"])
	}
	if f, _ := res["fallbacks"].(float64); f != 1 {
		t.Errorf("fallbacks = %v want 1", res["fallbacks"])
	}
	// fallback rate = 1/3.
	if rate, _ := res["fallback_rate"].(float64); rate < 0.33 || rate > 0.34 {
		t.Errorf("fallback_rate = %v want ~0.333", rate)
	}
	byP, _ := res["by_primary"].(map[string]any)
	if n, _ := byP["openai"].(float64); n != 2 {
		t.Errorf("by_primary[openai] = %v want 2", byP["openai"])
	}
}

// TestProviderRejections_FoldsCapabilityEvents — `agt provider rejections` folds
// capability.rejected + capability.rerouted newest-first (M92).
func TestProviderRejections_FoldsCapabilityEvents(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	k.Bus().Publish(event.Spec{
		Subject: "governor.capability", Kind: event.KindCapabilityRejected, Actor: "governor",
		Payload: map[string]any{"model": "text-only", "capability": "tool_call"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "governor.capability", Kind: event.KindCapabilityRejected, Actor: "controlplane",
		Payload: map[string]any{"model": "mock", "capability": "vision"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "governor.capability", Kind: event.KindCapabilityRerouted, Actor: "governor",
		Payload: map[string]any{"from_model": "text-only", "to_model": "gpt-tools", "capability": "tool_call"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdProviderRejections, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := res["rejections"].([]any)
	if len(rows) != 3 {
		t.Fatalf("rejections = %d want 3", len(rows))
	}
	// Newest first: the rerouted.
	first, _ := rows[0].(map[string]any)
	if first["kind"] != "rerouted" || first["to_model"] != "gpt-tools" {
		t.Errorf("newest = %v want rerouted → gpt-tools", first)
	}
	// A vision rejection is present.
	var vision bool
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		if m["kind"] == "rejected" && m["capability"] == "vision" && m["model"] == "mock" {
			vision = true
		}
	}
	if !vision {
		t.Errorf("vision rejection not found in %v", rows)
	}
}
