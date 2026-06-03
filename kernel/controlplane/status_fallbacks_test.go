// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// CmdStatus folds provider.fallback events into a count + last reason (M280), so
// a silently-degraded provider is visible at a glance.
func TestStatusSurfacesProviderFallbacks(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// No fallbacks yet → count 0.
	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	fb, _ := res["provider_fallbacks"].(map[string]any)
	if fb == nil {
		t.Fatal("status missing provider_fallbacks")
	}
	if n, _ := fb["count"].(float64); n != 0 {
		t.Errorf("initial fallback count = %v, want 0", fb["count"])
	}

	// Publish two fallback events; the second's reason should be the last shown.
	for _, reason := range []string{"openai: status 500", "openai: status 400 tools[6].name"} {
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "governor.fallback", Kind: event.KindProviderFallback, Actor: "governor",
			Payload: map[string]any{"failed": "localgw", "next": "mock", "reason": reason},
		}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	res, err = c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	fb, _ = res["provider_fallbacks"].(map[string]any)
	if n, _ := fb["count"].(float64); n != 2 {
		t.Errorf("fallback count = %v, want 2", fb["count"])
	}
	if last, _ := fb["last_reason"].(string); last != "openai: status 400 tools[6].name" {
		t.Errorf("last_reason = %q, want the most recent fallback's reason", last)
	}
}
