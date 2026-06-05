// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestStanding_CRUDRoundTrip drives the full management surface over the control
// plane: add → list → pause → remove, asserting each step and that every
// mutation is journaled (standing.created/updated/removed) (M403, SPEC-16 §4).
func TestStanding_CRUDRoundTrip(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add a cron-triggered order.
	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":       "portfolio watch",
			"triggers":   []any{map[string]any{"type": "cron", "schedule": "0 8 * * *"}},
			"plan":       "brief me each morning",
			"initiative": map[string]any{"mode": "act_or_ask", "max_trust": "L2"},
		},
	})
	if err != nil {
		t.Fatalf("standing_add: %v", err)
	}
	o, _ := add["order"].(map[string]any)
	id, _ := o["id"].(string)
	if id == "" {
		t.Fatalf("add returned no id: %v", add)
	}
	if en, _ := o["enabled"].(bool); !en {
		t.Error("new order should be enabled")
	}

	// An invalid order is rejected.
	if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{"name": ""},
	}); err == nil {
		t.Error("adding an invalid order should error")
	}

	// List shows it.
	list, err := c.Call(ctx, controlplane.CmdStandingList, nil)
	if err != nil {
		t.Fatalf("standing_list: %v", err)
	}
	if cnt, _ := list["count"].(float64); int(cnt) != 1 {
		t.Errorf("list count = %v, want 1", list["count"])
	}

	// Pause it.
	pause, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": false})
	if err != nil {
		t.Fatalf("standing pause: %v", err)
	}
	po, _ := pause["order"].(map[string]any)
	if en, _ := po["enabled"].(bool); en {
		t.Error("order should be paused")
	}

	// Remove it.
	rm, err := c.Call(ctx, controlplane.CmdStandingRemove, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("standing remove: %v", err)
	}
	if removed, _ := rm["removed"].(bool); !removed {
		t.Error("remove should report removed=true")
	}

	// Every mutation is journaled.
	for _, want := range []event.Kind{event.KindStandingCreated, event.KindStandingUpdated, event.KindStandingRemoved} {
		n := 0
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Kind == want {
				n++
			}
			return nil
		})
		if n == 0 {
			t.Errorf("no %s event journaled", want)
		}
	}
}
