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
